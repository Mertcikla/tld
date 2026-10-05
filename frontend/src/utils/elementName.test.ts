import { describe, expect, it } from 'vitest'
import {
  ELEMENT_NAME_LINE_HEIGHT,
  fitElementName,
  resetElementNameFitCache,
  splitNameSegments,
  type NameFitOptions,
} from './elementName'

const measure = (text: string, fontSize: number) => text.length * fontSize * 0.6

function fit(name: string, overrides: Partial<NameFitOptions> = {}) {
  resetElementNameFitCache()
  return fitElementName({
    name,
    maxWidth: 160,
    maxLines: 2,
    fontSizes: [18, 16, 14],
    measure,
    measureKey: 'test',
    ...overrides,
  })
}

describe('splitNameSegments', () => {
  it('keeps a single token intact', () => {
    expect(splitNameSegments('Payments')).toEqual(['Payments'])
  })

  it('breaks camelCase and PascalCase boundaries', () => {
    expect(splitNameSegments('paymentProcessingService')).toEqual([
      'payment',
      'Processing',
      'Service',
    ])
    expect(splitNameSegments('PaymentProcessingService')).toEqual([
      'Payment',
      'Processing',
      'Service',
    ])
  })

  it('keeps acronym runs together', () => {
    expect(splitNameSegments('HTTPServer')).toEqual(['HTTP', 'Server'])
  })

  it('breaks at path, dot, snake and kebab separators', () => {
    expect(splitNameSegments('internal/codeindex/community')).toEqual([
      'internal/',
      'codeindex/',
      'community',
    ])
    expect(splitNameSegments('pkg.Type.Method')).toEqual(['pkg.', 'Type.', 'Method'])
    expect(splitNameSegments('snake_case_name')).toEqual(['snake_', 'case_', 'name'])
    expect(splitNameSegments('kebab-case-name')).toEqual(['kebab-', 'case-', 'name'])
  })

  it('keeps spaces attached so rejoined lines preserve them', () => {
    expect(splitNameSegments('Payment Processing Service')).toEqual([
      'Payment ',
      'Processing ',
      'Service',
    ])
  })
})

describe('fitElementName', () => {
  it('keeps short names on one line at the largest size', () => {
    const result = fit('Payments')
    expect(result.lines).toEqual(['Payments'])
    expect(result.fontSize).toBe(18)
    expect(result.truncated).toBe(false)
  })

  it('wraps camelCase names into balanced lines without shrinking', () => {
    const result = fit('PaymentProcessingService', { maxWidth: 220 })
    expect(result.fontSize).toBe(18)
    expect(result.lines).toHaveLength(2)
    expect(result.lines.join('')).toBe('PaymentProcessingService')
    expect(result.truncated).toBe(false)
  })

  it('wraps path names at separators', () => {
    const result = fit('internal/codeindex/community')
    expect(result.lines).toHaveLength(2)
    expect(result.lines[0].endsWith('/')).toBe(true)
    expect(result.truncated).toBe(false)
  })

  it('preserves spaces when wrapping multi-word names', () => {
    const result = fit('Payment Processing Service', { maxWidth: 200 })
    expect(result.lines).toHaveLength(2)
    expect(result.lines.join(' ').replace(/\s+/g, ' ')).toBe('Payment Processing Service')
  })

  it('shrinks in quantized steps before truncating', () => {
    const result = fit('PaymentProcessingService', { maxWidth: 150 })
    expect(result.truncated).toBe(false)
    expect([18, 16, 14]).toContain(result.fontSize)
    expect(result.fontSize).toBeLessThan(18)
  })

  it('middle-elides a single unbreakable token that cannot fit', () => {
    const result = fit('ExtraordinarilyLongUnbrokenIdentifier', { maxWidth: 60 })
    expect(result.truncated).toBe(true)
    expect(result.lines).toHaveLength(1)
    expect(result.lines[0]).toContain('\u2026')
    expect(measure(result.lines[0], result.fontSize)).toBeLessThanOrEqual(60)
  })

  it('keeps head and tail segments for path names that cannot fit', () => {
    const result = fit('internal/codeindex/materialize/community/builder', {
      maxWidth: 220,
      fontSizes: [18],
    })
    expect(result.truncated).toBe(true)
    expect(result.lines[0]).toBe('internal/\u2026builder')
  })

  it('respects the vertical band by shrinking or wrapping', () => {
    const bandHeight = 2 * 16 * ELEMENT_NAME_LINE_HEIGHT
    const result = fit('PaymentProcessingService', { bandHeight })
    expect(result.truncated).toBe(false)
    expect(result.lines).toHaveLength(2)
    expect(result.fontSize).toBeLessThanOrEqual(16)
    expect(result.lines.length * result.fontSize * ELEMENT_NAME_LINE_HEIGHT).toBeLessThanOrEqual(
      bandHeight,
    )
  })

  it('falls back to a single line when only one line is allowed', () => {
    const result = fit('PaymentProcessingService', { maxLines: 1, maxWidth: 80 })
    expect(result.lines).toHaveLength(1)
    expect(result.truncated).toBe(true)
    expect(result.fontSize).toBe(14)
  })

  it('caches identical requests', () => {
    resetElementNameFitCache()
    const options: NameFitOptions = {
      name: 'PaymentProcessingService',
      maxWidth: 160,
      maxLines: 2,
      fontSizes: [18, 16, 14],
      measure,
      measureKey: 'test',
    }
    const first = fitElementName(options)
    const second = fitElementName(options)
    expect(second).toBe(first)
  })
})
