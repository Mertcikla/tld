package watch

import (
	"context"
	"database/sql"
	"testing"

	_ "modernc.org/sqlite"
)

func TestComputeViewDensityLevelsNoSignalsDefaultsZero(t *testing.T) {
	db := densityTestDB(t)
	store := NewStore(db)
	got, err := ComputeViewDensityLevels(context.Background(), store)
	if err != nil {
		t.Fatalf("ComputeViewDensityLevels: %v", err)
	}
	if got[1] != 0 || got[2] != 0 {
		t.Fatalf("levels = %+v, want zeros", got)
	}
}

func TestComputeViewDensityLevelsBucketsSignalScores(t *testing.T) {
	db := densityTestDB(t)
	_, err := db.Exec(`
		INSERT INTO watch_materialization(repository_id, owner_type, owner_key, resource_type, resource_id) VALUES
		  (1, 'symbol', 'low', 'element', 101),
		  (1, 'symbol', 'high', 'element', 201);
		INSERT INTO watch_filter_decisions(owner_type, owner_key, score, tier) VALUES
		  ('symbol', 'low', 0.1, 9),
		  ('symbol', 'high', 0.9, 1);
		INSERT INTO watch_architecture_links(target_resource_type, target_resource_id, confidence) VALUES
		  ('element', 201, 0.95);`)
	if err != nil {
		t.Fatalf("seed signals: %v", err)
	}

	got, err := ComputeViewDensityLevels(context.Background(), NewStore(db))
	if err != nil {
		t.Fatalf("ComputeViewDensityLevels: %v", err)
	}
	if got[1] >= got[2] {
		t.Fatalf("levels = %+v, want signaled high view above low view", got)
	}
}

func densityTestDB(t *testing.T) *sql.DB {
	t.Helper()
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	_, err = db.Exec(`
		CREATE TABLE views(id INTEGER PRIMARY KEY);
		CREATE TABLE placements(view_id INTEGER NOT NULL, element_id INTEGER NOT NULL);
		CREATE TABLE watch_materialization(repository_id INTEGER, owner_type TEXT, owner_key TEXT, resource_type TEXT, resource_id INTEGER);
		CREATE TABLE watch_filter_decisions(owner_type TEXT, owner_key TEXT, score REAL, tier INTEGER);
		CREATE TABLE watch_architecture_links(target_resource_type TEXT, target_resource_id INTEGER, confidence REAL);
		INSERT INTO views(id) VALUES (1), (2);
		INSERT INTO placements(view_id, element_id) VALUES (1, 101), (2, 201);`)
	if err != nil {
		t.Fatalf("create schema: %v", err)
	}
	return db
}
