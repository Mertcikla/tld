import type { HandleSide } from './connectorHandles'

export const AUTO_LAYOUT_NODE_W = 180
export const AUTO_LAYOUT_NODE_H = 85
export const AUTO_LAYOUT_GAP = 70
export const AUTO_LAYOUT_GRID_X = AUTO_LAYOUT_NODE_W + AUTO_LAYOUT_GAP
export const AUTO_LAYOUT_GRID_Y = AUTO_LAYOUT_NODE_H + AUTO_LAYOUT_GAP
export const AUTO_LAYOUT_GRID: [number, number] = [AUTO_LAYOUT_GRID_X, AUTO_LAYOUT_GRID_Y]

const DEFAULT_MAX_STEPS = 12

export interface GridStep {
  x: number
  y: number
}

export function snapPositionToGrid(
  position: { x: number; y: number },
  grid: GridStep = { x: AUTO_LAYOUT_GRID_X, y: AUTO_LAYOUT_GRID_Y },
): { x: number; y: number } {
  return {
    x: Math.round(position.x / grid.x) * grid.x,
    y: Math.round(position.y / grid.y) * grid.y,
  }
}

export interface Rect {
  x: number
  y: number
  width: number
  height: number
}

export interface Neighbor {
  rect: Rect
  side: HandleSide
}

export interface AutoPlacementInput {
  neighbors?: Neighbor[]
  obstacles?: Rect[]
  fallback?: { x: number; y: number }
  width?: number
  height?: number
  gap?: number
  padding?: number
  maxSteps?: number
  grid?: GridStep
}

export function rectsOverlap(a: Rect, b: Rect, padding = 0): boolean {
  return (
    a.x < b.x + b.width + padding &&
    a.x + a.width + padding > b.x &&
    a.y < b.y + b.height + padding &&
    a.y + a.height + padding > b.y
  )
}

function centeredY(rect: Rect, height: number) {
  return rect.y + rect.height / 2 - height / 2
}

function centeredX(rect: Rect, width: number) {
  return rect.x + rect.width / 2 - width / 2
}

// idealPositionForSide returns the top-left position for a new node placed
// directly adjacent to `rect` on `side`, centered on the perpendicular axis.
export function idealPositionForSide(
  rect: Rect,
  side: HandleSide,
  width: number,
  height: number,
  gap: number,
): { x: number; y: number } {
  switch (side) {
    case 'right':
      return { x: rect.x + rect.width + gap, y: centeredY(rect, height) }
    case 'left':
      return { x: rect.x - width - gap, y: centeredY(rect, height) }
    case 'bottom':
      return { x: centeredX(rect, width), y: rect.y + rect.height + gap }
    case 'top':
      return { x: centeredX(rect, width), y: rect.y - height - gap }
  }
}

// sideFacingPoint picks the node side whose anchor faces `point`, used when no
// explicit source handle is known (e.g. multi-selection connects).
export function sideFacingPoint(rect: Rect, point: { x: number; y: number }): HandleSide {
  const dx = point.x - (rect.x + rect.width / 2)
  const dy = point.y - (rect.y + rect.height / 2)
  if (Math.abs(dx) >= Math.abs(dy)) return dx >= 0 ? 'right' : 'left'
  return dy >= 0 ? 'bottom' : 'top'
}

function fanCandidates(
  ideal: { x: number; y: number },
  side: HandleSide,
  width: number,
  height: number,
  gap: number,
  maxSteps: number,
) {
  const horizontal = side === 'left' || side === 'right'
  const perpStep = horizontal ? height + gap : width + gap
  const outStep = horizontal ? width + gap : height + gap
  const outSign = side === 'right' || side === 'bottom' ? 1 : -1

  const candidates: { x: number; y: number }[] = []
  for (let out = 0; out <= maxSteps; out++) {
    for (let fan = 0; fan <= maxSteps; fan++) {
      const fanOffsets = fan === 0 ? [0] : [fan, -fan]
      for (const fanOffset of fanOffsets) {
        candidates.push({
          x: ideal.x + (horizontal ? outSign * outStep * out : perpStep * fanOffset),
          y: ideal.y + (horizontal ? perpStep * fanOffset : outSign * outStep * out),
        })
      }
    }
  }
  return candidates
}

function gridCandidates(
  origin: { x: number; y: number },
  width: number,
  height: number,
  gap: number,
  maxSteps: number,
) {
  const stepX = width + gap
  const stepY = height + gap
  const candidates: { x: number; y: number }[] = [{ ...origin }]
  for (let ring = 1; ring <= maxSteps; ring++) {
    for (let dx = -ring; dx <= ring; dx++) {
      for (let dy = -ring; dy <= ring; dy++) {
        if (Math.max(Math.abs(dx), Math.abs(dy)) !== ring) continue
        candidates.push({ x: origin.x + dx * stepX, y: origin.y + dy * stepY })
      }
    }
  }
  return candidates
}

// findAutoPlacementPosition computes a non-overlapping top-left position for a
// newly added node. When connectors are known it anchors the node to the facing
// side of the neighbor(s), fanning out to avoid collisions; otherwise it finds
// the nearest free slot to `fallback`.
export function findAutoPlacementPosition(input: AutoPlacementInput): { x: number; y: number } {
  const width = input.width ?? AUTO_LAYOUT_NODE_W
  const height = input.height ?? AUTO_LAYOUT_NODE_H
  const gap = input.gap ?? AUTO_LAYOUT_GAP
  const padding = input.padding ?? 0
  const maxSteps = input.maxSteps ?? DEFAULT_MAX_STEPS
  const obstacles = input.obstacles ?? []
  const neighbors = input.neighbors ?? []
  const fallback = input.fallback ?? { x: 0, y: 0 }
  const grid = input.grid
  const snap = (position: { x: number; y: number }) => (grid ? snapPositionToGrid(position, grid) : position)

  const collides = (candidate: { x: number; y: number }) =>
    obstacles.some((obstacle) =>
      rectsOverlap({ x: candidate.x, y: candidate.y, width, height }, obstacle, padding),
    )

  let candidates: { x: number; y: number }[]
  if (neighbors.length > 0) {
    candidates = []
    for (const neighbor of neighbors) {
      const ideal = snap(idealPositionForSide(neighbor.rect, neighbor.side, width, height, gap))
      candidates.push(...fanCandidates(ideal, neighbor.side, width, height, gap, maxSteps))
    }
  } else {
    candidates = gridCandidates(snap(fallback), width, height, gap, maxSteps)
  }

  if (neighbors.length === 0) {
    return candidates.find((candidate) => !collides(candidate)) ?? snap(fallback)
  }

  let best: { x: number; y: number } | null = null
  let bestDistance = Infinity
  for (const candidate of candidates) {
    if (collides(candidate)) continue
    const distance = Math.hypot(candidate.x - fallback.x, candidate.y - fallback.y)
    if (distance < bestDistance) {
      bestDistance = distance
      best = candidate
    }
  }

  return best ?? candidates[0] ?? fallback
}

export function nodeRect(node: {
  position: { x: number; y: number }
  width?: number | null
  height?: number | null
}): Rect {
  return {
    x: node.position.x,
    y: node.position.y,
    width: node.width ?? AUTO_LAYOUT_NODE_W,
    height: node.height ?? AUTO_LAYOUT_NODE_H,
  }
}

export interface IncrementalEdge {
  source: number
  target: number
  side: HandleSide
}

export interface IncrementalLayoutOptions {
  center: { x: number; y: number }
  obstacles?: Rect[]
  width?: number
  height?: number
  gap?: number
  padding?: number
  grid?: GridStep
}

// planIncrementalLayout positions a freshly pasted/duplicated group relative to
// its own connectors, starting from the paste center. Root nodes (no incoming
// edge) are spread around the center, then every connected node is anchored to
// its already-placed neighbor(s), avoiding existing obstacles and each other.
export function planIncrementalLayout(
  ids: number[],
  edges: IncrementalEdge[],
  options: IncrementalLayoutOptions,
): Map<number, { x: number; y: number }> {
  const width = options.width ?? AUTO_LAYOUT_NODE_W
  const height = options.height ?? AUTO_LAYOUT_NODE_H
  const gap = options.gap ?? AUTO_LAYOUT_GAP
  const padding = options.padding ?? 0
  const obstacles = options.obstacles ?? []
  const center = options.center
  const grid = options.grid

  const idSet = new Set(ids)
  const relevantEdges = edges.filter((edge) => idSet.has(edge.source) && idSet.has(edge.target))

  const incoming = new Map<number, IncrementalEdge[]>()
  const outgoing = new Map<number, IncrementalEdge[]>()
  const indegree = new Map<number, number>()
  ids.forEach((id) => {
    incoming.set(id, [])
    outgoing.set(id, [])
    indegree.set(id, 0)
  })
  relevantEdges.forEach((edge) => {
    outgoing.get(edge.source)!.push(edge)
    incoming.get(edge.target)!.push(edge)
    indegree.set(edge.target, (indegree.get(edge.target) ?? 0) + 1)
  })

  const placed = new Map<number, { x: number; y: number }>()
  const remaining = new Set(ids)

  const currentObstacles = () => [
    ...obstacles,
    ...Array.from(placed.values()).map((position) => ({
      x: position.x,
      y: position.y,
      width,
      height,
    })),
  ]

  const neighborsFor = (id: number): Neighbor[] =>
    incoming.get(id)!
      .filter((edge) => placed.has(edge.source))
      .map((edge) => {
        const source = placed.get(edge.source)!
        return { rect: { x: source.x, y: source.y, width, height }, side: edge.side }
      })

  let rootIndex = 0
  const place = (id: number, neighbors: Neighbor[]) => {
    const fallback = {
      x: center.x - width / 2 + rootIndex * (width + gap),
      y: center.y - height / 2,
    }
    rootIndex += 1
    const position = findAutoPlacementPosition({
      neighbors,
      obstacles: currentObstacles(),
      fallback,
      width,
      height,
      gap,
      padding,
      grid,
    })
    placed.set(id, position)
    remaining.delete(id)
  }

  ids
    .filter((id) => (indegree.get(id) ?? 0) === 0)
    .forEach((id) => {
      if (remaining.has(id)) place(id, [])
    })

  let progressed = true
  while (remaining.size > 0 && progressed) {
    progressed = false
    for (const id of Array.from(remaining)) {
      const neighbors = neighborsFor(id)
      if (neighbors.length === 0) continue
      place(id, neighbors)
      progressed = true
    }
  }

  // Remaining nodes belong to cycles with no placed predecessor; place them last.
  for (const id of Array.from(remaining)) place(id, [])

  return placed
}

