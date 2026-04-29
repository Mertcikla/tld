package watch

import (
	"bufio"
	"context"
	"crypto/sha1"
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"errors"
	"io"
	"net"
	"net/http"
	"strings"
	"time"
)

func (h *Handler) watchWebSocket(w http.ResponseWriter, r *http.Request) {
	if !strings.EqualFold(r.Header.Get("Upgrade"), "websocket") {
		writeError(w, http.StatusBadRequest, "websocket upgrade required")
		return
	}
	conn, rw, err := upgradeWebSocket(w, r)
	if err != nil {
		return
	}
	defer conn.Close()

	ctx, cancel := context.WithCancel(r.Context())
	defer cancel()
	controlEvents := make(chan Event, 4)
	go h.watchWebSocketReads(ctx, rw, controlEvents, cancel)

	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()
	var lastRepresentationHash string
	var lastVersionID int64
	for {
		lock, live, err := h.Store.ActiveLiveLock(ctx, LockHeartbeatTimeout)
		if err != nil {
			_ = writeWebSocketJSON(rw, Event{Type: "watch.error", At: nowString(), Message: err.Error()})
			return
		}
		if live && lock.Status == "stopping" {
			_ = writeWebSocketJSON(rw, Event{Type: "lock.disabled", RepositoryID: lock.RepositoryID, At: nowString(), Data: lock})
			_ = writeWebSocketJSON(rw, Event{Type: "watch.stopped", RepositoryID: lock.RepositoryID, At: nowString(), Data: lock})
			return
		}
		eventType := "watch.stopped"
		if live {
			eventType = "watch.heartbeat"
		}
		if err := writeWebSocketJSON(rw, Event{Type: eventType, RepositoryID: lock.RepositoryID, At: nowString(), Data: lock}); err != nil {
			return
		}
		if live {
			summary, err := h.Store.RepresentationSummary(ctx, lock.RepositoryID)
			if err == nil && summary.RepresentationHash != "" && summary.RepresentationHash != lastRepresentationHash {
				lastRepresentationHash = summary.RepresentationHash
				if err := writeWebSocketJSON(rw, Event{Type: "representation.updated", RepositoryID: lock.RepositoryID, At: nowString(), Data: summary}); err != nil {
					return
				}
			}
			version, found, err := h.Store.LatestWatchVersion(ctx, lock.RepositoryID)
			if err == nil && found && version.ID != lastVersionID {
				lastVersionID = version.ID
				if err := writeWebSocketJSON(rw, Event{Type: "version.created", RepositoryID: lock.RepositoryID, At: nowString(), Data: version}); err != nil {
					return
				}
			}
		}
		select {
		case <-ctx.Done():
			return
		case event := <-controlEvents:
			for {
				if err := writeWebSocketJSON(rw, event); err != nil {
					return
				}
				if event.Type == "watch.stopped" {
					return
				}
				select {
				case event = <-controlEvents:
				default:
					goto next
				}
			}
		case <-ticker.C:
		}
	next:
	}
}

func (h *Handler) watchWebSocketReads(ctx context.Context, reader io.Reader, controlEvents chan<- Event, cancel context.CancelFunc) {
	defer cancel()
	for {
		msg, err := readWebSocketMessage(reader)
		if err != nil {
			return
		}
		var body struct {
			Type         string `json:"type"`
			RepositoryID int64  `json:"repository_id"`
			RemoteURL    string `json:"remote_url"`
		}
		if err := json.Unmarshal(msg, &body); err != nil {
			continue
		}
		switch body.Type {
		case "watch.stop":
			if body.RepositoryID > 0 {
				_ = h.Store.RequestStop(ctx, body.RepositoryID)
				emitControlEvent(controlEvents, Event{Type: "lock.disabled", RepositoryID: body.RepositoryID, At: nowString()})
				emitControlEvent(controlEvents, Event{Type: "watch.stopped", RepositoryID: body.RepositoryID, At: nowString()})
			} else {
				lock, live, _ := h.Store.ActiveLiveLock(ctx, LockHeartbeatTimeout)
				_ = h.Store.RequestStopActive(ctx)
				if live {
					emitControlEvent(controlEvents, Event{Type: "lock.disabled", RepositoryID: lock.RepositoryID, At: nowString(), Data: lock})
					emitControlEvent(controlEvents, Event{Type: "watch.stopped", RepositoryID: lock.RepositoryID, At: nowString(), Data: lock})
				} else {
					emitControlEvent(controlEvents, Event{Type: "watch.stopped", At: nowString()})
				}
			}
		case "watch.reassociateRepo":
			if body.RepositoryID > 0 && strings.TrimSpace(body.RemoteURL) != "" {
				_, _ = h.Store.ReassociateRepository(ctx, body.RepositoryID, body.RemoteURL)
			}
		case "watch.status", "watch.rescan":
		}
	}
}

func emitControlEvent(ch chan<- Event, event Event) {
	select {
	case ch <- event:
	default:
	}
}

func upgradeWebSocket(w http.ResponseWriter, r *http.Request) (net.Conn, *bufio.ReadWriter, error) {
	hj, ok := w.(http.Hijacker)
	if !ok {
		http.Error(w, "websocket unsupported", http.StatusInternalServerError)
		return nil, nil, errors.New("hijack unsupported")
	}
	key := strings.TrimSpace(r.Header.Get("Sec-WebSocket-Key"))
	if key == "" {
		http.Error(w, "missing Sec-WebSocket-Key", http.StatusBadRequest)
		return nil, nil, errors.New("missing websocket key")
	}
	conn, rw, err := hj.Hijack()
	if err != nil {
		return nil, nil, err
	}
	accept := websocketAccept(key)
	_, err = rw.WriteString("HTTP/1.1 101 Switching Protocols\r\n" +
		"Upgrade: websocket\r\n" +
		"Connection: Upgrade\r\n" +
		"Sec-WebSocket-Accept: " + accept + "\r\n\r\n")
	if err != nil {
		_ = conn.Close()
		return nil, nil, err
	}
	if err := rw.Flush(); err != nil {
		_ = conn.Close()
		return nil, nil, err
	}
	return conn, rw, nil
}

func websocketAccept(key string) string {
	sum := sha1.Sum([]byte(key + "258EAFA5-E914-47DA-95CA-C5AB0DC85B11"))
	return base64.StdEncoding.EncodeToString(sum[:])
}

func writeWebSocketJSON(rw *bufio.ReadWriter, value any) error {
	data, err := json.Marshal(value)
	if err != nil {
		return err
	}
	if err := writeWebSocketFrame(rw, data); err != nil {
		return err
	}
	return rw.Flush()
}

func writeWebSocketFrame(w io.Writer, payload []byte) error {
	header := []byte{0x81}
	switch {
	case len(payload) < 126:
		header = append(header, byte(len(payload)))
	case len(payload) <= 65535:
		header = append(header, 126, byte(len(payload)>>8), byte(len(payload)))
	default:
		header = append(header, 127)
		var size [8]byte
		binary.BigEndian.PutUint64(size[:], uint64(len(payload)))
		header = append(header, size[:]...)
	}
	if _, err := w.Write(header); err != nil {
		return err
	}
	_, err := w.Write(payload)
	return err
}

func readWebSocketMessage(r io.Reader) ([]byte, error) {
	var hdr [2]byte
	if _, err := io.ReadFull(r, hdr[:]); err != nil {
		return nil, err
	}
	opcode := hdr[0] & 0x0f
	if opcode == 0x8 {
		return nil, io.EOF
	}
	masked := hdr[1]&0x80 != 0
	length := int(hdr[1] & 0x7f)
	if length == 126 {
		var ext [2]byte
		if _, err := io.ReadFull(r, ext[:]); err != nil {
			return nil, err
		}
		length = int(binary.BigEndian.Uint16(ext[:]))
	} else if length == 127 {
		var ext [8]byte
		if _, err := io.ReadFull(r, ext[:]); err != nil {
			return nil, err
		}
		length = int(binary.BigEndian.Uint64(ext[:]))
	}
	var mask [4]byte
	if masked {
		if _, err := io.ReadFull(r, mask[:]); err != nil {
			return nil, err
		}
	}
	payload := make([]byte, length)
	if _, err := io.ReadFull(r, payload); err != nil {
		return nil, err
	}
	if masked {
		for i := range payload {
			payload[i] ^= mask[i%4]
		}
	}
	return payload, nil
}
