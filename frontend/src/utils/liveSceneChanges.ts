import type { RepositoryImpactScene } from '../api/client'

/** Follow fresh edits, including reversions, rather than every cumulative
 * change since the base commit. Hashes distinguish equal-sized body edits. */
export function changedScenePlacements(previous: RepositoryImpactScene | null, next: RepositoryImpactScene): Array<{ viewId: number; elementId: number }> {
  const signatures = (scene: RepositoryImpactScene | null) => new Map((scene?.files ?? []).map((file) => [file.path, JSON.stringify([file.baseHash, file.headHash, file.change, file.linesAdded, file.linesRemoved, file.metadataOnly, file.binary])]))
  const before = signatures(previous), after = signatures(next)
  const changed = new Set([...before.keys(), ...after.keys()].filter((path) => before.get(path) !== after.get(path)))
  // Older artifacts may have coverage but no file manifest.
  if (!next.files && !previous?.files) for (const overlay of Object.values(next.overlays)) if (overlay.change !== 'unchanged') changed.add(overlay.path)
  const refs = new Map<string, { viewId: number; elementId: number }>()
  for (const scene of [next, previous]) {
    for (const entry of scene?.coverage ?? []) {
      if (!changed.has(entry.path)) continue
      for (const key of entry.placementKeys) {
        const placement = scene?.placements?.[key]
        if (placement) refs.set(key, { viewId: placement.viewId, elementId: placement.resourceId })
      }
    }
  }
  return [...refs.values()]
}

