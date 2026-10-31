package installer

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
)

// AtomicInstall moves srcPath to destPath (same filesystem not required for
// src; it copies into a temp file in the destination directory first, then
// renames). If destPath already exists it is kept as a backup until the
// rename succeeds, and restored on failure.
func AtomicInstall(srcPath, destPath string) (err error) {
	destDir := filepath.Dir(destPath)
	if err := os.MkdirAll(destDir, 0o755); err != nil {
		return err
	}

	tmp, err := os.CreateTemp(destDir, "."+filepath.Base(destPath)+"-*")
	if err != nil {
		return fmt.Errorf("creating temp file in %s: %w", destDir, err)
	}
	tmpPath := tmp.Name()
	defer func() {
		if err != nil {
			os.Remove(tmpPath)
		}
	}()

	src, err := os.Open(srcPath)
	if err != nil {
		tmp.Close()
		return err
	}
	_, copyErr := tmp.ReadFrom(src)
	src.Close()
	if copyErr != nil {
		tmp.Close()
		return fmt.Errorf("copying binary: %w", copyErr)
	}
	if err := tmp.Chmod(0o755); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}

	// Keep the existing binary as a backup until the rename succeeds.
	backup := ""
	if _, statErr := os.Lstat(destPath); statErr == nil {
		backup = destPath + ".cmam-backup"
		if err := os.Rename(destPath, backup); err != nil {
			return fmt.Errorf("backing up existing binary: %w", err)
		}
	}

	if err := os.Rename(tmpPath, destPath); err != nil {
		if backup != "" {
			if rbErr := os.Rename(backup, destPath); rbErr != nil {
				return errors.Join(fmt.Errorf("installing %s: %w", destPath, err),
					fmt.Errorf("rollback failed, backup left at %s: %w", backup, rbErr))
			}
		}
		return fmt.Errorf("installing %s (rolled back): %w", destPath, err)
	}
	if backup != "" {
		os.Remove(backup)
	}
	return nil
}

// RemoveBinary deletes an installed binary; a missing file is not an error.
func RemoveBinary(path string) error {
	if err := os.Remove(path); err != nil && !errors.Is(err, fs.ErrNotExist) {
		return err
	}
	return nil
}
