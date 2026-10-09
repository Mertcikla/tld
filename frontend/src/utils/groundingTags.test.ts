import { describe, expect, it } from 'vitest'
import { GROUNDING_IGNORE_TAG, isGroundingIgnoreTag } from './groundingTags'

describe('grounding ignore tag', () => {
  it('recognizes the reserved marker exactly', () => {
    expect(GROUNDING_IGNORE_TAG).toBe('$ignored')
    expect(isGroundingIgnoreTag('$ignored')).toBe(true)
    expect(isGroundingIgnoreTag('  $IGNORED  ')).toBe(true)
  })

  it('does not match ordinary or lookalike tags', () => {
    expect(isGroundingIgnoreTag('ignored')).toBe(false)
    expect(isGroundingIgnoreTag('$ignored-extra')).toBe(false)
    expect(isGroundingIgnoreTag('external')).toBe(false)
    expect(isGroundingIgnoreTag('group:12345678-1234-4234-a234-123456789012')).toBe(false)
  })
})
