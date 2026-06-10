import React from 'react'
import { act, create } from 'react-test-renderer'
import { describe, expect, it, vi } from 'vitest'
import ImportModal from './ImportModal'

vi.mock('../config/runtime', () => ({ isWailsApp: false }))

vi.mock('../api/client', () => ({
  api: {
    import: {
      parseStructurizr: vi.fn(),
    },
  },
}))

vi.mock('../lib/desktop', () => ({
  mermaidImportFilters: [],
  onFileDrop: vi.fn(),
  openTextFile: vi.fn(),
  readTextFile: vi.fn(),
}))

vi.mock('@chakra-ui/react', async () => {
  const ReactModule = await import('react')
  type NodeProps = { children?: React.ReactNode; isOpen?: boolean }
  const BoxLike = ({ children, isOpen, ...props }: NodeProps) => (isOpen === false ? null : ReactModule.createElement('div', props, children))
  const ButtonLike = ({ children, onClick, ...props }: { children?: React.ReactNode; onClick?: () => void }) => ReactModule.createElement('button', { ...props, onClick }, children)

  return {
    Box: BoxLike,
    Button: ButtonLike,
    Divider: BoxLike,
    FormControl: BoxLike,
    FormLabel: BoxLike,
    HStack: BoxLike,
    Modal: BoxLike,
    ModalBody: BoxLike,
    ModalCloseButton: BoxLike,
    ModalContent: BoxLike,
    ModalFooter: BoxLike,
    ModalHeader: BoxLike,
    ModalOverlay: BoxLike,
    Tab: BoxLike,
    TabList: BoxLike,
    TabPanel: BoxLike,
    TabPanels: BoxLike,
    Tabs: BoxLike,
    Text: BoxLike,
    Textarea: ({ onChange, ...props }: { onChange?: React.ChangeEventHandler<HTMLTextAreaElement> }) => ReactModule.createElement('textarea', { ...props, onChange }),
    VStack: BoxLike,
  }
})

describe('ImportModal warnings', () => {
  it('shows non-blocking import warnings and still allows confirmation', async () => {
    const onImport = vi.fn()
    const getImportWarnings = vi.fn(() => ['Name mismatch warning'])
    let renderer!: ReturnType<typeof create>
    await act(async () => {
      renderer = create(
        <ImportModal
          isOpen
          onClose={vi.fn()}
          onImport={onImport}
          getImportWarnings={getImportWarnings}
        />,
      )
    })

    act(() => {
      renderer.root.findByProps({ 'data-testid': 'import-mermaid-textarea' }).props.onChange({
        target: { value: 'flowchart LR\n  A --> B' },
      })
    })
    await act(async () => {
      await renderer.root.findByProps({ 'data-testid': 'import-next' }).props.onClick()
    })

    expect(getImportWarnings).toHaveBeenCalled()
    expect(JSON.stringify(renderer.toJSON())).toContain('Name mismatch warning')

    await act(async () => {
      await renderer.root.findByProps({ 'data-testid': 'import-confirm' }).props.onClick()
    })

    expect(onImport).toHaveBeenCalled()
  })
})
