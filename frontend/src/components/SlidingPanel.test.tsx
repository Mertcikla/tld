import React from 'react'
import { act, create } from 'react-test-renderer'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import SlidingPanel from './SlidingPanel'

const { layout } = vi.hoisted(() => ({ layout: { isMobile: false, leftPanelOffset: 0, sidePanelStyle: undefined as 'flat' | undefined } }))

vi.mock('framer-motion', async () => {
  const ReactModule = await import('react')
  const MotionDiv = ({ children, ...props }: { children?: React.ReactNode }) => ReactModule.createElement('div', props, children)

  return {
    AnimatePresence: ({ children }: { children?: React.ReactNode }) => ReactModule.createElement(ReactModule.Fragment, null, children),
    motion: {
      div: MotionDiv,
    },
  }
})

vi.mock('@chakra-ui/react', async () => {
  const ReactModule = await import('react')
  const BoxLike = ({ children, ...props }: { children?: React.ReactNode }) => ReactModule.createElement('div', props, children)
  const FocusLockLike = ({ children, isDisabled }: { children?: React.ReactNode; isDisabled?: boolean }) => (
    ReactModule.createElement('div', { 'data-testid': 'focus-lock', 'data-disabled': String(!!isDisabled) }, children)
  )

  return {
    Box: BoxLike,
    FocusLock: FocusLockLike,
    useBreakpointValue: () => layout.isMobile,
  }
})

vi.mock('../pages/ViewEditor/context', () => ({
  useViewEditorContext: () => ({
    isMarkdownOpen: false,
    markdownPaneWidth: 0,
    leftPanelOffset: layout.leftPanelOffset,
    sidePanelStyle: layout.sidePanelStyle,
  }),
}))

beforeEach(() => { layout.isMobile = false; layout.leftPanelOffset = 0; layout.sidePanelStyle = undefined })

describe('SlidingPanel focus lock', () => {
  it('does not trap focus for modeless panels without a backdrop', () => {
    const renderer = create(
      <SlidingPanel isOpen onClose={vi.fn()} panelKey="element" hasBackdrop={false}>
        <button>Panel action</button>
      </SlidingPanel>,
    )

    expect(renderer.root.findByProps({ 'data-testid': 'focus-lock' }).props['data-disabled']).toBe('true')
  })

  it('keeps focus trapped for modal panels with a backdrop', () => {
    const renderer = create(
      <SlidingPanel isOpen onClose={vi.fn()} panelKey="element" hasBackdrop>
        <button>Panel action</button>
      </SlidingPanel>,
    )

    expect(renderer.root.findByProps({ 'data-testid': 'focus-lock' }).props['data-disabled']).toBe('false')
  })
})


describe('SlidingPanel order beside Flows', () => {
  it.each([undefined, 'flat'] as const)('places left panels after the docked flow panel in %s mode', style => {
    layout.leftPanelOffset = 360
    layout.sidePanelStyle = style
    const renderer = create(<SlidingPanel isOpen onClose={vi.fn()} panelKey="inventory" side="left" hasBackdrop={false} data-testid="inventory">Inventory</SlidingPanel>)
    const panel = renderer.root.findAllByType('div').find(node => node.props['data-testid'] === 'inventory')!
    const wrapper = renderer.root.findAllByType('div').find(node => node.props.style?.pointerEvents === 'none')!
    expect(wrapper.props.style.left).toBe(style === 'flat' ? 360 : 'calc(1rem + 360px)')
    expect(panel.props.maxW).toBe(style === 'flat' ? 'calc(100% - 360px)' : 'calc(100vw - 384px)')
    act(() => renderer.unmount())
  })

  it('returns left panels to the edge when Flows closes', () => {
    layout.leftPanelOffset = 360
    const renderer = create(<SlidingPanel isOpen onClose={vi.fn()} panelKey="inventory" side="left" hasBackdrop={false}>Inventory</SlidingPanel>)
    layout.leftPanelOffset = 0
    act(() => renderer.update(<SlidingPanel isOpen onClose={vi.fn()} panelKey="inventory" side="left" hasBackdrop={false}>Inventory</SlidingPanel>))
    const wrapper = renderer.root.findAllByType('div').find(node => node.props.style?.pointerEvents === 'none')!
    expect(wrapper.props.style.left).toBe('1rem')
    act(() => renderer.unmount())
  })

  it('keeps mobile drawers and right panels at their existing edges', () => {
    layout.leftPanelOffset = 360
    layout.isMobile = true
    const renderer = create(<SlidingPanel isOpen onClose={vi.fn()} panelKey="inventory" side="left" hasBackdrop={false}>Inventory</SlidingPanel>)
    let wrapper = renderer.root.findAllByType('div').find(node => node.props.style?.pointerEvents === 'none')!
    expect(wrapper.props.style.left).toBe('1rem')
    layout.isMobile = false
    act(() => renderer.update(<SlidingPanel isOpen onClose={vi.fn()} panelKey="inspector" side="right" hasBackdrop={false}>Inspector</SlidingPanel>))
    wrapper = renderer.root.findAllByType('div').find(node => node.props.style?.pointerEvents === 'none')!
    expect(wrapper.props.style.left).toBeUndefined()
    expect(wrapper.props.style.right).toBe('calc(1rem + 0px)')
    act(() => renderer.unmount())
  })
})
