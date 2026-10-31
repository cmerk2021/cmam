package installer

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/cmerk2021/cmam/internal/config"
	"github.com/cmerk2021/cmam/internal/github"
	"github.com/cmerk2021/cmam/internal/registry"
	"github.com/cmerk2021/cmam/internal/state"
	"github.com/cmerk2021/cmam/internal/ui"
)

// buildFakeRelease returns a valid tar.gz containing the binary and the
// matching checksums.txt content.
func buildFakeRelease(t *testing.T, app, version string) (archive []byte, checksums string, assetName string) {
	t.Helper()
	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gz)
	content := []byte("#!/bin/sh\necho " + app + " " + version + "\n")
	_ = tw.WriteHeader(&tar.Header{Name: app, Mode: 0o755, Size: int64(len(content)), Typeflag: tar.TypeReg})
	_, _ = tw.Write(content)
	tw.Close()
	gz.Close()

	assetName = github.AssetName(app, version, runtime.GOOS, runtime.GOARCH)
	sum := sha256.Sum256(buf.Bytes())
	checksums = fmt.Sprintf("%s  %s\n", hex.EncodeToString(sum[:]), assetName)
	return buf.Bytes(), checksums, assetName
}

func TestInstallEndToEnd(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("install pipeline targets linux")
	}
	archive, checksums, assetName := buildFakeRelease(t, "wgo", "1.2.0")

	mux := http.NewServeMux()
	mux.HandleFunc("/index.json", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `[{"name":"wgo","description":"Watch files","repo":"cmerk2021/wgo"}]`)
	})
	mux.HandleFunc("/cmerk2021/wgo/releases/latest", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Location", "https://github.com/cmerk2021/wgo/releases/tag/v1.2.0")
		w.WriteHeader(http.StatusFound)
	})
	mux.HandleFunc("/cmerk2021/wgo/releases/download/v1.2.0/"+assetName, func(w http.ResponseWriter, r *http.Request) {
		w.Write(archive)
	})
	mux.HandleFunc("/cmerk2021/wgo/releases/download/v1.2.0/checksums.txt", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, checksums)
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	root := t.TempDir()
	t.Setenv(config.EnvHome, root)
	paths, err := config.ResolvePaths()
	if err != nil {
		t.Fatal(err)
	}
	if err := paths.EnsureDirs(); err != nil {
		t.Fatal(err)
	}

	reg := registry.NewClient(srv.URL+"/index.json", srv.URL+"/contents", func(name string) string {
		return srv.URL + "/apps/" + name + ".toml"
	}, paths.RegistryCacheDir)
	reg.HTTP = srv.Client()
	gh := &github.Client{BaseURL: srv.URL, APIBaseURL: srv.URL, HTTP: srv.Client()}

	var out strings.Builder
	ins := &Installer{
		Paths: paths, GH: gh, Registry: reg,
		State:    &state.Store{Path: paths.StateFile()},
		Reporter: ui.NewPlainReporter(&out),
	}

	// Patch DownloadURL indirection: the installer builds URLs from
	// github.DownloadURL which hardcodes github.com, so serve via BaseURL by
	// checking the built URL. Instead, verify the full pipeline through a
	// derived client that rewrites URLs.
	gh.HTTP = rewriteClient(srv)

	res, err := ins.Install(context.Background(), "wgo", "", false)
	if err != nil {
		t.Fatalf("install failed: %v\noutput:\n%s", err, out.String())
	}
	if res.Version != "1.2.0" {
		t.Errorf("version = %q", res.Version)
	}

	// Binary on disk, executable, recorded in state.
	info, err := os.Stat(filepath.Join(paths.BinDir, "wgo"))
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm()&0o111 == 0 {
		t.Error("binary not executable")
	}
	apps, err := ins.State.Load()
	if err != nil || apps["wgo"].Version != "1.2.0" || apps["wgo"].ArchiveSHA256 == "" {
		t.Fatalf("state record wrong: %+v, %v", apps["wgo"], err)
	}

	// Installing the same version again is a friendly no-op.
	res2, err := ins.Install(context.Background(), "wgo", "", false)
	if err != nil || !res2.Skipped {
		t.Fatalf("expected skip, got %+v, %v", res2, err)
	}

	// Uninstall removes binary and record.
	if _, err := ins.Uninstall("wgo"); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(paths.BinDir, "wgo")); !os.IsNotExist(err) {
		t.Error("binary not removed")
	}
}

func TestInstallRefusesForeignBinary(t *testing.T) {
	root := t.TempDir()
	t.Setenv(config.EnvHome, root)
	paths, _ := config.ResolvePaths()
	_ = paths.EnsureDirs()

	// A binary cmam did not install already occupies the destination.
	if err := os.WriteFile(filepath.Join(paths.BinDir, "wgo"), []byte("foreign"), 0o755); err != nil {
		t.Fatal(err)
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/index.json", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `[{"name":"wgo","description":"Watch files","repo":"cmerk2021/wgo"}]`)
	})
	mux.HandleFunc("/cmerk2021/wgo/releases/latest", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Location", "https://github.com/cmerk2021/wgo/releases/tag/v1.2.0")
		w.WriteHeader(http.StatusFound)
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	reg := registry.NewClient(srv.URL+"/index.json", srv.URL+"/contents", func(name string) string {
		return srv.URL + "/apps/" + name + ".toml"
	}, paths.RegistryCacheDir)
	reg.HTTP = srv.Client()

	ins := &Installer{
		Paths:    paths,
		GH:       &github.Client{BaseURL: srv.URL, APIBaseURL: srv.URL, HTTP: srv.Client()},
		Registry: reg, State: &state.Store{Path: paths.StateFile()},
		Reporter: ui.NewPlainReporter(&strings.Builder{}),
	}
	_, err := ins.Install(context.Background(), "wgo", "", false)
	if err == nil || !strings.Contains(err.Error(), "--force") {
		t.Fatalf("expected foreign-binary refusal, got %v", err)
	}
}

// rewriteClient returns an *http.Client that redirects any github.com
// request to the test server, so hardcoded download URLs stay testable.
func rewriteClient(srv *httptest.Server) *http.Client {
	base := srv.Client().Transport
	return &http.Client{Transport: rewriteTransport{base: base, srv: srv}}
}

type rewriteTransport struct {
	base http.RoundTripper
	srv  *httptest.Server
}

func (t rewriteTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	if req.URL.Host == "github.com" {
		clone := req.Clone(req.Context())
		clone.URL.Scheme = "http"
		clone.URL.Host = strings.TrimPrefix(t.srv.URL, "http://")
		return t.base.RoundTrip(clone)
	}
	return t.base.RoundTrip(req)
}
