/** Whether a canvas edge is new, gone, or changed in the compared revisions. */
export type ZUIEdgeChange = 'added' | 'removed' | 'modified'

export const EDGE_CHANGE_META: Record<ZUIEdgeChange, { label: string; color: string; dash: number[] }> = {
  added: { label: 'Added in this change', color: '#48bb78', dash: [] },
  removed: { label: 'Removed in this change', color: '#fc8181', dash: [6, 4] },
  modified: { label: 'Changed in this change', color: '#ecc94b', dash: [] },
}

/**
 * Reads edge change state from connector tags. The scene tags connectors
 * spanning a new, removed, or modified file relationship as
 * `change:added|removed|modified`; anything else carries no claim.
 */
export function connectorChangeFromTags(tags?: string[]): ZUIEdgeChange | undefined {
  if (!tags) return undefined
  if (tags.includes('change:added')) return 'added'
  if (tags.includes('change:removed')) return 'removed'
  if (tags.includes('change:modified')) return 'modified'
  return undefined
}
