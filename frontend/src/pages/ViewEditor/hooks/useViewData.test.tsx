import React from 'react'
import { act, create } from 'react-test-renderer'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { afterEach, describe, expect, it } from 'vitest'
import type { Connector, PlacedElement } from '../../../types'
import { useStore } from '../../../store/useStore'
import { useViewData } from './useViewData'

const noop = () => {}
const asyncNoop = async () => {}
const tagColors = {}
const hoveredZoomRef = { current: null }
const emptyTags: string[] = []
function Harness({ selected = emptyTags, hidden = emptyTags, hovered = null, color = '#f00', connectorSelection = null, dragged = null }: {
  selected?: string[]; hidden?: string[]; hovered?: string[] | null; color?: string; connectorSelection?: Connector | null; dragged?: string[] | null
}) {
  useViewData({
    viewId: null, interactionSourceId: null, clickConnectMode: null, selectedConnector: connectorSelection,
    activeTags: selected, draggedConnectorTags: dragged, hiddenLayerTags: hidden, hoveredLayerTags: hovered, hoveredLayerColor: color, tagColors,
    stableOnZoomIn: asyncNoop, stableOnZoomOut: asyncNoop, stableOnNavigateToView: noop,
    stableOnSelect: noop, stableOnOpenCodePreview: noop, stableOnInteractionStart: noop,
    stableOnConnectTo: asyncNoop, stableOnStartHandleReconnect: noop, stableOnRemoveElement: asyncNoop,
    stableOnHoverZoom: noop, hoveredZoomRef,
  })
  return null
}

function placed(id: number, tags: string[]): PlacedElement {
  return {
    id, element_id: id, view_id: 1, position_x: id * 100, position_y: 30,
    name: `Element ${id}`, kind: 'service', description: null, technology: null, url: null,
    logo_url: null, technology_connectors: [], tags, has_view: false, view_label: null,
  }
}
function connector(id: number, source: number, target: number, tags: string[]): Connector {
  return { id, view_id: 1, source_element_id: source, target_element_id: target, tags,
    label: 'reads', description: null, relationship: null, direction: 'both', style: 'bezier',
    url: null, source_handle: null, target_handle: null, created_at: '', updated_at: '',
  }
}

afterEach(() => useStore.setState({ viewElements: [], connectors: [], nodes: [], edges: [] }))

describe('editor canvas tags', () => {
  it('keeps connector endpoints as context, colors hover strokes and arrows, hides tags, and resets', () => {
    const client = new QueryClient({ defaultOptions: { queries: { retry: false } } })
    const render = (props: React.ComponentProps<typeof Harness>) => <QueryClientProvider client={client}><Harness {...props} /></QueryClientProvider>
    let renderer: ReturnType<typeof create>
    act(() => { renderer = create(render({})) })
    act(() => useStore.setState({
      viewElements: [placed(1, ['api']), placed(2, ['storage']), placed(3, ['api'])],
      connectors: [connector(10, 1, 2, ['critical']), connector(11, 1, 2, []), connector(12, 1, 3, [])],
    }))
    const initial = useStore.getState().nodes.map((node) => node.position)
    const initialHandles = useStore.getState().edges.map((edge) => [edge.sourceHandle, edge.targetHandle])
    const node = (id: string) => useStore.getState().nodes.find((node) => node.id === id)!
    const edge = (id: string) => useStore.getState().edges.find((edge) => edge.id === id)!
    act(() => renderer.update(render({ selected: ['critical'] })))
    expect(node('1').style?.opacity).toBe(0.55)
    expect(node('2').style?.opacity).toBe(0.55)
    expect(node('3').style?.opacity).toBe(0.1)
    expect(edge('10').style?.opacity).toBe(0.8)
    expect(edge('11').style?.opacity).toBe(0.1)
    expect(useStore.getState().nodes.map((node) => node.position)).toEqual(initial)
    expect(node('1').data.tags).toEqual(['api'])
    expect(node('1').data.tagHiddenHandleIds).toContain(edge('11').sourceHandle)
    expect(node('2').data.tagHiddenHandleIds).toContain(edge('11').targetHandle)
    expect(node('1').data.tagHiddenHandleIds).not.toContain(edge('10').sourceHandle)
    expect(node('2').data.tagHiddenHandleIds).not.toContain(edge('10').targetHandle)
    expect(node('1').data.connectedHandleIds).toContain(edge('11').sourceHandle)
    expect(node('1').data.reconnectCandidates.map((candidate: { edgeId: string }) => candidate.edgeId)).toEqual(['10'])
    expect(useStore.getState().edges.map((edge) => [edge.sourceHandle, edge.targetHandle])).toEqual(initialHandles)

    const dimmedSelection = useStore.getState().connectors[1]
    act(() => renderer.update(render({ selected: ['critical'], connectorSelection: dimmedSelection })))
    expect(node('1').data.selectedHandleIds).toEqual([])
    expect(node('2').data.selectedHandleIds).toEqual([])
    expect(node('1').data.isConnectorHighlighted).toBe(false)

    act(() => renderer.update(render({ hovered: ['critical'] })))
    expect(edge('10').style).toMatchObject({ stroke: '#f00', strokeWidth: 3, opacity: 0.8 })
    expect(edge('10').markerEnd).toMatchObject({ color: '#f00' })
    expect(edge('10').markerStart).toMatchObject({ color: '#f00' })
    expect(node('1').data.forceShowTagPopup).toBe(false)
    expect(node('1').style?.opacity).toBe(0.55)
    expect(node('1').data.tagHiddenHandleIds).toContain(edge('11').sourceHandle)
    expect(node('1').data.tagHiddenHandleIds).not.toContain(edge('10').sourceHandle)
    act(() => renderer.update(render({ hovered: ['critical'], color: '#0f0' })))
    expect(edge('10').style?.stroke).toBe('#0f0')

    // Starting a drag must invert the source-row hover preview: already-tagged
    // connectors dim, while new targets become interactive even if hover lingers.
    act(() => renderer.update(render({ hovered: ['critical'], dragged: ['critical'] })))
    expect(edge('10').style).toMatchObject({ opacity: 0.1, pointerEvents: 'none' })
    expect(edge('11').style).toMatchObject({ opacity: 0.8, pointerEvents: 'auto' })
    expect(edge('12').style).toMatchObject({ opacity: 0.8, pointerEvents: 'auto' })
    expect(node('1').data.tagHiddenHandleIds).toEqual([edge('10').sourceHandle])
    expect(node('2').data.tagHiddenHandleIds).toEqual([edge('10').targetHandle])
    // A layer is still useful when the connector has only some of its tags.
    act(() => renderer.update(render({ hovered: ['critical'], dragged: ['critical', 'backend'] })))
    expect(edge('10').style).toMatchObject({ opacity: 0.8, pointerEvents: 'auto' })
    act(() => renderer.update(render({})))
    expect(edge('10').style).toMatchObject({ opacity: 0.8, pointerEvents: 'auto' })
    expect(node('1').data.tagHiddenHandleIds).toEqual([])

    act(() => renderer.update(render({ hidden: ['critical'] })))
    expect(edge('10').style?.opacity).toBe(0.1)
    expect(node('1').style).toBeUndefined()
    expect(node('1').data.tagHiddenHandleIds).toEqual([edge('10').sourceHandle])
    expect(node('2').data.tagHiddenHandleIds).toEqual([edge('10').targetHandle])
    act(() => renderer.update(render({ selected: ['critical'], hidden: ['storage'] })))
    expect(edge('10').style?.opacity).toBe(0.1)
    expect(node('2').style?.pointerEvents).toBe('none')

    act(() => renderer.update(render({ selected: ['api'] })))
    expect(edge('12').style?.opacity).toBe(0.8)
    expect(edge('10').style?.opacity).toBe(0.1)
    act(() => renderer.update(render({})))
    expect(useStore.getState().nodes.every((node) => node.style === undefined && node.data.tagHiddenHandleIds.length === 0)).toBe(true)
    expect(node('1').data.reconnectCandidates).toHaveLength(3)
    expect(useStore.getState().edges.every((edge) => edge.style?.opacity === 0.8 && edge.style.strokeWidth === 2)).toBe(true)
    act(() => renderer.unmount())
    client.clear()
  })
})
