import { act, create, type ReactTestInstance, type ReactTestRenderer } from 'react-test-renderer'
import { useState } from 'react'
import { afterEach, describe, expect, it, vi } from 'vitest'
import { FlowPanel, type FlowPanelProps } from './FlowPanel'
import { flowApi } from './api'
import type { Connector } from '../types'
import { newStep, type FlowDiagram, type StoryFlow } from './model'

vi.mock('@chakra-ui/react', () => ({ Badge: 'badge', FormControl: 'div', FormLabel: 'label', Box: 'div', Button: 'button', Checkbox: 'checkbox', Collapse: ({ in: expanded, children }: { in: boolean; children: React.ReactNode }) => expanded ? children : null, HStack: 'div', IconButton: 'button', Input: 'input', Menu: 'div', MenuButton: 'button', MenuItem: 'menuitem', MenuList: 'div', Select: 'select', Tag: 'tag', TagLabel: 'taglabel', TagCloseButton: 'button', Text: 'p', Textarea: 'textarea', VStack: 'div', Tooltip: ({ children }: { children: React.ReactNode }) => children }))
vi.mock('framer-motion', () => ({ motion: { div: 'div' }, Reorder: { Group: 'reorder-group', Item: 'reorder-item' }, useDragControls: () => ({ start: vi.fn() }) }))
vi.mock('./FlowYamlControls', () => ({ FlowYamlControls: () => null }))
vi.mock('./api', () => ({ flowApi: { list: vi.fn(), save: vi.fn(), remove: vi.fn() } }))
const { queryClient, queryState } = vi.hoisted(() => ({ queryClient: { setQueryData: vi.fn(), invalidateQueries: vi.fn(), getQueryData: vi.fn(), setQueryDefaults: vi.fn(), removeQueries: vi.fn() }, queryState: { data: [] as StoryFlow[], isLoading: false, error: null as Error | null, refetch: vi.fn() } }))
vi.mock('@tanstack/react-query', () => ({ useQueryClient: () => queryClient, useQuery: () => queryState }))
const diagram: FlowDiagram = {
  elements: [{ element_id: 1, name: 'API' }, { element_id: 2, name: 'DB' }, { element_id: 3, name: 'Worker' }],
  connectors: [
    { id: 10, source_element_id: 1, target_element_id: 2, label: 'Stores' },
    { id: 11, source_element_id: 1, target_element_id: 3, label: 'Queues' },
  ] as Connector[],
}
let renderer: ReactTestRenderer
const button = (label: string) => renderer.root.findAllByType('button').find(node => node.props.children === label || node.props['aria-label'] === label)!
const checkbox = (label: string) => renderer.root.findAllByType('checkbox' as never).find(node => node.props.children === label || node.props['aria-label'] === label)!
const change = (node: ReactTestInstance, value: string) => act(() => node.props.onChange({ target: { value } }))
const searchHighlights = (value: string) => change(renderer.root.findByProps({ 'aria-label': 'Search highlights' }), value)
const check = (node: ReactTestInstance, checked: boolean) => act(() => node.props.onChange({ target: { checked } }))
function mount(selection?: { elementId?: number; connectorId?: number }, extra: Partial<FlowPanelProps> = {}) {
  vi.stubGlobal('window', { addEventListener: vi.fn(), removeEventListener: vi.fn(), confirm: vi.fn(() => true) })
  const onFocus = vi.fn()
  act(() => { renderer = create(<FlowPanel open viewId={1} editMode diagram={diagram} selection={selection} onClose={vi.fn()} onFocus={onFocus} {...extra} />) })
  act(() => button('+ New flow').props.onClick())
  act(() => button('+ Step').props.onClick())
  return onFocus
}
afterEach(() => { act(() => renderer?.unmount()); vi.unstubAllGlobals(); vi.clearAllMocks(); queryState.data = []; queryState.error = null; queryClient.getQueryData.mockReset() })

describe('flow step editing', () => {
  it('saves multiple elements and connections with independent directions', async () => {
    const onFocus = mount()
    expect(renderer.root.findAllByProps({ 'aria-label': 'New step type' })).toHaveLength(0)
    searchHighlights('api')
    check(checkbox('API'), true)
    searchHighlights('db')
    check(checkbox('DB'), true)
    act(() => renderer.root.findByProps({ 'aria-label': 'Show connections' }).props.onClick())
    searchHighlights('api')
    check(checkbox('API → DB · Stores'), true)
    check(checkbox('API → Worker · Queues'), true)
    act(() => renderer.root.findByProps({ 'aria-label': 'Reverse API → DB · Stores' }).props.onClick())
    change(renderer.root.findByProps({ 'aria-label': 'Step title' }), 'Save and queue')
    const expected = { title: 'Save and queue', elementIds: [1, 2], connectors: [{ id: 10, reverse: true }, { id: 11, reverse: false }], metadata: {} }
    expect(onFocus).toHaveBeenLastCalledWith({ viewId: 1, elementIds: [1, 2, 3], connectors: expected.connectors })
    vi.mocked(flowApi.save).mockImplementation(async flow => ({ ...flow, id: 'saved', revision: 1 }))
    await act(async () => button('Save flow').props.onClick())
    expect(flowApi.save).toHaveBeenCalledWith(expect.objectContaining({ steps: [expect.objectContaining(expected)] }))
  })

  it('reorders whole steps with the handle keyboard controls and preserves the active editor', async () => {
    mount({ elementId: 2, connectorId: 10 })
    change(renderer.root.findByProps({ 'aria-label': 'Step title' }), 'Store data')
    act(() => button('+ Step').props.onClick())
    change(renderer.root.findByProps({ 'aria-label': 'Step title' }), 'Finish')
    const key = (label: string, value: string) => act(() => button(label).props.onKeyDown({ key: value, preventDefault: vi.fn(), stopPropagation: vi.fn() }))
    key('Reorder step 2', 'ArrowUp')
    key('Reorder step 1', 'ArrowUp') // Already at the start.
    expect(renderer.root.findByProps({ 'aria-label': 'Step title' }).props.value).toBe('Finish')
    expect(button('Move step up')).toBeUndefined()
    expect(button('Move step down')).toBeUndefined()
    vi.mocked(flowApi.save).mockImplementation(async flow => ({ ...flow, id: 'saved', revision: 1 }))
    await act(async () => button('Save flow').props.onClick())
    expect(vi.mocked(flowApi.save).mock.lastCall![0].steps.map(step => step.title)).toEqual(['Finish', 'Store data'])
    expect(vi.mocked(flowApi.save).mock.lastCall![0].steps[1]).toMatchObject({ elementIds: [2], connectors: [{ id: 10, reverse: false }] })
  })

  it('accepts drag order and duplicates or removes a collapsed step without opening it first', async () => {
    mount({ elementId: 1 })
    change(renderer.root.findByProps({ 'aria-label': 'Step title' }), 'First')
    act(() => button('+ Step').props.onClick())
    change(renderer.root.findByProps({ 'aria-label': 'Step title' }), 'Second')
    const group = renderer.root.findByType('reorder-group' as never)
    const [first, second] = group.props.values
    act(() => group.props.onReorder([second, first]))
    act(() => button('Duplicate step 2').props.onClick())
    expect(renderer.root.findByProps({ 'aria-label': 'Step title' }).props.value).toBe('First')
    act(() => button('Remove step 1').props.onClick())
    vi.mocked(flowApi.save).mockImplementation(async flow => ({ ...flow, id: 'saved', revision: 1 }))
    await act(async () => button('Save flow').props.onClick())
    const steps = vi.mocked(flowApi.save).mock.lastCall![0].steps
    expect(steps.map(step => step.title)).toEqual(['First', 'First'])
    expect(steps[0].id).not.toBe(steps[1].id)
    expect(steps.map(step => step.elementIds)).toEqual([[1], [1]])
  })

  it('starts from the canvas selection, supports removing every highlight and adds empty steps', async () => {
    const onFocus = mount({ elementId: 2, connectorId: 10 })
    expect(renderer.root.findByProps({ 'aria-label': 'Remove element DB' })).toBeDefined()
    act(() => renderer.root.findByProps({ 'aria-label': 'Remove element DB' }).props.onClick())
    act(() => renderer.root.findByProps({ 'aria-label': 'Show connections' }).props.onClick())
    expect(renderer.root.findByProps({ 'aria-label': 'Remove connection API → DB · Stores' })).toBeDefined()
    act(() => renderer.root.findByProps({ 'aria-label': 'Remove connection API → DB · Stores' }).props.onClick())
    expect(onFocus).toHaveBeenLastCalledWith({ viewId: 1, elementIds: [], connectors: [] })
    vi.mocked(flowApi.save).mockImplementation(async (flow: StoryFlow) => ({ ...flow, id: 'saved', revision: 1 }))
    await act(async () => button('Save flow').props.onClick())
    expect(flowApi.save).toHaveBeenCalledWith(expect.objectContaining({ steps: [expect.objectContaining({ elementIds: [], connectors: [], title: '', description: '', metadata: {} })] }))
  })
  it('shows search results only for a query and preserves highlights when search clears', () => {
    mount()
    expect(renderer.root.findAllByType('checkbox' as never).filter(node => node.props['aria-label'] !== 'Whole flow overview')).toHaveLength(0)
    searchHighlights('api')
    check(checkbox('API'), true)
    searchHighlights('db')
    expect(checkbox('API')).toBeUndefined()
    check(checkbox('DB'), true)
    searchHighlights('  ')
    expect(renderer.root.findAllByProps({ 'data-testid': 'highlight-search-results' })).toHaveLength(0)
    expect(renderer.root.findByProps({ 'aria-label': 'Remove element API' })).toBeDefined()
    expect(renderer.root.findByProps({ 'aria-label': 'Remove element DB' })).toBeDefined()
    searchHighlights('api')
    expect(checkbox('API').props.isChecked).toBe(true)
    searchHighlights('db')
    expect(checkbox('DB').props.isChecked).toBe(true)
  })

  it('caps both result lists at five and finds highlights beyond the initial matches', () => {
    const largeDiagram: FlowDiagram = {
      elements: Array.from({ length: 8 }, (_, id) => ({ element_id: id + 1, name: `Service ${id}` })),
      connectors: Array.from({ length: 8 }, (_, id) => ({ id: id + 10, source_element_id: 1, target_element_id: id + 1, label: `Message ${id}` })) as Connector[],
    }
    mount(undefined, { diagram: largeDiagram })
    searchHighlights('service')
    expect(renderer.root.findAllByType('checkbox' as never).filter(node => node.props['aria-label'] !== 'Whole flow overview')).toHaveLength(5)
    check(checkbox('Service 0'), true)
    searchHighlights('service 7')
    expect(renderer.root.findAllByType('checkbox' as never).filter(node => node.props['aria-label'] !== 'Whole flow overview')).toHaveLength(1)
    check(checkbox('Service 7'), true)
    act(() => renderer.root.findByProps({ 'aria-label': 'Show connections' }).props.onClick())
    expect(renderer.root.findAllByProps({ 'data-testid': 'highlight-search-results' })).toHaveLength(0)
    searchHighlights('message')
    expect(renderer.root.findAllByType('checkbox' as never).filter(node => node.props['aria-label'] !== 'Whole flow overview')).toHaveLength(5)
    check(checkbox('Message 0'), true)
    act(() => renderer.root.findByProps({ 'aria-label': 'Reverse Service 0 → Service 0 · Message 0' }).props.onClick())
    searchHighlights('message 7')
    check(checkbox('Message 7'), true)
    searchHighlights('')
    expect(renderer.root.findAllByType('checkbox' as never).filter(node => node.props['aria-label'] !== 'Whole flow overview')).toHaveLength(0)
    expect(renderer.root.findByProps({ 'aria-label': 'Reverse Service 0 → Service 0 · Message 0' }).props['aria-pressed']).toBe(true)
    expect(renderer.root.findByProps({ 'aria-label': 'Remove connection Service 0 → Service 7 · Message 7' })).toBeDefined()
    searchHighlights('no such connection')
    expect(renderer.root.findAllByType('checkbox' as never).filter(node => node.props['aria-label'] !== 'Whole flow overview')).toHaveLength(0)
  })

  it('shows concise connection labels while keeping endpoint search and selections', () => {
    mount({ connectorId: 10 })
    act(() => renderer.root.findByProps({ 'aria-label': 'Show connections' }).props.onClick())
    searchHighlights('api')
    expect(checkbox('API → DB · Stores').props.children).toBe('Stores')
    change(renderer.root.findByProps({ 'aria-label': 'Search highlights' }), 'db')
    expect(checkbox('Stores').props.isChecked).toBe(true)
    expect(checkbox('Queues')).toBeUndefined()
    change(renderer.root.findByProps({ 'aria-label': 'Search highlights' }), 'queues')
    expect(checkbox('Queues')).toBeDefined()
    change(renderer.root.findByProps({ 'aria-label': 'Search highlights' }), '')
    expect(checkbox('Stores')).toBeUndefined()
    expect(renderer.root.findByProps({ 'aria-label': 'Remove connection API → DB · Stores' })).toBeDefined()
  })

  it('retries loading without discarding an unsaved draft', () => {
    queryState.error = new Error('Connection lost')
    mount()
    change(renderer.root.findByProps({ 'aria-label': 'Flow name' }), 'My unsaved flow')
    act(() => button('Retry loading').props.onClick())
    expect(queryState.refetch).toHaveBeenCalledOnce()
    expect(renderer.root.findByProps({ 'aria-label': 'Flow name' }).props.value).toBe('My unsaved flow')
    expect(window.confirm).not.toHaveBeenCalled()
  })

  it('keeps playback open during editing and preserves the draft when closing and reopening', () => {
    mount()
    expect(button('Auto-play')).toBeDefined()
    const props = renderer.root.findByType(FlowPanel).props
    act(() => renderer.update(<FlowPanel {...props} open={false} />))
    act(() => renderer.update(<FlowPanel {...props} open />))
    expect(renderer.root.findByProps({ 'aria-label': 'Flow name' }).props.value).toBe('New flow')
    expect(button('Save flow').props.isDisabled).toBe(false)
  })

  it('pins playback outside the editor scroll area and pauses when collapsed', () => {
    mount({ elementId: 1 })
    act(() => button('+ Step').props.onClick())
    const dock = renderer.root.findByProps({ 'data-testid': 'flow-playback-dock' })
    expect(renderer.root.findByProps({ 'data-flow-scroll': true }).findAllByProps({ 'data-testid': 'flow-playback-dock' })).toHaveLength(0)
    expect(dock.props.flexShrink).toBe(0)
    expect(button('Toggle playback controls').props['aria-expanded']).toBe(true)
    act(() => button('Auto-play').props.onClick())
    expect(button('Pause')).toBeDefined()
    act(() => button('Toggle playback controls').props.onClick())
    expect(button('Pause')).toBeUndefined()
    act(() => button('Toggle playback controls').props.onClick())
    expect(button('Auto-play')).toBeDefined()
    expect(renderer.root.findByProps({ 'aria-label': 'Flow name' }).props.value).toBe('New flow')
    expect(flowApi.save).not.toHaveBeenCalled()
  })

  it('records a new step for each click, including repeats, from an empty flow', async () => {
    const onCanvasRecorderChange = vi.fn()
    const onFocus = mount(undefined, { onCanvasRecorderChange })
    act(() => button('Remove step 1').props.onClick())
    expect(button('Pick on canvas')).toBeUndefined()
    expect(button('Fit highlights')).toBeUndefined()
    act(() => button('Record steps').props.onClick())
    const capture = (target: { elementId: number } | { connectorId: number }) => act(() => onCanvasRecorderChange.mock.lastCall![0].capture(target))
    capture({ elementId: 1 })
    capture({ connectorId: 10 })
    expect(onFocus).toHaveBeenLastCalledWith({ viewId: 1, elementIds: [1, 2], connectors: [{ id: 10, reverse: false }] })
    capture({ elementId: 1 })
    capture({ elementId: 99 })
    expect(button('Stop recording').props['aria-pressed']).toBe(true)
    act(() => onCanvasRecorderChange.mock.lastCall![0].stop())
    expect(onCanvasRecorderChange).toHaveBeenLastCalledWith(null)
    vi.mocked(flowApi.save).mockImplementation(async flow => ({ ...flow, id: 'saved', revision: 1 }))
    await act(async () => button('Save flow').props.onClick())
    const steps = vi.mocked(flowApi.save).mock.lastCall![0].steps
    expect(steps).toEqual([
      expect.objectContaining({ elementIds: [1], connectors: [] }),
      expect.objectContaining({ elementIds: [], connectors: [{ id: 10, reverse: false }] }),
      expect.objectContaining({ elementIds: [1], connectors: [] }),
    ])
    expect(new Set(steps.map(step => step.id)).size).toBe(3)
  })

  it('picks highlights into the existing step, preserving directions and stored metadata', async () => {
    const onCanvasRecorderChange = vi.fn()
    const onClose = vi.fn()
    const step = { id: 'original', title: 'Original title', description: '', elementIds: [1], connectors: [{ id: 10, reverse: true }], metadata: { owner: 'Commerce' } }
    queryClient.getQueryData.mockReturnValue({ id: 'saved', revision: 1, viewId: 1, name: 'Story', description: '', steps: [step] })
    vi.stubGlobal('window', { addEventListener: vi.fn(), removeEventListener: vi.fn(), confirm: vi.fn(() => true) })
    act(() => { renderer = create(<FlowPanel open viewId={1} editMode diagram={diagram} onCanvasRecorderChange={onCanvasRecorderChange} onClose={onClose} onFocus={vi.fn()} />) })
    act(() => renderer.root.findAllByType('button').find(node => node.props['aria-expanded'] === false && !node.props['aria-label'])!.props.onClick())
    expect(renderer.root.findAllByProps({ 'aria-label': 'Metadata key' })).toHaveLength(0)
    act(() => button('Pick from canvas').props.onClick())
    expect(onCanvasRecorderChange.mock.lastCall![0].mode).toBe('pick')
    const capture = (target: { elementId: number } | { connectorId: number }) => act(() => onCanvasRecorderChange.mock.lastCall![0].capture(target))
    capture({ elementId: 2 })
    capture({ connectorId: 11 })
    capture({ elementId: 2 })
    capture({ connectorId: 10 })
    capture({ connectorId: 99 })
    expect(button('Done picking').props['aria-pressed']).toBe(true)
    const listener = vi.mocked(window.addEventListener).mock.calls.filter(([event]) => event === 'keydown').slice(-1)[0]![1] as (event: unknown) => void
    act(() => listener({ key: 'Escape', preventDefault: vi.fn() }))
    expect(onCanvasRecorderChange).toHaveBeenLastCalledWith(null)
    expect(onClose).not.toHaveBeenCalled()
    vi.mocked(flowApi.save).mockImplementation(async flow => ({ ...flow, revision: 2 }))
    await act(async () => button('Save flow').props.onClick())
    expect(flowApi.save).toHaveBeenCalledWith(expect.objectContaining({ steps: [{ ...step, elementIds: [1, 2], connectors: [{ id: 10, reverse: true }, { id: 11, reverse: false }] }] }))
  })

  it('switches between picking and recording without mixing their behaviors', async () => {
    const onCanvasRecorderChange = vi.fn()
    mount(undefined, { onCanvasRecorderChange })
    act(() => button('Pick from canvas').props.onClick())
    act(() => onCanvasRecorderChange.mock.lastCall![0].capture({ elementId: 1 }))
    act(() => button('Record steps').props.onClick())
    expect(onCanvasRecorderChange.mock.lastCall![0].mode).toBe('record')
    act(() => onCanvasRecorderChange.mock.lastCall![0].capture({ elementId: 2 }))
    act(() => onCanvasRecorderChange.mock.lastCall![0].stop())
    act(() => button('Pick from canvas').props.onClick())
    act(() => onCanvasRecorderChange.mock.lastCall![0].capture({ elementId: 3 }))
    act(() => onCanvasRecorderChange.mock.lastCall![0].stop())
    vi.mocked(flowApi.save).mockImplementation(async flow => ({ ...flow, id: 'saved', revision: 1 }))
    await act(async () => button('Save flow').props.onClick())
    expect(vi.mocked(flowApi.save).mock.lastCall![0].steps.map(step => step.elementIds)).toEqual([[1], [2, 3]])
  })

  it('stops picking when another step is selected or preview starts', () => {
    const onCanvasRecorderChange = vi.fn()
    mount(undefined, { onCanvasRecorderChange })
    act(() => button('Pick from canvas').props.onClick())
    act(() => button('+ Step').props.onClick())
    expect(onCanvasRecorderChange).toHaveBeenLastCalledWith(null)
    act(() => button('Pick from canvas').props.onClick())
    const props = renderer.root.findByType(FlowPanel).props
    act(() => renderer.update(<FlowPanel {...props} editMode={false} />))
    expect(onCanvasRecorderChange).toHaveBeenLastCalledWith(null)
  })

  it('stops recording when preview starts or the panel closes', () => {
    const onCanvasRecorderChange = vi.fn()
    mount(undefined, { onCanvasRecorderChange })
    act(() => button('Record steps').props.onClick())
    const props = renderer.root.findByType(FlowPanel).props
    act(() => renderer.update(<FlowPanel {...props} editMode={false} />))
    expect(onCanvasRecorderChange).toHaveBeenLastCalledWith(null)
    act(() => renderer.update(<FlowPanel {...props} />))
    act(() => button('Record steps').props.onClick())
    act(() => renderer.update(<FlowPanel {...props} open={false} />))
    expect(onCanvasRecorderChange).toHaveBeenLastCalledWith(null)
  })

  it('stops at the step limit and prevents recording more than 500 steps', () => {
    queryClient.getQueryData.mockReturnValue({ id: '', viewId: 1, name: 'Full story', description: '', revision: 0,
      steps: Array.from({ length: 499 }, (_, index) => ({ id: String(index), title: '', description: '', elementIds: [1], connectors: [], metadata: {} })) })
    const onCanvasRecorderChange = vi.fn()
    vi.stubGlobal('window', { addEventListener: vi.fn(), removeEventListener: vi.fn(), confirm: vi.fn(() => true) })
    act(() => { renderer = create(<FlowPanel open viewId={1} editMode diagram={diagram} onCanvasRecorderChange={onCanvasRecorderChange} onClose={vi.fn()} onFocus={vi.fn()} />) })
    act(() => button('Record steps').props.onClick())
    act(() => onCanvasRecorderChange.mock.lastCall![0].capture({ elementId: 1 }))
    expect(onCanvasRecorderChange).toHaveBeenLastCalledWith(null)
    expect(button('Record steps').props.isDisabled).toBe(true)
    expect(button('+ Step').props.isDisabled).toBe(true)
  })

  it.each(['Record steps', 'Pick from canvas'])('shrinks the compact panel during %s and restores its controls after stopping', start => {
    const onCanvasRecorderChange = vi.fn()
    mount(undefined, { compact: true, onCanvasRecorderChange })
    act(() => button(start).props.onClick())
    expect(renderer.root.findByProps({ 'data-flow-scroll': true }).props.display).toBe('none')
    expect(button('Save flow')).toBeUndefined()
    expect(renderer.root.findAllByProps({ 'data-testid': 'flow-yaml-footer' })).toHaveLength(0)
    act(() => onCanvasRecorderChange.mock.lastCall![0].stop())
    expect(renderer.root.findByProps({ 'data-flow-scroll': true }).props.display).toBe('flex')
    expect(button('Save flow')).toBeDefined()
  })

  it.each(['Record steps', 'Pick from canvas'])('Escape stops %s before closing and Cmd/Ctrl+S saves from an input', async start => {
    const onClose = vi.fn()
    mount(undefined, { onCanvasRecorderChange: vi.fn(), onClose })
    act(() => button(start).props.onClick())
    const key = (value: string, ctrlKey = false) => renderer.root.findByProps({ 'data-testid': 'flow-panel' }).props.onKeyDown({ key: value, ctrlKey, target: {}, currentTarget: {}, preventDefault: vi.fn(), stopPropagation: vi.fn() })
    act(() => key('Escape'))
    expect(onClose).not.toHaveBeenCalled()
    vi.mocked(flowApi.save).mockImplementation(async flow => ({ ...flow, id: 'saved', revision: 1 }))
    await act(async () => key('s', true))
    expect(flowApi.save).toHaveBeenCalledOnce()
    act(() => key('Escape'))
    expect(onClose).toHaveBeenCalledOnce()
  })

  it('Escape from the canvas stops recording without closing the panel', () => {
    const onClose = vi.fn()
    const onCanvasRecorderChange = vi.fn()
    mount(undefined, { onClose, onCanvasRecorderChange })
    act(() => button('Record steps').props.onClick())
    const listener = vi.mocked(window.addEventListener).mock.calls.filter(([event]) => event === 'keydown').slice(-1)[0]![1] as (event: unknown) => void
    act(() => listener({ key: 'Escape', preventDefault: vi.fn() }))
    expect(onCanvasRecorderChange).toHaveBeenLastCalledWith(null)
    expect(onClose).not.toHaveBeenCalled()
  })

  it('restores unsaved work after navigating away from a diagram', () => {
    mount()
    change(renderer.root.findByProps({ 'aria-label': 'Step title' }), 'Keep this step')
    const cached = queryClient.setQueryData.mock.calls.filter(([key]) => key[0] === 'flow-draft').slice(-1)[0]![1]
    act(() => renderer.unmount())
    queryClient.getQueryData.mockReturnValue(cached)
    act(() => { renderer = create(<FlowPanel open viewId={1} editMode diagram={diagram} onClose={vi.fn()} onFocus={vi.fn()} />) })
    expect(renderer.root.findByProps({ 'aria-label': 'Flow name' }).props.value).toBe('New flow')
    expect(button('Save flow').props.isDisabled).toBe(false)
    act(() => button('Discard').props.onClick())
    expect(queryClient.removeQueries).toHaveBeenLastCalledWith({ queryKey: ['flow-draft', 1], exact: true })
  })

  it('always edits new flows and keeps playback available without a mode toggle', () => {
    vi.stubGlobal('window', { addEventListener: vi.fn(), removeEventListener: vi.fn(), confirm: vi.fn(() => true) })
    act(() => { renderer = create(<FlowPanel open viewId={1} onEditModeChange={vi.fn()} diagram={diagram} onClose={vi.fn()} onFocus={vi.fn()} />) })
    act(() => button('+ New flow').props.onClick())
    expect(renderer.root.findByProps({ 'aria-label': 'Flow name' }).props.value).toBe('New flow')
    expect(button('Edit flow')).toBeUndefined()
    expect(button('Preview')).toBeUndefined()
    expect(button('Save flow')).toBeDefined()
    expect(button('Toggle playback controls').props['aria-expanded']).toBe(true)
    act(() => button('Toggle playback controls').props.onClick())
    expect(button('Auto-play')).toBeUndefined()
    change(renderer.root.findByProps({ 'aria-label': 'Flow name' }), 'Draft flow')
    act(() => button('Toggle playback controls').props.onClick())
    expect(renderer.root.findByProps({ 'aria-label': 'Flow name' }).props.value).toBe('Draft flow')
    expect(button('Auto-play')).toBeDefined()
  })

  it('lets the flow chooser handle Escape before the panel handles it', () => {
    const onClose = vi.fn()
    mount(undefined, { onClose })
    const preventDefault = vi.fn()
    act(() => renderer.root.findByProps({ 'data-testid': 'flow-panel' }).props.onKeyDown({ key: 'Escape', defaultPrevented: true, preventDefault, stopPropagation: vi.fn() }))
    expect(onClose).not.toHaveBeenCalled()
    expect(preventDefault).not.toHaveBeenCalled()
  })

  it('updates the chooser count as draft steps are added and removed', () => {
    mount()
    const options = () => renderer.root.findAllByType('menuitem' as never)
    expect(options().find(node => node.props['aria-label'] === 'New flow, 1 step')).toBeDefined()
    act(() => button('+ Step').props.onClick())
    expect(options().find(node => node.props['aria-label'] === 'New flow, 2 steps')).toBeDefined()
    act(() => button('Remove step 2').props.onClick())
    act(() => button('Remove step 1').props.onClick())
    expect(options().find(node => node.props['aria-label'] === 'New flow, 0 steps')).toBeDefined()
  })

  it('shows playback and flow management while editing and duplicates into a draft', () => {
    vi.stubGlobal('window', { addEventListener: vi.fn(), removeEventListener: vi.fn(), confirm: vi.fn(() => true) })
    const original: StoryFlow = { id: 'saved', viewId: 1, name: 'Story', description: '', revision: 1, steps: [{ id: 's1', title: 'API', description: '', elementIds: [1], connectors: [], metadata: {} }] }
    queryState.data = [original]
    const onEditModeChange = vi.fn()
    act(() => { renderer = create(<FlowPanel open viewId={1} onEditModeChange={onEditModeChange} diagram={diagram} onClose={vi.fn()} onFocus={vi.fn()} />) })
    act(() => renderer.root.findAllByType('menuitem' as never).find(node => node.props['aria-label'] === 'Story, 1 step')!.props.onClick())
    expect(button('Auto-play')).toBeDefined()
    expect(button('Toggle playback controls')).toBeDefined()
    const footer = renderer.root.findByProps({ 'data-testid': 'flow-yaml-footer' })
    expect(footer.findAllByProps({ 'aria-label': 'Duplicate flow' })).not.toHaveLength(0)
    expect(button('Duplicate flow').props.isDisabled).toBe(false)
    expect(button('Delete flow').props.isDisabled).toBe(false)
    act(() => button('Duplicate flow').props.onClick())
    expect(onEditModeChange).toHaveBeenLastCalledWith(true)
    const copied = queryClient.setQueryData.mock.calls.filter(([key]) => key[0] === 'flow-draft').slice(-1)[0]![1] as StoryFlow
    expect(copied).toMatchObject({ id: '', name: 'Story copy', steps: [expect.objectContaining({ title: 'API', elementIds: [1] })] })
    expect(copied.steps[0].id).not.toBe(original.steps[0].id)
  })

  it('confirms and deletes a saved flow from preview while respecting edit permissions', async () => {
    vi.stubGlobal('window', { addEventListener: vi.fn(), removeEventListener: vi.fn(), confirm: vi.fn(() => true) })
    const original: StoryFlow = { id: 'saved', viewId: 1, name: 'Story', description: '', revision: 1, steps: [] }
    queryState.data = [original]
    const props = { open: true, viewId: 1, diagram, onClose: vi.fn(), onFocus: vi.fn() }
    act(() => { renderer = create(<FlowPanel {...props} />) })
    act(() => renderer.root.findAllByType('menuitem' as never).find(node => node.props['aria-label'] === 'Story, 0 steps')!.props.onClick())
    expect(button('Delete flow')).toBeUndefined()
    expect(button('Duplicate flow')).toBeUndefined()
    act(() => renderer.update(<FlowPanel {...props} onEditModeChange={vi.fn()} />))
    act(() => button('Delete flow').props.onClick())
    expect(flowApi.remove).not.toHaveBeenCalled()
    act(() => button('Cancel').props.onClick())
    expect(flowApi.remove).not.toHaveBeenCalled()
    act(() => button('Delete flow').props.onClick())
    await act(async () => button('Delete').props.onClick())
    expect(flowApi.remove).toHaveBeenCalledWith(original)
  })

  it('defaults to the first flow and previews each flow selected from the chooser', () => {
    vi.stubGlobal('window', { addEventListener: vi.fn(), removeEventListener: vi.fn(), confirm: vi.fn(() => true) })
    queryState.data = [
      { id: 'a', viewId: 1, name: 'First story', description: '', revision: 1, steps: [] },
      { id: 'b', viewId: 1, name: 'Second story', description: '', revision: 1, steps: [] },
    ]
    function ControlledPanel() {
      const [editing, setEditing] = useState(false)
      return <FlowPanel open viewId={1} editMode={editing} onEditModeChange={setEditing} diagram={diagram} onClose={vi.fn()} onFocus={vi.fn()} />
    }
    act(() => { renderer = create(<ControlledPanel />) })
    expect(button('Edit flow')).toBeUndefined()
    expect(renderer.root.findByProps({ 'aria-label': 'First story, 0 steps' }).props['aria-current']).toBe('true')
    expect(renderer.root.findByProps({ 'aria-label': 'Flow name' }).props.value).toBe('First story')
    act(() => renderer.root.findAllByType('menuitem' as never).find(node => node.props['aria-label'] === 'Second story, 0 steps')!.props.onClick())
    expect(renderer.root.findByProps({ 'aria-label': 'Flow name' }).props.value).toBe('Second story')
    expect(button('Edit flow')).toBeUndefined()
    expect(renderer.root.findAllByType('menuitem' as never).some(node => node.props.children === 'Choose a flow')).toBe(false)
    expect(button('+ New flow')).toBeDefined()
  })

  it('remembers the selected flow across remounts and falls back when it is removed', () => {
    const storage = new Map<string, string>()
    const localStorage = { getItem: (key: string) => storage.get(key) ?? null, setItem: (key: string, value: string) => storage.set(key, value) }
    vi.stubGlobal('window', { localStorage, addEventListener: vi.fn(), removeEventListener: vi.fn() })
    queryState.data = [
      { id: 'a', viewId: 1, name: 'First story', description: '', revision: 1, steps: [] },
      { id: 'b', viewId: 1, name: 'Second story', description: '', revision: 1, steps: [] },
    ]
    const props = { open: true, viewId: 1, onClose: vi.fn(), onFocus: vi.fn() }
    const selected = (name: string) => renderer.root.findByProps({ 'aria-label': `${name}, 0 steps` }).props['aria-current']
    act(() => { renderer = create(<FlowPanel {...props} />) })
    act(() => renderer.root.findByProps({ 'aria-label': 'Second story, 0 steps' }).props.onClick())
    act(() => renderer.unmount())
    // A reload can mount before the flow list arrives.
    const flows = queryState.data
    queryState.data = []
    act(() => { renderer = create(<FlowPanel {...props} />) })
    queryState.data = flows
    act(() => renderer.update(<FlowPanel {...props} />))
    expect(selected('Second story')).toBe('true')
    act(() => renderer.unmount())
    queryState.data = flows.slice(0, 1)
    act(() => { renderer = create(<FlowPanel {...props} />) })
    expect(selected('First story')).toBe('true')
    act(() => renderer.unmount())
    queryState.data = flows
    act(() => { renderer = create(<FlowPanel {...props} viewId={2} />) })
    expect(selected('First story')).toBe('true')
  })

})

describe('flow operation editing and navigation', () => {
  it('edits steps inside alternate paths using the existing highlight editor', async () => {
    const onCanvasRecorderChange = vi.fn()
    mount({ elementId: 2 }, { onCanvasRecorderChange })
    act(() => button('Add alternate paths').props.onClick())
    const addButtons = renderer.root.findAllByType('button').filter(node => node.props.children === '+ Step')
    expect(addButtons).toHaveLength(3)
    act(() => addButtons[2].props.onClick())
    const titles = renderer.root.findAllByProps({ 'aria-label': 'Step title' })
    change(titles[titles.length - 1], 'Failure handling')
    act(() => button('Pick from canvas').props.onClick())
    act(() => onCanvasRecorderChange.mock.lastCall![0].capture({ connectorId: 11 }))
    vi.mocked(flowApi.save).mockImplementation(async flow => ({ ...flow, id: 'saved', revision: 1 }))
    await act(async () => button('Save flow').props.onClick())
    const step = vi.mocked(flowApi.save).mock.lastCall![0].steps[0]
    expect(step.branches?.mode).toBe('alternate')
    expect(step.branches?.paths[0].steps).toEqual([])
    expect(step.branches?.paths[1].steps[0]).toMatchObject({ title: 'Failure handling', elementIds: [2], connectors: [{ id: 11, reverse: false }] })
  })

  it('preserves an unsaved caller while previewing a linked flow', async () => {
    queryState.data = [{ id: 'child', viewId: 1, name: 'Child', description: '', revision: 1, steps: [] }]
    mount()
    change(renderer.root.findByProps({ 'aria-label': 'Step title' }), 'Unsaved caller')
    change(renderer.root.findByProps({ 'aria-label': 'Go to flow' }), 'child')
    act(() => button('Go to flow').props.onClick())
    expect(button('Discard')).toBeUndefined()
    expect(button('Save flow')).toBeUndefined()
    act(() => button('Return to previous flow').props.onClick())
    expect(renderer.root.findByProps({ 'aria-label': 'Step title' }).props.value).toBe('Unsaved caller')
    vi.mocked(flowApi.save).mockImplementation(async flow => ({ ...flow, id: 'saved', revision: 1 }))
    await act(async () => button('Save flow').props.onClick())
    expect(vi.mocked(flowApi.save).mock.lastCall![0].steps[0]).toMatchObject({ title: 'Unsaved caller', goToFlowId: 'child' })
  })

  function linkedFlows() {
    const child: StoryFlow = { id: 'child', viewId: 2, name: 'Child', revision: 1, description: '', steps: [{ ...newStep(diagram), id: 'child-step', elementIds: [2] }] }
    const parent: StoryFlow = { id: 'parent', viewId: 1, name: 'Parent', revision: 1, description: '', steps: [
      { ...newStep(diagram), id: 'decision', branches: { mode: 'alternate', paths: [
        { id: 'a', name: 'A', steps: [] },
        { id: 'b', name: 'B', steps: [{ ...newStep(diagram), id: 'call', goToFlowId: child.id }] },
      ] } },
      { ...newStep(diagram), id: 'end' },
    ] }
    return [parent, child]
  }
  function selectParentLink() {
    act(() => renderer.root.findAllByType('menuitem' as never).find(node => node.props['aria-label'] === 'Parent, 3 steps')!.props.onClick())
    // The editor can select a nested step even before its path has been selected.
    const group = renderer.root.findAllByProps({ 'aria-label': 'Current flow step' })[0]
    change(group, 'call')
  }

  it('opens and returns from a cross-diagram link across editor remounts, restoring the chosen path', () => {
    vi.stubGlobal('window', { addEventListener: vi.fn(), removeEventListener: vi.fn(), confirm: vi.fn(() => true) })
    queryState.data = linkedFlows()
    const onNavigateView = vi.fn(), onFocus = vi.fn()
    const render = (viewId: number) => <FlowPanel open viewId={viewId} diagram={diagram} onNavigateView={onNavigateView} onClose={vi.fn()} onFocus={onFocus} />
    act(() => { renderer = create(render(1)) })
    selectParentLink()
    act(() => button('Next flow step').props.onClick())
    expect(onNavigateView).toHaveBeenLastCalledWith(2)
    const cachedJourney = () => queryClient.setQueryData.mock.calls.filter(([key]) => key[0] === 'flow-navigation').slice(-1)[0]![1]
    let cached = cachedJourney()
    expect(cached.stack[0]).toMatchObject({ stepId: 'call', choices: { decision: 'b' } })
    queryClient.getQueryData.mockImplementation(key => key[0] === 'flow-navigation' ? cached : undefined)
    act(() => renderer.unmount())
    act(() => { renderer = create(render(2)) })
    expect(onFocus).toHaveBeenLastCalledWith({ viewId: 2, elementIds: [2], connectors: [] })
    act(() => button('Return to previous flow').props.onClick())
    expect(onNavigateView).toHaveBeenLastCalledWith(1)
    cached = cachedJourney()
    act(() => renderer.unmount())
    act(() => { renderer = create(render(1)) })
    expect(renderer.root.findByProps({ 'aria-label': 'Current flow step' }).props.value).toBe('call')
    act(() => button('Skip flow').props.onClick())
    expect(renderer.root.findByProps({ 'aria-label': 'Current flow step' }).props.value).toBe('end')
  })

  it('changes diagram focus for linked flows in the explorer without changing routes', () => {
    queryState.data = linkedFlows()
    const onFocus = vi.fn()
    act(() => { renderer = create(<FlowPanel open diagrams={{ 1: diagram, 2: diagram }} onClose={vi.fn()} onFocus={onFocus} />) })
    selectParentLink()
    act(() => button('Go to flow').props.onClick())
    expect(onFocus).toHaveBeenLastCalledWith({ viewId: 2, elementIds: [2], connectors: [] })
    act(() => button('Return to previous flow').props.onClick())
    expect(renderer.root.findByProps({ 'aria-label': 'Current flow step' }).props.value).toBe('call')
    expect(onFocus).toHaveBeenLastCalledWith({ viewId: 1, elementIds: [], connectors: [] })
  })

  it('renders a left-to-right SVG minimap in playback navigation, highlights the active step, and selects steps on click', () => {
    queryState.data = linkedFlows()
    const onFocus = vi.fn()
    act(() => { renderer = create(<FlowPanel open viewId={1} diagram={diagram} onClose={vi.fn()} onFocus={onFocus} />) })
    const nav = renderer.root.findByProps({ 'data-testid': 'flow-playback-navigation' })
    const minimap = nav.findByProps({ 'data-testid': 'flow-playback-minimap' })
    expect(minimap.type).toBe('svg')

    const steps = () => minimap.findAllByProps({ 'data-testid': 'flow-minimap-step' })
    expect(steps().map(node => node.props['data-step-id'])).toEqual(['decision', 'call', 'end'])
    expect(steps().filter(node => node.props['data-active'] === 'true')).toHaveLength(0)

    act(() => steps().find(node => node.props['data-step-id'] === 'call')!.props.onClick())
    expect(renderer.root.findByProps({ 'aria-label': 'Current flow step' }).props.value).toBe('call')
    const active = steps().find(node => node.props['data-step-id'] === 'call')!
    expect(active.props['data-active']).toBe('true')
    expect(active.props['aria-current']).toBe('step')
    expect(active.findAllByProps({ 'data-testid': 'flow-minimap-active-halo' })).toHaveLength(1)
    expect(steps().find(node => node.props['data-step-id'] === 'decision')!.props['data-visited']).toBe('true')

    const waypoint = minimap.findByProps({ 'data-testid': 'flow-minimap-waypoint' })
    act(() => waypoint.props.onClick())
    expect(renderer.root.findByProps({ 'aria-label': 'Current flow step' }).props.value).toBe('decision')
    expect(renderer.root.findByProps({ 'aria-label': 'Choose flow path' }).props.value).toBe('a')
  })
})
