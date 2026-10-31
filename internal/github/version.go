// Package github resolves release versions and downloads release assets,
// handling GitHub API rate limits gracefully.
package github

import (
	"fmt"
	"regexp"
	"runtime"
	"strings"

	"golang.org/x/mod/semver"
)

var versionRe = regexp.MustCompile(`^v?[0-9][0-9A-Za-z.+-]*$`)

// ParseAppSpec splits an "app" or "app@version" argument. The returned
// version is normalized (no leading "v") and empty when not given.
func ParseAppSpec(spec string) (name, ver string, err error) {
	name, ver, found := strings.Cut(spec, "@")
	if name == "" {
		return "", "", fmt.Errorf("invalid app spec %q", spec)
	}
	if !found {
		return name, "", nil
	}
	norm, err := NormalizeVersion(ver)
	if err != nil {
		return "", "", fmt.Errorf("invalid version in %q: %w", spec, err)
	}
	return name, norm, nil
}

// NormalizeVersion accepts "1.2.0" or "v1.2.0" and returns "1.2.0".
func NormalizeVersion(v string) (string, error) {
	if !versionRe.MatchString(v) {
		return "", fmt.Errorf("version %q is not a valid version string", v)
	}
	return strings.TrimPrefix(v, "v"), nil
}

// TagFor returns the git tag for a normalized version, e.g. "v1.2.0".
func TagFor(version string) string { return "v" + version }

// CompareVersions compares two normalized versions, returning -1, 0, or 1.
// Non-semver versions fall back to string comparison.
func CompareVersions(a, b string) int {
	va, vb := "v"+a, "v"+b
	if semver.IsValid(va) && semver.IsValid(vb) {
		return semver.Compare(va, vb)
	}
	return strings.Compare(a, b)
}

// AssetName builds the archive filename for the release asset convention,
// e.g. "wgo_1.2.0_linux_amd64.tar.gz". version must be normalized.
func AssetName(app, version, goos, goarch string) string {
	return fmt.Sprintf("%s_%s_%s_%s.tar.gz", app, version, goos, goarch)
}

// SupportedPlatform verifies the current OS/arch is supported and returns
// them, or a clear error.
func SupportedPlatform() (goos, goarch string, err error) {
	goos, goarch = runtime.GOOS, runtime.GOARCH
	if goos != "linux" {
		return "", "", fmt.Errorf("cmam only supports Linux; detected OS %q", goos)
	}
	if goarch != "amd64" && goarch != "arm64" {
		return "", "", fmt.Errorf("unsupported architecture %q: cmam supports amd64 and arm64", goarch)
	}
	return goos, goarch, nil
}

// DownloadURL constructs the direct download URL for a release asset.
func DownloadURL(repo, tag, asset string) string {
	return fmt.Sprintf("https://github.com/%s/releases/download/%s/%s", repo, tag, asset)
}
