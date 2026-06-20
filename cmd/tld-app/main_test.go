package main

import (
	"net"
	"testing"
)

func TestListenDesktopLocalServerUsesAvailableRequestedAddr(t *testing.T) {
	listener, addr, err := listenDesktopLocalServer("127.0.0.1:0")
	if err != nil {
		t.Fatalf("listenDesktopLocalServer: %v", err)
	}
	defer func() { _ = listener.Close() }()

	host, port, err := net.SplitHostPort(addr)
	if err != nil {
		t.Fatalf("split client addr %q: %v", addr, err)
	}
	if host != "127.0.0.1" {
		t.Fatalf("client addr host = %q, want 127.0.0.1", host)
	}
	if port == "" || port == "0" {
		t.Fatalf("client addr port = %q, want assigned port", port)
	}
}

func TestListenDesktopLocalServerFallsBackWhenRequestedAddrIsBusy(t *testing.T) {
	busy, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("reserve busy addr: %v", err)
	}
	defer func() { _ = busy.Close() }()

	listener, addr, err := listenDesktopLocalServer(busy.Addr().String())
	if err != nil {
		t.Fatalf("listenDesktopLocalServer: %v", err)
	}
	defer func() { _ = listener.Close() }()

	if addr == busy.Addr().String() {
		t.Fatalf("client addr = busy addr %q, expected fallback addr", addr)
	}
	host, port, err := net.SplitHostPort(addr)
	if err != nil {
		t.Fatalf("split fallback client addr %q: %v", addr, err)
	}
	if host != "127.0.0.1" {
		t.Fatalf("fallback client addr host = %q, want 127.0.0.1", host)
	}
	if port == "" || port == "0" {
		t.Fatalf("fallback client addr port = %q, want assigned port", port)
	}
}

func TestClientAddrForListenerNormalizesUnspecifiedHost(t *testing.T) {
	listener, err := net.Listen("tcp", "0.0.0.0:0")
	if err != nil {
		t.Fatalf("listen on unspecified host: %v", err)
	}
	defer func() { _ = listener.Close() }()

	addr := clientAddrForListener(listener)
	host, _, err := net.SplitHostPort(addr)
	if err != nil {
		t.Fatalf("split client addr %q: %v", addr, err)
	}
	if host != "127.0.0.1" {
		t.Fatalf("client addr host = %q, want 127.0.0.1", host)
	}
}
