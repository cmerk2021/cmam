package installer

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestParseChecksums(t *testing.T) {
	input := "abc\n" // invalid: too short
	if _, err := ParseChecksums(strings.NewReader(input)); err == nil {
		t.Error("expected error for malformed line")
	}

	h := strings.Repeat("ab", 32)
	good := fmt.Sprintf("%s  wgo_1.2.0_linux_amd64.tar.gz\n%s  *wgo_1.2.0_linux_arm64.tar.gz\n\n# comment\n", h, h)
	sums, err := ParseChecksums(strings.NewReader(good))
	if err != nil {
		t.Fatal(err)
	}
	if sums["wgo_1.2.0_linux_amd64.tar.gz"] != h {
		t.Error("missing amd64 entry")
	}
	if sums["wgo_1.2.0_linux_arm64.tar.gz"] != h {
		t.Error("binary-mode '*' prefix not stripped")
	}
}

func writeFile(t *testing.T, dir, name string, data []byte) string {
	t.Helper()
	p := filepath.Join(dir, name)
	if err := os.WriteFile(p, data, 0o644); err != nil {
		t.Fatal(err)
	}
	return p
}

func TestVerifyChecksum(t *testing.T) {
	dir := t.TempDir()
	data := []byte("archive-bytes")
	sum := sha256.Sum256(data)
	hexSum := hex.EncodeToString(sum[:])
	asset := "wgo_1.0.0_linux_amd64.tar.gz"

	t.Run("match", func(t *testing.T) {
		p := writeFile(t, dir, asset, data)
		got, err := VerifyChecksum(p, asset, map[string]string{asset: hexSum})
		if err != nil || got != hexSum {
			t.Fatalf("got %q, %v", got, err)
		}
	})

	t.Run("mismatch deletes file", func(t *testing.T) {
		p := writeFile(t, dir, asset, data)
		_, err := VerifyChecksum(p, asset, map[string]string{asset: strings.Repeat("00", 32)})
		if err == nil {
			t.Fatal("expected mismatch error")
		}
		if _, statErr := os.Stat(p); !os.IsNotExist(statErr) {
			t.Error("file not deleted on mismatch")
		}
	})

	t.Run("missing entry is an error", func(t *testing.T) {
		p := writeFile(t, dir, asset, data)
		if _, err := VerifyChecksum(p, asset, map[string]string{}); err == nil {
			t.Fatal("expected missing-entry error")
		}
	})
}

// makeTarGz builds a tar.gz with the given entries.
type tarEntry struct {
	name     string
	typeflag byte
	data     []byte
	linkname string
}

func makeTarGz(t *testing.T, entries []tarEntry) string {
	t.Helper()
	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gz)
	for _, e := range entries {
		hdr := &tar.Header{
			Name: e.name, Mode: 0o755, Size: int64(len(e.data)),
			Typeflag: e.typeflag, Linkname: e.linkname,
		}
		if err := tw.WriteHeader(hdr); err != nil {
			t.Fatal(err)
		}
		if len(e.data) > 0 {
			if _, err := tw.Write(e.data); err != nil {
				t.Fatal(err)
			}
		}
	}
	tw.Close()
	gz.Close()
	return writeFile(t, t.TempDir(), "archive.tar.gz", buf.Bytes())
}

func TestExtractBinary(t *testing.T) {
	t.Run("extracts root binary, ignores extras", func(t *testing.T) {
		archive := makeTarGz(t, []tarEntry{
			{name: "LICENSE", typeflag: tar.TypeReg, data: []byte("mit")},
			{name: "README.md", typeflag: tar.TypeReg, data: []byte("readme")},
			{name: "wgo", typeflag: tar.TypeReg, data: []byte("ELFBINARY")},
		})
		dest := t.TempDir()
		out, err := ExtractBinary(archive, "wgo", dest)
		if err != nil {
			t.Fatal(err)
		}
		data, _ := os.ReadFile(out)
		if string(data) != "ELFBINARY" {
			t.Errorf("content = %q", data)
		}
	})

	t.Run("missing binary", func(t *testing.T) {
		archive := makeTarGz(t, []tarEntry{{name: "LICENSE", typeflag: tar.TypeReg, data: []byte("x")}})
		if _, err := ExtractBinary(archive, "wgo", t.TempDir()); err == nil {
			t.Fatal("expected error")
		}
	})

	t.Run("rejects path traversal", func(t *testing.T) {
		archive := makeTarGz(t, []tarEntry{
			{name: "../../wgo", typeflag: tar.TypeReg, data: []byte("evil")},
		})
		_, err := ExtractBinary(archive, "wgo", t.TempDir())
		if err == nil || !strings.Contains(err.Error(), "traversal") {
			t.Fatalf("expected traversal error, got %v", err)
		}
	})

	t.Run("rejects symlink named like the binary", func(t *testing.T) {
		archive := makeTarGz(t, []tarEntry{
			{name: "wgo", typeflag: tar.TypeSymlink, linkname: "/etc/passwd"},
		})
		_, err := ExtractBinary(archive, "wgo", t.TempDir())
		if err == nil || !strings.Contains(err.Error(), "link") {
			t.Fatalf("expected symlink rejection, got %v", err)
		}
	})

	t.Run("ignores nested file with same basename", func(t *testing.T) {
		archive := makeTarGz(t, []tarEntry{
			{name: "sub/wgo", typeflag: tar.TypeReg, data: []byte("nested")},
		})
		if _, err := ExtractBinary(archive, "wgo", t.TempDir()); err == nil {
			t.Fatal("nested binary should not satisfy root-binary expectation")
		}
	})

	t.Run("rejects oversized declared entry", func(t *testing.T) {
		// Craft a header-only archive declaring a huge size.
		var buf bytes.Buffer
		gz := gzip.NewWriter(&buf)
		tw := tar.NewWriter(gz)
		_ = tw.WriteHeader(&tar.Header{Name: "wgo", Mode: 0o755, Size: MaxBinarySize + 1, Typeflag: tar.TypeReg})
		tw.Close()
		gz.Close()
		archive := writeFile(t, t.TempDir(), "big.tar.gz", buf.Bytes())
		_, err := ExtractBinary(archive, "wgo", t.TempDir())
		if err == nil || !strings.Contains(err.Error(), "large") {
			t.Fatalf("expected size-cap error, got %v", err)
		}
	})
}

func TestAtomicInstall(t *testing.T) {
	t.Run("fresh install", func(t *testing.T) {
		dir := t.TempDir()
		src := writeFile(t, dir, "src", []byte("new"))
		dest := filepath.Join(dir, "bin", "wgo")
		if err := AtomicInstall(src, dest); err != nil {
			t.Fatal(err)
		}
		info, err := os.Stat(dest)
		if err != nil {
			t.Fatal(err)
		}
		if info.Mode().Perm() != 0o755 {
			t.Errorf("mode = %v, want 0755", info.Mode().Perm())
		}
	})

	t.Run("replace keeps working binary and cleans backup", func(t *testing.T) {
		dir := t.TempDir()
		dest := writeFile(t, dir, "wgo", []byte("old"))
		src := writeFile(t, dir, "src", []byte("new"))
		if err := AtomicInstall(src, dest); err != nil {
			t.Fatal(err)
		}
		data, _ := os.ReadFile(dest)
		if string(data) != "new" {
			t.Errorf("content = %q", data)
		}
		if _, err := os.Stat(dest + ".cmam-backup"); !os.IsNotExist(err) {
			t.Error("backup not cleaned up after success")
		}
	})

	t.Run("rollback on failure", func(t *testing.T) {
		dir := t.TempDir()
		dest := writeFile(t, dir, "wgo", []byte("old"))
		// A missing source triggers failure before any rename.
		err := AtomicInstall(filepath.Join(dir, "nonexistent"), dest)
		if err == nil {
			t.Fatal("expected error")
		}
		data, readErr := os.ReadFile(dest)
		if readErr != nil || string(data) != "old" {
			t.Errorf("original binary lost: %q, %v", data, readErr)
		}
	})
}
