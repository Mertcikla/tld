package cmd

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	diagv1connect "buf.build/gen/go/tldiagramcom/diagram/connectrpc/go/diag/v1/diagv1connect"
	diagv1 "buf.build/gen/go/tldiagramcom/diagram/protocolbuffers/go/diag/v1"
	"connectrpc.com/connect"
	"google.golang.org/protobuf/types/known/timestamppb"
)

// RunCmd executes a tld command rooted at dir with the given args.
func RunCmd(t *testing.T, dir string, args ...string) (stdout, stderr string, err error) {
	t.Helper()
	return RunCmdWithStdin(t, dir, strings.NewReader(""), args...)
}

// MustRunCmd is like RunCmd but fails the test on error.
func MustRunCmd(t *testing.T, dir string, args ...string) (stdout, stderr string) {
	t.Helper()
	stdout, stderr, err := RunCmd(t, dir, args...)
	if err != nil {
		t.Fatalf("runCmd %v failed: %v\nstdout: %s\nstderr: %s", args, err, stdout, stderr)
	}
	return stdout, stderr
}

// RunCmdWithStdin is like RunCmd but allows injecting stdin content.
func RunCmdWithStdin(t *testing.T, dir string, stdin io.Reader, args ...string) (stdout, stderr string, err error) {
	t.Helper()

	// Isolate configuration
	if os.Getenv("TLD_CONFIG_DIR") == "" {
		configDir := t.TempDir()
		_ = os.Setenv("TLD_CONFIG_DIR", configDir)
		t.Cleanup(func() { _ = os.Unsetenv("TLD_CONFIG_DIR") })
	}

	root := NewRootCmd()
	outBuf, errBuf := &bytes.Buffer{}, &bytes.Buffer{}
	root.SetOut(outBuf)
	root.SetErr(errBuf)
	root.SetIn(stdin)
	root.SetArgs(append([]string{"--workspace", dir}, args...))
	err = root.Execute()
	return outBuf.String(), errBuf.String(), err
}

// MustInitWorkspace runs "tld init <dir>" and fails the test on error.
func MustInitWorkspace(t *testing.T, dir string) {
	t.Helper()
	_, _, err := RunCmd(t, ".", "init", dir)
	if err != nil {
		t.Fatalf("init workspace: %v", err)
	}
}

// InitGitRepo initializes a git repo in dir and commits a file.
func InitGitRepo(t *testing.T, dir string, filename string, source string) {
	t.Helper()
	if err := os.MkdirAll(dir, 0750); err != nil {
		t.Fatal(err)
	}
	run := func(args ...string) {
		cmd := exec.Command("git", args...)
		cmd.Dir = dir
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v failed: %v\n%s", args, err, out)
		}
	}
	run("init")
	run("config", "user.email", "test@example.com")
	run("config", "user.name", "Test User")
	run("config", "commit.gpgsign", "false")
	
	absPath := filepath.Join(dir, filename)
	if err := os.MkdirAll(filepath.Dir(absPath), 0750); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(absPath, []byte(source), 0600); err != nil {
		t.Fatal(err)
	}
	run("add", ".")
	run("commit", "-m", "initial commit")
}

// Mocking helpers

type MockDiagramService struct {
	diagv1connect.UnimplementedWorkspaceServiceHandler
	Mu                sync.Mutex
	LastRequest       *diagv1.ApplyPlanRequest
	LastHeader        http.Header
	ApplyFunc         func(*diagv1.ApplyPlanRequest) (*diagv1.ApplyPlanResponse, error)
	DeleteDiagramFunc func(*diagv1.DeleteViewRequest) (*diagv1.DeleteViewResponse, error)
	DeleteObjectFunc  func(*diagv1.DeleteElementRequest) (*diagv1.DeleteElementResponse, error)
	ExportFunc        func(*diagv1.ExportOrganizationRequest) (*diagv1.ExportOrganizationResponse, error)
}

func (m *MockDiagramService) ExportWorkspace(_ context.Context, req *connect.Request[diagv1.ExportOrganizationRequest]) (*connect.Response[diagv1.ExportOrganizationResponse], error) {
	if m.ExportFunc != nil {
		resp, err := m.ExportFunc(req.Msg)
		if err != nil {
			return nil, err
		}
		return connect.NewResponse(resp), nil
	}
	return connect.NewResponse(&diagv1.ExportOrganizationResponse{}), nil
}

func (m *MockDiagramService) ApplyWorkspacePlan(_ context.Context, req *connect.Request[diagv1.ApplyPlanRequest]) (*connect.Response[diagv1.ApplyPlanResponse], error) {
	m.Mu.Lock()
	m.LastRequest = req.Msg
	m.LastHeader = req.Header()
	m.Mu.Unlock()

	if m.ApplyFunc != nil {
		resp, err := m.ApplyFunc(req.Msg)
		if err != nil {
			return nil, err
		}
		return connect.NewResponse(resp), nil
	}
	return connect.NewResponse(SuccessResponse(req.Msg)), nil
}

func SuccessResponse(req *diagv1.ApplyPlanRequest) *diagv1.ApplyPlanResponse {
	resp := &diagv1.ApplyPlanResponse{
		Summary: &diagv1.PlanSummary{
			ElementsPlanned:   int32(len(req.Elements)),
			ElementsCreated:   int32(len(req.Elements)),
			ConnectorsPlanned: int32(len(req.Connectors)),
			ConnectorsCreated: int32(len(req.Connectors)),
		},
		ElementMetadata:   make(map[string]*diagv1.ResourceMetadata),
		ViewMetadata:      make(map[string]*diagv1.ResourceMetadata),
		ConnectorMetadata: make(map[string]*diagv1.ResourceMetadata),
	}

	var diagramCount int32
	var nextID int32 = 1
	for _, element := range req.Elements {
		elementID := nextID
		nextID++
		resp.CreatedElements = append(resp.CreatedElements, &diagv1.Element{
			Id:        elementID,
			Name:      element.Name,
			Kind:      element.Kind,
			HasView:   element.HasView,
			ViewLabel: element.ViewLabel,
		})
		resp.ElementMetadata[element.Ref] = &diagv1.ResourceMetadata{Id: elementID, UpdatedAt: timestamppb.Now()}
		if element.HasView {
			diagramID := nextID
			nextID++
			diagramCount++
			resp.CreatedViews = append(resp.CreatedViews, &diagv1.ViewSummary{
				Id:             diagramID,
				OwnerElementId: &elementID,
				Name:           element.Name,
				Label:          element.ViewLabel,
			})
			resp.ViewMetadata[element.Ref] = &diagv1.ResourceMetadata{Id: diagramID, UpdatedAt: timestamppb.Now()}
		}
	}
	resp.Summary.ViewsPlanned = diagramCount
	resp.Summary.ViewsCreated = diagramCount

	for _, connector := range req.Connectors {
		connectorID := nextID
		nextID++
		resp.CreatedConnectors = append(resp.CreatedConnectors, &diagv1.Connector{
			Id:              connectorID,
			SourceElementId: 99,
			TargetElementId: 100,
			Label:           connector.Label,
			Direction:       ValueOr(connector.Direction, "forward"),
			Style:           ValueOr(connector.Style, "solid"),
		})
		resp.ConnectorMetadata[connector.Ref] = &diagv1.ResourceMetadata{Id: connectorID, UpdatedAt: timestamppb.Now()}
	}

	return resp
}

func ValueOr(value *string, fallback string) string {
	if value == nil || *value == "" {
		return fallback
	}
	return *value
}

func NewMockServer(t *testing.T, svc diagv1connect.WorkspaceServiceHandler) string {
	t.Helper()
	mux := http.NewServeMux()
	path, handler := diagv1connect.NewWorkspaceServiceHandler(svc)
	mux.Handle("/api"+path, http.StripPrefix("/api", handler))
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv.URL
}

const TestWorkspaceID = "aaaaaaaa-bbbb-cccc-dddd-eeeeeeeeeeee"

func WriteConfig(t *testing.T, _, serverURL, apiKey string) {
	t.Helper()
	configDir := os.Getenv("TLD_CONFIG_DIR")
	if configDir == "" {
		configDir = t.TempDir()
		t.Setenv("TLD_CONFIG_DIR", configDir)
	}
	cfg := fmt.Sprintf("server_url: %s\napi_key: %q\norg_id: %q\n", serverURL, apiKey, TestWorkspaceID)
	if err := os.MkdirAll(configDir, 0700); err != nil {
		t.Fatalf("mkdir config: %v", err)
	}
	if err := os.WriteFile(filepath.Join(configDir, "tld.yaml"), []byte(cfg), 0600); err != nil {
		t.Fatalf("write tld.yaml: %v", err)
	}
}

func SetupApplyWorkspace(t *testing.T, dir, serverURL string) {
	t.Helper()
	configDir := t.TempDir()
	t.Setenv("TLD_CONFIG_DIR", configDir)
	MustInitWorkspace(t, dir)
	WriteConfig(t, dir, serverURL, "test-api-key")
}

func SeedElementWorkspace(t *testing.T, dir string) {
	t.Helper()
	MustRunCmd(t, dir, "add", "Platform", "--ref", "platform", "--kind", "workspace")
	MustRunCmd(t, dir, "add", "API", "--ref", "api", "--parent", "platform", "--kind", "service")
	MustRunCmd(t, dir, "add", "DB", "--ref", "db", "--parent", "platform", "--kind", "database")
	MustRunCmd(t, dir, "connect", "--from", "api", "--to", "db", "--label", "reads")
}
