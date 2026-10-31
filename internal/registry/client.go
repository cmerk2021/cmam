package registry

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/cmerk2021/cmam/internal/version"
)

// TTL is how long a cached index is considered fresh.
const TTL = time.Hour

// Client fetches the registry index with local caching and ETag
// revalidation. The zero value is not usable; use NewClient.
type Client struct {
	// IndexURL is the raw URL of index.json.
	IndexURL string
	// ContentsAPIURL lists apps/ via the GitHub contents API (fallback).
	ContentsAPIURL string
	// RawManifestURL formats a raw manifest URL for an app name.
	RawManifestURL func(name string) string
	// CacheDir stores the cached index and metadata.
	CacheDir string
	// HTTP is the HTTP client used for all requests.
	HTTP *http.Client
	// Token, when set, is sent as a bearer token on API requests.
	Token string
	// Now allows tests to control time.
	Now func() time.Time
}

// NewClient builds a Client for the configured registry repository.
func NewClient(indexURL, contentsAPIURL string, rawManifestURL func(string) string, cacheDir string) *Client {
	return &Client{
		IndexURL:       indexURL,
		ContentsAPIURL: contentsAPIURL,
		RawManifestURL: rawManifestURL,
		CacheDir:       cacheDir,
		HTTP:           &http.Client{Timeout: 30 * time.Second},
		Token:          os.Getenv("GITHUB_TOKEN"),
		Now:            time.Now,
	}
}

// Result is the outcome of loading the registry.
type Result struct {
	Apps []Manifest
	// Stale is true when the data came from an expired cache because the
	// network was unavailable.
	Stale bool
	// Age is how old the cached data is (zero for fresh fetches).
	Age time.Duration
}

type cacheMeta struct {
	ETag      string    `json:"etag"`
	FetchedAt time.Time `json:"fetched_at"`
}

func (c *Client) indexPath() string { return filepath.Join(c.CacheDir, "index.json") }
func (c *Client) metaPath() string  { return filepath.Join(c.CacheDir, "index.meta.json") }

// Load returns the registry apps, using the cache when fresh unless refresh
// is true, revalidating with ETag, and falling back to stale cache on
// network failure.
func (c *Client) Load(ctx context.Context, refresh bool) (Result, error) {
	cached, meta, cacheErr := c.readCache()
	if cacheErr == nil && !refresh && c.Now().Sub(meta.FetchedAt) < TTL {
		return Result{Apps: cached, Age: c.Now().Sub(meta.FetchedAt)}, nil
	}

	etag := ""
	if cacheErr == nil {
		etag = meta.ETag
	}
	apps, newETag, notModified, err := c.fetchIndex(ctx, etag)
	if err == nil {
		if notModified {
			meta.FetchedAt = c.Now()
			_ = c.writeMeta(meta)
			return Result{Apps: cached}, nil
		}
		_ = c.writeCache(apps, newETag)
		return Result{Apps: apps}, nil
	}

	// Fallback: list manifests through the contents API.
	if apps, apiErr := c.fetchViaContentsAPI(ctx); apiErr == nil {
		_ = c.writeCache(apps, "")
		return Result{Apps: apps}, nil
	}

	// Last resort: stale cache.
	if cacheErr == nil {
		return Result{Apps: cached, Stale: true, Age: c.Now().Sub(meta.FetchedAt)}, nil
	}
	return Result{}, fmt.Errorf("fetching registry index: %w", err)
}

// Get returns a single app by name, or an error listing that it is unknown.
func (c *Client) Get(ctx context.Context, name string, refresh bool) (Manifest, error) {
	res, err := c.Load(ctx, refresh)
	if err != nil {
		return Manifest{}, err
	}
	for _, m := range res.Apps {
		if m.Name == name {
			return m, nil
		}
	}
	return Manifest{}, fmt.Errorf("app %q not found in the registry", name)
}

// CachedNames returns app names from the local cache only (for shell
// completion); it never touches the network.
func (c *Client) CachedNames() []string {
	apps, _, err := c.readCache()
	if err != nil {
		return nil
	}
	names := make([]string, 0, len(apps))
	for _, m := range apps {
		names = append(names, m.Name)
	}
	sort.Strings(names)
	return names
}

func (c *Client) fetchIndex(ctx context.Context, etag string) (apps []Manifest, newETag string, notModified bool, err error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.IndexURL, nil)
	if err != nil {
		return nil, "", false, err
	}
	req.Header.Set("User-Agent", version.UserAgent())
	if etag != "" {
		req.Header.Set("If-None-Match", etag)
	}
	resp, err := c.HTTP.Do(req)
	if err != nil {
		return nil, "", false, err
	}
	defer resp.Body.Close()

	switch resp.StatusCode {
	case http.StatusNotModified:
		return nil, etag, true, nil
	case http.StatusOK:
		body, err := io.ReadAll(io.LimitReader(resp.Body, 8<<20))
		if err != nil {
			return nil, "", false, err
		}
		var raw []Manifest
		if err := json.Unmarshal(body, &raw); err != nil {
			return nil, "", false, fmt.Errorf("parsing index.json: %w", err)
		}
		apps = raw[:0]
		for _, m := range raw {
			if m.Validate() == nil {
				apps = append(apps, m)
			}
		}
		sortApps(apps)
		return apps, resp.Header.Get("ETag"), false, nil
	default:
		return nil, "", false, fmt.Errorf("fetching %s: HTTP %d", c.IndexURL, resp.StatusCode)
	}
}

type contentsEntry struct {
	Name string `json:"name"`
	Type string `json:"type"`
}

func (c *Client) fetchViaContentsAPI(ctx context.Context) ([]Manifest, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.ContentsAPIURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", version.UserAgent())
	req.Header.Set("Accept", "application/vnd.github+json")
	if c.Token != "" {
		req.Header.Set("Authorization", "Bearer "+c.Token)
	}
	resp, err := c.HTTP.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("listing registry contents: HTTP %d", resp.StatusCode)
	}
	var entries []contentsEntry
	if err := json.NewDecoder(resp.Body).Decode(&entries); err != nil {
		return nil, fmt.Errorf("parsing contents listing: %w", err)
	}

	var apps []Manifest
	var errs []error
	for _, e := range entries {
		if e.Type != "file" || !strings.HasSuffix(e.Name, ".toml") {
			continue
		}
		name := strings.TrimSuffix(e.Name, ".toml")
		m, err := c.fetchManifest(ctx, name)
		if err != nil {
			errs = append(errs, err)
			continue
		}
		apps = append(apps, m)
	}
	if len(apps) == 0 && len(errs) > 0 {
		return nil, errors.Join(errs...)
	}
	sortApps(apps)
	return apps, nil
}

func (c *Client) fetchManifest(ctx context.Context, name string) (Manifest, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.RawManifestURL(name), nil)
	if err != nil {
		return Manifest{}, err
	}
	req.Header.Set("User-Agent", version.UserAgent())
	resp, err := c.HTTP.Do(req)
	if err != nil {
		return Manifest{}, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return Manifest{}, fmt.Errorf("fetching manifest %s: HTTP %d", name, resp.StatusCode)
	}
	data, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return Manifest{}, err
	}
	return ParseManifest(data, name)
}

func (c *Client) readCache() ([]Manifest, cacheMeta, error) {
	data, err := os.ReadFile(c.indexPath())
	if err != nil {
		return nil, cacheMeta{}, err
	}
	var apps []Manifest
	if err := json.Unmarshal(data, &apps); err != nil {
		return nil, cacheMeta{}, fmt.Errorf("corrupt registry cache: %w", err)
	}
	var meta cacheMeta
	if metaData, err := os.ReadFile(c.metaPath()); err == nil {
		_ = json.Unmarshal(metaData, &meta)
	}
	return apps, meta, nil
}

func (c *Client) writeCache(apps []Manifest, etag string) error {
	if err := os.MkdirAll(c.CacheDir, 0o755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(apps, "", "  ")
	if err != nil {
		return err
	}
	if err := atomicWrite(c.indexPath(), data); err != nil {
		return err
	}
	return c.writeMeta(cacheMeta{ETag: etag, FetchedAt: c.Now()})
}

func (c *Client) writeMeta(meta cacheMeta) error {
	data, err := json.Marshal(meta)
	if err != nil {
		return err
	}
	return atomicWrite(c.metaPath(), data)
}

func atomicWrite(path string, data []byte) error {
	tmp, err := os.CreateTemp(filepath.Dir(path), ".tmp-*")
	if err != nil {
		return err
	}
	defer os.Remove(tmp.Name())
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmp.Name(), path)
}

func sortApps(apps []Manifest) {
	sort.Slice(apps, func(i, j int) bool { return apps[i].Name < apps[j].Name })
}
