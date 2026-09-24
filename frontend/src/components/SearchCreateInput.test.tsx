import React from 'react'
import { act, create } from 'react-test-renderer'
import { describe, expect, it, vi } from 'vitest'
import SearchCreateInput from './SearchCreateInput'

vi.mock('@chakra-ui/react', async () => {
  const ReactModule = await import('react')
  const BoxLike = ({ children, ...props }: { children?: React.ReactNode }) => ReactModule.createElement('div', props, children)
  const InputLike = (props: Record<string, unknown>) => ReactModule.createElement('input', props)
  return {
    Box: BoxLike,
    HStack: BoxLike,
    Input: InputLike,
    Text: BoxLike,
    VStack: BoxLike,
  }
})

function hostNodesByTestId(renderer: ReturnType<typeof create>, testId: string) {
  return renderer.root.findAll((node) => typeof node.type === 'string' && node.props['data-testid'] === testId)
}

function renderFocused(props: Partial<React.ComponentProps<typeof SearchCreateInput>> = {}) {
  const renderer = create(
    <SearchCreateInput value="" onChange={vi.fn()} options={[]} {...props} />,
  )
  act(() => {
    renderer.root.findByProps({ 'data-testid': 'search-create-input' }).props.onFocus()
  })
  return renderer
}

describe('SearchCreateInput', () => {
  it('shows the create option first and existing matches below', () => {
    const renderer = renderFocused({ value: 'pay', options: ['payments', 'payroll'] })

    expect(hostNodesByTestId(renderer, 'search-create-create-option')).toHaveLength(1)
    expect(hostNodesByTestId(renderer, 'search-create-existing-option')).toHaveLength(2)
  })

  it('hides the create option when the query exactly matches an existing option', () => {
    const renderer = renderFocused({ value: 'payments', options: ['payments'] })

    expect(hostNodesByTestId(renderer, 'search-create-create-option')).toHaveLength(0)
    expect(hostNodesByTestId(renderer, 'search-create-existing-option')).toHaveLength(1)
  })

  it('selects an existing option on click', () => {
    const onChange = vi.fn()
    const renderer = renderFocused({ value: 'pay', onChange, options: ['payments'] })

    act(() => {
      hostNodesByTestId(renderer, 'search-create-existing-option')[0].props.onMouseDown({ preventDefault: vi.fn() })
    })

    expect(onChange).toHaveBeenCalledWith('payments')
  })

  it('submits the typed value on Enter when not selecting', () => {
    const onSubmit = vi.fn()
    const renderer = renderFocused({ value: 'new group', onSubmit, options: ['payments'] })

    act(() => {
      renderer.root
        .findByProps({ 'data-testid': 'search-create-input' })
        .props.onKeyDown({ key: 'Enter', preventDefault: vi.fn() })
    })

    expect(onSubmit).toHaveBeenCalledWith('new group')
  })

  it('submits the selected option on click when submitOnSelect is set', () => {
    const onChange = vi.fn()
    const onSubmit = vi.fn()
    const renderer = renderFocused({ value: 'pay', onChange, onSubmit, options: ['payments'], submitOnSelect: true })

    act(() => {
      hostNodesByTestId(renderer, 'search-create-existing-option')[0].props.onMouseDown({ preventDefault: vi.fn() })
    })

    expect(onChange).toHaveBeenCalledWith('payments')
    expect(onSubmit).toHaveBeenCalledWith('payments')
  })
})
