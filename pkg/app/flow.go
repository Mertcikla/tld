package app

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/google/uuid"
	"github.com/uptrace/bun"
)

const (
	MaxFlowSteps       = 1000
	MaxFlowPaths       = 26
	MaxFlowBranchDepth = 8
)

// ErrFlowConflict means another writer changed or deleted the story.
var ErrFlowConflict = errors.New("flow changed; reload before saving")

// FlowRecord is a flow with its ordered step tree. Only the envelope and text
// fields live in flows; Steps is assembled from flow_steps and flow_paths.
type FlowRecord struct {
	bun.BaseModel `bun:"table:flows,alias:flow"`
	ID            string     `bun:"id,pk"`
	ViewID        int64      `bun:"view_id"`
	OrgID         *uuid.UUID `bun:"org_id"`
	Name          string     `bun:"name"`
	Description   string     `bun:"description"`
	Revision      int32      `bun:"revision"`
	Steps         []FlowStep `bun:"-"`
}

type FlowStep struct {
	ID           string
	Title        string
	Description  string
	ElementIDs   []int32
	ConnectorIDs []int32
	Branches     *FlowBranches
	GoToFlowID   string
}

type FlowBranches struct {
	Mode  string
	Paths []FlowPath
}

type FlowPath struct {
	ID    string
	Name  string
	Steps []FlowStep
}

type flowStepModel struct {
	bun.BaseModel `bun:"table:flow_steps,alias:flow_step"`
	FlowID        string  `bun:"flow_id,pk"`
	ID            string  `bun:"id,pk"`
	PathID        *string `bun:"path_id"`
	Position      int     `bun:"position"`
	Title         string  `bun:"title"`
	Description   string  `bun:"description"`
	ElementIDs    string  `bun:"element_ids"`
	ConnectorIDs  string  `bun:"connector_ids"`
	BranchMode    string  `bun:"branch_mode,nullzero"`
	GoToFlowID    string  `bun:"go_to_flow_id,nullzero"`
}

type flowPathModel struct {
	bun.BaseModel `bun:"table:flow_paths,alias:flow_path"`
	FlowID        string `bun:"flow_id,pk"`
	ID            string `bun:"id,pk"`
	StepID        string `bun:"step_id"`
	Position      int    `bun:"position"`
	Name          string `bun:"name"`
}

func (s *Store) ListFlows(ctx context.Context, viewID int64) ([]FlowRecord, error) {
	rows := []FlowRecord{}
	// A snapshot keeps envelope revisions and their step trees consistent even
	// when another client commits between these three bounded queries.
	err := s.bun.RunInTx(ctx, &sql.TxOptions{Isolation: sql.LevelRepeatableRead, ReadOnly: true}, func(ctx context.Context, tx bun.Tx) error {
		q := tx.NewSelect().Model(&rows).OrderExpr("view_id, id")
		if viewID != 0 {
			q.Where("view_id = ?", viewID)
		}
		if orgID := TenantOrgIDFromCtx(ctx); orgID != uuid.Nil {
			q.Where("org_id = ?", orgID)
		}
		if err := q.Scan(ctx); err != nil {
			return err
		}
		if len(rows) == 0 {
			return nil
		}
		ids := make([]string, len(rows))
		for i := range rows {
			ids[i] = rows[i].ID
		}
		var steps []flowStepModel
		if err := tx.NewSelect().Model(&steps).Where("flow_id IN (?)", bun.List(ids)).OrderExpr("position, id").Scan(ctx); err != nil {
			return err
		}
		var paths []flowPathModel
		if err := tx.NewSelect().Model(&paths).Where("flow_id IN (?)", bun.List(ids)).OrderExpr("position, id").Scan(ctx); err != nil {
			return err
		}
		stepsByFlow := map[string][]flowStepModel{}
		pathsByFlow := map[string][]flowPathModel{}
		for _, step := range steps {
			stepsByFlow[step.FlowID] = append(stepsByFlow[step.FlowID], step)
		}
		for _, path := range paths {
			pathsByFlow[path.FlowID] = append(pathsByFlow[path.FlowID], path)
		}
		for i := range rows {
			tree, err := assembleFlowSteps(stepsByFlow[rows[i].ID], pathsByFlow[rows[i].ID])
			if err != nil {
				return fmt.Errorf("read flow %s: %w", rows[i].ID, err)
			}
			rows[i].Steps = tree
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return rows, nil
}

// SaveFlow replaces a complete ordered tree in the same transaction as its
// compare-and-swap revision update. A flow's home view cannot be reassigned.
func (s *Store) SaveFlow(ctx context.Context, row FlowRecord) (FlowRecord, error) {
	if strings.TrimSpace(row.Name) == "" {
		return FlowRecord{}, errors.New("flow name is required")
	}
	if row.ID == "" && row.Revision != 0 || row.ID != "" && row.Revision <= 0 {
		return FlowRecord{}, errors.New("invalid flow revision")
	}
	steps, paths, err := flattenFlowSteps(row.Steps)
	if err != nil {
		return FlowRecord{}, err
	}
	err = s.bun.RunInTx(ctx, nil, func(ctx context.Context, tx bun.Tx) error {
		var view viewModel
		if err := tx.NewSelect().Model(&view).Where("id = ?", row.ViewID).Scan(ctx); err != nil {
			return err
		}
		row.OrgID = view.OrgID
		if row.ID == "" {
			row.ID = uuid.NewString()
			row.Revision = 1
			if _, err := tx.NewInsert().Model(&row).Exec(ctx); err != nil {
				return err
			}
		} else {
			q := tx.NewUpdate().Model((*FlowRecord)(nil)).Set("name = ?", row.Name).Set("description = ?", row.Description).Set("revision = revision + 1").Where("id = ? AND view_id = ? AND revision = ?", row.ID, row.ViewID, row.Revision)
			if orgID := TenantOrgIDFromCtx(ctx); orgID != uuid.Nil {
				q.Where("org_id = ?", orgID)
			}
			result, err := q.Exec(ctx)
			if err != nil {
				return err
			}
			count, err := result.RowsAffected()
			if err != nil {
				return err
			}
			if count != 1 {
				return ErrFlowConflict
			}
			row.Revision++
			// Paths and all descendants cascade with their owning steps.
			if _, err := tx.NewDelete().Model((*flowStepModel)(nil)).Where("flow_id = ?", row.ID).Exec(ctx); err != nil {
				return err
			}
		}
		for i := range steps {
			steps[i].FlowID = row.ID
		}
		for i := range paths {
			paths[i].FlowID = row.ID
		}
		// Deferred composite foreign keys allow bulk insertion of the whole tree.
		// Bound batches also cover flows with thousands of empty paths.
		for start := 0; start < len(steps); start += 500 {
			batch := steps[start:min(start+500, len(steps))]
			if _, err := tx.NewInsert().Model(&batch).Exec(ctx); err != nil {
				return err
			}
		}
		for start := 0; start < len(paths); start += 500 {
			batch := paths[start:min(start+500, len(paths))]
			if _, err := tx.NewInsert().Model(&batch).Exec(ctx); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		return FlowRecord{}, err
	}
	return row, nil
}

func (s *Store) DeleteFlow(ctx context.Context, id string, viewID int64, revision int32) error {
	if _, err := s.ViewByID(ctx, viewID); err != nil {
		return err
	}
	q := s.bun.NewDelete().Model((*FlowRecord)(nil)).Where("id = ? AND view_id = ? AND revision = ?", id, viewID, revision)
	if orgID := TenantOrgIDFromCtx(ctx); orgID != uuid.Nil {
		q.Where("org_id = ?", orgID)
	}
	result, err := q.Exec(ctx)
	if err != nil {
		return err
	}
	count, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if count != 1 {
		return ErrFlowConflict
	}
	return nil
}

func flattenFlowSteps(tree []FlowStep) ([]flowStepModel, []flowPathModel, error) {
	var steps []flowStepModel
	var paths []flowPathModel
	seen := map[string]bool{}
	claim := func(id string) error {
		if strings.TrimSpace(id) == "" || seen[id] {
			return errors.New("step and path ids must be unique and nonempty")
		}
		seen[id] = true
		return nil
	}
	var visit func([]FlowStep, *string, int) error
	visit = func(items []FlowStep, pathID *string, depth int) error {
		if depth > MaxFlowBranchDepth {
			return errors.New("flow branches are too deeply nested")
		}
		for position, step := range items {
			if len(steps) >= MaxFlowSteps {
				return errors.New("too many flow steps")
			}
			if err := claim(step.ID); err != nil {
				return err
			}
			elements, _ := json.Marshal(step.ElementIDs)
			connectors, _ := json.Marshal(step.ConnectorIDs)
			model := flowStepModel{ID: step.ID, PathID: pathID, Position: position, Title: step.Title, Description: step.Description, ElementIDs: string(elements), ConnectorIDs: string(connectors), GoToFlowID: step.GoToFlowID}
			if step.Branches != nil {
				if step.GoToFlowID != "" {
					return errors.New("choose branches or a linked flow")
				}
				model.BranchMode = step.Branches.Mode
				if model.BranchMode != "alternate" && model.BranchMode != "parallel" {
					return errors.New("unknown flow branch mode")
				}
				if len(step.Branches.Paths) < 2 || len(step.Branches.Paths) > MaxFlowPaths {
					return errors.New("invalid number of flow paths")
				}
			}
			steps = append(steps, model)
			if step.Branches != nil {
				for position, path := range step.Branches.Paths {
					if err := claim(path.ID); err != nil {
						return err
					}
					if strings.TrimSpace(path.Name) == "" {
						return errors.New("flow path name is required")
					}
					paths = append(paths, flowPathModel{ID: path.ID, StepID: step.ID, Position: position, Name: path.Name})
					if err := visit(path.Steps, &path.ID, depth+1); err != nil {
						return err
					}
				}
			}
		}
		return nil
	}
	if err := visit(tree, nil, 0); err != nil {
		return nil, nil, err
	}
	return steps, paths, nil
}

func assembleFlowSteps(steps []flowStepModel, paths []flowPathModel) ([]FlowStep, error) {
	byPath := map[string][]flowStepModel{}
	byStep := map[string][]flowPathModel{}
	for _, step := range steps {
		key := ""
		if step.PathID != nil {
			key = *step.PathID
		}
		byPath[key] = append(byPath[key], step)
	}
	for _, path := range paths {
		byStep[path.StepID] = append(byStep[path.StepID], path)
	}
	seenSteps, seenPaths := 0, 0
	var visit func(string, int) ([]FlowStep, error)
	visit = func(pathID string, depth int) ([]FlowStep, error) {
		if depth > MaxFlowBranchDepth {
			return nil, errors.New("invalid flow tree depth")
		}
		result := make([]FlowStep, 0, len(byPath[pathID]))
		for _, model := range byPath[pathID] {
			seenSteps++
			step := FlowStep{ID: model.ID, Title: model.Title, Description: model.Description, GoToFlowID: model.GoToFlowID}
			if err := json.Unmarshal([]byte(model.ElementIDs), &step.ElementIDs); err != nil {
				return nil, fmt.Errorf("step %s elements: %w", model.ID, err)
			}
			if err := json.Unmarshal([]byte(model.ConnectorIDs), &step.ConnectorIDs); err != nil {
				return nil, fmt.Errorf("step %s connectors: %w", model.ID, err)
			}
			if model.BranchMode != "" {
				step.Branches = &FlowBranches{Mode: model.BranchMode, Paths: make([]FlowPath, 0, len(byStep[model.ID]))}
				for _, path := range byStep[model.ID] {
					seenPaths++
					children, err := visit(path.ID, depth+1)
					if err != nil {
						return nil, err
					}
					step.Branches.Paths = append(step.Branches.Paths, FlowPath{ID: path.ID, Name: path.Name, Steps: children})
				}
			}
			result = append(result, step)
		}
		return result, nil
	}
	tree, err := visit("", 0)
	if err != nil {
		return nil, err
	}
	if seenSteps != len(steps) || seenPaths != len(paths) {
		return nil, errors.New("flow tree contains unreachable steps or paths")
	}
	return tree, nil
}
