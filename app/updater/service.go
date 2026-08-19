package updater

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/mqtt-home/roborock-mqtt/supervisor"
)

const AllowedImage = "ghcr.io/itashi37/roborock-mqtt-loxone"

var allowedTag = regexp.MustCompile(`^(edge|latest|v[0-9]+\.[0-9]+\.[0-9]+|[0-9]+|[0-9]+\.[0-9]+)$`)
var requestIDPattern = regexp.MustCompile(`^[A-Za-z0-9-]+$`)
var commitPattern = regexp.MustCompile(`^[0-9a-fA-F]{7,40}$`)

type Stage string

const (
	StageIdle       Stage = "idle"
	StagePreparing  Stage = "preparing"
	StagePulling    Stage = "pulling"
	StageBackingUp  Stage = "backing_up"
	StageRestarting Stage = "restarting"
	StageValidating Stage = "validating"
	StageSuccess    Stage = "success"
	StageRollback   Stage = "rollback"
	StageFailed     Stage = "failed"
)

type Operation struct {
	ID              string    `json:"id,omitempty"`
	Stage           Stage     `json:"stage"`
	Tag             string    `json:"tag,omitempty"`
	ExpectedVersion string    `json:"expected_version,omitempty"`
	ExpectedCommit  string    `json:"expected_commit,omitempty"`
	PreviousImage   string    `json:"previous_image,omitempty"`
	TargetImage     string    `json:"target_image,omitempty"`
	BackupPath      string    `json:"backup_path,omitempty"`
	StartedAt       time.Time `json:"started_at,omitempty"`
	UpdatedAt       time.Time `json:"updated_at,omitempty"`
	CompletedAt     time.Time `json:"completed_at,omitempty"`
	Error           string    `json:"error,omitempty"`
	RollbackError   string    `json:"rollback_error,omitempty"`
}

type Request struct {
	RequestID       string `json:"request_id"`
	Tag             string `json:"tag"`
	ExpectedVersion string `json:"expected_version"`
	ExpectedCommit  string `json:"expected_commit,omitempty"`
}

type Dependencies struct {
	Supervisor     supervisor.UpdateSupervisor
	ContainerName  string
	HealthURL      string
	DataDir        string
	MinimumFree    uint64
	HealthTimeout  time.Duration
	FreeBytes      func(string) (uint64, error)
	RegistryCheck  func(context.Context) error
	Backup         func(string, string) (string, error)
	Now            func() time.Time
	TargetArtifact func(string) (string, error)
}

type Service struct {
	dependencies    Dependencies
	mu              sync.RWMutex
	operation       Operation
	running         bool
	statePath       string
	usedPath        string
	transactionPath string
	usedIDs         []string
	usedSet         map[string]struct{}
}

func NewService(dependencies Dependencies) (*Service, error) {
	if dependencies.Supervisor == nil {
		return nil, fmt.Errorf("update supervisor is required")
	}
	if dependencies.ContainerName == "" {
		dependencies.ContainerName = "roborock-mqtt-loxone"
	}
	if dependencies.DataDir == "" {
		dependencies.DataDir = "/bridge-data"
	}
	if dependencies.MinimumFree == 0 {
		dependencies.MinimumFree = 512 * 1024 * 1024
	}
	if dependencies.HealthTimeout <= 0 {
		dependencies.HealthTimeout = 3 * time.Minute
	}
	if dependencies.Now == nil {
		dependencies.Now = time.Now
	}
	if dependencies.TargetArtifact == nil {
		dependencies.TargetArtifact = func(tag string) (string, error) {
			if !allowedTag.MatchString(tag) {
				return "", fmt.Errorf("tag is not allowed")
			}
			return AllowedImage + ":" + tag, nil
		}
	}
	if dependencies.FreeBytes == nil {
		dependencies.FreeBytes = FilesystemFreeBytes
	}
	if dependencies.RegistryCheck == nil {
		dependencies.RegistryCheck = CheckRegistry
	}
	if dependencies.Backup == nil {
		dependencies.Backup = BackupData
	}
	service := &Service{dependencies: dependencies, operation: Operation{Stage: StageIdle}, statePath: filepath.Join(dependencies.DataDir, "update-operation.json"), usedPath: filepath.Join(dependencies.DataDir, "update-request-ids.json"), transactionPath: filepath.Join(dependencies.DataDir, "update-transaction.json"), usedSet: make(map[string]struct{})}
	if data, err := os.ReadFile(service.usedPath); err == nil {
		_ = json.Unmarshal(data, &service.usedIDs)
		for _, id := range service.usedIDs {
			service.usedSet[id] = struct{}{}
		}
	}
	if data, err := os.ReadFile(service.statePath); err == nil {
		_ = json.Unmarshal(data, &service.operation)
		if !terminal(service.operation.Stage) {
			service.recoverInterrupted()
		}
	}
	return service, nil
}

func (s *Service) recoverInterrupted() {
	var replacement supervisor.Replacement
	data, readErr := os.ReadFile(s.transactionPath)
	if readErr == nil && json.Unmarshal(data, &replacement) == nil && replacement.PreviousInstance != "" {
		s.operation.Stage = StageRollback
		s.operation.UpdatedAt = s.dependencies.Now().UTC()
		_ = s.persistLocked()
		ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
		err := s.dependencies.Supervisor.Rollback(ctx, replacement)
		cancel()
		s.operation.Stage = StageFailed
		s.operation.Error = "updater restarted during replacement; the previous service was restored"
		if err != nil {
			s.operation.Error = "updater restarted during replacement"
			s.operation.RollbackError = redactError(err)
		} else {
			_ = os.Remove(s.transactionPath)
		}
	} else {
		s.operation.Stage = StageFailed
		s.operation.Error = "updater restarted during an incomplete operation before service replacement"
	}
	s.operation.UpdatedAt = s.dependencies.Now().UTC()
	s.operation.CompletedAt = s.operation.UpdatedAt
	_ = s.persistLocked()
}

func (s *Service) Status() Operation {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.operation
}

func (s *Service) Start(request Request) (Operation, error) {
	request.RequestID = strings.TrimSpace(request.RequestID)
	request.Tag = strings.TrimSpace(request.Tag)
	request.ExpectedVersion = strings.TrimPrefix(strings.TrimSpace(request.ExpectedVersion), "v")
	request.ExpectedCommit = strings.ToLower(strings.TrimSpace(request.ExpectedCommit))
	if len(request.RequestID) < 8 || len(request.RequestID) > 64 || !requestIDPattern.MatchString(request.RequestID) {
		return Operation{}, fmt.Errorf("invalid request_id")
	}
	if !allowedTag.MatchString(request.Tag) {
		return Operation{}, fmt.Errorf("tag is not allowed")
	}
	if request.ExpectedVersion == "" {
		return Operation{}, fmt.Errorf("expected_version is required")
	}
	if request.ExpectedCommit != "" && !commitPattern.MatchString(request.ExpectedCommit) {
		return Operation{}, fmt.Errorf("invalid expected_commit")
	}
	if request.Tag == "edge" && request.ExpectedCommit == "" {
		return Operation{}, fmt.Errorf("expected_commit is required for edge updates")
	}
	s.mu.Lock()
	if s.running {
		operation := s.operation
		s.mu.Unlock()
		return operation, fmt.Errorf("an update is already running")
	}
	if _, used := s.usedSet[request.RequestID]; used {
		operation := s.operation
		s.mu.Unlock()
		return operation, fmt.Errorf("request_id has already been used")
	}
	now := s.dependencies.Now().UTC()
	targetArtifact, err := s.dependencies.TargetArtifact(request.Tag)
	if err != nil {
		s.mu.Unlock()
		return Operation{}, err
	}
	s.operation = Operation{ID: request.RequestID, Stage: StagePreparing, Tag: request.Tag, ExpectedVersion: request.ExpectedVersion, ExpectedCommit: request.ExpectedCommit, TargetImage: targetArtifact, StartedAt: now, UpdatedAt: now}
	s.usedIDs = append(s.usedIDs, request.RequestID)
	if len(s.usedIDs) > 50 {
		delete(s.usedSet, s.usedIDs[0])
		s.usedIDs = s.usedIDs[len(s.usedIDs)-50:]
	}
	s.usedSet[request.RequestID] = struct{}{}
	s.running = true
	_ = s.persistUsedLocked()
	_ = s.persistLocked()
	operation := s.operation
	s.mu.Unlock()
	go s.run(operation)
	return operation, nil
}

func (s *Service) run(operation Operation) {
	ctx := context.Background()
	var replacement supervisor.Replacement
	fail := func(err error, rollback bool) {
		message := redactError(err)
		if rollback && replacement.PreviousInstance != "" {
			s.setStage(StageRollback, "")
			if rollbackErr := s.dependencies.Supervisor.Rollback(ctx, replacement); rollbackErr != nil {
				s.finish(StageFailed, message, redactError(rollbackErr))
				return
			}
		}
		s.finish(StageFailed, message, "")
	}

	free, err := s.dependencies.FreeBytes(s.dependencies.DataDir)
	if err != nil {
		fail(fmt.Errorf("check data volume free space: %w", err), false)
		return
	}
	if free < s.dependencies.MinimumFree {
		fail(fmt.Errorf("insufficient free space: %d bytes available", free), false)
		return
	}
	if err := s.dependencies.RegistryCheck(ctx); err != nil {
		fail(fmt.Errorf("registry unavailable: %w", err), false)
		return
	}
	previous, err := s.dependencies.Supervisor.CurrentArtifact(ctx, s.dependencies.ContainerName)
	if err != nil {
		fail(fmt.Errorf("inspect current bridge: %w", err), false)
		return
	}
	s.update(func(current *Operation) { current.PreviousImage = previous })

	s.setStage(StagePulling, "")
	if err := s.dependencies.Supervisor.Fetch(ctx, operation.TargetImage); err != nil {
		fail(fmt.Errorf("pull target image: %w", err), false)
		return
	}

	s.setStage(StageBackingUp, "")
	backupPath, err := s.dependencies.Backup(s.dependencies.DataDir, operation.ID)
	if err != nil {
		fail(fmt.Errorf("backup data: %w", err), false)
		return
	}
	s.update(func(current *Operation) { current.BackupPath = backupPath })

	s.setStage(StageRestarting, "")
	replacement, err = s.dependencies.Supervisor.Prepare(ctx, s.dependencies.ContainerName, operation.TargetImage)
	if err != nil {
		fail(fmt.Errorf("prepare bridge replacement: %w", err), false)
		return
	}
	if err := s.persistTransaction(replacement); err != nil {
		fail(fmt.Errorf("persist update transaction: %w", err), false)
		return
	}
	if err := s.dependencies.Supervisor.Activate(ctx, &replacement); err != nil {
		fail(fmt.Errorf("activate bridge replacement: %w", err), true)
		return
	}
	_ = s.persistTransaction(replacement)

	s.setStage(StageValidating, "")
	if err := s.dependencies.Supervisor.WaitHealthy(ctx, s.dependencies.ContainerName, s.dependencies.HealthTimeout); err != nil {
		fail(fmt.Errorf("new bridge is unhealthy: %w", err), true)
		return
	}
	if err := s.dependencies.Supervisor.VerifyVersion(ctx, s.dependencies.HealthURL, operation.ExpectedVersion, operation.ExpectedCommit); err != nil {
		fail(fmt.Errorf("new bridge artifact mismatch: %w", err), true)
		return
	}
	if err := s.dependencies.Supervisor.Finalize(ctx, replacement); err != nil {
		fail(fmt.Errorf("finalize update: %w", err), true)
		return
	}
	s.finish(StageSuccess, "", "")
}

func (s *Service) setStage(stage Stage, errorMessage string) {
	s.update(func(operation *Operation) { operation.Stage, operation.Error = stage, errorMessage })
}

func (s *Service) update(change func(*Operation)) {
	s.mu.Lock()
	change(&s.operation)
	s.operation.UpdatedAt = s.dependencies.Now().UTC()
	_ = s.persistLocked()
	s.mu.Unlock()
}

func (s *Service) finish(stage Stage, errorMessage, rollbackError string) {
	s.mu.Lock()
	now := s.dependencies.Now().UTC()
	s.operation.Stage, s.operation.Error, s.operation.RollbackError = stage, errorMessage, rollbackError
	s.operation.UpdatedAt, s.operation.CompletedAt = now, now
	s.running = false
	_ = s.persistLocked()
	if rollbackError == "" {
		_ = os.Remove(s.transactionPath)
	}
	s.mu.Unlock()
}

func (s *Service) persistTransaction(replacement supervisor.Replacement) error {
	data, err := json.MarshalIndent(replacement, "", "  ")
	if err != nil {
		return err
	}
	temporary := s.transactionPath + ".tmp-" + randomSuffix()
	if err := os.WriteFile(temporary, append(data, '\n'), 0600); err != nil {
		return err
	}
	return os.Rename(temporary, s.transactionPath)
}

func (s *Service) persistLocked() error {
	data, err := json.MarshalIndent(s.operation, "", "  ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(s.statePath), 0700); err != nil {
		return err
	}
	temporary := s.statePath + ".tmp-" + randomSuffix()
	if err := os.WriteFile(temporary, append(data, '\n'), 0600); err != nil {
		return err
	}
	return os.Rename(temporary, s.statePath)
}

func (s *Service) persistUsedLocked() error {
	data, err := json.MarshalIndent(s.usedIDs, "", "  ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(s.usedPath), 0700); err != nil {
		return err
	}
	temporary := s.usedPath + ".tmp-" + randomSuffix()
	if err := os.WriteFile(temporary, append(data, '\n'), 0600); err != nil {
		return err
	}
	return os.Rename(temporary, s.usedPath)
}

func terminal(stage Stage) bool {
	return stage == StageIdle || stage == StageSuccess || stage == StageFailed
}

func randomSuffix() string {
	buffer := make([]byte, 6)
	_, _ = rand.Read(buffer)
	return hex.EncodeToString(buffer)
}

func redactError(err error) string {
	if err == nil {
		return ""
	}
	message := err.Error()
	if len(message) > 500 {
		message = message[:500]
	}
	return message
}
