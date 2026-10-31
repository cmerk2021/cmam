// Package state persists records of installed apps in installed.json and
// serializes concurrent cmam runs with a file lock.
package state

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"time"

	"golang.org/x/sys/unix"
)

// Record describes one installed app.
type Record struct {
	Name          string    `json:"name"`
	Version       string    `json:"version"`
	Repo          string    `json:"repo"`
	InstalledAt   time.Time `json:"installed_at"`
	BinaryPath    string    `json:"binary_path"`
	ArchiveSHA256 string    `json:"archive_sha256"`
}

// Store reads and writes installed.json atomically.
type Store struct {
	// Path is the location of installed.json.
	Path string
}

type fileFormat struct {
	Apps map[string]Record `json:"apps"`
}

// Load reads all records. A missing file yields an empty map; a corrupt file
// yields a descriptive error.
func (s *Store) Load() (map[string]Record, error) {
	data, err := os.ReadFile(s.Path)
	if errors.Is(err, fs.ErrNotExist) {
		return map[string]Record{}, nil
	}
	if err != nil {
		return nil, fmt.Errorf("reading state: %w", err)
	}
	var f fileFormat
	if err := json.Unmarshal(data, &f); err != nil {
		return nil, fmt.Errorf("state file %s is corrupt (%v); fix or delete it", s.Path, err)
	}
	if f.Apps == nil {
		f.Apps = map[string]Record{}
	}
	return f.Apps, nil
}

// Save writes all records atomically (temp file + rename).
func (s *Store) Save(apps map[string]Record) error {
	if err := os.MkdirAll(filepath.Dir(s.Path), 0o755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(fileFormat{Apps: apps}, "", "  ")
	if err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(s.Path), ".installed-*")
	if err != nil {
		return err
	}
	defer os.Remove(tmp.Name())
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmp.Name(), s.Path)
}

// Set upserts a record and saves.
func (s *Store) Set(rec Record) error {
	apps, err := s.Load()
	if err != nil {
		return err
	}
	apps[rec.Name] = rec
	return s.Save(apps)
}

// Remove deletes a record and saves. Removing an absent record is a no-op.
func (s *Store) Remove(name string) error {
	apps, err := s.Load()
	if err != nil {
		return err
	}
	delete(apps, name)
	return s.Save(apps)
}

// Names returns installed app names, sorted.
func (s *Store) Names() []string {
	apps, err := s.Load()
	if err != nil {
		return nil
	}
	names := make([]string, 0, len(apps))
	for n := range apps {
		names = append(names, n)
	}
	sort.Strings(names)
	return names
}

// Lock represents a held exclusive file lock.
type Lock struct{ f *os.File }

// Acquire takes an exclusive flock on path, waiting up to the duration that
// the kernel blocks (non-blocking attempt first for a friendly error).
func Acquire(path string) (*Lock, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return nil, err
	}
	f, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0o644)
	if err != nil {
		return nil, err
	}
	if err := unix.Flock(int(f.Fd()), unix.LOCK_EX|unix.LOCK_NB); err != nil {
		f.Close()
		return nil, fmt.Errorf("another cmam process is running (lock %s held): %w", path, err)
	}
	return &Lock{f: f}, nil
}

// Release drops the lock.
func (l *Lock) Release() error {
	if l == nil || l.f == nil {
		return nil
	}
	defer l.f.Close()
	return unix.Flock(int(l.f.Fd()), unix.LOCK_UN)
}
