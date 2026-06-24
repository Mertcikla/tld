import React from 'react'
import { act, create } from 'react-test-renderer'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import type { LibraryElement } from '../types'
import { fontAwesomeIconUrlForTechnologyLabel } from '../utils/fontAwesomeIcon'
import ElementPanel from './ElementPanel'

const apiMocks = vi.hoisted(() => ({
  createElement: vi.fn(),
  createCustomTechnology: vi.fn(),
  updateElement: vi.fn(),
}))

vi.mock('../api/client', () => ({
  api: {
    elements: {
      create: apiMocks.createElement,
      update: apiMocks.updateElement,
    },
    technology: {
      createCustom: apiMocks.createCustomTechnology,
    },
  },
}))

vi.mock('react-router-dom', () => ({
  useNavigate: () => vi.fn(),
}))

vi.mock('../pages/ViewEditor/context', () => ({
  useViewEditorContext: () => ({
    viewId: 1,
    canEdit: true,
    isOwner: true,
    isFreePlan: false,
    snapToGrid: true,
    setSnapToGrid: vi.fn(),
    selectedElement: null,
    selectedConnector: null,
  }),
}))

vi.mock('../utils/technologyCatalog', () => ({
  getTechnologyCatalogIndex: vi.fn(async () => ({ bySlug: new Map(), items: [] })),
  getTechnologyCatalogItemBySlug: vi.fn(async () => null),
  invalidateTechnologyCatalog: vi.fn(),
  resolveWithBase: (value: string) => value,
  searchTechnologyCatalog: vi.fn(async () => []),
}))

vi.mock('@chakra-ui/react', async () => {
  const ReactModule = await import('react')
  const BoxLike = ({ children, as: Component = 'div', ...props }: { children?: React.ReactNode; as?: React.ElementType }) => ReactModule.createElement(Component, props, children)
  const ButtonLike = ({ children, onClick, ...props }: { children?: React.ReactNode; onClick?: () => void }) => ReactModule.createElement('button', { ...props, onClick }, children)
  const InputLike = ReactModule.forwardRef<HTMLInputElement, Record<string, unknown>>((props, ref) => ReactModule.createElement('input', { ...props, ref }))
  InputLike.displayName = 'InputLike'

  return {
    Badge: BoxLike,
    Box: BoxLike,
    Button: ButtonLike,
    CloseButton: ButtonLike,
    Flex: BoxLike,
    FormControl: BoxLike,
    FormLabel: BoxLike,
    HStack: BoxLike,
    Input: InputLike,
    InputGroup: BoxLike,
    InputRightElement: BoxLike,
    Modal: ({ children, isOpen }: { children?: React.ReactNode; isOpen?: boolean }) => (isOpen ? ReactModule.createElement('div', {}, children) : null),
    ModalBody: BoxLike,
    ModalCloseButton: ButtonLike,
    ModalContent: BoxLike,
    ModalFooter: BoxLike,
    ModalHeader: BoxLike,
    ModalOverlay: BoxLike,
    Popover: BoxLike,
    PopoverArrow: BoxLike,
    PopoverBody: BoxLike,
    PopoverContent: BoxLike,
    PopoverTrigger: BoxLike,
    Slider: ({ children, isDisabled, onChangeEnd, ...props }: { children?: React.ReactNode; isDisabled?: boolean; onChangeEnd?: (value: number) => void }) => ReactModule.createElement('input', {
      ...props,
      disabled: isDisabled,
      isDisabled,
      type: 'range',
      onChange: (event: React.ChangeEvent<HTMLInputElement>) => onChangeEnd?.(Number(event.target.value)),
    }, children),
    SliderFilledTrack: BoxLike,
    SliderThumb: BoxLike,
    SliderTrack: BoxLike,
    Switch: ({ isChecked, isDisabled, onChange, ...props }: { isChecked?: boolean; isDisabled?: boolean; onChange?: React.ChangeEventHandler<HTMLInputElement> }) => ReactModule.createElement('input', {
      ...props,
      checked: isChecked,
      disabled: isDisabled,
      type: 'checkbox',
      onChange,
    }),
    Tag: BoxLike,
    TagCloseButton: ButtonLike,
    TagLabel: BoxLike,
    Text: BoxLike,
    Textarea: (props: Record<string, unknown>) => ReactModule.createElement('textarea', props),
    VStack: BoxLike,
    Wrap: BoxLike,
    WrapItem: BoxLike,
    useBreakpointValue: () => false,
    useDisclosure: () => {
      const [isOpen, setIsOpen] = ReactModule.useState(false)
      return { isOpen, onClose: () => setIsOpen(false), onOpen: () => setIsOpen(true) }
    },
    useToast: () => vi.fn(),
  }
})

vi.mock('./SlidingPanel', () => ({
  default: ({ isOpen, children }: { isOpen: boolean; children: React.ReactNode }) => (isOpen ? <div>{children}</div> : null),
}))
vi.mock('./ConfirmDialog', () => ({ default: () => null }))
vi.mock('./PanelHeader', () => ({ default: ({ title }: { title: string }) => <div>{title}</div> }))
vi.mock('./GitSourceLinker', () => ({ default: () => null }))
vi.mock('./ScrollIndicatorWrapper', () => ({ default: ({ children }: { children: React.ReactNode }) => <div>{children}</div> }))
vi.mock('./TagUpsert', () => ({ default: () => null }))

function element(overrides: Partial<LibraryElement> = {}): LibraryElement {
  return {
    id: 10,
    name: 'API',
    kind: 'service',
    description: null,
    technology: null,
    url: null,
    logo_url: null,
    technology_connectors: [],
    tags: [],
    repo: null,
    branch: null,
    file_path: null,
    language: null,
    created_at: '2024-01-01',
    updated_at: '2024-01-01',
    has_view: false,
    view_label: null,
    bypass_noise_gate: false,
    ...overrides,
  }
}

describe('ElementPanel bypass noise gate', () => {
  beforeEach(() => {
    vi.useFakeTimers()
    apiMocks.createElement.mockReset()
    apiMocks.createCustomTechnology.mockReset()
    apiMocks.updateElement.mockReset()
    apiMocks.createCustomTechnology.mockImplementation(async () => ({
      iconUrl: '/icons/acme-platform.svg',
      name: 'Acme Platform',
      nameShort: 'Acme',
      defaultSlug: 'acme-platform',
      aliases: ['acme-sdk'],
    }))
    apiMocks.updateElement.mockImplementation(async (id: number, payload: Partial<LibraryElement>) => ({
      ...element(),
      ...payload,
      id,
    }))
    Object.defineProperty(globalThis, 'window', {
      configurable: true,
      value: {
        addEventListener: vi.fn(),
        clearTimeout,
        removeEventListener: vi.fn(),
        setTimeout,
      },
    })
    Object.defineProperty(globalThis, 'URL', {
      configurable: true,
      value: {
        createObjectURL: vi.fn(() => 'blob:custom-icon'),
        revokeObjectURL: vi.fn(),
      },
    })
  })

  afterEach(() => {
    vi.useRealTimers()
  })

  it('autosaves the bypass flag and hides the noise gate slider while bypassed', async () => {
    const onSave = vi.fn()
    let renderer: ReturnType<typeof create>
    await act(async () => {
      renderer = create(
        <ElementPanel
          isOpen
          autoSave
          element={element()}
          onClose={vi.fn()}
          onSave={onSave}
          onVisibilityOverrideDeltaChange={vi.fn()}
        />,
      )
      await Promise.resolve()
    })

    expect(renderer!.root.findByProps({ 'aria-label': 'Element noise gate' }).props.isDisabled).toBe(false)

    const bypassToggle = renderer!.root.findByProps({ 'data-testid': 'element-panel-bypass-noise-gate' })
    expect(bypassToggle.props.isChecked).toBe(true)

    await act(async () => {
      bypassToggle.props.onChange({ target: { checked: false } })
    })
    await act(async () => {
      vi.runAllTimers()
      await Promise.resolve()
    })

    expect(apiMocks.updateElement).toHaveBeenCalledWith(10, expect.objectContaining({ bypass_noise_gate: true }))
    expect(onSave).toHaveBeenCalledWith(expect.objectContaining({ bypass_noise_gate: true }))
    expect(renderer!.root.findAllByProps({ 'aria-label': 'Element noise gate' })).toHaveLength(0)
  })

  it('creates a custom technology inline, attaches it, and autosaves the primary icon', async () => {
    const onSave = vi.fn()
    const fileInputClick = vi.fn()
    let renderer: ReturnType<typeof create>
    await act(async () => {
      renderer = create(
        <ElementPanel
          isOpen
          autoSave
          element={element()}
          onClose={vi.fn()}
          onSave={onSave}
          onVisibilityOverrideDeltaChange={vi.fn()}
        />,
        {
          createNodeMock: (node: { props?: Record<string, unknown> }) => {
            if (node.props?.['data-testid'] === 'custom-technology-file') {
              return { click: fileInputClick }
            }
            return null
          },
        },
      )
      await Promise.resolve()
    })

    expect(renderer!.root.findAllByProps({ 'data-testid': 'element-panel-custom-technology-open' })).toHaveLength(0)

    await act(async () => {
      renderer!.root.findByProps({ 'data-testid': 'element-panel-technology-input' }).props.onChange({ target: { value: 'Acme Platform' } })
      await Promise.resolve()
    })

    expect(renderer!.root.findAllByProps({ 'data-testid': 'element-panel-custom-technology-create' })).toHaveLength(0)
    await act(async () => {
      vi.advanceTimersByTime(150)
      await Promise.resolve()
    })
    const customCreateRows = renderer!.root.findAll((node) => (
      node.type === 'div' &&
      node.props['data-testid'] === 'element-panel-custom-technology-create'
    ))
    expect(customCreateRows).toHaveLength(1)
    expect(renderer!.root.findAllByProps({ 'data-testid': 'custom-technology-icon-dropzone' })).toHaveLength(0)

    await act(async () => {
      customCreateRows[0].props.onClick()
    })

    const iconDropzones = renderer!.root.findAll((node) => (
      node.type === 'button' &&
      node.props['data-testid'] === 'custom-technology-icon-dropzone'
    ))
    expect(iconDropzones).toHaveLength(1)
    const iconDropzone = iconDropzones[0]
    expect(iconDropzone.props['aria-label']).toBe('Choose custom technology icon')
    expect(renderer!.root.findAllByProps({ 'data-testid': 'custom-technology-file-trigger' })).toHaveLength(0)
    await act(async () => {
      iconDropzone.props.onClick({ stopPropagation: vi.fn() })
    })
    expect(fileInputClick).toHaveBeenCalledTimes(1)

    const file = new File(['<svg xmlns="http://www.w3.org/2000/svg"></svg>'], 'acme-platform.svg', { type: 'image/svg+xml' })
    await act(async () => {
      renderer!.root.findByProps({ 'data-testid': 'custom-technology-file' }).props.onChange({ target: { files: [file] } })
    })
    expect(renderer!.root.findAllByProps({ 'data-testid': 'custom-technology-aliases' })).toHaveLength(0)
    await act(async () => {
      renderer!.root.findByProps({ 'data-testid': 'custom-technology-options-toggle' }).props.onClick({ stopPropagation: vi.fn() })
    })
    await act(async () => {
      renderer!.root.findByProps({ 'data-testid': 'custom-technology-aliases' }).props.onChange({ target: { value: 'acme-sdk' } })
    })
    await act(async () => {
      await renderer!.root.findByProps({ 'data-testid': 'custom-technology-save' }).props.onClick({ stopPropagation: vi.fn() })
      await Promise.resolve()
    })
    await act(async () => {
      vi.runAllTimers()
      await Promise.resolve()
    })

    expect(apiMocks.createCustomTechnology).toHaveBeenCalledWith(expect.objectContaining({
      name: 'Acme Platform',
      aliases: ['acme-sdk'],
      media_type: 'image/svg+xml',
    }))
    expect(apiMocks.createCustomTechnology.mock.calls[0][0].icon).toBeInstanceOf(Uint8Array)
    expect(apiMocks.updateElement).toHaveBeenCalledWith(10, expect.objectContaining({
      logo_url: '/icons/acme-platform.svg',
      technology: 'Acme Platform',
      technology_connectors: [
        {
          type: 'catalog',
          slug: 'acme-platform',
          label: 'Acme Platform',
          is_primary_icon: true,
        },
      ],
    }))
    expect(onSave).toHaveBeenCalledWith(expect.objectContaining({ logo_url: '/icons/acme-platform.svg' }))
  })

  it('adds and deselects a Font Awesome technology icon without catalog metadata', async () => {
    const onSave = vi.fn()
    let renderer: ReturnType<typeof create>
    await act(async () => {
      renderer = create(
        <ElementPanel
          isOpen
          autoSave
          element={element()}
          onClose={vi.fn()}
          onSave={onSave}
          onVisibilityOverrideDeltaChange={vi.fn()}
        />,
      )
      await Promise.resolve()
    })

    await act(async () => {
      renderer!.root.findByProps({ 'data-testid': 'element-panel-technology-input' }).props.onChange({ target: { value: 'car' } })
      await Promise.resolve()
    })

    const fontAwesomeRows = renderer!.root.findAll((node) => (
      node.type === 'div' &&
      node.props['data-testid'] === 'element-panel-fontawesome-technology-option'
    ))
    expect(fontAwesomeRows).toHaveLength(1)

    await act(async () => {
      fontAwesomeRows[0].props.onClick()
      await Promise.resolve()
    })
    await act(async () => {
      vi.runAllTimers()
      await Promise.resolve()
      await Promise.resolve()
    })
    await act(async () => {
      vi.runAllTimers()
      await Promise.resolve()
      await Promise.resolve()
    })

    expect(apiMocks.updateElement).toHaveBeenLastCalledWith(10, expect.objectContaining({
      logo_url: expect.stringMatching(/^data:image\/svg\+xml;charset=utf-8,/),
      technology: 'fa:car',
      technology_connectors: [
        expect.objectContaining({
          type: 'catalog',
          label: 'fa:car',
          slug: 'fa:car',
          is_primary_icon: true,
        }),
      ],
    }))

    await act(async () => {
      renderer!.root.findByProps({ 'data-testid': 'element-panel-technology-chip' }).props.onClick()
      await Promise.resolve()
    })
    await act(async () => {
      vi.runAllTimers()
      await Promise.resolve()
      await Promise.resolve()
    })

    expect(apiMocks.updateElement).toHaveBeenLastCalledWith(10, expect.objectContaining({
      logo_url: '',
      technology_connectors: [
        expect.objectContaining({
          type: 'custom',
          label: 'fa:car',
          is_primary_icon: false,
        }),
      ],
    }))
  })

  it('persists a switched Font Awesome primary icon using the backend-compatible link shape', async () => {
    const onSave = vi.fn()
    let renderer: ReturnType<typeof create>
    await act(async () => {
      renderer = create(
        <ElementPanel
          isOpen
          autoSave
          element={element({
            technology: 'fa:fa-server',
            logo_url: fontAwesomeIconUrlForTechnologyLabel('fa:fa-server'),
            technology_connectors: [
              { type: 'custom', label: 'fa:fa-server', is_primary_icon: true },
            ],
          })}
          onClose={vi.fn()}
          onSave={onSave}
          onVisibilityOverrideDeltaChange={vi.fn()}
        />,
      )
      await Promise.resolve()
    })

    apiMocks.updateElement.mockClear()

    await act(async () => {
      renderer!.root.findByProps({ 'data-testid': 'element-panel-technology-input' }).props.onChange({ target: { value: 'fa:fa-car' } })
      await Promise.resolve()
    })
    const fontAwesomeRows = renderer!.root.findAll((node) => (
      node.type === 'div' &&
      node.props['data-testid'] === 'element-panel-fontawesome-technology-option'
    ))
    expect(fontAwesomeRows).toHaveLength(1)

    await act(async () => {
      fontAwesomeRows[0].props.onClick()
      await Promise.resolve()
    })
    await act(async () => {
      vi.runAllTimers()
      await Promise.resolve()
      await Promise.resolve()
    })
    apiMocks.updateElement.mockClear()

    const chips = renderer!.root.findAll((node) => (
      node.type === 'div' &&
      node.props['data-testid'] === 'element-panel-technology-chip'
    ))
    expect(chips).toHaveLength(2)
    await act(async () => {
      chips[1].props.onClick()
      await Promise.resolve()
    })
    await act(async () => {
      vi.runAllTimers()
      await Promise.resolve()
      await Promise.resolve()
    })

    expect(apiMocks.updateElement).toHaveBeenLastCalledWith(10, expect.objectContaining({
      logo_url: expect.stringMatching(/^data:image\/svg\+xml;charset=utf-8,/),
      technology: 'fa:fa-server, fa:car',
      technology_connectors: [
        expect.objectContaining({
          type: 'custom',
          label: 'fa:fa-server',
          is_primary_icon: false,
        }),
        expect.objectContaining({
          type: 'catalog',
          label: 'fa:car',
          slug: 'fa:car',
          is_primary_icon: true,
        }),
      ],
    }))
  })
})
