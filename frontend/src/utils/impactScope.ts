import type { RepositoryImpact } from '../api/client'

// Maximum blast radius the UI exposes; matches the context depth the backend
// computes when a comparison is requested.
export const MAX_BLAST_RADIUS = 3

// Node budget above which the displayed blast radius is narrowed with a
// warning. Mirrors internal/codeindex/impact.DefaultMaxNodes.
export const DEFAULT_MAX_NODES = 400

// fitBlastRadius picks the widest radius at or below maxRadius whose node count
// fits maxNodes. Direct changes cannot be narrowed away, so a diagram whose
// direct changes alone exceed the budget returns 0. Mirrors
// internal/codeindex/impact.FitRadius.
export function fitBlastRadius(diagram: RepositoryImpact, maxRadius: number, maxNodes = DEFAULT_MAX_NODES): number {
  const widest = Math.max(0, Math.min(maxRadius, Math.max(diagram.maxRadius, 0)))
  if (maxNodes <= 0) return widest
  const count = (radius: number) => diagram.nodes.filter((node) => node.distance <= radius).length
  if (count(widest) <= maxNodes) return widest
  for (let radius = widest - 1; radius > 0; radius--) {
    if (count(radius) <= maxNodes) return radius
  }
  return 0
}
