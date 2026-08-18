package roborock

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
)

const retainedTopicsFile = "mqtt-retained-topics.json"

type RetainedTopicLedger struct {
	mu     sync.Mutex
	path   string
	topics map[string]bool
}

func NewRetainedTopicLedger(dataDir string) (*RetainedTopicLedger, error) {
	ledger := &RetainedTopicLedger{path: filepath.Join(dataDir, retainedTopicsFile), topics: make(map[string]bool)}
	data, err := os.ReadFile(ledger.path)
	if err != nil {
		if os.IsNotExist(err) {
			return ledger, nil
		}
		return nil, fmt.Errorf("read retained topic ledger: %w", err)
	}
	var topics []string
	if err := json.Unmarshal(data, &topics); err != nil {
		return nil, fmt.Errorf("parse retained topic ledger: %w", err)
	}
	for _, topic := range topics {
		if topic = strings.TrimSpace(topic); topic != "" {
			ledger.topics[topic] = true
		}
	}
	return ledger, nil
}

// Reconcile atomically stores the topics expected for this run and returns
// topics from the previous successful run that should be deleted retained.
func (l *RetainedTopicLedger) Reconcile(expected []string) ([]string, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	next := make(map[string]bool, len(expected))
	for _, topic := range expected {
		if topic = strings.TrimSpace(topic); topic != "" {
			next[topic] = true
		}
	}
	stale := make([]string, 0)
	for topic := range l.topics {
		if !next[topic] {
			stale = append(stale, topic)
		}
	}
	sort.Strings(stale)
	current := make([]string, 0, len(next))
	for topic := range next {
		current = append(current, topic)
	}
	sort.Strings(current)
	if err := os.MkdirAll(filepath.Dir(l.path), 0700); err != nil {
		return nil, fmt.Errorf("create retained topic ledger directory: %w", err)
	}
	data, err := json.MarshalIndent(current, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("marshal retained topic ledger: %w", err)
	}
	temporary := l.path + ".tmp"
	if err := os.WriteFile(temporary, append(data, '\n'), 0600); err != nil {
		return nil, fmt.Errorf("write retained topic ledger: %w", err)
	}
	if err := os.Rename(temporary, l.path); err != nil {
		return nil, fmt.Errorf("replace retained topic ledger: %w", err)
	}
	l.topics = next
	return stale, nil
}
