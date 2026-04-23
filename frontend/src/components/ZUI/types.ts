// src/components/ZUI/types.ts

import type { ProxyConnectorDetails } from '../../crossBranch/types'

/** Pan + zoom state for the canvas viewport. */
export interface ZUIViewState {
  /** World-space X of the canvas origin (panX). */
  x: number
  /** World-space Y of the canvas origin (panY). */
  y: number
  /** Current zoom multiplier (1 = 1 world-pixel per screen-pixel). */
  zoom: number
}

/**
 * A node in the global world-space layout.
 *
 * Root-level nodes are placed directly from diagram coordinates.
 * Each node that has a linked child diagram stores that diagram's
 * elements in `children`, expressed in child-local (diagram-editor)
 * coordinates.  The `childScale` and `childOffset*` values map those
 * local coords into the node's own NODE_W × NODE_H world footprint.
 */
export interface LayoutNode {
  // ── Identity ────────────────────────────────────────────────────
  /** Unique stable id: "d{diagramId}-o{elementId}" */
  id: string
  elementId: number
  diagramId: number

  // ── World position (global coords) ──────────────────────────────
  worldX: number
  worldY: number
  /** Fixed: always NODE_W. */
  worldW: number
  /** Fixed: always NODE_H. */
  worldH: number

  // ── Appearance ───────────────────────────────────────────────────
  label: string
  /** One of: person | system | container | component | database | queue | api | service | external */
  type: string
  logoUrl: string | null
  description: string | null
  technology: string | null
  tags: string[]
  isCircular?: boolean
  ancestorElementIds: number[]
  pathElementIds: number[]

  // ── Children (from linked diagram) ──────────────────────────────
  linkedDiagramId?: number
  linkedDiagramLabel?: string
  /** True when this node's children come from a portal (diagram-to-diagram) link, not an element-child link. */
  isPortal?: boolean
  /**
   * Child nodes in CHILD-LOCAL coordinates (same coord space as the
   * diagram editor).  To draw them, apply:
   *   ctx.translate(worldX, worldY)
   *   ctx.scale(childScale, childScale)
   *   ctx.translate(-childOffsetX, -childOffsetY)
   *   …draw each child at child.worldX/worldY…
   */
  children: LayoutNode[]
  /** Scale factor: child-local → node world-space. */
  childScale: number
  /** BBox minX used to align children to top-left of node. */
  childOffsetX: number
  /** BBox minY used to align children to top-left of node. */
  childOffsetY: number

  // ── Edges within the same diagram ────────────────────────────────
  edgesOut: Array<{
    /** LayoutNode id of the target. */
    targetId: string
    label: string
    direction: 'forward' | 'backward' | 'both' | 'bidirectional' | string
    sourceHandle: string | null
    targetHandle: string | null
    type: string
  }>
}

/** Top-level group wrapping one root diagram. */
export interface DiagramGroupLayout {
  /** True when this group is a portal-linked target diagram (not a hierarchy root). */
  isPortal?: boolean
  diagramId: number
  label: string
  description: string | null
  level: number
  levelLabel: string | null
  /** Top-left of this diagram in world space. */
  worldX: number
  worldY: number
  worldW: number
  worldH: number
  /** Dimensions of the main diagram box (excluding portals below). */
  diagramW: number
  diagramH: number
  diagramX: number
  diagramY: number
  nodes: LayoutNode[]
  /** Edges whose both endpoints are in this diagram. */
  edges: Array<{
    sourceId: string
    targetId: string
    label: string
    direction: string
    sourceHandle: string | null
    targetHandle: string | null
    type: string
  }>
}

/** Bounding box in world space. */
export interface BBox {
  minX: number
  minY: number
  maxX: number
  maxY: number
}

/** Top-level result of the layout pass. */
export interface ZUILayout {
  groups: DiagramGroupLayout[]
  /** Bounding box of the whole layout in world space. */
  bbox: BBox
}

export type HoveredItem =
  | { type: 'node'; data: LayoutNode; absX: number; absY: number; absW: number; absH: number }
  | {
    type: 'edge';
    data: {
      sourceId: string;
      targetId: string;
      label: string;
      diagramId: number;
      sourceObjId?: number;
      targetObjId?: number;
      targetDiagId?: number;
      isPortalConn?: boolean;
      isProxy?: boolean;
      details?: ProxyConnectorDetails;
    };
    absX: number;
    absY: number
  }
  | { type: 'group'; data: DiagramGroupLayout }
