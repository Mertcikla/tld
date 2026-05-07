package tagcolors

import (
	"context"
	crand "crypto/rand"
	"database/sql"
	"fmt"
	"hash/fnv"
	"strings"
)

var SwatchColors = []string{
	"#F56565", "#ED8936", "#ECC94B", "#48BB78", "#38B2AC",
	"#4299E1", "#667EEA", "#9F7AEA", "#ED64A6", "#A0AEC0",
}

func Ensure(ctx context.Context, db *sql.DB, tags []string) error {
	if len(tags) == 0 {
		return nil
	}

	rows, err := db.QueryContext(ctx, `SELECT name, color FROM tags ORDER BY name`)
	if err != nil {
		return err
	}
	defer func() { _ = rows.Close() }()

	existing := map[string]struct{}{}
	var usedColors []string
	for rows.Next() {
		var name, color string
		if err := rows.Scan(&name, &color); err != nil {
			return err
		}
		existing[name] = struct{}{}
		usedColors = append(usedColors, color)
	}
	if err := rows.Err(); err != nil {
		return err
	}

	for _, name := range tags {
		name = strings.TrimSpace(name)
		if name == "" {
			continue
		}
		if _, ok := existing[name]; ok {
			continue
		}
		color := PickUnusedColor(usedColors)
		if _, err := db.ExecContext(ctx, `INSERT OR IGNORE INTO tags(name, color, description) VALUES (?, ?, NULL)`, name, color); err != nil {
			return err
		}
		usedColors = append(usedColors, color)
		existing[name] = struct{}{}
	}
	return nil
}

func PickUnusedColor(usedColors []string) string {
	used := make(map[string]bool)
	for _, c := range usedColors {
		used[strings.ToUpper(c)] = true
	}

	var pool []string
	for _, c := range SwatchColors {
		if !used[strings.ToUpper(c)] {
			pool = append(pool, c)
		}
	}

	source := pool
	if len(source) == 0 {
		return randomUnusedColor(used)
	}

	return source[randomIndex(len(source))]
}

func randomIndex(n int) int {
	if n <= 1 {
		return 0
	}
	var b [1]byte
	if _, err := crand.Read(b[:]); err == nil {
		return int(b[0]) % n
	}
	return 0
}

func randomUnusedColor(used map[string]bool) string {
	var b [3]byte
	for range 32 {
		if _, err := crand.Read(b[:]); err == nil {
			color := fmt.Sprintf("#%02X%02X%02X", b[0], b[1], b[2])
			if !used[color] {
				return color
			}
		}
	}
	return fallbackUnusedColor(used)
}

func fallbackUnusedColor(used map[string]bool) string {
	for i := 0; ; i++ {
		h := fnv.New32a()
		_, _ = fmt.Fprintf(h, "tld-tag-color-%d", i)
		sum := h.Sum32()
		color := fmt.Sprintf("#%06X", sum&0xFFFFFF)
		if !used[color] {
			return color
		}
	}
}
