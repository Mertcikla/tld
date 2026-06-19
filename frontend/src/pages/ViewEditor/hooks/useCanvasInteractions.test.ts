import { afterEach, describe, expect, it } from 'vitest'
import type { Node as RFNode, NodeChange } from 'reactflow'
import type { Connector } from '../../../types'
import {
  PENDING_ELEMENT_NODE_ID,
  accelerateViewEditorPinchZoomFactor,
  applyPendingElementNodeChanges,
  getDraggedElementNodes,
  getDraggedSelectionElementNodes,
  getConnectorDeletionTarget,
  getPlacementPositionTimerKeys,
  pendingElementPositionFromFlowPoint,
  resolveConnectorDragAttachHandles,
  resolveConnectorDropTarget,
  shouldDisplayConnectorDragPlaceholder,
  shouldZoomViewEditorWheel,
  zoomViewportAroundClientPoint,
  type PendingElementState,
} from './useCanvasInteractions'
import type { WheelDeltaLike } from '../../../utils/wheel'

const connector = (id: number): Connector => ({
  id,
  view_id: 1,
  source_element_id: 10,
  target_element_id: 20,
  label: null,
  description: null,
  relationship: null,
  direction: 'forward',
  style: 'bezier',
  url: null,
  source_handle: 'right',
  target_handle: 'left',
  tags: [],
  created_at: '2024-01-01',
  updated_at: '2024-01-01',
})

const originalElement = globalThis.Element
const originalDocument = globalThis.document
const hadOriginalElement = 'Element' in globalThis
const hadOriginalDocument = 'document' in globalThis

class FakeElement {
  id = ''

  constructor(
    private readonly closestResults: Record<string, FakeElement | null>,
    private readonly attrs: Record<string, string> = {},
  ) { }

  closest(selector: string) {
    return this.closestResults[selector] ?? null
  }

  getAttribute(name: string) {
    return this.attrs[name] ?? null
  }
}

function node(id: string, options: Partial<RFNode> = {}): RFNode {
  return { id, type: 'elementNode', position: { x: 0, y: 0 }, data: {}, ...options } as RFNode
}

function wheel(overrides: Partial<WheelDeltaLike>): WheelDeltaLike {
  return {
    deltaX: 0,
    deltaY: 0,
    deltaMode: 0,
    ctrlKey: false,
    ...overrides,
  }
}

function installPointHitTest(elements: FakeElement[]) {
  Object.defineProperty(globalThis, 'Element', {
    configurable: true,
    value: FakeElement,
  })
  Object.defineProperty(globalThis, 'document', {
    configurable: true,
    value: {
      elementsFromPoint: () => elements,
      elementFromPoint: () => elements[0] ?? null,
    },
  })
}

afterEach(() => {
  if (hadOriginalElement) {
    Object.defineProperty(globalThis, 'Element', {
      configurable: true,
      value: originalElement,
    })
  } else {
    Reflect.deleteProperty(globalThis, 'Element')
  }

  if (hadOriginalDocument) {
    Object.defineProperty(globalThis, 'document', {
      configurable: true,
      value: originalDocument,
    })
  } else {
    Reflect.deleteProperty(globalThis, 'document')
  }
})

describe('getConnectorDeletionTarget', () => {
  it('returns the selected connector id', () => {
    expect(getConnectorDeletionTarget(connector(7))).toBe(7)
  })

  it('returns null when nothing is selected', () => {
    expect(getConnectorDeletionTarget(null)).toBeNull()
  })
})

describe('pending element node state', () => {
  const pending = (): PendingElementState => ({
    id: PENDING_ELEMENT_NODE_ID,
    position: { x: 100, y: 200 },
    mode: 'add',
    sourceElementIds: [],
    sourceHandle: null,
  })

  it('uses top-left placement from the requested flow point', () => {
    expect(pendingElementPositionFromFlowPoint(320, 180)).toEqual({ x: 220, y: 140 })
  })

  it('tracks pending node drag position without persisting it', () => {
    const changes: NodeChange[] = [{
      id: PENDING_ELEMENT_NODE_ID,
      type: 'position',
      position: { x: 150, y: 240 },
      dragging: true,
    }]

    expect(applyPendingElementNodeChanges(pending(), changes)).toEqual({
      ...pending(),
      position: { x: 150, y: 240 },
      dragging: true,
    })
  })

  it('cancels pending node state when the node is removed', () => {
    expect(applyPendingElementNodeChanges(pending(), [{ id: PENDING_ELEMENT_NODE_ID, type: 'remove' }])).toBeNull()
  })

  it('keeps preview metadata while tracking position updates', () => {
    const previewPending = { ...pending(), preview: true }
    const changes: NodeChange[] = [{
      id: PENDING_ELEMENT_NODE_ID,
      type: 'position',
      position: { x: 180, y: 260 },
      dragging: false,
    }]

    expect(applyPendingElementNodeChanges(previewPending, changes)).toEqual({
      ...previewPending,
      position: { x: 180, y: 260 },
      dragging: false,
    })
  })
})

describe('dragged element node resolution', () => {
  it('uses React Flow drag payload positions before stale selected refs', () => {
    const primary = node('1', { selected: true, position: { x: 0, y: 0 } })
    const staleSelection = [
      primary,
      node('2', { selected: true, position: { x: 100, y: 0 } }),
    ]
    const draggedNodes = [
      node('1', { selected: true, position: { x: 40, y: 20 } }),
      node('2', { selected: true, position: { x: 140, y: 20 } }),
    ]

    expect(getDraggedElementNodes(primary, draggedNodes, staleSelection).map((dragged) => ({
      id: dragged.id,
      position: dragged.position,
    }))).toEqual([
      { id: '1', position: { x: 40, y: 20 } },
      { id: '2', position: { x: 140, y: 20 } },
    ])
  })

  it('includes selected fallback nodes when React Flow drag payload only contains the primary node', () => {
    const primary = node('1', { selected: true, position: { x: 40, y: 20 } })
    const latestSelection = [
      node('1', { selected: true, position: { x: 35, y: 15 } }),
      node('2', { selected: true, position: { x: 140, y: 20 } }),
    ]

    expect(getDraggedElementNodes(primary, [primary], latestSelection).map((dragged) => ({
      id: dragged.id,
      position: dragged.position,
    }))).toEqual([
      { id: '1', position: { x: 40, y: 20 } },
      { id: '2', position: { x: 140, y: 20 } },
    ])
  })

  it('falls back to selected element refs when React Flow gives no drag payload', () => {
    const primary = node('1', { selected: true })
    const fallbackNodes = [
      primary,
      node('2', { selected: true }),
      node('ctx:3', { selected: true, type: 'contextNode' }),
    ]

    expect(getDraggedElementNodes(primary, [], fallbackNodes).map((dragged) => dragged.id)).toEqual(['1', '2'])
  })

  it('resolves selected nodes for React Flow selection drags', () => {
    const latestSelection = [
      node('1', { selected: true, position: { x: 0, y: 0 } }),
      node('2', { selected: true, position: { x: 100, y: 0 } }),
      node('ctx:3', { selected: true, type: 'contextNode' }),
      node('4', { selected: false, position: { x: 300, y: 0 } }),
    ]
    const draggedNodes = [
      node('1', { selected: true, position: { x: 30, y: 25 } }),
      node('2', { selected: true, position: { x: 130, y: 25 } }),
    ]

    expect(getDraggedSelectionElementNodes(draggedNodes, latestSelection).map((dragged) => ({
      id: dragged.id,
      position: dragged.position,
    }))).toEqual([
      { id: '1', position: { x: 30, y: 25 } },
      { id: '2', position: { x: 130, y: 25 } },
    ])
  })
})

describe('placement position debounce timer keys', () => {
  it('keys single placement moves by the element so follow-up nudges debounce', () => {
    expect(getPlacementPositionTimerKeys('1', [1])).toEqual(['1'])
    expect(getPlacementPositionTimerKeys('selection:1', [1])).toEqual(['selection:1', '1'])
  })

  it('keys multi-placement moves by the whole batch instead of each element', () => {
    expect(getPlacementPositionTimerKeys('1', [1, 2])).toEqual(['batch:1:2'])
    expect(getPlacementPositionTimerKeys('2', [2, 1])).toEqual(['batch:1:2'])
    expect(getPlacementPositionTimerKeys('selection:1:2', [1, 2])).toEqual(['batch:1:2'])
  })
})

describe('connector drag placeholder visibility', () => {
  it('shows the placeholder over empty canvas', () => {
    expect(shouldDisplayConnectorDragPlaceholder(null)).toBe(true)
  })

  it('shows the placeholder over the pending node itself to avoid self-flicker', () => {
    expect(shouldDisplayConnectorDragPlaceholder({ nodeId: PENDING_ELEMENT_NODE_ID, isHandle: false })).toBe(true)
  })

  it('hides the placeholder over a node body or handle', () => {
    expect(shouldDisplayConnectorDragPlaceholder({ nodeId: '12', isHandle: false })).toBe(false)
    expect(shouldDisplayConnectorDragPlaceholder({ nodeId: '12', isHandle: true })).toBe(false)
    expect(shouldDisplayConnectorDragPlaceholder({ isHandle: true })).toBe(false)
  })
})

describe('connector drag attach handle resolution', () => {
  it('preserves explicit handle sides instead of choosing closest geometry', () => {
    expect(resolveConnectorDragAttachHandles('bottom-2', 'right-4')).toEqual({
      sourceHandle: 'bottom',
      targetHandle: 'right',
    })
  })

  it('falls back to default sides when a drag attach has no explicit target handle', () => {
    expect(resolveConnectorDragAttachHandles('top-2', null)).toEqual({
      sourceHandle: 'top',
      targetHandle: 'left',
    })
  })
})

describe('viewport zoom helpers', () => {
  it('zooms vertical smooth wheel input before React Flow can pan it', () => {
    expect(shouldZoomViewEditorWheel(wheel({ deltaY: 6 }), false)).toBe(true)
    expect(shouldZoomViewEditorWheel(wheel({ deltaY: 20.5 }), false)).toBe(true)
  })

  it('keeps two-axis wheel gestures available for canvas panning', () => {
    expect(shouldZoomViewEditorWheel(wheel({ deltaX: 8, deltaY: 20 }), false)).toBe(false)
    expect(shouldZoomViewEditorWheel(wheel({ deltaY: 6 }), true)).toBe(false)
  })

  it('lets React Flow handle ctrl-wheel pinch gestures such as Firefox trackpad pinch', () => {
    expect(shouldZoomViewEditorWheel(wheel({ ctrlKey: true, deltaY: 6 }), false)).toBe(false)
    expect(shouldZoomViewEditorWheel(wheel({ ctrlKey: true, deltaY: 1, deltaMode: 1 }), false)).toBe(false)
  })

  it('doubles incremental pinch zoom distance from neutral', () => {
    expect(accelerateViewEditorPinchZoomFactor(1)).toBe(1)
    expect(accelerateViewEditorPinchZoomFactor(1.04)).toBeCloseTo(1.08)
    expect(accelerateViewEditorPinchZoomFactor(0.96)).toBeCloseTo(0.92)
  })

  it('keeps the flow point under the client point fixed while zooming', () => {
    const viewport = { x: 80, y: 40, zoom: 2 }
    const rect = { left: 10, top: 20 }
    const point = { clientX: 310, clientY: 220 }
    const beforeFlow = {
      x: (point.clientX - rect.left - viewport.x) / viewport.zoom,
      y: (point.clientY - rect.top - viewport.y) / viewport.zoom,
    }

    const next = zoomViewportAroundClientPoint(viewport, rect, point, 1.5, 0.1, 4)
    const afterFlow = {
      x: (point.clientX - rect.left - next.x) / next.zoom,
      y: (point.clientY - rect.top - next.y) / next.zoom,
    }

    expect(next.zoom).toBe(3)
    expect(afterFlow.x).toBeCloseTo(beforeFlow.x)
    expect(afterFlow.y).toBeCloseTo(beforeFlow.y)
  })

  it('clamps zoom while preserving the anchored client point', () => {
    const next = zoomViewportAroundClientPoint(
      { x: 0, y: 0, zoom: 3 },
      { left: 0, top: 0 },
      { clientX: 300, clientY: 150 },
      2,
      0.5,
      4,
    )

    expect(next.zoom).toBe(4)
    expect((300 - next.x) / next.zoom).toBeCloseTo(100)
    expect((150 - next.y) / next.zoom).toBeCloseTo(50)
  })
})

describe('connector drop target resolution', () => {
  it('uses coordinate hit-testing before event target fallback', () => {
    const sourceNodeElement = new FakeElement({}, { 'data-id': '1' })
    const sourceEventTarget = new FakeElement({ '.react-flow__node': sourceNodeElement })
    const canvasElement = new FakeElement({})
    installPointHitTest([canvasElement])

    expect(resolveConnectorDropTarget(sourceEventTarget as unknown as EventTarget, 120, 160, [node('1')])).toEqual({
      droppedNode: null,
      droppedHandleId: null,
    })
  })

  it('resolves the handle under the release point', () => {
    const targetNodeElement = new FakeElement({}, { 'data-id': '2' })
    const targetHandleElement = new FakeElement({ '.react-flow__node': targetNodeElement }, { 'data-handleid': 'left-2' })
    const releaseTarget = new FakeElement({
      '.react-flow__node': targetNodeElement,
      '.react-flow__handle': targetHandleElement,
    })
    installPointHitTest([releaseTarget])

    expect(resolveConnectorDropTarget(null, 120, 160, [node('1'), node('2')])).toEqual({
      droppedNode: node('2'),
      droppedHandleId: 'left-2',
    })
  })
})
