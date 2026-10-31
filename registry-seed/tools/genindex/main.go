// Command genindex validates every manifest in apps/ and writes index.json.
// It uses only the standard library so the registry repo needs no go.sum.
//
// Manifests are simple TOML files restricted to `key = "string"` pairs and
// comments; unknown keys are ignored so the format can grow.
package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
)

type manifest struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	Repo        string `json:"repo"`
}

var (
	nameRe = regexp.MustCompile(`^[a-z0-9-]+$`)
	repoRe = regexp.MustCompile(`^[A-Za-z0-9_.-]+/[A-Za-z0-9_.-]+$`)
	lineRe = regexp.MustCompile(`^([A-Za-z0-9_-]+)\s*=\s*(".*")\s*$`)
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
}

func run() error {
	entries, err := os.ReadDir("apps")
	if err != nil {
		return fmt.Errorf("reading apps/: %w", err)
	}

	var manifests []manifest
	var failures []string
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".toml") {
			continue
		}
		expected := strings.TrimSuffix(e.Name(), ".toml")
		m, err := parseFile(filepath.Join("apps", e.Name()), expected)
		if err != nil {
			failures = append(failures, fmt.Sprintf("apps/%s: %v", e.Name(), err))
			continue
		}
		if m.Name == "example" {
			continue // the example manifest is excluded from the index
		}
		manifests = append(manifests, m)
	}
	if len(failures) > 0 {
		return fmt.Errorf("invalid manifests:\n  %s", strings.Join(failures, "\n  "))
	}

	sort.Slice(manifests, func(i, j int) bool { return manifests[i].Name < manifests[j].Name })
	if manifests == nil {
		manifests = []manifest{}
	}
	data, err := json.MarshalIndent(manifests, "", "  ")
	if err != nil {
		return err
	}
	if err := os.WriteFile("index.json", append(data, '\n'), 0o644); err != nil {
		return err
	}
	fmt.Printf("index.json written with %d app(s)\n", len(manifests))
	return nil
}

func parseFile(path, expectedName string) (manifest, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return manifest{}, err
	}
	var m manifest
	for i, raw := range strings.Split(string(data), "\n") {
		line := strings.TrimSpace(raw)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		match := lineRe.FindStringSubmatch(line)
		if match == nil {
			return manifest{}, fmt.Errorf("line %d: expected `key = \"value\"`, got %q", i+1, line)
		}
		value, err := strconv.Unquote(match[2])
		if err != nil {
			return manifest{}, fmt.Errorf("line %d: invalid string: %v", i+1, err)
		}
		switch match[1] {
		case "name":
			m.Name = value
		case "description":
			m.Description = value
		case "repo":
			m.Repo = value
		}
	}

	if !nameRe.MatchString(m.Name) {
		return manifest{}, fmt.Errorf("name %q must match [a-z0-9-]+", m.Name)
	}
	if m.Name != expectedName {
		return manifest{}, fmt.Errorf("name %q does not match filename %q", m.Name, expectedName)
	}
	if m.Description == "" {
		return manifest{}, fmt.Errorf("description must not be empty")
	}
	if !repoRe.MatchString(m.Repo) {
		return manifest{}, fmt.Errorf("repo must be owner/name, got %q", m.Repo)
	}
	return m, nil
}
