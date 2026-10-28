import { act, create, type ReactTestInstance, type ReactTestRenderer } from 'react-test-renderer'
import { useState } from 'react'
import { useQuery } from '@tanstack/react-query'
import { afterEach, describe, expect, it, vi } from 'vitest'
import { FlowPanel, type FlowPanelProps } from './FlowPanel'
import { flowApi } from './api'
import { api } from '../api/client'
import { FlowYamlControls } from './FlowYamlControls'
import { exportCompleteFlowYaml } from './yaml'
import type { Connector } from '../types'
import { FLOW_MAX_STEPS, FLOW_MAX_PATHS, FLOW_MAX_BRANCH_DEPTH, newStep, type FlowDiagram, type StoryFlow } from './model'

vi.mock('@chakra-ui/react', () => ({ Badge: 'badge', FormControl: 'div', FormLabel: 'label', Box: 'div', Button: 'button', Checkbox: 'checkbox', Collapse: ({ in: expanded, children }: { in: boolean; children: React.ReactNode }) => expanded ? children : null, HStack: 'div', IconButton: 'button', Input: 'input', Menu: 'div', MenuButton: 'button', MenuItem: 'menuitem', MenuList: 'div', Portal: ({ children }: { children: React.ReactNode }) => children, Select: 'select', Tag: 'tag', TagLabel: 'taglabel', TagCloseButton: 'button', Text: 'p', Textarea: 'textarea', VStack: 'div', Tooltip: ({ children }: { children: React.ReactNode }) => children }))
vi.mock('framer-motion', () => ({ motion: { div: 'div' }, Reorder: { Group: 'reorder-group', Item: 'reorder-item' }, useDragControls: () => ({ start: vi.fn() }) }))
vi.mock('./FlowYamlControls', () => ({ FlowYamlControls: () => null }))
vi.mock('./api', () => ({ flowApi: { list: vi.fn(), save: vi.fn(), remove: vi.fn() } }))
const { queryClient, queryState } = vi.hoisted(() => ({ queryClient: { setQueryData: vi.fn(), invalidateQueries: vi.fn(), getQueryData: vi.fn(), setQueryDefaults: vi.fn(), removeQueries: vi.fn() }, queryState: { data: [] as StoryFlow[], isLoading: false, error: null as Error | null, refetch: vi.fn() } }))
vi.mock('@tanstack/react-query', () => ({ useQueryClient: () => queryClient, useQuery: vi.fn(() => queryState) }))
const diagram: FlowDiagram = {
  elements: [{ element_id: 1, name: 'API' }, { element_id: 2, name: 'DB' }, { element_id: 3, name: 'Worker' }],
  connectors: [
    { id: 10, source_element_id: 1, target_element_id: 2, label: 'Stores' },
    { id: 11, source_element_id: 1, target_element_id: 3, label: 'Queues' },
  ] as Connector[],
}
let renderer: ReactTestRenderer
const button = (label: string) => renderer.root.findAllByType('button').find(node => node.props.children === label || node.props['aria-label'] === label)!
const menuItem = (label: string) => renderer.root.findAllByType('menuitem' as never).find(node => node.props['aria-label'] === label)!
const checkbox = (label: string) => renderer.root.findAllByType('checkbox' as never).find(node => node.props.children === label || node.props['aria-label'] === label)!
const change = (node: ReactTestInstance, value: string) => {
  if (node.props['data-inline-preview']) {
    const label = node.props['aria-label']
    act(() => node.props.onClick())
    node = renderer.root.findByProps({ 'aria-label': label })
  }
  act(() => node.props.onChange({ target: { value } }))
}
const searchHighlights = (value: string) => change(renderer.root.findByProps({ 'aria-label': 'Search highlights' }), value)
const currentDraft = () => queryClient.setQueryData.mock.calls.filter(([key]) => key[0] === 'flow-draft').at(-1)![1] as StoryFlow
function mount(selection?: { elementId?: number; connectorId?: number }, extra: Partial<FlowPanelProps> = {}) {
  vi.stubGlobal('window', { addEventListener: vi.fn(), removeEventListener: vi.fn(), confirm: vi.fn(() => true), location: { protocol: 'http:', hostname: 'localhost', port: '5173' } })
  const onFocus = vi.fn()
  act(() => { renderer = create(<FlowPanel open viewId={1} editMode diagram={diagram} selection={selection} onClose={vi.fn()} onFocus={onFocus} {...extra} />) })
  act(() => button('+ New flow').props.onClick())
  act(() => button('Add step').props.onClick())
  return onFocus
}
afterEach(() => { act(() => renderer?.unmount()); vi.unstubAllGlobals(); vi.clearAllMocks(); queryState.data = []; queryState.error = null; queryClient.getQueryData.mockReset() })

describe('flow step editing', () => {
  it('uses only the snapshot catalog, bypassing workspace queries, drafts, journeys, and editing', () => {
    const snapshot: StoryFlow = { id: 'snapshot-flow-0', viewId: 1, name: 'Shared snapshot', description: '', revision: 0, steps: [] }
    const workspace: StoryFlow = { ...snapshot, id: 'private-flow', name: 'Private workspace flow' }
    queryState.data = [workspace]
    queryState.error = new Error('Unrelated workspace error')
    queryClient.getQueryData.mockReturnValue(workspace)
    const onFocus = vi.fn()
    act(() => { renderer = create(<FlowPanel open docked editMode snapshotCatalog={{ flows: [snapshot], views: [{ id: 1, name: 'Snapshot' }] }} diagrams={{ 1: diagram }} onClose={vi.fn()} onFocus={onFocus} onEditModeChange={vi.fn()} />) })
    expect(vi.mocked(useQuery).mock.calls.every(([options]) => !options.enabled)).toBe(true)
    expect(queryClient.getQueryData).not.toHaveBeenCalled()
    expect(queryClient.setQueryData).not.toHaveBeenCalled()
    expect(button('Save flow')).toBeUndefined()
    expect(renderer.root.findAllByProps({ role: 'alert' })).toHaveLength(0)
    expect(renderer.root.findByType(FlowYamlControls).props.flow).toBe(snapshot)
    expect(renderer.root.findByType(FlowYamlControls).props.canImport).toBe(false)
    expect(onFocus).toHaveBeenCalledWith({ viewId: 1, elementIds: [], connectorIds: [] })
  })
  it('compacts and expands the flow description without changing the saved text', async () => {
    mount()
    const value = 'Request overview\n\nDetails about the round trip.'
    change(renderer.root.findByProps({ 'aria-label': 'Flow description' }), value)
    act(() => button('Compact flow description').props.onClick())
    expect(button('Expand flow description').props['aria-expanded']).toBe(false)
    const preview = renderer.root.findByProps({ 'aria-label': 'Flow description' })
    expect(preview.type).toBe('button')
    expect(preview.findByProps({ isTruncated: true }).props.children).toBe(value)
    expect(renderer.root.findAllByType('textarea').some(node => node.props['aria-label'] === 'Flow description')).toBe(false)
    act(() => button('Expand flow description').props.onClick())
    expect(button('Compact flow description').props['aria-expanded']).toBe(true)
    act(() => button('Compact flow description').props.onClick())
    act(() => button('Flow description').props.onClick())
    expect(button('Compact flow description').props['aria-expanded']).toBe(true)
    expect(renderer.root.findByProps({ 'aria-label': 'Flow description' }).props.value).toBe(value)
    vi.mocked(flowApi.save).mockImplementation(async flow => ({ ...flow, id: 'saved', revision: 1 }))
    await act(async () => button('Save flow').props.onClick())
    expect(flowApi.save).toHaveBeenCalledWith(expect.objectContaining({ description: value }))
  })

  it('fits the description to its content and refits when the panel width changes', () => {
    vi.stubGlobal('window', { addEventListener: vi.fn(), removeEventListener: vi.fn() })
    let resize = () => {}
    const disconnect = vi.fn()
    vi.stubGlobal('ResizeObserver', class {
      constructor(callback: () => void) { resize = callback }
      observe = vi.fn()
      disconnect = disconnect
    })
    const field = { style: { height: '' }, scrollHeight: 110, offsetHeight: 34, clientHeight: 32, clientWidth: 280 }
    queryState.data = [{ id: 'saved', viewId: 1, name: 'Story', description: 'Long description', revision: 1, steps: [] }]
    act(() => { renderer = create(<FlowPanel open docked editMode viewId={1} diagram={diagram} onClose={vi.fn()} onFocus={vi.fn()} />, {
      createNodeMock: element => element.type === 'textarea' ? field : null,
    }) })
    expect(field.style.height).toBe('112px')
    field.clientWidth = 180
    field.scrollHeight = 170
    act(() => resize())
    expect(field.style.height).toBe('172px')
    field.scrollHeight = 30
    change(renderer.root.findByProps({ 'aria-label': 'Flow description' }), 'Short')
    expect(field.style.height).toBe('32px')
    expect(disconnect).toHaveBeenCalled()
  })

  it('can compact and expand multiline descriptions during playback', () => {
    const value = 'Overview\n\nMore details'
    queryState.data = [{ id: 'saved', viewId: 1, name: 'Story', description: value, revision: 1, steps: [] }]
    act(() => { renderer = create(<FlowPanel open viewId={1} diagram={diagram} onClose={vi.fn()} onFocus={vi.fn()} />) })
    const description = () => renderer.root.findByProps({ 'aria-label': 'Flow description' })
    expect(description().props.children).toBe(value)
    expect(description().props.whiteSpace).toBe('pre-wrap')
    act(() => button('Compact flow description').props.onClick())
    expect(description().type).toBe('button')
    expect(description().findByProps({ isTruncated: true }).props.children).toBe(value)
    act(() => button('Expand flow description').props.onClick())
    expect(description().props.whiteSpace).toBe('pre-wrap')
    expect(queryClient.setQueryData).not.toHaveBeenCalled()
  })

  it('edits the title in the card header and the description in place', async () => {
    mount({ elementId: 1 })
    const header = renderer.root.findByProps({ 'data-flow-step-header': true })
    expect(header.findByProps({ 'aria-label': 'Step title' }).type).toBe('button')
    expect(renderer.root.findAllByType('p').some(node => node.props.children === 'Story')).toBe(false)
    change(header.findByProps({ 'aria-label': 'Step title' }), 'Receive request')
    const title = renderer.root.findByProps({ 'aria-label': 'Step title' })
    expect(title.type).toBe('input')
    act(() => title.props.onKeyDown({ key: 'Enter', preventDefault: vi.fn(), stopPropagation: vi.fn() }))
    expect(header.findByProps({ 'aria-label': 'Step title' }).type).toBe('button')
    change(renderer.root.findByProps({ 'aria-label': 'Step description' }), 'First line\nSecond line')
    const description = renderer.root.findByProps({ 'aria-label': 'Step description' })
    expect(description.type).toBe('textarea')
    const preventDefault = vi.fn()
    act(() => description.props.onKeyDown({ key: 'Enter', preventDefault, stopPropagation: vi.fn() }))
    expect(preventDefault).not.toHaveBeenCalled()
    act(() => description.props.onKeyDown({ key: 'Enter', ctrlKey: true, preventDefault, stopPropagation: vi.fn() }))
    expect(renderer.root.findByProps({ 'aria-label': 'Step description' }).type).toBe('button')
    vi.mocked(flowApi.save).mockImplementation(async flow => ({ ...flow, id: 'saved', revision: 1 }))
    await act(async () => button('Save flow').props.onClick())
    expect(vi.mocked(flowApi.save).mock.lastCall![0].steps[0]).toMatchObject({ title: 'Receive request', description: 'First line\nSecond line', elementIds: [1] })
  })

  it('cancels inline edits with Escape without closing the panel', () => {
    const onClose = vi.fn()
    mount(undefined, { onClose })
    change(renderer.root.findByProps({ 'aria-label': 'Step title' }), 'Original')
    act(() => renderer.root.findByProps({ 'aria-label': 'Step title' }).props.onBlur())
    for (const label of ['Step title', 'Step description']) {
      change(renderer.root.findByProps({ 'aria-label': label }), 'Discard this edit')
      const stopPropagation = vi.fn()
      act(() => renderer.root.findByProps({ 'aria-label': label }).props.onKeyDown({ key: 'Escape', preventDefault: vi.fn(), stopPropagation }))
      expect(stopPropagation).toHaveBeenCalledOnce()
      expect(renderer.root.findByProps({ 'aria-label': label }).type).toBe('button')
    }
    expect(currentDraft().steps[0]).toMatchObject({ title: 'Original', description: '' })
    expect(onClose).not.toHaveBeenCalled()
  })

  it('saves multiple elements and connections without direction controls', async () => {
    const onFocus = mount()
    expect(renderer.root.findAllByProps({ 'aria-label': 'New step type' })).toHaveLength(0)
    searchHighlights('api')
    act(() => button('Add element API').props.onClick())
    searchHighlights('db')
    act(() => button('Add element DB').props.onClick())
    searchHighlights('api')
    act(() => button('Add connection API → DB · Stores').props.onClick())
    searchHighlights('api')
    act(() => button('Add connection API → Worker · Queues').props.onClick())
    expect(renderer.root.findAllByType('button').some(node => node.props['aria-label']?.startsWith('Reverse '))).toBe(false)
    change(renderer.root.findByProps({ 'aria-label': 'Step title' }), 'Save and queue')
    const expected = { title: 'Save and queue', elementIds: [1, 2], connectorIds: [10, 11] }
    expect(onFocus).toHaveBeenLastCalledWith({ viewId: 1, elementIds: [1, 2, 3], connectorIds: expected.connectorIds })
    vi.mocked(flowApi.save).mockImplementation(async flow => ({ ...flow, id: 'saved', revision: 1 }))
    await act(async () => button('Save flow').props.onClick())
    expect(flowApi.save).toHaveBeenCalledWith(expect.objectContaining({ steps: [expect.objectContaining(expected)] }))
  })

  it('reorders whole steps with the handle keyboard controls and preserves the active editor', async () => {
    mount({ elementId: 2, connectorId: 10 })
    change(renderer.root.findByProps({ 'aria-label': 'Step title' }), 'Store data')
    act(() => button('Add step').props.onClick())
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
    expect(vi.mocked(flowApi.save).mock.lastCall![0].steps[1]).toMatchObject({ elementIds: [2], connectorIds: [10] })
  })

  it('accepts drag order and duplicates or removes a collapsed step without opening it first', async () => {
    mount({ elementId: 1 })
    change(renderer.root.findByProps({ 'aria-label': 'Step title' }), 'First')
    act(() => button('Add step').props.onClick())
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
    expect(renderer.root.findByProps({ 'aria-label': 'Remove connection API → DB · Stores' })).toBeDefined()
    act(() => renderer.root.findByProps({ 'aria-label': 'Remove connection API → DB · Stores' }).props.onClick())
    expect(onFocus).toHaveBeenLastCalledWith({ viewId: 1, elementIds: [], connectorIds: [] })
    vi.mocked(flowApi.save).mockImplementation(async (flow: StoryFlow) => ({ ...flow, id: 'saved', revision: 1 }))
    await act(async () => button('Save flow').props.onClick())
    expect(flowApi.save).toHaveBeenCalledWith(expect.objectContaining({ steps: [expect.objectContaining({ elementIds: [], connectorIds: [], title: '', description: '' })] }))
  })
  it('searches both kinds, adds one result at a time and excludes selected highlights', () => {
    mount()
    expect(renderer.root.findAllByProps({ 'data-testid': 'highlight-search-results' })).toHaveLength(0)
    expect(renderer.root.findAllByType('checkbox' as never)).toHaveLength(0)
    searchHighlights('api')
    expect(button('Add element API')).toBeDefined()
    expect(button('Add connection API → DB · Stores')).toBeDefined()
    expect(button('Add connection API → Worker · Queues')).toBeDefined()
    act(() => button('Add element API').props.onClick())
    expect(renderer.root.findByProps({ 'aria-label': 'Search highlights' }).props.value).toBe('')
    expect(renderer.root.findAllByProps({ 'data-testid': 'highlight-search-results' })).toHaveLength(0)
    searchHighlights('api')
    expect(button('Add element API')).toBeUndefined()
    act(() => button('Add connection API → DB · Stores').props.onClick())
    expect(renderer.root.findByProps({ 'aria-label': 'Selected highlights' }).findAllByType('button')).toHaveLength(2)
    searchHighlights('db')
    act(() => button('Add element DB').props.onClick())
    searchHighlights('  ')
    expect(renderer.root.findByProps({ 'aria-label': 'Remove element API' })).toBeDefined()
    expect(renderer.root.findByProps({ 'aria-label': 'Remove element DB' })).toBeDefined()
    expect(renderer.root.findByProps({ 'aria-label': 'Remove connection API → DB · Stores' })).toBeDefined()
    searchHighlights('api')
    expect(button('Add connection API → DB · Stores')).toBeUndefined()
    expect(button('Add connection API → Worker · Queues')).toBeDefined()
  })

  it('caps combined results at five and finds items beyond the first matches', () => {
    const largeDiagram: FlowDiagram = {
      elements: Array.from({ length: 8 }, (_, id) => ({ element_id: id + 1, name: `Service ${id}` })),
      connectors: Array.from({ length: 8 }, (_, id) => ({ id: id + 10, source_element_id: 1, target_element_id: id + 1, label: `Message ${id}` })) as Connector[],
    }
    mount(undefined, { diagram: largeDiagram })
    const results = () => renderer.root.findByProps({ 'data-testid': 'highlight-search-results' }).findAllByProps({ role: 'option' })
    searchHighlights('service')
    expect(results()).toHaveLength(5)
    act(() => button('Add element Service 0').props.onClick())
    searchHighlights('service 7')
    expect(results()).toHaveLength(2)
    act(() => button('Add element Service 7').props.onClick())
    searchHighlights('message')
    expect(results()).toHaveLength(5)
    act(() => button('Add connection Service 0 → Service 0 · Message 0').props.onClick())
    searchHighlights('message 7')
    act(() => button('Add connection Service 0 → Service 7 · Message 7').props.onClick())
    expect(renderer.root.findByProps({ 'aria-label': 'Remove connection Service 0 → Service 0 · Message 0' })).toBeDefined()
    expect(renderer.root.findByProps({ 'aria-label': 'Remove connection Service 0 → Service 7 · Message 7' })).toBeDefined()
    searchHighlights('no such connection')
    expect(results()).toHaveLength(0)
  })

  it('keeps connection labels concise while searching labels and endpoint names', () => {
    mount({ connectorId: 10 })
    const selected = renderer.root.findByProps({ 'aria-label': 'Selected highlights' })
    expect(selected.findAllByType('p').some(node => node.props.children === 'Stores')).toBe(true)
    expect(selected.findAllByType('p').some(node => node.props.children === 'API → DB')).toBe(true)
    searchHighlights('api')
    expect(button('Add connection API → DB · Stores')).toBeUndefined()
    expect(button('Add connection API → Worker · Queues')).toBeDefined()
    searchHighlights('queues')
    expect(button('Add connection API → Worker · Queues')).toBeDefined()
    act(() => button('Remove connection API → DB · Stores').props.onClick())
    searchHighlights('stores')
    expect(button('Add connection API → DB · Stores')).toBeDefined()
  })

  it('shows technology icons and connection markers in results and selected highlights', () => {
    const iconDiagram: FlowDiagram = { ...diagram, elements: [
      { ...diagram.elements[0], kind: 'api', technology_connectors: [{ type: 'catalog', slug: 'typescript', label: 'TypeScript', is_primary_icon: true }] },
      ...diagram.elements.slice(1),
    ] }
    mount(undefined, { diagram: iconDiagram })
    searchHighlights('api')
    const api = button('Add element API')
    expect(api.findByProps({ as: 'img' }).props.src).toBe('/icons/typescript.svg')
    const connection = button('Add connection API → DB · Stores')
    expect(connection.findAllByType('p').some(node => node.props.children === '→')).toBe(true)
    act(() => api.props.onClick())
    searchHighlights('stores')
    act(() => button('Add connection API → DB · Stores').props.onClick())
    const selected = renderer.root.findByProps({ 'aria-label': 'Selected highlights' })
    expect(selected.findByProps({ as: 'img' }).props.src).toBe('/icons/typescript.svg')
    expect(selected.findAllByType('p').some(node => node.props.children === '→')).toBe(true)
    expect(selected.findAllByType('p').some(node => node.props.children === 'API → DB')).toBe(true)
  })

  it('adds keyboard-selected results and clears search with Escape without closing the panel', () => {
    const onClose = vi.fn()
    mount(undefined, { onClose })
    const input = () => renderer.root.findByProps({ 'aria-label': 'Search highlights' })
    const key = (value: string) => act(() => input().props.onKeyDown({ key: value, preventDefault: vi.fn(), stopPropagation: vi.fn() }))
    searchHighlights('api')
    key('ArrowDown')
    key('Enter')
    expect(currentDraft().steps[0]).toMatchObject({ elementIds: [], connectorIds: [10] })
    expect(input().props.value).toBe('')
    searchHighlights('api')
    key('Enter')
    expect(currentDraft().steps[0].elementIds).toEqual([1])
    searchHighlights('api')
    expect(button('Add element API')).toBeUndefined()
    key('Escape')
    expect(input().props.value).toBe('')
    expect(onClose).not.toHaveBeenCalled()
  })

  it('keeps unavailable highlights removable in the unified selected list', () => {
    mount({ elementId: 1, connectorId: 10 })
    const props = renderer.root.findByType(FlowPanel).props
    act(() => renderer.update(<FlowPanel {...props} diagram={{ elements: [], connectors: [] }} />))
    expect(button('Remove element Unavailable element #1')).toBeDefined()
    expect(button('Remove connection Unavailable connection #10')).toBeDefined()
    act(() => button('Remove element Unavailable element #1').props.onClick())
    act(() => button('Remove connection Unavailable connection #10').props.onClick())
    expect(currentDraft().steps[0]).toMatchObject({ elementIds: [], connectorIds: [] })
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
    act(() => button('Add step').props.onClick())
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

  it('adjusts the autoplay interval with bounded stepper buttons', () => {
    mount()
    const interval = () => renderer.root.findByProps({ role: 'group', 'aria-label': 'Autoplay interval' }).findByProps({ as: 'output' }).props.children.join('')
    expect(interval()).toBe('3s')
    for (const value of ['2s', '1s']) {
      act(() => button('Decrease autoplay interval').props.onClick())
      expect(interval()).toBe(value)
    }
    expect(button('Decrease autoplay interval').props.isDisabled).toBe(true)
    for (const value of ['2s', '3s', '5s', '8s']) {
      act(() => button('Increase autoplay interval').props.onClick())
      expect(interval()).toBe(value)
    }
    expect(button('Increase autoplay interval').props.isDisabled).toBe(true)
    expect(renderer.root.findAllByType('select').some(node => node.props['aria-label'] === 'Autoplay interval')).toBe(false)
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
    expect(onFocus).toHaveBeenLastCalledWith({ viewId: 1, elementIds: [1, 2], connectorIds: [10] })
    capture({ elementId: 1 })
    capture({ elementId: 99 })
    expect(button('Stop recording').props['aria-pressed']).toBe(true)
    act(() => onCanvasRecorderChange.mock.lastCall![0].stop())
    expect(onCanvasRecorderChange).toHaveBeenLastCalledWith(null)
    vi.mocked(flowApi.save).mockImplementation(async flow => ({ ...flow, id: 'saved', revision: 1 }))
    await act(async () => button('Save flow').props.onClick())
    const steps = vi.mocked(flowApi.save).mock.lastCall![0].steps
    expect(steps).toEqual([
      expect.objectContaining({ elementIds: [1], connectorIds: [] }),
      expect.objectContaining({ elementIds: [], connectorIds: [10] }),
      expect.objectContaining({ elementIds: [1], connectorIds: [] }),
    ])
    expect(new Set(steps.map(step => step.id)).size).toBe(3)
  })

  it('picks highlights into the existing step without replacing its highlights', async () => {
    const onCanvasRecorderChange = vi.fn()
    const onClose = vi.fn()
    const step = { id: 'original', title: 'Original title', description: '', elementIds: [1], connectorIds: [10] }
    queryClient.getQueryData.mockReturnValue({ id: 'saved', revision: 1, viewId: 1, name: 'Story', description: '', steps: [step] })
    vi.stubGlobal('window', { addEventListener: vi.fn(), removeEventListener: vi.fn(), confirm: vi.fn(() => true) })
    act(() => { renderer = create(<FlowPanel open viewId={1} editMode diagram={diagram} onCanvasRecorderChange={onCanvasRecorderChange} onClose={onClose} onFocus={vi.fn()} />) })
    act(() => button('Toggle 1: Original title').props.onClick())
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
    expect(flowApi.save).toHaveBeenCalledWith(expect.objectContaining({ steps: [{ ...step, elementIds: [1, 2], connectorIds: [10, 11] }] }))
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
    act(() => button('Add step').props.onClick())
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

  it('stops at the step limit and prevents recording beyond the total step limit', () => {
    queryClient.getQueryData.mockReturnValue({ id: '', viewId: 1, name: 'Full story', description: '', revision: 0,
      steps: Array.from({ length: FLOW_MAX_STEPS - 1 }, (_, index) => ({ id: String(index), title: '', description: '', elementIds: [1], connectorIds: [] })) })
    const onCanvasRecorderChange = vi.fn()
    vi.stubGlobal('window', { addEventListener: vi.fn(), removeEventListener: vi.fn(), confirm: vi.fn(() => true) })
    act(() => { renderer = create(<FlowPanel open viewId={1} editMode diagram={diagram} onCanvasRecorderChange={onCanvasRecorderChange} onClose={vi.fn()} onFocus={vi.fn()} />) })
    act(() => button('Record steps').props.onClick())
    act(() => onCanvasRecorderChange.mock.lastCall![0].capture({ elementId: 1 }))
    expect(onCanvasRecorderChange).toHaveBeenLastCalledWith(null)
    expect(button('Record steps').props.isDisabled).toBe(true)
    expect(button('Add step').props.isDisabled).toBe(true)
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
    act(() => button('Add step').props.onClick())
    expect(options().find(node => node.props['aria-label'] === 'New flow, 2 steps')).toBeDefined()
    act(() => button('Remove step 2').props.onClick())
    act(() => button('Remove step 1').props.onClick())
    expect(options().find(node => node.props['aria-label'] === 'New flow, 0 steps')).toBeDefined()
  })

  it('shows playback and flow management while editing and duplicates into a draft', () => {
    vi.stubGlobal('window', { addEventListener: vi.fn(), removeEventListener: vi.fn(), confirm: vi.fn(() => true) })
    const original: StoryFlow = { id: 'saved', viewId: 1, name: 'Story', description: '', revision: 1, steps: [{ id: 's1', title: 'API', description: '', elementIds: [1], connectorIds: [] }] }
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
    expect(selected('First story')).toBeUndefined()
    expect(selected('Second story')).toBeUndefined()
    expect(button('Choose flow').props.isDisabled).toBe(false)
  })

})

describe('flow operation editing and navigation', () => {
  it('shows a styled hierarchical position beside the playback step selector', () => {
    queryState.data = linkedFlows()
    act(() => { renderer = create(<FlowPanel open viewId={1} diagram={diagram} onClose={vi.fn()} onFocus={vi.fn()} />) })
    const heading = () => renderer.root.findByProps({ 'data-testid': 'flow-playback-heading' })
    change(renderer.root.findByProps({ 'aria-label': 'Current flow step' }), 'decision')
    expect(heading().findByProps({ 'aria-label': 'Flow position 1.a' })).toBeDefined()
    change(renderer.root.findByProps({ 'aria-label': 'Choose flow path' }), 'b')
    expect(heading().findByProps({ 'aria-label': 'Flow position 1.b' })).toBeDefined()
    change(renderer.root.findByProps({ 'aria-label': 'Current flow step' }), 'call')
    expect(heading().findByProps({ 'aria-label': 'Flow position 1.b.1' })).toBeDefined()
  })

  it('caps path creation and duplication at the alphabet limit', () => {
    mount()
    act(() => menuItem('Add alternate paths').props.onClick())
    for (let i = 2; i < FLOW_MAX_PATHS; i++) act(() => button('Add path').props.onClick())
    expect(currentDraft().steps[0].branches?.paths).toHaveLength(FLOW_MAX_PATHS)
    expect(button('Add path').props.isDisabled).toBe(true)
    expect(button('Duplicate path Path A').props.isDisabled).toBe(true)
    expect(renderer.root.findByProps({ 'aria-label': 'Flow position 1.z' })).toBeDefined()
  })

  it('prevents another branch after the maximum nested depth', () => {
    let step = { ...newStep(diagram), id: 'leaf' }
    for (let depth = FLOW_MAX_BRANCH_DEPTH - 1; depth >= 0; depth--) step = {
      ...newStep(diagram), id: `branch-${depth}`, title: `Branch ${depth}`,
      branches: { mode: 'alternate', paths: [{ id: `a-${depth}`, name: 'A', steps: [step] }, { id: `b-${depth}`, name: 'B', steps: [] }] },
    }
    const draft: StoryFlow = { id: '', viewId: 1, name: 'Deep', revision: 0, description: '', steps: [step] }
    queryClient.getQueryData.mockImplementation(key => key[0] === 'flow-draft' ? draft : undefined)
    vi.stubGlobal('window', { addEventListener: vi.fn(), removeEventListener: vi.fn(), confirm: vi.fn(() => true) })
    act(() => { renderer = create(<FlowPanel open viewId={1} editMode diagram={diagram} onClose={vi.fn()} onFocus={vi.fn()} />) })
    for (let depth = 0; depth < FLOW_MAX_BRANCH_DEPTH; depth++) {
      change(renderer.root.findByProps({ 'aria-label': 'Current flow step' }), `branch-${depth}`)
      act(() => button(`Edit paths for Branch ${depth}`).props.onClick())
    }
    change(renderer.root.findByProps({ 'aria-label': 'Current flow step' }), 'leaf')
    expect(menuItem('Add alternate paths').props.isDisabled).toBe(true)
    expect(menuItem('Add parallel paths').props.isDisabled).toBe(true)
  })

  it('requests the global flow list and lists flows from other diagrams', async () => {
    queryState.data = [
      { id: 'local', viewId: 1, name: 'Local flow', description: '', revision: 1, steps: [] },
      { id: 'remote', viewId: 2, name: 'Other diagram flow', description: '', revision: 1, steps: [] },
    ]
    act(() => { renderer = create(<FlowPanel open viewId={1} diagram={diagram} onClose={vi.fn()} onFocus={vi.fn()} />) })
    const request = vi.mocked(useQuery).mock.calls.find(([options]) => options.queryKey[0] === 'flows')![0]
    expect(request.queryKey).toEqual(['flows', 0])
    await (request.queryFn as () => Promise<StoryFlow[]>)()
    expect(flowApi.list).toHaveBeenCalledWith()
    expect(menuItem('Local flow, 0 steps').props['aria-current']).toBe('true')
    expect(menuItem('Other diagram flow, 0 steps').props.isDisabled).toBe(false)
    expect(menuItem('Other diagram flow, 0 steps').props['aria-description']).toBe('Diagram #2')
  })

  it('opens another diagram and restores the selected flow for editing after remounting', async () => {
    vi.stubGlobal('window', { addEventListener: vi.fn(), removeEventListener: vi.fn(), confirm: vi.fn(() => true) })
    const local: StoryFlow = { id: 'local', viewId: 1, name: 'Local', description: '', revision: 1, steps: [] }
    const target: StoryFlow = { id: 'chosen', viewId: 2, name: 'Chosen', description: '', revision: 1, steps: [{ ...newStep(diagram), elementIds: [42] }] }
    queryState.data = [local, { ...target, id: 'other', name: 'Other' }, target]
    const onNavigateView = vi.fn(), onFocus = vi.fn()
    act(() => { renderer = create(<FlowPanel open viewId={1} editMode diagram={diagram} onNavigateView={onNavigateView} onClose={vi.fn()} onFocus={onFocus} />) })
    act(() => menuItem('Chosen, 1 step').props.onClick())
    expect(onNavigateView).toHaveBeenCalledWith(2)
    expect(onFocus.mock.calls.some(([focus]) => focus?.viewId === 2)).toBe(false)
    const visit = queryClient.setQueryData.mock.calls.filter(([key]) => key[0] === 'flow-navigation').at(-1)![1]
    queryClient.getQueryData.mockImplementation(key => key[0] === 'flow-navigation' ? visit : undefined)
    act(() => renderer.unmount())
    const targetDiagram: FlowDiagram = { elements: [{ element_id: 42, name: 'Remote API' }], connectors: [] }
    act(() => { renderer = create(<FlowPanel open viewId={2} editMode diagram={targetDiagram} onNavigateView={onNavigateView} onClose={vi.fn()} onFocus={onFocus} />) })
    expect(renderer.root.findByProps({ 'aria-label': 'Flow name' }).props.value).toBe('Chosen')
    change(renderer.root.findByProps({ 'aria-label': 'Current flow step' }), target.steps[0].id)
    expect(onFocus).toHaveBeenLastCalledWith({ viewId: 2, elementIds: [42], connectorIds: [] })
    change(renderer.root.findByProps({ 'aria-label': 'Flow name' }), 'Edited remote flow')
    vi.mocked(flowApi.save).mockImplementation(async flow => ({ ...flow, revision: 2 }))
    await act(async () => button('Save flow').props.onClick())
    expect(flowApi.save).toHaveBeenCalledWith(expect.objectContaining({ id: 'chosen', viewId: 2, name: 'Edited remote flow' }))
    expect(queryClient.setQueryData).toHaveBeenCalledWith(['flows', 0], expect.any(Function))
  })

  it('offers global flows in a diagram with no flows without navigating until one is selected', () => {
    queryState.data = [{ id: 'remote', viewId: 2, name: 'Remote', description: '', revision: 1, steps: [] }]
    const onNavigateView = vi.fn(), onFocus = vi.fn()
    act(() => { renderer = create(<FlowPanel open viewId={1} diagram={diagram} onNavigateView={onNavigateView} onClose={vi.fn()} onFocus={onFocus} />) })
    expect(menuItem('Remote, 0 steps').props.isDisabled).toBe(false)
    expect(onNavigateView).not.toHaveBeenCalled()
    expect(onFocus).toHaveBeenLastCalledWith(null)
    expect(button('Choose flow').findAllByType('p').some(node => node.props.children === 'Choose a flow')).toBe(true)
    act(() => menuItem('Remote, 0 steps').props.onClick())
    expect(onNavigateView).toHaveBeenCalledWith(2)
  })

  it('preserves the current draft when cross-diagram selection is cancelled', () => {
    queryState.data = [{ id: 'remote', viewId: 2, name: 'Remote', description: '', revision: 1, steps: [] }]
    const onNavigateView = vi.fn()
    mount(undefined, { onNavigateView })
    change(renderer.root.findByProps({ 'aria-label': 'Flow name' }), 'Unsaved local flow')
    vi.mocked(window.confirm).mockReturnValue(false)
    act(() => menuItem('Remote, 0 steps').props.onClick())
    expect(onNavigateView).not.toHaveBeenCalled()
    expect(currentDraft().name).toBe('Unsaved local flow')
    expect(renderer.root.findByProps({ 'aria-label': 'Flow name' }).props.value).toBe('Unsaved local flow')
  })

  it('asks before replacing a different draft in the target diagram', () => {
    queryState.data = [{ id: 'remote', viewId: 2, name: 'Remote', description: '', revision: 1, steps: [] }]
    const targetDraft = { ...queryState.data[0], id: '', name: 'Unsaved target flow' }
    queryClient.getQueryData.mockImplementation(key => key[0] === 'flow-draft' && key[1] === 2 ? targetDraft : undefined)
    const onNavigateView = vi.fn()
    vi.stubGlobal('window', { addEventListener: vi.fn(), removeEventListener: vi.fn(), confirm: vi.fn(() => false) })
    act(() => { renderer = create(<FlowPanel open viewId={1} diagram={diagram} onNavigateView={onNavigateView} onClose={vi.fn()} onFocus={vi.fn()} />) })
    act(() => menuItem('Remote, 0 steps').props.onClick())
    expect(onNavigateView).not.toHaveBeenCalled()
    expect(queryClient.removeQueries).not.toHaveBeenCalledWith({ queryKey: ['flow-draft', 2], exact: true })
    vi.mocked(window.confirm).mockReturnValue(true)
    act(() => menuItem('Remote, 0 steps').props.onClick())
    expect(queryClient.removeQueries).toHaveBeenCalledWith({ queryKey: ['flow-draft', 2], exact: true })
    expect(onNavigateView).toHaveBeenCalledWith(2)
  })

  it.each(['alternate', 'parallel'] as const)('focuses %s paths with a breadcrumb back to the main steps', mode => {
    mount()
    change(renderer.root.findByProps({ 'aria-label': 'Step title' }), 'Decision')
    act(() => button('Add step').props.onClick())
    change(renderer.root.findByProps({ 'aria-label': 'Step title' }), 'After')
    const [decision, after] = currentDraft().steps
    change(renderer.root.findByProps({ 'aria-label': 'Current flow step' }), decision.id)
    act(() => menuItem(`Add ${mode} paths`).props.onClick())
    expect(renderer.root.findByProps({ 'data-testid': 'flow-path-editor' })).toBeDefined()
    expect(renderer.root.findByProps({ 'aria-label': 'Path editor breadcrumb' })).toBeDefined()
    expect(renderer.root.findAllByProps({ 'aria-label': 'Reorder step 1' })).toHaveLength(0)
    expect(button('Add step')).toBeUndefined()
    expect(renderer.root.findAllByProps({ 'aria-label': 'Path name' })).toHaveLength(2)
    act(() => button('Back to flow steps').props.onClick())
    expect(renderer.root.findAllByProps({ 'data-testid': 'flow-path-editor' })).toHaveLength(0)
    expect(button('Reorder step 2')).toBeDefined()
    expect(renderer.root.findAllByProps({ 'aria-label': 'Step title' })).toHaveLength(0)
    change(renderer.root.findByProps({ 'aria-label': 'Current flow step' }), decision.id)
    act(() => button('Edit paths for Decision').props.onClick())
    expect(renderer.root.findByProps({ 'data-testid': 'flow-path-editor' })).toBeDefined()
    change(renderer.root.findByProps({ 'aria-label': 'Current flow step' }), after.id)
    expect(renderer.root.findAllByProps({ 'data-testid': 'flow-path-editor' })).toHaveLength(0)
    expect(renderer.root.findByProps({ 'aria-label': 'Step title' }).props.value).toBe('After')
  })

  it('opens nested branches at full width and navigates back through their breadcrumb', async () => {
    mount({ elementId: 2 })
    change(renderer.root.findByProps({ 'aria-label': 'Step title' }), 'Decision')
    act(() => menuItem('Add alternate paths').props.onClick())
    act(() => button('Add step to path Path A').props.onClick())
    change(renderer.root.findByProps({ 'aria-label': 'Step title' }), 'Concurrent work')
    act(() => menuItem('Add parallel paths').props.onClick())
    expect(renderer.root.findAllByProps({ 'data-testid': 'flow-path-editor' })).toHaveLength(1)
    expect(renderer.root.findAllByProps({ 'aria-label': 'Path name' })).toHaveLength(2)
    expect(button('Parallel paths').props['aria-pressed']).toBe(true)
    const breadcrumb = renderer.root.findByProps({ 'aria-label': 'Path editor breadcrumb' })
    expect(breadcrumb.findAllByType('p').some(node => node.props.children === 'Path A')).toBe(true)
    act(() => button('Add step to path Path B').props.onClick())
    change(renderer.root.findByProps({ 'aria-label': 'Step title' }), 'Background task')
    act(() => button('Edit paths for Decision').props.onClick())
    expect(button('Alternate paths').props['aria-pressed']).toBe(true)
    expect(renderer.root.findAllByProps({ 'aria-label': 'Path name' })).toHaveLength(2)
    act(() => button('Back to flow steps').props.onClick())
    vi.mocked(flowApi.save).mockImplementation(async flow => ({ ...flow, id: 'saved', revision: 1 }))
    await act(async () => button('Save flow').props.onClick())
    const decision = vi.mocked(flowApi.save).mock.lastCall![0].steps[0]
    expect(decision.elementIds).toEqual([2])
    const nested = decision.branches?.paths[0].steps[0]
    expect(nested).toMatchObject({ title: 'Concurrent work', branches: { mode: 'parallel' } })
    expect(nested?.branches?.paths[1].steps[0].title).toBe('Background task')
  })

  it('offers text-only storytelling with implicit continuation', async () => {
    mount()
    change(renderer.root.findByProps({ 'aria-label': 'Step title' }), 'Explain the next phase')
    expect(renderer.root.findAllByType('p').some(node => node.props.children === 'Leave empty for a text-only step.')).toBe(true)
    expect(renderer.root.findAllByProps({ 'aria-label': 'Go to flow' })).toHaveLength(0)
    act(() => menuItem('Link to flow').props.onClick())
    expect(renderer.root.findByProps({ 'aria-label': 'Go to flow' }).props.value).toBe('')
    act(() => button('Remove flow link').props.onClick())
    vi.mocked(flowApi.save).mockImplementation(async flow => ({ ...flow, id: 'saved', revision: 1 }))
    await act(async () => button('Save flow').props.onClick())
    expect(vi.mocked(flowApi.save).mock.lastCall![0].steps[0]).toMatchObject({ title: 'Explain the next phase', elementIds: [], connectorIds: [], goToFlowId: undefined })
  })

  it('preserves path contents when changing branch mode without a remove-all action', () => {
    mount({ elementId: 2 })
    act(() => menuItem('Add alternate paths').props.onClick())
    act(() => button('Add step to path Path A').props.onClick())
    const titles = renderer.root.findAllByProps({ 'aria-label': 'Step title' })
    change(titles[titles.length - 1], 'Handle the result')
    const parent = () => currentDraft().steps[0]
    const originalPaths = parent().branches.paths
    act(() => button('Parallel paths').props.onClick())
    expect(parent().branches).toEqual({ mode: 'parallel', paths: originalPaths })
    expect(window.confirm).not.toHaveBeenCalled()
    expect(button('Remove paths')).toBeUndefined()
    expect(parent().elementIds).toEqual([2])
    expect(renderer.root.findAllByProps({ 'aria-label': 'Path name' })).toHaveLength(2)
  })

  it('renames paths inline in their card headers and supports committing and cancelling edits', () => {
    mount()
    act(() => menuItem('Add alternate paths').props.onClick())
    act(() => button('Add step to path Path A').props.onClick())
    const original = currentDraft().steps[0].branches.paths[0]
    const name = () => renderer.root.findAllByProps({ 'aria-label': 'Path name' })[0]
    const editName = (value: string) => {
      act(() => name().props.onClick())
      act(() => name().props.onChange({ target: { value } }))
    }
    expect(name().type).toBe('button')
    const header = renderer.root.findByProps({ 'data-flow-path-id': original.id }).findAllByProps({ 'data-flow-step-header': true })[0]
    expect(header.findByProps({ 'aria-label': 'Path name' })).toBe(name())
    expect(renderer.root.findAllByType('input').filter(node => node.props['aria-label'] === 'Path name')).toHaveLength(0)
    editName('Success')
    expect(name().type).toBe('input')
    expect(name().props.maxLength).toBe(200)
    act(() => name().props.onKeyDown({ key: 'Enter', preventDefault: vi.fn(), stopPropagation: vi.fn() }))
    expect(name().type).toBe('button')
    expect(currentDraft().steps[0].branches.paths[0]).toEqual({ ...original, name: 'Success' })
    editName('Discard this')
    act(() => name().props.onKeyDown({ key: 'Escape', preventDefault: vi.fn(), stopPropagation: vi.fn() }))
    expect(name().type).toBe('button')
    expect(currentDraft().steps[0].branches.paths[0].name).toBe('Success')
    editName('Completed')
    act(() => name().props.onBlur())
    expect(name().type).toBe('button')
    expect(currentDraft().steps[0].branches.paths[0]).toEqual({ ...original, name: 'Completed' })
  })

  it('switches a linked step to paths without losing its highlights', async () => {
    mount({ elementId: 2, connectorId: 10 })
    act(() => menuItem('Link to flow').props.onClick())
    change(renderer.root.findByProps({ 'aria-label': 'Go to flow' }), 'child')
    act(() => button('Remove flow link').props.onClick())
    act(() => menuItem('Add alternate paths').props.onClick())
    vi.mocked(flowApi.save).mockImplementation(async flow => ({ ...flow, id: 'saved', revision: 1 }))
    await act(async () => button('Save flow').props.onClick())
    expect(vi.mocked(flowApi.save).mock.lastCall![0].steps[0]).toMatchObject({ elementIds: [2], connectorIds: [10], goToFlowId: undefined, branches: { mode: 'alternate' } })
  })

  it('reuses the sortable cards for path reordering, duplication and removal', () => {
    mount()
    act(() => menuItem('Add alternate paths').props.onClick())
    expect(button('Remove path Path A').props.isDisabled).toBe(true)
    const paths = () => renderer.root.findAllByProps({ 'aria-label': 'Path name' }).map(node => node.props.value)
    const key = (label: string, value: string) => act(() => button(label).props.onKeyDown({ key: value, preventDefault: vi.fn(), stopPropagation: vi.fn() }))
    key('Reorder path 2', 'ArrowUp')
    expect(paths()).toEqual(['Path B', 'Path A'])
    key('Reorder path 1', 'ArrowUp')
    expect(paths()).toEqual(['Path B', 'Path A'])
    const pathGroup = renderer.root.findAllByType('reorder-group' as never).find(node => node.props.values[0]?.name === 'Path B')!
    const [b, a] = pathGroup.props.values
    act(() => pathGroup.props.onReorder([a, b]))
    expect(paths()).toEqual(['Path A', 'Path B'])
    act(() => button('Duplicate path Path A').props.onClick())
    expect(paths()).toEqual(['Path A', 'Path A copy', 'Path B'])
    act(() => button('Remove path Path A copy').props.onClick())
    expect(paths()).toEqual(['Path A', 'Path B'])
  })

  it('edits and reorders nested steps with the shared editor and can collapse their path', async () => {
    mount({ elementId: 2 })
    act(() => menuItem('Add alternate paths').props.onClick())
    const addToPath = () => act(() => button('Add step to path Path A').props.onClick())
    const title = () => renderer.root.findAllByProps({ 'aria-label': 'Step title' }).at(-1)!
    addToPath()
    change(title(), 'First')
    addToPath()
    change(title(), 'Second')
    const nested = renderer.root.findAllByType('reorder-group' as never).find(node => node.props.values[0]?.title === 'First')!
    const [first, second] = nested.props.values
    act(() => nested.props.onReorder([second, first]))
    expect(title().props.value).toBe('Second')
    const pathHeader = () => button('Toggle 1.a: Path A')
    act(() => pathHeader().props.onClick())
    expect(pathHeader().props['aria-expanded']).toBe(false)
    expect(renderer.root.findAllByProps({ 'aria-label': 'Step title' })).toHaveLength(0)
    act(() => pathHeader().props.onClick())
    expect(pathHeader().props['aria-expanded']).toBe(true)
    vi.mocked(flowApi.save).mockImplementation(async flow => ({ ...flow, id: 'saved', revision: 1 }))
    await act(async () => button('Save flow').props.onClick())
    const parent = vi.mocked(flowApi.save).mock.lastCall![0].steps[0]
    expect(parent.elementIds).toEqual([2])
    expect(parent.branches?.paths[0].steps.map(step => step.title)).toEqual(['Second', 'First'])
    expect(parent.branches?.paths[1].steps).toEqual([])
  })

  it.each(['alternate', 'parallel'] as const)('adds %s paths independently of highlights', async mode => {
    mount({ elementId: 2, connectorId: 10 })
    expect(renderer.root.findAllByProps({ 'aria-label': 'Step continuation' })).toHaveLength(0)
    expect(renderer.root.findAllByType('option').some(node => node.props.children === 'Continue to next step')).toBe(false)
    expect(button('Add alternate paths')).toBeUndefined()
    expect(button('Add parallel paths')).toBeUndefined()
    expect(checkbox('Whole flow overview')).toBeUndefined()
    act(() => menuItem(`Add ${mode} paths`).props.onClick())
    expect(button(`${mode === 'alternate' ? 'Alternate' : 'Parallel'} paths`).props['aria-pressed']).toBe(true)
    expect(renderer.root.findAllByProps({ 'aria-label': 'Go to flow' })).toHaveLength(0)
    vi.mocked(flowApi.save).mockImplementation(async flow => ({ ...flow, id: 'saved', revision: 1 }))
    await act(async () => button('Save flow').props.onClick())
    const step = vi.mocked(flowApi.save).mock.lastCall![0].steps[0]
    expect(step).toMatchObject({ elementIds: [2], connectorIds: [10], branches: { mode } })
    expect(step.branches?.paths.map(path => ({ name: path.name, steps: path.steps }))).toEqual([
      { name: 'Path A', steps: [] }, { name: 'Path B', steps: [] },
    ])
  })

  it('edits steps inside alternate paths using the existing highlight editor', async () => {
    const onCanvasRecorderChange = vi.fn()
    mount({ elementId: 2 }, { onCanvasRecorderChange })
    act(() => menuItem('Add alternate paths').props.onClick())
    const addButtons = renderer.root.findAllByType('button').filter(node => node.props['aria-label']?.startsWith('Add step'))
    expect(addButtons).toHaveLength(2)
    act(() => button('Add step to path Path B').props.onClick())
    const titles = renderer.root.findAllByProps({ 'aria-label': 'Step title' })
    change(titles[titles.length - 1], 'Failure handling')
    act(() => button('Pick from canvas').props.onClick())
    act(() => onCanvasRecorderChange.mock.lastCall![0].capture({ connectorId: 11 }))
    vi.mocked(flowApi.save).mockImplementation(async flow => ({ ...flow, id: 'saved', revision: 1 }))
    await act(async () => button('Save flow').props.onClick())
    const step = vi.mocked(flowApi.save).mock.lastCall![0].steps[0]
    expect(step.branches?.mode).toBe('alternate')
    expect(step.branches?.paths[0].steps).toEqual([])
    expect(step.branches?.paths[1].steps[0]).toMatchObject({ title: 'Failure handling', elementIds: [2], connectorIds: [11] })
  })

  it('preserves an unsaved caller while previewing a linked flow', async () => {
    queryState.data = [{ id: 'child', viewId: 1, name: 'Child', description: '', revision: 1, steps: [] }]
    mount()
    change(renderer.root.findByProps({ 'aria-label': 'Step title' }), 'Unsaved caller')
    act(() => menuItem('Link to flow').props.onClick())
    change(renderer.root.findByProps({ 'aria-label': 'Go to flow' }), 'child')
    expect(renderer.root.findByProps({ 'aria-label': 'Go to flow' }).props.value).toBe('child')
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
    expect(onFocus).toHaveBeenLastCalledWith({ viewId: 2, elementIds: [2], connectorIds: [] })
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
    expect(onFocus).toHaveBeenLastCalledWith({ viewId: 2, elementIds: [2], connectorIds: [] })
    act(() => button('Return to previous flow').props.onClick())
    expect(renderer.root.findByProps({ 'aria-label': 'Current flow step' }).props.value).toBe('call')
    expect(onFocus).toHaveBeenLastCalledWith({ viewId: 1, elementIds: [], connectorIds: [] })
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


describe('complete flow import orchestration', () => {
  afterEach(() => vi.restoreAllMocks())
  const source = () => exportCompleteFlowYaml({ id: '', revision: 0, viewId: 1, name: 'Imported story', description: '', steps: [newStep(diagram, { connectorId: 10 })] }, diagram)

  it('creates a new element-owned view, caches its rebound draft and navigates without discarding the current draft', async () => {
    const onNavigateView = vi.fn()
    const resources = vi.spyOn(api.import, 'resources').mockResolvedValue({ view_id: 88, view_url: '/views/88' })
    vi.spyOn(api.workspace.views, 'content').mockResolvedValue({ placements: diagram.elements as never, connectors: diagram.connectors })
    mount(undefined, { onNavigateView })
    const oldDraft = currentDraft()
    queryClient.removeQueries.mockClear()
    const completeSource = source()
    const onImportComplete = renderer.root.findByType(FlowYamlControls).props.onImportComplete
    await act(async () => { expect(await onImportComplete(completeSource)).toBe(true) })
    expect(resources).toHaveBeenCalledOnce()
    expect(resources.mock.calls[0][1].elements[0]).toMatchObject({ name: 'Imported story', hasView: true, placements: [{ parentRef: 'root' }] })
    expect(queryClient.setQueryData).toHaveBeenCalledWith(['flow-draft', 88], expect.objectContaining({ name: 'Imported story', viewId: 88, id: '', revision: 0 }))
    const imported = queryClient.setQueryData.mock.calls.find(([key]) => key[0] === 'flow-draft' && key[1] === 88)![1] as StoryFlow
    expect(imported.steps[0].connectorIds).toEqual([10])
    expect(queryClient.removeQueries).not.toHaveBeenCalledWith({ queryKey: ['flow-draft', 1], exact: true })
    expect(oldDraft.name).toBe('New flow')
    expect(window.confirm).not.toHaveBeenCalled()
    expect(flowApi.save).not.toHaveBeenCalled()
    expect(onNavigateView).toHaveBeenCalledWith(88)
  })

  it('reuses created resources if loading the new view fails and is retried', async () => {
    const resources = vi.spyOn(api.import, 'resources').mockResolvedValue({ view_id: 89, view_url: '/views/89' })
    const content = vi.spyOn(api.workspace.views, 'content').mockRejectedValueOnce(new Error('Read failed')).mockResolvedValue({ placements: diagram.elements as never, connectors: diagram.connectors })
    const onNavigateView = vi.fn()
    mount(undefined, { onNavigateView })
    const completeSource = source()
    await act(async () => {
      await expect(renderer.root.findByType(FlowYamlControls).props.onImportComplete(completeSource)).rejects.toThrow(/Resources were created.*Retry/)
    })
    expect(onNavigateView).not.toHaveBeenCalled()
    await act(async () => { await renderer.root.findByType(FlowYamlControls).props.onImportComplete(completeSource) })
    expect(resources).toHaveBeenCalledOnce()
    expect(content).toHaveBeenCalledTimes(2)
    expect(onNavigateView).toHaveBeenCalledWith(89)
  })
})
