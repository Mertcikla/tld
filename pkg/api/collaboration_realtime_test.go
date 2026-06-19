package api

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/websocket"
)

func TestRealtimeWritePumpStopsWhenClientLifecycleEnds(t *testing.T) {
	serverConnCh := make(chan *websocket.Conn, 1)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := realtimeUpgrader.Upgrade(w, r, nil)
		if err != nil {
			t.Errorf("upgrade websocket: %v", err)
			return
		}
		serverConnCh <- conn
	}))
	defer server.Close()

	clientConn, _, err := websocket.DefaultDialer.Dial("ws"+strings.TrimPrefix(server.URL, "http"), nil)
	if err != nil {
		t.Fatalf("dial websocket: %v", err)
	}
	defer func() { _ = clientConn.Close() }()

	var serverConn *websocket.Conn
	select {
	case serverConn = <-serverConnCh:
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for server websocket connection")
	}

	client := &realtimeClient{
		conn:      serverConn,
		send:      make(chan []byte, 1),
		writeDone: make(chan struct{}),
	}
	client.markActive(time.Now())

	stopped := make(chan struct{})
	go func() {
		(&CollaborationRealtimeHandler{}).writePump(client)
		close(stopped)
	}()

	client.stopWritePump()

	select {
	case <-stopped:
	case <-time.After(time.Second):
		t.Fatal("writePump did not stop after client lifecycle ended")
	}
}
