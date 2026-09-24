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
	"testing/fstest"

	diagv1connect "buf.build/gen/go/tldiagramcom/diagram/connectrpc/go/diag/v1/diagv1connect"
	diagv1 "buf.build/gen/go/tldiagramcom/diagram/protocolbuffers/go/diag/v1"
	"connectrpc.com/connect"
	"github.com/google/uuid"
	assets "github.com/mertcikla/tld/v2"
	"github.com/mertcikla/tld/v2/internal/localserver"
	localapi "github.com/mertcikla/tld/v2/internal/server"
	"github.com/mertcikla/tld/v2/internal/store"
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
		t.Setenv("TLD_CONFIG_DIR", t.TempDir())
	}
	if os.Getenv("TLD_DATA_DIR") == "" {
		t.Setenv("TLD_DATA_DIR", t.TempDir())
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

// MockDiagramService is an in-memory WorkspaceService implementation for tests.
// It supports fine-grained CRUD (elements, views, placements, connectors) plus
// ExportWorkspace, so synchronous CLI commands can run against it. It seeds a
// single root view with ID 1.
type MockDiagramService struct {
	diagv1connect.UnimplementedWorkspaceServiceHandler
	Mu                  sync.Mutex
	ExportFunc          func(*diagv1.ExportOrganizationRequest) (*diagv1.ExportOrganizationResponse, error)
	ListElementsFunc    func(*diagv1.ListElementsRequest) ([]*diagv1.Element, error)
	DeleteElementFunc   func(*diagv1.DeleteElementRequest) (*diagv1.DeleteElementResponse, error)
	UpdateElementFunc   func(*diagv1.UpdateElementRequest) (*diagv1.UpdateElementResponse, error)
	UpdateConnectorFunc func(*diagv1.UpdateConnectorRequest) (*diagv1.UpdateConnectorResponse, error)

	nextID     int32
	elements   map[int32]*diagv1.Element
	views      map[int32]*diagv1.View
	placements map[string]*diagv1.PlacedElement
	connectors map[int32]*diagv1.Connector
}

func (m *MockDiagramService) initLocked() {
	if m.views != nil {
		return
	}
	m.nextID = 100
	m.elements = map[int32]*diagv1.Element{}
	m.views = map[int32]*diagv1.View{
		1: {Id: 1, Name: "Workspace Root", UpdatedAt: timestamppb.Now()},
	}
	m.placements = map[string]*diagv1.PlacedElement{}
	m.connectors = map[int32]*diagv1.Connector{}
}

func (m *MockDiagramService) allocLocked() int32 {
	m.nextID++
	return m.nextID
}

func placementKey(viewID, elementID int32) string {
	return fmt.Sprintf("%d:%d", viewID, elementID)
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

func (m *MockDiagramService) CreateElement(_ context.Context, req *connect.Request[diagv1.CreateElementRequest]) (*connect.Response[diagv1.CreateElementResponse], error) {
	m.Mu.Lock()
	defer m.Mu.Unlock()
	m.initLocked()
	in := req.Msg
	id := m.allocLocked()
	el := &diagv1.Element{
		Id:              id,
		Name:            in.GetName(),
		Kind:            in.Kind,
		Description:     in.Description,
		Technology:      in.Technology,
		Url:             in.Url,
		LogoUrl:         in.LogoUrl,
		TechnologyLinks: in.TechnologyLinks,
		Tags:            in.Tags,
		Repo:            in.Repo,
		Branch:          in.Branch,
		Language:        in.Language,
		FilePath:        in.FilePath,
		UpdatedAt:       timestamppb.Now(),
		CreatedAt:       timestamppb.Now(),
	}
	if in.BypassNoiseGate != nil {
		el.BypassNoiseGate = *in.BypassNoiseGate
	}
	m.elements[id] = el
	return connect.NewResponse(&diagv1.CreateElementResponse{Element: el}), nil
}

func (m *MockDiagramService) GetElement(_ context.Context, req *connect.Request[diagv1.GetElementRequest]) (*connect.Response[diagv1.GetElementResponse], error) {
	m.Mu.Lock()
	defer m.Mu.Unlock()
	m.initLocked()
	el, ok := m.elements[req.Msg.GetElementId()]
	if !ok {
		return nil, connect.NewError(connect.CodeNotFound, fmt.Errorf("element not found"))
	}
	return connect.NewResponse(&diagv1.GetElementResponse{Element: el}), nil
}

func (m *MockDiagramService) UpdateElement(_ context.Context, req *connect.Request[diagv1.UpdateElementRequest]) (*connect.Response[diagv1.UpdateElementResponse], error) {
	if m.UpdateElementFunc != nil {
		resp, err := m.UpdateElementFunc(req.Msg)
		if err != nil {
			return nil, err
		}
		return connect.NewResponse(resp), nil
	}
	m.Mu.Lock()
	defer m.Mu.Unlock()
	m.initLocked()
	in := req.Msg
	el, ok := m.elements[in.GetElementId()]
	if !ok {
		return nil, connect.NewError(connect.CodeNotFound, fmt.Errorf("element not found"))
	}
	if in.GetName() != "" {
		el.Name = in.GetName()
	}
	if in.Kind != nil {
		el.Kind = in.Kind
	}
	if in.Description != nil {
		el.Description = in.Description
	}
	if in.Technology != nil {
		el.Technology = in.Technology
	}
	if in.Url != nil {
		el.Url = in.Url
	}
	if in.LogoUrl != nil {
		el.LogoUrl = in.LogoUrl
	}
	if in.TechnologyLinks != nil {
		el.TechnologyLinks = in.TechnologyLinks
	}
	if in.Tags != nil {
		el.Tags = in.Tags
	}
	if in.Repo != nil {
		el.Repo = in.Repo
	}
	if in.Branch != nil {
		el.Branch = in.Branch
	}
	if in.Language != nil {
		el.Language = in.Language
	}
	if in.FilePath != nil {
		el.FilePath = in.FilePath
	}
	el.UpdatedAt = timestamppb.Now()
	return connect.NewResponse(&diagv1.UpdateElementResponse{Element: el}), nil
}

func (m *MockDiagramService) DeleteElement(_ context.Context, req *connect.Request[diagv1.DeleteElementRequest]) (*connect.Response[diagv1.DeleteElementResponse], error) {
	if m.DeleteElementFunc != nil {
		resp, err := m.DeleteElementFunc(req.Msg)
		if err != nil {
			return nil, err
		}
		return connect.NewResponse(resp), nil
	}
	m.Mu.Lock()
	defer m.Mu.Unlock()
	m.initLocked()
	id := req.Msg.GetElementId()
	if _, ok := m.elements[id]; !ok {
		return nil, connect.NewError(connect.CodeNotFound, fmt.Errorf("element not found"))
	}
	delete(m.elements, id)
	for key, p := range m.placements {
		if p.GetElementId() == id {
			delete(m.placements, key)
		}
	}
	for viewID, v := range m.views {
		if v.OwnerElementId != nil && *v.OwnerElementId == id {
			delete(m.views, viewID)
		}
	}
	for connID, c := range m.connectors {
		if c.GetSourceElementId() == id || c.GetTargetElementId() == id {
			delete(m.connectors, connID)
		}
	}
	return connect.NewResponse(&diagv1.DeleteElementResponse{}), nil
}

func (m *MockDiagramService) ListElements(_ context.Context, req *connect.Request[diagv1.ListElementsRequest]) (*connect.Response[diagv1.ListElementsResponse], error) {
	if m.ListElementsFunc != nil {
		els, err := m.ListElementsFunc(req.Msg)
		if err != nil {
			return nil, err
		}
		return connect.NewResponse(&diagv1.ListElementsResponse{Elements: els}), nil
	}
	m.Mu.Lock()
	defer m.Mu.Unlock()
	m.initLocked()
	var out []*diagv1.Element
	for _, el := range m.elements {
		if search := req.Msg.GetSearch(); search != "" && !strings.Contains(el.GetName(), search) {
			continue
		}
		out = append(out, el)
	}
	return connect.NewResponse(&diagv1.ListElementsResponse{Elements: out}), nil
}

// ElementCount returns the number of elements stored on the mock server.
func (m *MockDiagramService) ElementCount() int {
	m.Mu.Lock()
	defer m.Mu.Unlock()
	m.initLocked()
	return len(m.elements)
}

// Element returns the element with the given ID, or nil when absent.
func (m *MockDiagramService) Element(id int32) *diagv1.Element {
	m.Mu.Lock()
	defer m.Mu.Unlock()
	m.initLocked()
	return m.elements[id]
}

// RemoveElement deletes an element from the mock server without going through
// the ConnectRPC handler, simulating an out-of-band deletion.
func (m *MockDiagramService) RemoveElement(id int32) {
	m.Mu.Lock()
	defer m.Mu.Unlock()
	m.initLocked()
	delete(m.elements, id)
}

func (m *MockDiagramService) ListViews(_ context.Context, _ *connect.Request[diagv1.ListViewsRequest]) (*connect.Response[diagv1.ListViewsResponse], error) {
	m.Mu.Lock()
	defer m.Mu.Unlock()
	m.initLocked()
	var out []*diagv1.View
	for _, v := range m.views {
		out = append(out, v)
	}
	return connect.NewResponse(&diagv1.ListViewsResponse{Views: out}), nil
}

func (m *MockDiagramService) CreateView(_ context.Context, req *connect.Request[diagv1.CreateViewRequest]) (*connect.Response[diagv1.CreateViewResponse], error) {
	m.Mu.Lock()
	defer m.Mu.Unlock()
	m.initLocked()
	in := req.Msg
	if in.OwnerElementId != nil {
		for _, v := range m.views {
			if v.OwnerElementId != nil && *v.OwnerElementId == *in.OwnerElementId {
				return connect.NewResponse(&diagv1.CreateViewResponse{View: v}), nil
			}
		}
	}
	id := m.allocLocked()
	v := &diagv1.View{
		Id:             id,
		Name:           in.GetName(),
		LevelLabel:     in.LevelLabel,
		OwnerElementId: in.OwnerElementId,
		UpdatedAt:      timestamppb.Now(),
		CreatedAt:      timestamppb.Now(),
	}
	m.views[id] = v
	return connect.NewResponse(&diagv1.CreateViewResponse{View: v}), nil
}

func (m *MockDiagramService) UpdateView(_ context.Context, req *connect.Request[diagv1.UpdateViewRequest]) (*connect.Response[diagv1.UpdateViewResponse], error) {
	m.Mu.Lock()
	defer m.Mu.Unlock()
	m.initLocked()
	v, ok := m.views[req.Msg.GetViewId()]
	if !ok {
		return nil, connect.NewError(connect.CodeNotFound, fmt.Errorf("view not found"))
	}
	if req.Msg.GetName() != "" {
		v.Name = req.Msg.GetName()
	}
	if req.Msg.LevelLabel != nil {
		v.LevelLabel = req.Msg.LevelLabel
	}
	if req.Msg.Description != nil {
		v.Description = req.Msg.Description
	}
	v.UpdatedAt = timestamppb.Now()
	return connect.NewResponse(&diagv1.UpdateViewResponse{View: v}), nil
}

// View returns the stored view by ID. It is a test helper for asserting
// server-side view state after synchronous commands.
func (m *MockDiagramService) View(id int32) *diagv1.View {
	m.Mu.Lock()
	defer m.Mu.Unlock()
	m.initLocked()
	return m.views[id]
}

// Connector returns the stored connector by ID. It is a test helper for
// asserting server-side connector state after synchronous commands.
func (m *MockDiagramService) Connector(id int32) *diagv1.Connector {
	m.Mu.Lock()
	defer m.Mu.Unlock()
	m.initLocked()
	return m.connectors[id]
}

func (m *MockDiagramService) DeleteView(_ context.Context, req *connect.Request[diagv1.DeleteViewRequest]) (*connect.Response[diagv1.DeleteViewResponse], error) {
	m.Mu.Lock()
	defer m.Mu.Unlock()
	m.initLocked()
	delete(m.views, req.Msg.GetViewId())
	return connect.NewResponse(&diagv1.DeleteViewResponse{}), nil
}

func (m *MockDiagramService) CreatePlacement(_ context.Context, req *connect.Request[diagv1.CreatePlacementRequest]) (*connect.Response[diagv1.CreatePlacementResponse], error) {
	m.Mu.Lock()
	defer m.Mu.Unlock()
	m.initLocked()
	in := req.Msg
	key := placementKey(in.GetViewId(), in.GetElementId())
	if existing, ok := m.placements[key]; ok {
		return connect.NewResponse(&diagv1.CreatePlacementResponse{Placement: existing}), nil
	}
	p := &diagv1.PlacedElement{
		Id:        m.allocLocked(),
		ViewId:    in.GetViewId(),
		ElementId: in.GetElementId(),
		PositionX: in.GetPositionX(),
		PositionY: in.GetPositionY(),
	}
	m.placements[key] = p
	return connect.NewResponse(&diagv1.CreatePlacementResponse{Placement: p}), nil
}

func (m *MockDiagramService) UpdatePlacementPosition(_ context.Context, req *connect.Request[diagv1.UpdatePlacementPositionRequest]) (*connect.Response[diagv1.UpdatePlacementPositionResponse], error) {
	m.Mu.Lock()
	defer m.Mu.Unlock()
	m.initLocked()
	in := req.Msg
	if p, ok := m.placements[placementKey(in.GetViewId(), in.GetElementId())]; ok {
		p.PositionX = in.GetPositionX()
		p.PositionY = in.GetPositionY()
		return connect.NewResponse(&diagv1.UpdatePlacementPositionResponse{}), nil
	}
	return nil, connect.NewError(connect.CodeNotFound, fmt.Errorf("placement not found"))
}

func (m *MockDiagramService) DeletePlacement(_ context.Context, req *connect.Request[diagv1.DeletePlacementRequest]) (*connect.Response[diagv1.DeletePlacementResponse], error) {
	m.Mu.Lock()
	defer m.Mu.Unlock()
	m.initLocked()
	delete(m.placements, placementKey(req.Msg.GetViewId(), req.Msg.GetElementId()))
	return connect.NewResponse(&diagv1.DeletePlacementResponse{}), nil
}

func (m *MockDiagramService) CreateConnector(_ context.Context, req *connect.Request[diagv1.CreateConnectorRequest]) (*connect.Response[diagv1.CreateConnectorResponse], error) {
	m.Mu.Lock()
	defer m.Mu.Unlock()
	m.initLocked()
	in := req.Msg
	id := m.allocLocked()
	direction := in.GetDirection()
	if direction == "" {
		direction = "forward"
	}
	style := in.GetStyle()
	if style == "" {
		style = "bezier"
	}
	c := &diagv1.Connector{
		Id:              id,
		ViewId:          in.GetViewId(),
		SourceElementId: in.GetSourceElementId(),
		TargetElementId: in.GetTargetElementId(),
		Label:           in.Label,
		Description:     in.Description,
		Relationship:    in.Relationship,
		Direction:       direction,
		Style:           style,
		Url:             in.Url,
		SourceHandle:    in.SourceHandle,
		TargetHandle:    in.TargetHandle,
		Tags:            in.Tags,
		UpdatedAt:       timestamppb.Now(),
		CreatedAt:       timestamppb.Now(),
	}
	m.connectors[id] = c
	return connect.NewResponse(&diagv1.CreateConnectorResponse{Connector: c}), nil
}

func (m *MockDiagramService) UpdateConnector(_ context.Context, req *connect.Request[diagv1.UpdateConnectorRequest]) (*connect.Response[diagv1.UpdateConnectorResponse], error) {
	if m.UpdateConnectorFunc != nil {
		resp, err := m.UpdateConnectorFunc(req.Msg)
		if err != nil {
			return nil, err
		}
		return connect.NewResponse(resp), nil
	}
	m.Mu.Lock()
	defer m.Mu.Unlock()
	m.initLocked()
	in := req.Msg
	c, ok := m.connectors[in.GetConnectorId()]
	if !ok {
		return nil, connect.NewError(connect.CodeNotFound, fmt.Errorf("connector not found"))
	}
	if in.GetSourceElementId() != 0 {
		c.SourceElementId = in.GetSourceElementId()
	}
	if in.GetTargetElementId() != 0 {
		c.TargetElementId = in.GetTargetElementId()
	}
	if in.Label != nil {
		c.Label = in.Label
	}
	if in.Description != nil {
		c.Description = in.Description
	}
	if in.Relationship != nil {
		c.Relationship = in.Relationship
	}
	if in.GetDirection() != "" {
		c.Direction = in.GetDirection()
	}
	if in.GetStyle() != "" {
		c.Style = in.GetStyle()
	}
	if in.Url != nil {
		c.Url = in.Url
	}
	c.UpdatedAt = timestamppb.Now()
	return connect.NewResponse(&diagv1.UpdateConnectorResponse{Connector: c}), nil
}

func (m *MockDiagramService) DeleteConnector(_ context.Context, req *connect.Request[diagv1.DeleteConnectorRequest]) (*connect.Response[diagv1.DeleteConnectorResponse], error) {
	m.Mu.Lock()
	defer m.Mu.Unlock()
	m.initLocked()
	delete(m.connectors, req.Msg.GetConnectorId())
	return connect.NewResponse(&diagv1.DeleteConnectorResponse{}), nil
}

func (m *MockDiagramService) ListConnectors(_ context.Context, _ *connect.Request[diagv1.ListConnectorsRequest]) (*connect.Response[diagv1.ListConnectorsResponse], error) {
	m.Mu.Lock()
	defer m.Mu.Unlock()
	m.initLocked()
	var out []*diagv1.Connector
	for _, c := range m.connectors {
		out = append(out, c)
	}
	return connect.NewResponse(&diagv1.ListConnectorsResponse{Connectors: out}), nil
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

func NewLocalAPIServer(t *testing.T, dataDir string) string {
	t.Helper()
	sqliteStore, err := store.Open(localserver.DatabasePath(dataDir), assets.FS)
	if err != nil {
		t.Fatalf("open local API database: %v", err)
	}
	t.Cleanup(func() { _ = sqliteStore.Legacy().Close() })

	static := fstest.MapFS{"frontend/dist/index.html": {Data: []byte("<html>app</html>")}}
	srv, err := localapi.New(sqliteStore, static, uuid.MustParse("11111111-1111-1111-1111-111111111111"))
	if err != nil {
		t.Fatalf("create local API server: %v", err)
	}
	httpSrv := httptest.NewServer(srv.Routes())
	t.Cleanup(httpSrv.Close)
	return httpSrv.URL
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
	if err := os.WriteFile(filepath.Join(configDir, "tld.global.yaml"), []byte(cfg), 0600); err != nil {
		t.Fatalf("write tld.global.yaml: %v", err)
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
	t.Setenv("TLD_DATA_DIR", t.TempDir())
	oldTarget, hadTarget := os.LookupEnv("TLD_APPLY_TARGET")
	if err := os.Setenv("TLD_APPLY_TARGET", "local"); err != nil {
		t.Fatalf("set TLD_APPLY_TARGET: %v", err)
	}
	defer func() {
		if hadTarget {
			_ = os.Setenv("TLD_APPLY_TARGET", oldTarget)
			return
		}
		_ = os.Unsetenv("TLD_APPLY_TARGET")
	}()
	// Synchronous commands write to the local DB and refresh the YAML cache
	// (including ID metadata) immediately.
	MustRunCmd(t, dir, "add", "Platform", "--ref", "platform", "--kind", "workspace")
	MustRunCmd(t, dir, "add", "API", "--ref", "api", "--parent", "platform", "--kind", "service")
	MustRunCmd(t, dir, "add", "DB", "--ref", "db", "--parent", "platform", "--kind", "database")
	MustRunCmd(t, dir, "connect", "--from", "api", "--to", "db", "--label", "reads")
}
