import React from 'react'
import { act, create, type ReactTestRenderer } from 'react-test-renderer'
import { afterEach, describe, expect, it, vi } from 'vitest'
import { Modal, ModalContent } from '@chakra-ui/react'
import CreateViewDialog from './CreateViewDialog'

vi.mock('@chakra-ui/react', async () => {
  const ReactModule = await import('react')
  const BoxLike = ({ children, ...props }: { children?: React.ReactNode }) => ReactModule.createElement('div', props, children)
  return {
    ...Object.fromEntries(['Modal', 'ModalBody', 'ModalCloseButton', 'ModalContent', 'ModalFooter', 'ModalHeader', 'ModalOverlay', 'FormControl', 'FormLabel', 'Text'].map((name) => [name, BoxLike])),
    Button: ({ children, ...props }: { children?: React.ReactNode }) => ReactModule.createElement('button', props, children),
    Input: ReactModule.forwardRef<HTMLInputElement>((props, ref) => ReactModule.createElement('input', { ...props, ref })),
  }
})

let renderer: ReactTestRenderer
afterEach(() => { act(() => renderer?.unmount()) })

function renderDialog(overrides: Partial<React.ComponentProps<typeof CreateViewDialog>> = {}) {
  const props = { isOpen: true, onClose: vi.fn(), name: 'Payments', onNameChange: vi.fn(), onSubmit: vi.fn(async () => {}), isSaving: false, ...overrides }
  act(() => { renderer = create(<CreateViewDialog {...props} />) })
  return props
}

function form() {
  return renderer.root.findAllByType(ModalContent).find((node) => node.props.as === 'form')!
}

describe('CreateViewDialog', () => {
  it('submits the form once without navigating the browser', async () => {
    const props = renderDialog()
    const preventDefault = vi.fn()
    await act(async () => { form().props.onSubmit({ preventDefault }) })
    expect(preventDefault).toHaveBeenCalledOnce()
    expect(props.onSubmit).toHaveBeenCalledOnce()
    expect(props.onClose).not.toHaveBeenCalled()
  })

  it.each(['', '   '])('rejects an empty name %j', async (name) => {
    const props = renderDialog({ name })
    await act(async () => { form().props.onSubmit({ preventDefault: vi.fn() }) })
    expect(props.onSubmit).not.toHaveBeenCalled()
    expect(renderer.root.findAllByType('button').find((node) => node.props.type === 'submit')?.props.isDisabled).toBe(true)
  })

  it('blocks submission and dismissal while a create request is pending', async () => {
    const props = renderDialog({ isSaving: true })
    await act(async () => { form().props.onSubmit({ preventDefault: vi.fn() }) })
    const modal = renderer.root.findAllByType(Modal).find((node) => node.props.isOpen)!
    act(() => { modal.props.onClose() })
    const cancel = renderer.root.findAllByType('button').find((node) => node.props.type === 'button')!
    act(() => { cancel.props.onClick() })
    expect(props.onSubmit).not.toHaveBeenCalled()
    expect(props.onClose).not.toHaveBeenCalled()
    expect(cancel.props.isDisabled).toBe(true)
    expect(renderer.root.findByType('input').props.isDisabled).toBe(true)
  })
})
