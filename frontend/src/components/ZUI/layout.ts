// src/components/ZUI/layout.ts

import type {
  DiagramGroupLayout,
  LayoutNode,
  ZUILayout,
} from './types'
import type {
  PlacedElement,
  ExploreData,
  ViewConnector,
} from '../../types'
import { resolveIconPath } from '../../utils/url'

// ── Constants ──────────────────────────────────────────────────────

export const NODE_W = 200
export const NODE_H = 100
const GROUP_PAD = 80         // padding inside a diagram group box
const GROUP_SPACING = 400    // horizontal gap between root diagrams
const CHILD_PAD = 1         // padding inside a node when rendering children

// ── Helpers ────────────────────────────────────────────────────────

function nodeId(diagramId: number, elementId: number): string {
  return `d${diagramId}-o${elementId}`
}

function getPos(obj: PlacedElement, axis: 'x' | 'y'): number {
  const val = axis === 'x' ? obj.position_x : obj.position_y
  return typeof val === 'number' && isFinite(val) ? val : 0
}

function calcBBox(elements: PlacedElement[]): {
  minX: number; minY: number; width: number; height: number
} {
  if (elements.length === 0) {
    return { minX: 0, minY: 0, width: NODE_W, height: NODE_H }
  }
  let minX = Infinity, minY = Infinity, maxX = -Infinity, maxY = -Infinity
  for (const o of elements) {
    const px = getPos(o, 'x')
    const py = getPos(o, 'y')
    minX = Math.min(minX, px)
    minY = Math.min(minY, py)
    maxX = Math.max(maxX, px + NODE_W)
    maxY = Math.max(maxY, py + NODE_H)
  }
  if (!isFinite(minX)) return { minX: 0, minY: 0, width: NODE_W, height: NODE_H }
  return { minX, minY, width: maxX - minX, height: maxY - minY }
}

/** Build a lookup: elementId → ElementDiagramLink for child connectors only. */
function buildChildLinkMap(
  links: ViewConnector[],
  fromDiagramId: number,
): Map<number, ViewConnector> {
  const m = new Map<number, ViewConnector>()
  for (const l of links) {
    if (l.from_view_id === fromDiagramId && l.relation_type === 'child' && l.element_id != null) {
      m.set(l.element_id, l)
    }
  }
  return m
}

/**
 * Recursively build LayoutNode[] for all elements in a diagram.
 * `worldOffsetX/Y` are the world-space origin of the diagram that
 * contains these elements.
 * `pad` is added to each element's local position (GROUP_PAD for root
 * diagrams, 0 for children stored in raw editor coords).
 *
 * `visited` is a Set of diagram IDs to prevent infinite recursion for cyclic links.
 */
function buildNodes(
  elements: PlacedElement[],
  views: ExploreData['views'],
  links: ViewConnector[],
  diagramId: number,
  worldOffsetX: number,
  worldOffsetY: number,
  bboxMinX: number,
  bboxMinY: number,
  visited: Set<number>,
  pad: number = GROUP_PAD,
  diagramX: number = 0,
  _ignoredPortalsXOffset: number = 0, // kept to minimize recursive signature change
  _diagramW: number = 0,
  _diagramH: number = 0,
  tree: ExploreData['tree'] = [],
  ancestorElementIds: number[] = [],
): LayoutNode[] {
  const childLinkMap = buildChildLinkMap(links, diagramId)
  const _treeMap = new Map(tree.map((d) => [d.id, d]))

  const realNodes: LayoutNode[] = elements.map((obj) => {
    const localX = getPos(obj, 'x') - bboxMinX + pad
    const localY = getPos(obj, 'y') - bboxMinY + pad

    const link = childLinkMap.get(obj.element_id)

    // ── Build children if this element connects to a child diagram ──
    let children: LayoutNode[] = []
    let childScale = 1
    let childOffsetX = 0
    let childOffsetY = 0
    let linkedDiagramId: number | undefined
    let linkedDiagramLabel: string | undefined
    let isCircular = false

    if (link) {
      linkedDiagramId = link.to_view_id
      linkedDiagramLabel = link.to_view_name

      // Check for cycle before recursing
      if (visited.has(link.to_view_id)) {
        isCircular = true
      } else {
        const childDiagData = views[String(link.to_view_id)]

        if (childDiagData && childDiagData.placements.length > 0) {
          const cBBox = calcBBox(childDiagData.placements)

          const contentW = cBBox.width + CHILD_PAD * 2
          const contentH = cBBox.height + CHILD_PAD * 2
          const fittedH = Math.min(contentH, contentW)
          childScale = Math.min(
            (NODE_W - CHILD_PAD * 2) / contentW,
            (Math.min(NODE_H, fittedH) - CHILD_PAD * 2) / Math.max(contentH, 1),
            0.45,
          )
          const scaledW = cBBox.width * childScale
          const scaledH = cBBox.height * childScale
          const marginX = (NODE_W - scaledW) / 2
          const marginY = (NODE_H - scaledH) / 2
          childOffsetX = cBBox.minX - marginX / childScale
          childOffsetY = cBBox.minY - marginY / childScale

          const nextVisited = new Set(visited)
          nextVisited.add(link.to_view_id)

          children = buildNodes(
            childDiagData.placements,
            views,
            links,
            link.to_view_id,
            0, 0, 0, 0,
            nextVisited,
            0,
            0, 0, 0, 0,
            tree,
            [...ancestorElementIds, obj.element_id],
          )
        }
      }
    }

    const edgesOut = (views[String(diagramId)]?.connectors ?? [])
      .filter((e) => e.source_element_id === obj.element_id)
      .map((e) => ({
        targetId: nodeId(diagramId, e.target_element_id),
        label: e.label ?? '',
        direction: e.direction ?? 'forward',
        sourceHandle: e.source_handle ?? null,
        targetHandle: e.target_handle ?? null,
        type: e.style || 'bezier',
      }))

    const derivedPrimaryIconPath = (() => {
      const selected = obj.technology_connectors?.find((link) => link.type === 'catalog' && !!(link.is_primary_icon ?? (link as any).isPrimaryIcon) && !!link.slug)
      if (!selected?.slug) return null
      return resolveIconPath(`/icons/${selected.slug}.png`)
    })()

    return {
      id: nodeId(diagramId, obj.element_id),
      elementId: obj.element_id,
      diagramId,
      worldX: worldOffsetX + diagramX + localX,
      worldY: worldOffsetY + localY,
      worldW: NODE_W,
      worldH: NODE_H,
      label: obj.name,
      type: obj.kind ?? 'system',
      logoUrl: obj.logo_url ? resolveIconPath(obj.logo_url) : derivedPrimaryIconPath,
      description: obj.description ?? null,
      technology: obj.technology ?? null,
      tags: obj.tags ?? [],
      ancestorElementIds,
      pathElementIds: [...ancestorElementIds, obj.element_id],
      linkedDiagramId,
      linkedDiagramLabel,
      isCircular,
      isPortal: false,
      children,
      childScale,
      childOffsetX,
      childOffsetY,
      edgesOut,
    }
  })

  return realNodes
}

// ── Public API ──────────────────────────────────────────────────────

/**
 * Compute the full world-space layout for all diagrams in `data`.
 * Root diagrams are placed side-by-side horizontally.
 */
export function computeLayout(data: ExploreData): ZUILayout {
  const rootDiagrams = (data.tree ?? []).filter((d) => !d.parent_view_id)
  const groups: DiagramGroupLayout[] = []
  let xCursor = 0

  for (const diag of rootDiagrams) {
    const diagData = data.views[String(diag.id)]
    if (!diagData) continue

    const bbox = calcBBox(diagData.placements ?? [])
    const diagramW = bbox.width + GROUP_PAD * 2
    const diagramH = bbox.height + GROUP_PAD * 2

    const TOP_PAD = 80
    const worldW = diagramW
    const worldH = diagramH + TOP_PAD
    const diagramX = 0
    const diagramY = TOP_PAD

    const visited = new Set<number>()
    visited.add(diag.id)

    const nodes = buildNodes(
      diagData.placements ?? [],
      data.views,
      data.navigations ?? [],
      diag.id,
      xCursor,
      diagramY,   // Note: this acts as worldOffsetY in buildNodes, effectively shifting everything down
      bbox.minX,
      bbox.minY,
      visited,
      GROUP_PAD,
      diagramX,   // Replacing portalYOffset placeholder with diagram parameters for proper circle tracing
      0,          // Replacing portalsXOffset
      diagramW,   // passing these so buildNodes can compute diagram center in world
      diagramH,
      data.tree ?? [],
      [],
    )

    // Edges within the same diagram (world-level, not children)
    const edges = (diagData.connectors ?? []).map((e) => ({
      sourceId: nodeId(diag.id, e.source_element_id),
      targetId: nodeId(diag.id, e.target_element_id),
      label: e.label ?? '',
      direction: e.direction ?? 'forward',
      sourceHandle: e.source_handle ?? null,
      targetHandle: e.target_handle ?? null,
      type: e.style || 'bezier',
    }))

    groups.push({
      diagramId: diag.id,
      label: diag.name,
      description: diag.description ?? null,
      level: diag.level,
      levelLabel: diag.level_label,
      worldX: xCursor,
      worldY: 0,
      worldW,
      worldH,
      diagramW,
      diagramH,
      diagramX,
      diagramY,
      nodes,
      edges,
    })

    xCursor += worldW + GROUP_SPACING
  }

  // Compute overall bounding box
  const allX = groups.flatMap((g) => [g.worldX, g.worldX + g.worldW])
  const allY = groups.flatMap((g) => [g.worldY, g.worldY + g.worldH])
  const bbox = {
    minX: Math.min(...allX, 0),
    minY: Math.min(...allY, 0),
    maxX: Math.max(...allX, 100),
    maxY: Math.max(...allY, 100),
  }

  return { groups, bbox }
}

/**
 * Compute the initial viewport that fits the entire layout on screen.
 * Returns a ZUIViewState (x=panX, y=panY, zoom).
 */
export function fitViewport(
  layout: ZUILayout,
  canvasW: number,
  canvasH: number,
  padding = 0.1,
): { x: number; y: number; zoom: number } {
  const { bbox } = layout
  const bboxW = bbox.maxX - bbox.minX
  const bboxH = bbox.maxY - bbox.minY
  if (bboxW <= 0 || bboxH <= 0) return { x: 0, y: 0, zoom: 1 }

  const pad = padding
  const zoom = Math.min(
    (canvasW * (1 - pad * 2)) / bboxW,
    (canvasH * (1 - pad * 2)) / bboxH,
    4,
  )
  const x = (canvasW - bboxW * zoom) / 2 - bbox.minX * zoom
  const y = (canvasH - bboxH * zoom) / 2 - bbox.minY * zoom
  return { x, y, zoom }
}
