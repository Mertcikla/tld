import React from 'react'
import { act, create } from 'react-test-renderer'
import { afterEach, describe, expect, it } from 'vitest'
import {
  DEFAULT_TAG_COLOR,
  beginTagDrag,
  endTagDrag,
  getTagDragPosition,
  layerDragMeta,
  suppressNativeDragImage,
  tagDragMeta,
  useTagDragGhost,
} from './tagDragGhostState'

const GROUP_TAG = 'group:12345678-1234-4234-a234-123456789012'

function Probe() {
  const meta = useTagDragGhost()
  return React.createElement('div', { 'data-kind': meta?.kind ?? 'none' })
}

describe('tagDragGhostState', () => {
  afterEach(() => endTagDrag())

  it('maps regular tags, preferring the display label', () => {
    expect(tagDragMeta('api', '#123456')).toEqual({ kind: 'tag', name: 'api', color: '#123456' })
    expect(tagDragMeta('api', '#123456', 'API tag')).toEqual({ kind: 'tag', name: 'API tag', color: '#123456' })
  })

  it('maps group tags and strips the group: prefix for display', () => {
    expect(tagDragMeta(GROUP_TAG, '#abcdef', 'group:Payments')).toEqual({
      kind: 'group',
      name: 'Payments',
      color: '#abcdef',
    })
  })

  it('maps layers with a pluralized tag-count detail and default color', () => {
    expect(layerDragMeta({ name: 'Backend', color: '#00ff00', tags: ['a', 'b'] })).toEqual({
      kind: 'layer',
      name: 'Backend',
      color: '#00ff00',
      detail: '2 tags',
    })
    expect(layerDragMeta({ name: 'Solo', color: undefined, tags: ['a'] })).toEqual({
      kind: 'layer',
      name: 'Solo',
      color: DEFAULT_TAG_COLOR,
      detail: '1 tag',
    })
  })

  it('tracks the active ghost and pointer position through begin/end', () => {
    let renderer!: ReturnType<typeof create>
    act(() => {
      renderer = create(React.createElement(Probe))
    })
    expect(renderer.root.findByType('div').props['data-kind']).toBe('none')

    act(() => {
      beginTagDrag({ kind: 'group', name: 'Payments', color: '#abcdef' }, { x: 12, y: 34 })
    })
    expect(renderer.root.findByType('div').props['data-kind']).toBe('group')
    expect(getTagDragPosition()).toEqual({ x: 12, y: 34 })

    act(() => {
      endTagDrag()
    })
    expect(renderer.root.findByType('div').props['data-kind']).toBe('none')
  })

  it('suppresses the native drag image safely outside the DOM', () => {
    expect(() => suppressNativeDragImage({ dataTransfer: {} } as never)).not.toThrow()
  })
})
