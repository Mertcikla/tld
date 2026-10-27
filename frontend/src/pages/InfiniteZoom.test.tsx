import { act, create, type ReactTestRenderer } from 'react-test-renderer'
import { afterEach, describe, expect, it, vi } from 'vitest'
import InfiniteZoom from './InfiniteZoom'
import { FlowPanel } from '../flows/FlowPanel'
import type { FlowFocus, StoryFlow } from '../flows/model'
import type { ExploreData } from '../types'
import { useQuery } from '@tanstack/react-query'

const { queryState } = vi.hoisted(() => ({ queryState: { data: [] as StoryFlow[], isPending: false, isError: false, isSuccess: true } }))
vi.mock('@tanstack/react-query', () => ({ useQuery: vi.fn(() => queryState) }))
vi.mock('react-router-dom', () => ({ useNavigate: () => vi.fn(), useLocation: () => ({ search: '' }), useParams: () => ({}) }))
vi.mock('@chakra-ui/react', () => ({
  Box: 'div', Button: 'button', Center: 'div', HStack: 'div', IconButton: 'button', Popover: 'div', PopoverBody: 'div', PopoverContent: 'div', PopoverTrigger: 'div', Portal: 'div', Text: 'span', Tooltip: 'tooltip', VStack: 'div', Spinner: 'span',
  useDisclosure: () => ({ isOpen: false, onClose: vi.fn(), onToggle: vi.fn() }),
}))
vi.mock('../components/CrossBranchControls', () => ({ default: () => null }))
vi.mock('../components/TagFilterControls', () => ({ TagFilterReset: () => null, TagFilterRow: () => null }))
vi.mock('../components/ZUI', async () => {
  const React = await import('react')
  return { ZUICanvas: React.forwardRef((props, _ref) => React.createElement('canvas', props)) }
})
vi.mock('../flows/FlowPanel', () => ({ FlowPanel: () => null }))
vi.mock('../flows/api', () => ({ flowApi: { list: vi.fn() } }))
vi.mock('./explore/useExploreData', () => ({ useExploreData: () => ({ data: null, loading: false, hasPlacements: false, reload: vi.fn() }) }))
vi.mock('./explore/useExploreTags', () => ({ useExploreTags: () => ({ allTags: [], layers: [], tagColors: {}, layerElementCounts: {}, tagCounts: {}, tagElementCounts: {}, tagConnectorCounts: {}, layerConnectorCounts: {}, highlightedTags: [], hiddenTags: [], selectedTags: [], toggleTagSelection: vi.fn(), resetFilters: vi.fn(), setHighlightedTags: vi.fn(), setHighlightColor: vi.fn(), toggleLayerVisibility: vi.fn(), toggleTagVisibility: vi.fn() }) }))
vi.mock('../crossBranch/settings', () => ({ useCrossBranchContextSettings: () => ({ settings: {}, setEnabled: vi.fn(), setConnectorBudget: vi.fn(), setConnectorPriority: vi.fn() }) }))

const flow: StoryFlow = { id: 'flow', viewId: 1, name: 'Checkout', description: '', revision: 1, steps: [] }
const data: ExploreData = { tree: [{ id: 1, parent_view_id: null }] as ExploreData['tree'], views: { '1': { placements: [], connectorIds: [] } }, navigations: [] }
const dataState = { data, loading: false, error: null, hasPlacements: true, reload: vi.fn() }
let renderer: ReactTestRenderer
function mount(sharedToken?: string) {
  vi.stubGlobal('window', { addEventListener: vi.fn(), removeEventListener: vi.fn() })
  act(() => { renderer = create(<InfiniteZoom dataState={dataState} sharedToken={sharedToken} />) })
  act(() => renderer.root.findByType('canvas').props.onReady())
}
const flowButton = () => renderer.root.findByProps({ 'data-testid': 'zui-flows-button' })
afterEach(() => { act(() => renderer?.unmount()); queryState.data = []; queryState.isPending = false; queryState.isError = false; queryState.isSuccess = true; vi.clearAllMocks(); vi.unstubAllGlobals() })

describe('Explore flow controls', () => {
  it('disables the floating-menu button and explains how to create the first flow', () => {
    mount()
    expect(flowButton().props.isDisabled).toBe(true)
    expect(flowButton().props.title).toBe('No flows yet. Create a flow in the diagram editor.')
    expect(renderer.root.findByProps({ label: 'No flows yet. Create a flow in the diagram editor.' }).props.shouldWrapChildren).toBe(true)
    act(() => flowButton().props.onClick())
    expect(renderer.root.findByType(FlowPanel).props.open).toBe(false)
  })

  it('opens playback from the menu and clears canvas focus when closed', () => {
    queryState.data = [flow]
    mount()
    expect(flowButton().props.isDisabled).toBe(false)
    act(() => flowButton().props.onClick())
    expect(flowButton().props['aria-expanded']).toBe(true)
    expect(renderer.root.findByType(FlowPanel).props.open).toBe(true)
    const focus: FlowFocus = { viewId: 1, elementIds: [1], connectorIds: [] }
    act(() => renderer.root.findByType(FlowPanel).props.onFocus(focus))
    expect(renderer.root.findByType('canvas').props.flowFocus).toBe(focus)
    act(() => renderer.root.findByType(FlowPanel).props.onClose())
    expect(flowButton().props['aria-expanded']).toBe(false)
    expect(renderer.root.findByType('canvas').props.flowFocus).toBeNull()
  })

  it('keeps workspace flow controls and requests out of shared Explore pages', () => {
    queryState.data = [flow]
    mount('shared-token')
    expect(renderer.root.findAllByProps({ 'data-testid': 'zui-flows-button' })).toHaveLength(0)
    expect(renderer.root.findAllByType(FlowPanel)).toHaveLength(0)
    expect(useQuery).toHaveBeenLastCalledWith(expect.objectContaining({ enabled: false }))
    expect(renderer.root.findByType('canvas').props.flowFocus).toBeNull()
  })

  it.each([
    { isPending: true, isError: false, isSuccess: false, hint: 'Loading flows…' },
    { isPending: false, isError: true, isSuccess: false, hint: 'Could not load flows. Refresh the page to try again.' },
  ])('explains unavailable flow lists: $hint', state => {
    Object.assign(queryState, state)
    mount()
    expect(flowButton().props.isDisabled).toBe(true)
    expect(flowButton().props.title).toBe(state.hint)
  })

  it('closes playback when the last flow is removed elsewhere', () => {
    queryState.data = [flow]
    mount()
    act(() => flowButton().props.onClick())
    queryState.data = []
    act(() => renderer.update(<InfiniteZoom dataState={dataState} />))
    expect(flowButton().props.isDisabled).toBe(true)
    expect(renderer.root.findByType(FlowPanel).props.open).toBe(false)
  })
})
