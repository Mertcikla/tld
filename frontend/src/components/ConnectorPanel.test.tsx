import React from 'react'
import { act, create } from 'react-test-renderer'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import type { Connector } from '../types'
import ConnectorPanel from './ConnectorPanel'
import { api } from '../api/client'

const testContext = vi.hoisted(() => ({ canEdit: true }))

vi.mock('../api/client', () => ({ api: { workspace: { connectors: { update: vi.fn() } } } }))

vi.mock('../pages/ViewEditor/context', () => ({
  useViewEditorContext: () => ({ canEdit: testContext.canEdit, viewId: 1 }),
}))

vi.mock('./ConfirmDialog', () => ({ default: () => null }))
vi.mock('./SlidingPanel', () => ({ default: ({ children }: { children?: React.ReactNode }) => <>{children}</> }))
vi.mock('./PanelHeader', () => ({ default: () => null }))
vi.mock('./TagUpsert', () => ({ default: () => null }))

vi.mock('@chakra-ui/react', async () => {
  const ReactModule = await import('react')
  const passthrough = ({ children, as: Component = 'div', ...props }: { children?: React.ReactNode; as?: React.ElementType }) =>
    ReactModule.createElement(Component, props, children)
  const slider = ({ children, ...props }: { children?: React.ReactNode }) =>
    ReactModule.createElement('div', props, children)
  return {
    Box: passthrough,
    Button: passthrough,
    Divider: passthrough,
    FormControl: passthrough,
    FormLabel: passthrough,
    HStack: passthrough,
    SimpleGrid: passthrough,
    Input: passthrough,
    Slider: slider,
    SliderFilledTrack: passthrough,
    SliderThumb: passthrough,
    SliderTrack: passthrough,
    Text: passthrough,
    Textarea: passthrough,
    Tag: passthrough,
    TagCloseButton: passthrough,
    TagLabel: passthrough,
    useBreakpointValue: () => false,
    useDisclosure: () => ({ isOpen: false, onOpen: vi.fn(), onClose: vi.fn() }),
    useToast: () => vi.fn(),
    VStack: passthrough,
    Wrap: passthrough,
    WrapItem: passthrough,
  }
})

function connector(id = 10): Connector {
  return {
    id,
    view_id: 1,
    source_element_id: 1,
    target_element_id: 2,
    label: 'connects',
    description: null,
    relationship: null,
    direction: 'forward',
    style: 'bezier',
    url: null,
    source_handle: null,
    target_handle: null,
    created_at: '2024-01-01',
    updated_at: '2024-01-01',
  }
}

describe('ConnectorPanel noise gate', () => {
  beforeEach(() => {
    vi.useFakeTimers()
    testContext.canEdit = true
    vi.mocked(api.workspace.connectors.update).mockReset()
    vi.stubGlobal('window', {
      addEventListener: vi.fn(),
      removeEventListener: vi.fn(),
      setTimeout,
      clearTimeout,
    })
  })

  afterEach(() => {
    vi.useRealTimers()
    vi.unstubAllGlobals()
  })

  it('shows a field spinner for the entire autosave request', async () => {
    let finish!: (value: Connector) => void
    vi.mocked(api.workspace.connectors.update).mockImplementationOnce(() => new Promise((resolve) => { finish = resolve }))
    let renderer!: ReturnType<typeof create>
    await act(async () => { renderer = create(<ConnectorPanel isOpen autoSave connector={connector()} orgId="" onSave={vi.fn()} onClose={vi.fn()} onDelete={vi.fn()} />) })
    await act(async () => { renderer.root.findByProps({ 'data-testid': 'connector-panel-label-input' }).props.onChange({ target: { value: 'Updated label' } }) })
    await act(async () => vi.advanceTimersByTime(150))
    expect(renderer.root.findByProps({ 'data-testid': 'autosave-field-connector-label' }).props.opacity).toBe(1)
    expect(renderer.root.findAllByProps({ 'data-testid': 'autosave-field-spinner' }).filter((node) => typeof node.type === 'string')).toHaveLength(1)
    await act(async () => { finish({ ...connector(), label: 'Updated label' }) })
    expect(renderer.root.findAllByProps({ 'data-testid': 'autosave-field-spinner' }).filter((node) => typeof node.type === 'string')).toHaveLength(0)
    expect(renderer.root.findByProps({ 'data-testid': 'autosave-field-connector-label' }).props.opacity).toBe(1)
    await act(async () => renderer.unmount())
  })

  it('reports autosave failure and retries the current draft', async () => {
    const onSave = vi.fn()
    const onClose = vi.fn()
    let renderer: ReturnType<typeof create>
    await act(async () => {
      renderer = create(<ConnectorPanel isOpen autoSave connector={connector()} orgId="" onSave={onSave} onClose={onClose} onDelete={vi.fn()} />)
    })
    expect(renderer!.root.findAllByProps({ 'data-testid': 'panel-save-status' })).toHaveLength(0)
    vi.mocked(api.workspace.connectors.update).mockRejectedValueOnce(new Error('offline'))
    await act(async () => {
      renderer!.root.findByProps({ 'data-testid': 'connector-panel-label-input' }).props.onChange({ target: { value: 'Updated label' } })
    })
    await act(async () => { vi.advanceTimersByTime(150) })
    expect(api.workspace.connectors.update).toHaveBeenCalledWith(1, 10, expect.objectContaining({ label: 'Updated label' }))
    expect(renderer!.root.findByProps({ 'data-testid': 'panel-save-status' }).props.role).toBe('alert')
    expect(onSave).not.toHaveBeenCalled()
    expect(onClose).not.toHaveBeenCalled()
    vi.mocked(api.workspace.connectors.update).mockResolvedValueOnce({ ...connector(), label: 'Updated label' })
    await act(async () => { renderer!.root.findByProps({ 'data-testid': 'panel-save-retry' }).props.onClick(); await Promise.resolve() })
    expect(onSave).toHaveBeenCalledWith(expect.objectContaining({ label: 'Updated label' }))
    expect(renderer!.root.findByProps({ 'data-testid': 'autosave-field-connector-label' }).props.opacity).toBe(1)
    expect(renderer!.root.findAllByProps({ 'data-testid': 'panel-save-status' })).toHaveLength(0)
    expect(onClose).not.toHaveBeenCalled()
    await act(async () => { renderer!.unmount() })
  })

  it('maps the slider level to an exact visibility delta', async () => {
    testContext.canEdit = true
    const onVisibilityOverrideDeltaChange = vi.fn()
    let renderer: ReturnType<typeof create>
    await act(async () => {
      renderer = create(
        <ConnectorPanel
          isOpen
          onClose={vi.fn()}
          connector={connector()}
          orgId=""
          onSave={vi.fn()}
          onDelete={vi.fn()}
          visibilityOverrideDelta={1}
          onVisibilityOverrideDeltaChange={onVisibilityOverrideDeltaChange}
        />,
      )
    })

    const slider = renderer!.root.findByProps({ 'aria-label': 'Connector noise gate' })
    expect(slider.props.value).toBe(-1)
    expect(renderer!.root.findAllByProps({ children: 'DENSITY' })).toHaveLength(0)
    expect(renderer!.root.findAll((node) => node.type === 'div' && node.children.includes('Quiet'))).toHaveLength(1)
    expect(renderer!.root.findAll((node) => node.type === 'div' && node.children.includes('Full'))).toHaveLength(1)

    await act(async () => {
      slider.props.onChangeEnd(-2)
      await Promise.resolve()
    })
    expect(onVisibilityOverrideDeltaChange).toHaveBeenCalledWith(10, 2)
  })

  it('updates the displayed level when the connector override changes', async () => {
    testContext.canEdit = true
    let renderer: ReturnType<typeof create>
    await act(async () => {
      renderer = create(
        <ConnectorPanel
          isOpen
          onClose={vi.fn()}
          connector={connector()}
          orgId=""
          onSave={vi.fn()}
          onDelete={vi.fn()}
          visibilityOverrideDelta={0}
          onVisibilityOverrideDeltaChange={vi.fn()}
        />,
      )
    })

    await act(async () => {
      renderer!.update(
        <ConnectorPanel
          isOpen
          onClose={vi.fn()}
          connector={connector(11)}
          orgId=""
          onSave={vi.fn()}
          onDelete={vi.fn()}
          visibilityOverrideDelta={-2}
          onVisibilityOverrideDeltaChange={vi.fn()}
        />,
      )
    })

    expect(renderer!.root.findByProps({ 'aria-label': 'Connector noise gate' }).props.value).toBe(2)
  })

  it('disables the slider for read-only users', async () => {
    testContext.canEdit = false
    let renderer: ReturnType<typeof create>
    await act(async () => {
      renderer = create(
        <ConnectorPanel
          isOpen
          onClose={vi.fn()}
          connector={connector()}
          orgId=""
          onSave={vi.fn()}
          onDelete={vi.fn()}
          visibilityOverrideDelta={0}
          onVisibilityOverrideDeltaChange={vi.fn()}
        />,
      )
    })

    expect(renderer!.root.findByProps({ 'aria-label': 'Connector noise gate' }).props.isDisabled).toBe(true)
    testContext.canEdit = true
  })
})
