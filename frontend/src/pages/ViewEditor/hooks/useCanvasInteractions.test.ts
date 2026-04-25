import { describe, expect, it } from 'vitest'
import type { Connector } from '../../../types'
import { getConnectorDeletionTarget } from './useCanvasInteractions'

const connector = (id: number): Connector => ({
  id,
  view_id: 1,
  source_element_id: 10,
  target_element_id: 20,
  label: null,
  description: null,
  relationship: null,
  direction: 'forward',
  style: 'bezier',
  url: null,
  source_handle: 'right',
  target_handle: 'left',
  created_at: '2024-01-01',
  updated_at: '2024-01-01',
})

describe('getConnectorDeletionTarget', () => {
  it('returns the selected connector id', () => {
    expect(getConnectorDeletionTarget(connector(7))).toBe(7)
  })

  it('returns null when nothing is selected', () => {
    expect(getConnectorDeletionTarget(null)).toBeNull()
  })
})
