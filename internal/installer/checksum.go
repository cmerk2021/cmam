// Package installer implements the download → verify → extract → install
// pipeline, including checksum verification, safe tar extraction, atomic
// replacement with rollback, and uninstall.
package installer

import (
	"bufio"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"strings"
)

// ParseChecksums parses standard `sha256sum` output ("<hash>  <filename>")
// into a filename → hex-hash map.
func ParseChecksums(r io.Reader) (map[string]string, error) {
	sums := map[string]string{}
	sc := bufio.NewScanner(r)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) != 2 {
			return nil, fmt.Errorf("malformed checksums line: %q", line)
		}
		hash := strings.ToLower(fields[0])
		if len(hash) != 64 {
			return nil, fmt.Errorf("invalid sha256 hash in checksums: %q", fields[0])
		}
		// sha256sum may prefix binary-mode filenames with '*'.
		name := strings.TrimPrefix(fields[1], "*")
		sums[name] = hash
	}
	if err := sc.Err(); err != nil {
		return nil, fmt.Errorf("reading checksums: %w", err)
	}
	return sums, nil
}

// FileSHA256 computes the hex SHA-256 of a file.
func FileSHA256(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

// VerifyChecksum checks that the file at path matches the entry for
// assetName in the checksums map. A missing entry is an error: verification
// is never skipped. On mismatch the file is deleted.
func VerifyChecksum(path, assetName string, sums map[string]string) (string, error) {
	want, ok := sums[assetName]
	if !ok {
		return "", fmt.Errorf("checksums.txt has no entry for %s; refusing to install unverified binary", assetName)
	}
	got, err := FileSHA256(path)
	if err != nil {
		return "", fmt.Errorf("hashing %s: %w", path, err)
	}
	if got != want {
		os.Remove(path)
		return "", fmt.Errorf("checksum mismatch for %s: expected %s, got %s (file deleted)", assetName, want, got)
	}
	return got, nil
}
