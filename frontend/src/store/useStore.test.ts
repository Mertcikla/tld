import { beforeEach, describe, expect, it } from 'vitest'
import type { Connector, LibraryElement, PlacedElement, ViewTreeNode } from '../types'
import {
  buildViewContentLinks,
  buildElementLibraryItems,
  canvasSelectors,
  emptyViewEditorUiState,
  findViewByOwner,
  findViewPath,
  mergeSavedElementIntoPlacements,
  removeConnectorFromList,
  removePlacedElement,
  selectConnectorById,
  selectElementById,
  selectExistingElementIds,
  updatePlacedElementPosition,
  upsertConnectorInList,
  useStore,
} from './useStore'

const tree: ViewTreeNode[] = [{
  id: 1,
  owner_element_id: null,
  name: 'Root',
  description: null,
  level_label: null,
  level: 0,
  depth: 0,
  created_at: '2024-01-01',
  updated_at: '2024-01-01',
  parent_view_id: null,
  children: [{
    id: 2,
    owner_element_id: 10,
    name: 'Child',
    description: null,
    level_label: null,
    level: 1,
    depth: 1,
    created_at: '2024-01-01',
    updated_at: '2024-01-01',
    parent_view_id: 1,
    children: [],
  }],
}]

const element = (element_id: number, x = 0, y = 0): PlacedElement => ({
  id: element_id + 100,
  view_id: 1,
  element_id,
  position_x: x,
  position_y: y,
  name: `Element ${element_id}`,
  description: null,
  kind: null,
  technology: null,
  url: null,
  logo_url: null,
  technology_connectors: [],
  tags: [],
  has_view: false,
  view_label: null,
})

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
  created_at: '2024-01-01',
  updated_at: '2024-01-01',
})

const libraryElement = (id: number): LibraryElement => ({
  id,
  name: 'Saved',
  kind: 'service',
  description: 'Updated',
  technology: 'TypeScript',
  url: 'https://example.com',
  logo_url: 'logo.svg',
  technology_connectors: [{ type: 'custom', label: 'TS' }],
  tags: ['api'],
  repo: 'repo',
  branch: 'main',
  file_path: 'src/index.ts',
  language: 'ts',
  created_at: '2024-01-01',
  updated_at: '2024-01-02',
  has_view: true,
  view_label: 'View',
})

beforeEach(() => {
  useStore.setState({
    ...emptyViewEditorUiState,
    view: undefined,
    viewElements: [],
    connectors: [],
    nodes: [],
    edges: [],
    linksMap: {},
    parentLinksMap: {},
    incomingLinks: [],
    treeData: [],
    allElements: [],
  })
})

describe('pure view helpers', () => {
  it('finds views by owner and path', () => {
    expect(findViewByOwner(tree, 10)?.id).toBe(2)
    expect(findViewByOwner(tree, 999)).toBeNull()
    expect(findViewPath(tree, 2)?.map((view) => view.id)).toEqual([1, 2])
    expect(findViewPath(tree, 999)).toBeNull()
  })

  it('builds child, parent, and incoming links for a view', () => {
    const result = buildViewContentLinks(tree, 1, [element(10), element(20)])
    expect(result.linksMap[10][0]).toMatchObject({ to_view_id: 2, relation_type: 'child' })
    expect(result.parentLinksMap).toEqual({})
    expect(result.incomingLinks).toEqual([])

    const childResult = buildViewContentLinks(tree, 2, [element(20)])
    expect(childResult.parentLinksMap[20][0]).toMatchObject({ from_view_id: 1, relation_type: 'parent' })
    expect(childResult.incomingLinks[0]).toMatchObject({ element_id: 10, from_view_id: 1, to_view_id: 2 })
  })

  it('updates placement positions structurally', () => {
    const elements = [element(10, 1, 2), element(20, 3, 4)]
    expect(updatePlacedElementPosition(elements, 10, 1, 2)).toBe(elements)
    const next = updatePlacedElementPosition(elements, 10, 9, 8)
    expect(next).not.toBe(elements)
    expect(next[0]).toMatchObject({ position_x: 9, position_y: 8 })
    expect(next[1]).toBe(elements[1])
  })

  it('removes placements and merges saved element fields', () => {
    const elements = [element(10), element(20)]
    expect(removePlacedElement(elements, 10).map((item) => item.element_id)).toEqual([20])
    const merged = mergeSavedElementIntoPlacements(elements, libraryElement(10))
    expect(merged[0]).toMatchObject({ name: 'Saved', kind: 'service', repo: 'repo', tags: ['api'] })
    expect(merged[1]).toBe(elements[1])
  })

  it('keeps library items available after removing their canvas placement', () => {
    const onCanvas = element(10)
    const libraryItems = buildElementLibraryItems([libraryElement(10), libraryElement(20)], [onCanvas])

    expect(libraryItems.map((item) => item.id)).toEqual([10, 20])
    expect(libraryItems[0]).toMatchObject({ id: 10, name: onCanvas.name, created_at: '2024-01-01' })

    const afterRemoval = buildElementLibraryItems([libraryElement(10), libraryElement(20)], removePlacedElement([onCanvas], 10))
    expect(afterRemoval.map((item) => item.id)).toEqual([10, 20])
    expect(afterRemoval[0]).toMatchObject({ id: 10, name: 'Saved' })
  })

  it('upserts and removes connectors', () => {
    const first = connector(1)
    const second = { ...connector(2), label: 'two' }
    expect(upsertConnectorInList([], first)).toEqual([first])
    expect(upsertConnectorInList([first], { ...first, label: 'updated' })[0].label).toBe('updated')
    expect(upsertConnectorInList([first], second)).toEqual([first, second])
    expect(removeConnectorFromList([first, second], 1)).toEqual([second])
  })

  it('selects existing element ids and entities by id', () => {
    const state = { viewElements: [element(10), element(20)], connectors: [connector(1)] }
    expect(Array.from(selectExistingElementIds(state))).toEqual([10, 20])
    expect(selectElementById(state, 20)?.name).toBe('Element 20')
    expect(selectConnectorById(state, 1)?.source_element_id).toBe(10)
    expect(canvasSelectors.existingElementIds(state).has(10)).toBe(true)
    expect(canvasSelectors.elementById(state, 10)?.id).toBe(110)
    expect(canvasSelectors.connectorById(state, 1)?.id).toBe(1)
  })
})

describe('zustand canvas store', () => {
  it('updates every scalar ui action', () => {
    const selectedElement = libraryElement(10)
    const selectedConnector = connector(1)
    useStore.getState().setViewEditorUi({ viewId: 1, canEdit: true, isOwner: true, isFreePlan: true })
    useStore.getState().setSnapToGrid(false)
    useStore.getState().setSelectedElement(selectedElement)
    useStore.getState().setSelectedConnector(selectedConnector)

    expect(useStore.getState()).toMatchObject({
      viewId: 1,
      canEdit: true,
      isOwner: true,
      isFreePlan: true,
      snapToGrid: false,
      selectedElement,
      selectedConnector,
    })
  })

  it('sets every core collection with values and updater functions', () => {
    useStore.getState().setView(tree[0])
    useStore.getState().setViewElements([element(10)])
    useStore.getState().setViewElements((previous) => [...previous, element(20)])
    useStore.getState().setConnectors([connector(1)])
    useStore.getState().setConnectors((previous) => [...previous, connector(2)])
    useStore.getState().setNodes([{ id: '10', position: { x: 0, y: 0 }, data: {} }])
    useStore.getState().setNodes((previous) => [...previous, { id: '20', position: { x: 1, y: 1 }, data: {} }])
    useStore.getState().setEdges([{ id: '1', source: '10', target: '20' }])
    useStore.getState().setEdges((previous) => [...previous, { id: '2', source: '20', target: '10' }])
    useStore.getState().setLinksMap({ 10: [] })
    useStore.getState().setLinksMap((previous) => ({ ...previous, 20: [] }))
    useStore.getState().setParentLinksMap({ 10: [] })
    useStore.getState().setParentLinksMap((previous) => ({ ...previous, 20: [] }))
    useStore.getState().setIncomingLinks([{ id: 1, element_id: 10, element_name: 'E', from_view_id: 1, from_view_name: 'Root', to_view_id: 2 }])
    useStore.getState().setTreeData(tree)
    useStore.getState().setAllElements([libraryElement(10)])

    const state = useStore.getState()
    expect(state.view?.id).toBe(1)
    expect(state.viewElements).toHaveLength(2)
    expect(state.connectors).toHaveLength(2)
    expect(state.nodes).toHaveLength(2)
    expect(state.edges).toHaveLength(2)
    expect(Object.keys(state.linksMap)).toEqual(['10', '20'])
    expect(Object.keys(state.parentLinksMap)).toEqual(['10', '20'])
    expect(state.incomingLinks).toHaveLength(1)
    expect(state.treeData).toBe(tree)
    expect(state.allElements).toHaveLength(1)
  })

  it('hydrates and resets canvas data', () => {
    const links = buildViewContentLinks(tree, 1, [element(10)])
    useStore.getState().setNodes([{ id: 'stale', position: { x: 0, y: 0 }, data: {} }])
    useStore.getState().setEdges([{ id: 'stale', source: '1', target: '2' }])
    useStore.getState().hydrateViewContent({
      view: tree[0],
      viewElements: [element(10)],
      connectors: [connector(1)],
      treeData: tree,
      ...links,
    })
    expect(useStore.getState()).toMatchObject({
      view: tree[0],
      viewElements: [element(10)],
      connectors: [connector(1)],
      treeData: tree,
    })
    useStore.getState().resetCanvas()
    expect(useStore.getState().nodes).toEqual([])
    expect(useStore.getState().edges).toEqual([])
  })

  it('runs placement, connector, and deletion actions', () => {
    useStore.getState().setViewElements([element(10), element(20)])
    useStore.getState().setConnectors([connector(1)])
    useStore.getState().updateElementPosition(10, 50, 60)
    expect(useStore.getState().viewElements[0]).toMatchObject({ position_x: 50, position_y: 60 })

    useStore.getState().removeElementPlacement(10)
    expect(useStore.getState().viewElements.map((item) => item.element_id)).toEqual([20])

    useStore.getState().setViewElements([element(10)])
    useStore.getState().setAllElements([libraryElement(10)])
    useStore.getState().mergeSavedElement(libraryElement(10))
    expect(useStore.getState().viewElements[0].name).toBe('Saved')
    expect(useStore.getState().allElements[0].name).toBe('Saved')

    useStore.getState().removeElementEverywhere(10)
    expect(useStore.getState().viewElements).toEqual([])
    expect(useStore.getState().allElements).toEqual([])

    useStore.getState().upsertConnector(connector(2))
    useStore.getState().replaceConnector({ ...connector(2), label: 'updated' })
    useStore.getState().removeConnector(1)
    expect(useStore.getState().connectors).toEqual([{ ...connector(2), label: 'updated' }])
  })
})
