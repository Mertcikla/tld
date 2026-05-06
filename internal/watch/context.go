package watch

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"math"
	"path"
	"sort"
	"strings"
)

const (
	contextActionShow  = "show"
	contextActionHide  = "hide"
	contextActionClean = "clean"
)

type contextOwner struct {
	OwnerType string
	OwnerKey  string
}

type contextPolicySet struct {
	Show map[string]string
	Hide map[string]string
}

type contextExpansionSet struct {
	Tiers   map[string]int
	MaxTier int
}

type contextExpansionAdjustment struct {
	TierBefore int
	TierAfter  int
	MaxTier    int
	Owners     int
}

type contextRemovalStats struct {
	Elements   int
	Connectors int
	Views      int
}

func (s *Store) ApplyContextAction(ctx context.Context, repositoryID int64, action string, req ContextResourceRequest, representReq RepresentRequest) (ContextActionResult, error) {
	if action != contextActionShow && action != contextActionHide && action != contextActionClean {
		return ContextActionResult{}, fmt.Errorf("unsupported context action %q", action)
	}
	owners, err := s.contextOwnersForResource(ctx, repositoryID, action, req)
	if err != nil {
		return ContextActionResult{}, err
	}
	if len(owners) == 0 {
		return ContextActionResult{}, fmt.Errorf("resource is not backed by watch materialization")
	}
	if action == contextActionShow {
		if err := s.focusedRescanContextOwners(ctx, repositoryID, owners); err != nil {
			return ContextActionResult{}, err
		}
	}
	maxTier := maxVisibilityTier(defaultVisibilityConfig(representReq.Visibility))
	adjustment := contextExpansionAdjustment{MaxTier: maxTier}
	policiesCreated, policiesUpdated, deactivated := 0, 0, 0
	switch action {
	case contextActionShow:
		adjustment, err = s.AdjustContextExpansion(ctx, repositoryID, req, owners, 1, maxTier)
		if err != nil {
			return ContextActionResult{}, err
		}
	case contextActionClean:
		adjustment, err = s.AdjustContextExpansion(ctx, repositoryID, req, owners, -1, maxTier)
		if err != nil {
			return ContextActionResult{}, err
		}
	default:
		policiesCreated, policiesUpdated, deactivated, err = s.saveContextPolicies(ctx, repositoryID, action, contextScope(req.ResourceType), owners)
		if err != nil {
			return ContextActionResult{}, err
		}
	}
	before, err := s.generatedWorkspaceCounts(ctx, repositoryID)
	if err != nil {
		return ContextActionResult{}, err
	}
	representation, err := NewRepresenter(s).Represent(ctx, repositoryID, representReq)
	if err != nil {
		return ContextActionResult{}, err
	}
	after, err := s.generatedWorkspaceCounts(ctx, repositoryID)
	if err != nil {
		return ContextActionResult{}, err
	}
	summary, err := s.RepresentationSummary(ctx, repositoryID)
	if err != nil {
		return ContextActionResult{}, err
	}
	return ContextActionResult{
		RepositoryID:        repositoryID,
		Action:              action,
		PoliciesCreated:     policiesCreated,
		PoliciesUpdated:     policiesUpdated,
		PoliciesDeactivated: deactivated,
		OwnersAffected:      len(owners),
		TierBefore:          adjustment.TierBefore,
		TierAfter:           adjustment.TierAfter,
		MaxTier:             adjustment.MaxTier,
		ElementsAdded:       positiveDelta(after.Elements, before.Elements),
		ConnectorsAdded:     positiveDelta(after.Connectors, before.Connectors),
		ViewsAdded:          positiveDelta(after.Views, before.Views),
		ElementsRemoved:     positiveDelta(before.Elements, after.Elements),
		ConnectorsRemoved:   positiveDelta(before.Connectors, after.Connectors),
		ViewsRemoved:        positiveDelta(before.Views, after.Views),
		Representation:      representation,
		Summary:             summary,
	}, nil
}

func contextScope(resourceType string) string {
	switch resourceType {
	case "view":
		return "view"
	default:
		return "element"
	}
}

func positiveDelta(before, after int) int {
	if before > after {
		return before - after
	}
	return 0
}

func (s *Store) generatedWorkspaceCounts(ctx context.Context, repositoryID int64) (contextRemovalStats, error) {
	var out contextRemovalStats
	for resourceType, dest := range map[string]*int{
		"element":   &out.Elements,
		"connector": &out.Connectors,
		"view":      &out.Views,
	} {
		if err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM watch_materialization WHERE repository_id = ? AND resource_type = ?`, repositoryID, resourceType).Scan(dest); err != nil {
			return contextRemovalStats{}, err
		}
	}
	return out, nil
}

func maxVisibilityTier(cfg VisibilityConfig) int {
	cfg = defaultVisibilityConfig(cfg)
	if cfg.TierMultiplier <= 0 {
		return 0
	}
	maxMultiplier := cfg.MaxExpansionMultiplier
	if maxMultiplier < 1 {
		maxMultiplier = 1
	}
	tier := int((maxMultiplier - 1) / cfg.TierMultiplier)
	if tier < 0 {
		return 0
	}
	if tier == 0 && maxMultiplier > 1 {
		return 1
	}
	return tier
}

func effectiveMaxElementsPerView(thresholds Thresholds, cfg VisibilityConfig, tier int) int {
	thresholds = defaultThresholds(thresholds)
	cfg = defaultVisibilityConfig(cfg)
	if tier <= 0 {
		return thresholds.MaxElementsPerView
	}
	multiplier := 1 + float64(tier)*cfg.TierMultiplier
	if multiplier > cfg.MaxExpansionMultiplier {
		multiplier = cfg.MaxExpansionMultiplier
	}
	out := int(math.Round(float64(thresholds.MaxElementsPerView) * multiplier))
	if out < thresholds.MaxElementsPerView {
		return thresholds.MaxElementsPerView
	}
	return out
}

func (s *Store) AdjustContextExpansion(ctx context.Context, repositoryID int64, req ContextResourceRequest, owners []contextOwner, delta, maxTier int) (contextExpansionAdjustment, error) {
	owners = uniqueContextOwners(owners)
	if len(owners) == 0 {
		return contextExpansionAdjustment{MaxTier: maxTier}, nil
	}
	before, err := s.contextExpansionTier(ctx, repositoryID, req)
	if err != nil {
		return contextExpansionAdjustment{}, err
	}
	after := before + delta
	if after < 0 {
		after = 0
	}
	if maxTier >= 0 && after > maxTier {
		after = maxTier
	}
	now := nowString()
	for _, owner := range owners {
		if after == 0 {
			if _, err := s.db.ExecContext(ctx, `
				DELETE FROM watch_context_expansions
				WHERE repository_id = ? AND scope_resource_type = ? AND scope_resource_id = ? AND scope_owner_type = ? AND scope_owner_key = ?`,
				repositoryID, req.ResourceType, req.ResourceID, owner.OwnerType, owner.OwnerKey); err != nil {
				return contextExpansionAdjustment{}, err
			}
			continue
		}
		if _, err := s.db.ExecContext(ctx, `
			INSERT INTO watch_context_expansions(repository_id, scope_resource_type, scope_resource_id, scope_owner_type, scope_owner_key, tier, created_at, updated_at)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?)
			ON CONFLICT(repository_id, scope_resource_type, scope_resource_id, scope_owner_type, scope_owner_key)
			DO UPDATE SET tier = excluded.tier, updated_at = excluded.updated_at`,
			repositoryID, req.ResourceType, req.ResourceID, owner.OwnerType, owner.OwnerKey, after, now, now); err != nil {
			return contextExpansionAdjustment{}, err
		}
	}
	return contextExpansionAdjustment{TierBefore: before, TierAfter: after, MaxTier: maxTier, Owners: len(owners)}, nil
}

func (s *Store) contextExpansionTier(ctx context.Context, repositoryID int64, req ContextResourceRequest) (int, error) {
	var tier sql.NullInt64
	err := s.db.QueryRowContext(ctx, `
		SELECT MAX(tier)
		FROM watch_context_expansions
		WHERE repository_id = ? AND scope_resource_type = ? AND scope_resource_id = ?`,
		repositoryID, req.ResourceType, req.ResourceID).Scan(&tier)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, nil
	}
	if err != nil {
		return 0, err
	}
	if !tier.Valid {
		return 0, nil
	}
	return int(tier.Int64), nil
}

func (s *Store) ActiveContextExpansionSet(ctx context.Context, repositoryID int64) (contextExpansionSet, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT scope_owner_type, scope_owner_key, tier
		FROM watch_context_expansions
		WHERE repository_id = ? AND tier > 0
		ORDER BY id`, repositoryID)
	if err != nil {
		return contextExpansionSet{}, err
	}
	defer func() { _ = rows.Close() }()
	set := contextExpansionSet{Tiers: map[string]int{}}
	for rows.Next() {
		var ownerType, ownerKey string
		var tier int
		if err := rows.Scan(&ownerType, &ownerKey, &tier); err != nil {
			return contextExpansionSet{}, err
		}
		key := ownerMapKey(ownerType, ownerKey)
		if tier > set.Tiers[key] {
			set.Tiers[key] = tier
		}
		if tier > set.MaxTier {
			set.MaxTier = tier
		}
	}
	return set, rows.Err()
}

func (s *Store) contextOwnersForResource(ctx context.Context, repositoryID int64, action string, req ContextResourceRequest) ([]contextOwner, error) {
	if req.ResourceID <= 0 {
		return nil, fmt.Errorf("invalid resource id")
	}
	switch req.ResourceType {
	case "element":
		return s.contextOwnersForElement(ctx, repositoryID, action, req.ResourceID)
	case "view":
		return s.contextOwnersForView(ctx, repositoryID, action, req.ResourceID)
	default:
		return nil, fmt.Errorf("unsupported resource type %q", req.ResourceType)
	}
}

func (s *Store) contextOwnersForElement(ctx context.Context, repositoryID int64, action string, elementID int64) ([]contextOwner, error) {
	mapping, ok, err := s.materializationByResource(ctx, repositoryID, "element", elementID)
	if err != nil || !ok {
		return nil, err
	}
	base := contextOwner{OwnerType: mapping.OwnerType, OwnerKey: mapping.OwnerKey}
	if action == contextActionShow {
		return []contextOwner{base}, nil
	}
	if action == contextActionClean {
		return []contextOwner{base}, nil
	}
	return s.hideOwnersForElement(ctx, repositoryID, elementID, base)
}

func (s *Store) contextOwnersForView(ctx context.Context, repositoryID int64, action string, viewID int64) ([]contextOwner, error) {
	var owners []contextOwner
	if mapping, ok, err := s.materializationByResource(ctx, repositoryID, "view", viewID); err != nil {
		return nil, err
	} else if ok {
		owners = append(owners, contextOwner{OwnerType: mapping.OwnerType, OwnerKey: mapping.OwnerKey})
	}
	placementOwners, err := s.materializedElementOwnersInView(ctx, repositoryID, viewID)
	if err != nil {
		return nil, err
	}
	owners = append(owners, placementOwners...)
	connectorOwners, err := s.materializedConnectorOwnersInView(ctx, repositoryID, viewID)
	if err != nil {
		return nil, err
	}
	owners = append(owners, connectorOwners...)
	owners = uniqueContextOwners(owners)
	if action == contextActionShow {
		return owners, nil
	}
	if action == contextActionClean {
		return owners, nil
	}
	return s.rankHideOwners(ctx, repositoryID, owners)
}

func (s *Store) hideOwnersForElement(ctx context.Context, repositoryID, elementID int64, base contextOwner) ([]contextOwner, error) {
	owners := []contextOwner{base}
	connectorOwners, err := s.materializedConnectorOwnersTouchingElement(ctx, repositoryID, elementID)
	if err != nil {
		return nil, err
	}
	owners = append(owners, connectorOwners...)
	neighborOwners, err := s.materializedNeighborOwners(ctx, repositoryID, elementID)
	if err != nil {
		return nil, err
	}
	owners = append(owners, neighborOwners...)
	return s.rankHideOwners(ctx, repositoryID, owners)
}

func (s *Store) rankHideOwners(ctx context.Context, repositoryID int64, owners []contextOwner) ([]contextOwner, error) {
	symbols, err := s.SymbolsForRepository(ctx, repositoryID)
	if err != nil {
		return nil, err
	}
	identityKeys, err := s.SymbolIdentityKeys(ctx, repositoryID)
	if err != nil {
		return nil, err
	}
	owners = expandContainerOwnersToSymbols(owners, symbols, identityKeys)
	keep := map[string]struct{}{}
	for _, sym := range symbols {
		key := ownerMapKey("symbol", symbolOwnerKey(sym, identityKeys))
		if isExportedSymbol(sym) {
			keep[key] = struct{}{}
		}
	}
	var out []contextOwner
	for _, owner := range uniqueContextOwners(owners) {
		if owner.OwnerType == "file" || owner.OwnerType == "folder" {
			continue
		}
		if _, ok := keep[ownerKey(owner)]; ok {
			continue
		}
		out = append(out, owner)
	}
	if len(out) == 0 {
		return uniqueContextOwners(owners), nil
	}
	sort.SliceStable(out, func(i, j int) bool {
		return hideOwnerPriority(out[i]) < hideOwnerPriority(out[j])
	})
	return out, nil
}

func expandContainerOwnersToSymbols(owners []contextOwner, symbols []Symbol, identityKeys map[string]string) []contextOwner {
	out := append([]contextOwner{}, owners...)
	for _, owner := range owners {
		switch owner.OwnerType {
		case "file":
			file := strings.TrimPrefix(owner.OwnerKey, "file:")
			for _, sym := range symbols {
				if sym.FilePath == file {
					out = append(out, contextOwner{OwnerType: "symbol", OwnerKey: symbolOwnerKey(sym, identityKeys)})
				}
			}
		case "folder":
			folder := strings.TrimSuffix(strings.TrimPrefix(owner.OwnerKey, "folder:"), "/")
			for _, sym := range symbols {
				if sym.FilePath == folder || strings.HasPrefix(sym.FilePath, folder+"/") {
					out = append(out, contextOwner{OwnerType: "symbol", OwnerKey: symbolOwnerKey(sym, identityKeys)})
				}
			}
		}
	}
	return out
}

func hideOwnerPriority(owner contextOwner) int {
	switch owner.OwnerType {
	case "reference", "file-reference", "folder-reference":
		return 0
	case "symbol":
		return 1
	case "file", "folder", "cluster":
		return 2
	default:
		return 3
	}
}

func (s *Store) focusedRescanContextOwners(ctx context.Context, repositoryID int64, owners []contextOwner) error {
	repo, err := s.Repository(ctx, repositoryID)
	if err != nil {
		return err
	}
	files, err := s.contextOwnerFiles(ctx, repositoryID, owners)
	if err != nil {
		return err
	}
	if len(files) == 0 {
		return nil
	}
	scanner := NewScanner(s)
	_, err = scanner.ScanFilesWithOptions(ctx, repo, files, ScanOptions{Force: true})
	return err
}

func (s *Store) contextOwnerFiles(ctx context.Context, repositoryID int64, owners []contextOwner) ([]string, error) {
	symbols, err := s.SymbolsForRepository(ctx, repositoryID)
	if err != nil {
		return nil, err
	}
	identityKeys, err := s.SymbolIdentityKeys(ctx, repositoryID)
	if err != nil {
		return nil, err
	}
	files := map[string]struct{}{}
	for _, owner := range owners {
		switch owner.OwnerType {
		case "file":
			if file := strings.TrimPrefix(owner.OwnerKey, "file:"); file != "" {
				files[file] = struct{}{}
			}
		case "symbol":
			for _, sym := range symbols {
				if owner.OwnerKey == sym.StableKey || owner.OwnerKey == symbolOwnerKey(sym, identityKeys) {
					files[sym.FilePath] = struct{}{}
				}
			}
		}
	}
	out := make([]string, 0, len(files))
	for file := range files {
		out = append(out, file)
	}
	sort.Strings(out)
	return out, nil
}

func (s *Store) materializationByResource(ctx context.Context, repositoryID int64, resourceType string, resourceID int64) (watchMaterializationMapping, bool, error) {
	var item watchMaterializationMapping
	err := s.db.QueryRowContext(ctx, `
		SELECT id, owner_type, owner_key, resource_type, resource_id, updated_at
		FROM watch_materialization
		WHERE repository_id = ? AND resource_type = ? AND resource_id = ?
		ORDER BY id DESC
		LIMIT 1`, repositoryID, resourceType, resourceID).Scan(&item.ID, &item.OwnerType, &item.OwnerKey, &item.ResourceType, &item.ResourceID, &item.UpdatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return watchMaterializationMapping{}, false, nil
	}
	return item, err == nil, err
}

func (s *Store) materializedElementOwnersInView(ctx context.Context, repositoryID, viewID int64) ([]contextOwner, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT wm.owner_type, wm.owner_key
		FROM placements p
		JOIN watch_materialization wm ON wm.resource_type = 'element' AND wm.resource_id = p.element_id
		WHERE wm.repository_id = ? AND p.view_id = ?`, repositoryID, viewID)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	return scanContextOwners(rows)
}

func (s *Store) materializedConnectorOwnersInView(ctx context.Context, repositoryID, viewID int64) ([]contextOwner, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT wm.owner_type, wm.owner_key
		FROM connectors c
		JOIN watch_materialization wm ON wm.resource_type = 'connector' AND wm.resource_id = c.id
		WHERE wm.repository_id = ? AND c.view_id = ?`, repositoryID, viewID)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	return scanContextOwners(rows)
}

func (s *Store) materializedConnectorOwnersTouchingElement(ctx context.Context, repositoryID, elementID int64) ([]contextOwner, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT wm.owner_type, wm.owner_key
		FROM connectors c
		JOIN watch_materialization wm ON wm.resource_type = 'connector' AND wm.resource_id = c.id
		WHERE wm.repository_id = ? AND (c.source_element_id = ? OR c.target_element_id = ?)`, repositoryID, elementID, elementID)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	return scanContextOwners(rows)
}

func (s *Store) materializedNeighborOwners(ctx context.Context, repositoryID, elementID int64) ([]contextOwner, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT wm.owner_type, wm.owner_key
		FROM connectors c
		JOIN watch_materialization wm ON wm.resource_type = 'element'
			AND wm.resource_id = CASE WHEN c.source_element_id = ? THEN c.target_element_id ELSE c.source_element_id END
		WHERE wm.repository_id = ? AND (c.source_element_id = ? OR c.target_element_id = ?)`, elementID, repositoryID, elementID, elementID)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	return scanContextOwners(rows)
}

func scanContextOwners(rows *sql.Rows) ([]contextOwner, error) {
	var owners []contextOwner
	for rows.Next() {
		var owner contextOwner
		if err := rows.Scan(&owner.OwnerType, &owner.OwnerKey); err != nil {
			return nil, err
		}
		owners = append(owners, owner)
	}
	return uniqueContextOwners(owners), rows.Err()
}

func (s *Store) saveContextPolicies(ctx context.Context, repositoryID int64, action, scope string, owners []contextOwner) (int, int, int, error) {
	now := nowString()
	opposite := contextActionShow
	if action == contextActionShow {
		opposite = contextActionHide
	}
	created, updated, deactivated := 0, 0, 0
	for _, owner := range uniqueContextOwners(owners) {
		res, err := s.db.ExecContext(ctx, `
			UPDATE watch_context_policies
			SET active = 0, updated_at = ?
			WHERE repository_id = ? AND owner_type = ? AND owner_key = ? AND action = ? AND active = 1`,
			now, repositoryID, owner.OwnerType, owner.OwnerKey, opposite)
		if err != nil {
			return 0, 0, 0, err
		}
		if rows, _ := res.RowsAffected(); rows > 0 {
			deactivated += int(rows)
		}
		var id int64
		err = s.db.QueryRowContext(ctx, `
			SELECT id FROM watch_context_policies
			WHERE repository_id = ? AND owner_type = ? AND owner_key = ? AND action = ? AND active = 1
			ORDER BY id DESC LIMIT 1`, repositoryID, owner.OwnerType, owner.OwnerKey, action).Scan(&id)
		if err != nil && !errors.Is(err, sql.ErrNoRows) {
			return 0, 0, 0, err
		}
		reason := "user context " + action
		if id != 0 {
			if _, err := s.db.ExecContext(ctx, `UPDATE watch_context_policies SET scope = ?, reason = ?, updated_at = ? WHERE id = ?`, scope, reason, now, id); err != nil {
				return 0, 0, 0, err
			}
			updated++
			continue
		}
		if _, err := s.db.ExecContext(ctx, `
			INSERT INTO watch_context_policies(repository_id, owner_type, owner_key, action, scope, active, reason, created_at, updated_at)
			VALUES (?, ?, ?, ?, ?, 1, ?, ?, ?)`, repositoryID, owner.OwnerType, owner.OwnerKey, action, scope, reason, now, now); err != nil {
			return 0, 0, 0, err
		}
		created++
	}
	return created, updated, deactivated, nil
}

func (s *Store) ActiveContextPolicySet(ctx context.Context, repositoryID int64) (contextPolicySet, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT owner_type, owner_key, action
		FROM watch_context_policies
		WHERE repository_id = ? AND active = 1
		ORDER BY id`, repositoryID)
	if err != nil {
		return contextPolicySet{}, err
	}
	defer func() { _ = rows.Close() }()
	policies := contextPolicySet{Show: map[string]string{}, Hide: map[string]string{}}
	for rows.Next() {
		var ownerType, ownerKey, action string
		if err := rows.Scan(&ownerType, &ownerKey, &action); err != nil {
			return contextPolicySet{}, err
		}
		key := ownerMapKey(ownerType, ownerKey)
		switch action {
		case contextActionShow:
			policies.Show[key] = "user marked as context"
			delete(policies.Hide, key)
		case contextActionHide:
			if _, shown := policies.Show[key]; !shown {
				policies.Hide[key] = "user marked as noise"
			}
		}
	}
	return policies, rows.Err()
}

func uniqueContextOwners(owners []contextOwner) []contextOwner {
	seen := map[string]struct{}{}
	var out []contextOwner
	for _, owner := range owners {
		owner.OwnerType = strings.TrimSpace(owner.OwnerType)
		owner.OwnerKey = strings.TrimSpace(owner.OwnerKey)
		if owner.OwnerType == "" || owner.OwnerKey == "" {
			continue
		}
		key := ownerKey(owner)
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		out = append(out, owner)
	}
	return out
}

func ownerKey(owner contextOwner) string {
	return ownerMapKey(owner.OwnerType, owner.OwnerKey)
}

func ownerMapKey(ownerType, ownerKey string) string {
	return ownerType + "\x00" + ownerKey
}

func (p contextExpansionSet) ownerTier(ownerType, ownerKey string) int {
	if p.Tiers == nil {
		return 0
	}
	return p.Tiers[ownerMapKey(ownerType, ownerKey)]
}

func (p contextExpansionSet) symbolTier(sym Symbol, identityKeys map[string]string) int {
	tier := maxInt(
		p.ownerTier("symbol", symbolOwnerKey(sym, identityKeys)),
		p.ownerTier("symbol", sym.StableKey),
		p.ownerTier("file", "file:"+sym.FilePath),
	)
	dir := path.Dir(sym.FilePath)
	for dir != "." && dir != "/" && dir != "" {
		tier = maxInt(tier, p.ownerTier("folder", "folder:"+dir))
		next := path.Dir(dir)
		if next == dir {
			break
		}
		dir = next
	}
	return tier
}

func (p contextExpansionSet) fileTier(filePath string) int {
	tier := p.ownerTier("file", "file:"+filePath)
	dir := path.Dir(filePath)
	for dir != "." && dir != "/" && dir != "" {
		tier = maxInt(tier, p.ownerTier("folder", "folder:"+dir))
		next := path.Dir(dir)
		if next == dir {
			break
		}
		dir = next
	}
	return tier
}

func maxInt(values ...int) int {
	out := 0
	for _, value := range values {
		if value > out {
			out = value
		}
	}
	return out
}

func referenceOwnerKey(ref Reference, symbols map[int64]Symbol, identityKeys map[string]string) string {
	source := symbols[ref.SourceSymbolID]
	target := symbols[ref.TargetSymbolID]
	return fmt.Sprintf("symbol:%s:%s:%s", symbolOwnerKey(source, identityKeys), symbolOwnerKey(target, identityKeys), ref.Kind)
}
