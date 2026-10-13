import { describe, expect, it } from 'vitest'
import { resetTagFilters, toggleHiddenTags, toggleSelectedTag } from './tagFilters'

describe('tag filter transitions', () => {
  it('adds multiple selections and removes the last selection', () => {
    const first = toggleSelectedTag([], 'go')
    const second = toggleSelectedTag(first.selectedTags, 'javascript')
    expect(second).toEqual({ selectedTags: ['go', 'javascript'], hiddenTags: [] })
    const remaining = toggleSelectedTag(second.selectedTags, 'go')
    expect(toggleSelectedTag(remaining.selectedTags, 'javascript')).toEqual(resetTagFilters())
  })

  it('switches globally between selection and tag/layer hiding', () => {
    expect(toggleSelectedTag(['go'], 'javascript').hiddenTags).toEqual([])
    const hidden = toggleHiddenTags([], ['go', 'javascript'])
    expect(hidden).toEqual({ selectedTags: [], hiddenTags: ['go', 'javascript'] })
    expect(toggleHiddenTags(hidden.hiddenTags, ['go', 'javascript'])).toEqual(resetTagFilters())
    expect(toggleHiddenTags(['go'], ['javascript']).hiddenTags).toEqual(['go', 'javascript'])
  })
})
