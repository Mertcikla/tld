import React from 'react'
import { act, create } from 'react-test-renderer'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import {
  PANEL_COLLAPSE_SNAP,
  PANEL_OVERLAY_SNAP,
  PANEL_RAIL_WIDTH,
  resizableColumnZone,
  useResizableColumn,
  type ResizableColumn,
  type ResizableColumnOptions,
} from './useResizableColumn'

type Listener = (event: unknown) => void

let listeners: Map<string, Set<Listener>>
let storage: Map<string, string>
let panel: ResizableColumn

function container(clientWidth: number): { current: HTMLElement | null } {
  return { current: { clientWidth } as HTMLElement }
}

function stubEnvironment() {
  listeners = new Map()
  storage = new Map()
  const addEventListener = (type: string, listener: Listener) => {
    const bucket = listeners.get(type) ?? new Set<Listener>()
    bucket.add(listener)
    listeners.set(type, bucket)
  }
  const removeEventListener = (type: string, listener: Listener) => {
    listeners.get(type)?.delete(listener)
  }
  vi.stubGlobal('window', {
    addEventListener,
    removeEventListener,
    localStorage: {
      getItem: (key: string) => storage.get(key) ?? null,
      setItem: (key: string, value: string) => {
        storage.set(key, value)
      },
    },
  })
  vi.stubGlobal('document', { body: { style: {} as Record<string, string> } })
}

function dispatch(type: string, event: Record<string, unknown>) {
  for (const listener of listeners.get(type) ?? []) listener(event)
}

function Harness({ options }: { options: ResizableColumnOptions }) {
  panel = useResizableColumn(options)
  return null
}

async function mount(options: ResizableColumnOptions) {
  await act(async () => {
    create(<Harness options={options} />)
  })
}

function options(overrides: Partial<ResizableColumnOptions> = {}): ResizableColumnOptions {
  return {
    storageKey: 'test:panel',
    defaultWidth: 380,
    collapseBelow: PANEL_COLLAPSE_SNAP,
    overlayAbove: PANEL_OVERLAY_SNAP,
    ...overrides,
  }
}

async function dragTo(clientX: number) {
  await act(async () => {
    dispatch('pointermove', { clientX, preventDefault: () => {} })
  })
}

async function beginDrag(clientX: number, pointerType = 'mouse', button = 0) {
  await act(async () => {
    panel.startResize({ clientX, pointerType, button, preventDefault: () => {} } as never)
  })
}

async function endDrag() {
  await act(async () => {
    dispatch('pointerup', {})
  })
}

describe('useResizableColumn', () => {
  beforeEach(() => {
    stubEnvironment()
  })

  afterEach(() => {
    vi.unstubAllGlobals()
  })

  describe('resizableColumnZone', () => {
    it('snaps under the collapse threshold into the collapsed zone', () => {
      expect(resizableColumnZone(0, PANEL_COLLAPSE_SNAP)).toBe('collapsed')
      expect(resizableColumnZone(PANEL_COLLAPSE_SNAP - 1, PANEL_COLLAPSE_SNAP)).toBe('collapsed')
    })

    it('stays docked between the two thresholds', () => {
      expect(resizableColumnZone(PANEL_COLLAPSE_SNAP, PANEL_COLLAPSE_SNAP, PANEL_OVERLAY_SNAP)).toBe('inline')
      expect(resizableColumnZone(380, PANEL_COLLAPSE_SNAP, PANEL_OVERLAY_SNAP)).toBe('inline')
      expect(resizableColumnZone(PANEL_OVERLAY_SNAP, PANEL_COLLAPSE_SNAP, PANEL_OVERLAY_SNAP)).toBe('inline')
    })

    it('snaps over the overlay threshold into the overlay zone', () => {
      expect(resizableColumnZone(PANEL_OVERLAY_SNAP + 1, PANEL_COLLAPSE_SNAP, PANEL_OVERLAY_SNAP)).toBe('overlay')
      expect(resizableColumnZone(900, PANEL_COLLAPSE_SNAP, PANEL_OVERLAY_SNAP)).toBe('overlay')
    })

    it('never enters the overlay zone when no threshold is given', () => {
      expect(resizableColumnZone(900, PANEL_COLLAPSE_SNAP)).toBe('inline')
    })
  })

  it('starts at the default width when nothing is persisted', async () => {
    await mount(options())
    expect(panel.width).toBe(380)
    expect(panel.renderedWidth).toBe(380)
    expect(panel.zone).toBe('inline')
  })

  it('restores the persisted width', async () => {
    storage.set('test:panel', '420')
    await mount(options())
    expect(panel.width).toBe(420)
  })

  it('persists the width on change', async () => {
    await mount(options())
    await act(async () => {
      panel.collapse()
    })
    expect(storage.get('test:panel')).toBe(String(PANEL_RAIL_WIDTH))
  })

  describe('dragging', () => {
    it('grows a right-docked column when the pointer moves left', async () => {
      await mount(options())
      await beginDrag(500)
      await dragTo(400)
      expect(panel.width).toBe(480)
      await endDrag()
    })

    it('grows a left-docked column when the pointer moves right', async () => {
      await mount(options({ side: 'start' }))
      await beginDrag(300)
      await dragTo(400)
      expect(panel.width).toBe(480)
      await endDrag()
    })

    it('ignores non-primary mouse buttons', async () => {
      await mount(options())
      await beginDrag(500, 'mouse', 1)
      await dragTo(400)
      expect(panel.isResizing).toBe(false)
      expect(panel.width).toBe(380)
    })

    it('stops resizing on pointerup', async () => {
      await mount(options())
      await beginDrag(500)
      expect(panel.isResizing).toBe(true)
      await endDrag()
      expect(panel.isResizing).toBe(false)
      await dragTo(300)
      expect(panel.width).toBe(380)
    })

    it('clamps to the rail width and collapses below the collapse threshold', async () => {
      await mount(options())
      await beginDrag(500)
      await dragTo(900)
      expect(panel.isCollapsed).toBe(true)
      expect(panel.width).toBe(PANEL_RAIL_WIDTH)
      expect(panel.renderedWidth).toBe(PANEL_RAIL_WIDTH)
    })

    it('snaps into the overlay zone past the overlay threshold', async () => {
      await mount(options())
      await beginDrag(500)
      await dragTo(500 - (PANEL_OVERLAY_SNAP + 20 - 380))
      expect(panel.isOverlay).toBe(true)
      expect(panel.width).toBe(PANEL_OVERLAY_SNAP + 20)
    })

    it('picks up from the rendered rail width when dragging out of the collapsed zone', async () => {
      await mount(options())
      await act(async () => {
        panel.collapse()
      })
      await beginDrag(500)
      await dragTo(500 - 30)
      expect(panel.isCollapsed).toBe(true)
      expect(panel.width).toBe(PANEL_RAIL_WIDTH + 30)
      await dragTo(500 - 200)
      expect(panel.isCollapsed).toBe(false)
      expect(panel.width).toBe(PANEL_RAIL_WIDTH + 200)
    })

    it('clamps to the maximum resolved from the measured container', async () => {
      const containerRef = container(600)
      await mount(options({ containerRef, maxWidth: (containerWidth) => containerWidth }))
      await beginDrag(500)
      await dragTo(0)
      expect(panel.width).toBe(600)
    })

    it('re-clamps when the window resizes', async () => {
      const containerRef = container(1000)
      await mount(options({ containerRef, maxWidth: (containerWidth) => containerWidth }))
      await beginDrag(500)
      await dragTo(-200)
      expect(panel.width).toBe(1000)
      await endDrag()

      containerRef.current = { clientWidth: 400 } as HTMLElement
      await act(async () => {
        dispatch('resize', {})
      })
      expect(panel.width).toBe(400)
    })
  })

  it('collapses to the rail and expands back to the default width', async () => {
    await mount(options())
    await act(async () => {
      panel.collapse()
    })
    expect(panel.isCollapsed).toBe(true)
    expect(panel.renderedWidth).toBe(PANEL_RAIL_WIDTH)

    await act(async () => {
      panel.expand()
    })
    expect(panel.isCollapsed).toBe(false)
    expect(panel.width).toBe(380)
  })

  it('keeps an inline width when expanding and keeps a docked width when docking', async () => {
    await mount(options())
    await beginDrag(500)
    await dragTo(420)
    expect(panel.width).toBe(460)
    await endDrag()

    await act(async () => {
      panel.expand()
    })
    expect(panel.width).toBe(460)

    await act(async () => {
      panel.dock()
    })
    expect(panel.width).toBe(460)
  })

  it('docks an overlaying column back to the default width', async () => {
    await mount(options())
    await beginDrag(500)
    await dragTo(500 - (PANEL_OVERLAY_SNAP + 100 - 380))
    expect(panel.isOverlay).toBe(true)
    await endDrag()

    await act(async () => {
      panel.dock()
    })
    expect(panel.isOverlay).toBe(false)
    expect(panel.width).toBe(380)
  })
})