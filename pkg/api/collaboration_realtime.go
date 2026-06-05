package api

import (
	"context"
	"encoding/json"
	"net/http"
	"strconv"
	"sync"
	"time"

	diagv1 "buf.build/gen/go/tldiagramcom/diagram/protocolbuffers/go/diag/v1"
	"github.com/google/uuid"
	"github.com/gorilla/websocket"
	"google.golang.org/protobuf/types/known/timestamppb"
)

type realtimeViewer struct {
	UserID   string `json:"user_id"`
	Username string `json:"username"`
	Online   bool   `json:"online"`
}

type realtimeCursor struct {
	UserID   string  `json:"user_id"`
	Username string  `json:"username"`
	X        float64 `json:"x"`
	Y        float64 `json:"y"`
}

type realtimeSelection struct {
	UserID      string `json:"user_id"`
	Username    string `json:"username"`
	ElementID   *int32 `json:"element_id"`
	ConnectorID *int32 `json:"connector_id"`
}

type realtimeViewport struct {
	UserID   string  `json:"user_id"`
	Username string  `json:"username"`
	X        float64 `json:"x"`
	Y        float64 `json:"y"`
	Zoom     float64 `json:"zoom"`
}

type realtimeCanvasVisibility struct {
	ActiveTags      []string `json:"active_tags"`
	HiddenLayerTags []string `json:"hidden_layer_tags"`
}

type realtimeCRDTElementState struct {
	ElementID   int32   `json:"element_id"`
	X           float64 `json:"x"`
	Y           float64 `json:"y"`
	Clock       int64   `json:"clock"`
	ActorUserID string  `json:"actor_user_id"`
}

type realtimeCRDTConnectorState struct {
	Connector   *realtimeConnector `json:"connector,omitempty"`
	ConnectorID int32              `json:"connector_id"`
	Deleted     bool               `json:"deleted"`
	Clock       int64              `json:"clock"`
	ActorUserID string             `json:"actor_user_id"`
}

type realtimeConnector struct {
	ID              int32    `json:"id"`
	ViewID          int32    `json:"view_id"`
	SourceElementID int32    `json:"source_element_id"`
	TargetElementID int32    `json:"target_element_id"`
	Label           *string  `json:"label"`
	Description     *string  `json:"description"`
	Relationship    *string  `json:"relationship"`
	Direction       string   `json:"direction"`
	Style           string   `json:"style"`
	URL             *string  `json:"url"`
	SourceHandle    *string  `json:"source_handle"`
	TargetHandle    *string  `json:"target_handle"`
	Tags            []string `json:"tags"`
	CreatedAt       string   `json:"created_at"`
	UpdatedAt       string   `json:"updated_at"`
}

type realtimeDrawingState struct {
	PathID   string          `json:"path_id"`
	UserID   string          `json:"user_id"`
	Points   json.RawMessage `json:"points"`
	Color    string          `json:"color"`
	Width    float64         `json:"width"`
	Text     string          `json:"text,omitempty"`
	FontSize float64         `json:"font_size,omitempty"`
}

type realtimeEnvelope struct {
	Type string `json:"type"`
}

type realtimeInboundCursor struct {
	Type string  `json:"type"`
	X    float64 `json:"x"`
	Y    float64 `json:"y"`
}

type realtimeInboundSelection struct {
	Type        string `json:"type"`
	ElementID   *int32 `json:"element_id"`
	ConnectorID *int32 `json:"connector_id"`
}

type realtimeInboundViewport struct {
	Type string  `json:"type"`
	X    float64 `json:"x"`
	Y    float64 `json:"y"`
	Zoom float64 `json:"zoom"`
}

type realtimeInboundCanvasVisibility struct {
	Type            string   `json:"type"`
	ActiveTags      []string `json:"active_tags"`
	HiddenLayerTags []string `json:"hidden_layer_tags"`
}

type realtimeInboundDrawing struct {
	Type     string          `json:"type"`
	PathID   string          `json:"path_id"`
	Points   json.RawMessage `json:"points"`
	Color    string          `json:"color"`
	Width    float64         `json:"width"`
	Text     string          `json:"text"`
	FontSize float64         `json:"font_size"`
}

type realtimeInboundDrawingDelete struct {
	Type   string `json:"type"`
	PathID string `json:"path_id"`
}

type realtimeInboundCRDTElementPosition struct {
	Type      string  `json:"type"`
	ElementID int32   `json:"element_id"`
	X         float64 `json:"x"`
	Y         float64 `json:"y"`
	Clock     int64   `json:"clock"`
}

type realtimeInboundCRDTConnectorUpsert struct {
	Type      string             `json:"type"`
	Connector *realtimeConnector `json:"connector"`
	Clock     int64              `json:"clock"`
}

type realtimeInboundCRDTConnectorDelete struct {
	Type        string `json:"type"`
	ConnectorID int32  `json:"connector_id"`
	Clock       int64  `json:"clock"`
}

type realtimePresenceSnapshot struct {
	Type             string                        `json:"type"`
	SelfUserID       string                        `json:"self_user_id"`
	SelfUsername     string                        `json:"self_username"`
	Viewers          []*realtimeViewer             `json:"viewers"`
	Collaborators    []*realtimeViewer             `json:"collaborators"`
	Cursors          []*realtimeCursor             `json:"cursors"`
	Selections       []*realtimeSelection          `json:"selections"`
	Viewports        []*realtimeViewport           `json:"viewports"`
	CRDTElements     []*realtimeCRDTElementState   `json:"crdt_elements"`
	CRDTConnectors   []*realtimeCRDTConnectorState `json:"crdt_connectors"`
	LegacyCRDTNodes  []*realtimeCRDTElementState   `json:"crdt_nodes,omitempty"`
	LegacyCRDTEdges  []*realtimeCRDTConnectorState `json:"crdt_edges,omitempty"`
	Drawings         []*realtimeDrawingState       `json:"drawings"`
	CanvasVisibility realtimeCanvasVisibility      `json:"canvas_visibility"`
	HasVisibility    bool                          `json:"has_canvas_visibility"`
}

type realtimePresenceJoin struct {
	Type   string          `json:"type"`
	Viewer *realtimeViewer `json:"viewer"`
}

type realtimePresenceLeave struct {
	Type   string `json:"type"`
	UserID string `json:"user_id"`
}

type realtimeClient struct {
	ctx        context.Context
	conn       *websocket.Conn
	send       chan []byte
	room       *realtimeRoom
	clientID   string
	userID     string
	username   string
	lastActive time.Time
}

type realtimeRoomViewer struct {
	ClientID string
	UserID   string
	Username string
	Count    int
}

type realtimeRoom struct {
	mu               sync.Mutex
	workspaceID      uuid.UUID
	viewID           int32
	clients          map[*realtimeClient]struct{}
	viewers          map[string]*realtimeRoomViewer
	cursors          map[string]realtimeCursor
	selections       map[string]realtimeSelection
	viewports        map[string]realtimeViewport
	crdtElements     map[int32]realtimeCRDTElementState
	crdtConnectors   map[int32]realtimeCRDTConnectorState
	drawings         map[string]realtimeDrawingState
	canvasVisibility realtimeCanvasVisibility
	seeded           bool
}

type CollaborationHub struct {
	mu    sync.Mutex
	rooms map[string]*realtimeRoom
}

func NewCollaborationHub() *CollaborationHub {
	return &CollaborationHub{rooms: map[string]*realtimeRoom{}}
}

func (h *CollaborationHub) roomKey(workspaceID uuid.UUID, viewID int32) string {
	return workspaceID.String() + ":" + strconv.Itoa(int(viewID))
}

func (h *CollaborationHub) getOrCreateRoom(workspaceID uuid.UUID, viewID int32) *realtimeRoom {
	h.mu.Lock()
	defer h.mu.Unlock()
	key := h.roomKey(workspaceID, viewID)
	if room, ok := h.rooms[key]; ok {
		return room
	}
	room := &realtimeRoom{
		workspaceID:    workspaceID,
		viewID:         viewID,
		clients:        map[*realtimeClient]struct{}{},
		viewers:        map[string]*realtimeRoomViewer{},
		cursors:        map[string]realtimeCursor{},
		selections:     map[string]realtimeSelection{},
		viewports:      map[string]realtimeViewport{},
		crdtElements:   map[int32]realtimeCRDTElementState{},
		crdtConnectors: map[int32]realtimeCRDTConnectorState{},
		drawings:       map[string]realtimeDrawingState{},
	}
	h.rooms[key] = room
	return room
}

func (h *CollaborationHub) maybeDeleteRoom(room *realtimeRoom) {
	if room == nil {
		return
	}
	room.mu.Lock()
	empty := len(room.clients) == 0
	room.mu.Unlock()
	if !empty {
		return
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	key := h.roomKey(room.workspaceID, room.viewID)
	if current, ok := h.rooms[key]; ok && current == room {
		delete(h.rooms, key)
	}
}

func (h *CollaborationHub) BroadcastViewEvent(viewID int32, payload any) {
	h.mu.Lock()
	rooms := make([]*realtimeRoom, 0, len(h.rooms))
	for _, room := range h.rooms {
		if room.viewID == viewID {
			rooms = append(rooms, room)
		}
	}
	h.mu.Unlock()
	for _, room := range rooms {
		room.broadcast(payload, nil)
	}
}

type CollaborationRealtimeHandler struct {
	Store            Store
	Hooks            WorkspaceHooks
	IdentityProvider CollaborationIdentityProvider
	Hub              *CollaborationHub
}

func (h *CollaborationRealtimeHandler) WS(w http.ResponseWriter, r *http.Request) {
	viewID, err := parseRequiredInt32("view_id", int32(parsePathInt(r.PathValue("id"))))
	if err != nil {
		http.Error(w, "invalid view id", http.StatusBadRequest)
		return
	}
	workspaceID := collaborationWorkspaceID(r.Context())
	if err := h.hooks().CheckRead(r.Context(), workspaceID); err != nil {
		http.Error(w, err.Error(), http.StatusUnauthorized)
		return
	}
	if _, err := h.Store.GetView(r.Context(), viewID, workspaceID); err != nil {
		http.Error(w, "view not found", http.StatusNotFound)
		return
	}
	service := &CollaborationService{
		Store:            h.Store,
		Hooks:            h.Hooks,
		IdentityProvider: h.IdentityProvider,
		Hub:              h.hub(),
	}
	room := h.hub().getOrCreateRoom(workspaceID, viewID)
	room.seedFromStore(r.Context(), h.Store)
	identity, err := service.currentIdentity(r.Context(), workspaceID, collaborationIdentityHeaderFromRequest(r))
	if err != nil {
		http.Error(w, err.Error(), http.StatusUnauthorized)
		return
	}
	identity = room.assignIdentity(identity)

	conn, err := realtimeUpgrader.Upgrade(w, r, nil)
	if err != nil {
		return
	}
	if room.isFull(identity) {
		msg := websocket.FormatCloseMessage(4409, "room is full (max 10 collaborators)")
		_ = conn.WriteMessage(websocket.CloseMessage, msg)
		_ = conn.Close()
		return
	}
	client := &realtimeClient{
		ctx:        context.WithoutCancel(r.Context()),
		conn:       conn,
		send:       make(chan []byte, 256),
		room:       room,
		clientID:   identity.ClientID,
		userID:     identity.UserID,
		username:   identity.Username,
		lastActive: time.Now(),
	}
	firstSession, viewer := room.addClient(client)
	if payload, err := json.Marshal(room.snapshot(client.userID, client.username)); err == nil {
		client.send <- payload
	}
	if firstSession {
		room.broadcast(&realtimePresenceJoin{Type: "presence_join", Viewer: viewer}, client)
	}
	go h.writePump(client)
	h.readPump(client)
	left, userID := room.removeClient(client)
	if left {
		room.broadcast(&realtimePresenceLeave{Type: "presence_leave", UserID: userID}, client)
	}
	h.hub().maybeDeleteRoom(room)
}

func (h *CollaborationRealtimeHandler) hooks() WorkspaceHooks {
	if h.Hooks == nil {
		return NopWorkspaceHooks{}
	}
	return h.Hooks
}

func (h *CollaborationRealtimeHandler) hub() *CollaborationHub {
	if h.Hub == nil {
		h.Hub = NewCollaborationHub()
	}
	return h.Hub
}

func parsePathInt(value string) int {
	var out int
	for _, r := range value {
		if r < '0' || r > '9' {
			return 0
		}
		out = out*10 + int(r-'0')
	}
	return out
}

func collaborationIdentityHeaderFromRequest(r *http.Request) http.Header {
	header := r.Header.Clone()
	query := r.URL.Query()
	if header.Get(collaborationClientIDHeader) == "" {
		header.Set(collaborationClientIDHeader, query.Get("client_id"))
	}
	if header.Get(collaborationUserIDHeader) == "" {
		header.Set(collaborationUserIDHeader, query.Get("user_id"))
	}
	if header.Get(collaborationUsernameHeader) == "" {
		header.Set(collaborationUsernameHeader, query.Get("username"))
	}
	return header
}

func (r *realtimeRoom) seedFromStore(ctx context.Context, store Store) {
	r.mu.Lock()
	if r.seeded {
		r.mu.Unlock()
		return
	}
	r.mu.Unlock()
	if placements, err := store.ListPlacements(ctx, r.viewID); err == nil {
		r.mu.Lock()
		for _, placement := range placements {
			r.crdtElements[placement.GetElementId()] = realtimeCRDTElementState{
				ElementID: placement.GetElementId(),
				X:         placement.GetPositionX(),
				Y:         placement.GetPositionY(),
				Clock:     0,
			}
		}
		r.mu.Unlock()
	}
	if connectors, err := store.ListConnectors(ctx, r.viewID, r.workspaceID); err == nil {
		r.mu.Lock()
		for _, connector := range connectors {
			r.crdtConnectors[connector.GetId()] = realtimeCRDTConnectorState{
				Connector:   realtimeConnectorFromProto(connector),
				ConnectorID: connector.GetId(),
				Deleted:     false,
			}
		}
		r.mu.Unlock()
	}
	if drawings, err := store.ListDrawings(ctx, r.workspaceID, r.viewID); err == nil {
		r.mu.Lock()
		for _, drawing := range drawings {
			r.drawings[drawing.PathID] = realtimeDrawingState(drawing)
		}
		r.mu.Unlock()
	}
	r.mu.Lock()
	r.seeded = true
	r.mu.Unlock()
}

func realtimeConnectorFromProto(connector *diagv1.Connector) *realtimeConnector {
	if connector == nil {
		return nil
	}
	return &realtimeConnector{
		ID:              connector.GetId(),
		ViewID:          connector.GetViewId(),
		SourceElementID: connector.GetSourceElementId(),
		TargetElementID: connector.GetTargetElementId(),
		Label:           cloneStringPtr(connector.Label),
		Description:     cloneStringPtr(connector.Description),
		Relationship:    cloneStringPtr(connector.Relationship),
		Direction:       connector.GetDirection(),
		Style:           connector.GetStyle(),
		URL:             cloneStringPtr(connector.Url),
		SourceHandle:    cloneStringPtr(connector.SourceHandle),
		TargetHandle:    cloneStringPtr(connector.TargetHandle),
		Tags:            cloneRealtimeStrings(connector.GetTags()),
		CreatedAt:       formatRealtimeTimestamp(connector.GetCreatedAt()),
		UpdatedAt:       formatRealtimeTimestamp(connector.GetUpdatedAt()),
	}
}

func cloneStringPtr(value *string) *string {
	if value == nil {
		return nil
	}
	out := *value
	return &out
}

func formatRealtimeTimestamp(ts *timestamppb.Timestamp) string {
	if ts == nil {
		return ""
	}
	return ts.AsTime().UTC().Format(time.RFC3339Nano)
}

func (r *realtimeRoom) assignIdentity(identity *CollaborationIdentity) *CollaborationIdentity {
	r.mu.Lock()
	defer r.mu.Unlock()
	next := *identity
	if viewer, exists := r.viewers[next.UserID]; exists && viewer.ClientID != next.ClientID {
		next.UserID = r.nextFallbackUserIDLocked()
		next.Username = next.UserID
	}
	return &next
}

func (r *realtimeRoom) nextFallbackUserIDLocked() string {
	used := map[int]struct{}{}
	for userID := range r.viewers {
		if len(userID) <= 4 || userID[:4] != "user" {
			continue
		}
		idx, err := strconv.Atoi(userID[4:])
		if err == nil && idx > 0 {
			used[idx] = struct{}{}
		}
	}
	for i := 1; ; i++ {
		if _, ok := used[i]; !ok {
			return "user" + strconv.Itoa(i)
		}
	}
}

func (r *realtimeRoom) isFull(identity *CollaborationIdentity) bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	if viewer, exists := r.viewers[identity.UserID]; exists && viewer.ClientID == identity.ClientID {
		return false
	}
	return len(r.viewers) >= realtimeMaxRoomViewers
}

func (r *realtimeRoom) addClient(client *realtimeClient) (bool, *realtimeViewer) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.clients[client] = struct{}{}
	viewer, exists := r.viewers[client.userID]
	if exists && viewer.ClientID == client.clientID {
		viewer.Count++
		return false, &realtimeViewer{UserID: viewer.UserID, Username: viewer.Username, Online: true}
	}
	r.viewers[client.userID] = &realtimeRoomViewer{ClientID: client.clientID, UserID: client.userID, Username: client.username, Count: 1}
	return true, &realtimeViewer{UserID: client.userID, Username: client.username, Online: true}
}

func (r *realtimeRoom) removeClient(client *realtimeClient) (bool, string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	delete(r.clients, client)
	viewer, exists := r.viewers[client.userID]
	if !exists {
		return false, ""
	}
	viewer.Count--
	if viewer.Count > 0 {
		return false, ""
	}
	delete(r.viewers, client.userID)
	delete(r.cursors, client.userID)
	delete(r.selections, client.userID)
	delete(r.viewports, client.userID)
	return true, client.userID
}

func (r *realtimeRoom) snapshot(selfUserID string, selfUsername string) *realtimePresenceSnapshot {
	r.mu.Lock()
	defer r.mu.Unlock()
	viewers := make([]*realtimeViewer, 0, len(r.viewers))
	for _, viewer := range r.viewers {
		viewers = append(viewers, &realtimeViewer{UserID: viewer.UserID, Username: viewer.Username, Online: true})
	}
	collaborators := make([]*realtimeViewer, 0, len(viewers))
	collaborators = append(collaborators, viewers...)
	cursors := make([]*realtimeCursor, 0, len(r.cursors))
	for _, cursor := range r.cursors {
		copy := cursor
		cursors = append(cursors, &copy)
	}
	selections := make([]*realtimeSelection, 0, len(r.selections))
	for _, selection := range r.selections {
		copy := selection
		selections = append(selections, &copy)
	}
	viewports := make([]*realtimeViewport, 0, len(r.viewports))
	for _, viewport := range r.viewports {
		copy := viewport
		viewports = append(viewports, &copy)
	}
	elements := make([]*realtimeCRDTElementState, 0, len(r.crdtElements))
	for _, element := range r.crdtElements {
		copy := element
		elements = append(elements, &copy)
	}
	connectors := make([]*realtimeCRDTConnectorState, 0, len(r.crdtConnectors))
	for _, connector := range r.crdtConnectors {
		copy := connector
		connectors = append(connectors, &copy)
	}
	drawings := make([]*realtimeDrawingState, 0, len(r.drawings))
	for _, drawing := range r.drawings {
		copy := drawing
		drawings = append(drawings, &copy)
	}
	visibility := realtimeCanvasVisibility{
		ActiveTags:      cloneRealtimeStrings(r.canvasVisibility.ActiveTags),
		HiddenLayerTags: cloneRealtimeStrings(r.canvasVisibility.HiddenLayerTags),
	}
	return &realtimePresenceSnapshot{
		Type:             "presence_snapshot",
		SelfUserID:       selfUserID,
		SelfUsername:     selfUsername,
		Viewers:          viewers,
		Collaborators:    collaborators,
		Cursors:          cursors,
		Selections:       selections,
		Viewports:        viewports,
		CRDTElements:     elements,
		CRDTConnectors:   connectors,
		LegacyCRDTNodes:  elements,
		LegacyCRDTEdges:  connectors,
		Drawings:         drawings,
		CanvasVisibility: visibility,
		HasVisibility:    len(visibility.ActiveTags) > 0 || len(visibility.HiddenLayerTags) > 0,
	}
}

func (r *realtimeRoom) setCursor(userID, username string, x, y float64) realtimeCursor {
	r.mu.Lock()
	defer r.mu.Unlock()
	cursor := realtimeCursor{UserID: userID, Username: username, X: x, Y: y}
	r.cursors[userID] = cursor
	return cursor
}

func (r *realtimeRoom) setSelection(userID, username string, elementID, connectorID *int32) realtimeSelection {
	r.mu.Lock()
	defer r.mu.Unlock()
	selection := realtimeSelection{UserID: userID, Username: username, ElementID: elementID, ConnectorID: connectorID}
	r.selections[userID] = selection
	return selection
}

func (r *realtimeRoom) setViewport(userID, username string, x, y, zoom float64) realtimeViewport {
	r.mu.Lock()
	defer r.mu.Unlock()
	viewport := realtimeViewport{UserID: userID, Username: username, X: x, Y: y, Zoom: zoom}
	r.viewports[userID] = viewport
	return viewport
}

func (r *realtimeRoom) setCanvasVisibility(activeTags, hiddenLayerTags []string) realtimeCanvasVisibility {
	r.mu.Lock()
	defer r.mu.Unlock()
	visibility := realtimeCanvasVisibility{ActiveTags: cloneRealtimeStrings(activeTags), HiddenLayerTags: cloneRealtimeStrings(hiddenLayerTags)}
	r.canvasVisibility = visibility
	return visibility
}

func (r *realtimeRoom) addDrawing(userID string, msg realtimeInboundDrawing) realtimeDrawingState {
	r.mu.Lock()
	defer r.mu.Unlock()
	drawing := realtimeDrawingState{
		PathID:   msg.PathID,
		UserID:   userID,
		Points:   msg.Points,
		Color:    msg.Color,
		Width:    msg.Width,
		Text:     msg.Text,
		FontSize: msg.FontSize,
	}
	r.drawings[msg.PathID] = drawing
	return drawing
}

func (r *realtimeRoom) removeDrawing(pathID string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	delete(r.drawings, pathID)
}

func (r *realtimeRoom) setCRDTElementPosition(elementID int32, actorUserID string, x, y float64, clock int64) (*realtimeCRDTElementState, bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	current, exists := r.crdtElements[elementID]
	if exists && !preferRealtimeUpdate(clock, actorUserID, current.Clock, current.ActorUserID) {
		return nil, false
	}
	updated := realtimeCRDTElementState{ElementID: elementID, X: x, Y: y, Clock: clock, ActorUserID: actorUserID}
	r.crdtElements[elementID] = updated
	return &updated, true
}

func (r *realtimeRoom) upsertCRDTConnector(connector *realtimeConnector, actorUserID string, clock int64) (*realtimeCRDTConnectorState, bool) {
	if connector == nil {
		return nil, false
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	current, exists := r.crdtConnectors[connector.ID]
	if exists && !preferRealtimeUpdate(clock, actorUserID, current.Clock, current.ActorUserID) {
		return nil, false
	}
	connectorCopy := *connector
	updated := realtimeCRDTConnectorState{Connector: &connectorCopy, ConnectorID: connector.ID, Deleted: false, Clock: clock, ActorUserID: actorUserID}
	r.crdtConnectors[connector.ID] = updated
	return &updated, true
}

func (r *realtimeRoom) deleteCRDTConnector(connectorID int32, actorUserID string, clock int64) (*realtimeCRDTConnectorState, bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	current, exists := r.crdtConnectors[connectorID]
	if exists && !preferRealtimeUpdate(clock, actorUserID, current.Clock, current.ActorUserID) {
		return nil, false
	}
	updated := realtimeCRDTConnectorState{ConnectorID: connectorID, Deleted: true, Clock: clock, ActorUserID: actorUserID}
	r.crdtConnectors[connectorID] = updated
	return &updated, true
}

func (r *realtimeRoom) broadcast(v any, skip *realtimeClient) {
	payload, err := json.Marshal(v)
	if err != nil {
		return
	}
	r.mu.Lock()
	clients := make([]*realtimeClient, 0, len(r.clients))
	for client := range r.clients {
		clients = append(clients, client)
	}
	r.mu.Unlock()
	for _, client := range clients {
		if skip != nil && client == skip {
			continue
		}
		select {
		case client.send <- payload:
		default:
		}
	}
}

var realtimeUpgrader = websocket.Upgrader{
	ReadBufferSize:  4096,
	WriteBufferSize: 4096,
	CheckOrigin: func(*http.Request) bool {
		return true
	},
}

const (
	realtimeWriteWait      = 10 * time.Second
	realtimePongWait       = 90 * time.Second
	realtimePingPeriod     = 81 * time.Second
	realtimeMaxMessageSize = 8 * 1024
	realtimeMaxRoomViewers = 10
	realtimeIdleTimeout    = 5 * time.Minute
)

func (h *CollaborationRealtimeHandler) readPump(client *realtimeClient) {
	defer func() {
		_ = client.conn.Close()
	}()
	client.conn.SetReadLimit(realtimeMaxMessageSize)
	_ = client.conn.SetReadDeadline(time.Now().Add(realtimePongWait))
	client.conn.SetPongHandler(func(string) error {
		return client.conn.SetReadDeadline(time.Now().Add(realtimePongWait))
	})
	for {
		_, payload, err := client.conn.ReadMessage()
		if err != nil {
			break
		}
		client.lastActive = time.Now()
		var envelope realtimeEnvelope
		if err := json.Unmarshal(payload, &envelope); err != nil {
			continue
		}
		switch envelope.Type {
		case "cursor":
			var msg realtimeInboundCursor
			if json.Unmarshal(payload, &msg) != nil {
				continue
			}
			cursor := client.room.setCursor(client.userID, client.username, msg.X, msg.Y)
			client.room.broadcast(&struct {
				realtimeCursor
				Type string `json:"type"`
			}{realtimeCursor: cursor, Type: "cursor"}, client)
		case "selection":
			var msg realtimeInboundSelection
			if json.Unmarshal(payload, &msg) != nil {
				continue
			}
			selection := client.room.setSelection(client.userID, client.username, msg.ElementID, msg.ConnectorID)
			client.room.broadcast(&struct {
				realtimeSelection
				Type string `json:"type"`
			}{realtimeSelection: selection, Type: "selection"}, client)
		case "viewport":
			var msg realtimeInboundViewport
			if json.Unmarshal(payload, &msg) != nil {
				continue
			}
			viewport := client.room.setViewport(client.userID, client.username, msg.X, msg.Y, msg.Zoom)
			client.room.broadcast(&struct {
				realtimeViewport
				Type string `json:"type"`
			}{realtimeViewport: viewport, Type: "viewport"}, client)
		case "canvas_visibility":
			var msg realtimeInboundCanvasVisibility
			if json.Unmarshal(payload, &msg) != nil {
				continue
			}
			visibility := client.room.setCanvasVisibility(msg.ActiveTags, msg.HiddenLayerTags)
			client.room.broadcast(&struct {
				Type string `json:"type"`
				realtimeCanvasVisibility
			}{Type: "canvas_visibility", realtimeCanvasVisibility: visibility}, client)
		case "drawing":
			var msg realtimeInboundDrawing
			if json.Unmarshal(payload, &msg) != nil || msg.PathID == "" {
				continue
			}
			drawing := client.room.addDrawing(client.userID, msg)
			client.room.broadcast(&struct {
				Type string `json:"type"`
				realtimeDrawingState
			}{Type: "drawing", realtimeDrawingState: drawing}, client)
			go h.persistDrawing(client.ctx, client.room.workspaceID, client.room.viewID, client.userID, msg)
		case "drawing_delete":
			var msg realtimeInboundDrawingDelete
			if json.Unmarshal(payload, &msg) != nil || msg.PathID == "" {
				continue
			}
			client.room.removeDrawing(msg.PathID)
			client.room.broadcast(&struct {
				Type   string `json:"type"`
				PathID string `json:"path_id"`
			}{Type: "drawing_delete", PathID: msg.PathID}, client)
			go h.persistDrawingDelete(client.ctx, client.room.workspaceID, client.room.viewID, msg.PathID)
		case "crdt_element_position", "crdt_node_position":
			var msg realtimeInboundCRDTElementPosition
			if json.Unmarshal(payload, &msg) != nil {
				continue
			}
			updated, ok := client.room.setCRDTElementPosition(msg.ElementID, client.userID, msg.X, msg.Y, msg.Clock)
			if !ok || updated == nil {
				continue
			}
			client.room.broadcast(&struct {
				Type string `json:"type"`
				*realtimeCRDTElementState
			}{Type: "crdt_element_position", realtimeCRDTElementState: updated}, nil)
		case "crdt_connector_upsert", "crdt_edge_upsert":
			var msg realtimeInboundCRDTConnectorUpsert
			if json.Unmarshal(payload, &msg) != nil || msg.Connector == nil || msg.Connector.ViewID != client.room.viewID {
				continue
			}
			updated, ok := client.room.upsertCRDTConnector(msg.Connector, client.userID, msg.Clock)
			if !ok || updated == nil {
				continue
			}
			client.room.broadcast(&struct {
				Type string `json:"type"`
				*realtimeCRDTConnectorState
			}{Type: "crdt_connector_upsert", realtimeCRDTConnectorState: updated}, nil)
		case "crdt_connector_delete", "crdt_edge_delete":
			var msg realtimeInboundCRDTConnectorDelete
			if json.Unmarshal(payload, &msg) != nil {
				continue
			}
			updated, ok := client.room.deleteCRDTConnector(msg.ConnectorID, client.userID, msg.Clock)
			if !ok || updated == nil {
				continue
			}
			client.room.broadcast(&struct {
				Type string `json:"type"`
				*realtimeCRDTConnectorState
			}{Type: "crdt_connector_delete", realtimeCRDTConnectorState: updated}, nil)
		}
	}
}

func (h *CollaborationRealtimeHandler) persistDrawing(baseCtx context.Context, workspaceID uuid.UUID, viewID int32, userID string, msg realtimeInboundDrawing) {
	ctx, cancel := context.WithTimeout(baseCtx, 5*time.Second)
	defer cancel()
	_ = h.Store.UpsertDrawing(ctx, workspaceID, viewID, RealtimeDrawingInput{
		PathID:   msg.PathID,
		UserID:   userID,
		Points:   msg.Points,
		Color:    msg.Color,
		Width:    msg.Width,
		Text:     msg.Text,
		FontSize: msg.FontSize,
	})
}

func (h *CollaborationRealtimeHandler) persistDrawingDelete(baseCtx context.Context, workspaceID uuid.UUID, viewID int32, pathID string) {
	ctx, cancel := context.WithTimeout(baseCtx, 5*time.Second)
	defer cancel()
	_ = h.Store.DeleteDrawing(ctx, workspaceID, viewID, pathID)
}

func (h *CollaborationRealtimeHandler) writePump(client *realtimeClient) {
	pingTicker := time.NewTicker(realtimePingPeriod)
	idleTicker := time.NewTicker(realtimeIdleTimeout / 2)
	defer func() {
		pingTicker.Stop()
		idleTicker.Stop()
		_ = client.conn.Close()
	}()
	for {
		select {
		case payload, ok := <-client.send:
			_ = client.conn.SetWriteDeadline(time.Now().Add(realtimeWriteWait))
			if !ok {
				_ = client.conn.WriteMessage(websocket.CloseMessage, []byte{})
				return
			}
			if err := client.conn.WriteMessage(websocket.TextMessage, payload); err != nil {
				return
			}
		case <-pingTicker.C:
			_ = client.conn.SetWriteDeadline(time.Now().Add(realtimeWriteWait))
			if err := client.conn.WriteMessage(websocket.PingMessage, nil); err != nil {
				return
			}
		case <-idleTicker.C:
			if time.Since(client.lastActive) >= realtimeIdleTimeout {
				msg := websocket.FormatCloseMessage(websocket.CloseNormalClosure, "idle timeout")
				_ = client.conn.WriteMessage(websocket.CloseMessage, msg)
				return
			}
		}
	}
}

func preferRealtimeUpdate(incomingClock int64, incomingActor string, currentClock int64, currentActor string) bool {
	if incomingClock > currentClock {
		return true
	}
	if incomingClock < currentClock {
		return false
	}
	return incomingActor > currentActor
}

func cloneRealtimeStrings(values []string) []string {
	if len(values) == 0 {
		return []string{}
	}
	out := make([]string, len(values))
	copy(out, values)
	return out
}
