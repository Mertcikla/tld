import { describe, expect, it } from 'vitest'
import { canonicalNodePairKey } from './pairKey'

describe('canonicalNodePairKey', () => {
  it('orders numeric node ids numerically', () => {
    expect(canonicalNodePairKey('36', '6')).toBe('6::36')
    expect(canonicalNodePairKey('6', '36')).toBe('6::36')
  })

  it('keeps lexicographic ordering for non-numeric ids', () => {
    expect(canonicalNodePairKey('ctx:2:25', '6')).toBe('6::ctx:2:25')
    expect(canonicalNodePairKey('ctx:b', 'ctx:a')).toBe('ctx:a::ctx:b')
  })
})