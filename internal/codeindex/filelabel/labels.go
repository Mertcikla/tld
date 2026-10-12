// Package filelabel derives language-independent display labels from repository
// paths. Labels never participate in source or element identity.
package filelabel

import (
	"path"
	"strings"
)

// Names returns the shortest unique path suffix for each distinct repository
// path. Supply the complete indexed file set, never a selected view or radius.
func Names(paths []string) map[string]string {
	counts := map[string]int{}
	parts := map[string][]string{}
	for _, p := range paths {
		if p == "" {
			continue
		}
		p = path.Clean(p)
		if _, exists := parts[p]; exists {
			continue
		}
		components := strings.Split(p, "/")
		parts[p] = components
		for i := len(components) - 1; i >= 0; i-- {
			counts[strings.Join(components[i:], "/")]++
		}
	}
	names := make(map[string]string, len(parts))
	for p, components := range parts {
		names[p] = p
		for i := len(components) - 1; i >= 0; i-- {
			suffix := strings.Join(components[i:], "/")
			if counts[suffix] == 1 {
				names[p] = suffix
				break
			}
		}
	}
	return names
}
