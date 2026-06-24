import { describe, expect, it } from 'vitest'
import {
  fontAwesomeIconUrlForTechnologyLabel,
  matchFontAwesomeTechnologyIconQuery,
  parseFontAwesomeTechnologyIconName,
} from './fontAwesomeIcon'

function decodeIconUrl(iconUrl: string | null): string {
  expect(iconUrl).toMatch(/^data:image\/svg\+xml;charset=utf-8,/)
  return decodeURIComponent(iconUrl?.split(',')[1] ?? '')
}

describe('fontAwesomeIconUrlForTechnologyLabel', () => {
  it('pads generated SVGs so tall glyphs like fa:person are not clipped by the viewBox', () => {
    const svg = decodeIconUrl(fontAwesomeIconUrlForTechnologyLabel('fa:fa-person'))

    expect(svg).toContain('width="384" height="512" viewBox="-30.72 -40.96 445.44 593.92"')
  })
})

describe('parseFontAwesomeTechnologyIconName', () => {
  it('keeps stored technology labels strict', () => {
    expect(parseFontAwesomeTechnologyIconName('car')).toBeNull()
    expect(parseFontAwesomeTechnologyIconName('fa:fa-car')).toBe('car')
  })
})

describe('matchFontAwesomeTechnologyIconQuery', () => {
  it('matches exact Font Awesome icon names without requiring a prefix', () => {
    const match = matchFontAwesomeTechnologyIconQuery('car')

    expect(match?.iconName).toBe('car')
    expect(match?.label).toBe('fa:car')
    expect(match?.iconUrl).toMatch(/^data:image\/svg\+xml;charset=utf-8,/)
  })

  it('matches class-like and space-separated icon queries', () => {
    expect(matchFontAwesomeTechnologyIconQuery('fa-solid fa-car')?.label).toBe('fa:car')
    expect(matchFontAwesomeTechnologyIconQuery('person walk')?.label).toBe('fa:person-walking')
  })

  it('does not treat very short partial queries as Font Awesome matches', () => {
    expect(matchFontAwesomeTechnologyIconQuery('go')).toBeNull()
  })
})
