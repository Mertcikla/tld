import { create } from 'zustand'
import type { Edge as RFEdge, Node as RFNode } from 'reactflow'
import type {
  Connector,
  IncomingViewConnector,
  LibraryElement,
  PlacedElement,
  ViewConnector,
  ViewTreeNode,
} from '../types'

export type StoreSetter<T> = T | ((previous: T) => T)

export type ViewEditorUiState = {
  viewId: number | null
  canEdit: boolean
  isOwner: boolean
  isFreePlan: boolean
  snapToGrid: boolean
  selectedElement: LibraryElement | null
  selectedConnector: Connector | null
}

export type ViewContentLinks = {
  linksMap: Record<number, ViewConnector[]>
  parentLinksMap: Record<number, ViewConnector[]>
  incomingLinks: IncomingViewConnector[]
}

export type ViewContentPayload = ViewContentLinks & {
  view: ViewTreeNode | null
  viewElements: PlacedElement[]
  connectors: Connector[]
  treeData: ViewTreeNode[]
}

export type CanvasStoreState = ViewEditorUiState & {
  view: ViewTreeNode | null | undefined
  viewElements: PlacedElement[]
  connectors: Connector[]
  nodes: RFNode[]
  edges: RFEdge[]
  linksMap: Record<number, ViewConnector[]>
  parentLinksMap: Record<number, ViewConnector[]>
  incomingLinks: IncomingViewConnector[]
  treeData: ViewTreeNode[]
  allElements: LibraryElement[]
  libraryRefresh: number

  setViewEditorUi: (patch: Partial<ViewEditorUiState>) => void
  setSnapToGrid: (snapToGrid: boolean) => void
  setSelectedElement: (selectedElement: LibraryElement | null) => void
  setSelectedConnector: (selectedConnector: Connector | null) => void
  setView: (view: ViewTreeNode | null | undefined) => void
  setViewElements: (next: StoreSetter<PlacedElement[]>) => void
  setConnectors: (next: StoreSetter<Connector[]>) => void
  setNodes: (next: StoreSetter<RFNode[]>) => void
  setEdges: (next: StoreSetter<RFEdge[]>) => void
  setLinksMap: (next: StoreSetter<Record<number, ViewConnector[]>>) => void
  setParentLinksMap: (next: StoreSetter<Record<number, ViewConnector[]>>) => void
  setIncomingLinks: (next: StoreSetter<IncomingViewConnector[]>) => void
  setTreeData: (next: StoreSetter<ViewTreeNode[]>) => void
  setAllElements: (next: StoreSetter<LibraryElement[]>) => void
  setLibraryRefresh: (next: StoreSetter<number>) => void
  resetCanvas: () => void
  hydrateViewContent: (payload: ViewContentPayload) => void
  updateElementPosition: (elementId: number, x: number, y: number) => void
  removeElementPlacement: (elementId: number) => void
  removeElementEverywhere: (elementId: number) => void
  mergeSavedElement: (saved: LibraryElement) => void
  upsertConnector: (connector: Connector) => void
  replaceConnector: (connector: Connector) => void
  removeConnector: (connectorId: number) => void
}

export const emptyViewEditorUiState: ViewEditorUiState = {
  viewId: null,
  canEdit: false,
  isOwner: false,
  isFreePlan: false,
  snapToGrid: false,
  selectedElement: null,
  selectedConnector: null,
}

function resolveSetter<T>(next: StoreSetter<T>, previous: T): T {
  return typeof next === 'function' ? (next as (previous: T) => T)(previous) : next
}

export function findViewByOwner(nodes: ViewTreeNode[], elementId: number): ViewTreeNode | null {
  for (const node of nodes) {
    if (node.owner_element_id !== null && Number(node.owner_element_id) === Number(elementId)) return node
    const found = findViewByOwner(node.children, elementId)
    if (found) return found
  }
  return null
}

export function findViewPath(nodes: ViewTreeNode[], targetId: number, path: ViewTreeNode[] = []): ViewTreeNode[] | null {
  for (const node of nodes) {
    if (node.id === targetId) return [...path, node]
    const found = findViewPath(node.children, targetId, [...path, node])
    if (found) return found
  }
  return null
}

export function buildViewContentLinks(tree: ViewTreeNode[], viewId: number, viewElements: PlacedElement[]): ViewContentLinks {
  const linksMap: Record<number, ViewConnector[]> = {}
  const parentLinksMap: Record<number, ViewConnector[]> = {}

  const viewPath = findViewPath(tree, viewId)
  const parentView = viewPath && viewPath.length > 1 ? viewPath[viewPath.length - 2] : null
  const currentViewInTree = viewPath ? viewPath[viewPath.length - 1] : null

  const incomingLinks: IncomingViewConnector[] = []
  if (parentView && currentViewInTree?.owner_element_id) {
    incomingLinks.push({
      id: 0,
      element_id: currentViewInTree.owner_element_id,
      element_name: 'Parent',
      from_view_id: parentView.id,
      from_view_name: parentView.name,
      to_view_id: viewId,
    })
  }

  for (const element of viewElements) {
    const childView = findViewByOwner(tree, element.element_id)
    if (childView) {
      linksMap[element.element_id] = [{
        id: 0,
        element_id: element.element_id,
        from_view_id: viewId,
        to_view_id: childView.id,
        to_view_name: childView.name,
        relation_type: 'child',
      }]
    }

    if (parentView) {
      parentLinksMap[element.element_id] = [{
        id: 0,
        element_id: element.element_id,
        from_view_id: parentView.id,
        to_view_id: parentView.id,
        to_view_name: parentView.name,
        relation_type: 'parent',
      }]
    }
  }

  return { linksMap, parentLinksMap, incomingLinks }
}

export function selectExistingElementIds(state: Pick<CanvasStoreState, 'viewElements'>): Set<number> {
  return new Set(state.viewElements.map((element) => element.element_id))
}

export function selectElementById(state: Pick<CanvasStoreState, 'viewElements'>, elementId: number): PlacedElement | undefined {
  return state.viewElements.find((element) => element.element_id === elementId)
}

export function selectConnectorById(state: Pick<CanvasStoreState, 'connectors'>, connectorId: number): Connector | undefined {
  return state.connectors.find((connector) => connector.id === connectorId)
}

export function updatePlacedElementPosition(elements: PlacedElement[], elementId: number, x: number, y: number): PlacedElement[] {
  let changed = false
  const next = elements.map((element) => {
    if (element.element_id !== elementId) return element
    if (element.position_x === x && element.position_y === y) return element
    changed = true
    return { ...element, position_x: x, position_y: y }
  })
  return changed ? next : elements
}

export function removePlacedElement(elements: PlacedElement[], elementId: number): PlacedElement[] {
  return elements.filter((element) => element.element_id !== elementId)
}

export function mergeSavedElementIntoPlacements(elements: PlacedElement[], saved: LibraryElement): PlacedElement[] {
  return elements.map((element) =>
    element.element_id === saved.id
      ? {
        ...element,
        name: saved.name,
        description: saved.description,
        kind: saved.kind,
        technology: saved.technology,
        url: saved.url,
        logo_url: saved.logo_url,
        technology_connectors: saved.technology_connectors,
        tags: saved.tags,
        repo: saved.repo,
        branch: saved.branch,
        file_path: saved.file_path,
        language: saved.language,
      }
      : element,
  )
}

export function upsertConnectorInList(connectors: Connector[], connector: Connector): Connector[] {
  const index = connectors.findIndex((candidate) => candidate.id === connector.id)
  if (index === -1) return [...connectors, connector]
  const next = connectors.slice()
  next[index] = connector
  return next
}

export function removeConnectorFromList(connectors: Connector[], connectorId: number): Connector[] {
  return connectors.filter((connector) => connector.id !== connectorId)
}

export const useStore = create<CanvasStoreState>((set) => ({
  ...emptyViewEditorUiState,
  view: undefined,
  viewElements: [],
  connectors: [],
  nodes: [],
  edges: [],
  linksMap: {},
  parentLinksMap: {},
  incomingLinks: [],
  treeData: [],
  allElements: [],
  libraryRefresh: 0,

  setViewEditorUi: (patch) => set((state) => ({ ...state, ...patch })),
  setSnapToGrid: (snapToGrid) => set({ snapToGrid }),
  setSelectedElement: (selectedElement) => set({ selectedElement }),
  setSelectedConnector: (selectedConnector) => set({ selectedConnector }),
  setView: (view) => set({ view }),
  setViewElements: (next) => set((state) => ({ viewElements: resolveSetter(next, state.viewElements) })),
  setConnectors: (next) => set((state) => ({ connectors: resolveSetter(next, state.connectors) })),
  setNodes: (next) => set((state) => ({ nodes: resolveSetter(next, state.nodes) })),
  setEdges: (next) => set((state) => ({ edges: resolveSetter(next, state.edges) })),
  setLinksMap: (next) => set((state) => ({ linksMap: resolveSetter(next, state.linksMap) })),
  setParentLinksMap: (next) => set((state) => ({ parentLinksMap: resolveSetter(next, state.parentLinksMap) })),
  setIncomingLinks: (next) => set((state) => ({ incomingLinks: resolveSetter(next, state.incomingLinks) })),
  setTreeData: (next) => set((state) => ({ treeData: resolveSetter(next, state.treeData) })),
  setAllElements: (next) => set((state) => ({ allElements: resolveSetter(next, state.allElements) })),
  setLibraryRefresh: (next) => set((state) => ({ libraryRefresh: resolveSetter(next, state.libraryRefresh) })),
  resetCanvas: () => set({ nodes: [], edges: [] }),
  hydrateViewContent: (payload) => set({
    view: payload.view,
    viewElements: payload.viewElements,
    connectors: payload.connectors,
    linksMap: payload.linksMap,
    parentLinksMap: payload.parentLinksMap,
    incomingLinks: payload.incomingLinks,
    treeData: payload.treeData,
  }),
  updateElementPosition: (elementId, x, y) => set((state) => ({
    viewElements: updatePlacedElementPosition(state.viewElements, elementId, x, y),
  })),
  removeElementPlacement: (elementId) => set((state) => ({
    viewElements: removePlacedElement(state.viewElements, elementId),
  })),
  removeElementEverywhere: (elementId) => set((state) => ({
    viewElements: removePlacedElement(state.viewElements, elementId),
    libraryRefresh: state.libraryRefresh + 1,
  })),
  mergeSavedElement: (saved) => set((state) => ({
    viewElements: mergeSavedElementIntoPlacements(state.viewElements, saved),
    libraryRefresh: state.libraryRefresh + 1,
  })),
  upsertConnector: (connector) => set((state) => ({
    connectors: upsertConnectorInList(state.connectors, connector),
  })),
  replaceConnector: (connector) => set((state) => ({
    connectors: upsertConnectorInList(state.connectors, connector),
  })),
  removeConnector: (connectorId) => set((state) => ({
    connectors: removeConnectorFromList(state.connectors, connectorId),
  })),
}))

export const canvasSelectors = {
  existingElementIds: selectExistingElementIds,
  elementById: selectElementById,
  connectorById: selectConnectorById,
}
