package api

import (
	"context"
	"fmt"

	"buf.build/gen/go/tldiagramcom/diagram/connectrpc/go/diag/v1/diagv1connect"
	diagv1 "buf.build/gen/go/tldiagramcom/diagram/protocolbuffers/go/diag/v1"
	"connectrpc.com/connect"
)

var _ diagv1connect.WorkspaceVersionServiceHandler = (*WorkspaceVersionService)(nil)

// WorkspaceVersionService implements diagv1connect.WorkspaceVersionServiceHandler.
type WorkspaceVersionService struct {
	Store Store
	Hooks WorkspaceHooks
	diagv1connect.UnimplementedWorkspaceVersionServiceHandler
}

func (s *WorkspaceVersionService) hooks() WorkspaceHooks {
	if s.Hooks == nil {
		return NopWorkspaceHooks{}
	}
	return s.Hooks
}

// ListVersions returns workspace versions for the organisation.
func (s *WorkspaceVersionService) ListVersions(ctx context.Context, req *connect.Request[diagv1.ListVersionsRequest]) (*connect.Response[diagv1.ListVersionsResponse], error) {
	workspaceID := WorkspaceIDFromCtx(ctx)
	if err := s.hooks().CheckRead(ctx, workspaceID); err != nil {
		return nil, err
	}

	limit := int(req.Msg.GetLimit())
	if limit <= 0 {
		limit = 50
	}
	if limit > 1000 {
		limit = 1000
	}

	versions, err := s.Store.ListVersions(ctx, workspaceID, limit)
	if err != nil {
		return nil, storeErr("list versions", err)
	}

	return connect.NewResponse(&diagv1.ListVersionsResponse{
		Versions: versions,
	}), nil
}

// GetLatest returns the most recent workspace version for the organisation.
func (s *WorkspaceVersionService) GetLatest(ctx context.Context, req *connect.Request[diagv1.GetLatestVersionRequest]) (*connect.Response[diagv1.GetLatestVersionResponse], error) {
	workspaceID := WorkspaceIDFromCtx(ctx)
	if err := s.hooks().CheckRead(ctx, workspaceID); err != nil {
		return nil, err
	}

	version, err := s.Store.GetLatestVersion(ctx, workspaceID)
	if err != nil {
		return nil, storeErr("get latest version", err)
	}

	return connect.NewResponse(&diagv1.GetLatestVersionResponse{
		Version: version,
	}), nil
}

// CreateVersion records a new workspace version snapshot.
func (s *WorkspaceVersionService) CreateVersion(ctx context.Context, req *connect.Request[diagv1.CreateVersionRequest]) (*connect.Response[diagv1.CreateVersionResponse], error) {
	workspaceID := WorkspaceIDFromCtx(ctx)
	if err := s.hooks().CheckWrite(ctx, workspaceID, ""); err != nil {
		return nil, err
	}

	if req.Msg.GetVersionId() == "" {
		return nil, invalidArg("version_id", "must not be empty")
	}

	enabled, err := s.Store.GetVersioningEnabled(ctx, workspaceID)
	if err != nil {
		return nil, storeErr("check versioning enabled", err)
	}
	if !enabled {
		return nil, connect.NewError(connect.CodePermissionDenied, fmt.Errorf("workspace versioning is not enabled for this organisation"))
	}

	var parentID *int32
	latest, latestErr := s.Store.GetLatestVersion(ctx, workspaceID)
	if latestErr == nil && latest != nil {
		// WorkspaceVersionInfo.Id is a string representation of the int32 row id.
		// Parse it back for the parent pointer.
		var pid int32
		if _, scanErr := fmt.Sscanf(latest.GetId(), "%d", &pid); scanErr == nil && pid > 0 {
			parentID = &pid
		}
	}

	views, elements, connectors, err := s.Store.GetWorkspaceResourceCounts(ctx, workspaceID)
	if err != nil {
		return nil, storeErr("get org resource counts", err)
	}

	descPtr := OptStr(req.Msg.GetDescription())
	hashPtr := OptStr(req.Msg.GetWorkspaceHash())

	version, err := s.Store.CreateVersion(
		ctx, workspaceID,
		req.Msg.GetVersionId(),
		"frontend",
		parentID,
		views, elements, connectors,
		descPtr, hashPtr,
	)
	if err != nil {
		return nil, storeErr("create version", err)
	}

	resp := &diagv1.CreateVersionResponse{Version: version}
	s.hooks().AfterWrite(ctx, workspaceID, "create", "version", version.GetId(), nil, resp)

	return connect.NewResponse(resp), nil
}

// GetSettings returns the versioning settings for the organisation.
func (s *WorkspaceVersionService) GetSettings(ctx context.Context, req *connect.Request[diagv1.GetVersionSettingsRequest]) (*connect.Response[diagv1.GetVersionSettingsResponse], error) {
	workspaceID := WorkspaceIDFromCtx(ctx)
	if err := s.hooks().CheckRead(ctx, workspaceID); err != nil {
		return nil, err
	}

	enabled, err := s.Store.GetVersioningEnabled(ctx, workspaceID)
	if err != nil {
		return nil, storeErr("get versioning settings", err)
	}

	return connect.NewResponse(&diagv1.GetVersionSettingsResponse{
		CliVersioningEnabled: enabled,
	}), nil
}

// UpdateSettings updates the versioning settings for the organisation.
func (s *WorkspaceVersionService) UpdateSettings(ctx context.Context, req *connect.Request[diagv1.UpdateVersionSettingsRequest]) (*connect.Response[diagv1.UpdateVersionSettingsResponse], error) {
	workspaceID := WorkspaceIDFromCtx(ctx)
	if err := s.hooks().CheckWrite(ctx, workspaceID, ""); err != nil {
		return nil, err
	}

	if err := s.Store.SetVersioningEnabled(ctx, workspaceID, req.Msg.GetCliVersioningEnabled()); err != nil {
		return nil, storeErr("update versioning settings", err)
	}

	resp := &diagv1.UpdateVersionSettingsResponse{
		CliVersioningEnabled: req.Msg.GetCliVersioningEnabled(),
	}
	s.hooks().AfterWrite(ctx, workspaceID, "update", "settings", "versioning", nil, resp)

	return connect.NewResponse(resp), nil
}
