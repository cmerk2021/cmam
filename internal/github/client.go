package github

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"strings"
	"time"

	"github.com/cmerk2021/cmam/internal/version"
)

// Client resolves releases and downloads assets.
type Client struct {
	// BaseURL is the GitHub web base URL (for release pages and downloads).
	BaseURL string
	// APIBaseURL is the GitHub REST API base URL.
	APIBaseURL string
	// HTTP is used for all requests.
	HTTP *http.Client
	// Token, when set, is sent as a bearer token on API requests.
	Token string
}

// NewClient builds a Client against github.com, reading GITHUB_TOKEN.
func NewClient() *Client {
	return &Client{
		BaseURL:    "https://github.com",
		APIBaseURL: "https://api.github.com",
		HTTP:       &http.Client{Timeout: 60 * time.Second},
		Token:      os.Getenv("GITHUB_TOKEN"),
	}
}

// LatestTag resolves the latest (non-prerelease, non-draft) release tag for
// a repo. It prefers the releases/latest redirect to avoid API quota, then
// falls back to the REST API.
func (c *Client) LatestTag(ctx context.Context, repo string) (string, error) {
	if tag, err := c.latestTagViaRedirect(ctx, repo); err == nil {
		return tag, nil
	}
	return c.latestTagViaAPI(ctx, repo)
}

func (c *Client) latestTagViaRedirect(ctx context.Context, repo string) (string, error) {
	url := fmt.Sprintf("%s/%s/releases/latest", c.BaseURL, repo)
	req, err := http.NewRequestWithContext(ctx, http.MethodHead, url, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("User-Agent", version.UserAgent())

	client := *c.HTTP
	client.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 300 || resp.StatusCode >= 400 {
		return "", fmt.Errorf("expected redirect from %s, got HTTP %d", url, resp.StatusCode)
	}
	loc := resp.Header.Get("Location")
	const marker = "/releases/tag/"
	i := strings.Index(loc, marker)
	if i < 0 {
		return "", fmt.Errorf("no releases found for %s", repo)
	}
	tag := path.Base(loc[i+len(marker):])
	if tag == "" {
		return "", fmt.Errorf("no releases found for %s", repo)
	}
	return tag, nil
}

type apiRelease struct {
	TagName    string `json:"tag_name"`
	Draft      bool   `json:"draft"`
	Prerelease bool   `json:"prerelease"`
}

func (c *Client) latestTagViaAPI(ctx context.Context, repo string) (string, error) {
	url := fmt.Sprintf("%s/repos/%s/releases/latest", c.APIBaseURL, repo)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("User-Agent", version.UserAgent())
	req.Header.Set("Accept", "application/vnd.github+json")
	if c.Token != "" {
		req.Header.Set("Authorization", "Bearer "+c.Token)
	}
	resp, err := c.HTTP.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusNotFound {
		return "", fmt.Errorf("no releases found for %s", repo)
	}
	if resp.StatusCode != http.StatusOK {
		return "", apiError(resp, c.Token != "")
	}
	var rel apiRelease
	if err := json.NewDecoder(resp.Body).Decode(&rel); err != nil {
		return "", fmt.Errorf("parsing release response: %w", err)
	}
	if rel.Draft || rel.Prerelease || rel.TagName == "" {
		return "", fmt.Errorf("no stable release found for %s", repo)
	}
	return rel.TagName, nil
}

// LatestVersion resolves the latest release and returns its normalized
// version (without the leading "v").
func (c *Client) LatestVersion(ctx context.Context, repo string) (string, error) {
	tag, err := c.LatestTag(ctx, repo)
	if err != nil {
		return "", err
	}
	v, err := NormalizeVersion(tag)
	if err != nil {
		return "", fmt.Errorf("release tag %q of %s is not a version: %w", tag, repo, err)
	}
	return v, nil
}

// Progress reports download progress. total is -1 when unknown.
type Progress func(downloaded, total int64)

// Download fetches url into dest atomically (via a .part file), reporting
// progress. It removes partial files on error or cancellation.
func (c *Client) Download(ctx context.Context, url, dest string, progress Progress) (err error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return err
	}
	req.Header.Set("User-Agent", version.UserAgent())

	// No overall timeout for large downloads; rely on context cancellation.
	client := *c.HTTP
	client.Timeout = 0
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("downloading %s: %w", url, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusNotFound {
		return fmt.Errorf("asset not found: %s", url)
	}
	if resp.StatusCode != http.StatusOK {
		if rl := rateLimitFromResponse(resp, c.Token != ""); rl != nil {
			return rl
		}
		return fmt.Errorf("downloading %s: HTTP %d", url, resp.StatusCode)
	}

	if err := os.MkdirAll(filepath.Dir(dest), 0o755); err != nil {
		return err
	}
	part := dest + ".part"
	f, err := os.OpenFile(part, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0o644)
	if err != nil {
		return err
	}
	defer func() {
		f.Close()
		if err != nil {
			os.Remove(part)
		}
	}()

	total := resp.ContentLength
	var done int64
	buf := make([]byte, 128<<10)
	for {
		if err := ctx.Err(); err != nil {
			return err
		}
		n, readErr := resp.Body.Read(buf)
		if n > 0 {
			if _, writeErr := f.Write(buf[:n]); writeErr != nil {
				return writeErr
			}
			done += int64(n)
			if progress != nil {
				progress(done, total)
			}
		}
		if readErr == io.EOF {
			break
		}
		if readErr != nil {
			return fmt.Errorf("downloading %s: %w", url, readErr)
		}
	}
	if err := f.Close(); err != nil {
		return err
	}
	return os.Rename(part, dest)
}
