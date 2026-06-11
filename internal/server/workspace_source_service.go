package server

import (
	"context"

	diagv1 "buf.build/gen/go/tldiagramcom/diagram/protocolbuffers/go/diag/v1"
	"connectrpc.com/connect"
	"github.com/google/uuid"
	"github.com/mertcikla/tld/v2/internal/workspacesource"
	"github.com/mertcikla/tld/v2/pkg/api"
)

type workspaceSourceStore interface {
	workspacesource.Store
}

type workspaceVersionService struct {
	*api.WorkspaceVersionService
	store       workspaceSourceStore
	workspaceID uuid.UUID
	base        workspacesource.Options
}

func (s *workspaceVersionService) GetWorkspaceSourceStatus(_ context.Context, req *connect.Request[diagv1.GetWorkspaceSourceStatusRequest]) (*connect.Response[diagv1.WorkspaceSourceStatus], error) {
	opts := s.options(req.Msg.GetRepository())
	return connect.NewResponse(workspaceSourceStatusToProto(workspacesource.GetStatus(opts))), nil
}

func (s *workspaceVersionService) ExportWorkspaceSource(ctx context.Context, req *connect.Request[diagv1.WorkspaceSourceRequest]) (*connect.Response[diagv1.WorkspaceSourceResult], error) {
	opts := s.options(req.Msg.GetRepository())
	status := workspacesource.GetStatus(opts)
	if !status.Available {
		return connect.NewResponse(workspaceSourceUnavailableResult(status, false)), nil
	}
	result, err := workspacesource.Export(ctx, s.store, opts)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, err)
	}
	return connect.NewResponse(workspaceSourceResultToProto(result)), nil
}

func (s *workspaceVersionService) ImportWorkspaceSource(ctx context.Context, req *connect.Request[diagv1.ImportWorkspaceSourceRequest]) (*connect.Response[diagv1.WorkspaceSourceResult], error) {
	opts := s.options(req.Msg.GetRepository())
	status := workspacesource.GetStatus(opts)
	if !status.Available {
		return connect.NewResponse(workspaceSourceUnavailableResult(status, req.Msg.GetDryRun())), nil
	}
	result, err := workspacesource.Import(ctx, s.store, opts, req.Msg.GetDryRun())
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, err)
	}
	return connect.NewResponse(workspaceSourceResultToProto(result)), nil
}

func (s *workspaceVersionService) options(repository string) workspacesource.Options {
	opts := s.base
	opts.WorkspaceID = s.workspaceID
	opts.RepositoryName = repository
	return opts
}

func workspaceSourceStatusToProto(status workspacesource.Status) *diagv1.WorkspaceSourceStatus {
	return &diagv1.WorkspaceSourceStatus{
		Available: status.Available,
		RootPath:  status.RootPath,
		ViewsDir:  status.ViewsDir,
		Message:   status.Message,
	}
}

func workspaceSourceUnavailableResult(status workspacesource.Status, dryRun bool) *diagv1.WorkspaceSourceResult {
	return &diagv1.WorkspaceSourceResult{
		Available:  status.Available,
		DryRun:     dryRun,
		RootPath:   status.RootPath,
		ViewsDir:   status.ViewsDir,
		Message:    status.Message,
		Views:      &diagv1.WorkspaceSourceCounts{},
		Elements:   &diagv1.WorkspaceSourceCounts{},
		Connectors: &diagv1.WorkspaceSourceCounts{},
	}
}

func workspaceSourceResultToProto(result *workspacesource.Result) *diagv1.WorkspaceSourceResult {
	if result == nil {
		return &diagv1.WorkspaceSourceResult{
			Views:      &diagv1.WorkspaceSourceCounts{},
			Elements:   &diagv1.WorkspaceSourceCounts{},
			Connectors: &diagv1.WorkspaceSourceCounts{},
		}
	}
	return &diagv1.WorkspaceSourceResult{
		Available:  result.Available,
		DryRun:     result.DryRun,
		RootPath:   result.RootPath,
		ViewsDir:   result.ViewsDir,
		Hash:       result.Hash,
		Views:      workspaceSourceCountsToProto(result.Views),
		Elements:   workspaceSourceCountsToProto(result.Elements),
		Connectors: workspaceSourceCountsToProto(result.Connectors),
		Warnings:   result.Warnings,
		Message:    result.Message,
	}
}

func workspaceSourceCountsToProto(counts workspacesource.ChangeCounts) *diagv1.WorkspaceSourceCounts {
	return &diagv1.WorkspaceSourceCounts{
		Planned: int32(counts.Planned),
		Applied: int32(counts.Applied),
		Created: int32(counts.Created),
		Updated: int32(counts.Updated),
		Deleted: int32(counts.Deleted),
	}
}
