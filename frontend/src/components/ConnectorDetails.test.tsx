import React from 'react'
import { act, create } from 'react-test-renderer'
import { afterEach, describe, expect, it, vi } from 'vitest'
import type { ProxyConnectorDetails, ProxyEndpoint, WorkspaceGraphSnapshot } from '../crossBranch/types'
import { ConnectorRouteCard, UnderlyingConnectors } from './ConnectorDetails'
import ProxyConnectorPanel from './ProxyConnectorPanel'
import { UNPLACED_ELEMENTS_VIEW_ID } from '../crossBranch/unplacedElements'

const context = vi.hoisted(() => ({ canEdit: true, navigate: vi.fn() }))
vi.mock('react-router-dom', () => ({ Link: () => null, useNavigate: () => context.navigate }))
vi.mock('../pages/ViewEditor/context', () => ({ useViewEditorContext: () => ({ canEdit: context.canEdit, viewId: 1 }) }))
vi.mock('./SlidingPanel', () => ({ default: ({ children }: { children: React.ReactNode }) => <>{children}</> }))
vi.mock('./PanelHeader', () => ({ default: () => null }))
vi.mock('@chakra-ui/react', async () => {
  const ReactModule = await import('react')
  const Box = ({ children, ...props }: { children?: React.ReactNode }) => ReactModule.createElement('div', props, children)
  const Button = ({ children, ...props }: { children?: React.ReactNode }) => ReactModule.createElement('button', props, children)
  return { Box, HStack: Box, Text: Box, VStack: Box, Flex: Box, Button, IconButton: Button }
})

function endpoint(id: number, name: string, viewId: number): ProxyEndpoint {
  return {
    actualElementId: id, actualElementName: name, anchorElementId: id / 10, anchorElementName: `Anchor ${id}`,
    anchorViewId: 1, anchorViewName: 'Overview', placementViewId: viewId, placementViewName: `View ${viewId}`,
    depth: 1, externalToView: true, currentBranchElementId: null,
    commonAncestorViewId: 1, commonAncestorViewName: 'Overview',
  }
}
const details: ProxyConnectorDetails = {
  key: '1:2', label: 'calls', count: 1, sourceAnchorId: '1', targetAnchorId: '2',
  sourceAnchorName: 'API', targetAnchorName: 'Database', ownerViewIds: [1], ownerViewNames: ['Overview'],
  connectors: [{
    ownerViewId: 1, ownerViewName: 'Overview', source: endpoint(10, 'Worker', 2), target: endpoint(20, 'Store', 3),
    connector: { id: 100, view_id: 1, source_element_id: 10, target_element_id: 20, label: 'loads', relationship: 'SQL',
      description: 'Loads account records', direction: 'backward', style: 'bezier', url: null,
      source_handle: null, target_handle: null, created_at: '', updated_at: '' },
  }],
}
const snapshot = {
  childViewIdByOwnerElementId: { 1: 2 }, viewById: { 2: { name: 'API internals' } },
} as unknown as WorkspaceGraphSnapshot
const event = { stopPropagation: vi.fn() }

afterEach(() => { context.canEdit = true; context.navigate.mockReset() })

describe('shared connector details', () => {
  it.each([
    { side: 'source', placementViewId: null, anchorViewId: null },
    { side: 'target', placementViewId: null, anchorViewId: null },
    { side: 'source', placementViewId: null, anchorViewId: 1 },
    { side: 'target', placementViewId: null, anchorViewId: 1 },
    { side: 'source', placementViewId: UNPLACED_ELEMENTS_VIEW_ID, anchorViewId: 1 },
    { side: 'target', placementViewId: UNPLACED_ELEMENTS_VIEW_ID, anchorViewId: 1 },
  ] as const)('does not link an unplaced $side endpoint (placement: $placementViewId, anchor: $anchorViewId)', ({ side, placementViewId, anchorViewId }) => {
    const leaf = details.connectors[0]
    const unplacedDetails: ProxyConnectorDetails = {
      ...details,
      connectors: [{ ...leaf, [side]: { ...leaf[side], placementViewId, anchorViewId } }],
    }
    const close = vi.fn()
    const renderer = create(<UnderlyingConnectors details={unplacedDetails} onNavigate={close} />)
    const row = renderer.root.findByType(ConnectorRouteCard)
    expect(row.props[`${side}To`]).toBeUndefined()
    expect(renderer.root.findAllByType('button').some((button) => button.props.title === leaf[side].actualElementName)).toBe(false)
    expect(close).not.toHaveBeenCalled()

    // The placed endpoint remains navigable and closes the panel on selection.
    const other = side === 'source' ? 'target' : 'source'
    const link = renderer.root.findAllByType('button').find((button) => button.props.title === leaf[other].actualElementName)!
    expect(link.props.to).toBe(`/views/${leaf[other].placementViewId}?element=${leaf[other].actualElementId}`)
    act(() => link.props.onClick(event))
    expect(close).toHaveBeenCalledOnce()
    renderer.unmount()
  })

  it('uses the same rows in the panel and compact list, with endpoint links and metadata', () => {
    const close = vi.fn()
    const panel = create(<ProxyConnectorPanel isOpen onClose={close} details={details} snapshot={snapshot} />)
    const compact = create(<UnderlyingConnectors details={details} />)
    const panelRow = panel.root.findAllByType(ConnectorRouteCard)[1]
    const compactRow = compact.root.findByType(ConnectorRouteCard)
    for (const row of [panelRow, compactRow]) {
      expect(row.props).toMatchObject({
        source: 'Worker', target: 'Store', direction: 'backward', subtitle: 'loads · SQL',
        description: 'Loads account records', sourceTo: '/views/2?element=10', targetTo: '/views/3?element=20',
      })
    }
    const endpointLink = panel.root.findAllByType('button').find((button) => button.props.to === '/views/2?element=10')!
    act(() => endpointLink.props.onClick(event))
    expect(close).toHaveBeenCalledOnce()
    panel.unmount()
    compact.unmount()
  })

  it('preserves panel edit, delete, and off-view navigation actions', () => {
    const edit = vi.fn()
    const remove = vi.fn()
    const close = vi.fn()
    const panel = create(<ProxyConnectorPanel isOpen onClose={close} details={details} snapshot={snapshot} onEdit={edit} onDelete={remove} />)
    act(() => panel.root.findByProps({ 'aria-label': 'Edit connector' }).props.onClick(event))
    expect(edit).toHaveBeenCalledWith(details.connectors[0].connector)
    act(() => panel.root.findByProps({ 'aria-label': 'Delete connector' }).props.onClick(event))
    expect(remove).toHaveBeenCalledWith(100, 1)
    act(() => panel.root.findByProps({ title: 'Open API internals' }).props.onClick(event))
    expect(context.navigate).toHaveBeenCalledWith('/views/2')
    expect(close).toHaveBeenCalledOnce()
    context.canEdit = false
    panel.update(<ProxyConnectorPanel isOpen onClose={close} details={details} snapshot={snapshot} onEdit={edit} onDelete={remove} />)
    expect(panel.root.findAllByProps({ 'aria-label': 'Edit connector' })).toHaveLength(0)
    expect(panel.root.findAllByProps({ 'aria-label': 'Delete connector' })).toHaveLength(0)
    panel.unmount()
  })
})
