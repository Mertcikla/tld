package dbrepo_test

import (
	"context"
	"database/sql"
	"fmt"
	"io/fs"
	"path/filepath"
	"strings"
	"testing"

	assets "github.com/mertcikla/tld/v2"
)

func TestCodeindexTenantKeysMigrationPreservesData(t *testing.T) {
	ctx := context.Background()
	db, err := sql.Open("sqlite", filepath.Join(t.TempDir(), "upgrade.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = db.Close() }()
	db.SetMaxOpenConns(1)
	const migration = "20261005000200_codeindex_tenant_keys.tx.up.sql"
	entries, err := fs.ReadDir(assets.FS, "migrations")
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".up.sql") || entry.Name() >= migration {
			continue
		}
		raw, err := assets.FS.ReadFile("migrations/" + entry.Name())
		if err != nil {
			t.Fatal(err)
		}
		if _, err := db.ExecContext(ctx, string(raw)); err != nil {
			t.Fatalf("apply %s: %v", entry.Name(), err)
		}
	}
	// Seed both legacy NULL-tenant and scoped data in every codeindex table,
	// including children with cascading foreign keys. Table creation order puts
	// repository and completed-map parents before their referencing children.
	rows, err := db.QueryContext(ctx, `SELECT name FROM sqlite_master WHERE type = 'table' AND name LIKE 'codeindex_%' ORDER BY rowid`)
	if err != nil {
		t.Fatal(err)
	}
	var tables []string
	for rows.Next() {
		var table string
		if err := rows.Scan(&table); err != nil {
			t.Fatal(err)
		}
		tables = append(tables, table)
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	_ = rows.Close()
	const orgA = "11111111-1111-1111-1111-111111111111"
	const orgB = "22222222-2222-2222-2222-222222222222"
	for _, table := range tables {
		rows, err := db.QueryContext(ctx, `PRAGMA table_info(`+table+`)`)
		if err != nil {
			t.Fatal(err)
		}
		var names, types []string
		for rows.Next() {
			var cid, nn, pk int
			var name, typ string
			var def sql.NullString
			if err := rows.Scan(&cid, &name, &typ, &nn, &def, &pk); err != nil {
				t.Fatal(err)
			}
			names = append(names, name)
			types = append(types, typ)
		}
		if err := rows.Err(); err != nil {
			t.Fatal(err)
		}
		_ = rows.Close()
		for _, label := range []string{"legacy", "scoped"} {
			var values []any
			var placeholders []string
			for i, name := range names {
				var value any = label
				switch {
				case name == "org_id":
					if label == "legacy" {
						value = nil
					} else {
						value = orgA
					}
				case types[i] == "BLOB":
					value = []byte(label)
				case types[i] == "INTEGER" || types[i] == "BIGINT" || types[i] == "REAL" || types[i] == "BOOLEAN":
					value = 1
				}
				values = append(values, value)
				placeholders = append(placeholders, "?")
			}
			if _, err := db.ExecContext(ctx, `INSERT INTO `+table+` (`+strings.Join(names, ",")+`) VALUES (`+strings.Join(placeholders, ",")+`)`, values...); err != nil {
				t.Fatalf("seed %s: %v", table, err)
			}
		}
	}
	raw, err := assets.FS.ReadFile("migrations/" + migration)
	if err != nil {
		t.Fatal(err)
	}
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = tx.Rollback() }()
	if _, err := tx.ExecContext(ctx, string(raw)); err != nil {
		t.Fatal(err)
	}
	if err := tx.Commit(); err != nil {
		t.Fatal(err)
	}
	for _, table := range tables {
		var total, legacy, scoped int
		if err := db.QueryRowContext(ctx, `SELECT COUNT(*), SUM(CASE WHEN org_id = '00000000-0000-0000-0000-000000000000' THEN 1 ELSE 0 END), SUM(CASE WHEN org_id = ? THEN 1 ELSE 0 END) FROM `+table, orgA).Scan(&total, &legacy, &scoped); err != nil {
			t.Fatal(err)
		}
		if total != 2 || legacy != 1 || scoped != 1 {
			t.Fatalf("%s: total=%d legacy=%d scoped=%d", table, total, legacy, scoped)
		}
		rows, err := db.QueryContext(ctx, `PRAGMA table_info(`+table+`)`)
		if err != nil {
			t.Fatal(err)
		}
		var columns, selects []string
		for rows.Next() {
			var cid, nn, pk int
			var name, typ string
			var def sql.NullString
			if err := rows.Scan(&cid, &name, &typ, &nn, &def, &pk); err != nil {
				t.Fatal(err)
			}
			columns = append(columns, name)
			if name == "org_id" {
				selects = append(selects, "?")
				if nn != 1 || pk != 1 {
					t.Fatalf("%s org_id must be non-null first PK column", table)
				}
			} else {
				selects = append(selects, name)
			}
		}
		if err := rows.Err(); err != nil {
			t.Fatal(err)
		}
		_ = rows.Close()
		// Clone the same complete key into another tenant. Foreign keys must also
		// resolve within that tenant, and a same-tenant duplicate must still fail.
		query := fmt.Sprintf("INSERT INTO %s (%s) SELECT %s FROM %s WHERE org_id = ?", table, strings.Join(columns, ","), strings.Join(selects, ","), table)
		if _, err := db.ExecContext(ctx, query, orgB, orgA); err != nil {
			t.Fatalf("clone %s across tenants: %v", table, err)
		}
		if _, err := db.ExecContext(ctx, query, orgB, orgA); err == nil {
			t.Fatalf("%s accepted duplicate within a tenant", table)
		}
	}
	rows, err = db.QueryContext(ctx, `PRAGMA foreign_key_check`)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = rows.Close() }()
	if rows.Next() {
		t.Fatal("foreign key violation after migration")
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
}
