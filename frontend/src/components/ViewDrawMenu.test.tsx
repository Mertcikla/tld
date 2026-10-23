import React from 'react'
import { act, create } from 'react-test-renderer'
import { describe, expect, it, vi } from 'vitest'
import ViewDrawMenu from './ViewDrawMenu'

vi.mock('@chakra-ui/react', () => ({
  Box: ({ as = 'div', children, ...props }: { as?: string; children?: React.ReactNode }) => React.createElement(as, props, children),
}))

describe('drawing toolbar controls', () => {
  it('exposes the chosen tool, color, and width and natively disables unavailable history', () => {
    const setDrawingTool = vi.fn()
    const onUndo = vi.fn()
    const renderer = create(<ViewDrawMenu
      drawingMode drawingTool="pencil" setDrawingTool={setDrawingTool}
      drawingColor="#ffffff" setDrawingColor={vi.fn()}
      drawingWidth={3} setDrawingWidth={vi.fn()}
      canUndo={false} canRedo onUndo={onUndo} onRedo={vi.fn()} setDrawingMode={vi.fn()}
    />)
    const buttons = renderer.root.findAllByType('button')
    const pen = buttons.find((button) => button.props.title === 'Pen (P)')!
    const eraser = buttons.find((button) => button.props.title === 'Eraser (E)')!
    const undo = buttons.find((button) => button.props.title === 'Undo (Cmd+Z)')!
    expect(pen.props['aria-pressed']).toBe(true)
    expect(eraser.props['aria-pressed']).toBe(false)
    expect(undo.props.disabled).toBe(true)
    expect(undo.props.onClick).toBeUndefined()
    expect(buttons.find((button) => button.props['data-width'] === 3)!.props['aria-pressed']).toBe(true)
    act(() => eraser.props.onClick())
    expect(setDrawingTool).toHaveBeenCalledWith('eraser')
    expect(onUndo).not.toHaveBeenCalled()
    act(() => renderer.unmount())
  })
})
