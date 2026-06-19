package api

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"connectrpc.com/connect"
	"github.com/google/uuid"
)

func TestCollaborationIdentityFromHeaders(t *testing.T) {
	clientID := uuid.NewString()
	header := http.Header{}
	header.Set(collaborationClientIDHeader, clientID)
	header.Set(collaborationUserIDHeader, "mert")
	header.Set(collaborationUsernameHeader, "Mert")

	identity, err := collaborationIdentityFromHeaderOrDefault(context.Background(), header, "user1")
	if err != nil {
		t.Fatal(err)
	}
	if identity.ClientID != clientID || identity.UserID != "mert" || identity.Username != "Mert" {
		t.Fatalf("identity = %+v, want header values", identity)
	}
}

func TestCollaborationIdentityDefaultsFromRemoteIP(t *testing.T) {
	ctx := WithCollaborationRequestInfo(context.Background(), CollaborationRequestInfo{
		RemoteAddr: "198.51.100.23:54231",
	})
	identity, err := collaborationIdentityFromHeaderOrDefault(ctx, http.Header{}, "user1")
	if err != nil {
		t.Fatal(err)
	}
	if identity.UserID != "198.51.100.23" || identity.Username != "198.51.100.23" {
		t.Fatalf("identity = %+v, want remote IP default", identity)
	}
	if _, err := uuid.Parse(identity.ClientID); err != nil {
		t.Fatalf("client id = %q, want generated uuid", identity.ClientID)
	}
}

func TestValidateCollaborationIdentity(t *testing.T) {
	tests := []struct {
		name     string
		identity CollaborationIdentity
	}{
		{name: "empty client", identity: CollaborationIdentity{UserID: "mert", Username: "Mert"}},
		{name: "empty user id", identity: CollaborationIdentity{ClientID: "client", UserID: " ", Username: "Mert"}},
		{name: "empty username", identity: CollaborationIdentity{ClientID: "client", UserID: "mert", Username: " "}},
		{name: "oversized user id", identity: CollaborationIdentity{ClientID: "client", UserID: strings.Repeat("a", 65), Username: "Mert"}},
		{name: "bad user id", identity: CollaborationIdentity{ClientID: "client", UserID: "mert/cikla", Username: "Mert"}},
		{name: "control username", identity: CollaborationIdentity{ClientID: "client", UserID: "mert", Username: "Mert\nCikla"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := validateCollaborationIdentity(&tt.identity); connect.CodeOf(err) != connect.CodeInvalidArgument {
				t.Fatalf("error = %v, want invalid_argument", err)
			}
		})
	}
	identity := CollaborationIdentity{ClientID: "client", UserID: "mert.cikla", Username: "Mert Cikla"}
	if err := validateCollaborationIdentity(&identity); err != nil {
		t.Fatalf("valid identity rejected: %v", err)
	}
}

func TestRealtimeRoomAssignsFallbackWhenUserIDAlreadyTaken(t *testing.T) {
	room := NewCollaborationHub().getOrCreateRoom(uuid.Nil, 1)
	first := &CollaborationIdentity{ClientID: "client-a", UserID: "user1", Username: "User 1"}
	first = room.assignIdentity(first)
	room.addClient(&realtimeClient{clientID: first.ClientID, userID: first.UserID, username: first.Username})

	second := room.assignIdentity(&CollaborationIdentity{ClientID: "client-b", UserID: "user1", Username: "User 1"})
	if second.UserID != "user2" || second.Username != "user2" {
		t.Fatalf("second identity = %+v, want user2 fallback", second)
	}
}

func TestRealtimeConnectorUpsertAcceptsFrontendPayload(t *testing.T) {
	payload := []byte(`{
		"type": "crdt_connector_upsert",
		"clock": 3,
		"connector": {
			"id": 42,
			"view_id": 7,
			"source_element_id": 10,
			"target_element_id": 20,
			"label": null,
			"description": null,
			"relationship": "calls",
			"direction": "forward",
			"style": "bezier",
			"url": null,
			"source_handle": "right",
			"target_handle": "left",
			"tags": ["api"],
			"created_at": "2026-06-05T12:00:00Z",
			"updated_at": "2026-06-05T12:00:01Z"
		}
	}`)
	var msg realtimeInboundCRDTConnectorUpsert
	if err := json.Unmarshal(payload, &msg); err != nil {
		t.Fatalf("unmarshal frontend connector payload: %v", err)
	}
	room := NewCollaborationHub().getOrCreateRoom(uuid.Nil, 7)
	updated, ok := room.upsertCRDTConnector(msg.Connector, "user1", msg.Clock)
	if !ok || updated == nil || updated.Connector == nil {
		t.Fatalf("upsert returned (%+v, %v), want connector state", updated, ok)
	}
	if updated.ConnectorID != 42 || updated.Connector.ViewID != 7 || updated.Connector.Relationship == nil || *updated.Connector.Relationship != "calls" {
		t.Fatalf("updated connector = %+v", updated)
	}
	if updated.Connector.Label != nil || updated.Connector.URL != nil {
		t.Fatalf("nullable fields = label:%v url:%v, want nil", updated.Connector.Label, updated.Connector.URL)
	}
}
