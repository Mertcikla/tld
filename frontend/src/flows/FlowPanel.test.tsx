import { act, create, type ReactTestInstance, type ReactTestRenderer } from 'react-test-renderer'
import { useState } from 'react'
import { afterEach, describe, expect, it, vi } from 'vitest'
import { FlowPanel, type FlowPanelProps } from './FlowPanel'
import { flowApi } from './api'
import type { Connector } from '../types'
import type { FlowDiagram, StoryFlow } from './model'

vi.mock('@chakra-ui/react', () => ({ FormControl: 'div', FormLabel: 'label', Box: 'div', Button: 'button', Checkbox: 'checkbox', HStack: 'div', Input: 'input', Select: 'select', Tag: 'tag', TagLabel: 'taglabel', TagCloseButton: 'button', Text: 'p', Textarea: 'textarea', VStack: 'div', Tooltip: ({ children }: { children: React.ReactNode }) => children }))
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
const button = (label: string) => renderer.root.findAllByType('button').find(node => node.props.children === label)!
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
  it('saves multiple elements and connections with optional metadata and independent directions', async () => {
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
    change(renderer.root.findByProps({ 'aria-label': 'Metadata key' }), 'owner')
    act(() => button('Add').props.onClick())
    change(renderer.root.findByProps({ 'aria-label': 'Metadata owner' }), 'Commerce')
    const expected = { title: 'Save and queue', elementIds: [1, 2], connectors: [{ id: 10, reverse: true }, { id: 11, reverse: false }], metadata: { owner: 'Commerce' } }
    expect(onFocus).toHaveBeenLastCalledWith({ viewId: 1, elementIds: [1, 2, 3], connectors: expected.connectors })
    vi.mocked(flowApi.save).mockImplementation(async flow => ({ ...flow, id: 'saved', revision: 1 }))
    await act(async () => button('Save flow').props.onClick())
    expect(flowApi.save).toHaveBeenCalledWith(expect.objectContaining({ steps: [expect.objectContaining(expected)] }))
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
    expect(renderer.root.findAllByType('checkbox' as never)).toHaveLength(0)
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
    expect(renderer.root.findAllByType('checkbox' as never)).toHaveLength(5)
    check(checkbox('Service 0'), true)
    searchHighlights('service 7')
    expect(renderer.root.findAllByType('checkbox' as never)).toHaveLength(1)
    check(checkbox('Service 7'), true)
    act(() => renderer.root.findByProps({ 'aria-label': 'Show connections' }).props.onClick())
    expect(renderer.root.findAllByProps({ 'data-testid': 'highlight-search-results' })).toHaveLength(0)
    searchHighlights('message')
    expect(renderer.root.findAllByType('checkbox' as never)).toHaveLength(5)
    check(checkbox('Message 0'), true)
    act(() => renderer.root.findByProps({ 'aria-label': 'Reverse Service 0 → Service 0 · Message 0' }).props.onClick())
    searchHighlights('message 7')
    check(checkbox('Message 7'), true)
    searchHighlights('')
    expect(renderer.root.findAllByType('checkbox' as never)).toHaveLength(0)
    expect(renderer.root.findByProps({ 'aria-label': 'Reverse Service 0 → Service 0 · Message 0' }).props['aria-pressed']).toBe(true)
    expect(renderer.root.findByProps({ 'aria-label': 'Remove connection Service 0 → Service 7 · Message 7' })).toBeDefined()
    searchHighlights('no such connection')
    expect(renderer.root.findAllByType('checkbox' as never)).toHaveLength(0)
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

  it('keeps playback out of editing and preserves the draft when closing and reopening', () => {
    mount()
    expect(button('Auto-play')).toBeUndefined()
    const props = renderer.root.findByType(FlowPanel).props
    act(() => renderer.update(<FlowPanel {...props} open={false} />))
    act(() => renderer.update(<FlowPanel {...props} open />))
    expect(renderer.root.findByProps({ 'aria-label': 'Flow name' }).props.value).toBe('New flow')
    expect(button('Save flow').props.isDisabled).toBe(false)
  })

  it('records a new step for each click, including repeats, from an empty flow', async () => {
    const onCanvasRecorderChange = vi.fn()
    const onFocus = mount(undefined, { onCanvasRecorderChange })
    act(() => button('Remove').props.onClick())
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

  it('shrinks the compact panel while recording and restores its controls after stopping', () => {
    const onCanvasRecorderChange = vi.fn()
    mount(undefined, { compact: true, onCanvasRecorderChange })
    act(() => button('Record steps').props.onClick())
    expect(renderer.root.findByProps({ 'data-flow-scroll': true }).props.display).toBe('none')
    expect(button('Save flow')).toBeUndefined()
    expect(renderer.root.findAllByProps({ 'data-testid': 'flow-yaml-footer' })).toHaveLength(0)
    act(() => onCanvasRecorderChange.mock.lastCall![0].stop())
    expect(renderer.root.findByProps({ 'data-flow-scroll': true }).props.display).toBe('flex')
    expect(button('Save flow')).toBeDefined()
  })

  it('Escape stops recording before closing and Cmd/Ctrl+S saves from an input', async () => {
    const onClose = vi.fn()
    mount(undefined, { onCanvasRecorderChange: vi.fn(), onClose })
    act(() => button('Record steps').props.onClick())
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
    const listener = vi.mocked(window.addEventListener).mock.calls.filter(([event]) => event === 'keydown').at(-1)![1] as (event: unknown) => void
    act(() => listener({ key: 'Escape', preventDefault: vi.fn() }))
    expect(onCanvasRecorderChange).toHaveBeenLastCalledWith(null)
    expect(onClose).not.toHaveBeenCalled()
  })

  it('restores unsaved work after navigating away from a diagram', () => {
    mount()
    change(renderer.root.findByProps({ 'aria-label': 'Step title' }), 'Keep this step')
    const cached = queryClient.setQueryData.mock.calls.filter(([key]) => key[0] === 'flow-draft').at(-1)![1]
    act(() => renderer.unmount())
    queryClient.getQueryData.mockReturnValue(cached)
    act(() => { renderer = create(<FlowPanel open viewId={1} editMode diagram={diagram} onClose={vi.fn()} onFocus={vi.fn()} />) })
    expect(renderer.root.findByProps({ 'aria-label': 'Flow name' }).props.value).toBe('New flow')
    expect(button('Save flow').props.isDisabled).toBe(false)
    act(() => button('Discard').props.onClick())
    expect(queryClient.removeQueries).toHaveBeenLastCalledWith({ queryKey: ['flow-draft', 1], exact: true })
  })

  it('keeps New flow visible, previews a new draft, and edits only after an explicit action', () => {
    vi.stubGlobal('window', { addEventListener: vi.fn(), removeEventListener: vi.fn(), confirm: vi.fn(() => true) })
    function ControlledPanel() {
      const [editing, setEditing] = useState(false)
      return <FlowPanel open viewId={1} editMode={editing} onEditModeChange={setEditing} diagram={diagram} onClose={vi.fn()} onFocus={vi.fn()} />
    }
    act(() => { renderer = create(<ControlledPanel />) })
    expect(button('+ New flow')).toBeDefined()
    expect(button('Edit flow')).toBeUndefined()
    expect(button('Edit')).toBeUndefined()
    expect(button('Preview')).toBeUndefined()
    act(() => button('+ New flow').props.onClick())
    expect(button('Edit flow')).toBeDefined()
    expect(renderer.root.findAllByProps({ 'aria-label': 'Flow name' })).toHaveLength(0)
    act(() => button('Edit flow').props.onClick())
    expect(button('+ New flow')).toBeDefined()
    expect(renderer.root.findByProps({ 'aria-label': 'Flow name' }).props.value).toBe('New flow')
    act(() => button('Preview').props.onClick())
    expect(button('Edit flow')).toBeDefined()
    expect(renderer.root.findAllByProps({ 'aria-label': 'Flow name' })).toHaveLength(0)
  })

  it('waits for a selection and previews each flow selected from the chooser', () => {
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
    change(renderer.root.findByProps({ 'aria-label': 'Choose flow' }), 'a')
    expect(button('Edit flow')).toBeDefined()
    act(() => button('Edit flow').props.onClick())
    expect(renderer.root.findByProps({ 'aria-label': 'Flow name' }).props.value).toBe('First story')
    change(renderer.root.findByProps({ 'aria-label': 'Choose flow' }), 'b')
    expect(renderer.root.findAllByProps({ 'aria-label': 'Flow name' })).toHaveLength(0)
    expect(button('Edit flow')).toBeDefined()
    change(renderer.root.findByProps({ 'aria-label': 'Choose flow' }), '')
    expect(button('Edit flow')).toBeUndefined()
    expect(button('+ New flow')).toBeDefined()
  })

})
