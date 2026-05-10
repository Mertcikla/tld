package watch

import (
	"context"
	"database/sql"
	"sort"
)

func ComputeViewDensityLevels(ctx context.Context, store *Store) (map[int64]int, error) {
	if store == nil || store.db == nil {
		return map[int64]int{}, nil
	}

	viewIDs, err := densityViewIDs(ctx, store.db)
	if err != nil {
		return nil, err
	}
	if len(viewIDs) == 0 {
		return map[int64]int{}, nil
	}

	scores, err := densityViewScores(ctx, store.db)
	if err != nil {
		return nil, err
	}
	if len(scores) == 0 {
		out := make(map[int64]int, len(viewIDs))
		for _, viewID := range viewIDs {
			out[viewID] = 0
		}
		return out, nil
	}

	return bucketDensityScores(viewIDs, scores), nil
}

func densityViewIDs(ctx context.Context, db *sql.DB) ([]int64, error) {
	rows, err := db.QueryContext(ctx, `SELECT id FROM views ORDER BY id`)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	var ids []int64
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	return ids, rows.Err()
}

func densityViewScores(ctx context.Context, db *sql.DB) (map[int64]float64, error) {
	rows, err := db.QueryContext(ctx, `
		SELECT p.view_id, p.element_id, MAX(wfd.score), MIN(wfd.tier), MAX(wal.confidence)
		FROM placements p
		LEFT JOIN watch_materialization wm
		  ON wm.resource_type = 'element'
		 AND wm.resource_id = p.element_id
		LEFT JOIN watch_filter_decisions wfd
		  ON wfd.owner_type = wm.owner_type
		 AND wfd.owner_key = wm.owner_key
		LEFT JOIN watch_architecture_links wal
		  ON wal.target_resource_type = 'element'
		 AND wal.target_resource_id = p.element_id
		GROUP BY p.view_id, p.element_id
		ORDER BY p.view_id, p.element_id`)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	type aggregate struct {
		total float64
		count int
	}
	views := map[int64]aggregate{}
	for rows.Next() {
		var viewID, elementID int64
		var score sql.NullFloat64
		var tier sql.NullInt64
		var confidence sql.NullFloat64
		if err := rows.Scan(&viewID, &elementID, &score, &tier, &confidence); err != nil {
			return nil, err
		}
		_ = elementID
		elementTotal := 0.0
		elementCount := 0
		if score.Valid {
			elementTotal += score.Float64
			elementCount++
		}
		if tier.Valid {
			elementTotal += float64(max(0, 10-int(tier.Int64))) / 10.0
			elementCount++
		}
		if confidence.Valid {
			elementTotal += confidence.Float64
			elementCount++
		}
		if elementCount == 0 {
			continue
		}
		item := views[viewID]
		item.total += elementTotal / float64(elementCount)
		item.count++
		views[viewID] = item
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	out := make(map[int64]float64, len(views))
	for viewID, item := range views {
		if item.count > 0 {
			out[viewID] = item.total / float64(item.count)
		}
	}
	return out, nil
}

func bucketDensityScores(viewIDs []int64, scores map[int64]float64) map[int64]int {
	type scoredView struct {
		id    int64
		score float64
	}
	scored := make([]scoredView, 0, len(scores))
	for viewID, score := range scores {
		scored = append(scored, scoredView{id: viewID, score: score})
	}
	sort.SliceStable(scored, func(i, j int) bool {
		if scored[i].score == scored[j].score {
			return scored[i].id < scored[j].id
		}
		return scored[i].score < scored[j].score
	})

	out := make(map[int64]int, len(viewIDs))
	for _, viewID := range viewIDs {
		out[viewID] = 0
	}
	if len(scored) == 1 {
		out[scored[0].id] = 0
		return out
	}

	levels := []int{-2, -1, 0, 1, 2}
	for index, item := range scored {
		bucket := index * len(levels) / len(scored)
		if bucket >= len(levels) {
			bucket = len(levels) - 1
		}
		out[item.id] = levels[bucket]
	}
	return out
}
