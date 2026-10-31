package state

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func testStore(t *testing.T) *Store {
	t.Helper()
	return &Store{Path: filepath.Join(t.TempDir(), "installed.json")}
}

func TestLoadMissingFile(t *testing.T) {
	s := testStore(t)
	apps, err := s.Load()
	if err != nil || len(apps) != 0 {
		t.Fatalf("got %v, %v", apps, err)
	}
}

func TestSetLoadRemove(t *testing.T) {
	s := testStore(t)
	rec := Record{
		Name: "wgo", Version: "1.2.0", Repo: "cmerk2021/wgo",
		InstalledAt: time.Now().UTC(), BinaryPath: "/home/u/.local/bin/wgo",
		ArchiveSHA256: "abc",
	}
	if err := s.Set(rec); err != nil {
		t.Fatal(err)
	}
	apps, err := s.Load()
	if err != nil {
		t.Fatal(err)
	}
	got, ok := apps["wgo"]
	if !ok || got.Version != "1.2.0" || got.Repo != "cmerk2021/wgo" {
		t.Fatalf("round trip failed: %+v", got)
	}

	if err := s.Remove("wgo"); err != nil {
		t.Fatal(err)
	}
	apps, _ = s.Load()
	if len(apps) != 0 {
		t.Fatal("record not removed")
	}
}

func TestCorruptState(t *testing.T) {
	s := testStore(t)
	if err := os.WriteFile(s.Path, []byte("{not json"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Load(); err == nil {
		t.Fatal("expected corruption error")
	}
}

func TestNames(t *testing.T) {
	s := testStore(t)
	_ = s.Set(Record{Name: "zeta"})
	_ = s.Set(Record{Name: "alpha"})
	names := s.Names()
	if len(names) != 2 || names[0] != "alpha" || names[1] != "zeta" {
		t.Fatalf("Names = %v", names)
	}
}

func TestLockExcludes(t *testing.T) {
	path := filepath.Join(t.TempDir(), "lock")
	l1, err := Acquire(path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := Acquire(path); err == nil {
		t.Fatal("second acquire should fail while held")
	}
	if err := l1.Release(); err != nil {
		t.Fatal(err)
	}
	l2, err := Acquire(path)
	if err != nil {
		t.Fatalf("acquire after release failed: %v", err)
	}
	l2.Release()
}
