// Package config holds the repository identifiers and filesystem path
// resolution (XDG) used by every other package. Change the constants here to
// point CMAM at a different owner or registry.
package config

import (
	"fmt"
	"os"
	"path/filepath"
)

// Repository identifiers. Everything else in the codebase derives URLs from
// these constants.
const (
	// Owner is the GitHub account that owns CMAM and its registry.
	Owner = "cmerk2021"
	// SelfRepo is the repository CMAM updates itself from.
	SelfRepo = "cmerk2021/cmam"
	// RegistryRepo is the repository holding app manifests under apps/.
	RegistryRepo = "cmerk2021/cmam-registry"
	// RegistryBranch is the branch of RegistryRepo the CLI reads from.
	RegistryBranch = "main"
)

// EnvHome is the environment variable that overrides every CMAM directory,
// used primarily by tests.
const EnvHome = "CMAM_HOME"

// Paths holds every directory and file location CMAM uses.
type Paths struct {
	// BinDir is where installed binaries live (~/.local/bin).
	BinDir string
	// DataDir holds persistent state such as installed.json.
	DataDir string
	// CacheDir is the root cache directory.
	CacheDir string
	// RegistryCacheDir caches the registry index.
	RegistryCacheDir string
	// DownloadDir holds downloaded archives.
	DownloadDir string
	// LockFile serializes concurrent cmam invocations.
	LockFile string
}

// StateFile returns the path of installed.json.
func (p Paths) StateFile() string { return filepath.Join(p.DataDir, "installed.json") }

// ResolvePaths computes all paths following XDG conventions, honoring the
// CMAM_HOME override.
func ResolvePaths() (Paths, error) {
	if root := os.Getenv(EnvHome); root != "" {
		return Paths{
			BinDir:           filepath.Join(root, "bin"),
			DataDir:          filepath.Join(root, "share", "cmam"),
			CacheDir:         filepath.Join(root, "cache", "cmam"),
			RegistryCacheDir: filepath.Join(root, "cache", "cmam", "registry"),
			DownloadDir:      filepath.Join(root, "cache", "cmam", "downloads"),
			LockFile:         filepath.Join(root, "state", "cmam", "lock"),
		}, nil
	}

	home, err := os.UserHomeDir()
	if err != nil {
		return Paths{}, fmt.Errorf("resolving home directory: %w", err)
	}

	data := os.Getenv("XDG_DATA_HOME")
	if data == "" {
		data = filepath.Join(home, ".local", "share")
	}
	cache := os.Getenv("XDG_CACHE_HOME")
	if cache == "" {
		cache = filepath.Join(home, ".cache")
	}
	stateDir := os.Getenv("XDG_STATE_HOME")
	if stateDir == "" {
		stateDir = filepath.Join(home, ".local", "state")
	}

	cacheRoot := filepath.Join(cache, "cmam")
	return Paths{
		BinDir:           filepath.Join(home, ".local", "bin"),
		DataDir:          filepath.Join(data, "cmam"),
		CacheDir:         cacheRoot,
		RegistryCacheDir: filepath.Join(cacheRoot, "registry"),
		DownloadDir:      filepath.Join(cacheRoot, "downloads"),
		LockFile:         filepath.Join(stateDir, "cmam", "lock"),
	}, nil
}

// EnsureDirs creates every directory CMAM needs with sane permissions.
func (p Paths) EnsureDirs() error {
	for _, d := range []string{p.BinDir, p.DataDir, p.RegistryCacheDir, p.DownloadDir, filepath.Dir(p.LockFile)} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			return fmt.Errorf("creating directory %s: %w", d, err)
		}
	}
	return nil
}

// RegistryIndexURL is the raw URL of the generated registry index.
func RegistryIndexURL() string {
	return fmt.Sprintf("https://raw.githubusercontent.com/%s/%s/index.json", RegistryRepo, RegistryBranch)
}

// RegistryManifestURL is the raw URL of a single manifest.
func RegistryManifestURL(name string) string {
	return fmt.Sprintf("https://raw.githubusercontent.com/%s/%s/apps/%s.toml", RegistryRepo, RegistryBranch, name)
}
