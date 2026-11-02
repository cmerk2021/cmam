package selfupdate

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"time"

	"github.com/cmerk2021/cmam/internal/config"
	"github.com/cmerk2021/cmam/internal/github"
	"github.com/cmerk2021/cmam/internal/version"
)

// EnvNoUpdateCheck disables the background update check when set.
const EnvNoUpdateCheck = "CMAM_NO_UPDATE_CHECK"

// noticeWait bounds how long a finished command waits for the live check
// before falling back to the last cached result.
var noticeWait = 750 * time.Millisecond

// fetchLatest resolves the latest cmam release; swapped out in tests.
var fetchLatest = func(ctx context.Context) (string, error) {
	return github.NewClient().LatestVersion(ctx, config.SelfRepo)
}

type checkStamp struct {
	LastCheck time.Time `json:"last_check"`
	Latest    string    `json:"latest"`
}

// StartBackgroundCheck kicks off a non-blocking check for a newer cmam
// release on every run. The returned function waits briefly for the result
// and returns a one-line notice ("" when cmam is up to date). If the live
// check has not finished in time, the last successfully fetched version is
// used instead. enabled should be false for non-interactive output.
func StartBackgroundCheck(ctx context.Context, paths config.Paths, enabled bool) func() string {
	if !enabled || os.Getenv(EnvNoUpdateCheck) != "" || version.Version == "dev" {
		return func() string { return "" }
	}
	stampPath := filepath.Join(paths.CacheDir, "update-check.json")
	var cached checkStamp
	if data, err := os.ReadFile(stampPath); err == nil {
		_ = json.Unmarshal(data, &cached)
	}

	fetch := fetchLatest
	result := make(chan string, 1)
	go func() {
		cctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
		defer cancel()
		latest, err := fetch(cctx)
		if err != nil {
			result <- cached.Latest
			return
		}
		data, _ := json.Marshal(checkStamp{LastCheck: time.Now(), Latest: latest})
		_ = os.MkdirAll(paths.CacheDir, 0o755)
		_ = os.WriteFile(stampPath, data, 0o644)
		result <- latest
	}()

	return func() string {
		select {
		case latest := <-result:
			return Notice(latest)
		case <-time.After(noticeWait):
			return Notice(cached.Latest) // never block the user on the courtesy check
		}
	}
}

// Notice returns the out-of-date warning for latest, or "" when the running
// build is current.
func Notice(latest string) string {
	if latest == "" || version.Version == "dev" || github.CompareVersions(latest, version.Version) <= 0 {
		return ""
	}
	return "cmam is out of date (v" + version.Version + " → v" + latest + ") — run `cmam self-update` to update"
}
