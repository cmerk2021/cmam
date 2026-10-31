package installer

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"time"

	"github.com/cmerk2021/cmam/internal/config"
	"github.com/cmerk2021/cmam/internal/github"
	"github.com/cmerk2021/cmam/internal/registry"
	"github.com/cmerk2021/cmam/internal/state"
	"github.com/cmerk2021/cmam/internal/ui"
)

// Installer orchestrates the full install pipeline for apps.
type Installer struct {
	Paths    config.Paths
	GH       *github.Client
	Registry *registry.Client
	State    *state.Store
	Reporter ui.Reporter
	// Force allows overwriting binaries CMAM did not install.
	Force bool
}

// Result summarizes a completed (or skipped) install.
type Result struct {
	Name    string
	Version string
	Path    string
	// Skipped is true when the requested version was already installed.
	Skipped bool
	// Switched is true when a different version replaced an installed one.
	Switched bool
	// Previous is the previously installed version, if any.
	Previous string
}

// Install installs the named app at the given normalized version (empty for
// latest), driving the Reporter through each step.
func (ins *Installer) Install(ctx context.Context, name, wantVersion string, refresh bool) (Result, error) {
	// Step 1: resolve app and version.
	step := "Resolve"
	ins.Reporter.AppStart(name, "")
	ins.Reporter.StepStart(name, step)
	start := time.Now()

	manifest, err := ins.Registry.Get(ctx, name, refresh)
	if err != nil {
		ins.Reporter.StepFail(name, step, err)
		return Result{}, err
	}
	version := wantVersion
	if version == "" {
		version, err = ins.GH.LatestVersion(ctx, manifest.Repo)
		if err != nil {
			ins.Reporter.StepFail(name, step, err)
			return Result{}, err
		}
	}
	ins.Reporter.AppStart(name, "v"+version)

	installed, err := ins.State.Load()
	if err != nil {
		ins.Reporter.StepFail(name, step, err)
		return Result{}, err
	}
	prev, wasInstalled := installed[name]
	if wasInstalled && prev.Version == version {
		if _, statErr := os.Stat(prev.BinaryPath); statErr == nil {
			ins.Reporter.StepDone(name, step, time.Since(start))
			ins.Reporter.Info(fmt.Sprintf("%s v%s is already installed at %s", name, version, prev.BinaryPath))
			return Result{Name: name, Version: version, Path: prev.BinaryPath, Skipped: true}, nil
		}
	}

	destPath := filepath.Join(ins.Paths.BinDir, name)
	if !wasInstalled && !ins.Force {
		if _, statErr := os.Lstat(destPath); statErr == nil {
			err := fmt.Errorf("%s exists but was not installed by cmam; use --force to overwrite", destPath)
			ins.Reporter.StepFail(name, step, err)
			return Result{}, err
		} else if !errors.Is(statErr, fs.ErrNotExist) {
			ins.Reporter.StepFail(name, step, statErr)
			return Result{}, statErr
		}
	}

	goos, goarch, err := github.SupportedPlatform()
	if err != nil {
		ins.Reporter.StepFail(name, step, err)
		return Result{}, err
	}
	asset := github.AssetName(name, version, goos, goarch)
	tag := github.TagFor(version)
	ins.Reporter.StepDone(name, step, time.Since(start))

	// Step 2: download archive and checksums.
	step = "Download"
	ins.Reporter.StepStart(name, step)
	start = time.Now()
	archivePath := filepath.Join(ins.Paths.DownloadDir, asset)
	sumsPath := filepath.Join(ins.Paths.DownloadDir, fmt.Sprintf("%s_%s_checksums.txt", name, version))
	err = ins.GH.Download(ctx, github.DownloadURL(manifest.Repo, tag, asset), archivePath,
		func(done, total int64) { ins.Reporter.StepProgress(name, step, done, total) })
	if err == nil {
		err = ins.GH.Download(ctx, github.DownloadURL(manifest.Repo, tag, "checksums.txt"), sumsPath, nil)
	}
	if err != nil {
		ins.Reporter.StepFail(name, step, err)
		return Result{}, err
	}
	ins.Reporter.StepDone(name, step, time.Since(start))

	// Step 3: verify SHA-256 against checksums.txt.
	step = "Verify"
	ins.Reporter.StepStart(name, step)
	start = time.Now()
	sumsFile, err := os.Open(sumsPath)
	if err != nil {
		ins.Reporter.StepFail(name, step, err)
		return Result{}, err
	}
	sums, err := ParseChecksums(sumsFile)
	sumsFile.Close()
	if err != nil {
		ins.Reporter.StepFail(name, step, err)
		return Result{}, err
	}
	sha, err := VerifyChecksum(archivePath, asset, sums)
	if err != nil {
		ins.Reporter.StepFail(name, step, err)
		return Result{}, err
	}
	ins.Reporter.StepDone(name, step, time.Since(start))

	// Step 4: extract the binary.
	step = "Extract"
	ins.Reporter.StepStart(name, step)
	start = time.Now()
	extracted, err := ExtractBinary(archivePath, name, ins.Paths.DownloadDir)
	if err != nil {
		ins.Reporter.StepFail(name, step, err)
		return Result{}, err
	}
	defer os.Remove(extracted)
	ins.Reporter.StepDone(name, step, time.Since(start))

	// Step 5: atomically install.
	step = "Install"
	ins.Reporter.StepStart(name, step)
	start = time.Now()
	if err := ctx.Err(); err != nil {
		ins.Reporter.StepFail(name, step, err)
		return Result{}, err
	}
	if err := AtomicInstall(extracted, destPath); err != nil {
		ins.Reporter.StepFail(name, step, err)
		return Result{}, err
	}
	ins.Reporter.StepDone(name, step, time.Since(start))

	// Step 6: record state.
	step = "Record"
	ins.Reporter.StepStart(name, step)
	start = time.Now()
	rec := state.Record{
		Name:          name,
		Version:       version,
		Repo:          manifest.Repo,
		InstalledAt:   time.Now().UTC(),
		BinaryPath:    destPath,
		ArchiveSHA256: sha,
	}
	if err := ins.State.Set(rec); err != nil {
		ins.Reporter.StepFail(name, step, err)
		return Result{}, err
	}
	ins.Reporter.StepDone(name, step, time.Since(start))

	res := Result{Name: name, Version: version, Path: destPath}
	if wasInstalled {
		res.Switched = prev.Version != version
		res.Previous = prev.Version
	}
	return res, nil
}

// Uninstall removes the app's binary and state record.
func (ins *Installer) Uninstall(name string) (state.Record, error) {
	installed, err := ins.State.Load()
	if err != nil {
		return state.Record{}, err
	}
	rec, ok := installed[name]
	if !ok {
		return state.Record{}, fmt.Errorf("%s is not installed", name)
	}
	if err := RemoveBinary(rec.BinaryPath); err != nil {
		return rec, fmt.Errorf("removing %s: %w", rec.BinaryPath, err)
	}
	if err := ins.State.Remove(name); err != nil {
		return rec, err
	}
	return rec, nil
}
