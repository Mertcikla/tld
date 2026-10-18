import { act, create, type ReactTestRenderer } from 'react-test-renderer'
import { afterEach, describe, expect, it, vi } from 'vitest'
import { useZUICamera, type ZUICameraController } from './useZUICamera'
import type { DiagramGroupLayout, LayoutNode } from './types'

const leaf: LayoutNode = {
  id: 'leaf', elementId: 2, diagramId: 7,
  worldX: 0, worldY: 0, worldW: 200, worldH: 100,
  label: 'App Shell', type: 'component', logoUrl: null, description: null,
  technology: null, tags: [], ancestorElementIds: [], pathElementIds: [],
  children: [], childScale: 1, childOffsetX: 0, childOffsetY: 0, edgesOut: [],
}
const checkoutFlow: LayoutNode = {
  ...leaf, id: 'checkout-flow', elementId: 15, diagramId: 7,
  worldX: 20, worldY: 30, label: 'Checkout Flow', linkedDiagramId: 3,
  children: [leaf],
}
const webApp: LayoutNode = {
  ...leaf, id: 'web-app', elementId: 57, diagramId: 6,
  worldX: 80, worldY: 40, label: 'Web App', linkedDiagramId: 7,
  children: [leaf, checkoutFlow], childScale: 0.5,
}
const commerce: LayoutNode = {
  ...leaf, id: 'commerce', elementId: 53, diagramId: 1,
  worldX: 100, worldY: 50, label: 'e-Commerce', linkedDiagramId: 6,
  children: [webApp], childScale: 0.5, childOffsetX: 20, childOffsetY: 10,
}
const workspace: DiagramGroupLayout = {
  diagramId: 1, label: 'Workspace', description: null, level: 0, levelLabel: null,
  worldX: 0, worldY: 0, worldW: 1200, worldH: 800,
  diagramW: 1200, diagramH: 800, diagramX: 0, diagramY: 0,
  nodes: [commerce], edges: [],
}

let renderer: ReactTestRenderer | undefined
let camera: ZUICameraController
const setViewState = vi.fn()

function Consumer({ detailViewId, overviewViewId }: { detailViewId?: number; overviewViewId?: number }) {
  camera = useZUICamera({
    containerRef: { current: { offsetWidth: 1000, offsetHeight: 600 } as HTMLDivElement },
    layoutGroups: [workspace],
    layoutBBox: { minX: 0, minY: 0, maxX: 1200, maxY: 800 },
    maxZoom: 10,
    isMobileLayout: false,
    initialCameraFrame: { profile: 'detail-to-overview', progress: 0, detailViewId, overviewViewId },
    fitView: vi.fn(),
    setViewState,
    viewStateRef: { current: { x: 0, y: 0, zoom: 1 } },
    setHoveredItem: vi.fn(),
  })
  return null
}

afterEach(() => {
  act(() => renderer?.unmount())
  renderer = undefined
  setViewState.mockClear()
})

describe('scripted ZUI camera target', () => {
  it('zooms from nested Checkout Flow out to Web App using their absolute scene bounds', () => {
    act(() => { renderer = create(<Consumer detailViewId={3} overviewViewId={7} />) })
    act(() => camera.fitInitialView(1000, 600))
    const initialViewport = setViewState.mock.lastCall![0]
    expect(initialViewport.x).toBeCloseTo(-908)
    expect(initialViewport.y).toBeCloseTo(-448)
    expect(initialViewport.zoom).toBeCloseTo(8.8)
    act(() => camera.setCameraFrame({ profile: 'detail-to-overview', progress: 1, detailViewId: 3, overviewViewId: 7 }))
    const finalViewport = setViewState.mock.lastCall![0]
    expect(finalViewport.x).toBeCloseTo(-652)
    expect(finalViewport.y).toBeCloseTo(-276)
    expect(finalViewport.zoom).toBeCloseTo(6.4)
    expect(finalViewport.zoom).toBeLessThan(initialViewport.zoom)
  })

  it('starts at the requested Web App within Workspace → e-Commerce, accounting for nested scale and offsets', () => {
    act(() => { renderer = create(<Consumer detailViewId={7} />) })
    act(() => camera.fitInitialView(1000, 600))
    const viewport = setViewState.mock.lastCall![0]
    expect(viewport.x).toBeCloseTo(-292)
    expect(viewport.y).toBeCloseTo(-96)
    expect(viewport.zoom).toBeCloseTo(4.4)
  })

  it('keeps the pinned target during scrolling and finishes at the workspace overview', () => {
    act(() => { renderer = create(<Consumer detailViewId={7} />) })
    act(() => camera.setCameraFrame({ profile: 'detail-to-overview', progress: 0.5, detailViewId: 7 }))
    expect(setViewState.mock.lastCall![0].zoom).toBeCloseTo(0.725)
    act(() => camera.setCameraFrame({ profile: 'detail-to-overview', progress: 1, detailViewId: 7 }))
    expect(setViewState.mock.lastCall![0]).toEqual({ x: 212, y: 108, zoom: 0.48 })
  })

  it.each([undefined, 999])('preserves automatic framing when the requested target is %s', (detailViewId) => {
    act(() => { renderer = create(<Consumer detailViewId={detailViewId} />) })
    act(() => camera.fitInitialView(1000, 600))
    const viewport = setViewState.mock.lastCall![0]
    expect(viewport.x).toBeCloseTo(60)
    expect(viewport.y).toBeCloseTo(80)
    expect(viewport.zoom).toBeCloseTo(2.2)
  })
})
