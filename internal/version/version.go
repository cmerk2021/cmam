// Package version exposes build metadata injected at link time via -X flags.
package version

// These variables are set by the linker (see .goreleaser.yaml and Makefile).
var (
	// Version is the semantic version of this build, e.g. "0.1.0".
	Version = "dev"
	// Commit is the git commit the binary was built from.
	Commit = "none"
	// Date is the build timestamp.
	Date = "unknown"
)

// UserAgent returns the User-Agent header value for all HTTP requests.
func UserAgent() string { return "cmam/" + Version }
