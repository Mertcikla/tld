import { describe, expect, it } from 'vitest'
import { resolveElementIconUrl } from './elementIcon'

describe('resolveElementIconUrl', () => {
  it('uses an explicit logo url before derived technology icons', () => {
    expect(resolveElementIconUrl('/custom.svg', [
      { type: 'catalog', slug: 'golang', label: 'Go', is_primary_icon: true },
    ])).toBe('/custom.svg')
  })

  it('derives the selected catalog technology icon when logo_url is missing', () => {
    expect(resolveElementIconUrl(null, [
      { type: 'catalog', slug: 'golang', label: 'Go', is_primary_icon: true },
    ])).toBe('/icons/golang.png')
  })

  it('preserves explicit no-icon clears instead of falling back to technology', () => {
    expect(resolveElementIconUrl('', [
      { type: 'catalog', slug: 'golang', label: 'Go', is_primary_icon: true },
    ])).toBeNull()
  })
})
