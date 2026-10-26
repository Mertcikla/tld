import { createElement } from 'react'
import { act, create, type ReactTestRenderer } from 'react-test-renderer'
import { afterEach, describe, expect, it, vi } from 'vitest'
import { FlowYamlControls } from './FlowYamlControls'
import { parseFlowYaml } from './yaml'
import type { StoryFlow } from './model'

vi.mock('@chakra-ui/react', () => ({
  Button: 'button', HStack: 'div', Input: 'input', Textarea: 'textarea', Text: 'p',
  Modal: ({ isOpen, children, onClose }: { isOpen: boolean; children: React.ReactNode; onClose: () => void }) => isOpen ? createElement('dialog', { onClose }, children) : null,
  ModalBody: 'div', ModalCloseButton: 'span', ModalContent: 'div', ModalFooter: 'div', ModalHeader: 'h2', ModalOverlay: 'div',
}))
const flow: StoryFlow = { id: 'saved-id', revision: 6, viewId: 1, name: 'Story', description: '', steps: [] }
const source = 'format: tld-flow\nversion: 1\nname: Imported story\nsteps: []\n'
let renderer: ReactTestRenderer
afterEach(() => act(() => renderer?.unmount()))
const button = (label: string) => renderer.root.findAllByType('button').find(node => node.props.children === label)!

describe('flow YAML controls', () => {
  it('opens exported YAML for read-only viewers without import controls', () => {
    act(() => { renderer = create(<FlowYamlControls flow={flow} diagram={{ elements: [], connectors: [] }} canImport={false} disabled={false} onImport={vi.fn()} />) })
    expect(button('Import YAML')).toBeUndefined()
    act(() => button('Export YAML').props.onClick())
    const field = renderer.root.findByType('textarea')
    expect(field.props.isReadOnly).toBe(true)
    expect(parseFlowYaml(field.props.value).name).toBe('Story')
    expect(field.props.value).not.toContain('saved-id')
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
