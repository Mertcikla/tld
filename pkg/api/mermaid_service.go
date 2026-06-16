package api

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"sort"
	"strconv"
	"strings"

	diagv1connect "buf.build/gen/go/tldiagramcom/diagram/connectrpc/go/diag/v1/diagv1connect"
	diagv1 "buf.build/gen/go/tldiagramcom/diagram/protocolbuffers/go/diag/v1"
	"connectrpc.com/connect"
	"github.com/google/uuid"
	"github.com/mertcikla/tld/v2/internal/mermaid"
)

var _ diagv1connect.MermaidServiceHandler = (*MermaidService)(nil)

type MermaidService struct {
	diagv1connect.UnimplementedMermaidServiceHandler

	Store Store
	Hooks WorkspaceHooks
}

func (s *MermaidService) hooks() WorkspaceHooks {
	if s.Hooks == nil {
		return NopWorkspaceHooks{}
	}
	return s.Hooks
}

func (s *MermaidService) ParseMermaid(ctx context.Context, req *connect.Request[diagv1.ParseMermaidRequest]) (*connect.Response[diagv1.ParseMermaidResponse], error) {
	parsed, err := mermaid.Parse(req.Msg.GetSource())
	if err != nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, err)
	}
	return connect.NewResponse(parsedMermaidResponse(parsed)), nil
}

func (s *MermaidService) ImportMermaidIntoView(ctx context.Context, req *connect.Request[diagv1.ImportMermaidIntoViewRequest]) (*connect.Response[diagv1.ImportMermaidIntoViewResponse], error) {
	m := req.Msg
	workspaceID, err := ResolveWorkspaceID(ctx, m.GetOrgId())
	if err != nil {
		return nil, err
	}
	if err := s.hooks().CheckWrite(ctx, workspaceID, "elements"); err != nil {
		return nil, err
	}
	viewID, err := parseRequiredInt32("view_id", m.GetViewId())
	if err != nil {
		return nil, err
	}
	if _, err := s.Store.GetView(ctx, viewID, workspaceID); err != nil {
		return nil, storeErr("get view", err)
	}
	parsed, err := mermaid.Parse(m.GetSource())
	if err != nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, err)
	}
	if len(parsed.Elements) == 0 && len(parsed.Connectors) == 0 {
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("no compatible diagram content found"))
	}

	result, err := s.importIntoViewSafely(ctx, workspaceID, viewID, parsed, mermaid.Point{X: m.GetCenterX(), Y: m.GetCenterY()}, m.GetDryRun())
	if err != nil {
		return nil, storeErr("import Mermaid", err)
	}
	if !m.GetDryRun() {
		content := result.Content
		applyResp := &diagv1.ApplyPlanResponse{}
		for _, placement := range content.GetPlacements() {
			if containsInt32(result.Summary.GetImportedElementIds(), placement.GetElementId()) {
				applyResp.CreatedPlacements = append(applyResp.CreatedPlacements, &diagv1.ElementPlacement{
					ViewId:    placement.GetViewId(),
					ElementId: placement.GetElementId(),
					PositionX: placement.GetPositionX(),
					PositionY: placement.GetPositionY(),
				})
			}
		}
		for _, connector := range content.GetConnectors() {
			if containsInt32(result.Summary.GetCreatedConnectorIds(), connector.GetId()) {
				applyResp.CreatedConnectors = append(applyResp.CreatedConnectors, connector)
			}
		}
		s.hooks().AfterApplyPlan(ctx, workspaceID, &diagv1.ApplyPlanRequest{OrgId: m.GetOrgId()}, applyResp)
	}
	return connect.NewResponse(result), nil
}

func (s *MermaidService) ExportMermaidView(ctx context.Context, req *connect.Request[diagv1.ExportMermaidViewRequest]) (*connect.Response[diagv1.ExportMermaidViewResponse], error) {
	m := req.Msg
	workspaceID, err := ResolveWorkspaceID(ctx, m.GetOrgId())
	if err != nil {
		return nil, err
	}
	if err := s.hooks().CheckRead(ctx, workspaceID); err != nil {
		return nil, err
	}
	viewID, err := parseRequiredInt32("view_id", m.GetViewId())
	if err != nil {
		return nil, err
	}
	content, err := s.Store.GetProjectedViewContent(ctx, viewID, workspaceID, m.DensityOverride)
	if err != nil {
		return nil, storeErr("get view content", err)
	}
	code := mermaid.ExportView(content, viewID, m.GetIncludeTldMetadata())
	resp := &diagv1.ExportMermaidViewResponse{Code: code}
	if m.GetMarkdownBlock() {
		resp.Markdown = mermaid.MermaidBlock(code)
	}
	return connect.NewResponse(resp), nil
}

func (s *MermaidService) InspectMermaidMarkdown(ctx context.Context, req *connect.Request[diagv1.InspectMermaidMarkdownRequest]) (*connect.Response[diagv1.InspectMermaidMarkdownResponse], error) {
	m := req.Msg
	workspaceID, err := ResolveWorkspaceID(ctx, m.GetOrgId())
	if err != nil {
		return nil, err
	}
	if err := s.hooks().CheckRead(ctx, workspaceID); err != nil {
		return nil, err
	}
	var (
		currentViewID *int32
		currentCode   string
		warnings      []string
	)
	if m.ViewId != nil {
		viewID := m.GetViewId()
		currentViewID = &viewID
		content, err := s.Store.GetProjectedViewContent(ctx, viewID, workspaceID, nil)
		if err != nil {
			return nil, storeErr("get view content", err)
		}
		currentCode = mermaid.ExportView(content, viewID, true)
	}
	blocks := mermaid.FindMarkdownBlocks(m.GetMarkdown())
	protoBlocks := make([]*diagv1.MermaidMarkdownBlockInfo, 0, len(blocks))
	status := diagv1.MermaidMarkdownSyncStatus_MERMAID_MARKDOWN_SYNC_STATUS_UNSPECIFIED
	for _, block := range blocks {
		blockStatus := mermaid.BlockSyncStatus(block, currentViewID, currentCode)
		protoBlocks = append(protoBlocks, markdownBlockToProto(block, blockStatus))
		if currentViewID != nil && block.ViewID != nil && *block.ViewID == *currentViewID {
			status = blockStatus
		}
	}
	if currentViewID != nil && status == diagv1.MermaidMarkdownSyncStatus_MERMAID_MARKDOWN_SYNC_STATUS_UNSPECIFIED {
		status = diagv1.MermaidMarkdownSyncStatus_MERMAID_MARKDOWN_SYNC_STATUS_MISSING
	}
	return connect.NewResponse(&diagv1.InspectMermaidMarkdownResponse{
		Blocks:     protoBlocks,
		SyncStatus: status,
		Warnings:   warnings,
	}), nil
}

func (s *MermaidService) UpsertMermaidMarkdownBlock(ctx context.Context, req *connect.Request[diagv1.UpsertMermaidMarkdownBlockRequest]) (*connect.Response[diagv1.UpsertMermaidMarkdownBlockResponse], error) {
	m := req.Msg
	workspaceID, err := ResolveWorkspaceID(ctx, m.GetOrgId())
	if err != nil {
		return nil, err
	}
	if err := s.hooks().CheckRead(ctx, workspaceID); err != nil {
		return nil, err
	}
	viewID, err := parseRequiredInt32("view_id", m.GetViewId())
	if err != nil {
		return nil, err
	}
	content, err := s.Store.GetProjectedViewContent(ctx, viewID, workspaceID, nil)
	if err != nil {
		return nil, storeErr("get view content", err)
	}
	code := mermaid.ExportView(content, viewID, m.GetIncludeTldMetadata())
	previousStatus := mermaid.SyncStatus(m.GetMarkdown(), viewID, code)
	return connect.NewResponse(&diagv1.UpsertMermaidMarkdownBlockResponse{
		Markdown:       mermaid.UpsertMarkdownBlock(m.GetMarkdown(), viewID, code),
		PreviousStatus: previousStatus,
	}), nil
}

func parsedMermaidResponse(parsed *mermaid.ParsedDiagram) *diagv1.ParseMermaidResponse {
	return &diagv1.ParseMermaidResponse{
		Elements:   parsed.Elements,
		Connectors: parsed.Connectors,
		Warnings:   parsed.Warnings,
		Direction:  mermaid.DirectionToProto(parsed.Direction),
		Source:     parsed.Source,
	}
}

func (s *MermaidService) importIntoView(ctx context.Context, workspaceID uuid.UUID, viewID int32, parsed *mermaid.ParsedDiagram, center mermaid.Point, dryRun bool) (*diagv1.ImportMermaidIntoViewResponse, error) {
	positions := mermaid.LayoutImport(parsed, center)
	placements, err := s.Store.ListPlacements(ctx, viewID)
	if err != nil {
		return nil, err
	}
	connectors, err := s.Store.ListConnectors(ctx, viewID, workspaceID)
	if err != nil {
		return nil, err
	}

	placedElementIDs := map[int32]struct{}{}
	for _, placement := range placements {
		placedElementIDs[placement.GetElementId()] = struct{}{}
	}
	existingConnectorByKey := map[string]*diagv1.Connector{}
	for _, connector := range connectors {
		existingConnectorByKey[connectorPersistenceKey(connector.GetSourceElementId(), connector.GetTargetElementId(), connector.GetLabel(), connector.GetRelationship())] = connector
	}

	summary := &diagv1.MermaidImportSummary{}
	elementsByRef := map[string]*diagv1.Element{}
	warnings := append([]string(nil), parsed.Warnings...)
	nextDryRunElementID := int32(-1)

	for _, element := range parsed.Elements {
		if existingID := mermaidRefElementID(element.GetRef()); existingID != 0 {
			existing, err := s.Store.GetElement(ctx, existingID, workspaceID)
			if err == nil {
				elementsByRef[element.GetRef()] = existing
				summary.ResolvedElementCount++
				summary.ResolvedElementIds = append(summary.ResolvedElementIds, existing.GetId())
				summary.ImportedElementIds = append(summary.ImportedElementIds, existing.GetId())
				if importedName, existingName := strings.TrimSpace(element.GetName()), strings.TrimSpace(existing.GetName()); importedName != "" && existingName != "" && importedName != existingName {
					warnings = append(warnings, fmt.Sprintf("%s will reuse existing workspace element %q", element.GetRef(), existingName))
				}
				continue
			}
			if !errors.Is(err, sql.ErrNoRows) {
				return nil, err
			}
		}

		if dryRun {
			elementsByRef[element.GetRef()] = &diagv1.Element{
				Id:   nextDryRunElementID,
				Name: element.GetName(),
			}
			nextDryRunElementID--
			summary.CreatedElementCount++
			continue
		}
		bypass := false
		if element.BypassNoiseGate != nil {
			bypass = element.GetBypassNoiseGate()
		}
		created, err := s.Store.CreateElement(ctx, workspaceID, ElementInput{
			Name:            element.GetName(),
			Description:     element.Description,
			Kind:            element.Kind,
			Technology:      element.Technology,
			URL:             element.Url,
			LogoURL:         element.LogoUrl,
			TechLinks:       element.GetTechnologyLinks(),
			Tags:            cloneStringSlice(element.GetTags()),
			Repo:            element.Repo,
			Branch:          element.Branch,
			Language:        element.Language,
			FilePath:        element.FilePath,
			BypassNoiseGate: &bypass,
			HasView:         element.GetHasView(),
			ViewLabel:       element.ViewLabel,
		})
		if err != nil {
			return nil, err
		}
		if element.GetHasView() {
			ownerID := created.GetId()
			if _, err := s.Store.CreateView(ctx, workspaceID, &ownerID, firstNonEmpty(element.GetName(), created.GetName()), element.ViewLabel, false); err != nil {
				return nil, err
			}
		}
		elementsByRef[element.GetRef()] = created
		summary.CreatedElementCount++
		summary.CreatedElementIds = append(summary.CreatedElementIds, created.GetId())
		summary.ImportedElementIds = append(summary.ImportedElementIds, created.GetId())
	}

	for _, element := range parsed.Elements {
		resolved := elementsByRef[element.GetRef()]
		if resolved == nil {
			continue
		}
		if _, ok := placedElementIDs[resolved.GetId()]; ok {
			continue
		}
		if dryRun {
			continue
		}
		position := positions[element.GetRef()]
		if _, err := s.Store.AddPlacement(ctx, viewID, resolved.GetId(), position.X, position.Y); err != nil {
			return nil, err
		}
		placedElementIDs[resolved.GetId()] = struct{}{}
	}

	handles := mermaid.ConnectorHandlesForDirection(parsed.Direction)
	for _, connector := range parsed.Connectors {
		source := elementsByRef[connector.GetSourceElementRef()]
		target := elementsByRef[connector.GetTargetElementRef()]
		if source == nil || target == nil {
			continue
		}
		key := connectorPersistenceKey(source.GetId(), target.GetId(), connector.GetLabel(), connector.GetRelationship())
		if existing := existingConnectorByKey[key]; existing != nil {
			summary.ResolvedConnectorCount++
			summary.ResolvedConnectorIds = append(summary.ResolvedConnectorIds, existing.GetId())
			if connectorWouldChangeExisting(existing, connector, handles) {
				warnings = append(warnings, fmt.Sprintf("Connector %d already exists; Mermaid style/metadata differences were not applied", existing.GetId()))
			}
			continue
		}
		if dryRun {
			summary.CreatedConnectorCount++
			continue
		}
		input := ConnectorInput{
			ViewID:       viewID,
			SourceID:     source.GetId(),
			TargetID:     target.GetId(),
			Label:        connector.Label,
			Description:  connector.Description,
			Relationship: connector.Relationship,
			Direction:    mermaidDerefStringDefault(connector.Direction, "forward"),
			Style:        mermaidDerefStringDefault(connector.Style, "bezier"),
			URL:          connector.Url,
			SourceHandle: defaultStringPointer(connector.SourceHandle, handles.Source),
			TargetHandle: defaultStringPointer(connector.TargetHandle, handles.Target),
		}
		created, err := s.Store.CreateConnector(ctx, workspaceID, input)
		if err != nil {
			return nil, err
		}
		existingConnectorByKey[key] = created
		summary.CreatedConnectorCount++
		summary.CreatedConnectorIds = append(summary.CreatedConnectorIds, created.GetId())
	}

	sortInt32s(summary.ImportedElementIds)
	sortInt32s(summary.ResolvedElementIds)
	sortInt32s(summary.CreatedElementIds)
	sortInt32s(summary.ResolvedConnectorIds)
	sortInt32s(summary.CreatedConnectorIds)

	content := &diagv1.ViewContent{}
	if !dryRun {
		content, err = s.Store.GetProjectedViewContent(ctx, viewID, workspaceID, nil)
		if err != nil {
			return nil, err
		}
	}
	return &diagv1.ImportMermaidIntoViewResponse{
		Summary:  summary,
		Warnings: warnings,
		Content:  content,
	}, nil
}

func (s *MermaidService) importIntoViewSafely(ctx context.Context, workspaceID uuid.UUID, viewID int32, parsed *mermaid.ParsedDiagram, center mermaid.Point, dryRun bool) (*diagv1.ImportMermaidIntoViewResponse, error) {
	if dryRun {
		return s.importIntoView(ctx, workspaceID, viewID, parsed, center, true)
	}
	transactional, ok := s.Store.(TransactionalStore)
	if !ok {
		return nil, fmt.Errorf("atomic Mermaid import: %w", ErrUnimplemented)
	}
	var result *diagv1.ImportMermaidIntoViewResponse
	err := transactional.RunInTransaction(ctx, func(txCtx context.Context, txStore Store) error {
		txService := *s
		if txStore != nil {
			txService.Store = txStore
		}
		var err error
		result, err = txService.importIntoView(txCtx, workspaceID, viewID, parsed, center, false)
		return err
	})
	if err != nil {
		return nil, err
	}
	if result == nil {
		return nil, errors.New("transactional Mermaid import returned no result")
	}
	return result, nil
}

func markdownBlockToProto(block mermaid.MarkdownBlock, status diagv1.MermaidMarkdownSyncStatus) *diagv1.MermaidMarkdownBlockInfo {
	info := &diagv1.MermaidMarkdownBlockInfo{
		Index:          int32(block.Index),
		Start:          int32(block.Start),
		End:            int32(block.End),
		CodeStart:      int32(block.CodeStart),
		CodeEnd:        int32(block.CodeEnd),
		LineStart:      int32(block.LineStart),
		LineEnd:        int32(block.LineEnd),
		Fence:          block.Fence,
		Code:           block.Code,
		HasTldMetadata: block.HasTldMetadata,
		Preview:        block.Preview,
		SyncStatus:     status,
	}
	if block.ViewID != nil {
		info.ViewId = block.ViewID
	}
	return info
}

func mermaidRefElementID(ref string) int32 {
	raw, ok := strings.CutPrefix(ref, "node_")
	if !ok {
		return 0
	}
	id, err := strconv.ParseInt(raw, 10, 32)
	if err != nil || id <= 0 {
		return 0
	}
	return int32(id)
}

func connectorPersistenceKey(sourceID, targetID int32, label, relationship string) string {
	return fmt.Sprintf("%d:%d:%s:%s", sourceID, targetID, strings.TrimSpace(label), strings.TrimSpace(relationship))
}

func connectorWouldChangeExisting(existing *diagv1.Connector, planned *diagv1.PlanConnector, handles mermaid.ConnectorHandles) bool {
	sourceHandle := handles.Source
	if planned.GetSourceHandle() != "" {
		sourceHandle = planned.GetSourceHandle()
	}
	targetHandle := handles.Target
	if planned.GetTargetHandle() != "" {
		targetHandle = planned.GetTargetHandle()
	}
	return strings.TrimSpace(existing.GetDescription()) != strings.TrimSpace(planned.GetDescription()) ||
		strings.TrimSpace(existing.GetDirection()) != strings.TrimSpace(mermaidDerefStringDefault(planned.Direction, "forward")) ||
		strings.TrimSpace(existing.GetStyle()) != strings.TrimSpace(mermaidDerefStringDefault(planned.Style, "bezier")) ||
		strings.TrimSpace(existing.GetUrl()) != strings.TrimSpace(planned.GetUrl()) ||
		strings.TrimSpace(existing.GetSourceHandle()) != strings.TrimSpace(sourceHandle) ||
		strings.TrimSpace(existing.GetTargetHandle()) != strings.TrimSpace(targetHandle)
}

func defaultStringPointer(value *string, fallback string) *string {
	if value != nil && strings.TrimSpace(*value) != "" {
		return value
	}
	if fallback == "" {
		return nil
	}
	return &fallback
}

func mermaidDerefStringDefault(value *string, fallback string) string {
	if value == nil || strings.TrimSpace(*value) == "" {
		return fallback
	}
	return *value
}

func cloneStringSlice(values []string) []string {
	if values == nil {
		return nil
	}
	return append([]string(nil), values...)
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

func sortInt32s(values []int32) {
	sort.Slice(values, func(i, j int) bool { return values[i] < values[j] })
}

func containsInt32(values []int32, needle int32) bool {
	for _, value := range values {
		if value == needle {
			return true
		}
	}
	return false
}
