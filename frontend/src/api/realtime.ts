import type {
  Connector,
  LibraryElement,
  PlacedElement,
} from '../types'
import type {
  RealtimeCanvasVisibility,
  RealtimeCRDTConnectorState,
  RealtimeCRDTElementState,
  RealtimeCursor,
  RealtimeDrawing,
  RealtimePresenceSnapshot,
  RealtimeReactionSummary,
  RealtimeSelection,
  RealtimeUserPresence,
  RealtimeViewComment,
  RealtimeViewStateEvent,
  RealtimeViewThread,
  RealtimeViewport,
  ViewRealtimeConnection,
  ViewRealtimeHandlers,
} from '../platform/types'
import { apiUrl, isWailsApp } from '../config/runtime'
import { getCollaborationIdentity } from './collaborationIdentity'

type RealtimePayload = { type: string } & Record<string, unknown>

function normalizeCRDTElementState(payload: Record<string, unknown>): RealtimeCRDTElementState {
  return {
    element_id: typeof payload.element_id === 'number' ? payload.element_id : 0,
    x: typeof payload.x === 'number' ? payload.x : 0,
    y: typeof payload.y === 'number' ? payload.y : 0,
    clock: typeof payload.clock === 'number' ? payload.clock : 0,
    actor_user_id: typeof payload.actor_user_id === 'string' ? payload.actor_user_id : '',
  }
}

function normalizeSelection(payload: Record<string, unknown>): RealtimeSelection {
  return {
    user_id: typeof payload.user_id === 'string' ? payload.user_id : '',
    username: typeof payload.username === 'string' ? payload.username : '',
    element_id: typeof payload.element_id === 'number' ? payload.element_id : null,
    connector_id: typeof payload.connector_id === 'number' ? payload.connector_id : null,
  }
}

function normalizeCRDTConnectorState(payload: Record<string, unknown>): RealtimeCRDTConnectorState {
  const connector = payload.connector as Connector | undefined
  const connectorId = typeof payload.connector_id === 'number'
    ? payload.connector_id
    : connector?.id ?? 0

  return {
    connector,
    connector_id: connectorId,
    deleted: !!payload.deleted,
    clock: typeof payload.clock === 'number' ? payload.clock : 0,
    actor_user_id: typeof payload.actor_user_id === 'string' ? payload.actor_user_id : '',
  }
}

function normalizeCanvasVisibility(payload: Record<string, unknown>): RealtimeCanvasVisibility {
  return {
    active_tags: Array.isArray(payload.active_tags)
      ? payload.active_tags.filter((item): item is string => typeof item === 'string')
      : [],
    hidden_layer_tags: Array.isArray(payload.hidden_layer_tags)
      ? payload.hidden_layer_tags.filter((item): item is string => typeof item === 'string')
      : [],
  }
}

function normalizePresenceSnapshot(payload: RealtimePayload): RealtimePresenceSnapshot {
  const rawElements = Array.isArray(payload.crdt_elements) ? payload.crdt_elements : payload.crdt_nodes
  const rawConnectors = Array.isArray(payload.crdt_connectors) ? payload.crdt_connectors : payload.crdt_edges
  return {
    self_user_id: typeof payload.self_user_id === 'string' ? payload.self_user_id : '',
    self_username: typeof payload.self_username === 'string' ? payload.self_username : undefined,
    viewers: (payload.viewers as RealtimeUserPresence[]) ?? [],
    collaborators: (payload.collaborators as RealtimeUserPresence[]) ?? [],
    cursors: (payload.cursors as RealtimeCursor[]) ?? [],
    selections: Array.isArray(payload.selections)
      ? payload.selections.map((item) => normalizeSelection((item ?? {}) as Record<string, unknown>))
      : [],
    viewports: (payload.viewports as RealtimeViewport[]) ?? [],
    crdt_elements: Array.isArray(rawElements)
      ? rawElements.map((item) => normalizeCRDTElementState((item ?? {}) as Record<string, unknown>))
      : [],
    crdt_connectors: Array.isArray(rawConnectors)
      ? rawConnectors.map((item) => normalizeCRDTConnectorState((item ?? {}) as Record<string, unknown>))
      : [],
    drawings: (payload.drawings as RealtimeDrawing[]) ?? [],
    canvas_visibility: normalizeCanvasVisibility((payload.canvas_visibility ?? {}) as Record<string, unknown>),
    has_canvas_visibility: payload.has_canvas_visibility === true,
  }
}

function wsUrlForView(viewId: number): string {
  const base = isWailsApp ? window.__TLD_SERVER_URL__ : window.location.href
  if (isWailsApp && !base) {
    throw new Error('Desktop server URL is not configured')
  }
  const url = new URL(apiUrl(`/views/${viewId}/ws`), base)
  url.protocol = url.protocol === 'https:' ? 'wss:' : 'ws:'
  const identity = getCollaborationIdentity()
  url.searchParams.set('client_id', identity.client_id)
  url.searchParams.set('user_id', identity.user_id)
  url.searchParams.set('username', identity.username)
  return url.toString()
}

function isViewStateChange(type: string): boolean {
  return type === 'workspace_changed' || /^(placement|connector|element|view|layer|view_markdown)_(create|update|delete)$/.test(type)
}

export function connectViewRealtime(viewId: number, handlers: ViewRealtimeHandlers): ViewRealtimeConnection {
  const socket = new WebSocket(wsUrlForView(viewId))
  let closeOnOpen = false
  let selfUserId = ''
  let collaboratorIds = new Set<string>()
  let lastElementPositionSentAt = 0

  socket.onopen = () => {
    if (closeOnOpen) {
      closeOnOpen = false
      socket.close()
    }
  }

  socket.onmessage = (event) => {
    try {
      const payload = JSON.parse(event.data) as RealtimePayload
      switch (payload.type) {
        case 'presence_snapshot': {
          const snapshot = normalizePresenceSnapshot(payload)
          selfUserId = snapshot.self_user_id
          collaboratorIds = new Set(snapshot.collaborators.map((user) => user.user_id).filter((id) => id && id !== selfUserId))
          handlers.onSnapshot(snapshot)
          break
        }
        case 'presence_join':
          if (payload.viewer) {
            const viewer = payload.viewer as RealtimeUserPresence
            if (viewer.user_id && viewer.user_id !== selfUserId) collaboratorIds.add(viewer.user_id)
            handlers.onPresenceJoin(viewer)
          }
          break
        case 'presence_leave':
          if (typeof payload.user_id === 'string') {
            collaboratorIds.delete(payload.user_id)
            handlers.onPresenceLeave(payload.user_id)
          }
          break
        case 'cursor':
          handlers.onCursor(payload as unknown as RealtimeCursor)
          break
        case 'selection':
          handlers.onSelection(normalizeSelection(payload))
          break
        case 'viewport':
          handlers.onViewport(payload as unknown as RealtimeViewport)
          break
        case 'canvas_visibility':
          handlers.onCanvasVisibility(normalizeCanvasVisibility(payload))
          break
        case 'drawing':
          handlers.onDrawing(payload as unknown as RealtimeDrawing)
          break
        case 'drawing_delete':
          if (typeof payload.path_id === 'string') handlers.onDrawingDelete(payload.path_id)
          break
        case 'crdt_element_position':
        case 'crdt_node_position':
          handlers.onCRDTElementPosition(normalizeCRDTElementState(payload))
          break
        case 'crdt_connector_upsert':
        case 'crdt_edge_upsert':
          handlers.onCRDTConnectorUpsert(normalizeCRDTConnectorState(payload))
          break
        case 'crdt_connector_delete':
        case 'crdt_edge_delete':
          handlers.onCRDTConnectorDelete(normalizeCRDTConnectorState(payload))
          break
        case 'view_element_add':
          if (payload.element) handlers.onViewElementAdd(payload.element as PlacedElement)
          break
        case 'view_element_remove':
          if (typeof payload.element_id === 'number') handlers.onViewElementRemove(payload.element_id)
          break
        case 'element_update':
          if (payload.element) {
            handlers.onElementUpdate(payload.element as LibraryElement)
          } else {
            handlers.onViewStateChange?.(payload as RealtimeViewStateEvent)
          }
          break
        case 'thread_upsert':
          if (payload.thread) handlers.onThreadUpsert(payload.thread as RealtimeViewThread)
          break
        case 'thread_resolve':
          handlers.onThreadResolve(payload as unknown as { thread_id: number; resolved: boolean })
          break
        case 'comment_create':
          if (payload.comment) handlers.onCommentCreate(payload.comment as RealtimeViewComment)
          break
        case 'reactions_snapshot':
          handlers.onReactionsSnapshot((payload.items as RealtimeReactionSummary[]) || [])
          break
        default:
          if (isViewStateChange(payload.type)) handlers.onViewStateChange?.(payload as RealtimeViewStateEvent)
          break
      }
    } catch {
      // Ignore malformed realtime frames; the next valid frame will refresh state.
    }
  }

  socket.onclose = (event) => {
    if (event.code === 4409) handlers.onRoomFull?.()
    handlers.onClose?.()
  }

  const send = (message: Record<string, unknown>) => {
    if (socket.readyState !== WebSocket.OPEN) return
    socket.send(JSON.stringify(message))
  }

  return {
    sendCursor: (x, y) => send({ type: 'cursor', x, y }),
    sendSelection: (elementId, connectorId) => send({ type: 'selection', element_id: elementId ?? null, connector_id: connectorId ?? null }),
    sendViewport: (x, y, zoom) => send({ type: 'viewport', x, y, zoom }),
    sendCanvasVisibility: (activeTags, hiddenLayerTags) => send({ type: 'canvas_visibility', active_tags: activeTags, hidden_layer_tags: hiddenLayerTags }),
    sendDrawing: (pathId, points, color, width, text, fontSize) => send({ type: 'drawing', path_id: pathId, points, color, width, text, font_size: fontSize }),
    sendDrawingDelete: (pathId) => send({ type: 'drawing_delete', path_id: pathId }),
    sendCRDTElementPosition: (elementId, x, y, clock) => {
      if (collaboratorIds.size === 0) return
      const now = performance.now()
      if (now - lastElementPositionSentAt < 25) return
      lastElementPositionSentAt = now
      send({ type: 'crdt_element_position', element_id: elementId, x, y, clock })
    },
    sendCRDTConnectorUpsert: (connector, clock) => send({ type: 'crdt_connector_upsert', connector, clock }),
    sendCRDTConnectorDelete: (connectorId, clock) => send({ type: 'crdt_connector_delete', connector_id: connectorId, clock }),
    disconnect: () => {
      if (socket.readyState === WebSocket.CONNECTING) {
        closeOnOpen = true
        return
      }
      if (socket.readyState === WebSocket.OPEN) socket.close()
    },
  }
}
