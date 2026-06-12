import React from 'react'
import { act, create } from 'react-test-renderer'
import { describe, expect, it, vi } from 'vitest'
import ExportModal from './ExportModal'

vi.mock('@chakra-ui/react', async () => {
  const ReactModule = await import('react')
  type NodeProps = { children?: React.ReactNode; isOpen?: boolean }
  const BoxLike = ({ children, isOpen, ...props }: NodeProps) => (isOpen === false ? null : ReactModule.createElement('div', props, children))
  const ButtonLike = ({ children, onClick, ...props }: { children?: React.ReactNode; onClick?: () => void }) => ReactModule.createElement('button', { ...props, onClick }, children)

  return {
    Button: ButtonLike,
    Checkbox: ({ children, isChecked, onChange, ...props }: { children?: React.ReactNode; isChecked?: boolean; onChange?: React.ChangeEventHandler<HTMLInputElement> }) => ReactModule.createElement('label', null,
      ReactModule.createElement('input', { ...props, checked: isChecked, onChange, type: 'checkbox' }),
      children,
    ),
    FormControl: BoxLike,
    FormLabel: BoxLike,
    HStack: BoxLike,
    Input: ({ onChange, ...props }: { onChange?: React.ChangeEventHandler<HTMLInputElement> }) => ReactModule.createElement('input', { ...props, onChange }),
    Modal: BoxLike,
    ModalBody: BoxLike,
    ModalCloseButton: BoxLike,
    ModalContent: BoxLike,
    ModalFooter: BoxLike,
    ModalHeader: BoxLike,
    ModalOverlay: BoxLike,
    Radio: ({ children, value, ...props }: { children?: React.ReactNode; value?: string }) => ReactModule.createElement('label', null,
      ReactModule.createElement('input', { ...props, type: 'radio', value }),
      children,
    ),
    RadioGroup: BoxLike,
    Text: BoxLike,
    VStack: BoxLike,
  }
})

describe('ExportModal', () => {
  it('passes the Mermaid metadata flag to export', async () => {
    const onExport = vi.fn()
    let renderer!: ReturnType<typeof create>
    await act(async () => {
      renderer = create(
        <ExportModal
          isOpen
          onClose={vi.fn()}
          defaultFilename="System"
          mermaidEnabled
          onExport={onExport}
        />,
      )
    })

    act(() => {
      renderer.root.findByProps({ name: 'format' }).props.onChange('mermaid')
    })
    act(() => {
      renderer.root.findAllByType('input').find((input) => input.props.type === 'checkbox')?.props.onChange({
        currentTarget: { checked: false },
      })
    })
    await act(async () => {
      await renderer.root.findByProps({ 'data-testid': 'export-submit' }).props.onClick()
    })

    expect(onExport).toHaveBeenCalledWith({
      format: 'mermaid',
      scale: 2,
      filename: 'System',
      includeTldMetadata: false,
    })
  })
})
