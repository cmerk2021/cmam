package github

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"time"
)

// RateLimitError indicates the GitHub API rate limit was exceeded.
type RateLimitError struct {
	// Reset is when the rate limit window resets (zero if unknown).
	Reset time.Time
	// Authenticated reports whether a token was used for the request.
	Authenticated bool
}

// Error implements the error interface.
func (e *RateLimitError) Error() string {
	msg := "GitHub API rate limit exceeded"
	if !e.Reset.IsZero() {
		msg += fmt.Sprintf(" (resets %s)", e.Reset.Local().Format("15:04:05"))
	}
	if !e.Authenticated {
		msg += "; set GITHUB_TOKEN to raise the limit"
	}
	return msg
}

// rateLimitFromResponse builds a RateLimitError when the response is a
// 403/429 caused by rate limiting, or nil otherwise.
func rateLimitFromResponse(resp *http.Response, authenticated bool) *RateLimitError {
	if resp.StatusCode != http.StatusForbidden && resp.StatusCode != http.StatusTooManyRequests {
		return nil
	}
	if resp.Header.Get("X-RateLimit-Remaining") != "0" && resp.StatusCode != http.StatusTooManyRequests {
		return nil
	}
	e := &RateLimitError{Authenticated: authenticated}
	if s := resp.Header.Get("X-RateLimit-Reset"); s != "" {
		if sec, err := strconv.ParseInt(s, 10, 64); err == nil {
			e.Reset = time.Unix(sec, 0)
		}
	}
	return e
}

// apiError decodes a GitHub API error response into a concise error without
// dumping raw JSON.
func apiError(resp *http.Response, authenticated bool) error {
	if rl := rateLimitFromResponse(resp, authenticated); rl != nil {
		return rl
	}
	var body struct {
		Message string `json:"message"`
	}
	_ = json.NewDecoder(resp.Body).Decode(&body)
	if body.Message != "" {
		return fmt.Errorf("GitHub API error (HTTP %d): %s", resp.StatusCode, body.Message)
	}
	return fmt.Errorf("GitHub API error: HTTP %d", resp.StatusCode)
}
