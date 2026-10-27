import { createElement } from 'react'
import { act, create, type ReactTestRenderer } from 'react-test-renderer'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { copyTextToClipboard } from '../utils/clipboard'
import { toast } from '../utils/toast'
import { FlowYamlControls } from './FlowYamlControls'
import { parseFlowYaml } from './yaml'
import type { StoryFlow } from './model'

vi.mock('../utils/clipboard', () => ({ copyTextToClipboard: vi.fn(async () => {}) }))
vi.mock('../utils/toast', () => ({ toast: vi.fn() }))
vi.mock('@chakra-ui/react', () => ({
  Button: 'button', IconButton: 'button', Tooltip: ({ children }: { children: React.ReactNode }) => children, HStack: 'div', Input: 'input', Textarea: 'textarea', Text: 'p',
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
