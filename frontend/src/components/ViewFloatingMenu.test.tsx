import React from 'react'
import { act, create } from 'react-test-renderer'
import { describe, expect, it, vi } from 'vitest'
import ViewFloatingMenu from './ViewFloatingMenu'

vi.mock('../pages/ViewEditor/context', () => ({
  useViewEditorContext: () => ({
    canEdit: true,
  }),
}))



vi.mock('@chakra-ui/react', async () => {
  const ReactModule = await import('react')
  const BoxLike = ({ children, ...props }: { children?: React.ReactNode }) => ReactModule.createElement('div', props, children)
  const ButtonLike = ({ children, onClick, ...props }: { children?: React.ReactNode; onClick?: () => void }) => ReactModule.createElement('button', { ...props, onClick }, children)

  return {
    Box: BoxLike,
    Button: ButtonLike,
    HStack: BoxLike,
    IconButton: ButtonLike,
    Popover: BoxLike,
    PopoverBody: BoxLike,
    PopoverContent: BoxLike,
    PopoverTrigger: BoxLike,
    Portal: BoxLike,
    Slider: ({ children, isDisabled, onChangeEnd, ...props }: { children?: React.ReactNode; isDisabled?: boolean; onChangeEnd?: (value: number) => void }) => ReactModule.createElement('div', {
      ...props,
      isDisabled,
      onChangeEnd,
      role: 'slider',
    }, children),
    SliderFilledTrack: BoxLike,
    SliderThumb: BoxLike,
    SliderTrack: BoxLike,
    Switch: ({ isChecked, isDisabled, onChange, ...props }: { isChecked?: boolean; isDisabled?: boolean; onChange?: React.ChangeEventHandler<HTMLInputElement> }) => ReactModule.createElement('input', {
      ...props,
      checked: isChecked,
      disabled: isDisabled,
      isDisabled,
      type: 'checkbox',
      onChange,
    }),
    Text: BoxLike,
    Tooltip: BoxLike,
    VStack: BoxLike,
    useDisclosure: () => ({ isOpen: true, onClose: vi.fn(), onToggle: vi.fn() }),
  }
})

function renderMenu(overrides: Partial<React.ComponentProps<typeof ViewFloatingMenu>> = {}) {
  return create(
    <ViewFloatingMenu
      drawingMode={false}
      setDrawingMode={vi.fn()}
      hasDrawingPaths={false}
      drawingVisible
      setDrawingVisible={vi.fn()}
      extrasOpen={false}
      setExtrasOpen={vi.fn()}
      onImport={vi.fn()}
      onExport={vi.fn()}
      focusMode={false}
      onFocusModeChange={vi.fn()}
      densityLevel={0}
      onDensityLevelChange={vi.fn()}
      allTags={[]}
      layers={[]}
      tagColors={{}}
      hiddenTags={[]}
      toggleTagVisibility={vi.fn()}
      toggleLayerVisibility={vi.fn()}
      tagCounts={{}}
      layerElementCounts={{}}
      setHighlightedTags={vi.fn()}
      setHighlightColor={vi.fn()}
      {...overrides}
    />,
  )
}

describe('ViewFloatingMenu noise gate toggle', () => {
  it('renders Mermaid copy and generic export actions in the extras menu', () => {
    const onCopyMermaid = vi.fn()
    const onExport = vi.fn()
    const renderer = renderMenu({
      extrasOpen: true,
      onCopyMermaid,
      onExport,
    })

    act(() => {
      renderer.root.findByProps({ 'data-testid': 'vieweditor-toolbar-copy-mermaid' }).props.onClick()
    })
    act(() => {
      renderer.root.findByProps({ 'data-testid': 'vieweditor-toolbar-export' }).props.onClick()
    })

    expect(onCopyMermaid).toHaveBeenCalledOnce()
    expect(onExport).toHaveBeenCalledOnce()
  })

  it('shows an off gate with a disabled density slider and can request initialization', () => {
    const onNoiseGateEnabledChange = vi.fn()
    const renderer = renderMenu({
      noiseGateEnabled: false,
      onNoiseGateEnabledChange,
    })

    expect(renderer.root.findByProps({ 'aria-label': 'Noise gate' }).props.isDisabled).toBe(true)

    act(() => {
      renderer.root.findByProps({ 'data-testid': 'vieweditor-noise-gate-toggle' }).props.onChange({ target: { checked: true } })
    })

    expect(onNoiseGateEnabledChange).toHaveBeenCalledWith(true)
  })

  it('allows density changes while enabled and can request disabling', () => {
    const onDensityLevelChange = vi.fn()
    const onNoiseGateEnabledChange = vi.fn()
    const renderer = renderMenu({
      noiseGateEnabled: true,
      onDensityLevelChange,
      onNoiseGateEnabledChange,
    })

    const slider = renderer.root.findByProps({ 'aria-label': 'Noise gate' })
    expect(slider.props.isDisabled).toBe(false)

    act(() => {
      slider.props.onChangeEnd(1)
    })
    act(() => {
      renderer.root.findByProps({ 'data-testid': 'vieweditor-noise-gate-toggle' }).props.onChange({ target: { checked: false } })
    })

    expect(onDensityLevelChange).toHaveBeenCalledWith(1)
    expect(onNoiseGateEnabledChange).toHaveBeenCalledWith(false)
  })
})
