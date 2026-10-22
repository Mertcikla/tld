package api

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"sort"
	"strconv"
	"strings"

	pb "buf.build/gen/go/tldiagramcom/diagram/protocolbuffers/go/diag/v1"
	"connectrpc.com/connect"
	"github.com/google/uuid"
	"github.com/mertcikla/tld/v2/pkg/app"
	"github.com/mertcikla/tld/v2/pkg/plans"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"
)

type agentEdit struct {
	Public        *pb.AgentWorkspaceEdit
	Input         map[string]any
	Response      map[string]any
	RequestID     string
	RequestHash   string
	Preconditions map[string]string
	// Stable virtual identities returned to agents, mapped to live IDs on commit.
	Creates map[string]int32
	Applied map[string]int32
}
type implementation struct {
	PlanID        string
	RepositoryID  string
	Revision      uint32
	Locked        bool
	GitRevision   string
	PendingCommit string
	NextID        int32
	Edits         []*agentEdit
}

func readImplementation(ctx context.Context, st *app.Store) (*implementation, error) {
	raw, err := st.Implementation(ctx, planScope(ctx))
	if err != nil {
		return nil, err
	}
	if raw == "" {
		return &implementation{NextID: 1000000000}, nil
	}
	out := &implementation{}
	err = json.Unmarshal([]byte(raw), out)
	return out, err
}
func saveImplementation(ctx context.Context, st *app.Store, v *implementation) error {
	expected := v.Revision
	v.Revision++
	raw, err := json.Marshal(v)
	if err != nil {
		return err
	}
	return st.SaveImplementation(ctx, planScope(ctx), v.PlanID, string(raw), v.Revision, expected)
}
func implementationResponse(v *implementation) *connect.Response[pb.ImplementationResponse] {
	out := &pb.ImplementationResponse{PlanId: v.PlanID, RepositoryId: v.RepositoryID, Revision: v.Revision, Locked: v.Locked, GitRevision: v.GitRevision, PendingCommit: v.PendingCommit}
	ids := map[string]int32{}
	for _, e := range v.Edits {
		if e.Public.Status == "committed" {
			for k, id := range e.Applied {
				ids[k] = id
			}
		}
	}
	for _, e := range v.Edits {
		item := proto.Clone(e.Public).(*pb.AgentWorkspaceEdit)
		for _, op := range item.Operations {
			remapAgentOperation(op, ids)
		}
		out.Edits = append(out.Edits, item)
	}
	return connect.NewResponse(out)
}
func remapAgentOperation(op *pb.PlanOperation, ids map[string]int32) {
	remapResource := func(r *pb.PlanResource) {
		if r == nil {
			return
		}
		g := remapGraph(plans.Graph{r.Key: r}, ids)
		for _, mapped := range g {
			proto.Reset(r)
			proto.Merge(r, mapped)
		}
	}
	remapKey := func(key string) string {
		kind, _ := splitResourceKey(key)
		if id, ok := ids[key]; ok {
			return plans.Key(kind, id)
		}
		return key
	}
	remapResource(op.GetCreate())
	if op.GetUpdate() != nil {
		remapResource(op.GetUpdate().Resource)
	}
	if op.GetRemove() != nil {
		op.GetRemove().Key = remapKey(op.GetRemove().Key)
	}
	for _, pre := range op.Preconditions {
		pre.ResourceKey = remapKey(pre.ResourceKey)
		remapResource(pre.Before)
	}
}
func pendingEdit(e *agentEdit) bool {
	return e.Public.Status != "committed" && e.Public.Status != "skipped" && e.Public.Status != "discarded"
}
func hasPending(v *implementation) bool {
	for _, e := range v.Edits {
		if pendingEdit(e) {
			return true
		}
	}
	return false
}
func (s *PlanService) LockImplementation(ctx context.Context, req *connect.Request[pb.LockImplementationRequest]) (*connect.Response[pb.ImplementationResponse], error) {
	if err := s.access(ctx, true); err != nil {
		return nil, err
	}
	if s.RepositoryRevision != nil {
		p, err := s.Store.Plan(ctx, planScope(ctx), req.Msg.PlanId)
		if err != nil {
			return nil, planErr(err)
		}
		revision, err := s.RepositoryRevision(ctx, p.RepositoryId)
		if err != nil {
			return nil, planErr(err)
		}
		req.Msg.GitRevision = revision
	}
	var v *implementation
	err := s.Store.RunPlanTransaction(ctx, func(st *app.Store) error {
		var err error
		v, err = readImplementation(ctx, st)
		if err != nil {
			return err
		}
		p, err := st.Plan(ctx, planScope(ctx), req.Msg.PlanId)
		if err != nil {
			return err
		}
		if v.Locked {
			if v.PlanID == p.Id {
				return nil
			}
			return fmt.Errorf("another plan has locked this workspace")
		}
		if p.Revision != req.Msg.ExpectedRevision {
			return app.ErrPlanConflict
		}
		if p.Status == "archived" || p.Status == "completed" {
			return fmt.Errorf("archived plan cannot be implemented")
		}
		if v.PlanID != p.Id {
			v.Edits = nil
		}
		// Starting implementation creates one immutable, ready specification.
		old := p.Revision
		p.Revision++
		p.ReadyRevision = p.Revision
		p.Status = "ready"
		p.UpdatedAt = nowPlan()
		if err = st.SavePlan(ctx, planScope(ctx), p, old); err != nil {
			return err
		}
		if err = s.freezeHandoff(ctx, st, p, req.Msg.GitRevision); err != nil {
			return err
		}
		v.PlanID = p.Id
		v.RepositoryID = p.RepositoryId
		v.Locked = true
		v.GitRevision = req.Msg.GitRevision
		v.PendingCommit = ""
		return saveImplementation(ctx, st, v)
	})
	if err != nil {
		return nil, planErr(err)
	}
	return implementationResponse(v), nil
}
func (s *PlanService) GetImplementation(ctx context.Context, req *connect.Request[pb.GetImplementationRequest]) (*connect.Response[pb.ImplementationResponse], error) {
	if err := s.access(ctx, false); err != nil {
		return nil, err
	}
	current, err := readImplementation(ctx, s.Store)
	if err != nil {
		return nil, planErr(err)
	}
	if s.RepositoryRevision != nil && current.Locked && (req.Msg.RepositoryId == "" || current.RepositoryID == req.Msg.RepositoryId) {
		revision, err := s.RepositoryRevision(ctx, current.RepositoryID)
		// Repository access can disappear while a session is locked. Recovery must
		// remain available; Git detection is supplementary to the stored ledger.
		if err == nil {
			req.Msg.GitRevision = revision
		}
	}
	var v *implementation
	err = s.Store.RunPlanTransaction(ctx, func(st *app.Store) error {
		var err error
		v, err = readImplementation(ctx, st)
		if err != nil {
			return err
		}
		if req.Msg.RepositoryId != "" && v.RepositoryID != req.Msg.RepositoryId {
			v = &implementation{}
			return nil
		}
		if rev := req.Msg.GitRevision; v.Locked && rev != "" && rev != v.GitRevision && rev != v.PendingCommit {
			if hasPending(v) {
				v.PendingCommit = rev
			} else {
				v.GitRevision = rev
			}
			return saveImplementation(ctx, st, v)
		}
		return nil
	})
	if err != nil {
		return nil, planErr(err)
	}
	return implementationResponse(v), nil
}

// The same interception is used by local CLI, MCP and HTTP workspace handlers.
// No caller-supplied plan flag is needed. A workspace-wide lock is intentional:
// shared/unassigned elements cannot safely be attributed to only one repository.
func (s *PlanService) lockedWorkspace(ctx context.Context, method string, in map[string]any) (map[string]any, bool, error) {
	var out map[string]any
	handled := false
	err := s.Store.RunPlanTransaction(ctx, func(st *app.Store) error {
		v, err := readImplementation(ctx, st)
		if err != nil {
			return err
		}
		if !v.Locked {
			return nil
		}
		handled = true
		read := strings.HasPrefix(method, "Get") || strings.HasPrefix(method, "List") || method == "ExportWorkspace"
		if err = s.access(ctx, !read); err != nil {
			return err
		}
		if !safeAgentMethod(method) {
			return fmt.Errorf("%s is unavailable while the workspace is locked; no live changes were made", method)
		}
		requestID := ""
		if wc, ok := ctx.Value(planWriteContextKey{}).(*planWriteContext); ok {
			requestID = wc.requestID
		}
		raw, _ := json.Marshal(in)
		hash := fmt.Sprintf("%x", sha256.Sum256(append([]byte(method), raw...)))
		if requestID != "" {
			for _, e := range v.Edits {
				if e.RequestID == requestID {
					if e.RequestHash != hash {
						return fmt.Errorf("request identity reused with different input")
					}
					out = e.Response
					return nil
				}
			}
		}
		simulatedCtx, _ := s.commitHooks(ctx) // Never flush simulation notifications.
		var entry *agentEdit
		err = st.SimulateWorkspace(ctx, func() error {
			ids := map[string]int32{}
			for _, e := range v.Edits {
				if e.Public.Status == "committed" {
					for k, id := range e.Applied {
						ids[k] = id
					}
					continue
				}
				if !pendingEdit(e) {
					continue
				}
				prior, eerr := s.baseline(ctx, st)
				if eerr != nil {
					return eerr
				}
				response, eerr := s.liveWorkspace(simulatedCtx, st, e.Public.Method, remapInput(e.Public.Method, e.Input, ids))
				if eerr != nil {
					return fmt.Errorf("pending %s cannot be projected: %w; review or skip it in Plans", e.Public.Method, eerr)
				}
				learnIDs(e.Response, response, ids)
				after, eerr := s.baseline(ctx, st)
				if eerr != nil {
					return eerr
				}
				learnCreated(e, prior, after, ids)
			}
			before, eerr := s.baseline(ctx, st)
			if eerr != nil {
				return eerr
			}
			out, eerr = s.liveWorkspace(simulatedCtx, st, method, remapInput(method, in, ids))
			if eerr != nil {
				return eerr
			}
			if read {
				out = remapOutput(out, invertIDs(ids))
				return nil
			}
			after, eerr := s.baseline(ctx, st)
			if eerr != nil {
				return eerr
			}
			ops := plans.Diff(before, after)
			if len(ops) == 0 {
				out = remapOutput(out, invertIDs(ids))
				return nil
			}
			// Allocate positive plan-local IDs, independent of changing live sequences.
			reverse := invertIDs(ids)
			creates := map[string]int32{}
			for _, op := range ops {
				if r := op.GetCreate(); r != nil {
					v.NextID++
					k := plans.Key(plans.Kind(r), plans.ID(r))
					reverse[k] = v.NextID
					creates[plans.Key(plans.Kind(r), v.NextID)] = plans.ID(r)
				}
			}
			pre := map[string]string{}
			for _, op := range ops {
				key := plans.Target(op)
				pre[key] = plans.Hash(before[key])
				r := after[key]
				if r == nil {
					r = before[key]
				}
				if r != nil {
					for _, k := range plans.Dependencies(before, r) {
						if before[k] != nil {
							pre[k] = plans.Hash(before[k])
						}
					}
				}
			}
			// Preconditions and diffs are expressed in stable virtual identities.
			stableBefore := remapGraph(before, reverse)
			stableAfter := remapGraph(after, reverse)
			stablePre := map[string]string{}
			for k := range pre {
				kind, id := splitResourceKey(k)
				if mapped, ok := reverse[k]; ok {
					id = mapped
				}
				key := plans.Key(kind, id)
				stablePre[key] = plans.Hash(stableBefore[key])
			}
			out = remapOutput(out, reverse)
			entry = &agentEdit{Public: &pb.AgentWorkspaceEdit{Id: uuid.NewString(), Method: method, Status: "pending", Operations: plans.Diff(stableBefore, stableAfter)}, Input: in, Response: out, RequestID: requestID, RequestHash: hash, Preconditions: stablePre, Creates: creates}
			if entry != nil {
				for _, op := range entry.Public.Operations {
					op.Id = uuid.NewString()
					op.AuthorKind = "agent"
					op.GestureId = entry.Public.Id
				}
			}
			return nil
		})
		if err != nil {
			return err
		}
		if entry != nil {
			v.Edits = append(v.Edits, entry)
			return saveImplementation(ctx, st, v)
		}
		return nil
	})
	return out, handled, planErr(err)
}
func safeAgentMethod(method string) bool {
	switch method {
	case "CreateElement", "UpdateElement", "DeleteElement", "CreateView", "UpdateView", "DeleteView", "CreateConnector", "UpdateConnector", "DeleteConnector", "CreatePlacement", "UpdatePlacement", "UpdatePlacementPosition", "DeletePlacement", "CreateViewLayer", "UpdateViewLayer", "DeleteViewLayer", "ApplyWorkspaceBatch", "ApplyWorkspacePlan", "ExportWorkspace", "GetElement", "ListElements", "GetView", "ListViews", "GetConnector", "ListConnectors", "ListPlacements", "GetExploreData", "ListViewLayers", "GetViewLayer", "GetViewCanvas", "GetViewContent":
		return true
	}
	// Reads do not touch managed note files; they can safely use normal facilities.
	return strings.HasPrefix(method, "Get") || strings.HasPrefix(method, "List")
}
func splitResourceKey(k string) (string, int32) {
	kind, raw, _ := strings.Cut(k, ":")
	id, _ := strconv.ParseInt(raw, 10, 32)
	return kind, int32(id)
}
func invertIDs(ids map[string]int32) map[string]int32 {
	out := map[string]int32{}
	for k, id := range ids {
		kind, virtual := splitResourceKey(k)
		out[plans.Key(kind, id)] = virtual
	}
	return out
}
func resourceKind(field string) string {
	switch field {
	case "created_elements", "element_metadata", "element_results", "elements", "element", "source_element_id", "target_element_id", "owner_element_id", "element_id":
		return "element"
	case "created_views", "view_metadata", "views", "view", "view_id", "parent_view_id", "children":
		return "view"
	case "created_connectors", "connector_metadata", "connector_results", "connectors", "connector", "connector_id":
		return "connector"
	case "created_placements", "placements", "placement", "placement_id":
		return "elementplacement"
	case "layers", "layer", "layer_id":
		return "viewlayer"
	}
	return ""
}
func methodKind(method string) string {
	for _, p := range []string{"Create", "Update", "Delete", "Get"} {
		method = strings.TrimPrefix(method, p)
	}
	switch method {
	case "Element":
		return "element"
	case "View":
		return "view"
	case "Connector":
		return "connector"
	case "Placement":
		return "elementplacement"
	case "ViewLayer":
		return "viewlayer"
	}
	return ""
}
func remapValue(v any, kind string, ids map[string]int32) any {
	switch x := v.(type) {
	case map[string]any:
		out := map[string]any{}
		for k, val := range x {
			rk := resourceKind(k)
			if k == "id" {
				rk = kind
			}
			if rk != "" {
				if n, ok := val.(float64); ok {
					if id, exists := ids[plans.Key(rk, int32(n))]; exists {
						out[k] = float64(id)
						continue
					}
				}
			}
			child := rk
			if child == "" {
				child = kind
			}
			out[k] = remapValue(val, child, ids)
		}
		return out
	case []any:
		out := make([]any, len(x))
		for i, item := range x {
			out[i] = remapValue(item, kind, ids)
		}
		return out
	}
	return v
}
func remapInput(method string, in map[string]any, ids map[string]int32) map[string]any {
	return remapValue(in, methodKind(method), ids).(map[string]any)
}
func remapOutput(in map[string]any, ids map[string]int32) map[string]any {
	return remapValue(in, "", ids).(map[string]any)
}
func learnIDs(original, current map[string]any, ids map[string]int32) {
	var walk func(any, any, string)
	walk = func(a, b any, kind string) {
		switch x := a.(type) {
		case map[string]any:
			y, ok := b.(map[string]any)
			if !ok {
				return
			}
			if n, ok := x["id"].(float64); ok && kind != "" {
				if actual, ok := y["id"].(float64); ok && n >= 1000000000 {
					ids[plans.Key(kind, int32(n))] = int32(actual)
				}
			}
			for k, v := range x {
				child := resourceKind(k)
				if child == "" {
					child = kind
				}
				walk(v, y[k], child)
			}
		case []any:
			y, ok := b.([]any)
			if !ok {
				return
			}
			for i, v := range x {
				if i < len(y) {
					walk(v, y[i], kind)
				}
			}
		}
	}
	walk(original, current, "")
}
func remapGraph(g plans.Graph, ids map[string]int32) plans.Graph {
	out := plans.Graph{}
	for _, r := range g {
		c := proto.Clone(r).(*pb.PlanResource)
		m := plans.Value(c)
		fields := remapValue(planJSON(m), plans.Kind(c), ids).(map[string]any)
		data, _ := json.Marshal(fields)
		_ = protojson.Unmarshal(data, m)
		c.Key = plans.Key(plans.Kind(c), plans.ID(c))
		out[c.Key] = c
	}
	return out
}

func (e *agentEdit) MarshalJSON() ([]byte, error) {
	type alias agentEdit
	data, err := protojson.Marshal(e.Public)
	if err != nil {
		return nil, err
	}
	return json.Marshal(struct {
		*alias
		Public json.RawMessage
	}{(*alias)(e), data})
}
func (e *agentEdit) UnmarshalJSON(data []byte) error {
	type alias agentEdit
	v := struct {
		*alias
		Public json.RawMessage
	}{alias: (*alias)(e)}
	if err := json.Unmarshal(data, &v); err != nil {
		return err
	}
	e.Public = &pb.AgentWorkspaceEdit{}
	return protojson.Unmarshal(v.Public, e.Public)
}
func learnCreated(e *agentEdit, before, after plans.Graph, ids map[string]int32) {
	originals := map[string][]string{}
	actual := map[string][]int32{}
	for k := range e.Creates {
		kind, _ := splitResourceKey(k)
		originals[kind] = append(originals[kind], k)
	}
	for k, r := range after {
		if before[k] == nil {
			kind := plans.Kind(r)
			actual[kind] = append(actual[kind], plans.ID(r))
		}
	}
	for kind, keys := range originals {
		sort.Slice(keys, func(i, j int) bool { return e.Creates[keys[i]] < e.Creates[keys[j]] })
		values := actual[kind]
		sort.Slice(values, func(i, j int) bool { return values[i] < values[j] })
		for i, k := range keys {
			if i < len(values) {
				ids[k] = values[i]
			}
		}
	}
}
func drift(e *agentEdit, g plans.Graph, ids map[string]int32) (string, string) {
	stable := remapGraph(g, invertIDs(ids))
	keys := make([]string, 0, len(e.Preconditions))
	for k := range e.Preconditions {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	var observed, changed []string
	for _, k := range keys {
		hash := plans.Hash(stable[k])
		observed = append(observed, k+"="+hash)
		if hash != e.Preconditions[k] {
			label := k
			if stable[k] != nil {
				if name := plans.Label(stable[k]); name != "" {
					label = name
				}
			}
			changed = append(changed, label)
		}
	}
	sum := sha256.Sum256([]byte(strings.Join(observed, "\n")))
	if len(changed) == 0 {
		return "", fmt.Sprintf("%x", sum)
	}
	return "Affected workspace objects changed: " + strings.Join(changed, ", "), fmt.Sprintf("%x", sum)
}

// Each queued RPC commits separately. A warning/error stops here without rolling
// back earlier successes. Ignore acknowledges exactly the observed primitive hashes.
func (s *PlanService) ReviewImplementation(ctx context.Context, req *connect.Request[pb.ReviewImplementationRequest]) (*connect.Response[pb.ImplementationResponse], error) {
	if err := s.access(ctx, true); err != nil {
		return nil, err
	}
	if s.RepositoryRevision != nil {
		current, err := s.GetImplementation(ctx, connect.NewRequest(&pb.GetImplementationRequest{RepositoryId: req.Msg.RepositoryId}))
		if err != nil {
			return nil, err
		}
		if current.Msg.Revision != req.Msg.ExpectedRevision {
			return nil, planErr(app.ErrPlanConflict)
		}
	}
	expected := req.Msg.ExpectedRevision
	action := req.Msg.Action
	if action != "commit" && action != "ignore" && action != "skip" && action != "discard" && action != "unlock" && action != "finish" && action != "cancel" {
		return nil, invalidArg("action", "invalid review action")
	}
	var v *implementation
	for {
		stop := false
		callCtx, notifications := s.commitHooks(ctx)
		err := s.Store.RunPlanTransaction(callCtx, func(st *app.Store) error {
			var err error
			v, err = readImplementation(callCtx, st)
			if err != nil {
				return err
			}
			if !v.Locked || v.RepositoryID != req.Msg.RepositoryId {
				return fmt.Errorf("implementation is not locked for this repository")
			}
			if v.Revision != expected {
				return app.ErrPlanConflict
			}
			if action == "discard" || action == "cancel" {
				for _, e := range v.Edits {
					if pendingEdit(e) {
						e.Public.Status = "discarded"
					}
				}
				if v.PendingCommit != "" {
					v.GitRevision = v.PendingCommit
					v.PendingCommit = ""
				}
				if action == "cancel" {
					v.Locked = false
					if err = s.endImplementation(callCtx, st, v, "draft"); err != nil {
						return err
					}
				}
				stop = true
				return saveImplementation(callCtx, st, v)
			}
			if action == "unlock" {
				if hasPending(v) {
					return fmt.Errorf("commit or discard pending edits before unlocking")
				}
				v.Locked = false
				stop = true
				return saveImplementation(callCtx, st, v)
			}
			ids := map[string]int32{}
			var next *agentEdit
			for _, e := range v.Edits {
				if e.Public.Status == "committed" {
					for k, id := range e.Applied {
						ids[k] = id
					}
				}
				if next == nil && pendingEdit(e) {
					next = e
				}
			}
			if next == nil {
				if req.Msg.Action == "finish" {
					v.Locked = false
					if err = s.endImplementation(callCtx, st, v, "completed"); err != nil {
						return err
					}
					if v.PendingCommit != "" {
						v.GitRevision = v.PendingCommit
						v.PendingCommit = ""
					}
					stop = true
					return saveImplementation(callCtx, st, v)
				}
				if v.PendingCommit != "" {
					v.GitRevision = v.PendingCommit
					v.PendingCommit = ""
					if err = saveImplementation(callCtx, st, v); err != nil {
						return err
					}
				}
				stop = true
				return nil
			}
			if action == "skip" {
				if next.Public.Id != req.Msg.OperationId {
					return app.ErrPlanConflict
				}
				next.Public.Status = "skipped"
				next.Public.Reason = "Skipped by reviewer"
				stop = true
				return saveImplementation(callCtx, st, v)
			}
			before, err := s.baseline(callCtx, st)
			if err != nil {
				return err
			}
			reason, hash := drift(next, before, ids)
			constraint, err := s.scopeWarning(callCtx, st, v, next)
			if err != nil {
				return err
			}
			if constraint != "" {
				if reason != "" {
					reason += "; "
				}
				reason += constraint
				hash = fmt.Sprintf("%x", sha256.Sum256([]byte(hash+constraint)))
			}
			if reason != "" && (action != "ignore" || next.Public.Id != req.Msg.OperationId || hash != req.Msg.ObservedHash) {
				next.Public.Status = "warning"
				next.Public.Reason = reason
				next.Public.ObservedHash = hash
				stop = true
				return saveImplementation(callCtx, st, v)
			}
			// A savepoint isolates failed handlers; successful earlier edits stay committed.
			var response map[string]any
			var after plans.Graph
			applyCtx, applyHooks := s.commitHooks(callCtx)
			err = st.ApplyImplementationStep(callCtx, func() error {
				var e error
				response, e = s.liveWorkspace(applyCtx, st, next.Public.Method, remapInput(next.Public.Method, next.Input, ids))
				if e != nil {
					return e
				}
				after, e = s.baseline(callCtx, st)
				return e
			})
			if err != nil {
				next.Public.Status = "error"
				next.Public.Reason = err.Error()
				stop = true
				return saveImplementation(callCtx, st, v)
			}
			// Forward deferred notifications only after both this step and its ledger commit.
			notifications.pending = append(notifications.pending, applyHooks.pending...)
			learnIDs(next.Response, response, ids)
			learnCreated(next, before, after, ids)
			next.Applied = ids
			next.Public.Status = "committed"
			next.Public.Reason = ""
			next.Public.ObservedHash = ""
			return saveImplementation(callCtx, st, v)
		})
		if err != nil {
			return nil, planErr(err)
		}
		notifications.flush()
		if stop {
			return implementationResponse(v), nil
		}
		expected = v.Revision
		action = "commit"
	}
}

// Ending a session advances plan metadata; pinned handoffs remain readable.
func (s *PlanService) endImplementation(ctx context.Context, st *app.Store, v *implementation, status string) error {
	p, err := st.Plan(ctx, planScope(ctx), v.PlanID)
	if err != nil {
		return err
	}
	old := p.Revision
	p.Revision++
	p.Status = status
	p.UpdatedAt = nowPlan()
	if err = st.SavePlan(ctx, planScope(ctx), p, old); err != nil {
		return err
	}
	if status == "completed" {
		// A completed plan continues to show its reviewed target architecture,
		// including notes, when opened without an explicit historical revision.
		raw, err := st.PlanHandoff(ctx, p.Id, p.ReadyRevision)
		if err != nil {
			return err
		}
		if raw != "" {
			return st.SavePlanHandoff(ctx, p.Id, p.Revision, raw)
		}
	}
	return nil
}

// Scope roles are review constraints. A reviewer must acknowledge changes to
// preserved/reference objects or to existing objects outside the specification.
func (s *PlanService) scopeWarning(ctx context.Context, st *app.Store, v *implementation, edit *agentEdit) (string, error) {
	p, err := st.Plan(ctx, planScope(ctx), v.PlanID)
	if err != nil {
		return "", err
	}
	h, base, err := readHandoff(ctx, st, p)
	if err != nil || h == nil {
		return "", err
	} // Legacy sessions have no frozen scope.
	ops, err := st.PlanOperations(ctx, p.Id, p.ReadyRevision)
	if err != nil {
		return "", err
	}
	state := plans.Recorded(base, ops)
	reasons := []string{}
	for _, op := range edit.Public.Operations {
		key := plans.Target(op)
		// A creation is new implementation detail; dependent placements are auxiliary.
		if op.GetCreate() != nil || strings.HasPrefix(key, "elementplacement:") {
			continue
		}
		member := state.Members[key]
		if member != nil && (member.Role == "preserve" || member.Role == "context") {
			reasons = append(reasons, plans.Describe(base, base[key])+" is marked "+member.Role)
		} else if member == nil && base[key] != nil {
			reasons = append(reasons, plans.Describe(base, base[key])+" is outside the planned scope")
		}
	}
	return strings.Join(reasons, "; "), nil
}
