package client

import (
	"context"
	"testing"

	"connectrpc.com/connect"
)

func TestNormalizeURL(t *testing.T) {
	cases := []struct {
		in   string
		want string
	}{
		{"https://api.tldiagram.com", "https://api.tldiagram.com/api"},
		{"https://api.tldiagram.com/", "https://api.tldiagram.com/api"},
		{"https://api.tldiagram.com/api", "https://api.tldiagram.com/api"},
		{"https://api.tldiagram.com/api/", "https://api.tldiagram.com/api"},
	}
	for _, tc := range cases {
		got := NormalizeURL(tc.in)
		if got != tc.want {
			t.Errorf("NormalizeURL(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

func TestBearerInterceptor(t *testing.T) {
	apiKey := "test-key"
	interceptor := newBearerInterceptor(apiKey)

	next := func(_ context.Context, req connect.AnyRequest) (connect.AnyResponse, error) {
		auth := req.Header().Get("Authorization")
		if auth != "Bearer "+apiKey {
			t.Errorf("expected Bearer %s, got %s", apiKey, auth)
		}
		return nil, nil
	}

	req := connect.NewRequest(&struct{}{})
	_, _ = interceptor.WrapUnary(next)(context.Background(), req)
}

func TestNew(t *testing.T) {
	c := New("https://example.com", "key", false)
	if c == nil {
		t.Error("expected non-nil client")
	}
}

func TestNewDeviceClient(t *testing.T) {
	c := NewDeviceClient("https://example.com", false)
	if c == nil {
		t.Error("expected non-nil client")
	}
}

func TestDebugInterceptor(t *testing.T) {
	interceptor := newDebugInterceptor()

	next := func(_ context.Context, _ connect.AnyRequest) (connect.AnyResponse, error) {
		return connect.NewResponse(&struct{}{}), nil
	}

	req := connect.NewRequest(&struct{}{})
	_, err := interceptor.WrapUnary(next)(context.Background(), req)
	if err != nil {
		t.Errorf("DebugInterceptor failed: %v", err)
	}
}
