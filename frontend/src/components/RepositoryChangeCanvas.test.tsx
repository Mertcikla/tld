import React from 'react'
import { act, create } from 'react-test-renderer'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import type { RepositoryImpact as ImpactDiagram, RepositoryImpactScene } from '../api/client'
import type { PlacedElement, ViewTreeNode } from '../types'
import { ZUICanvas } from './ZUI'
import { PANEL_COLLAPSE_SNAP, PANEL_OVERLAY_SNAP, PANEL_RAIL_WIDTH } from '../hooks/useResizableColumn'
import RepositoryChangeCanvas from './RepositoryChangeCanvas'
import { changedScenePlacements } from '../utils/liveSceneChanges'
import RepositoryChangeMenu from './RepositoryChangeMenu'
import RepositoryImpactInspector from './RepositoryImpactInspector'
import RepositoryChangeMermaid from './RepositoryChangeMermaid'

const storageKey = 'tld:repositories:mermaidPaneWidth'

const canvasActions = vi.hoisted(() => ({ followElements: vi.fn(), focusElements: vi.fn(() => true), focusDiagram: vi.fn(() => true), focusElement: vi.fn(() => true), cancelCameraTransition: vi.fn() }))
vi.mock('./ZUI', async () => {
  const ReactModule = await import('react')
  return { ZUICanvas: ReactModule.forwardRef((_props, ref) => {
    ReactModule.useImperativeHandle(ref, () => canvasActions)
    return null
  }) }
})
vi.mock('./RepositoryImpactInspector', () => ({
  default: (props: Record<string, unknown>) =>
    React.createElement('div', {
      'data-testid': 'mock-inspector',
      'data-collapsed': String(!!props.collapsed),
      'data-overlay': String(!!props.overlay),
    }),
}))
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
    IconButton: BoxLike,
    Popover: BoxLike,
    PopoverBody: BoxLike,
    PopoverContent: BoxLike,
    PopoverTrigger: BoxLike,
    Portal: ({ children }: { children?: React.ReactNode }) => ReactModule.createElement('div', null, children),
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

function stubEnvironment(storedWidth?: string, search = '') {
  const values = new Map<string, string>()
  if (storedWidth != null) values.set(storageKey, storedWidth)
  vi.stubGlobal('window', {
    location: { search },
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

function LiveCanvas(props: React.ComponentProps<typeof RepositoryChangeCanvas>) {
  const [followMode, setFollowMode] = React.useState<'off' | 'following' | 'paused'>('off')
  return <RepositoryChangeCanvas {...props} followMode={followMode} onFollowModeChange={setFollowMode} />
}

async function renderCanvas() {
  let renderer!: ReturnType<typeof create>
  await act(async () => {
    renderer = create(<RepositoryChangeCanvas diagram={diagram} selectedPath="" />)
  })
  return renderer
}

describe('RepositoryChangeCanvas side panel', () => {
  beforeEach(() => {
    vi.clearAllMocks()
  })

  afterEach(() => {
    vi.unstubAllGlobals()
  })

  const taggedScene = (): RepositoryImpactScene => ({
    repositoryId: 'repo-1', comparisonKey: 'pair', version: 'v1', schemaVersion: '2',
    fromGitRevision: 'a', toGitRevision: 'b', fallbackViewId: -1, overlays: {}, authoredViewIds: [], navigations: [],
    tree: [{ id: 1, name: 'Root', children: [], parent_view_id: null } as unknown as ViewTreeNode],
    views: { 1: { placements: [
      { element_id: 1, name: 'Go', tags: ['go'], position_x: 0, position_y: 0 } as PlacedElement,
      { element_id: 2, name: 'JavaScript', tags: ['javascript'], position_x: 200, position_y: 0 } as PlacedElement,
    ], connectors: [] } },
    placements: Object.fromEntries([1, 2].map((id) => [`placement|1|${id}`, {
      key: `placement|1|${id}`, viewId: 1, resourceId: id, name: '', origin: 'authored' as const,
      grounding: 'file', role: 'direct', paths: [], distance: 0,
    }])),
    coverage: [1, 2].map((id) => ({ path: `${id}.txt`, placementKeys: [`placement|1|${id}`], factsUnavailable: false })),
  })

  it('retains the canvas while the next scene loads, including failed updates', async () => {
    stubEnvironment()
    const { api } = await import('../api/client')
    vi.mocked(api.repositories.impactScene).mockResolvedValueOnce(taggedScene())
    const renderer = await renderCanvas()
    const canvas = renderer.root.findByType(ZUICanvas)
    let reject!: (reason: Error) => void
    vi.mocked(api.repositories.impactScene).mockImplementationOnce(() => new Promise((_resolve, fail) => { reject = fail }))
    await act(async () => renderer.update(<RepositoryChangeCanvas diagram={{ ...diagram, comparisonKey: 'new', version: 'v2' }} selectedPath="" />))
    expect(renderer.root.findByType(ZUICanvas)).toBe(canvas)
    await act(async () => reject(new Error('Connection lost')))
    expect(renderer.root.findByType(ZUICanvas)).toBe(canvas)
    expect(canvas.props.data.views[1].placements[0].name).toBe('Go')
    renderer.unmount()
  })

  it('consumes pushed scenes directly and pauses and resumes follow on manual navigation', async () => {
    stubEnvironment()
    const { api } = await import('../api/client')
    const first = taggedScene()
    first.overlays[1] = { change: 'modified', path: '1.txt', symbols: [], symbolDetails: [], distance: 0, reason: 'direct' }
    let renderer!: ReturnType<typeof create>
    await act(async () => { renderer = create(<LiveCanvas diagram={{ ...diagram, scene: first }} selectedPath="" live />) })
    const canvas = renderer.root.findByType(ZUICanvas)
    expect(api.repositories.impactScene).not.toHaveBeenCalled()
    expect(renderer.root.findByType(RepositoryChangeCanvas).props.followMode).toBe('off')
    expect(canvasActions.followElements).not.toHaveBeenCalled()
    await act(async () => renderer.root.findByType(RepositoryChangeCanvas).props.onFollowModeChange('following'))
    expect(canvasActions.followElements).toHaveBeenLastCalledWith([{ viewId: 1, elementId: 1 }])
    await act(async () => canvas.props.onPan())
    expect(renderer.root.findByType(RepositoryChangeCanvas).props.followMode).toBe('paused')
    canvasActions.followElements.mockClear()
    const next = { ...first, version: 'v2' }
    await act(async () => renderer.update(<LiveCanvas diagram={{ ...diagram, version: 'v2', scene: next }} selectedPath="" live />))
    expect(renderer.root.findByType(ZUICanvas)).toBe(canvas)
    expect(canvasActions.followElements).not.toHaveBeenCalled()
    await act(async () => renderer.root.findByType(RepositoryChangeCanvas).props.onFollowModeChange('following'))
    expect(canvasActions.followElements).toHaveBeenCalledTimes(1)
    await act(async () => canvas.props.onZoom())
    expect(renderer.root.findByType(RepositoryChangeCanvas).props.followMode).toBe('paused')
    await act(async () => renderer.root.findByType(RepositoryChangeCanvas).props.onFollowModeChange('off'))
    expect(canvasActions.cancelCameraTransition).toHaveBeenCalled()
    renderer.unmount()
  })

  it('focuses history clicks, clears hiding filters, pauses follow, and does not refocus later pushes', async () => {
    stubEnvironment()
    const first = taggedScene()
    const target = { key: 'placement|1|2', kind: 'element' as const, viewId: 1, elementIds: [{ viewId: 1, elementId: 2 }], ancestorViewIds: [] }
    let renderer!: ReturnType<typeof create>
    const onLiveScene = vi.fn()
    await act(async () => { renderer = create(<LiveCanvas diagram={{ ...diagram, scene: first }} selectedPath="" live onLiveScene={onLiveScene} />) })
    const menu = () => renderer.root.findByType(RepositoryChangeMenu)
    await act(async () => renderer.root.findByType(RepositoryChangeCanvas).props.onFollowModeChange('following'))
    await act(async () => menu().props.onToggleTagVisibility('javascript'))
    const request = { repositoryId: 'repo-1', targets: [target] }
    await act(async () => renderer.update(<LiveCanvas diagram={{ ...diagram, scene: first }} selectedPath="" live onLiveScene={onLiveScene} focusRequest={request} />))
    expect(menu().props.hiddenTags).toEqual([])
    expect(renderer.root.findByType(RepositoryChangeCanvas).props.followMode).toBe('paused')
    expect(canvasActions.focusElement).toHaveBeenCalledWith(1, 2)
    expect(onLiveScene).toHaveBeenCalledWith(first)
    canvasActions.focusElement.mockClear()
    await act(async () => renderer.update(<LiveCanvas diagram={{ ...diagram, version: 'v2', scene: { ...first, version: 'v2' } }} selectedPath="" live focusRequest={request} />))
    expect(canvasActions.focusElement).not.toHaveBeenCalled()
    await act(async () => renderer.update(<LiveCanvas diagram={{ ...diagram, scene: first }} selectedPath="" live focusRequest={{ ...request }} />))
    expect(canvasActions.focusElement).toHaveBeenCalledWith(1, 2)
    await act(async () => renderer.unmount())
  })

  it('excludes plain presentation scenes from history and restores the canonical scene before focusing', async () => {
    stubEnvironment()
    const { api } = await import('../api/client')
    const first = taggedScene()
    const onLiveScene = vi.fn()
    let renderer!: ReturnType<typeof create>
    await act(async () => { renderer = create(<RepositoryChangeCanvas diagram={{ ...diagram, scene: first }} selectedPath="" live onLiveScene={onLiveScene} />) })
    onLiveScene.mockClear()
    vi.mocked(api.repositories.impactScene).mockResolvedValueOnce({ ...first, views: {} })
    await act(async () => renderer.root.findByType(RepositoryChangeMenu).props.onViewModeChange('plain'))
    expect(onLiveScene).not.toHaveBeenCalled()
    const request = { repositoryId: 'repo-1', targets: [{ key: 'placement|1|2', kind: 'element' as const, viewId: 1, elementIds: [{ viewId: 1, elementId: 2 }], ancestorViewIds: [] }] }
    await act(async () => renderer.update(<RepositoryChangeCanvas diagram={{ ...diagram, scene: first }} selectedPath="" live onLiveScene={onLiveScene} focusRequest={request} />))
    expect(renderer.root.findByType(RepositoryChangeMenu).props.viewMode).toBe('standard')
    expect(onLiveScene).toHaveBeenLastCalledWith(first)
    expect(canvasActions.focusElement).toHaveBeenCalledWith(1, 2)
    await act(async () => renderer.unmount())
  })

  it('identifies equal-sized edits and reversions without following unchanged cumulative changes', () => {
    const first = taggedScene()
    first.files = [{ path: '1.txt', baseHash: 'base', headHash: 'one', symbols: [] }, { path: '2.txt', baseHash: 'base2', headHash: 'two', symbols: [] }] as unknown as RepositoryImpactScene['files']
    const next = { ...first, version: 'v2', files: first.files!.map((file) => file.path === '1.txt' ? { ...file, headHash: 'new' } : file) }
    expect(changedScenePlacements(first, next)).toEqual([{ viewId: 1, elementId: 1 }])
    expect(changedScenePlacements(next, { ...next, version: 'v3', files: next.files.map((file) => ({ ...file, symbols: ['renumbered-symbol-ref'] })) })).toEqual([])
    expect(changedScenePlacements(next, { ...next, version: 'v4', files: next.files.filter((file) => file.path !== '1.txt') })).toEqual([{ viewId: 1, elementId: 1 }])
  })

  it('updates inspector coverage when selected tags hide changes', async () => {
    stubEnvironment(undefined, '?debugImpact=1')
    const { api } = await import('../api/client')
    vi.mocked(api.repositories.impactScene).mockResolvedValueOnce(taggedScene())
    const renderer = await renderCanvas()
    const menu = () => renderer.root.findByType(RepositoryChangeMenu)
    const hiddenChanges = () => renderer.root.findByType(RepositoryImpactInspector).props.hiddenChanges
    expect(hiddenChanges()).toBe(0)
    await act(async () => menu().props.onToggleTagSelection('go'))
    expect(hiddenChanges()).toBe(1)
    expect(renderer.root.findByType(ZUICanvas).props.selectedTags).toEqual(['go'])
    await act(async () => menu().props.onToggleTagSelection('javascript'))
    expect(hiddenChanges()).toBe(0)
    await act(async () => menu().props.onToggleTagVisibility('go'))
    expect(menu().props.selectedTags).toEqual([])
    expect(hiddenChanges()).toBe(1)
    await act(async () => menu().props.onResetTagFilters())
    expect(hiddenChanges()).toBe(0)
    renderer.unmount()
  })

  it('filters frozen embedded scenes without API reads or changing Mermaid content', async () => {
    stubEnvironment()
    const { api } = await import('../api/client')
    let renderer!: ReturnType<typeof create>
    await act(async () => {
      renderer = create(<RepositoryChangeCanvas diagram={null} selectedPath="" embedded={{ scene: taggedScene(), markdown: 'Frozen diagram' }} />)
    })
    const menu = () => renderer.root.findByType(RepositoryChangeMenu)
    await act(async () => menu().props.onToggleTagSelection('go'))
    expect(renderer.root.findByType(ZUICanvas).props.selectedTags).toEqual(['go'])
    expect(renderer.root.findByType(RepositoryChangeMermaid).props.embeddedMarkdown).toBe('Frozen diagram')
    expect(api.repositories.impactScene).not.toHaveBeenCalled()
    await act(async () => menu().props.onToggleTagVisibility('javascript'))
    expect(menu().props.selectedTags).toEqual([])
    expect(menu().props.hiddenTags).toEqual(['javascript'])
    await act(async () => menu().props.onResetTagFilters())
    expect(menu().props.hiddenTags).toEqual([])
    renderer.unmount()
  })

  it('defaults to Mermaid without requesting debug provenance', async () => {
    stubEnvironment()
    const renderer = await renderCanvas()
    const { api } = await import('../api/client')
    expect(renderer.root.findAllByType(RepositoryChangeMermaid)).toHaveLength(1)
    expect(renderer.root.findAllByType(RepositoryImpactInspector)).toHaveLength(0)
    expect(api.repositories.impactScene).toHaveBeenLastCalledWith('repo-1', 'pair', expect.any(AbortSignal), { debugProvenance: false, plain: false })
    renderer.unmount()
  })

  it('enables the inspector only with the explicit developer flag', async () => {
    stubEnvironment(undefined, '?debugImpact=1')
    const renderer = await renderCanvas()
    const { api } = await import('../api/client')
    expect(renderer.root.findAllByType(RepositoryChangeMermaid)).toHaveLength(0)
    expect(hostNodes(renderer, 'mock-inspector')).toHaveLength(1)
    expect(hostNodes(renderer, 'repository-impact-inspector-resize')).toHaveLength(1)
    expect(api.repositories.impactScene).toHaveBeenLastCalledWith('repo-1', 'pair', expect.any(AbortSignal), { debugProvenance: true, plain: false })

    await act(async () => { renderer.root.findByType(RepositoryImpactInspector).props.onDebugChange(false) })
    expect(hostNodes(renderer, 'mock-inspector')).toHaveLength(1)
    expect(renderer.root.findAllByType(RepositoryChangeMermaid)).toHaveLength(0)
    expect(api.repositories.impactScene).toHaveBeenLastCalledWith('repo-1', 'pair', expect.any(AbortSignal), { debugProvenance: false, plain: false })
    renderer.unmount()
  })

  it('keeps Mermaid for a disabled developer flag', async () => {
    stubEnvironment(undefined, '?debugImpact=0')
    const renderer = await renderCanvas()
    expect(hostNodes(renderer, 'mock-mermaid')).toHaveLength(1)
    expect(renderer.root.findAllByType(RepositoryImpactInspector)).toHaveLength(0)
    renderer.unmount()
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
    expect(handle.props.position).toBe('relative')
    expect(handle.props.right).toBeUndefined()
    expect(hostNodes(renderer, 'mock-mermaid')[0].props['data-collapsed']).toBe('false')
    expect(hostNodes(renderer, 'mock-mermaid')[0].props['data-overlay']).toBe('false')
  })

  it('keeps the resize handle at the left edge above the canvas-covering pane', async () => {
    stubEnvironment(String(PANEL_OVERLAY_SNAP + 50))
    const renderer = await renderCanvas()

    const handle = hostNodes(renderer, 'repository-change-mermaid-resize')[0]
    const slot = hostNodes(renderer, 'repository-change-mermaid-slot')[0]
    expect(handle.props.zIndex).toBe(40)
    expect(handle.props.zIndex).toBeGreaterThan(slot.props.zIndex)
    expect(handle.props.position).toBe('absolute')
    expect(handle.props.right).toBe(`${PANEL_OVERLAY_SNAP + 50}px`)
    expect(handle.props.top).toBe(0)
    expect(handle.props.bottom).toBe(0)
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
    expect(hostNodes(renderer, 'repository-change-mermaid-resize')).toHaveLength(0)
  })

  it('covers the canvas with an expanded change diagram past 650px', async () => {
    stubEnvironment(String(PANEL_OVERLAY_SNAP + 50))
    const renderer = await renderCanvas()

    const slot = hostNodes(renderer, 'repository-change-mermaid-slot')[0]
    expect(slot.props.w).toEqual({ base: 'full', lg: `${PANEL_OVERLAY_SNAP + 50}px` })
    expect(slot.props.position).toEqual({ base: 'relative', lg: 'absolute' })
    expect(slot.props.boxShadow).toBeUndefined()
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

  it.each([380, PANEL_OVERLAY_SNAP + 50])('collapses a %ipx pane fully to the rail and expands it again', async (width) => {
    stubEnvironment(String(width))
    const renderer = await renderCanvas()

    await act(async () => { renderer.root.findByType(RepositoryChangeMermaid).props.onDock() })

    const slot = hostNodes(renderer, 'repository-change-mermaid-slot')[0]
    expect(slot.props.w).toEqual({ base: 'full', lg: `${PANEL_RAIL_WIDTH}px` })
    expect(slot.props.h).toEqual({ base: `${PANEL_RAIL_WIDTH}px`, lg: 'auto' })
    expect(slot.props.position).toBe('relative')
    expect(hostNodes(renderer, 'mock-mermaid')[0].props['data-collapsed']).toBe('true')
    expect(hostNodes(renderer, 'repository-change-mermaid-resize')).toHaveLength(0)
    expect(window.localStorage.getItem(storageKey)).toBe(String(PANEL_RAIL_WIDTH))

    await act(async () => { renderer.root.findByType(RepositoryChangeMermaid).props.onExpand() })

    expect(hostNodes(renderer, 'repository-change-mermaid-slot')[0].props.w).toEqual({ base: 'full', lg: '380px' })
    expect(hostNodes(renderer, 'mock-mermaid')[0].props['data-collapsed']).toBe('false')
    expect(hostNodes(renderer, 'repository-change-mermaid-resize')).toHaveLength(1)
    renderer.unmount()
  })

  it('renders a single grounded diagram with no scope toggle', async () => {
    stubEnvironment()
    const renderer = await renderCanvas()
    const menu = () => renderer.root.findByType(RepositoryChangeMenu)
    expect(menu().props.viewMode).toBe('standard')
    expect(hostNodes(renderer, 'repositories-diagram-scope')).toHaveLength(0)
    renderer.unmount()
  })

  it('keeps the Mermaid panel alongside the loaded scene', async () => {
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
