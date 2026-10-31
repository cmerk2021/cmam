package registry

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestParseManifest(t *testing.T) {
	cases := []struct {
		name     string
		toml     string
		expected string
		wantErr  bool
	}{
		{"valid", "name = \"wgo\"\ndescription = \"Watch files\"\nrepo = \"cmerk2021/wgo\"\n", "wgo", false},
		{"unknown fields ignored", "name = \"wgo\"\ndescription = \"d\"\nrepo = \"a/b\"\nfuture_field = 1\n", "wgo", false},
		{"bad name uppercase", "name = \"Wgo\"\ndescription = \"d\"\nrepo = \"a/b\"\n", "", true},
		{"empty description", "name = \"wgo\"\ndescription = \"\"\nrepo = \"a/b\"\n", "wgo", true},
		{"bad repo", "name = \"wgo\"\ndescription = \"d\"\nrepo = \"nope\"\n", "wgo", true},
		{"name mismatch", "name = \"other\"\ndescription = \"d\"\nrepo = \"a/b\"\n", "wgo", true},
		{"invalid toml", "name = [", "wgo", true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			m, err := ParseManifest([]byte(tc.toml), tc.expected)
			if tc.wantErr {
				if err == nil {
					t.Fatalf("expected error, got %+v", m)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
		})
	}
}

const indexJSON = `[{"name":"wgo","description":"Watch files","repo":"cmerk2021/wgo"}]`

func newTestClient(t *testing.T, srv *httptest.Server) *Client {
	t.Helper()
	c := NewClient(srv.URL+"/index.json", srv.URL+"/contents", func(name string) string {
		return srv.URL + "/apps/" + name + ".toml"
	}, t.TempDir())
	c.HTTP = srv.Client()
	return c
}

func TestLoadCachesWithTTL(t *testing.T) {
	hits := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits++
		w.Header().Set("ETag", `"abc"`)
		w.Write([]byte(indexJSON))
	}))
	defer srv.Close()

	c := newTestClient(t, srv)
	now := time.Now()
	c.Now = func() time.Time { return now }

	res, err := c.Load(context.Background(), false)
	if err != nil || len(res.Apps) != 1 || res.Apps[0].Name != "wgo" {
		t.Fatalf("first load: %+v, %v", res, err)
	}
	if hits != 1 {
		t.Fatalf("expected 1 hit, got %d", hits)
	}

	// Within TTL: served from cache, no network.
	if _, err := c.Load(context.Background(), false); err != nil {
		t.Fatal(err)
	}
	if hits != 1 {
		t.Fatalf("expected cache hit, got %d requests", hits)
	}

	// After TTL: revalidates.
	now = now.Add(2 * time.Hour)
	if _, err := c.Load(context.Background(), false); err != nil {
		t.Fatal(err)
	}
	if hits != 2 {
		t.Fatalf("expected revalidation, got %d requests", hits)
	}
}

func TestLoadETagRevalidation(t *testing.T) {
	var gotETag string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotETag = r.Header.Get("If-None-Match")
		if gotETag == `"abc"` {
			w.WriteHeader(http.StatusNotModified)
			return
		}
		w.Header().Set("ETag", `"abc"`)
		w.Write([]byte(indexJSON))
	}))
	defer srv.Close()

	c := newTestClient(t, srv)
	now := time.Now()
	c.Now = func() time.Time { return now }

	if _, err := c.Load(context.Background(), false); err != nil {
		t.Fatal(err)
	}
	now = now.Add(2 * time.Hour)
	res, err := c.Load(context.Background(), false)
	if err != nil {
		t.Fatal(err)
	}
	if gotETag != `"abc"` {
		t.Fatalf("If-None-Match not sent, got %q", gotETag)
	}
	if len(res.Apps) != 1 {
		t.Fatalf("expected cached apps after 304, got %+v", res.Apps)
	}
	// 304 refreshes the TTL: the next load within TTL stays local.
	now = now.Add(30 * time.Minute)
	if _, err := c.Load(context.Background(), false); err != nil {
		t.Fatal(err)
	}
}

func TestLoadStaleFallback(t *testing.T) {
	up := true
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !up {
			http.Error(w, "boom", http.StatusInternalServerError)
			return
		}
		w.Write([]byte(indexJSON))
	}))
	defer srv.Close()

	c := newTestClient(t, srv)
	now := time.Now()
	c.Now = func() time.Time { return now }

	if _, err := c.Load(context.Background(), false); err != nil {
		t.Fatal(err)
	}

	up = false
	now = now.Add(3 * time.Hour)
	res, err := c.Load(context.Background(), false)
	if err != nil {
		t.Fatalf("expected stale fallback, got error: %v", err)
	}
	if !res.Stale {
		t.Fatal("expected Stale=true")
	}
	if res.Age < 3*time.Hour {
		t.Fatalf("expected age >= 3h, got %s", res.Age)
	}
	if len(res.Apps) != 1 {
		t.Fatalf("expected cached apps, got %+v", res.Apps)
	}
}

func TestLoadRefreshBypassesTTL(t *testing.T) {
	hits := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits++
		w.Write([]byte(indexJSON))
	}))
	defer srv.Close()

	c := newTestClient(t, srv)
	if _, err := c.Load(context.Background(), false); err != nil {
		t.Fatal(err)
	}
	if _, err := c.Load(context.Background(), true); err != nil {
		t.Fatal(err)
	}
	if hits != 2 {
		t.Fatalf("expected refresh to hit the network, got %d requests", hits)
	}
}

func TestContentsAPIFallback(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/index.json", func(w http.ResponseWriter, r *http.Request) {
		http.NotFound(w, r)
	})
	mux.HandleFunc("/contents", func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`[{"name":"wgo.toml","type":"file"},{"name":"README.md","type":"file"}]`))
	})
	mux.HandleFunc("/apps/wgo.toml", func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("name = \"wgo\"\ndescription = \"Watch files\"\nrepo = \"cmerk2021/wgo\"\n"))
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	c := newTestClient(t, srv)
	res, err := c.Load(context.Background(), false)
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Apps) != 1 || res.Apps[0].Name != "wgo" {
		t.Fatalf("fallback failed: %+v", res.Apps)
	}
}

func TestGetUnknownApp(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(indexJSON))
	}))
	defer srv.Close()

	c := newTestClient(t, srv)
	if _, err := c.Get(context.Background(), "nope", false); err == nil {
		t.Fatal("expected error for unknown app")
	}
}
