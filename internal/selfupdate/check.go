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

// EnvNoUpdateCheck disables the daily background update check when set.
const EnvNoUpdateCheck = "CMAM_NO_UPDATE_CHECK"

type checkStamp struct {
	LastCheck time.Time `json:"last_check"`
	Latest    string    `json:"latest"`
}

// StartBackgroundCheck kicks off a non-blocking, at-most-daily check for a
// newer cmam release. The returned function waits briefly for the result and
// returns a one-line notice ("" when there is nothing to say). isTTY should
// be false for piped/CI output, which disables the check entirely.
func StartBackgroundCheck(ctx context.Context, paths config.Paths, isTTY bool) func() string {
	if !isTTY || os.Getenv(EnvNoUpdateCheck) != "" || version.Version == "dev" {
		return func() string { return "" }
	}
	stampPath := filepath.Join(paths.CacheDir, "update-check.json")
	var stamp checkStamp
	if data, err := os.ReadFile(stampPath); err == nil {
		_ = json.Unmarshal(data, &stamp)
	}

	// A cached result from a previous run is enough for the notice.
	if time.Since(stamp.LastCheck) < 24*time.Hour {
		return func() string { return notice(stamp.Latest) }
	}

	result := make(chan string, 1)
	go func() {
		cctx, cancel := context.WithTimeout(ctx, 5*time.Second)
		defer cancel()
		latest, err := github.NewClient().LatestVersion(cctx, config.SelfRepo)
		if err != nil {
			result <- ""
			return
		}
		data, _ := json.Marshal(checkStamp{LastCheck: time.Now(), Latest: latest})
		_ = os.MkdirAll(paths.CacheDir, 0o755)
		_ = os.WriteFile(stampPath, data, 0o644)
		result <- notice(latest)
	}()

	return func() string {
		select {
		case n := <-result:
			return n
		case <-time.After(250 * time.Millisecond):
			return "" // never block the user on the courtesy check
		}
	}
}

func notice(latest string) string {
	if latest == "" || github.CompareVersions(latest, version.Version) <= 0 {
		return ""
	}
	return "cmam v" + latest + " is available — run `cmam self-update` to upgrade"
}
