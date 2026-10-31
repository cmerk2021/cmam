// Package selfupdate updates the cmam binary itself and performs the daily
// background new-version check.
package selfupdate

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/cmerk2021/cmam/internal/config"
	"github.com/cmerk2021/cmam/internal/github"
	"github.com/cmerk2021/cmam/internal/installer"
	"github.com/cmerk2021/cmam/internal/version"
)

// Updater replaces the running cmam executable with the latest release.
type Updater struct {
	GH    *github.Client
	Paths config.Paths
}

// Check returns the latest available version and whether it is newer than
// the running build.
func (u *Updater) Check(ctx context.Context) (latest string, newer bool, err error) {
	latest, err = u.GH.LatestVersion(ctx, config.SelfRepo)
	if err != nil {
		return "", false, err
	}
	if version.Version == "dev" {
		return latest, false, nil
	}
	return latest, github.CompareVersions(latest, version.Version) > 0, nil
}

// Apply downloads, verifies, and atomically installs the given version over
// the running executable. progress may be nil.
func (u *Updater) Apply(ctx context.Context, ver string, progress github.Progress) (string, error) {
	exe, err := os.Executable()
	if err != nil {
		return "", fmt.Errorf("locating the running executable: %w", err)
	}
	exe, err = filepath.EvalSymlinks(exe)
	if err != nil {
		return "", err
	}
	exeDir := filepath.Dir(exe)
	if f, err := os.CreateTemp(exeDir, ".cmam-writetest-*"); err != nil {
		return "", fmt.Errorf("%s is not writable; re-run from a writable location or update manually: %w", exeDir, err)
	} else {
		f.Close()
		os.Remove(f.Name())
	}

	goos, goarch, err := github.SupportedPlatform()
	if err != nil {
		return "", err
	}
	asset := github.AssetName("cmam", ver, goos, goarch)
	tag := github.TagFor(ver)
	archivePath := filepath.Join(u.Paths.DownloadDir, asset)
	sumsPath := filepath.Join(u.Paths.DownloadDir, fmt.Sprintf("cmam_%s_checksums.txt", ver))

	if err := u.GH.Download(ctx, github.DownloadURL(config.SelfRepo, tag, asset), archivePath, progress); err != nil {
		return "", err
	}
	if err := u.GH.Download(ctx, github.DownloadURL(config.SelfRepo, tag, "checksums.txt"), sumsPath, nil); err != nil {
		return "", err
	}
	sumsFile, err := os.Open(sumsPath)
	if err != nil {
		return "", err
	}
	sums, err := installer.ParseChecksums(sumsFile)
	sumsFile.Close()
	if err != nil {
		return "", err
	}
	if _, err := installer.VerifyChecksum(archivePath, asset, sums); err != nil {
		return "", err
	}
	extracted, err := installer.ExtractBinary(archivePath, "cmam", u.Paths.DownloadDir)
	if err != nil {
		return "", err
	}
	defer os.Remove(extracted)
	if err := installer.AtomicInstall(extracted, exe); err != nil {
		return "", err
	}
	return exe, nil
}
