import React from 'react'
import { act, create } from 'react-test-renderer'
import { describe, expect, it, vi } from 'vitest'
import ElementLibrary from './ElementLibrary'
import type { LibraryElement } from '../types'

vi.mock('../api/client', () => ({
  api: {
    elements: {
      list: vi.fn(async () => []),
    },
  },
}))

vi.mock('../pages/ViewEditor/context', () => ({
  useViewEditorContext: () => ({ canEdit: true }),
}))

vi.mock('./SlidingPanel', async () => {
  const ReactModule = await import('react')
  return {
    default: ({ children, isOpen }: { children?: React.ReactNode; isOpen?: boolean }) => (
      isOpen === false ? null : ReactModule.createElement('div', { 'data-testid': 'sliding-panel' }, children)
    ),
  }
})

vi.mock('./PanelHeader', async () => {
  const ReactModule = await import('react')
  return {
    default: ({ children }: { children?: React.ReactNode }) => ReactModule.createElement('div', null, children),
  }
})

vi.mock('./PanelUI', async () => {
  const ReactModule = await import('react')
  return {
    KbdHint: ({ children }: { children?: React.ReactNode }) => ReactModule.createElement('span', null, children),
  }
})

vi.mock('./ScrollIndicatorWrapper', async () => {
  const ReactModule = await import('react')
  return {
    default: ReactModule.forwardRef<HTMLDivElement, { children?: React.ReactNode }>(
      ({ children, ...props }, ref) => ReactModule.createElement('div', { ...props, ref }, children),
    ),
  }
})

vi.mock('@chakra-ui/icons', async () => {
  const ReactModule = await import('react')
  const Icon = () => ReactModule.createElement('span', null)
  return {
    AddIcon: Icon,
    CheckIcon: Icon,
    SearchIcon: Icon,
    ViewIcon: Icon,
  }
})

vi.mock('@chakra-ui/react', async () => {
  const ReactModule = await import('react')
  type NodeProps = {
    as?: React.ElementType
    children?: React.ReactNode
    icon?: React.ReactNode
    isOpen?: boolean
  }
  const BoxLike = ({ as, children, icon, isOpen, ...props }: NodeProps) => {
    if (isOpen === false) return null
    const Element = as ?? 'div'
    return ReactModule.createElement(Element, props, icon ?? children)
  }
  const InputLike = ({ onChange, ...props }: { onChange?: React.ChangeEventHandler<HTMLInputElement> }) => (
    ReactModule.createElement('input', { ...props, onChange })
  )

  return {
    Badge: BoxLike,
    Box: BoxLike,
    Button: BoxLike,
    Checkbox: BoxLike,
    Divider: BoxLike,
    Flex: BoxLike,
    HStack: BoxLike,
    IconButton: BoxLike,
    Input: InputLike,
    InputGroup: BoxLike,
    InputLeftElement: BoxLike,
    Spinner: BoxLike,
    Text: BoxLike,
    Tooltip: BoxLike,
    VStack: BoxLike,
    useBreakpointValue: () => false,
  }
})

function libraryElement(overrides: Partial<LibraryElement>): LibraryElement {
  return {
    id: 1,
    name: 'Car',
    kind: 'service',
    description: null,
    technology: null,
    url: null,
    logo_url: null,
    technology_connectors: [],
    tags: [],
    created_at: '2026-01-01T00:00:00Z',
    updated_at: '2026-01-01T00:00:00Z',
    has_view: false,
    view_label: null,
    bypass_noise_gate: false,
    ...overrides,
  }
}

describe('ElementLibrary icons', () => {
  it('renders a Font Awesome technology connector when logo_url is missing', async () => {
    let renderer!: ReturnType<typeof create>
    await act(async () => {
      renderer = create(
        <ElementLibrary
          existingElementIds={new Set()}
          existingElements={[libraryElement({
            technology_connectors: [
              { type: 'custom', label: 'fa:fa-car', is_primary_icon: true },
            ],
          })]}
          onCreateNew={vi.fn()}
          isOpen
          onClose={vi.fn()}
        />,
      )
    })

    const images = renderer.root.findAllByType('img')
    expect(images).toHaveLength(1)
    expect(images[0].props.src).toMatch(/^data:image\/svg\+xml;charset=utf-8,/)
  })
})
