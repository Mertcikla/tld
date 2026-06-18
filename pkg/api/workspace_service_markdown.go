package api

import (
	"context"
	"database/sql"
	"errors"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	diagv1 "buf.build/gen/go/tldiagramcom/diagram/protocolbuffers/go/diag/v1"
	"connectrpc.com/connect"
)

func (s *WorkspaceService) GetViewMarkdown(
	ctx context.Context,
	req *connect.Request[diagv1.GetViewMarkdownRequest],
) (*connect.Response[diagv1.GetViewMarkdownResponse], error) {
	workspaceID := WorkspaceIDFromCtx(ctx)
	if err := s.hooks().CheckRead(ctx, workspaceID); err != nil {
		return nil, err
	}
	viewID, err := parseRequiredInt32("view_id", req.Msg.GetViewId())
	if err != nil {
		return nil, err
	}
	markdown, content, err := s.Store.GetViewMarkdown(ctx, viewID, workspaceID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) || errors.Is(err, os.ErrNotExist) {
			return nil, connect.NewError(connect.CodeNotFound, errors.New("view markdown not found"))
		}
		return nil, storeErr("get view markdown", err)
	}
	return connect.NewResponse(&diagv1.GetViewMarkdownResponse{
		Markdown: markdown,
		Content:  content,
	}), nil
}

func (s *WorkspaceService) CreateViewMarkdown(
	ctx context.Context,
	req *connect.Request[diagv1.CreateViewMarkdownRequest],
) (*connect.Response[diagv1.CreateViewMarkdownResponse], error) {
	workspaceID := WorkspaceIDFromCtx(ctx)
	if err := s.hooks().CheckWrite(ctx, workspaceID, "views"); err != nil {
		return nil, err
	}
	viewID, err := parseRequiredInt32("view_id", req.Msg.GetViewId())
	if err != nil {
		return nil, err
	}
	if req.Msg.FileName != nil {
		trimmed := strings.TrimSpace(req.Msg.GetFileName())
		if trimmed == "" {
			return nil, invalidArg("file_name", "must not be empty when provided")
		}
		if strings.ContainsRune(trimmed, filepath.Separator) || strings.Contains(trimmed, "/") || strings.Contains(trimmed, "\\") {
			return nil, invalidArg("file_name", "must not contain path separators")
		}
	}
	targetKind := normalizeViewMarkdownTargetKind(req.Msg.GetTargetKind())
	if !isWritableViewMarkdownTargetKind(targetKind) {
		return nil, invalidArg("target_kind", "must be PRIVATE_WORKSPACE, PRIVATE_APP, or REPO")
	}
	if req.Msg.Path != nil {
		path := strings.TrimSpace(req.Msg.GetPath())
		if path == "" {
			return nil, invalidArg("path", "must not be empty when provided")
		}
		if !isMarkdownPath(path) {
			return nil, invalidArg("path", "must point to a markdown file")
		}
	}
	v, err := s.Store.CreateViewMarkdown(ctx, viewID, workspaceID, req.Msg.FileName, req.Msg.InitialContent, targetKind, req.Msg.Path)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) || errors.Is(err, os.ErrNotExist) {
			return nil, connect.NewError(connect.CodeNotFound, errors.New("view not found"))
		}
		return nil, storeErr("create view markdown", err)
	}
	resp := &diagv1.CreateViewMarkdownResponse{View: v}
	s.hooks().AfterWrite(ctx, workspaceID, "create", "view_markdown", strconv.Itoa(int(viewID)), nil, resp)
	return connect.NewResponse(resp), nil
}

func (s *WorkspaceService) LinkViewMarkdown(
	ctx context.Context,
	req *connect.Request[diagv1.LinkViewMarkdownRequest],
) (*connect.Response[diagv1.LinkViewMarkdownResponse], error) {
	workspaceID := WorkspaceIDFromCtx(ctx)
	if err := s.hooks().CheckWrite(ctx, workspaceID, "views"); err != nil {
		return nil, err
	}
	viewID, err := parseRequiredInt32("view_id", req.Msg.GetViewId())
	if err != nil {
		return nil, err
	}
	path := strings.TrimSpace(req.Msg.GetPath())
	if path == "" {
		return nil, invalidArg("path", "must not be empty")
	}
	if !isMarkdownPath(path) {
		return nil, invalidArg("path", "must point to a markdown file")
	}
	v, err := s.Store.LinkViewMarkdown(ctx, viewID, workspaceID, path)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) || errors.Is(err, os.ErrNotExist) {
			return nil, connect.NewError(connect.CodeNotFound, errors.New("markdown file not found"))
		}
		return nil, storeErr("link view markdown", err)
	}
	resp := &diagv1.LinkViewMarkdownResponse{View: v}
	s.hooks().AfterWrite(ctx, workspaceID, "update", "view_markdown", strconv.Itoa(int(viewID)), map[string]any{"path": path}, resp)
	return connect.NewResponse(resp), nil
}

func (s *WorkspaceService) SaveViewMarkdown(
	ctx context.Context,
	req *connect.Request[diagv1.SaveViewMarkdownRequest],
) (*connect.Response[diagv1.SaveViewMarkdownResponse], error) {
	workspaceID := WorkspaceIDFromCtx(ctx)
	if err := s.hooks().CheckWrite(ctx, workspaceID, "views"); err != nil {
		return nil, err
	}
	viewID, err := parseRequiredInt32("view_id", req.Msg.GetViewId())
	if err != nil {
		return nil, err
	}
	markdown, err := s.Store.SaveViewMarkdown(ctx, viewID, workspaceID, req.Msg.GetContent(), req.Msg.ExpectedFileVersion, req.Msg.GetForce())
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) || errors.Is(err, os.ErrNotExist) {
			return nil, connect.NewError(connect.CodeNotFound, errors.New("view markdown not found"))
		}
		return nil, storeErr("save view markdown", err)
	}
	resp := &diagv1.SaveViewMarkdownResponse{Markdown: markdown}
	s.hooks().AfterWrite(ctx, workspaceID, "update", "view_markdown", strconv.Itoa(int(viewID)), nil, resp)
	return connect.NewResponse(resp), nil
}

func (s *WorkspaceService) UnlinkViewMarkdown(
	ctx context.Context,
	req *connect.Request[diagv1.UnlinkViewMarkdownRequest],
) (*connect.Response[diagv1.UnlinkViewMarkdownResponse], error) {
	workspaceID := WorkspaceIDFromCtx(ctx)
	if err := s.hooks().CheckWrite(ctx, workspaceID, "views"); err != nil {
		return nil, err
	}
	viewID, err := parseRequiredInt32("view_id", req.Msg.GetViewId())
	if err != nil {
		return nil, err
	}
	v, err := s.Store.UnlinkViewMarkdown(ctx, viewID, workspaceID, req.Msg.GetDeleteManagedFile())
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) || errors.Is(err, os.ErrNotExist) {
			return nil, connect.NewError(connect.CodeNotFound, errors.New("view markdown not found"))
		}
		return nil, storeErr("unlink view markdown", err)
	}
	resp := &diagv1.UnlinkViewMarkdownResponse{View: v}
	s.hooks().AfterWrite(ctx, workspaceID, "delete", "view_markdown", strconv.Itoa(int(viewID)), nil, resp)
	return connect.NewResponse(resp), nil
}

func isMarkdownPath(path string) bool {
	lower := strings.ToLower(strings.TrimSpace(path))
	return strings.HasSuffix(lower, ".md") || strings.HasSuffix(lower, ".markdown") || strings.HasSuffix(lower, ".mdx")
}

func normalizeViewMarkdownTargetKind(value string) string {
	switch strings.ToUpper(strings.TrimSpace(value)) {
	case "", "PRIVATE":
		return "PRIVATE_WORKSPACE"
	case "PRIVATE_WORKSPACE", "PRIVATE_APP", "REPO", "ATTACHED":
		return strings.ToUpper(strings.TrimSpace(value))
	default:
		return strings.ToUpper(strings.TrimSpace(value))
	}
}

func isWritableViewMarkdownTargetKind(value string) bool {
	return value == "PRIVATE_WORKSPACE" || value == "PRIVATE_APP" || value == "REPO"
}
