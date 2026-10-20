package api

import (
	"context"

	diagv1 "buf.build/gen/go/tldiagramcom/diagram/protocolbuffers/go/diag/v1"
	"github.com/google/uuid"
)

// WorkspaceHooks allows consumers (like the SaaS backend) to inject access control, limit checks,
// auditing, and real-time update logic into core services.
type WorkspaceHooks interface {
	// CheckRead performs access control before a read operation.
	CheckRead(ctx context.Context, workspaceID uuid.UUID) error

	// CheckWrite performs access control and resource limit checks before a write operation.
	// resourceType identifies the limited resource (e.g., "elements", "views").
	CheckWrite(ctx context.Context, workspaceID uuid.UUID, resourceType string) error

	// AfterWrite provides a hook to perform auditing, notifications, or real-time
	// real-time broadcasts (e.g., WebSockets) after a successful write operation.
	// 'action' is the operation type (e.g., "create", "update", "delete").
	// 'resourceType' is the type of resource (e.g., "element", "view").
	// 'resourceID' is the ID of the resource affected.
	// 'details' contains any additional contextual data for auditing or broadcasting.
	// 'response' contains the optional protobuf response object if further inspection is needed.
	AfterWrite(ctx context.Context, workspaceID uuid.UUID, action string, resourceType string, resourceID string, details map[string]any, response any)

	// CheckApplyBatch allows the SaaS backend to perform resource limit checks
	// specifically for the bulk ApplyBatch operation.
	CheckApplyBatch(ctx context.Context, workspaceID uuid.UUID, req *diagv1.ApplyWorkspaceBatchRequest) error

	// AfterApplyBatch provides a hook to perform layout, versioning, and cache
	// invalidation after a successful bulk ApplyBatch operation.
	AfterApplyBatch(ctx context.Context, workspaceID uuid.UUID, req *diagv1.ApplyWorkspaceBatchRequest, resp *diagv1.ApplyWorkspaceBatchResponse)
}

// NopWorkspaceHooks provides a no-op implementation for the CLI/standalone mode.
type NopWorkspaceHooks struct{}

var _ WorkspaceHooks = (*NopWorkspaceHooks)(nil)

func (NopWorkspaceHooks) CheckRead(context.Context, uuid.UUID) error          { return nil }
func (NopWorkspaceHooks) CheckWrite(context.Context, uuid.UUID, string) error { return nil }
func (NopWorkspaceHooks) AfterWrite(context.Context, uuid.UUID, string, string, string, map[string]any, any) {
}
func (NopWorkspaceHooks) CheckApplyBatch(context.Context, uuid.UUID, *diagv1.ApplyWorkspaceBatchRequest) error {
	return nil
}
func (NopWorkspaceHooks) AfterApplyBatch(context.Context, uuid.UUID, *diagv1.ApplyWorkspaceBatchRequest, *diagv1.ApplyWorkspaceBatchResponse) {
}
