package installer

import (
	"archive/tar"
	"compress/gzip"
	"fmt"
	"io"
	"os"
	"path"
	"path/filepath"
)

// MaxBinarySize caps extraction to guard against decompression bombs.
const MaxBinarySize = 512 << 20 // 512 MiB

// ExtractBinary extracts the single expected binary (named binName at the
// archive root) from the tar.gz at archivePath into destDir, returning the
// extracted file path. It rejects path traversal, refuses symlinks matching
// the target name, and caps the extracted size.
func ExtractBinary(archivePath, binName, destDir string) (string, error) {
	f, err := os.Open(archivePath)
	if err != nil {
		return "", err
	}
	defer f.Close()

	gz, err := gzip.NewReader(f)
	if err != nil {
		return "", fmt.Errorf("opening gzip stream: %w", err)
	}
	defer gz.Close()

	tr := tar.NewReader(gz)
	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			return "", fmt.Errorf("archive does not contain a %q binary at its root", binName)
		}
		if err != nil {
			return "", fmt.Errorf("reading archive: %w", err)
		}

		name := path.Clean(hdr.Name)
		if path.IsAbs(name) || name == ".." || len(name) >= 3 && name[:3] == "../" {
			return "", fmt.Errorf("archive entry %q attempts path traversal", hdr.Name)
		}
		// Only the binary at the root matters; everything else (LICENSE,
		// README, directories) is ignored.
		if path.Base(name) != binName || name != binName {
			continue
		}
		switch hdr.Typeflag {
		case tar.TypeReg:
		case tar.TypeSymlink, tar.TypeLink:
			return "", fmt.Errorf("archive entry %q is a link, expected a regular file", hdr.Name)
		default:
			continue
		}
		if hdr.Size > MaxBinarySize {
			return "", fmt.Errorf("archive entry %q is too large (%d bytes, max %d)", hdr.Name, hdr.Size, int64(MaxBinarySize))
		}

		out := filepath.Join(destDir, binName+".extracted")
		dst, err := os.OpenFile(out, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0o755)
		if err != nil {
			return "", err
		}
		n, err := io.Copy(dst, io.LimitReader(tr, MaxBinarySize+1))
		closeErr := dst.Close()
		if err != nil {
			os.Remove(out)
			return "", fmt.Errorf("extracting %s: %w", binName, err)
		}
		if closeErr != nil {
			os.Remove(out)
			return "", closeErr
		}
		if n > MaxBinarySize {
			os.Remove(out)
			return "", fmt.Errorf("extracted %s exceeds the %d byte size cap", binName, int64(MaxBinarySize))
		}
		return out, nil
	}
}
