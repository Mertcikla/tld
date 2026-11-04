import { linesForHandle } from './subElements'

export interface BundlePort {
  x: number
  y: number
  side: 'left' | 'right'
}

export interface BundleInput {
  key: string
  ports: BundlePort[]
  target?: boolean
}

export interface BundleFan {
  x: number
  y: number
  side: 'left' | 'right'
  branches: string[]
  stems: string[]
}

interface Point {
  x: number
  y: number
}

interface ChainPath {
  points: Point[]
  branch: boolean
}

interface BundleRoute {
  key: string
  x: number
  y: number
  railX: number
  railMinY: number
  railMaxY: number
  side: 'left' | 'right'
  ports: BundlePort[]
  paths: ChainPath[]
}

interface Segment {
  owner: string
  a: Point
  b: Point
  cornerA: boolean
  cornerB: boolean
}

interface Crossing extends Point {
  horizontal: string
  vertical: string
}

const FIRST_RAIL_OFFSET = 16
const LANE_SPACING = 24
const TRUNK_CLEARANCE = 16
const CORNER_RADIUS = 6
const BRIDGE_RADIUS = 4
const CREST_GAP_HALF = 1.5
const TRUNK_MIN_SEPARATION = 12
const PORT_MIN_SEPARATION = 8

/**
 * Flip a bundle handle's visual side toward its connected peer while preserving
 * all participating declaration lines and the underlying stored handle.
 */
export function closestBundleHandle(
  handle: string,
  centerX: number,
  otherCenterX: number,
  forceLineSide = false,
): string {
  const lineCount = linesForHandle(handle).length
  if ((lineCount < 2 && !(forceLineSide && lineCount === 1)) || centerX === otherCenterX) {
    return handle
  }
  return handle.replace(/^(left|right)-/, otherCenterX < centerX ? 'left-' : 'right-')
}

function pointKey(point: Point): string {
  return `${point.x}:${point.y}`
}

function deduplicateAndSortPorts(ports: readonly BundlePort[]): BundlePort[] {
  const unique = new Map<string, BundlePort>()
  for (const port of ports) {
    if (!Number.isFinite(port.x) || !Number.isFinite(port.y)) continue
    const key = `${port.side}:${port.x}:${port.y}`
    if (!unique.has(key)) unique.set(key, port)
  }
  return [...unique.values()].sort((a, b) => a.y - b.y || a.x - b.x)
}

function simplifyCollinearPoints(points: readonly Point[]): Point[] {
  const deduped = points.filter(
    (point, index) => index === 0 || point.x !== points[index - 1].x || point.y !== points[index - 1].y,
  )
  return deduped.filter((point, index, all) => {
    if (index === 0 || index === all.length - 1) return true
    const prev = all[index - 1]
    const next = all[index + 1]
    const sameX = prev.x === point.x && point.x === next.x
    const sameY = prev.y === point.y && point.y === next.y
    return !sameX && !sameY
  })
}

/**
 * Build the orthogonal fork graph and decompose it into non-overlapping chains
 * between degree != 2 vertices so shared rail sections and the trunk stub are
 * stroked once and dash animations stay in phase.
 */
function buildForkTree(ports: readonly BundlePort[], railX: number, root: Point, isTarget: boolean): ChainPath[] {
  const vertices = new Map<string, Point>()
  const adjacency = new Map<string, Set<string>>()

  const addEdge = (a: Point, b: Point) => {
    const ak = pointKey(a)
    const bk = pointKey(b)
    if (ak === bk) return
    vertices.set(ak, a)
    vertices.set(bk, b)
    let aLinks = adjacency.get(ak)
    if (!aLinks) {
      aLinks = new Set()
      adjacency.set(ak, aLinks)
    }
    let bLinks = adjacency.get(bk)
    if (!bLinks) {
      bLinks = new Set()
      adjacency.set(bk, bLinks)
    }
    aLinks.add(bk)
    bLinks.add(ak)
  }

  const railRows = [...new Set([...ports.map((port) => port.y), root.y])].sort((a, b) => a - b)
  for (let i = 1; i < railRows.length; i += 1) {
    addEdge({ x: railX, y: railRows[i - 1] }, { x: railX, y: railRows[i] })
  }
  for (const port of ports) {
    addEdge(port, { x: railX, y: port.y })
  }
  addEdge({ x: railX, y: root.y }, root)

  const rootKey = pointKey(root)
  const distanceFromRoot = new Map<string, number>([[rootKey, 0]])
  const queue = [rootKey]
  for (let i = 0; i < queue.length; i += 1) {
    const current = queue[i]
    const currentDist = distanceFromRoot.get(current)!
    for (const neighbor of adjacency.get(current) ?? []) {
      if (distanceFromRoot.has(neighbor)) continue
      distanceFromRoot.set(neighbor, currentDist + 1)
      queue.push(neighbor)
    }
  }

  const portKeys = new Set(ports.map(pointKey))
  const visitedEdges = new Set<string>()
  const edgeId = (a: string, b: string) => (a < b ? `${a}|${b}` : `${b}|${a}`)
  const paths: ChainPath[] = []

  for (const [start, neighbors] of adjacency) {
    if (neighbors.size === 2) continue
    for (const neighbor of neighbors) {
      const firstEdge = edgeId(start, neighbor)
      if (visitedEdges.has(firstEdge)) continue
      visitedEdges.add(firstEdge)

      const chain = [start, neighbor]
      while ((adjacency.get(chain[chain.length - 1])?.size ?? 0) === 2) {
        const tail = chain[chain.length - 1]
        const prev = chain[chain.length - 2]
        const next = [...adjacency.get(tail)!].find((candidate) => candidate !== prev)
        if (!next) break
        const nextEdge = edgeId(tail, next)
        if (visitedEdges.has(nextEdge)) break
        visitedEdges.add(nextEdge)
        chain.push(next)
      }

      const end = chain[chain.length - 1]
      const startDist = distanceFromRoot.get(start) ?? 0
      const endDist = distanceFromRoot.get(end) ?? 0
      // Source branches run from port -> trunk; target branches run from trunk -> port.
      if ((startDist < endDist) !== isTarget) {
        chain.reverse()
      }

      paths.push({
        points: simplifyCollinearPoints(chain.map((id) => vertices.get(id)!)),
        branch: portKeys.has(start) || portKeys.has(end),
      })
    }
  }

  return paths
}

function pickTrunkY(
  ports: readonly BundlePort[],
  usedTrunkYs: readonly number[],
  otherPortYs: readonly number[],
): number {
  const meanY = ports.reduce((sum, port) => sum + port.y, 0) / ports.length
  const minY = ports[0].y
  const maxY = ports[ports.length - 1].y

  const isCleanCandidate = (candidate: number, strictOtherPorts: boolean): boolean => {
    if (usedTrunkYs.some((used) => Math.abs(used - candidate) < TRUNK_MIN_SEPARATION)) {
      return false
    }
    if (strictOtherPorts && otherPortYs.some((portY) => Math.abs(portY - candidate) < PORT_MIN_SEPARATION)) {
      return false
    }
    // Avoid micro-jogs near the bundle's own ports where a rounded corner cannot fit.
    for (const port of ports) {
      const dist = Math.abs(port.y - candidate)
      if (dist > 0.01 && dist < PORT_MIN_SEPARATION) return false
    }
    return true
  }

  if (isCleanCandidate(meanY, true)) return meanY

  // Prefer candidates that stay inside [minY, maxY] before stepping outside.
  for (const strict of [true, false]) {
    for (let step = 1; step <= 24; step += 1) {
      const delta = step * TRUNK_MIN_SEPARATION
      for (const candidate of [meanY + delta, meanY - delta]) {
        if (candidate >= minY && candidate <= maxY && isCleanCandidate(candidate, strict)) {
          return candidate
        }
      }
    }
  }

  let fallback = meanY
  while (usedTrunkYs.some((used) => Math.abs(used - fallback) < TRUNK_MIN_SEPARATION)) {
    fallback += TRUNK_MIN_SEPARATION
  }
  return fallback
}

function collectRouteSegments(routes: Iterable<BundleRoute>): Segment[] {
  const segments: Segment[] = []
  for (const route of routes) {
    for (const { points } of route.paths) {
      for (let i = 1; i < points.length; i += 1) {
        segments.push({
          owner: route.key,
          a: points[i - 1],
          b: points[i],
          cornerA: i > 1,
          cornerB: i + 1 < points.length,
        })
      }
    }
  }
  return segments
}

function findCrossings(routes: Map<string, BundleRoute>): Crossing[] {
  const segments = collectRouteSegments(routes.values())
  const horizontal = segments.filter((s) => s.a.y === s.b.y)
  const vertical = segments.filter((s) => s.a.x === s.b.x)
  const crossings: Crossing[] = []

  for (const h of horizontal) {
    const hMinX = Math.min(h.a.x, h.b.x)
    const hMaxX = Math.max(h.a.x, h.b.x)
    const hLeftClear = (h.a.x < h.b.x ? h.cornerA : h.cornerB) ? CORNER_RADIUS + BRIDGE_RADIUS : BRIDGE_RADIUS + 2
    const hRightClear = (h.a.x < h.b.x ? h.cornerB : h.cornerA) ? CORNER_RADIUS + BRIDGE_RADIUS : BRIDGE_RADIUS + 2

    for (const v of vertical) {
      if (h.owner === v.owner) continue
      const x = v.a.x
      const y = h.a.y
      if (x <= hMinX + hLeftClear || x >= hMaxX - hRightClear) continue

      const vRoute = routes.get(v.owner)
      if (!vRoute) continue
      const vMinY = Math.min(v.a.y, v.b.y)
      const vMaxY = Math.max(v.a.y, v.b.y)
      if (y <= vMinY || y >= vMaxY) continue
      if (y <= vRoute.railMinY + CORNER_RADIUS + BRIDGE_RADIUS || y >= vRoute.railMaxY - CORNER_RADIUS - BRIDGE_RADIUS) {
        continue
      }

      // Skip if the crossing lands too close to any corner or junction on either bundle.
      const hitsJunction = segments.some((segment) => {
        if (segment.owner !== h.owner && segment.owner !== v.owner) return false
        const onSegment = segment.a.x === segment.b.x
          ? segment.a.x === x && y >= Math.min(segment.a.y, segment.b.y) && y <= Math.max(segment.a.y, segment.b.y)
          : segment.a.y === y && x >= Math.min(segment.a.x, segment.b.x) && x <= Math.max(segment.a.x, segment.b.x)
        if (!onSegment) return false
        const distA = Math.hypot(x - segment.a.x, y - segment.a.y)
        const distB = Math.hypot(x - segment.b.x, y - segment.b.y)
        const minClearA = segment.cornerA ? CORNER_RADIUS + BRIDGE_RADIUS : BRIDGE_RADIUS + 1
        const minClearB = segment.cornerB ? CORNER_RADIUS + BRIDGE_RADIUS : BRIDGE_RADIUS + 1
        return distA <= minClearA || distB <= minClearB
      })

      if (!hitsJunction) {
        crossings.push({ x, y, horizontal: h.owner, vertical: v.owner })
      }
    }
  }

  return crossings
}

/**
 * Emit an SVG path (`M` / `L` / `Q`) for a chain of orthogonal points with
 * rounded corners and crossing bridges/gaps.
 */
function buildBridgedPath(owner: string, points: readonly Point[], crossings: readonly Crossing[]): string {
  if (points.length === 0) return ''
  const commands = [`M ${points[0].x},${points[0].y}`]
  let prevCornerRadius = 0

  for (let i = 1; i < points.length; i += 1) {
    const a = points[i - 1]
    const b = points[i]
    const next = points[i + 1]
    const segLen = Math.hypot(b.x - a.x, b.y - a.y)
    const dx = Math.sign(b.x - a.x)
    const dy = Math.sign(b.y - a.y)
    const nextLen = next ? Math.hypot(next.x - b.x, next.y - b.y) : 0
    const radius = next ? Math.min(CORNER_RADIUS, segLen / 2, nextLen / 2) : 0

    const start = {
      x: a.x + dx * prevCornerRadius,
      y: a.y + dy * prevCornerRadius,
    }
    const end = {
      x: b.x - dx * radius,
      y: b.y - dy * radius,
    }

    if (dx !== 0) {
      const minX = Math.min(start.x, end.x)
      const maxX = Math.max(start.x, end.x)
      const hits = crossings
        .filter((c) => c.horizontal === owner && c.y === a.y && c.x - BRIDGE_RADIUS >= minX && c.x + BRIDGE_RADIUS <= maxX)
        .sort((left, right) => (left.x - right.x) * dx)
      const uniqueHits = [...new Map(hits.map((hit) => [`${hit.x}:${hit.y}`, hit])).values()]

      for (const hit of uniqueHits) {
        commands.push(
          `L ${hit.x - dx * BRIDGE_RADIUS},${hit.y}`,
          `Q ${hit.x - dx * BRIDGE_RADIUS},${hit.y - BRIDGE_RADIUS} ${hit.x},${hit.y - BRIDGE_RADIUS}`,
          `Q ${hit.x + dx * BRIDGE_RADIUS},${hit.y - BRIDGE_RADIUS} ${hit.x + dx * BRIDGE_RADIUS},${hit.y}`,
        )
      }
    } else if (dy !== 0) {
      const minY = Math.min(start.y, end.y)
      const maxY = Math.max(start.y, end.y)
      // The bridge crest crosses the vertical rail at y = hit.y - BRIDGE_RADIUS.
      const hits = crossings
        .filter((c) => {
          if (c.vertical !== owner || c.x !== a.x) return false
          const crestY = c.y - BRIDGE_RADIUS
          return crestY - CREST_GAP_HALF >= minY && crestY + CREST_GAP_HALF <= maxY
        })
        .sort((left, right) => (left.y - right.y) * dy)
      const uniqueHits = [...new Map(hits.map((hit) => [`${hit.x}:${hit.y}`, hit])).values()]

      let currentY = start.y
      for (let hitIndex = 0; hitIndex < uniqueHits.length; hitIndex += 1) {
        const hit = uniqueHits[hitIndex]
        let farY = hit.y
        while (
          hitIndex + 1 < uniqueHits.length &&
          Math.abs(uniqueHits[hitIndex + 1].y - farY) <= CREST_GAP_HALF * 2
        ) {
          hitIndex += 1
          farY = uniqueHits[hitIndex].y
        }
        const beforeY = hit.y - BRIDGE_RADIUS - dy * CREST_GAP_HALF
        const afterY = farY - BRIDGE_RADIUS + dy * CREST_GAP_HALF
        if ((beforeY - currentY) * dy > 0 && (end.y - afterY) * dy >= 0) {
          commands.push(`L ${hit.x},${beforeY}`, `M ${hit.x},${afterY}`)
          currentY = afterY
        }
      }
    }

    commands.push(`L ${end.x},${end.y}`)
    if (next && radius > 0) {
      const nextDx = Math.sign(next.x - b.x)
      const nextDy = Math.sign(next.y - b.y)
      commands.push(`Q ${b.x},${b.y} ${b.x + nextDx * radius},${b.y + nextDy * radius}`)
    }
    prevCornerRadius = radius
  }

  return commands.join(' ')
}

/**
 * Compute separated, crossing-aware fork geometries for all bundles on a card.
 */
export function bundleFans(inputs: readonly BundleInput[]): Map<string, BundleFan> {
  const drafts = inputs
    .map((input) => ({
      ...input,
      ports: deduplicateAndSortPorts(input.ports),
    }))
    .filter((input) => input.ports.length > 0)
    .sort((a, b) => (
      a.ports[0].y - b.ports[0].y ||
      a.ports[a.ports.length - 1].y - b.ports[b.ports.length - 1].y ||
      a.ports.length - b.ports.length ||
      a.key.localeCompare(b.key, undefined, { numeric: true })
    ))

  const routes = new Map<string, BundleRoute>()

  for (const side of ['left', 'right'] as const) {
    const sideDrafts = drafts.filter((input) => input.ports[0].side === side)
    const multiPortGroup = sideDrafts.filter((input) => input.ports.length > 1)
    const usedTrunkYs: number[] = []
    const dir = side === 'left' ? -1 : 1

    for (const [lane, input] of multiPortGroup.entries()) {
      const baseX = input.ports[0].x
      const railX = baseX + dir * (FIRST_RAIL_OFFSET + lane * LANE_SPACING)
      const x = baseX + dir * (FIRST_RAIL_OFFSET + TRUNK_CLEARANCE + (multiPortGroup.length - 1) * LANE_SPACING)
      const otherPortYs = sideDrafts
        .filter((other) => other.key !== input.key)
        .flatMap((other) => other.ports.map((port) => port.y))
      const y = pickTrunkY(input.ports, usedTrunkYs, otherPortYs)
      usedTrunkYs.push(y)

      const railMinY = Math.min(input.ports[0].y, y)
      const railMaxY = Math.max(input.ports[input.ports.length - 1].y, y)
      const paths = buildForkTree(input.ports, railX, { x, y }, !!input.target)
      routes.set(input.key, {
        key: input.key,
        x,
        y,
        railX,
        railMinY,
        railMaxY,
        side,
        ports: input.ports,
        paths,
      })
    }
  }

  const crossings = findCrossings(routes)

  return new Map(
    drafts.map((input) => {
      const route = routes.get(input.key)
      if (!route) {
        const single = input.ports[0]
        return [
          input.key,
          {
            x: single.x,
            y: single.y,
            side: single.side,
            branches: [],
            stems: [],
          },
        ]
      }
      return [
        input.key,
        {
          x: route.x,
          y: route.y,
          side: route.side,
          branches: route.paths
            .filter((path) => path.branch)
            .map((path) => buildBridgedPath(input.key, path.points, crossings)),
          stems: route.paths
            .filter((path) => !path.branch)
            .map((path) => buildBridgedPath(input.key, path.points, crossings)),
        },
      ]
    }),
  )
}

export function bundleFan(ports: readonly BundlePort[], target = false): BundleFan | null {
  return bundleFans([{ key: 'single', ports: [...ports], target }]).get('single') ?? null
}

/**
 * Sample M/L/Q bundle path strings into polylines for spatial indexing and hit-testing,
 * keeping gaps (`M`) as separate polylines.
 */
export function bundlePathPolylines(path: string): Point[][] {
  const tokens = path.match(/[MLQ]|-?\d+(?:\.\d+)?(?:e[+-]?\d+)?/gi) ?? []
  const routes: Point[][] = []
  let current: Point[] = []

  for (let i = 0; i < tokens.length;) {
    const command = tokens[i++].toUpperCase()
    const nextPoint = (): Point => ({ x: Number(tokens[i++]), y: Number(tokens[i++]) })

    if (command === 'M') {
      current = [nextPoint()]
      routes.push(current)
    } else if (command === 'L') {
      current.push(nextPoint())
    } else if (command === 'Q') {
      const start = current[current.length - 1]
      const control = nextPoint()
      const end = nextPoint()
      if (!start) {
        current = [end]
        routes.push(current)
        continue
      }
      for (let step = 1; step <= 8; step += 1) {
        const t = step / 8
        const u = 1 - t
        current.push({
          x: u * u * start.x + 2 * u * t * control.x + t * t * end.x,
          y: u * u * start.y + 2 * u * t * control.y + t * t * end.y,
        })
      }
    }
  }

  return routes
}
