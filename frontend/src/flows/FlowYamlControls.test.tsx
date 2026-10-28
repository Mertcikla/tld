import { createElement } from 'react'
import { act, create, type ReactTestRenderer } from 'react-test-renderer'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { copyTextToClipboard } from '../utils/clipboard'
import { toast } from '../utils/toast'
import { FlowYamlControls } from './FlowYamlControls'
import { exportCompleteFlowYaml, parseFlowYaml } from './yaml'
import type { StoryFlow } from './model'
import { decodeFlowSnapshot } from './sharing'

vi.mock('../utils/clipboard', () => ({ copyTextToClipboard: vi.fn(async () => {}) }))
vi.mock('react-router-dom', () => ({ useHref: (path: string) => `/app${path}` }))
vi.mock('../utils/toast', () => ({ toast: vi.fn() }))
vi.mock('@chakra-ui/react', () => ({
  Checkbox: 'checkbox', Button: 'button', IconButton: 'button', Tooltip: ({ children }: { children: React.ReactNode }) => children, HStack: 'div', Input: 'input', Textarea: 'textarea', Text: 'p',
  Modal: ({ isOpen, children, onClose }: { isOpen: boolean; children: React.ReactNode; onClose: () => void }) => isOpen ? createElement('dialog', { onClose }, children) : null,
  ModalBody: 'div', ModalCloseButton: 'span', ModalContent: 'div', ModalFooter: 'div', ModalHeader: 'h2', ModalOverlay: 'div',
}))
const flow: StoryFlow = { id: 'saved-id', revision: 6, viewId: 1, name: 'Story', description: '', steps: [] }
const source = 'format: tld-flow\nversion: 1\nname: Imported story\nsteps: []\n'
let renderer: ReactTestRenderer
afterEach(() => act(() => renderer?.unmount()))
const button = (label: string) => renderer.root.findAllByType('button').find(node => node.props.children === label || node.props['aria-label'] === label)!

describe('flow YAML controls', () => {
  beforeEach(() => {
    vi.mocked(copyTextToClipboard).mockReset().mockResolvedValue(undefined)
    vi.mocked(toast).mockClear()
  })

  it('opens exported YAML for read-only viewers without import controls', () => {
    act(() => { renderer = create(<FlowYamlControls flow={flow} diagram={{ elements: [], connectors: [] }} canImport={false} disabled={false} onImport={vi.fn()} />) })
    expect(button('Import YAML')).toBeUndefined()
    act(() => button('Export YAML').props.onClick())
    const field = renderer.root.findByType('textarea')
    expect(field.props.isReadOnly).toBe(true)
    expect(parseFlowYaml(field.props.value).name).toBe('Story')
    expect(field.props.value).not.toContain('saved-id')
  })

  it('shares a complete snapshot independently of the export checkbox, preserving the router basename', async () => {
    vi.stubGlobal('window', { location: { href: 'https://example.com/app/views/42?element=123' } })
    try {
      act(() => { renderer = create(<FlowYamlControls flow={flow} diagram={{ elements: [], connectors: [] }} canImport={false} disabled={false} onImport={vi.fn()} />) })
      await act(async () => { await button('Share flow').props.onClick() })
      // onClick intentionally starts async work without awaiting it, as React does.
      await vi.waitFor(() => expect(renderer.root.findByProps({ 'aria-label': 'Flow share link' }).props.value).toContain('#flow=v1.'))
      const link = renderer.root.findByProps({ 'aria-label': 'Flow share link' }).props.value
      expect(new URL(link).pathname).toBe('/app/explore/shared/flow-snapshot')
      expect((await decodeFlowSnapshot(new URL(link).hash)).catalog.flows[0].name).toBe('Story')
      await act(async () => { button('Copy link').props.onClick() })
      expect(copyTextToClipboard).toHaveBeenCalledWith(link)
      expect(toast).toHaveBeenCalledWith({ status: 'success', title: 'Copied flow link' })
      expect(button('Import YAML')).toBeUndefined()
    } finally { vi.unstubAllGlobals() }
  })

  it('selects and copies the full exported YAML when clicked, and offers a copy button', async () => {
    act(() => { renderer = create(<FlowYamlControls flow={flow} diagram={{ elements: [], connectors: [] }} canImport={false} disabled={false} onImport={vi.fn()} />) })
    act(() => button('Export YAML').props.onClick())
    const field = renderer.root.findByType('textarea')
    const select = vi.fn()
    await act(async () => { field.props.onClick({ currentTarget: { select } }) })
    expect(select).toHaveBeenCalledOnce()
    expect(copyTextToClipboard).toHaveBeenCalledWith(field.props.value)
    expect(toast).toHaveBeenCalledWith({ status: 'success', title: 'Copied flow YAML' })

    vi.mocked(copyTextToClipboard).mockClear()
    await act(async () => { button('Copy YAML').props.onClick() })
    expect(copyTextToClipboard).toHaveBeenCalledWith(field.props.value)
  })

  it('reports copy failure while keeping export and retry available', async () => {
    vi.mocked(copyTextToClipboard).mockRejectedValueOnce(new Error('Clipboard denied'))
    act(() => { renderer = create(<FlowYamlControls flow={flow} diagram={{ elements: [], connectors: [] }} canImport={false} disabled={false} onImport={vi.fn()} />) })
    act(() => button('Export YAML').props.onClick())
    await act(async () => { button('Copy YAML').props.onClick() })
    expect(toast).toHaveBeenCalledWith(expect.objectContaining({ status: 'error', title: 'Copy failed' }))
    expect(button('Download YAML').props.isDisabled).toBe(false)
    await act(async () => { button('Copy YAML').props.onClick() })
    expect(toast).toHaveBeenLastCalledWith({ status: 'success', title: 'Copied flow YAML' })
  })

  it('keeps import text editable without selecting or copying on click', () => {
    act(() => { renderer = create(<FlowYamlControls flow={null} viewId={42} diagram={{ elements: [], connectors: [] }} canImport disabled={false} onImport={vi.fn()} />) })
    act(() => button('Import YAML').props.onClick())
    const field = renderer.root.findByType('textarea')
    const select = vi.fn()
    act(() => field.props.onClick({ currentTarget: { select } }))
    expect(field.props.isReadOnly).toBe(false)
    expect(select).not.toHaveBeenCalled()
    expect(copyTextToClipboard).not.toHaveBeenCalled()
    expect(button('Copy YAML')).toBeUndefined()
  })

  it('validates before importing, creates an unsaved draft in the target view, and closes', () => {
    const onImport = vi.fn(() => true)
    act(() => { renderer = create(<FlowYamlControls flow={null} viewId={42} diagram={{ elements: [], connectors: [] }} canImport disabled={false} onImport={onImport} />) })
    act(() => button('Import YAML').props.onClick())
    act(() => renderer.root.findByType('textarea').props.onChange({ target: { value: 'version: 999' } }))
    act(() => button('Import as draft').props.onClick())
    expect(onImport).not.toHaveBeenCalled()
    expect(renderer.root.findByProps({ role: 'alert' })).toBeDefined()
    act(() => renderer.root.findByType('textarea').props.onChange({ target: { value: source } }))
    act(() => button('Import as draft').props.onClick())
    expect(onImport).toHaveBeenCalledWith({ id: '', revision: 0, viewId: 42, name: 'Imported story', description: '', steps: [] })
    expect(renderer.root.findAllByType('dialog')).toHaveLength(0)
  })

  it('keeps the import open if the caller declines discarding an existing draft', () => {
    const onImport = vi.fn(() => false)
    act(() => { renderer = create(<FlowYamlControls flow={flow} viewId={42} diagram={{ elements: [], connectors: [] }} canImport disabled={false} onImport={onImport} />) })
    act(() => button('Import YAML').props.onClick())
    act(() => renderer.root.findByType('textarea').props.onChange({ target: { value: source } }))
    act(() => button('Import as draft').props.onClick())
    expect(renderer.root.findAllByType('dialog')).toHaveLength(1)
    expect(renderer.root.findByType('textarea').props.value).toBe(source)
  })
})


describe('complete flow controls', () => {
  const bundled = exportCompleteFlowYaml({ ...flow, name: 'Portable story' }, { elements: [], connectors: [] })
  function openImport(onImport = vi.fn(() => true), onImportComplete = vi.fn(async () => true)) {
    act(() => { renderer = create(<FlowYamlControls flow={null} viewId={42} diagram={{ elements: [], connectors: [] }} canImport disabled={false} onImport={onImport} onImportComplete={onImportComplete} />) })
    act(() => button('Import YAML').props.onClick())
    act(() => renderer.root.findByType('textarea').props.onChange({ target: { value: bundled } }))
    act(() => button('Import as draft').props.onClick())
    return { onImport, onImportComplete }
  }

  it('defaults to lightweight export and resets the complete option on reopen', () => {
    act(() => { renderer = create(<FlowYamlControls flow={flow} diagram={{ elements: [], connectors: [] }} canImport={false} disabled={false} onImport={vi.fn()} />) })
    act(() => button('Export YAML').props.onClick())
    expect(parseFlowYaml(renderer.root.findByType('textarea').props.value).resources).toBeUndefined()
    act(() => renderer.root.findByType('checkbox' as never).props.onChange({ target: { checked: true } }))
    expect(parseFlowYaml(renderer.root.findByType('textarea').props.value).resources).toEqual({ elements: [], connectors: [] })
    act(() => button('Close').props.onClick())
    act(() => button('Export YAML').props.onClick())
    expect(renderer.root.findByType('checkbox' as never).props.isChecked).toBe(false)
    expect(parseFlowYaml(renderer.root.findByType('textarea').props.value).resources).toBeUndefined()
  })

  it('waits for confirmation before invoking resource creation and allows cancellation', async () => {
    const { onImport, onImportComplete } = openImport()
    expect(onImport).not.toHaveBeenCalled()
    expect(onImportComplete).not.toHaveBeenCalled()
    expect(renderer.root.findAllByType('textarea')).toHaveLength(0)
    act(() => button('Back').props.onClick())
    expect(renderer.root.findByType('textarea').props.value).toBe(bundled)
    act(() => button('Import as draft').props.onClick())
    await act(async () => { button('Create new view').props.onClick() })
    expect(onImportComplete).toHaveBeenCalledWith(bundled)
    expect(onImport).not.toHaveBeenCalled()
    expect(renderer.root.findAllByType('dialog')).toHaveLength(0)
  })

  it('offers the existing import path for bundles without creating resources', () => {
    const { onImport, onImportComplete } = openImport()
    act(() => button('Use existing diagram').props.onClick())
    expect(onImport).toHaveBeenCalledWith(expect.objectContaining({ viewId: 42, id: '', name: 'Portable story' }))
    expect(onImportComplete).not.toHaveBeenCalled()
  })

  it('keeps confirmation available on failure and blocks closing while creation is pending', async () => {
    let reject!: (error: Error) => void
    const onImportComplete = vi.fn(() => new Promise<boolean>((_, no) => { reject = no }))
    openImport(undefined, onImportComplete)
    await act(async () => { button('Create new view').props.onClick() })
    expect(button('Close').props.isDisabled).toBe(true)
    act(() => button('Close').props.onClick())
    expect(renderer.root.findAllByType('dialog')).toHaveLength(1)
    await act(async () => { reject(new Error('Write failed')) })
    expect(renderer.root.findByProps({ role: 'alert' }).props.children).toBe('Write failed')
    expect(button('Create new view').props.isLoading).toBe(false)
  })
})
