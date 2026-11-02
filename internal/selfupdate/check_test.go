package selfupdate

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/cmerk2021/cmam/internal/config"
	"github.com/cmerk2021/cmam/internal/version"
)

func withVersion(t *testing.T, v string) {
	t.Helper()
	old := version.Version
	version.Version = v
	t.Cleanup(func() { version.Version = old })
}

func withFetch(t *testing.T, f func(context.Context) (string, error)) {
	t.Helper()
	old := fetchLatest
	fetchLatest = f
	t.Cleanup(func() { fetchLatest = old })
}

func TestNotice(t *testing.T) {
	withVersion(t, "1.0.0")
	if n := Notice("1.0.0"); n != "" {
		t.Fatalf("up to date should be silent, got %q", n)
	}
	if n := Notice("0.9.0"); n != "" {
		t.Fatalf("older release should be silent, got %q", n)
	}
	n := Notice("1.1.0")
	if !strings.Contains(n, "out of date") || !strings.Contains(n, "cmam self-update") || !strings.Contains(n, "v1.1.0") {
		t.Fatalf("unexpected notice %q", n)
	}
}

func TestBackgroundCheckEveryRun(t *testing.T) {
	withVersion(t, "1.0.0")
	t.Setenv(EnvNoUpdateCheck, "")
	paths := config.Paths{CacheDir: t.TempDir()}
	calls := 0
	withFetch(t, func(context.Context) (string, error) { calls++; return "1.2.0", nil })

	for i := 0; i < 2; i++ {
		if n := StartBackgroundCheck(context.Background(), paths, true)(); !strings.Contains(n, "v1.2.0") {
			t.Fatalf("run %d: expected notice, got %q", i, n)
		}
	}
	if calls != 2 {
		t.Fatalf("expected a check on every run, got %d", calls)
	}

	// A failing or slow check falls back to the cached result.
	withFetch(t, func(context.Context) (string, error) { return "", errors.New("offline") })
	if n := StartBackgroundCheck(context.Background(), paths, true)(); !strings.Contains(n, "v1.2.0") {
		t.Fatalf("expected cached notice when offline, got %q", n)
	}
	old := noticeWait
	noticeWait = 10 * time.Millisecond
	t.Cleanup(func() { noticeWait = old })
	block := make(chan struct{})
	defer close(block)
	withFetch(t, func(context.Context) (string, error) { <-block; return "", errors.New("slow") })
	if n := StartBackgroundCheck(context.Background(), paths, true)(); !strings.Contains(n, "v1.2.0") {
		t.Fatalf("expected cached notice when slow, got %q", n)
	}
}

func TestBackgroundCheckDisabled(t *testing.T) {
	withVersion(t, "1.0.0")
	withFetch(t, func(context.Context) (string, error) { t.Fatal("should not fetch"); return "", nil })
	paths := config.Paths{CacheDir: t.TempDir()}
	if n := StartBackgroundCheck(context.Background(), paths, false)(); n != "" {
		t.Fatalf("non-interactive should be silent, got %q", n)
	}
	t.Setenv(EnvNoUpdateCheck, "1")
	if n := StartBackgroundCheck(context.Background(), paths, true)(); n != "" {
		t.Fatalf("env opt-out should be silent, got %q", n)
	}
}
