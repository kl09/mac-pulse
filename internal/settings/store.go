package settings

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"log/slog"
	"os"
	"path/filepath"
	"slices"
	"sync"
)

const fileName = "settings.json"

// Store guards the current settings; any goroutine may call it.
type Store struct {
	path string
	mu   sync.Mutex
	cur  Settings
}

// Open reads dir/settings.json over the defaults. A missing file, unknown keys and invalid
// values leave the defaults in place. A file that is not JSON does too, and is first copied
// to settings.json.bak: the next Set writes all the defaults over it, and one bad byte must
// not cost the user every setting.
//
// One backup, the newest; a second breakage writes over the first. Name it by
// the time, as the history store does, if both ever matter.
func Open(dir string) (*Store, error) {
	s := &Store{path: filepath.Join(dir, fileName), cur: Default()}
	raw, err := os.ReadFile(s.path)
	if errors.Is(err, fs.ErrNotExist) {
		return s, nil
	}
	if err != nil {
		return nil, fmt.Errorf("read settings: %w", err)
	}
	var saved map[string]json.RawMessage
	if err := json.Unmarshal(raw, &saved); err != nil {
		slog.Warn("settings file is not JSON: kept as "+fileName+".bak, using defaults", "path", s.path, "err", err)
		if err := os.WriteFile(s.path+".bak", raw, 0o600); err != nil {
			return nil, fmt.Errorf("keep the unreadable settings: %w", err)
		}
		return s, nil
	}
	for key, value := range saved {
		if err := s.cur.set(key, value); err != nil {
			slog.Warn("settings file entry ignored", "err", err)
		}
	}
	return s, nil
}

func (s *Store) Get() Settings {
	s.mu.Lock()
	defer s.mu.Unlock()
	cur := s.cur
	cur.MenuBar, cur.AlertMuted, cur.AlertRules = slices.Clone(cur.MenuBar), slices.Clone(cur.AlertMuted), slices.Clone(cur.AlertRules)
	cur.ClockZones, cur.TabOrder, cur.TileOrder = slices.Clone(cur.ClockZones), slices.Clone(cur.TabOrder), slices.Clone(cur.TileOrder)
	return cur
}

// Set validates one key, then saves the file before the new value becomes visible.
func (s *Store) Set(key string, value json.RawMessage) error {
	// The lock is held across a sub-millisecond local write so that two Sets
	// cannot save out of order; split into a save queue if Get ever waits on it.
	s.mu.Lock()
	defer s.mu.Unlock()
	next := s.cur
	if err := next.set(key, value); err != nil {
		return err
	}
	raw, err := json.MarshalIndent(next, "", "  ")
	if err != nil {
		return fmt.Errorf("encode settings: %w", err)
	}
	if err := os.MkdirAll(filepath.Dir(s.path), 0o700); err != nil {
		return fmt.Errorf("create settings dir: %w", err)
	}
	// Write-then-rename: a crash mid-write must not leave a truncated file.
	tmp := s.path + ".tmp"
	if err := os.WriteFile(tmp, raw, 0o600); err != nil {
		return fmt.Errorf("write settings: %w", err)
	}
	if err := os.Rename(tmp, s.path); err != nil {
		return fmt.Errorf("replace settings: %w", err)
	}
	s.cur = next
	return nil
}
