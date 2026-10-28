package flow

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"testing"

	diagv1 "buf.build/gen/go/tldiagramcom/diagram/protocolbuffers/go/diag/v1"
	"connectrpc.com/connect"
	"github.com/mertcikla/tld/v2/internal/cmdutil"
	"github.com/mertcikla/tld/v2/internal/exec"
	"github.com/mertcikla/tld/v2/internal/flowdoc"
)

type importRunner struct {
	exec.FlowRunner
	saveErr    error
	deleteErr  error
	saved      *diagv1.Flow
	deleted    []string
	batchCalls int
	reconcile  bool
}

func (r *importRunner) ApplyBatch(_ context.Context, req *diagv1.ApplyWorkspaceBatchRequest) (*diagv1.ApplyWorkspaceBatchResponse, error) {
	r.batchCalls++
	if req.GetDryRun() {
		return nil, fmt.Errorf("unexpected mutation call in a dry run")
	}
	return &diagv1.ApplyWorkspaceBatchResponse{ViewMetadata: map[string]*diagv1.ResourceMetadata{"flow-owner": {Id: 7}}, ElementMetadata: map[string]*diagv1.ResourceMetadata{"flow-owner": {Id: 10}, "element-0": {Id: 11}}, CreatedElements: []*diagv1.Element{{Id: 10}, {Id: 11}}}, nil
}
func (r *importRunner) ViewContent(context.Context, int32) (*diagv1.ViewContent, error) {
	return &diagv1.ViewContent{Placements: []*diagv1.PlacedElement{{ElementId: 11, Name: "API"}}}, nil
}
func (r *importRunner) SaveFlow(_ context.Context, req *diagv1.SaveFlowRequest) (*diagv1.Flow, error) {
	r.saved = &diagv1.Flow{Id: "saved", ViewId: req.ViewId, Revision: 1, Content: req.Content}
	return r.saved, r.saveErr
}
func (r *importRunner) ListFlows(context.Context, int32) ([]*diagv1.Flow, error) {
	if r.reconcile {
		return []*diagv1.Flow{r.saved}, nil
	}
	return nil, connect.NewError(connect.CodeUnavailable, fmt.Errorf("offline"))
}
func (r *importRunner) DeleteView(_ context.Context, id int32) error {
	r.deleted = append(r.deleted, fmt.Sprintf("view %d", id))
	return r.deleteErr
}
func (r *importRunner) DeleteElement(_ context.Context, id int32) error {
	r.deleted = append(r.deleted, fmt.Sprintf("element %d", id))
	return r.deleteErr
}
func TestCompleteImportRecovery(t *testing.T) {
	for _, tc := range []struct {
		name                  string
		saveErr, errorCleanup error
		reconcile, dry        bool
		wantErr, partial      bool
		deletes, batches      int
	}{
		{name: "success", batches: 1},
		{name: "preview", dry: true},
		{name: "definite failure", saveErr: connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("rejected")), wantErr: true, deletes: 3, batches: 1},
		{name: "cleanup failure", saveErr: connect.NewError(connect.CodePermissionDenied, fmt.Errorf("rejected")), errorCleanup: fmt.Errorf("cleanup denied"), wantErr: true, partial: true, deletes: 2, batches: 1},
		{name: "uncertain saved", saveErr: connect.NewError(connect.CodeUnavailable, fmt.Errorf("lost response")), reconcile: true, batches: 1},
		{name: "uncertain unknown", saveErr: connect.NewError(connect.CodeUnavailable, fmt.Errorf("offline")), wantErr: true, partial: true, batches: 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			runner := &importRunner{saveErr: tc.saveErr, deleteErr: tc.errorCleanup, reconcile: tc.reconcile}
			c := newImport()
			c.SetContext(context.Background())
			c.SetOut(&bytes.Buffer{})
			c.SetErr(&bytes.Buffer{})
			if tc.dry {
				if err := c.Flags().Set("dry-run", "true"); err != nil {
					t.Fatal(err)
				}
			}
			doc := &flowdoc.Document{Name: "Example", Steps: []flowdoc.Step{{ID: "s", Elements: []string{"API"}}}, Resources: &flowdoc.Resources{Elements: []flowdoc.Element{{Name: "API"}}}}
			err := importComplete(c, &environment{runner: runner}, doc, "Example view")
			if (err != nil) != tc.wantErr {
				t.Fatalf("unexpected error: %v", err)
			}
			var detail *cmdutil.CommandError
			if tc.partial && (!errors.As(err, &detail) || !detail.Applied) {
				t.Fatalf("missing applied-state error: %v", err)
			}
			if len(runner.deleted) != tc.deletes || runner.batchCalls != tc.batches {
				t.Fatalf("unexpected writes: deleted=%v batches=%d", runner.deleted, runner.batchCalls)
			}
		})
	}
}
func TestBundleValidationBeforeWrites(t *testing.T) {
	runner := &importRunner{}
	doc := &flowdoc.Document{Name: "Example", Steps: []flowdoc.Step{{ID: "s", GoToFlow: &flowdoc.Link{Name: "Missing"}}}, Resources: &flowdoc.Resources{}}
	if err := importComplete(newImport(), &environment{runner: runner}, doc, "New"); err == nil {
		t.Fatal("missing link accepted")
	}
	doc.Steps = nil
	doc.Resources.Elements = []flowdoc.Element{{Name: "API", TechnologyConnectors: []flowdoc.Technology{{Type: "custom", Label: "Custom", Primary: true}}}}
	if err := importComplete(newImport(), &environment{runner: runner}, doc, "New"); err == nil {
		t.Fatal("invalid metadata accepted")
	}
	if runner.batchCalls != 0 {
		t.Fatal("wrote before preflight succeeded")
	}
}
func TestUnqualifiedLinksCannotBindToAnotherView(t *testing.T) {
	doc := &flowdoc.Document{Name: "Example", Steps: []flowdoc.Step{{ID: "link", GoToFlow: &flowdoc.Link{Name: "Other"}}}, Resources: &flowdoc.Resources{}}
	env := &environment{runner: &importRunner{}, catalog: flowdoc.Catalog{Flows: []*diagv1.Flow{{Id: "other", ViewId: 1, Content: &diagv1.FlowContent{Name: "Other"}}}}}
	if err := importComplete(newImport(), env, doc, "New"); err == nil {
		t.Fatal("unqualified link bound to old diagram")
	}
}
func TestUnknownEnumDoesNotPanic(t *testing.T) {
	_, err := flowdoc.Parse([]byte("format: tld-flow\nversion: 1\nname: Example\nsteps: [{branches: {mode: {}, paths: []}}]\n"))
	if err == nil {
		t.Fatal("invalid enum accepted")
	}
}
