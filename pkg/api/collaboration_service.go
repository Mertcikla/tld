package api

import (
	"context"
	"errors"
	"net"
	"net/http"
	"os"
	"regexp"
	"strconv"
	"strings"
	"time"

	"buf.build/gen/go/tldiagramcom/diagram/connectrpc/go/diag/v1/diagv1connect"
	diagv1 "buf.build/gen/go/tldiagramcom/diagram/protocolbuffers/go/diag/v1"
	"connectrpc.com/connect"
	"github.com/google/uuid"
)

const (
	collaborationClientIDHeader = "x-tld-collab-client-id"
	collaborationUserIDHeader   = "x-tld-collab-user-id"
	collaborationUsernameHeader = "x-tld-collab-username"
)

var _ diagv1connect.CollaborationServiceHandler = (*CollaborationService)(nil)

type CollaborationIdentityProvider interface {
	CurrentCollaborationIdentity(ctx context.Context, workspaceID uuid.UUID) (*CollaborationIdentity, bool, error)
}

type CollaborationService struct {
	Store            Store
	Hooks            WorkspaceHooks
	IdentityProvider CollaborationIdentityProvider
	Hub              *CollaborationHub

	diagv1connect.UnimplementedCollaborationServiceHandler
}

func (s *CollaborationService) hooks() WorkspaceHooks {
	if s.Hooks == nil {
		return NopWorkspaceHooks{}
	}
	return s.Hooks
}

type collaborationRequestInfoKey struct{}

type CollaborationRequestInfo struct {
	RemoteAddr string
}

func WithCollaborationRequestInfo(ctx context.Context, info CollaborationRequestInfo) context.Context {
	return context.WithValue(ctx, collaborationRequestInfoKey{}, info)
}

func collaborationRequestInfo(ctx context.Context) CollaborationRequestInfo {
	info, _ := ctx.Value(collaborationRequestInfoKey{}).(CollaborationRequestInfo)
	return info
}

func collaborationWorkspaceID(ctx context.Context) uuid.UUID {
	return WorkspaceIDFromCtx(ctx)
}

func (s *CollaborationService) ListThreads(ctx context.Context, req *connect.Request[diagv1.ListThreadsRequest]) (*connect.Response[diagv1.ListThreadsResponse], error) {
	workspaceID := collaborationWorkspaceID(ctx)
	viewID, elementID, connectorID, err := s.validateThreadTarget(ctx, workspaceID, req.Msg.GetViewId(), req.Msg.GetElementId(), req.Msg.GetConnectorId())
	if err != nil {
		return nil, err
	}
	if err := s.hooks().CheckRead(ctx, workspaceID); err != nil {
		return nil, err
	}
	threads, err := s.Store.ListViewThreads(ctx, workspaceID, viewID, elementID, connectorID)
	if err != nil {
		return nil, storeErr("list threads", err)
	}
	return connect.NewResponse(&diagv1.ListThreadsResponse{Threads: threads}), nil
}

func (s *CollaborationService) CreateThread(ctx context.Context, req *connect.Request[diagv1.CreateThreadRequest]) (*connect.Response[diagv1.CreateThreadResponse], error) {
	workspaceID := collaborationWorkspaceID(ctx)
	viewID, elementID, connectorID, err := s.validateThreadTarget(ctx, workspaceID, req.Msg.GetViewId(), req.Msg.GetElementId(), req.Msg.GetConnectorId())
	if err != nil {
		return nil, err
	}
	if err := s.hooks().CheckRead(ctx, workspaceID); err != nil {
		return nil, err
	}
	body := strings.TrimSpace(req.Msg.GetBody())
	if body == "" {
		return nil, invalidArg("body", "must not be empty")
	}
	identity, err := s.currentIdentity(ctx, workspaceID, req.Header())
	if err != nil {
		return nil, err
	}
	thread, err := s.Store.CreateViewThread(ctx, workspaceID, viewID, elementID, connectorID, identity.UserID, identity.Username)
	if err != nil {
		return nil, storeErr("create thread", err)
	}
	comment, err := s.Store.CreateViewComment(ctx, workspaceID, viewID, thread.GetId(), identity.UserID, identity.Username, body)
	if err != nil {
		return nil, storeErr("create comment", err)
	}
	thread.Comments = []*diagv1.CommentInfo{comment}
	if thread.CreatedByUsername == "" {
		thread.CreatedByUsername = identity.Username
	}
	s.hooks().AfterWrite(ctx, workspaceID, "create", "thread", strconv.Itoa(int(thread.GetId())), map[string]any{"view_id": viewID}, nil)
	s.broadcast(viewID, map[string]any{"type": "thread_upsert", "thread": thread})
	return connect.NewResponse(&diagv1.CreateThreadResponse{Thread: thread}), nil
}

func (s *CollaborationService) AddComment(ctx context.Context, req *connect.Request[diagv1.AddCommentRequest]) (*connect.Response[diagv1.AddCommentResponse], error) {
	workspaceID := collaborationWorkspaceID(ctx)
	viewID, err := parseRequiredInt32("view_id", req.Msg.GetViewId())
	if err != nil {
		return nil, err
	}
	threadID, err := parseRequiredInt32("thread_id", req.Msg.GetThreadId())
	if err != nil {
		return nil, err
	}
	if err := s.hooks().CheckRead(ctx, workspaceID); err != nil {
		return nil, err
	}
	if _, err := s.Store.GetView(ctx, viewID, workspaceID); err != nil {
		return nil, connect.NewError(connect.CodeNotFound, errors.New("view not found"))
	}
	if _, err := s.Store.GetViewThread(ctx, workspaceID, viewID, threadID); err != nil {
		return nil, connect.NewError(connect.CodeNotFound, errors.New("thread not found"))
	}
	body := strings.TrimSpace(req.Msg.GetBody())
	if body == "" {
		return nil, invalidArg("body", "must not be empty")
	}
	identity, err := s.currentIdentity(ctx, workspaceID, req.Header())
	if err != nil {
		return nil, err
	}
	comment, err := s.Store.CreateViewComment(ctx, workspaceID, viewID, threadID, identity.UserID, identity.Username, body)
	if err != nil {
		return nil, storeErr("add comment", err)
	}
	s.hooks().AfterWrite(ctx, workspaceID, "comment", "thread", strconv.Itoa(int(threadID)), map[string]any{"view_id": viewID}, nil)
	s.broadcast(viewID, map[string]any{"type": "comment_create", "comment": comment})
	return connect.NewResponse(&diagv1.AddCommentResponse{Comment: comment}), nil
}

func (s *CollaborationService) ResolveThread(ctx context.Context, req *connect.Request[diagv1.ResolveThreadRequest]) (*connect.Response[diagv1.ResolveThreadResponse], error) {
	workspaceID := collaborationWorkspaceID(ctx)
	viewID, err := parseRequiredInt32("view_id", req.Msg.GetViewId())
	if err != nil {
		return nil, err
	}
	threadID, err := parseRequiredInt32("thread_id", req.Msg.GetThreadId())
	if err != nil {
		return nil, err
	}
	if err := s.hooks().CheckRead(ctx, workspaceID); err != nil {
		return nil, err
	}
	if _, err := s.Store.GetViewThread(ctx, workspaceID, viewID, threadID); err != nil {
		return nil, connect.NewError(connect.CodeNotFound, errors.New("thread not found"))
	}
	if err := s.Store.SetViewThreadResolved(ctx, workspaceID, viewID, threadID, req.Msg.GetResolved()); err != nil {
		return nil, storeErr("resolve thread", err)
	}
	s.hooks().AfterWrite(ctx, workspaceID, "resolve", "thread", strconv.Itoa(int(threadID)), map[string]any{"view_id": viewID, "resolved": req.Msg.GetResolved()}, nil)
	s.broadcast(viewID, map[string]any{"type": "thread_resolve", "thread_id": threadID, "resolved": req.Msg.GetResolved()})
	return connect.NewResponse(&diagv1.ResolveThreadResponse{}), nil
}

func (s *CollaborationService) ListReactions(ctx context.Context, req *connect.Request[diagv1.ListReactionsRequest]) (*connect.Response[diagv1.ListReactionsResponse], error) {
	workspaceID := collaborationWorkspaceID(ctx)
	viewID, err := parseRequiredInt32("view_id", req.Msg.GetViewId())
	if err != nil {
		return nil, err
	}
	if err := s.hooks().CheckRead(ctx, workspaceID); err != nil {
		return nil, err
	}
	if _, err := s.Store.GetView(ctx, viewID, workspaceID); err != nil {
		return nil, connect.NewError(connect.CodeNotFound, errors.New("view not found"))
	}
	identity, err := s.currentIdentity(ctx, workspaceID, req.Header())
	if err != nil {
		return nil, err
	}
	reactions, err := s.Store.ListViewElementReactions(ctx, workspaceID, viewID, identity.UserID)
	if err != nil {
		return nil, storeErr("list reactions", err)
	}
	return connect.NewResponse(&diagv1.ListReactionsResponse{Reactions: reactions}), nil
}

func (s *CollaborationService) ToggleReaction(ctx context.Context, req *connect.Request[diagv1.ToggleReactionRequest]) (*connect.Response[diagv1.ToggleReactionResponse], error) {
	workspaceID := collaborationWorkspaceID(ctx)
	viewID, err := parseRequiredInt32("view_id", req.Msg.GetViewId())
	if err != nil {
		return nil, err
	}
	elementID, err := parseRequiredInt32("element_id", req.Msg.GetElementId())
	if err != nil {
		return nil, err
	}
	if err := s.hooks().CheckRead(ctx, workspaceID); err != nil {
		return nil, err
	}
	if err := s.validateElementInView(ctx, workspaceID, viewID, elementID); err != nil {
		return nil, err
	}
	emoji := strings.TrimSpace(req.Msg.GetEmoji())
	if emoji == "" || len([]rune(emoji)) > 8 {
		return nil, invalidArg("emoji", "must be a short non-empty value")
	}
	identity, err := s.currentIdentity(ctx, workspaceID, req.Header())
	if err != nil {
		return nil, err
	}
	active, err := s.Store.ToggleElementReaction(ctx, workspaceID, viewID, elementID, identity.UserID, emoji)
	if err != nil {
		return nil, storeErr("toggle reaction", err)
	}
	reactions, err := s.Store.ListViewElementReactions(ctx, workspaceID, viewID, identity.UserID)
	if err == nil {
		s.broadcast(viewID, map[string]any{"type": "reactions_snapshot", "items": reactions})
	}
	s.hooks().AfterWrite(ctx, workspaceID, "toggle", "reaction", strconv.Itoa(int(elementID)), map[string]any{"view_id": viewID, "emoji": emoji, "active": active}, nil)
	return connect.NewResponse(&diagv1.ToggleReactionResponse{Active: active}), nil
}

func (s *CollaborationService) validateThreadTarget(ctx context.Context, workspaceID uuid.UUID, rawViewID, rawElementID, rawConnectorID int32) (int32, *int32, *int32, error) {
	viewID, err := parseRequiredInt32("view_id", rawViewID)
	if err != nil {
		return 0, nil, nil, err
	}
	if _, err := s.Store.GetView(ctx, viewID, workspaceID); err != nil {
		return 0, nil, nil, connect.NewError(connect.CodeNotFound, errors.New("view not found"))
	}
	hasElement := rawElementID != 0
	hasConnector := rawConnectorID != 0
	if hasElement == hasConnector {
		return 0, nil, nil, invalidArg("element_id/connector_id", "provide exactly one of element_id or connector_id")
	}
	if hasElement {
		elementID, err := parseRequiredInt32("element_id", rawElementID)
		if err != nil {
			return 0, nil, nil, err
		}
		if err := s.validateElementInView(ctx, workspaceID, viewID, elementID); err != nil {
			return 0, nil, nil, err
		}
		return viewID, &elementID, nil, nil
	}
	connectorID, err := parseRequiredInt32("connector_id", rawConnectorID)
	if err != nil {
		return 0, nil, nil, err
	}
	if err := s.validateConnectorInView(ctx, workspaceID, viewID, connectorID); err != nil {
		return 0, nil, nil, err
	}
	return viewID, nil, &connectorID, nil
}

func (s *CollaborationService) validateElementInView(ctx context.Context, workspaceID uuid.UUID, viewID, elementID int32) error {
	placements, err := s.Store.ListPlacements(ctx, viewID)
	if err != nil {
		return storeErr("list placements", err)
	}
	for _, placement := range placements {
		if placement.GetElementId() == elementID {
			return nil
		}
	}
	return connect.NewError(connect.CodeInvalidArgument, errors.New("element not in view"))
}

func (s *CollaborationService) validateConnectorInView(ctx context.Context, workspaceID uuid.UUID, viewID, connectorID int32) error {
	connector, err := s.Store.GetConnector(ctx, connectorID, workspaceID)
	if err != nil {
		return connect.NewError(connect.CodeInvalidArgument, errors.New("connector not in view"))
	}
	if connector.GetViewId() != viewID {
		return connect.NewError(connect.CodeInvalidArgument, errors.New("connector not in view"))
	}
	return nil
}

func (s *CollaborationService) broadcast(viewID int32, payload any) {
	if s.Hub != nil {
		s.Hub.BroadcastViewEvent(viewID, payload)
	}
}

func (s *CollaborationService) currentIdentity(ctx context.Context, workspaceID uuid.UUID, header http.Header) (*CollaborationIdentity, error) {
	if s.IdentityProvider != nil {
		if identity, handled, err := s.IdentityProvider.CurrentCollaborationIdentity(ctx, workspaceID); handled || err != nil {
			if err != nil {
				return nil, err
			}
			return identity, nil
		}
	}
	return collaborationIdentityFromHeaderOrDefault(ctx, header, "user1")
}

func collaborationIdentityFromHeaderOrDefault(ctx context.Context, header http.Header, fallbackUserID string) (*CollaborationIdentity, error) {
	identity := &CollaborationIdentity{
		ClientID: strings.TrimSpace(header.Get(collaborationClientIDHeader)),
		UserID:   strings.TrimSpace(header.Get(collaborationUserIDHeader)),
		Username: strings.TrimSpace(header.Get(collaborationUsernameHeader)),
	}
	if identity.ClientID == "" {
		identity.ClientID = uuid.NewString()
	}
	if identity.UserID == "" {
		identity.UserID = normalizeUserIDCandidate(defaultIdentityCandidate(ctx))
	}
	if identity.UserID == "" {
		identity.UserID = fallbackUserID
	}
	if identity.Username == "" {
		identity.Username = identity.UserID
	}
	if err := validateCollaborationIdentity(identity); err != nil {
		return nil, err
	}
	return identity, nil
}

var userIDPattern = regexp.MustCompile(`^[A-Za-z0-9._-]+$`)

func validateCollaborationIdentity(identity *CollaborationIdentity) error {
	identity.ClientID = strings.TrimSpace(identity.ClientID)
	identity.UserID = strings.TrimSpace(identity.UserID)
	identity.Username = strings.TrimSpace(identity.Username)
	if identity.ClientID == "" {
		return invalidArg("client_id", "must not be empty")
	}
	if len(identity.ClientID) > 128 {
		return invalidArg("client_id", "must be 128 characters or fewer")
	}
	if identity.UserID == "" {
		return invalidArg("user_id", "must not be empty")
	}
	if identity.Username == "" {
		return invalidArg("username", "must not be empty")
	}
	if len(identity.UserID) > 64 {
		return invalidArg("user_id", "must be 64 characters or fewer")
	}
	if len(identity.Username) > 80 {
		return invalidArg("username", "must be 80 characters or fewer")
	}
	if !userIDPattern.MatchString(identity.UserID) {
		return invalidArg("user_id", "may only contain letters, numbers, dots, underscores, and hyphens")
	}
	if strings.ContainsAny(identity.Username, "\r\n\t") {
		return invalidArg("username", "must not contain control characters")
	}
	return nil
}

func defaultIdentityCandidate(ctx context.Context) string {
	info := collaborationRequestInfo(ctx)
	host := remoteHost(info.RemoteAddr)
	if ip := net.ParseIP(host); ip != nil {
		if !ip.IsLoopback() && !ip.IsUnspecified() {
			lookupCtx, cancel := context.WithTimeout(ctx, 150*time.Millisecond)
			defer cancel()
			if names, err := net.DefaultResolver.LookupAddr(lookupCtx, ip.String()); err == nil && len(names) > 0 {
				return strings.TrimSuffix(names[0], ".")
			}
			return ip.String()
		}
	}
	if hostname, err := os.Hostname(); err == nil && strings.TrimSpace(hostname) != "" {
		return hostname
	}
	return ""
}

func remoteHost(remoteAddr string) string {
	remoteAddr = strings.TrimSpace(remoteAddr)
	if remoteAddr == "" {
		return ""
	}
	if host, _, err := net.SplitHostPort(remoteAddr); err == nil {
		return host
	}
	return remoteAddr
}

func normalizeUserIDCandidate(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	value = strings.TrimSuffix(value, ".")
	var b strings.Builder
	for _, r := range value {
		switch {
		case r >= 'a' && r <= 'z':
			b.WriteRune(r)
		case r >= '0' && r <= '9':
			b.WriteRune(r)
		case r == '.' || r == '_' || r == '-':
			b.WriteRune(r)
		default:
			b.WriteRune('-')
		}
	}
	out := strings.Trim(b.String(), ".-_")
	if len(out) > 64 {
		out = out[:64]
		out = strings.Trim(out, ".-_")
	}
	return out
}
