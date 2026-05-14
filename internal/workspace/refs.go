package workspace

import (
	"fmt"
	"regexp"
	"strings"
)

const RootRef = "root"

var refPattern = regexp.MustCompile(`^[a-z0-9][a-z0-9._-]*$`)

// ValidateRef checks a workspace resource ref. Set allowRoot for parent/view
// refs where the synthetic root view is valid.
func ValidateRef(ref string, allowRoot bool) error {
	ref = strings.TrimSpace(ref)
	if ref == "" {
		return fmt.Errorf("ref is required")
	}
	if ref == RootRef {
		if allowRoot {
			return nil
		}
		return fmt.Errorf("%q is reserved for the synthetic root view", RootRef)
	}
	if !refPattern.MatchString(ref) {
		return fmt.Errorf("invalid ref %q: use lowercase letters, numbers, hyphens, underscores, or dots", ref)
	}
	return nil
}

func ValidateElementRef(ref string) error {
	return ValidateRef(ref, false)
}

func ValidateParentRef(ref string) error {
	return ValidateRef(ref, true)
}
