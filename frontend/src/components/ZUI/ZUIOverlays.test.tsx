import React from 'react'
import { act, create } from 'react-test-renderer'
import { afterEach, describe, expect, it, vi } from 'vitest'
import { ZUIBreadcrumb, ZUIHoverPopover } from './ZUIOverlays'
import type { PathItem } from './camera'
import type { HoveredItem, LayoutNode } from './types'

vi.mock('@chakra-ui/icons', () => ({
  ExternalLinkIcon: () => React.createElement('span'),
}))

vi.mock('@chakra-ui/react', async () => {
  const ReactModule = await import('react')
  const BoxLike = ReactModule.forwardRef<HTMLDivElement, { children?: React.ReactNode }>(function MockBox({ children, ...props }, ref) {
    return ReactModule.createElement('div', { ...props, ref }, children)
  })
  const ButtonLike = ({ children, ...props }: { children?: React.ReactNode }) => ReactModule.createElement('button', props, children)

  return {
    Badge: BoxLike,
    Box: BoxLike,
    Breadcrumb: BoxLike,
    BreadcrumbItem: BoxLike,
    BreadcrumbLink: BoxLike,
    Button: ButtonLike,
    Divider: BoxLike,
    HStack: BoxLike,
    Icon: BoxLike,
    Image: BoxLike,
    Popover: BoxLike,
    PopoverArrow: BoxLike,
    PopoverBody: BoxLike,
    PopoverContent: ReactModule.forwardRef<HTMLDivElement, { children?: React.ReactNode }>(function MockPopoverContent({ children, ...props }, ref) {
      // Match the motion component retaining its initial forwarded ref.
      const initialRef = ReactModule.useRef(ref)
      return ReactModule.createElement('div', { 'data-testid': 'zui-popover-content', ...props, ref: initialRef.current }, children)
    }),
    PopoverHeader: BoxLike,
    PopoverTrigger: BoxLike,
    Portal: BoxLike,
    Text: BoxLike,
    VStack: BoxLike,
  }
})

vi.mock('react-router-dom', () => ({
  Link: ({ children, ...props }: { children?: React.ReactNode }) => React.createElement('a', props, children),
}))

describe('ZUIBreadcrumb overflow', () => {
  let renderer: ReturnType<typeof create> | undefined

  afterEach(() => {
    act(() => renderer?.unmount())
    renderer = undefined
    vi.unstubAllGlobals()
  })

  it.each([false, true])('uses the available canvas width and follows canvas resizes (mobile: %s)', (isMobileLayout) => {
    let canvasWidth = 600
    const naturalWidth = 265.5
    let onResize!: () => void
    const canvas = { get clientWidth() { return canvasWidth } }
    const container = { parentElement: canvas }
    const measure = { getBoundingClientRect: () => ({ width: naturalWidth }) }
    vi.stubGlobal('window', { innerWidth: 1600 })
    const observe = vi.fn()
    vi.stubGlobal('ResizeObserver', class {
      constructor(callback: () => void) { onResize = callback }
      observe = observe
      disconnect() {}
    })
    const path: PathItem[] = ['Workspace', 'e-Commerce', 'Web App'].map((label, index) => ({
      id: String(index), label, type: index === 0 ? 'group' : 'node',
      absX: 0, absY: 0, absW: 100, absH: 100,
    }))
    act(() => {
      renderer = create(
        <ZUIBreadcrumb initialized isMobileLayout={isMobileLayout} currentPath={path} onZoomToPathItem={vi.fn()} />,
        { createNodeMock: (element) => element.props['data-testid'] === 'zui-breadcrumb' ? container : measure },
      )
    })
    expect(observe).toHaveBeenCalledWith(canvas)
    expect(renderer!.root.findAllByProps({ 'aria-label': 'Show full path' })).toHaveLength(0)
    act(() => { canvasWidth = 297; onResize() })
    expect(renderer!.root.findAllByProps({ 'aria-label': 'Show full path' }).length).toBeGreaterThan(0)
    act(() => { canvasWidth = 298; onResize() })
    expect(renderer!.root.findAllByProps({ 'aria-label': 'Show full path' })).toHaveLength(0)
  })

  it.each([false, true])('reserves space for a toolbar on the same row, including search expansion (mobile: %s)', (isMobileLayout) => {
    let toolbarLeft = 226
    let onResize!: () => void
    const canvas = { clientWidth: 904 }
    const top = isMobileLayout ? 66 : 16
    const container = {
      parentElement: canvas,
      getBoundingClientRect: () => ({ left: 16, top, bottom: top + 32 }),
    }
    const measure = { getBoundingClientRect: () => ({ width: 267.828125 }) }
    const toolbar = { getBoundingClientRect: () => ({ left: toolbarLeft, top: 12, bottom: 60 }) }
    const breadcrumbBoundaryRef = { current: toolbar as unknown as HTMLDivElement }
    const observe = vi.fn()
    vi.stubGlobal('window', {})
    vi.stubGlobal('ResizeObserver', class {
      constructor(callback: () => void) { onResize = callback }
      observe = observe
      disconnect() {}
    })
    const path: PathItem[] = ['Workspace', 'e-Commerce', 'Web App'].map((label, index) => ({
      id: String(index), label, type: index === 0 ? 'group' : 'node',
      absX: 0, absY: 0, absW: 100, absH: 100,
    }))
    act(() => {
      renderer = create(
        <ZUIBreadcrumb initialized isMobileLayout={isMobileLayout} currentPath={path} onZoomToPathItem={vi.fn()} breadcrumbBoundaryRef={breadcrumbBoundaryRef} />,
        { createNodeMock: (element) => element.props['data-testid'] === 'zui-breadcrumb' ? container : measure },
      )
    })
    const compact = () => renderer!.root.findAllByProps({ 'aria-label': 'Show full path' }).length > 0
    const maxWidth = () => renderer!.root.findAllByProps({ 'data-testid': 'zui-breadcrumb' })[0].props.maxW
    expect(observe).toHaveBeenCalledWith(toolbar)
    expect(compact()).toBe(!isMobileLayout)
    expect(maxWidth()).toBe(isMobileLayout ? '872px' : '194px')
    act(() => { toolbarLeft = 400; onResize() })
    expect(compact()).toBe(false)
    act(() => { toolbarLeft = 182; onResize() })
    expect(compact()).toBe(!isMobileLayout)
    expect(maxWidth()).toBe(isMobileLayout ? '872px' : '150px')
  })
})

function node(): LayoutNode {
  return {
    id: 'd1-o1',
    elementId: 1,
    diagramId: 1,
    worldX: 0,
    worldY: 0,
    worldW: 100,
    worldH: 80,
    label: 'Service',
    type: 'service',
    logoUrl: null,
    description: null,
    technology: null,
    tags: [],
    ancestorElementIds: [],
    pathElementIds: [1],
    children: [],
    childScale: 1,
    childOffsetX: 0,
    childOffsetY: 0,
    edgesOut: [],
  }
}

function hoveredNode(): HoveredItem {
  return {
    type: 'node',
    data: node(),
    absX: 0,
    absY: 0,
    absW: 100,
    absH: 80,
  }
}
function renderPopover(open: boolean, onHoverLock: (locked: boolean) => void) {
  return (
    <ZUIHoverPopover
      hoveredItem={open ? hoveredNode() : null}
      hoveredScreenRect={open ? { sx: 0, sy: 0, sw: 100, sh: 80 } : null}
      isHoveredItemFullyVisible={open}
      onHoverLock={onHoverLock}
    />
  )
}

describe('ZUIHoverPopover hover lock', () => {
  it('releases the lock when the popover closes without a mouseleave', () => {
    const onHoverLock = vi.fn()
    const renderer = create(renderPopover(true, onHoverLock))
    const content = renderer.root.findByProps({ 'data-testid': 'zui-hover-popover' })

    act(() => {
      content.props.onMouseEnter()
    })

    expect(onHoverLock).toHaveBeenLastCalledWith(true)

    act(() => {
      renderer.update(renderPopover(false, onHoverLock))
    })

    expect(onHoverLock).toHaveBeenLastCalledWith(false)
  })
})

describe('ZUIHoverPopover provenance', () => {
  it('renders no provenance row: provenance reads through tags now', () => {
    const item = hoveredNode()
    if (item.type !== 'node') throw new Error('expected a node hover target')
    item.data = {
      ...item.data,
      tags: [...item.data.tags, 'augmented'],
      changeOverlay: { change: 'modified', path: 'new.go', symbols: [], reason: 'direct' },
    }
    const renderer = create(
      <ZUIHoverPopover
        hoveredItem={item}
        hoveredScreenRect={{ sx: 0, sy: 0, sw: 100, sh: 80 }}
        isHoveredItemFullyVisible
        onHoverLock={() => {}}
      />,
    )
    expect(renderer.root.findAllByProps({ 'data-testid': 'zui-provenance-details' })).toHaveLength(0)
    expect(renderer.root.findAllByProps({ 'data-testid': 'zui-change-details' }).length).toBeGreaterThanOrEqual(1)
  })
})

describe('ZUIHoverPopover edge change', () => {
  it('badges edges new in the diff', () => {
    const renderer = create(
      <ZUIHoverPopover
        hoveredItem={{
          type: 'edge',
          data: {
            sourceId: 'a', targetId: 'b', label: 'calls', diagramId: 1,
            change: 'added',
          },
          absX: 0, absY: 0,
        }}
        hoveredScreenRect={{ sx: 0, sy: 0, sw: 100, sh: 80 }}
        isHoveredItemFullyVisible
        onHoverLock={() => {}}
      />,
    )
    expect(renderer.root.findAllByProps({ 'data-testid': 'zui-edge-change' }).length).toBeGreaterThanOrEqual(1)
  })

  it('shows no badge for unchanged edges', () => {
    const renderer = create(
      <ZUIHoverPopover
        hoveredItem={{
          type: 'edge',
          data: { sourceId: 'a', targetId: 'b', label: 'calls', diagramId: 1 },
          absX: 0, absY: 0,
        }}
        hoveredScreenRect={{ sx: 0, sy: 0, sw: 100, sh: 80 }}
        isHoveredItemFullyVisible
        onHoverLock={() => {}}
      />,
    )
    expect(renderer.root.findAllByProps({ 'data-testid': 'zui-edge-change' })).toHaveLength(0)
  })
})


describe('ZUIHoverPopover safe corridor bounds', () => {
  it('reports the mounted content when opening without replacing its forwarded ref', () => {
    const onPopoverElement = vi.fn()
    const onHoverLock = vi.fn()
    const element = {} as HTMLDivElement
    const render = (open: boolean) => (
      <ZUIHoverPopover
        hoveredItem={open ? hoveredNode() : null}
        hoveredScreenRect={open ? { sx: 0, sy: 0, sw: 100, sh: 80 } : null}
        isHoveredItemFullyVisible={open}
        onHoverLock={onHoverLock}
        onPopoverElement={onPopoverElement}
      />
    )
    const renderer = create(render(false), { createNodeMock: () => element })
    expect(onPopoverElement).toHaveBeenLastCalledWith(null)
    act(() => { renderer.update(render(true)) })
    expect(onPopoverElement).toHaveBeenLastCalledWith(element)
    act(() => { renderer.update(render(false)) })
    expect(onPopoverElement).toHaveBeenLastCalledWith(null)
    act(() => { renderer.update(render(true)) })
    expect(onPopoverElement).toHaveBeenLastCalledWith(element)
    act(() => { renderer.unmount() })
    expect(onPopoverElement).toHaveBeenLastCalledWith(null)
  })
})
