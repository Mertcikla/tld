import React from 'react'
import { act, create } from 'react-test-renderer'
import { describe, expect, it, vi } from 'vitest'
import TagUpsert from './TagUpsert'

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

const GROUP_TAG = 'group:12345678-1234-4234-a234-123456789012'

function hostNodesByTestId(renderer: ReturnType<typeof create>, testId: string) {
  return renderer.root.findAll((node) => typeof node.type === 'string' && node.props['data-testid'] === testId)
}

function type(renderer: ReturnType<typeof create>, value: string) {
  act(() => {
    renderer.root.findByProps({ 'data-testid': 'tag-upsert-input' }).props.onChange({ target: { value } })
  })
}

describe('TagUpsert group search', () => {
  it('searches groups by name after the group: prefix and adds the group tag', () => {
    const onAddTag = vi.fn()
    const renderer = create(
      <TagUpsert
        currentTags={[]}
        availableTags={['api', 'payments']}
        groups={[{ tag: GROUP_TAG, name: 'Payments', color: '#336699' }]}
        onAddTag={onAddTag}
      />,
    )

    type(renderer, 'group:pay')

    const groupOptions = hostNodesByTestId(renderer, 'tag-upsert-group-option')
    expect(groupOptions).toHaveLength(1)
    expect(hostNodesByTestId(renderer, 'tag-upsert-existing-option')).toHaveLength(0)
    expect(hostNodesByTestId(renderer, 'tag-upsert-create-option')).toHaveLength(0)

    act(() => groupOptions[0].props.onClick())
    expect(onAddTag).toHaveBeenCalledWith(GROUP_TAG)
  })

  it('lists all groups for the bare group: prefix', () => {
    const renderer = create(
      <TagUpsert
        currentTags={[]}
        availableTags={[]}
        groups={[
          { tag: GROUP_TAG, name: 'Payments' },
          { tag: 'group:aaaaaaaa-bbbb-cccc-dddd-eeeeeeeeeeee', name: 'Platform' },
        ]}
        onAddTag={vi.fn()}
      />,
    )

    type(renderer, 'group:')

    expect(hostNodesByTestId(renderer, 'tag-upsert-group-option')).toHaveLength(2)
  })

  it('excludes groups already on the element', () => {
    const renderer = create(
      <TagUpsert
        currentTags={[GROUP_TAG]}
        availableTags={[]}
        groups={[{ tag: GROUP_TAG, name: 'Payments' }]}
        onAddTag={vi.fn()}
      />,
    )

    type(renderer, 'group:')

    expect(hostNodesByTestId(renderer, 'tag-upsert-group-option')).toHaveLength(0)
  })

  it('does not offer a create option for the group: prefix', () => {
    const renderer = create(
      <TagUpsert currentTags={[]} availableTags={[]} groups={[]} onAddTag={vi.fn()} />,
    )

    type(renderer, 'group:new-group')

    expect(hostNodesByTestId(renderer, 'tag-upsert-create-option')).toHaveLength(0)
    expect(hostNodesByTestId(renderer, 'tag-upsert-group-option')).toHaveLength(0)
  })
})
