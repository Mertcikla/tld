import { describe, expect, it } from 'vitest'
import { resolveElementIconUrl } from './elementIcon'

describe('resolveElementIconUrl', () => {
  it('uses an explicit logo url before derived technology icons', () => {
    expect(resolveElementIconUrl('/custom.svg', [
      { type: 'catalog', slug: 'go', label: 'Go', is_primary_icon: true },
    ])).toBe('/custom.svg')
  })

  it('normalizes explicit png catalog icon urls with alias remapping', () => {
    expect(resolveElementIconUrl('/icons/golang.png', [])).toBe('/icons/go.svg')
  })

  it('preserves explicit custom png icon urls', () => {
    expect(resolveElementIconUrl('/icons/acme-platform.png', [])).toBe('/icons/acme-platform.png')
  })

  it('derives the selected catalog technology icon when logo_url is missing', () => {
    expect(resolveElementIconUrl(null, [
      { type: 'catalog', slug: 'go', label: 'Go', is_primary_icon: true },
    ])).toBe('/icons/go.svg')
  })

  it('derives the selected Font Awesome custom technology icon when logo_url is missing', () => {
    const iconUrl = resolveElementIconUrl(null, [
      { type: 'custom', label: 'fa:fa-car', is_primary_icon: true },
    ])

    expect(iconUrl).toMatch(/^data:image\/svg\+xml;charset=utf-8,/)
    expect(decodeURIComponent(iconUrl?.split(',')[1] ?? '')).toContain('viewBox="0 0 512 512"')
  })

  it('prefers a selected Font Awesome custom icon over an unselected catalog icon', () => {
    expect(resolveElementIconUrl(null, [
      { type: 'catalog', slug: 'go', label: 'Go' },
      { type: 'custom', label: 'fa:fa-car', is_primary_icon: true },
    ])).toMatch(/^data:image\/svg\+xml;charset=utf-8,/)
  })

  it('falls back to the first catalog link when the API omits primary icon metadata', () => {
    expect(resolveElementIconUrl(null, [
      { type: 'catalog', slug: 'javascript', label: 'JavaScript' },
    ])).toBe('/icons/javascript.svg')
  })

  it('preserves explicit no-icon clears instead of falling back to technology', () => {
    expect(resolveElementIconUrl('', [
      { type: 'catalog', slug: 'go', label: 'Go', is_primary_icon: true },
    ])).toBeNull()
  })

  it('does not infer icons from custom technology labels when logo_url is missing', () => {
    expect(resolveElementIconUrl(null, [
      { type: 'custom', label: 'kafka' },
    ])).toBeNull()
  })
})
