package community

import "strings"

// genericFolders are repository-layout roots that do not discriminate one
// component from another. A group whose only common folder is generic prefers a
// distinctive lexical name instead of a duplicate "internal" or "src" label.
var genericFolders = map[string]struct{}{
	"internal": {}, "src": {}, "pkg": {}, "cmd": {}, "app": {}, "apps": {}, "lib": {},
	"source": {}, "core": {}, "test": {}, "tests": {}, "frontend": {}, "backend": {},
	"api": {}, "server": {}, "client": {}, "config": {}, "docs": {}, "scripts": {},
	"examples": {}, "services": {}, "service": {}, "packages": {}, "modules": {},
}

// layoutTerminals are path segments that mark a container rather than a
// component. A prefix ending in one of them, such as frontend/src or
// packages/app/src, describes the repository layout rather than the group.
var layoutTerminals = map[string]struct{}{
	"src": {}, "source": {}, "internal": {}, "lib": {}, "app": {}, "apps": {},
	"packages": {}, "modules": {}, "test": {}, "tests": {}, "cmd": {}, "config": {},
	"docs": {}, "scripts": {}, "examples": {}, "frontend": {}, "backend": {},
}

// genericPrefix reports whether a folder path describes repository layout
// rather than a discriminable component.
func genericPrefix(folder string) bool {
	if folder == "" {
		return false
	}
	segments := strings.Split(folder, "/")
	if len(segments) == 1 {
		_, ok := genericFolders[segments[0]]
		return ok
	}
	_, ok := layoutTerminals[segments[len(segments)-1]]
	return ok
}

// folderOf returns the slash-normalized directory of a file path, or "." for a
// file at the repository root.
func folderOf(path string) string {
	path = strings.ReplaceAll(path, "\\", "/")
	if idx := strings.LastIndex(path, "/"); idx >= 0 {
		path = path[:idx]
	} else {
		return "."
	}
	parts := make([]string, 0, 4)
	for _, part := range strings.Split(path, "/") {
		switch part {
		case "", ".":
		case "..":
			if len(parts) > 0 && parts[len(parts)-1] != ".." {
				parts = parts[:len(parts)-1]
			} else {
				parts = append(parts, "..")
			}
		default:
			parts = append(parts, part)
		}
	}
	if len(parts) == 0 {
		return "."
	}
	return strings.Join(parts, "/")
}

// folderSegments splits a normalized folder into path segments.
func folderSegments(folder string) []string {
	if folder == "." || folder == "" {
		return nil
	}
	return strings.Split(folder, "/")
}

// commonFolderPrefix returns the longest directory path shared by every member
// folder, or "" when the members do not share one.
func commonFolderPrefix(members []int, files []File) string {
	if len(members) == 0 {
		return ""
	}
	var prefix []string
	for i, member := range members {
		segments := folderSegments(folderOf(files[member].Path))
		if i == 0 {
			prefix = segments
			continue
		}
		limit := len(prefix)
		if len(segments) < limit {
			limit = len(segments)
		}
		shared := 0
		for shared < limit && prefix[shared] == segments[shared] {
			shared++
		}
		prefix = prefix[:shared]
		if len(prefix) == 0 {
			return ""
		}
	}
	if len(prefix) == 0 {
		return ""
	}
	return strings.Join(prefix, "/")
}
