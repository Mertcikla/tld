package api

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strconv"
	"strings"

	pb "buf.build/gen/go/tldiagramcom/diagram/protocolbuffers/go/diag/v1"
	"connectrpc.com/connect"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protoreflect"
	"google.golang.org/protobuf/types/dynamicpb"
)

// PlanMiddleware preserves existing WorkspaceService wire contracts while
// recording workspace writes carrying a plan context.
func (s *PlanService) PlanMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		id := r.Header.Get("X-Tld-Plan")
		if strings.Contains(r.URL.Path, "/diag.v1.PlanService/") {
			next.ServeHTTP(w, r)
			return
		}
		fail := func(err error) {
			code := connect.CodeOf(planErr(err))
			status := http.StatusBadRequest
			if code == connect.CodeAborted {
				status = http.StatusConflict
			}
			if code == connect.CodeNotFound {
				status = http.StatusNotFound
			}
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(status)
			_ = json.NewEncoder(w).Encode(map[string]string{"code": code.String(), "message": err.Error()})
		}
		prefix := "/api/diag.v1.WorkspaceService/"
		if !strings.HasPrefix(r.URL.Path, prefix) {
			next.ServeHTTP(w, r)
			return
		}

		method := strings.TrimPrefix(r.URL.Path, prefix)
		descriptor := pb.File_diag_v1_workspace_service_proto.Services().ByName("WorkspaceService").Methods().ByName(protoreflect.Name(method))
		if descriptor == nil {
			fail(invalidArg("method", "unsupported plan operation"))
			return
		}
		body, err := io.ReadAll(io.LimitReader(r.Body, 8<<20))
		if err != nil {
			fail(err)
			return
		}
		input := dynamicpb.NewMessage(descriptor.Input())
		binary := strings.Contains(r.Header.Get("Content-Type"), "application/proto")
		if binary {
			err = proto.Unmarshal(body, input)
		} else {
			err = protojson.Unmarshal(body, input)
		}
		if err != nil {
			fail(err)
			return
		}
		data, _ := protojson.MarshalOptions{UseProtoNames: true}.Marshal(input)
		fields := map[string]any{}
		if err = json.Unmarshal(data, &fields); err != nil {
			fail(err)
			return
		}
		writeContext := &planWriteContext{requestID: r.Header.Get("X-Tld-Request-Id")}
		if raw := r.Header.Get("X-Tld-Plan-Revision"); raw != "" {
			rev, e := strconv.ParseUint(raw, 10, 32)
			if e != nil {
				fail(e)
				return
			}
			n := uint32(rev)
			writeContext.expected = &n
		}
		ctx := context.WithValue(r.Context(), planWriteContextKey{}, writeContext)
		result, handled, err := s.lockedWorkspace(ctx, method, fields)
		if err == nil && !handled {
			if id != "" {
				result, err = s.PlanWorkspace(ctx, id, method, fields)
			} else {
				r.Body = io.NopCloser(strings.NewReader(string(body)))
				next.ServeHTTP(w, r)
				return
			}
		}
		if err != nil {
			fail(err)
			return
		}
		w.Header().Set("X-Tld-Plan-Revision", strconv.FormatUint(uint64(writeContext.observed), 10))
		data, err = json.Marshal(result)
		if err != nil {
			fail(err)
			return
		}
		output := dynamicpb.NewMessage(descriptor.Output())
		if err = (protojson.UnmarshalOptions{DiscardUnknown: true}).Unmarshal(data, output); err != nil {
			fail(err)
			return
		}
		if binary {
			data, err = proto.Marshal(output)
			w.Header().Set("Content-Type", "application/proto")
		} else {
			data, err = protojson.Marshal(output)
			w.Header().Set("Content-Type", "application/json")
		}
		if err != nil {
			fail(err)
			return
		}
		_, _ = w.Write(data)
	})
}

type planWriteContextKey struct{}
type planWriteContext struct {
	expected  *uint32
	observed  uint32
	requestID string
}
