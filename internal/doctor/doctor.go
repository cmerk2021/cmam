// Package doctor implements the health checks behind `cmam doctor`.
package doctor

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/cmerk2021/cmam/internal/config"
	"github.com/cmerk2021/cmam/internal/registry"
	"github.com/cmerk2021/cmam/internal/state"
	"github.com/cmerk2021/cmam/internal/version"
)

// Status is the outcome of one check.
type Status int

// Check outcomes.
const (
	OK Status = iota
	Warning
	Error
)

// Result is one diagnostic finding.
type Result struct {
	Name   string
	Status Status
	Detail string
	// Fix is a short suggestion shown indented under the check.
	Fix string
}

// Run executes every diagnostic check and returns the results.
func Run(ctx context.Context, paths config.Paths, store *state.Store, reg *registry.Client) []Result {
	var results []Result
	results = append(results, checkBinDir(paths)...)
	results = append(results, checkWritable(paths)...)
	results = append(results, checkRegistry(ctx, reg))
	results = append(results, checkRateLimit(ctx))
	results = append(results, checkInstalled(paths, store)...)
	return results
}

func checkBinDir(paths config.Paths) []Result {
	var out []Result
	if _, err := os.Stat(paths.BinDir); err != nil {
		out = append(out, Result{
			Name: paths.BinDir + " exists", Status: Error,
			Detail: "directory is missing",
			Fix:    "mkdir -p " + paths.BinDir,
		})
	} else {
		out = append(out, Result{Name: paths.BinDir + " exists", Status: OK})
	}

	if onPath(paths.BinDir) {
		out = append(out, Result{Name: paths.BinDir + " is on PATH", Status: OK})
	} else {
		out = append(out, Result{
			Name: paths.BinDir + " is on PATH", Status: Error,
			Detail: "installed apps will not be found",
			Fix:    PathHint(paths.BinDir),
		})
	}
	return out
}

// onPath reports whether dir is an entry of $PATH.
func onPath(dir string) bool {
	clean := filepath.Clean(dir)
	for _, p := range filepath.SplitList(os.Getenv("PATH")) {
		if p != "" && filepath.Clean(p) == clean {
			return true
		}
	}
	return false
}

// PathHint returns the exact line to add to the user's shell config, using
// the syntax of the detected shell.
func PathHint(binDir string) string {
	shell := filepath.Base(os.Getenv("SHELL"))
	switch shell {
	case "fish":
		return fmt.Sprintf("add to ~/.config/fish/config.fish:  fish_add_path %s", binDir)
	case "zsh":
		return fmt.Sprintf("add to ~/.zshrc:  export PATH=\"%s:$PATH\"", binDir)
	default:
		return fmt.Sprintf("add to ~/.bashrc:  export PATH=\"%s:$PATH\"", binDir)
	}
}

func checkWritable(paths config.Paths) []Result {
	var out []Result
	for _, dir := range []string{paths.BinDir, paths.DataDir, paths.CacheDir} {
		name := dir + " is writable"
		f, err := os.CreateTemp(dir, ".cmam-doctor-*")
		if err != nil {
			out = append(out, Result{Name: name, Status: Error, Detail: err.Error(),
				Fix: "check ownership and permissions of " + dir})
			continue
		}
		f.Close()
		os.Remove(f.Name())
		out = append(out, Result{Name: name, Status: OK})
	}
	return out
}

func checkRegistry(ctx context.Context, reg *registry.Client) Result {
	cctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	res, err := reg.Load(cctx, false)
	switch {
	case err != nil:
		return Result{Name: "registry reachable", Status: Error, Detail: err.Error(),
			Fix: "check your network connection"}
	case res.Stale:
		return Result{Name: "registry reachable", Status: Warning,
			Detail: fmt.Sprintf("offline; using cached registry from %s ago", res.Age.Round(time.Minute)),
			Fix:    "run `cmam search --refresh` once the network is back"}
	default:
		return Result{Name: "registry reachable", Status: OK,
			Detail: fmt.Sprintf("%d apps, cache age %s", len(res.Apps), res.Age.Round(time.Second))}
	}
}

func checkRateLimit(ctx context.Context) Result {
	name := "GitHub API rate limit"
	token := os.Getenv("GITHUB_TOKEN")
	cctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(cctx, http.MethodGet, "https://api.github.com/rate_limit", nil)
	if err != nil {
		return Result{Name: name, Status: Warning, Detail: err.Error()}
	}
	req.Header.Set("User-Agent", version.UserAgent())
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return Result{Name: name, Status: Warning, Detail: "could not reach api.github.com: " + err.Error(),
			Fix: "check your network connection"}
	}
	defer resp.Body.Close()

	var body struct {
		Resources struct {
			Core struct {
				Remaining int   `json:"remaining"`
				Limit     int   `json:"limit"`
				Reset     int64 `json:"reset"`
			} `json:"core"`
		} `json:"resources"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		return Result{Name: name, Status: Warning, Detail: "unexpected rate-limit response"}
	}
	core := body.Resources.Core
	detail := strconv.Itoa(core.Remaining) + "/" + strconv.Itoa(core.Limit) + " requests remaining"
	if token != "" {
		detail += " (GITHUB_TOKEN set)"
	} else {
		detail += " (GITHUB_TOKEN not set)"
	}
	if core.Remaining == 0 {
		return Result{Name: name, Status: Error,
			Detail: detail + ", resets " + time.Unix(core.Reset, 0).Local().Format("15:04:05"),
			Fix:    "export GITHUB_TOKEN=<a personal access token> to raise the limit"}
	}
	if core.Remaining < 10 {
		return Result{Name: name, Status: Warning, Detail: detail,
			Fix: "set GITHUB_TOKEN to avoid hitting the limit"}
	}
	return Result{Name: name, Status: OK, Detail: detail}
}

func checkInstalled(paths config.Paths, store *state.Store) []Result {
	apps, err := store.Load()
	if err != nil {
		return []Result{{Name: "state file readable", Status: Error, Detail: err.Error(),
			Fix: "inspect or delete " + store.Path}}
	}
	out := []Result{{Name: "state file readable", Status: OK,
		Detail: fmt.Sprintf("%d apps installed", len(apps))}}

	for _, rec := range apps {
		name := "binary present: " + rec.Name
		info, err := os.Stat(rec.BinaryPath)
		switch {
		case err != nil:
			out = append(out, Result{Name: name, Status: Error,
				Detail: rec.BinaryPath + " is missing but recorded in state",
				Fix:    fmt.Sprintf("cmam install %s  (or `cmam uninstall %s` to forget it)", rec.Name, rec.Name)})
			continue
		case info.Mode()&0o111 == 0:
			out = append(out, Result{Name: name, Status: Error,
				Detail: rec.BinaryPath + " is not executable",
				Fix:    "chmod 0755 " + rec.BinaryPath})
			continue
		default:
			out = append(out, Result{Name: name, Status: OK, Detail: "v" + rec.Version})
		}

		if shadow := findShadow(rec.Name, paths.BinDir); shadow != "" {
			out = append(out, Result{Name: "PATH shadowing: " + rec.Name, Status: Warning,
				Detail: shadow + " shadows " + rec.BinaryPath,
				Fix:    "move " + paths.BinDir + " earlier in PATH or remove the other binary"})
		}
	}
	return out
}

// findShadow returns the path of a same-named executable that appears
// earlier in PATH than binDir, or "".
func findShadow(name, binDir string) string {
	cleanBin := filepath.Clean(binDir)
	for _, dir := range filepath.SplitList(os.Getenv("PATH")) {
		if dir == "" {
			continue
		}
		if filepath.Clean(dir) == cleanBin {
			return "" // reached our dir first: no shadowing
		}
		candidate := filepath.Join(dir, name)
		if info, err := os.Stat(candidate); err == nil && !info.IsDir() && info.Mode()&0o111 != 0 {
			if !strings.HasPrefix(candidate, cleanBin) {
				return candidate
			}
		}
	}
	return ""
}
