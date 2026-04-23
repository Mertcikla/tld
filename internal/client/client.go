// Package client provides a gRPC/Connect client factory for the tlDiagram API.
package client

import (
	"context"
	"fmt"
	"net/http"
	"strings"

	diagv1connect "buf.build/gen/go/tldiagramcom/diagram/connectrpc/go/diag/v1/diagv1connect"
	"connectrpc.com/connect"
)

// NormalizeURL ensures the server URL has the required '/api' suffix and no trailing slashes.
func NormalizeURL(serverURL string) string {
	baseURL := strings.TrimRight(serverURL, "/")
	if !strings.HasSuffix(baseURL, "/api") {
		baseURL += "/api"
	}
	return baseURL
}

// New creates a WorkspaceServiceClient with bearer token authentication.
func New(serverURL, apiKey string, debug bool) diagv1connect.WorkspaceServiceClient {
	interceptors := []connect.Interceptor{
		newBearerInterceptor(apiKey),
	}
	if debug {
		interceptors = append(interceptors, newDebugInterceptor())
	}

	return diagv1connect.NewWorkspaceServiceClient(
		&http.Client{},
		NormalizeURL(serverURL),
		connect.WithInterceptors(interceptors...),
	)
}

// NewDeviceClient creates a DeviceServiceClient without bearer token authentication.
func NewDeviceClient(serverURL string, debug bool) diagv1connect.DeviceServiceClient {
	interceptors := []connect.Interceptor{}
	if debug {
		interceptors = append(interceptors, newDebugInterceptor())
	}

	return diagv1connect.NewDeviceServiceClient(
		&http.Client{},
		NormalizeURL(serverURL),
		connect.WithInterceptors(interceptors...),
	)
}

type bearerInterceptor struct {
	apiKey string
}

func newBearerInterceptor(apiKey string) connect.Interceptor {
	return &bearerInterceptor{apiKey: apiKey}
}

func (b *bearerInterceptor) WrapUnary(next connect.UnaryFunc) connect.UnaryFunc {
	return func(ctx context.Context, req connect.AnyRequest) (connect.AnyResponse, error) {
		req.Header().Set("Authorization", "Bearer "+b.apiKey)
		return next(ctx, req)
	}
}

func (b *bearerInterceptor) WrapStreamingClient(next connect.StreamingClientFunc) connect.StreamingClientFunc {
	return next
}

func (b *bearerInterceptor) WrapStreamingHandler(next connect.StreamingHandlerFunc) connect.StreamingHandlerFunc {
	return next
}

type debugInterceptor struct{}

func newDebugInterceptor() connect.Interceptor {
	return &debugInterceptor{}
}

func (d *debugInterceptor) WrapUnary(next connect.UnaryFunc) connect.UnaryFunc {
	return func(ctx context.Context, req connect.AnyRequest) (connect.AnyResponse, error) {
		fmt.Printf("--> %s %s\n", req.HTTPMethod(), req.Spec().Procedure)
		for k, v := range req.Header() {
			fmt.Printf("--> %s: %s\n", k, strings.Join(v, ", "))
		}

		resp, err := next(ctx, req)

		if err != nil {
			fmt.Printf("<-- ERROR: %v\n", err)
			return nil, err
		}

		fmt.Printf("<-- %s\n", resp.Header().Get("Content-Type"))
		return resp, nil
	}
}

func (d *debugInterceptor) WrapStreamingClient(next connect.StreamingClientFunc) connect.StreamingClientFunc {
	return next
}

func (d *debugInterceptor) WrapStreamingHandler(next connect.StreamingHandlerFunc) connect.StreamingHandlerFunc {
	return next
}
