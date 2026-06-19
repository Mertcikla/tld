import type { ComponentType, ReactNode } from 'react'
import type { Connector, ElementReactionSummary, LibraryElement, PlacedElement, ThreadResolveEvent, ViewComment, ViewThread } from '../types'

export interface PlatformRouteContext<User = unknown> {
  user: User | null
  onLogin?: (user: User) => void
  onLogout?: () => void
}

export interface RealtimeUserPresence {
  user_id: string
  username: string
  online: boolean
  [key: string]: unknown
}

export interface RealtimeCursor {
  user_id: string
  username: string
  x: number
  y: number
  [key: string]: unknown
}

export interface RealtimeSelection {
  user_id: string
  username: string
  element_id: number | null
  connector_id: number | null
  [key: string]: unknown
}

export interface RealtimeViewport {
  user_id?: string
  x?: number
  y?: number
  zoom?: number
  [key: string]: unknown
}

export interface RealtimeDrawing {
  path_id?: string
  points?: { x: number; y: number; pressure?: number }[]
  color?: string
  width?: number
  text?: string
  font_size?: number
  [key: string]: unknown
}

export interface RealtimeCRDTElementState {
  element_id: number
  x: number
  y: number
  clock: number
  actor_user_id: string
}

export interface RealtimeCRDTConnectorState {
  connector?: Connector
  connector_id: number
  deleted: boolean
  clock: number
  actor_user_id: string
}

export interface RealtimeCanvasVisibility {
  active_tags: string[]
  hidden_layer_tags: string[]
}

export interface RealtimePresenceSnapshot {
  self_user_id: string
  self_username?: string
  viewers: RealtimeUserPresence[]
  collaborators: RealtimeUserPresence[]
  cursors: RealtimeCursor[]
  selections: RealtimeSelection[]
  viewports: RealtimeViewport[]
  crdt_elements: RealtimeCRDTElementState[]
  crdt_connectors: RealtimeCRDTConnectorState[]
  drawings: RealtimeDrawing[]
  canvas_visibility: RealtimeCanvasVisibility
  has_canvas_visibility?: boolean
}

export type RealtimeThreadResolveEvent = ThreadResolveEvent
export type RealtimeViewThread = ViewThread
export type RealtimeViewComment = ViewComment
export type RealtimeReactionSummary = ElementReactionSummary

export interface RealtimeViewStateEvent {
  type: string
  resource_id?: string
  details?: Record<string, unknown>
  [key: string]: unknown
}

export interface ViewRealtimeHandlers {
  onSnapshot: (snapshot: RealtimePresenceSnapshot) => void
  onPresenceJoin: (viewer: RealtimeUserPresence) => void
  onPresenceLeave: (userId: string) => void
  onCursor: (cursor: RealtimeCursor) => void
  onSelection: (selection: RealtimeSelection) => void
  onViewport: (viewport: RealtimeViewport) => void
  onCanvasVisibility: (visibility: RealtimeCanvasVisibility) => void
  onDrawing: (drawing: RealtimeDrawing) => void
  onDrawingDelete: (pathId: string) => void
  onCRDTElementPosition: (state: RealtimeCRDTElementState) => void
  onCRDTConnectorUpsert: (state: RealtimeCRDTConnectorState) => void
  onCRDTConnectorDelete: (state: RealtimeCRDTConnectorState) => void
  onViewElementAdd: (element: PlacedElement) => void
  onViewElementRemove: (elementId: number) => void
  onElementUpdate: (element: LibraryElement) => void
  onThreadUpsert: (thread: RealtimeViewThread) => void
  onThreadResolve: (event: RealtimeThreadResolveEvent) => void
  onCommentCreate: (comment: RealtimeViewComment) => void
  onReactionsSnapshot: (items: RealtimeReactionSummary[]) => void
  onViewStateChange?: (event: RealtimeViewStateEvent) => void
  onClose?: () => void
  onRoomFull?: () => void
}

export interface ViewRealtimeConnection {
  sendCursor: (x: number, y: number) => void
  sendSelection: (elementId: number | null, connectorId: number | null) => void
  sendViewport: (x: number, y: number, zoom: number) => void
  sendCanvasVisibility: (activeTags: string[], hiddenLayerTags: string[]) => void
  sendDrawing: (pathId: string, points: { x: number; y: number; pressure?: number }[], color: string, width: number, text?: string, fontSize?: number) => void
  sendDrawingDelete: (pathId: string) => void
  sendCRDTElementPosition: (elementId: number, x: number, y: number, clock: number) => void
  sendCRDTConnectorUpsert: (connector: unknown, clock: number) => void
  sendCRDTConnectorDelete: (connectorId: number, clock: number) => void
  disconnect: () => void
}

export interface PlatformFeatures<User = unknown> {
  hasAuth?: boolean
  hasBilling?: boolean
  initPlatform: (orgId?: string) => Promise<void>
  getUpgradePath?: () => string | null
  getRoutes: (context: PlatformRouteContext<User>) => ReactNode[]
  getAuthenticatedRoutes: (context: PlatformRouteContext<User>) => ReactNode[]
  getSettingsRoutes: (context: PlatformRouteContext<User>) => ReactNode[]
  AuthLayout: ComponentType<unknown>
  connectRealtime?: (viewId: number, handlers: ViewRealtimeHandlers) => ViewRealtimeConnection
}
