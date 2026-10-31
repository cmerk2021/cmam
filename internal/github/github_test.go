package github

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"
)

func TestParseAppSpec(t *testing.T) {
	cases := []struct {
		spec, name, ver string
		wantErr         bool
	}{
		{"wgo", "wgo", "", false},
		{"wgo@1.2.0", "wgo", "1.2.0", false},
		{"wgo@v1.2.0", "wgo", "1.2.0", false},
		{"wgo@", "", "", true},
		{"@1.0.0", "", "", true},
		{"wgo@banana", "", "", true},
	}
	for _, tc := range cases {
		name, ver, err := ParseAppSpec(tc.spec)
		if tc.wantErr {
			if err == nil {
				t.Errorf("%q: expected error", tc.spec)
			}
			continue
		}
		if err != nil || name != tc.name || ver != tc.ver {
			t.Errorf("%q: got (%q, %q, %v), want (%q, %q)", tc.spec, name, ver, err, tc.name, tc.ver)
		}
	}
}

func TestNormalizeVersion(t *testing.T) {
	for _, in := range []string{"1.2.0", "v1.2.0"} {
		got, err := NormalizeVersion(in)
		if err != nil || got != "1.2.0" {
			t.Errorf("NormalizeVersion(%q) = %q, %v", in, got, err)
		}
	}
	if _, err := NormalizeVersion("not-a-version"); err == nil {
		t.Error("expected error for invalid version")
	}
}

func TestCompareVersions(t *testing.T) {
	cases := []struct {
		a, b string
		want int
	}{
		{"1.2.0", "1.1.0", 1},
		{"1.2.0", "1.2.0", 0},
		{"1.2.0", "1.10.0", -1},
		{"2024.01", "2024.02", -1}, // non-semver falls back to string compare
	}
	for _, tc := range cases {
		if got := CompareVersions(tc.a, tc.b); got != tc.want {
			t.Errorf("CompareVersions(%q, %q) = %d, want %d", tc.a, tc.b, got, tc.want)
		}
	}
}

func TestAssetName(t *testing.T) {
	got := AssetName("wgo", "1.2.0", "linux", "amd64")
	want := "wgo_1.2.0_linux_amd64.tar.gz"
	if got != want {
		t.Errorf("AssetName = %q, want %q", got, want)
	}
}

func TestDownloadURL(t *testing.T) {
	got := DownloadURL("cmerk2021/wgo", "v1.2.0", "wgo_1.2.0_linux_amd64.tar.gz")
	want := "https://github.com/cmerk2021/wgo/releases/download/v1.2.0/wgo_1.2.0_linux_amd64.tar.gz"
	if got != want {
		t.Errorf("DownloadURL = %q, want %q", got, want)
	}
}

func newTestGH(srv *httptest.Server) *Client {
	return &Client{BaseURL: srv.URL, APIBaseURL: srv.URL, HTTP: srv.Client()}
}

func TestLatestTagViaRedirect(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/cmerk2021/wgo/releases/latest" {
			w.Header().Set("Location", "https://github.com/cmerk2021/wgo/releases/tag/v1.2.0")
			w.WriteHeader(http.StatusFound)
			return
		}
		http.NotFound(w, r)
	}))
	defer srv.Close()

	tag, err := newTestGH(srv).LatestTag(context.Background(), "cmerk2021/wgo")
	if err != nil || tag != "v1.2.0" {
		t.Fatalf("LatestTag = %q, %v", tag, err)
	}
}

func TestLatestTagAPIFallback(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/cmerk2021/wgo/releases/latest":
			w.WriteHeader(http.StatusOK) // no redirect: forces fallback
		case "/repos/cmerk2021/wgo/releases/latest":
			w.Write([]byte(`{"tag_name":"v2.0.0","draft":false,"prerelease":false}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	tag, err := newTestGH(srv).LatestTag(context.Background(), "cmerk2021/wgo")
	if err != nil || tag != "v2.0.0" {
		t.Fatalf("LatestTag = %q, %v", tag, err)
	}
}

func TestRateLimitError(t *testing.T) {
	reset := time.Now().Add(30 * time.Minute).Unix()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/cmerk2021/wgo/releases/latest":
			w.WriteHeader(http.StatusOK) // force API fallback
		case "/repos/cmerk2021/wgo/releases/latest":
			w.Header().Set("X-RateLimit-Remaining", "0")
			w.Header().Set("X-RateLimit-Reset", strconv.FormatInt(reset, 10))
			w.WriteHeader(http.StatusForbidden)
			w.Write([]byte(`{"message":"API rate limit exceeded"}`))
		}
	}))
	defer srv.Close()

	_, err := newTestGH(srv).LatestTag(context.Background(), "cmerk2021/wgo")
	rl, ok := err.(*RateLimitError)
	if !ok {
		t.Fatalf("expected *RateLimitError, got %T: %v", err, err)
	}
	if rl.Reset.Unix() != reset {
		t.Errorf("Reset = %v, want unix %d", rl.Reset, reset)
	}
	if rl.Authenticated {
		t.Error("expected Authenticated=false")
	}
	msg := rl.Error()
	if msg == "" || !strings.Contains(msg, "rate limit") || !strings.Contains(msg, "GITHUB_TOKEN") {
		t.Errorf("unhelpful rate limit message: %q", msg)
	}
}

func TestDownloadWritesPartThenRenames(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("hello world"))
	}))
	defer srv.Close()

	dest := filepath.Join(t.TempDir(), "file.bin")
	var sawProgress bool
	err := newTestGH(srv).Download(context.Background(), srv.URL+"/x", dest, func(done, total int64) {
		sawProgress = true
	})
	if err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(dest)
	if err != nil || string(data) != "hello world" {
		t.Fatalf("file content = %q, %v", data, err)
	}
	if !sawProgress {
		t.Error("progress callback never fired")
	}
	if _, err := os.Stat(dest + ".part"); !os.IsNotExist(err) {
		t.Error(".part file left behind")
	}
}

func TestDownloadCancelRemovesPartial(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Length", "1000000")
		w.Write(make([]byte, 1000))
		if f, ok := w.(http.Flusher); ok {
			f.Flush()
		}
		cancel()
		time.Sleep(200 * time.Millisecond)
	}))
	defer srv.Close()

	dest := filepath.Join(t.TempDir(), "file.bin")
	err := newTestGH(srv).Download(ctx, srv.URL+"/x", dest, nil)
	if err == nil {
		t.Fatal("expected cancellation error")
	}
	if _, statErr := os.Stat(dest + ".part"); !os.IsNotExist(statErr) {
		t.Error("partial file not removed on cancellation")
	}
	if _, statErr := os.Stat(dest); !os.IsNotExist(statErr) {
		t.Error("dest should not exist after cancellation")
	}
}
