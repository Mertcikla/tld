import React from 'react'
import { act, create } from 'react-test-renderer'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import type { RepositoryImpact as ImpactDiagram } from '../api/client'
import { PANEL_COLLAPSE_SNAP, PANEL_OVERLAY_SNAP, PANEL_RAIL_WIDTH } from '../hooks/useResizableColumn'
import RepositoryChangeCanvas from './RepositoryChangeCanvas'
import RepositoryChangeMenu from './RepositoryChangeMenu'

const storageKey = 'tld:repositories:mermaidPaneWidth'

vi.mock('./ZUI', () => ({ ZUICanvas: () => null }))
vi.mock('./RepositoryChangeMermaid', () => ({
  default: (props: Record<string, unknown>) =>
    React.createElement('div', {
      'data-testid': 'mock-mermaid',
      'data-collapsed': String(!!props.collapsed),
      'data-overlay': String(!!props.overlay),
    }),
}))
vi.mock('../api/client', () => ({
  api: { repositories: { impactScene: vi.fn(() => new Promise(() => {})) } },
}))

vi.mock('@chakra-ui/react', async () => {
  const ReactModule = await import('react')
  // forwardRef so the split row ref resolves, which is what bounds the max width
  const BoxLike = ReactModule.forwardRef(({ children, ...props }: { children?: React.ReactNode }, ref: React.ForwardedRef<unknown>) => {
    if (typeof ref === 'object' && ref) ref.current = { clientWidth: 1000 }
    return ReactModule.createElement('div', props, children)
  })
  return {
    Box: BoxLike,
    Button: BoxLike,
    Flex: BoxLike,
    HStack: BoxLike,
    Text: BoxLike,
    Tooltip: ({ children }: { children?: React.ReactNode }) => ReactModule.createElement('div', null, children),
    VStack: BoxLike,
  }
})

const diagram = {
  repositoryId: 'repo-1',
  comparisonKey: 'pair',
  version: 'v1',
  viewId: 9,
  nodes: [],
  edges: [],
  diff: { fromSnapshotId: 'snap-0', toSnapshotId: 'snap-1', sources: [], facts: { added: 0, removed: 0, modified: 0 }, edgeFacts: { added: 0, removed: 0, modified: 0 } },
} as unknown as ImpactDiagram

function stubEnvironment(storedWidth?: string) {
  const values = new Map<string, string>()
  if (storedWidth != null) values.set(storageKey, storedWidth)
  vi.stubGlobal('window', {
    addEventListener: vi.fn(),
    removeEventListener: vi.fn(),
    localStorage: {
      getItem: (key: string) => values.get(key) ?? null,
      setItem: (key: string, value: string) => {
        values.set(key, value)
      },
    },
  })
  vi.stubGlobal('document', { body: { style: {} as Record<string, string> } })
}

/** Only rendered DOM nodes, ignoring the mocked Chakra components themselves. */
function hostNodes(renderer: ReturnType<typeof create>, testId: string) {
  return renderer.root.findAll((node) => node.type === 'div' && node.props['data-testid'] === testId)
}

/**
 * The `data-testid` of every rendered div between `node` and the root. The
 * handle relies on `align-self: stretch`, so it must sit directly in the flex
 * row: any extra div in this chain means the drag target has no height.
 */
function divTestIdAncestry(node: { type: unknown; props: Record<string, unknown>; parent: unknown }) {
  const chain: (string | null)[] = []
  let current = node as { type: unknown; props: Record<string, unknown>; parent: unknown } | null
  while (current) {
    if (current.type === 'div') {
      const testId = current.props['data-testid']
      chain.push(typeof testId === 'string' ? testId : null)
    }
    current = current.parent as typeof current
  }
  return chain
}

async function renderCanvas() {
  let renderer!: ReturnType<typeof create>
  await act(async () => {
    renderer = create(<RepositoryChangeCanvas diagram={diagram} selectedPath="" />)
  })
  return renderer
}

describe('RepositoryChangeCanvas mermaid pane', () => {
  beforeEach(() => {
    vi.clearAllMocks()
  })

  afterEach(() => {
    vi.unstubAllGlobals()
  })

  it('renders the grounded diagram with view-mode toggle and legend', async () => {
    stubEnvironment()
    let renderer!: ReturnType<typeof create>
    await act(async () => {
      renderer = create(<RepositoryChangeCanvas diagram={diagram} selectedPath="" />)
    })
    const menu = () => renderer.root.findByType(RepositoryChangeMenu)
    expect(menu().props.viewMode).toBe('standard')
    expect(hostNodes(renderer, 'repositories-view-mode-standard')).toHaveLength(1)
    expect(hostNodes(renderer, 'repositories-radius')).toHaveLength(0)

    await act(async () => { menu().props.onViewModeChange('plain') })
    expect(menu().props.viewMode).toBe('plain')
    renderer.unmount()
  })

  it('docks the change diagram beside the canvas with a resize handle', async () => {
    stubEnvironment()
    const renderer = await renderCanvas()

    const handle = hostNodes(renderer, 'repository-change-mermaid-resize')[0]
    expect(handle).toBeDefined()
    expect(handle.props.alignSelf).toBe('stretch')
    expect(divTestIdAncestry(handle).slice(0, 2)).toEqual([
      'repository-change-mermaid-resize',
      'repository-change-split',
    ])

    const slot = hostNodes(renderer, 'repository-change-mermaid-slot')[0]
    expect(slot.props.w).toEqual({ base: 'full', lg: '380px' })
    expect(slot.props.position).toBe('relative')
    expect(hostNodes(renderer, 'mock-mermaid')[0].props['data-collapsed']).toBe('false')
    expect(hostNodes(renderer, 'mock-mermaid')[0].props['data-overlay']).toBe('false')
  })

  it('keeps the resize handle above the canvas-covering pane', async () => {
    stubEnvironment(String(PANEL_OVERLAY_SNAP + 50))
    const renderer = await renderCanvas()

    const handle = hostNodes(renderer, 'repository-change-mermaid-resize')[0]
    const slot = hostNodes(renderer, 'repository-change-mermaid-slot')[0]
    expect(handle.props.zIndex).toBe(40)
    expect(handle.props.zIndex).toBeGreaterThan(slot.props.zIndex)
  })

  it('resizes through a pointer drag on the handle', async () => {
    const pointerListeners = new Map<string, (event: unknown) => void>()
    stubEnvironment()
    vi.stubGlobal('window', {
      addEventListener: (type: string, listener: (event: unknown) => void) => pointerListeners.set(type, listener),
      removeEventListener: vi.fn(),
      localStorage: { getItem: () => null, setItem: () => {} },
    })
    const renderer = await renderCanvas()

    await act(async () => {
      hostNodes(renderer, 'repository-change-mermaid-resize')[0].props.onPointerDown({
        clientX: 600,
        pointerType: 'mouse',
        button: 0,
        preventDefault: () => {},
      })
    })
    await act(async () => {
      pointerListeners.get('pointermove')?.({ clientX: 500, preventDefault: () => {} })
    })

    expect(hostNodes(renderer, 'repository-change-mermaid-slot')[0].props.w).toEqual({ base: 'full', lg: '480px' })
    await act(async () => {
      pointerListeners.get('pointerup')?.({})
    })
  })

  it('snaps a change diagram under 200px into the collapsed rail', async () => {
    stubEnvironment(String(PANEL_COLLAPSE_SNAP - 1))
    const renderer = await renderCanvas()

    const slot = hostNodes(renderer, 'repository-change-mermaid-slot')[0]
    expect(slot.props.w).toEqual({ base: 'full', lg: `${PANEL_RAIL_WIDTH}px` })
    expect(hostNodes(renderer, 'mock-mermaid')[0].props['data-collapsed']).toBe('true')
    // the rail stays grabbable so it can be dragged back open
    expect(hostNodes(renderer, 'repository-change-mermaid-resize')).toHaveLength(1)
  })

  it('covers the canvas with an expanded change diagram past 650px', async () => {
    stubEnvironment(String(PANEL_OVERLAY_SNAP + 50))
    const renderer = await renderCanvas()

    const slot = hostNodes(renderer, 'repository-change-mermaid-slot')[0]
    expect(slot.props.w).toEqual({ base: 'full', lg: `${PANEL_OVERLAY_SNAP + 50}px` })
    expect(slot.props.position).toEqual({ base: 'relative', lg: 'absolute' })
    expect(slot.props.boxShadow).toBeTruthy()
    expect(hostNodes(renderer, 'mock-mermaid')[0].props['data-overlay']).toBe('true')
  })

  it('keeps 200px and 650px docked to the canvas edge', async () => {
    stubEnvironment(String(PANEL_COLLAPSE_SNAP))
    const atCollapse = await renderCanvas()
    expect(hostNodes(atCollapse, 'repository-change-mermaid-slot')[0].props.position).toBe('relative')
    expect(hostNodes(atCollapse, 'mock-mermaid')[0].props['data-collapsed']).toBe('false')

    stubEnvironment(String(PANEL_OVERLAY_SNAP))
    const atOverlay = await renderCanvas()
    expect(hostNodes(atOverlay, 'repository-change-mermaid-slot')[0].props.position).toBe('relative')
    expect(hostNodes(atOverlay, 'mock-mermaid')[0].props['data-overlay']).toBe('false')
  })

  it('renders a single grounded diagram with no scope toggle', async () => {
    stubEnvironment()
    const renderer = await renderCanvas()
    const menu = () => renderer.root.findByType(RepositoryChangeMenu)
    expect(menu().props.viewMode).toBe('standard')
    expect(hostNodes(renderer, 'repositories-diagram-scope')).toHaveLength(0)
    renderer.unmount()
  })

  it('feeds the loaded scene into the mermaid pane', async () => {
    stubEnvironment()
    const { api } = await import('../api/client')
    const impactScene = api.repositories.impactScene as unknown as ReturnType<typeof vi.fn>
    impactScene.mockResolvedValueOnce({
      tree: [{ id: 10, name: 'Workspace', children: [{ id: 51, name: 'Mine', children: [] }] }],
      views: {
        10: { placements: [], connectors: [] },
        51: { placements: [{ element_id: 7, tags: [], file_path: 'x' }], connectors: [] },
      },
      navigations: [],
      fallbackViewId: 0,
      repositoryId: 'repo-1',
      comparisonKey: 'pair',
      version: 'v1',
      schemaVersion: '1',
      fromGitRevision: 'a',
      toGitRevision: 'b',
      overlays: { 7: { change: 'modified', path: 'x', symbols: [], distance: 0 } },
      authoredViewIds: [51],
    })
    let renderer!: ReturnType<typeof create>
    await act(async () => {
      renderer = create(<RepositoryChangeCanvas diagram={diagram} selectedPath="" />)
    })
    const menu = () => renderer.root.findByType(RepositoryChangeMenu)
    expect(menu().props.viewMode).toBe('standard')
    expect(hostNodes(renderer, 'mock-mermaid')).toHaveLength(1)
    renderer.unmount()
  })
})