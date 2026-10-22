package api

import (
	pb "buf.build/gen/go/tldiagramcom/diagram/protocolbuffers/go/diag/v1"
	"context"
	"github.com/google/uuid"
)

type planHooksKey struct{}

// Notifications may read through the root store. Run them only after the
// transaction releases its connection, and discard them on rollback.
type planCommitHooks struct {
	WorkspaceHooks
	pending []func()
}

func (s *PlanService) commitHooks(ctx context.Context) (context.Context, *planCommitHooks) {
	base := s.Hooks
	if base == nil {
		base = NopWorkspaceHooks{}
	}
	hooks := &planCommitHooks{WorkspaceHooks: base}
	return context.WithValue(ctx, planHooksKey{}, hooks), hooks
}
func (h *planCommitHooks) AfterWrite(ctx context.Context, id uuid.UUID, action, kind, key string, details map[string]any, response any) {
	h.pending = append(h.pending, func() { h.WorkspaceHooks.AfterWrite(ctx, id, action, kind, key, details, response) })
}
func (h *planCommitHooks) AfterApplyBatch(ctx context.Context, id uuid.UUID, req *pb.ApplyWorkspaceBatchRequest, resp *pb.ApplyWorkspaceBatchResponse) {
	h.pending = append(h.pending, func() { h.WorkspaceHooks.AfterApplyBatch(ctx, id, req, resp) })
}
func (h *planCommitHooks) flush() {
	for _, notify := range h.pending {
		notify()
	}
}
