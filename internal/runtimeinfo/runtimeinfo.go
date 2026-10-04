// Package runtimeinfo renders the shared status fields printed by the
// `serve` and `status` commands so both stay in the same order and format.
package runtimeinfo

import (
	"fmt"
	"io"
	"strings"

	"github.com/mertcikla/tld/v2/internal/term"
)

// NormalizeDBDriver maps a configured database driver to its canonical name.
func NormalizeDBDriver(driver string) string {
	switch strings.ToLower(strings.TrimSpace(driver)) {
	case "postgres", "postgresql":
		return "postgres"
	default:
		return "sqlite"
	}
}

// HumanBytes formats a byte count using binary units.
func HumanBytes(size int64) string {
	const unit = 1024
	if size < unit {
		return fmt.Sprintf("%d B", size)
	}
	div, exp := int64(unit), 0
	for n := size / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %cB", float64(size)/float64(div), "KMGTPE"[exp])
}

// ResourceCounts describes the number of resources served by an instance.
type ResourceCounts struct {
	Views      int
	Elements   int
	Connectors int
}

// Format returns the canonical "N views, N elements, N connectors" string.
func (r ResourceCounts) Format() string {
	return fmt.Sprintf("%d views, %d elements, %d connectors", r.Views, r.Elements, r.Connectors)
}

// Storage describes the on-disk state shared by `serve` and `status`.
type Storage struct {
	DataDir      string
	DBDriver     string
	DBPath       string
	DBSize       int64
	DBModifiedAt string
}

// PrintStorage prints the storage fields in canonical order. DataDir is
// omitted when empty so callers that do not track it can reuse this.
func PrintStorage(out io.Writer, s Storage) {
	if s.DataDir != "" {
		term.Label(out, term.DefaultLabelWidth, "Data dir", term.Path(out, s.DataDir))
	}
	if NormalizeDBDriver(s.DBDriver) == "postgres" {
		term.Label(out, term.DefaultLabelWidth, "DB", "postgres")
		return
	}
	if s.DBPath != "" {
		term.Label(out, term.DefaultLabelWidth, "DB", term.Path(out, s.DBPath))
	}
	if s.DBSize > 0 {
		term.Label(out, term.DefaultLabelWidth, "DB size", HumanBytes(s.DBSize))
	}
	if s.DBModifiedAt != "" {
		term.Label(out, term.DefaultLabelWidth, "DB modified", s.DBModifiedAt)
	}
}
