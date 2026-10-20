// Package browse lists the subdirectories of a directory, for navigating
// a filesystem hierarchy when picking a working directory for a new session.
package browse

import (
	"os"
	"sort"
	"strings"
)

// Subdirs returns the names of dir's direct subdirectories, sorted
// alphabetically. Dotdirs (e.g. .git) are excluded.
func Subdirs(dir string) ([]string, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}
	var names []string
	for _, e := range entries {
		if !e.IsDir() || strings.HasPrefix(e.Name(), ".") {
			continue
		}
		names = append(names, e.Name())
	}
	sort.Strings(names)
	return names, nil
}
