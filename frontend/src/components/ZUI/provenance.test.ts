import { describe, expect, it } from 'vitest'
import { PROVENANCE_META } from './provenance'
import { CHANGE_RING_COLORS } from './changeOverlay'
import { CHANGE_STATE_TAGS, PROVENANCE_TAGS, REPOSITORY_TAG_COLORS } from '../../utils/repositoryChangeScene'

describe('overlay tags', () => {
  it('covers every provenance kind in metadata', () => {
    for (const kind of PROVENANCE_TAGS) {
      expect(PROVENANCE_META[kind].label).toBeTruthy()
      expect(PROVENANCE_META[kind].color).toMatch(/^#/)
      expect(PROVENANCE_META[kind].glyph).toBeTruthy()
    }
  })

  it('seeds a color for every change-state and provenance tag', () => {
    for (const tag of [...CHANGE_STATE_TAGS, ...PROVENANCE_TAGS]) {
      expect(REPOSITORY_TAG_COLORS[tag]).toMatch(/^#/)
    }
  })

  it('matches change tag colors to the node ring colors', () => {
    for (const tag of CHANGE_STATE_TAGS) {
      expect(REPOSITORY_TAG_COLORS[tag]).toBe(CHANGE_RING_COLORS[tag])
    }
  })
})
