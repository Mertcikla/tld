package infra

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"path/filepath"
	"time"

	"github.com/gorilla/websocket"
	_ "modernc.org/sqlite"
)

type TldClient struct {
	BaseURL string
	DataDir string
}

func NewTldClient(port int, dataDir string) *TldClient {
	return &TldClient{
		BaseURL: fmt.Sprintf("http://127.0.0.1:%d", port),
		DataDir: dataDir,
	}
}

type WatchStatus struct {
	Active     bool `json:"active"`
	Repository struct {
		ID int64 `json:"id"`
	} `json:"repository"`
}

func (c *TldClient) GetStatus() (*WatchStatus, error) {
	resp, err := http.Get(c.BaseURL + "/api/watch/status")
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("unexpected status: %d", resp.StatusCode)
	}

	var status WatchStatus
	if err := json.NewDecoder(resp.Body).Decode(&status); err != nil {
		return nil, err
	}
	return &status, nil
}

func (c *TldClient) WaitForStatus(active bool, timeout time.Duration) error {
	start := time.Now()
	for time.Since(start) < timeout {
		status, err := c.GetStatus()
		if err == nil && status.Active == active {
			return nil
		}
		time.Sleep(200 * time.Millisecond)
	}
	return fmt.Errorf("timeout waiting for status active=%v", active)
}

type WatchEvent struct {
	Type string      `json:"type"`
	Data interface{} `json:"data"`
}

func (c *TldClient) ListenEvents(ctx context.Context, eventType string, count int, timeout time.Duration) ([]WatchEvent, error) {
	u, _ := url.Parse(c.BaseURL)
	u.Scheme = "ws"
	u.Path = "/api/watch/ws"

	conn, _, err := websocket.DefaultDialer.Dial(u.String(), nil)
	if err != nil {
		return nil, err
	}
	defer func() { _ = conn.Close() }()

	events := make([]WatchEvent, 0, count)
	deadline := time.Now().Add(timeout)
	
	for len(events) < count {
		if time.Now().After(deadline) {
			return events, fmt.Errorf("timeout waiting for %d events of type %s (got %d)", count, eventType, len(events))
		}
		
		_ = conn.SetReadDeadline(deadline)
		_, message, err := conn.ReadMessage()
		if err != nil {
			return events, err
		}

		var ev WatchEvent
		if err := json.Unmarshal(message, &ev); err != nil {
			continue
		}

		if ev.Type == eventType {
			events = append(events, ev)
		}
		
		select {
		case <-ctx.Done():
			return events, ctx.Err()
		default:
		}
	}

	return events, nil
}

func OpenDB(dbPath string) (*sql.DB, error) {
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		return nil, err
	}
	if _, err := db.Exec(`PRAGMA foreign_keys = ON;`); err != nil {
		_ = db.Close()
		return nil, err
	}
	return db, nil
}

// QueryDB executes a query against the local SQLite store and returns the first result.
func (c *TldClient) QueryDB(query string, args ...interface{}) (int, error) {
	dbPath := filepath.Join(c.DataDir, "tld.db")
	db, err := OpenDB(dbPath)
	if err != nil {
		return 0, err
	}
	defer func() { _ = db.Close() }()

	var count int
	err = db.QueryRow(query, args...).Scan(&count)
	return count, err
}
