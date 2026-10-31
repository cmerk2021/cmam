// Package registry fetches, caches, and validates app manifests from the
// CMAM registry repository.
package registry

import (
	"fmt"
	"regexp"

	"github.com/BurntSushi/toml"
)

// Manifest describes one app in the registry. Unknown TOML fields are
// ignored so the format can be extended without breaking older clients.
type Manifest struct {
	Name        string `toml:"name" json:"name"`
	Description string `toml:"description" json:"description"`
	Repo        string `toml:"repo" json:"repo"`
}

var (
	nameRe = regexp.MustCompile(`^[a-z0-9-]+$`)
	repoRe = regexp.MustCompile(`^[A-Za-z0-9_.-]+/[A-Za-z0-9_.-]+$`)
)

// ParseManifest parses TOML bytes into a Manifest and validates it.
// expectedName, when non-empty, must match the manifest's name field
// (it is derived from the manifest filename).
func ParseManifest(data []byte, expectedName string) (Manifest, error) {
	var m Manifest
	if _, err := toml.Decode(string(data), &m); err != nil {
		return Manifest{}, fmt.Errorf("parsing manifest: %w", err)
	}
	if err := m.Validate(); err != nil {
		return Manifest{}, err
	}
	if expectedName != "" && m.Name != expectedName {
		return Manifest{}, fmt.Errorf("manifest name %q does not match filename %q", m.Name, expectedName)
	}
	return m, nil
}

// Validate checks the manifest fields against the registry rules.
func (m Manifest) Validate() error {
	if !nameRe.MatchString(m.Name) {
		return fmt.Errorf("invalid app name %q: must match [a-z0-9-]+", m.Name)
	}
	if m.Description == "" {
		return fmt.Errorf("manifest %q: description must not be empty", m.Name)
	}
	if !repoRe.MatchString(m.Repo) {
		return fmt.Errorf("manifest %q: repo must be in owner/name form, got %q", m.Name, m.Repo)
	}
	return nil
}
