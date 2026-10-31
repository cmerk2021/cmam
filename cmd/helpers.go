package cmd

import (
	"path/filepath"

	"github.com/cmerk2021/cmam/internal/github"
)

// parseSpec splits "app[@version]" into a name and normalized version.
func parseSpec(spec string) (name, ver string, err error) {
	return github.ParseAppSpec(spec)
}

// splitPathList splits a PATH-style list and cleans each entry.
func splitPathList(pathList string) []string {
	parts := filepath.SplitList(pathList)
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		if p != "" {
			out = append(out, filepath.Clean(p))
		}
	}
	return out
}
