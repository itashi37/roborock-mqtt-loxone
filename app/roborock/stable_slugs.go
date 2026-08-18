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

const stableSlugFile = "device-slugs.json"

// StableSlugStore persists the external identifier of each robot by Roborock
// DUID. Renaming a robot in the Roborock app therefore does not rename its API
// paths or MQTT topics.
type StableSlugStore struct {
	mu       sync.Mutex
	file     string
	byDevice map[string]string
}

func NewStableSlugStore(dataDir string) (*StableSlugStore, error) {
	store := &StableSlugStore{file: filepath.Join(dataDir, stableSlugFile), byDevice: make(map[string]string)}
	data, err := os.ReadFile(store.file)
	if err != nil {
		if os.IsNotExist(err) {
			return store, nil
		}
		return nil, fmt.Errorf("read stable device slugs: %w", err)
	}
	if err := json.Unmarshal(data, &store.byDevice); err != nil {
		return nil, fmt.Errorf("parse stable device slugs: %w", err)
	}
	return store, nil
}

func (s *StableSlugStore) Resolve(devices []DeviceInfo) ([]string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	used := make(map[string]bool, len(s.byDevice)+len(devices))
	for _, slug := range s.byDevice {
		if slug = strings.TrimSpace(slug); slug != "" {
			used[slug] = true
		}
	}
	changed := false
	resolved := make([]string, 0, len(devices))
	for index, device := range devices {
		key := strings.TrimSpace(device.DID)
		if key == "" {
			key = fmt.Sprintf("legacy:%s:%s:%d", strings.TrimSpace(device.ProductID), strings.TrimSpace(device.Name), index)
		}
		slug := strings.TrimSpace(s.byDevice[key])
		if slug == "" {
			base := Slugify(device.Name)
			if base == "" {
				base = "robot"
			}
			slug = base
			for suffix := 2; used[slug]; suffix++ {
				slug = fmt.Sprintf("%s-%d", base, suffix)
			}
			s.byDevice[key] = slug
			changed = true
		}
		used[slug] = true
		resolved = append(resolved, slug)
	}
	if changed {
		if err := s.saveLocked(); err != nil {
			return nil, err
		}
	}
	return resolved, nil
}

func (s *StableSlugStore) saveLocked() error {
	if err := os.MkdirAll(filepath.Dir(s.file), 0700); err != nil {
		return fmt.Errorf("create stable slug directory: %w", err)
	}
	// Marshal through a sorted list only to make the persisted JSON stable for
	// diagnostics; it is decoded back into the simple map above.
	keys := make([]string, 0, len(s.byDevice))
	for key := range s.byDevice {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	ordered := make(map[string]string, len(keys))
	for _, key := range keys {
		ordered[key] = s.byDevice[key]
	}
	data, err := json.MarshalIndent(ordered, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal stable device slugs: %w", err)
	}
	temporary := s.file + ".tmp"
	if err := os.WriteFile(temporary, data, 0600); err != nil {
		return fmt.Errorf("write stable device slugs: %w", err)
	}
	if err := os.Rename(temporary, s.file); err != nil {
		return fmt.Errorf("replace stable device slugs: %w", err)
	}
	return nil
}
