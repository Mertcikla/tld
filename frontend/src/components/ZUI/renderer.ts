// src/components/ZUI/renderer.ts

import type { DiagramGroupLayout, LayoutNode, ZUIViewState } from './types'
import {
  DEFAULT_SOURCE_HANDLE_SIDE,
  DEFAULT_TARGET_HANDLE_SIDE,
  getHandleFlowPosition,
  getLogicalHandleId,
  getVisualHandleIdForGroup,
} from '../../utils/edgeDistribution'

// ── Thresholds (screen pixels) ─────────────────────────────────────
// Responsive thresholds: smaller screens expand earlier.
export function getExpandThresholds(canvasW: number) {
  return {
    start: clamp(canvasW * 0.25, 80, 450),
    end: clamp(canvasW * 0.4, 200, 640),
  }
}

const MIN_LABEL_PX = 12    // below this screen width, skip label text
const MIN_DRAW_PX = 2     // below this screen width, skip node entirely
const BADGE_THRESHOLD = 100 // node width in screen pixels below which we hide type badge and zoom icon

// ── Screen-space font limits (px) ──────────────────────────────────
const MIN_FONT_NAME = 10
const MAX_FONT_NAME = 50
const MIN_FONT_BADGE = 12
const MAX_FONT_BADGE = 30
const MIN_FONT_HINT = 12
const MAX_FONT_HINT = 24

export interface ScreenRect {
  left: number
  top: number
  right: number
  bottom: number
}

const frameLabelRects: ScreenRect[] = []

/**
 * Returns a world-space font size that, when multiplied by zoom,
 * stays within [minScreenSize, maxScreenSize] screen pixels,
 * while preferring baseWorldSize if possible.
 */
function getClampedFontSize(baseWorldSize: number, minScreenSize: number, maxScreenSize: number, zoom: number): number {
  return clamp(baseWorldSize, minScreenSize / zoom, maxScreenSize / zoom)
}

// ── Chakra v2 type palette - mirrors TYPE_COLORS in src/types/index.ts ─
// .400 variants: used for type badge text and border tint
const TYPE_COLOR_400: Record<string, string> = {
  person: '#38b2ac',  // teal.400
  system: '#63b3ed',  // blue.400
  container: '#9f7aea',  // purple.400
  component: '#f6ad55',  // orange.400
  database: '#4fd1c5',  // cyan.400
  queue: '#f6e05e',  // yellow.400
  api: '#68d391',  // green.400
  service: '#f687b3',  // pink.400
  external: '#a0aec0',  // gray.400
}

/** Border color: type .400 at 50% alpha - bold branded tint */
const typeBorderColorCache = new Map<string, string>()
function typeBorderColor(type: string, alpha = 0.5): string {
  const cacheKey = `${type}:${alpha}`
  const cached = typeBorderColorCache.get(cacheKey)
  if (cached) return cached

  const color = TYPE_COLOR_400[type]
  const hex = typeof color === 'string' ? color : '#a0aec0'
  const r = parseInt(hex.slice(1, 3), 16)
  const g = parseInt(hex.slice(3, 5), 16)
  const b = parseInt(hex.slice(5, 7), 16)
  const rgba = `rgba(${r},${g},${b},${alpha})`
  typeBorderColorCache.set(cacheKey, rgba)
  return rgba
}

interface RendererThemeVars {
  canvasBg: string
  nodeBg: string
  accent: string
  labelBg: string
}

const themeFallbacks: RendererThemeVars = {
  canvasBg: '#0d121e',
  nodeBg: '#2d3748',
  accent: '#63b3ed',
  labelBg: '#171923',
}

let cachedThemeVars: RendererThemeVars = themeFallbacks
let themeObserverStarted = false

function refreshThemeVars(): void {
  if (typeof document === 'undefined') return
  const styles = getComputedStyle(document.documentElement)
  cachedThemeVars = {
    canvasBg: styles.getPropertyValue('--bg-main').trim() || themeFallbacks.canvasBg,
    nodeBg: styles.getPropertyValue('--bg-element').trim() || themeFallbacks.nodeBg,
    accent: styles.getPropertyValue('--accent').trim() || themeFallbacks.accent,
    labelBg: styles.getPropertyValue('--chakra-colors-gray-900').trim() || themeFallbacks.labelBg,
  }
}

function getThemeVars(): RendererThemeVars {
  if (!themeObserverStarted && typeof document !== 'undefined') {
    themeObserverStarted = true
    refreshThemeVars()
    const update = () => refreshThemeVars()
    const mo = new MutationObserver(update)
    mo.observe(document.documentElement, { attributes: true, attributeFilter: ['class', 'style', 'data-theme'] })
    window.matchMedia?.('(prefers-color-scheme: dark)').addEventListener?.('change', update)
    window.matchMedia?.('(prefers-color-scheme: light)').addEventListener?.('change', update)
  }
  return cachedThemeVars
}

// ── Geometry helpers ───────────────────────────────────────────────

const imageCache = new Map<string, HTMLImageElement>()

let onImageLoadCallback: (() => void) | null = null
export function setOnImageLoadCallback(cb: (() => void) | null) {
  onImageLoadCallback = cb
}

let currentHighlightedTags: Set<string> = new Set()
export function setHighlightedTags(tags: Set<string>): void {
  currentHighlightedTags = tags
}

let currentHighlightColor = ''
export function setHighlightColor(color: string): void {
  currentHighlightColor = color
}

let currentHiddenTags: Set<string> = new Set()
export function setHiddenTags(tags: Set<string>): void {
  currentHiddenTags = tags
}

/**
 * Get image from cache or start loading it.
 * Returns the image if already loaded, null otherwise.
 */
function getOrLoadImage(url: string | null): HTMLImageElement | null {
  if (!url) return null
  const cached = imageCache.get(url)
  if (cached) {
    return cached.complete && cached.naturalWidth > 0 ? cached : null
  }

  const img = new Image()
  img.src = url
  img.onload = () => {
    if (onImageLoadCallback) onImageLoadCallback()
  }
  imageCache.set(url, img)
  return null
}

function clamp(v: number, min: number, max: number): number {
  return v < min ? min : v > max ? max : v
}

function transitionT(screenW: number, start: number, end: number): number {
  return clamp((screenW - start) / (end - start), 0, 1)
}

function rectsOverlap(a: ScreenRect, b: ScreenRect): boolean {
  return a.left < b.right && a.right > b.left && a.top < b.bottom && a.bottom > b.top
}

export function pickEdgeLabelPosition(
  matrix: DOMMatrix,
  midX: number,
  midY: number,
  textW: number,
  textH: number,
  dx: number,
  dy: number,
  occupiedLabelRects: ScreenRect[],
) {
  const screenMidX = matrix.a * midX + matrix.c * midY + matrix.e
  const screenMidY = matrix.b * midX + matrix.d * midY + matrix.f
  const screenTextW = Math.max(1, textW * matrix.a)
  const screenTextH = Math.max(1, textH * matrix.d)
  const gap = 6
  const step = screenTextH + gap
  const length = Math.hypot(dx, dy) || 1
  const normalX = -dy / length
  const normalY = dx / length
  const tangentX = dx / length
  const tangentY = dy / length

  for (let i = 0; i < 9; i++) {
    let offsetX = 0
    let offsetY = 0
    if (i === 1) {
      offsetX = normalX * step
      offsetY = normalY * step
    } else if (i === 2) {
      offsetX = -normalX * step
      offsetY = -normalY * step
    } else if (i === 3) {
      offsetX = normalX * step * 2
      offsetY = normalY * step * 2
    } else if (i === 4) {
      offsetX = -normalX * step * 2
      offsetY = -normalY * step * 2
    } else if (i === 5) {
      offsetX = tangentX * step
      offsetY = tangentY * step
    } else if (i === 6) {
      offsetX = -tangentX * step
      offsetY = -tangentY * step
    } else if (i === 7) {
      offsetX = tangentX * step + normalX * step
      offsetY = tangentY * step + normalY * step
    } else if (i === 8) {
      offsetX = -tangentX * step - normalX * step
      offsetY = -tangentY * step - normalY * step
    }

    const centerX = screenMidX + offsetX
    const centerY = screenMidY + offsetY
    const rect: ScreenRect = {
      left: centerX - screenTextW / 2 - gap,
      top: centerY - screenTextH / 2 - gap / 2,
      right: centerX + screenTextW / 2 + gap,
      bottom: centerY + screenTextH / 2 + gap / 2,
    }
    if (occupiedLabelRects.some((existing) => rectsOverlap(rect, existing))) continue
    occupiedLabelRects.push(rect)
    const det = matrix.a * matrix.d - matrix.b * matrix.c
    if (det === 0) return { x: midX, y: midY }
    const translatedX = centerX - matrix.e
    const translatedY = centerY - matrix.f
    return {
      x: (matrix.d * translatedX - matrix.c * translatedY) / det,
      y: (-matrix.b * translatedX + matrix.a * translatedY) / det,
    }
  }

  const fallbackRect: ScreenRect = {
    left: screenMidX - screenTextW / 2 - gap,
    top: screenMidY - screenTextH / 2 - gap / 2,
    right: screenMidX + screenTextW / 2 + gap,
    bottom: screenMidY + screenTextH / 2 + gap / 2,
  }
  occupiedLabelRects.push(fallbackRect)
  return { x: midX, y: midY }
}

/** Is the rect (in world space) visible on screen? */
export function isVisible(
  worldX: number, worldY: number, worldW: number, worldH: number,
  view: ZUIViewState, canvasW: number, canvasH: number,
): boolean {
  const sx = worldX * view.zoom + view.x
  const sy = worldY * view.zoom + view.y
  const sw = worldW * view.zoom
  const sh = worldH * view.zoom
  return sx + sw > 0 && sy + sh > 0 && sx < canvasW && sy < canvasH
}

/** Is the rect (in world space) FULLY visible on screen? */
export function isFullyVisible(
  worldX: number, worldY: number, worldW: number, worldH: number,
  view: ZUIViewState, canvasW: number, canvasH: number,
): boolean {
  const sx = worldX * view.zoom + view.x
  const sy = worldY * view.zoom + view.y
  const sw = worldW * view.zoom
  const sh = worldH * view.zoom
  return sx >= 0 && sy >= 0 && sx + sw <= canvasW && sy + sh <= canvasH
}

/** Draw the ZoomIn magnifying glass icon. */
function drawZoomInIcon(ctx: CanvasRenderingContext2D, x: number, y: number, size: number, strokeWidth: number): void {
  ctx.save()
  ctx.translate(x, y)
  const s = size / 24
  ctx.scale(s, s)
  ctx.beginPath()
  // Magnifying glass circle: cx="11" cy="11" r="8"
  ctx.arc(11, 11, 8, 0, Math.PI * 2)
  // Handle: x1="21" y1="21" x2="16.65" y2="16.65"
  ctx.moveTo(21, 21)
  ctx.lineTo(16.65, 16.65)
  // Plus vertical: x1="11" y1="8" x2="11" y2="14"
  ctx.moveTo(11, 8)
  ctx.lineTo(11, 14)
  // Plus horizontal: x1="8" y1="11" x2="14" y2="11"
  ctx.moveTo(8, 11)
  ctx.lineTo(14, 11)
  ctx.lineWidth = strokeWidth
  ctx.lineCap = 'round'
  ctx.lineJoin = 'round'
  ctx.stroke()
  ctx.restore()
}

/** Draw a portal arrow icon (↗) for portal nodes. */
function drawPortalIcon(ctx: CanvasRenderingContext2D, x: number, y: number, size: number, strokeWidth: number, color: string): void {
  ctx.save()
  ctx.strokeStyle = color
  ctx.lineWidth = strokeWidth
  ctx.lineCap = 'round'
  ctx.lineJoin = 'round'
  ctx.translate(x, y)
  const s = size / 16
  ctx.scale(s, s)
  ctx.beginPath()
  // Arrow shaft: (2,14) → (13,3)
  ctx.moveTo(2, 14)
  ctx.lineTo(13, 3)
  // Arrow head
  ctx.moveTo(5, 3)
  ctx.lineTo(13, 3)
  ctx.lineTo(13, 11)
  ctx.stroke()
  ctx.restore()
}

/** Draw a cycle icon (↺) for circular nodes. */
function drawCycleIcon(ctx: CanvasRenderingContext2D, x: number, y: number, size: number, strokeWidth: number, color: string): void {
  ctx.save()
  ctx.strokeStyle = color
  ctx.lineWidth = strokeWidth
  ctx.lineCap = 'round'
  ctx.lineJoin = 'round'
  ctx.translate(x, y)
  const s = size / 24
  ctx.scale(s, s)
  ctx.beginPath()
  // Circular arrow
  ctx.arc(12, 12, 8, 0, Math.PI * 1.5)
  ctx.moveTo(12, 4)
  ctx.lineTo(16, 4)
  ctx.lineTo(16, 0)
  ctx.stroke()
  ctx.restore()
}

/** Parse a hex CSS color (#rrggbb or #rgb) into { r, g, b } 0-255. */
function parseHex(hex: string): { r: number; g: number; b: number } {
  hex = hex.trim().replace(/^#/, '')
  if (hex.length === 3) hex = hex[0] + hex[0] + hex[1] + hex[1] + hex[2] + hex[2]
  return {
    r: parseInt(hex.slice(0, 2), 16),
    g: parseInt(hex.slice(2, 4), 16),
    b: parseInt(hex.slice(4, 6), 16),
  }
}

/** Derive a portal tint color from the accent: same hue, very low alpha. */
const portalTintColorCache = new Map<string, string>()
function portalTintColor(accent: string, alpha: number): string {
  const cacheKey = `${accent}:${alpha}`
  const cached = portalTintColorCache.get(cacheKey)
  if (cached) return cached
  const { r, g, b } = parseHex(accent)
  const rgba = `rgba(${r},${g},${b},${alpha})`
  portalTintColorCache.set(cacheKey, rgba)
  return rgba
}

/** Draw a squiggly line from (x1, y1) to (x2, y2). */
function drawSquigglyLine(ctx: CanvasRenderingContext2D, x1: number, y1: number, x2: number, y2: number, zoom: number): void {
  ctx.save()
  ctx.beginPath()
  ctx.moveTo(x1, y1)
  ctx.lineTo(x2, y2)
  const dashLen = 6 / zoom
  ctx.setLineDash([dashLen, dashLen * 1.5])
  ctx.stroke()
  ctx.restore()
}

/** Calculate coordinate for a named handle on a node. */
function getHandlePos(nodeX: number, nodeY: number, nodeW: number, nodeH: number, handleId: string | null, isSource: boolean): { x: number, y: number, pos: 'top' | 'bottom' | 'left' | 'right' } {
  const fallback = isSource ? DEFAULT_SOURCE_HANDLE_SIDE : DEFAULT_TARGET_HANDLE_SIDE
  const { x, y, side } = getHandleFlowPosition(nodeX, nodeY, nodeW, nodeH, handleId, fallback)
  return { x, y, pos: side }
}

/** Draw a closed arrow head matching React Flow MarkerType.ArrowClosed. */
function drawArrowHead(ctx: CanvasRenderingContext2D, x: number, y: number, angle: number, size: number, color: string): void {
  ctx.save()
  ctx.translate(x, y)
  ctx.rotate(angle)
  ctx.beginPath()
  // React Flow ArrowClosed is roughly a triangle
  // size 14x14
  ctx.moveTo(0, 0)
  ctx.lineTo(-size, -size * 0.45)
  ctx.lineTo(-size, size * 0.45)
  ctx.closePath()
  ctx.fillStyle = color
  ctx.fill()
  ctx.restore()
}

// ── Node drawing ───────────────────────────────────────────────────

/**
 * Draw a single node.
 *
 * @param ctx      Canvas 2D context (already in world-space transform)
 * @param node     The node to draw
 * @param screenW  Width of this node in screen pixels (worldW * zoom)
 * @param alpha    Outer opacity multiplier (from parent's childrenOpacity)
 * @param zoom     Current zoom (needed for font sizes)
 * @param accent   Resolved --accent CSS color (passed from renderFrame to avoid re-reading per node)
 * @param labelBg  Resolved label background color (passed through to avoid per-edge CSS reads)
 * @param absX     Absolute world-space X of this node (for child visibility culling)
 * @param absY     Absolute world-space Y of this node (for child visibility culling)
 * @param absScale Accumulated product of ancestor childScale values (world-space scale factor).
 *                 1 for top-level nodes; multiplied by each parent's childScale going deeper.
 *                 Required to correctly map child-local displacements to world-space for culling.
 */
function drawNode(
  ctx: CanvasRenderingContext2D,
  node: LayoutNode,
  screenW: number,
  thresholds: { start: number; end: number },
  alpha: number,
  zoom: number,
  nodeBg: string,
  canvasBg: string,
  view: ZUIViewState,
  canvasW: number,
  canvasH: number,
  accent: string,
  labelBg: string,
  absX: number,
  absY: number,
  absScale: number,
  occupiedLabelRects: ScreenRect[],
): void {
  if (screenW < MIN_DRAW_PX || alpha < 0.01) return

  // Skip nodes whose tags are all hidden
  if (currentHiddenTags.size > 0 && node.tags.length > 0 && node.tags.some(t => currentHiddenTags.has(t))) return

  const x = node.worldX
  const y = node.worldY
  const w = node.worldW
  const h = node.worldH

  let drawZoom = zoom
  let drawScreenW = screenW

  const hasChildren = node.children && node.children.length > 0
  const t = hasChildren ? transitionT(screenW, thresholds.start, thresholds.end) : 0

  // ── Cap leaf nodes visually ──
  if (!hasChildren && screenW > thresholds.end) {
    const s = thresholds.end / screenW
    drawZoom = zoom * s
    drawScreenW = thresholds.end
    ctx.save()
    const cx = x + w / 2
    const cy = y + h / 2
    ctx.translate(cx, cy)
    ctx.scale(s, s)
    ctx.translate(-cx, -cy)
  }

  const parentAlpha = alpha * (1 - t)
  const childAlpha = alpha * t
  const r = 8 / drawZoom  // matches Chakra rounded="lg" (8px)

  const borderColor = typeBorderColor(node.type)

  const traceShape = (ox = 0, oy = 0) => {
    ctx.beginPath()
    ctx.roundRect(x + ox, y + oy, w, h, r)
  }

  // ── Circular Link Overlay - subtle indicator ──────────────────────
  if (node.isCircular && parentAlpha > 0.1) {
    ctx.save()
    ctx.globalAlpha = parentAlpha * 0.15
    ctx.fillStyle = accent
    traceShape()
    ctx.fill()
    ctx.restore()
  }

  // ── Zoomable Stack Signal - subtle card stack behind ───────────────
  if (hasChildren && parentAlpha > 0.1 && t < 0.5) {
    const stackT = 1 - (t / 0.5) // Fades out completely by t=0.5
    ctx.save()
    ctx.globalAlpha = parentAlpha * stackT * 0.4
    ctx.fillStyle = nodeBg
    ctx.strokeStyle = borderColor
    ctx.lineWidth = 1 / drawZoom

    const offset1 = 4 / drawZoom
    const offset2 = 8 / drawZoom

    // Draw two offset rectangles behind the node
    // Rect 2 (deepest)
    traceShape(offset2, offset2)
    ctx.fill()
    ctx.stroke()

    // Rect 1
    traceShape(offset1, offset1)
    ctx.fill()
    ctx.stroke()
    ctx.restore()
  }

  // ── Background ───────────────────────────────────────────────────
  // We draw two backgrounds:
  // 1. A base background (canvasBg) that remains opaque (total 'alpha').
  //    This hides connectors from parent levels.
  // 2. The node's branded background (nodeBg) that fades out as we zoom in ('parentAlpha').
  //    This makes the nested diagram appear on a clean canvas background.
  if (alpha > 0.01) {
    ctx.save()
    traceShape()

    // Base background (20% transparent to allow slight ghosting of connectors)
    ctx.globalAlpha = alpha * 0.8
    ctx.fillStyle = canvasBg
    ctx.fill()

    // Fading node background
    if (parentAlpha > 0.01) {
      ctx.globalAlpha = parentAlpha * 0.8
      ctx.fillStyle = nodeBg
      ctx.fill()

      // Portal overlay: accent-tinted fill derived from --accent CSS var
      if (node.isPortal) {
        ctx.fillStyle = portalTintColor(accent, 0.10)
        ctx.fill()
      }
    }

    ctx.restore()
  }

  // ── Technology Icon - Top Center like ElementNode.tsx (no fade) ──────
  // Hide when node is too small (drawScreenW < 60)
  if (node.logoUrl && parentAlpha > 0.05 && drawScreenW > 60) {
    const img = getOrLoadImage(node.logoUrl)
    if (img) {
      ctx.save()
      ctx.globalAlpha = parentAlpha * 1

      // Scale logoMaxDim and topOffset relative to node world height 'h'
      // instead of fixed screen pixels.
      const logoMaxDim = h * 0.35
      const topOffset = h * 0.06

      const aspect = img.width / img.height
      let drawW = logoMaxDim
      let drawH = drawW / aspect

      if (drawH > logoMaxDim) {
        drawH = logoMaxDim
        drawW = drawH * aspect
      }

      // Center icon at top
      const iconX = x + (w - drawW) / 2
      const iconY = y + topOffset + (logoMaxDim - drawH) / 2

      ctx.drawImage(img, iconX, iconY, drawW, drawH)
      ctx.restore()
    }
  }
  // ── Border - portal uses accent long-dash; others use type-tinted border ─
  ctx.save()
  ctx.globalAlpha = alpha
  traceShape()
  if (node.isPortal) {
    // Solid accent border per latest request
    ctx.strokeStyle = accent
    ctx.lineWidth = 1 / drawZoom
    ctx.setLineDash([])
  } else {
    ctx.strokeStyle = borderColor
    ctx.lineWidth = 1.5 / drawZoom
    if (t > 0.15) {
      const dashLen = 6
      ctx.setLineDash([dashLen, dashLen * 0.7])
    } else {
      ctx.setLineDash([])
    }
  }
  ctx.stroke()
  ctx.setLineDash([])
  ctx.restore()

  // ── Label - portal shows "PORTAL" badge in accent; otherwise type badge ─
  if (screenW >= MIN_LABEL_PX && parentAlpha > 0.1) {
    // Dynamic minimum: don't let font be larger than a fraction of node height on screen
    const minName = Math.min(MIN_FONT_NAME, screenW * 0.35)
    // w=200, so 0.10w = 20px (Chakra 'xl')
    const nameFontSize = getClampedFontSize(w * 0.10, minName, MAX_FONT_NAME, drawZoom)
    const screenFontSize = nameFontSize * drawZoom

    if (screenFontSize >= 6) {
      ctx.save()
      ctx.globalAlpha = parentAlpha
      ctx.font = `600 ${nameFontSize}px Inter, system-ui, sans-serif`
      ctx.fillStyle = '#f7fafc'  // gray.100
      ctx.textAlign = 'center'
      ctx.textBaseline = 'middle'

      const worldPadding = w * 0.08
      const maxW = w - worldPadding
      let label = node.label
      const totalW = ctx.measureText(label).width
      if (totalW > maxW) {
        const ratio = maxW / totalW
        label = label.slice(0, Math.max(3, Math.floor(label.length * ratio)))
        if (label.length < node.label.length) label += '…'
      }

      // If logo exists and is shown, push text down similar to ElementNode.tsx (pt=9/36px)
      const showLogo = !!node.logoUrl && drawScreenW > 60
      const baseOffset = showLogo ? 0.15 : 0
      const nameY = drawScreenW > BADGE_THRESHOLD ? y + h * (0.42 + baseOffset) : y + h * (0.5 + baseOffset)
      ctx.fillText(label, x + w / 2, nameY)

      // Type badge - using regular element type display
      if (drawScreenW > BADGE_THRESHOLD) {
        const minBadge = Math.min(MIN_FONT_BADGE, screenW * 0.20)
        // 0.05w = 10px (Chakra '2xs')
        const badgeFontSize = getClampedFontSize(w * 0.05, minBadge, MAX_FONT_BADGE, drawZoom)
        if (badgeFontSize * drawZoom >= 5) {
          ctx.font = `${badgeFontSize}px Inter, system-ui, sans-serif`
          const badgeColor = TYPE_COLOR_400[node.type]
          ctx.fillStyle = typeof badgeColor === 'string' ? badgeColor : '#a0aec0'
          const displayType = typeof node.type === 'string' ? node.type.toUpperCase() : 'UNKNOWN'
          ctx.fillText(displayType, x + w / 2, y + h * (0.62 + baseOffset))
        }
      }
      ctx.restore()
    }
  }

  // ── Linked-diagram hint below node during transition ─────────────
  if (node.linkedDiagramLabel && t > 0.05 && alpha > 0.05) {
    const hintFontSize = getClampedFontSize(14, MIN_FONT_HINT, MAX_FONT_HINT, drawZoom)
    const screenFontSize = hintFontSize * drawZoom

    if (screenFontSize >= 6) {
      let hintX = x + w / 2
      let hintY = y + h + 10 // Fixed distance in world units

      if (t > 0.8) {
        // Sticky hint Y: stick to viewport bottom
        const viewportBottomWorld = (canvasH - screenFontSize - view.y) / view.zoom
        hintY = Math.min(hintY, viewportBottomWorld)
        hintY = Math.max(hintY, y + h / 2) // avoid overlapping center

        // Sticky hint X: stick to viewport sides
        const vwL = -view.x / view.zoom
        const vwR = (canvasW - view.x) / view.zoom

        ctx.save()
        ctx.font = `${hintFontSize}px Inter, system-ui, sans-serif`
        const tw = ctx.measureText('⊞ ' + node.linkedDiagramLabel).width
        ctx.restore()

        const pad = 30 / view.zoom
        hintX = Math.max(hintX, vwL + tw / 2 + pad)
        hintX = Math.min(hintX, vwR - tw / 2 - pad)
        // Ensure it stays within node boundaries (with some padding)
        hintX = clamp(hintX, x + tw / 2 + 10, x + w - tw / 2 - 10)
      }

      ctx.save()
      ctx.globalAlpha = alpha * 0.7
      ctx.font = `${hintFontSize}px Inter, system-ui, sans-serif`
      ctx.fillStyle = node.isCircular ? accent : '#718096'  // accent for circular to draw attention
      ctx.textAlign = 'center'
      ctx.textBaseline = 'top'
      const hintPrefix = node.isCircular ? '↺ ' : '⊞ '
      const hintSuffix = node.isCircular ? ' (Circular)' : ''
      ctx.fillText(hintPrefix + node.linkedDiagramLabel + hintSuffix, hintX, hintY)
      ctx.restore()
    }
  }

  // ── Children ─────────────────────────────────────────────────────
  if (childAlpha > 0.01 && node.children.length > 0) {
    ctx.save()
    // Clip to the node's rect so children don't bleed out
    traceShape()
    ctx.clip()

    // Transform into child-local space
    ctx.translate(x, y)
    ctx.scale(node.childScale, node.childScale)
    ctx.translate(-node.childOffsetX, -node.childOffsetY)

    const childZoom = zoom * node.childScale
    const edgeZoom = drawZoom * node.childScale

    // Recursive children's edges DRAWN FIRST (below nodes)
    if (childAlpha > 0.2) {
      drawEdges(ctx, node.children, childAlpha * 0.5, edgeZoom, thresholds, accent, labelBg, occupiedLabelRects)
    }

    const nextAbsScale = absScale * node.childScale
    for (const child of node.children) {
      const childAbsX = absX + (child.worldX - node.childOffsetX) * node.childScale * absScale
      const childAbsY = absY + (child.worldY - node.childOffsetY) * node.childScale * absScale
      const childAbsW = child.worldW * node.childScale * absScale
      const childAbsH = child.worldH * node.childScale * absScale
      if (!isVisible(childAbsX, childAbsY, childAbsW, childAbsH, view, canvasW, canvasH)) continue

      const childScreenW = child.worldW * childZoom
      drawNode(ctx, child, childScreenW, thresholds, childAlpha, childZoom, nodeBg, canvasBg, view, canvasW, canvasH, accent, labelBg, childAbsX, childAbsY, nextAbsScale, occupiedLabelRects)
    }

    ctx.restore()
  }

  // ── Zoomable indicator (top-right) ──────────────────────────────
  if ((hasChildren || node.isCircular) && t < 0.9 && alpha > 0.2 && drawScreenW > BADGE_THRESHOLD) {
    const iconSize = getClampedFontSize(12, 10, 16, drawZoom)
    const padding = 8 / drawZoom

    ctx.save()
    // Noticeable but subtle: opacity fades as we zoom in (t increases)
    ctx.globalAlpha = alpha * (1 - t) * 0.8
    ctx.strokeStyle = accent
    if (node.isCircular) {
      drawCycleIcon(ctx, x + w - iconSize - padding, y + padding, iconSize, 3.5, accent)
    } else if (node.isPortal) {
      // Portal: use arrow icon instead of magnifying glass
      drawPortalIcon(ctx, x + w - iconSize - padding, y + padding, iconSize, 3.5, accent)
    } else {
      drawZoomInIcon(ctx, x + w - iconSize - padding, y + padding, iconSize, 3.5)
    }
    ctx.restore()
  }

  // ── Tag highlighting dim / glow ──────────────────────────────────
  if (currentHighlightedTags.size > 0 && parentAlpha > 0.05) {
    const isHighlighted = node.tags.length > 0 && node.tags.some(t => currentHighlightedTags.has(t))
    if (!isHighlighted) {
      ctx.save()
      ctx.globalAlpha = parentAlpha * 0.82
      ctx.fillStyle = canvasBg
      traceShape()
      ctx.fill()
      ctx.restore()
    } else {
      const glowColor = currentHighlightColor || accent
      ctx.save()
      ctx.globalAlpha = parentAlpha
      ctx.shadowColor = glowColor
      ctx.shadowBlur = 8 / drawZoom
      ctx.strokeStyle = glowColor
      ctx.lineWidth = 2.5 / drawZoom
      ctx.setLineDash([])
      traceShape()
      ctx.stroke()
      ctx.shadowBlur = 0
      ctx.restore()
    }
  }

  if (!hasChildren && screenW > thresholds.end) {
    ctx.restore()
  }
}

// ── Edge drawing ───────────────────────────────────────────────────

interface HandleUsage {
  edgeKey: string
  type: 'source' | 'target'
  otherNodeCoord: number
}

interface DrawEdgesLayoutMetadata {
  nodeMap: Map<string, LayoutNode>
  handleUsage: Record<string, HandleUsage[]>
  handleUsageIndex: Record<string, number>
}

const drawEdgesMetadataCache = new WeakMap<LayoutNode[], DrawEdgesLayoutMetadata>()
const emptyHandleUsage: HandleUsage[] = []

function getDrawEdgesLayoutMetadata(nodes: LayoutNode[]): DrawEdgesLayoutMetadata {
  const cached = drawEdgesMetadataCache.get(nodes)
  if (cached) return cached

  const nodeMap = new Map<string, LayoutNode>()
  const handleUsage: Record<string, HandleUsage[]> = {}
  const handleUsageIndex: Record<string, number> = {}

  for (const node of nodes) {
    nodeMap.set(node.id, node)
  }

  for (const node of nodes) {
    for (let edgeIndex = 0; edgeIndex < node.edgesOut.length; edgeIndex++) {
      const edge = node.edgesOut[edgeIndex]
      const target = nodeMap.get(edge.targetId)
      if (!target) continue

      const edgeKey = `${node.id}:${edgeIndex}`
      const sourceSide = getLogicalHandleId(edge.sourceHandle, DEFAULT_SOURCE_HANDLE_SIDE) ?? DEFAULT_SOURCE_HANDLE_SIDE
      const targetSide = getLogicalHandleId(edge.targetHandle, DEFAULT_TARGET_HANDLE_SIDE) ?? DEFAULT_TARGET_HANDLE_SIDE

      const srcKey = `${node.id}-${sourceSide}`
      handleUsage[srcKey] ??= []
      handleUsage[srcKey].push({
        edgeKey,
        type: 'source',
        otherNodeCoord: sourceSide === 'left' || sourceSide === 'right'
          ? target.worldY + target.worldH / 2
          : target.worldX + target.worldW / 2,
      })

      const tgtKey = `${target.id}-${targetSide}`
      handleUsage[tgtKey] ??= []
      handleUsage[tgtKey].push({
        edgeKey,
        type: 'target',
        otherNodeCoord: targetSide === 'left' || targetSide === 'right'
          ? node.worldY + node.worldH / 2
          : node.worldX + node.worldW / 2,
      })
    }
  }

  for (const [usageKey, usages] of Object.entries(handleUsage)) {
    usages.sort((a, b) => a.otherNodeCoord - b.otherNodeCoord)
    for (let i = 0; i < usages.length; i++) {
      const usage = usages[i]
      handleUsageIndex[`${usageKey}:${usage.edgeKey}:${usage.type}`] = i
    }
  }

  const metadata = { nodeMap, handleUsage, handleUsageIndex }
  drawEdgesMetadataCache.set(nodes, metadata)
  return metadata
}

function drawEdges(
  ctx: CanvasRenderingContext2D,
  nodes: LayoutNode[],
  alpha: number,
  zoom: number,
  thresholds: { start: number; end: number },
  accent: string,
  labelBg: string,
  occupiedLabelRects: ScreenRect[],
): void {
  if (alpha < 0.05) return
  const { nodeMap, handleUsage, handleUsageIndex } = getDrawEdgesLayoutMetadata(nodes)

  for (const node of nodes) {
    for (const [edgeIndex, edge] of node.edgesOut.entries()) {
      const target = nodeMap.get(edge.targetId)
      if (!target) continue

      // Skip edge if either endpoint is hidden by tag filter
      if (currentHiddenTags.size > 0) {
        const srcHidden = node.tags.length > 0 && node.tags.some(t => currentHiddenTags.has(t))
        const tgtHidden = target.tags.length > 0 && target.tags.some(t => currentHiddenTags.has(t))
        if (srcHidden || tgtHidden) continue
      }

      const dir = edge.direction ?? 'forward'
      const type = edge.type || 'bezier'

      // ── Effective visual dimensions (handles capping) ─────────────
      const hasSourceChildren = node.children && node.children.length > 0
      const sourceScreenW = node.worldW * zoom
      const sSource = (!hasSourceChildren && sourceScreenW > thresholds.end) ? thresholds.end / sourceScreenW : 1
      const effWSource = node.worldW * sSource
      const effHSource = node.worldH * sSource
      const cxSource = node.worldX + node.worldW / 2
      const cySource = node.worldY + node.worldH / 2
      const effXSource = cxSource - effWSource / 2
      const effYSource = cySource - effHSource / 2

      const hasTargetChildren = target.children && target.children.length > 0
      const targetScreenW = target.worldW * zoom
      const sTarget = (!hasTargetChildren && targetScreenW > thresholds.end) ? thresholds.end / targetScreenW : 1
      const effWTarget = target.worldW * sTarget
      const effHTarget = target.worldH * sTarget
      const cxTarget = target.worldX + target.worldW / 2
      const cyTarget = target.worldY + target.worldH / 2
      const effXTarget = cxTarget - effWTarget / 2
      const effYTarget = cyTarget - effHTarget / 2

      const edgeKey = `${node.id}:${edgeIndex}`
      const sourceSide = getLogicalHandleId(edge.sourceHandle, DEFAULT_SOURCE_HANDLE_SIDE) ?? DEFAULT_SOURCE_HANDLE_SIDE
      const targetSide = getLogicalHandleId(edge.targetHandle, DEFAULT_TARGET_HANDLE_SIDE) ?? DEFAULT_TARGET_HANDLE_SIDE
      const srcKey = `${node.id}-${sourceSide}`
      const tgtKey = `${target.id}-${targetSide}`
      const srcGroup = handleUsage[srcKey] ?? emptyHandleUsage
      const tgtGroup = handleUsage[tgtKey] ?? emptyHandleUsage
      const sourceGroupIndex = handleUsageIndex[`${srcKey}:${edgeKey}:source`] ?? -1
      const targetGroupIndex = handleUsageIndex[`${tgtKey}:${edgeKey}:target`] ?? -1

      const sH = getHandlePos(
        effXSource,
        effYSource,
        effWSource,
        effHSource,
        getVisualHandleIdForGroup(sourceSide, sourceGroupIndex, Math.max(srcGroup.length, 1)),
        true,
      )
      const tH = getHandlePos(
        effXTarget,
        effYTarget,
        effWTarget,
        effHTarget,
        getVisualHandleIdForGroup(targetSide, targetGroupIndex, Math.max(tgtGroup.length, 1)),
        false,
      )

      ctx.save()
      ctx.globalAlpha = alpha * 0.8
      ctx.strokeStyle = accent
      ctx.lineWidth = 2 / zoom

      let midX = (sH.x + tH.x) / 2
      let midY = (sH.y + tH.y) / 2
      let finalAngleS = 0
      let finalAngleT = 0

      if (type === 'bezier') {
        const curvature = 0.5
        let cp1x = sH.x, cp1y = sH.y, cp2x = tH.x, cp2y = tH.y
        const dx = Math.abs(tH.x - sH.x)
        const dy = Math.abs(tH.y - sH.y)

        // Minimum stem: control point must extend at least half the node's
        // dimension along the handle's exit axis. This prevents the curve
        // from taking a sharp turn when dx or dy is small relative to the node.
        const minStemSH = (sH.pos === 'left' || sH.pos === 'right') ? effWSource * 0.5 : effHSource * 0.5
        const minStemTH = (tH.pos === 'left' || tH.pos === 'right') ? effWTarget * 0.5 : effHTarget * 0.5

        if (sH.pos === 'left' || sH.pos === 'right') {
          const stem = Math.max(dx * curvature, minStemSH)
          cp1x += sH.pos === 'left' ? -stem : stem
        } else {
          const stem = Math.max(dy * curvature, minStemSH)
          cp1y += sH.pos === 'top' ? -stem : stem
        }

        if (tH.pos === 'left' || tH.pos === 'right') {
          const stem = Math.max(dx * curvature, minStemTH)
          cp2x += tH.pos === 'left' ? -stem : stem
        } else {
          const stem = Math.max(dy * curvature, minStemTH)
          cp2y += tH.pos === 'top' ? -stem : stem
        }

        ctx.beginPath()
        ctx.moveTo(sH.x, sH.y)
        ctx.bezierCurveTo(cp1x, cp1y, cp2x, cp2y, tH.x, tH.y)
        ctx.stroke()

        midX = 0.125 * sH.x + 0.375 * cp1x + 0.375 * cp2x + 0.125 * tH.x
        midY = 0.125 * sH.y + 0.375 * cp1y + 0.375 * cp2y + 0.125 * tH.y
        finalAngleT = Math.atan2(tH.y - cp2y, tH.x - cp2x)
        finalAngleS = Math.atan2(sH.y - cp1y, sH.x - cp1x)

      } else if (type === 'straight') {
        ctx.beginPath()
        ctx.moveTo(sH.x, sH.y)
        ctx.lineTo(tH.x, tH.y)
        ctx.stroke()
        finalAngleT = Math.atan2(tH.y - sH.y, tH.x - sH.x)
        finalAngleS = Math.atan2(sH.y - tH.y, sH.x - tH.x)

      } else if (type === 'step' || type === 'smoothstep') {
        const borderRadius = type === 'smoothstep' ? 6 / zoom : 0

        const points: Array<{ x: number, y: number }> = [{ x: sH.x, y: sH.y }]
        const sOrth = sH.pos === 'left' || sH.pos === 'right' ? 'h' : 'v'
        const tOrth = tH.pos === 'left' || tH.pos === 'right' ? 'h' : 'v'

        if (sOrth === 'h' && tOrth === 'h') {
          // Both horizontal: exit H, then V turn, then enter H
          points.push({ x: midX, y: sH.y })
          points.push({ x: midX, y: tH.y })
        } else if (sOrth === 'v' && tOrth === 'v') {
          // Both vertical: exit V, then H turn, then enter V
          points.push({ x: sH.x, y: midY })
          points.push({ x: tH.x, y: midY })
        } else if (sOrth === 'h' && tOrth === 'v') {
          // Mixed: exit H, turn V, enter V
          points.push({ x: tH.x, y: sH.y })
        } else if (sOrth === 'v' && tOrth === 'h') {
          // Mixed: exit V, turn H, enter H
          points.push({ x: sH.x, y: tH.y })
        }
        points.push({ x: tH.x, y: tH.y })

        // Calculate label midpoint along the orthogonal segments
        if (points.length === 4) {
          // H-H or V-V: put label in the middle of the middle segment
          midX = (points[1].x + points[2].x) / 2
          midY = (points[1].y + points[2].y) / 2
        } else if (points.length === 3) {
          // Mixed H-V or V-H: put label in the middle of the longer segment
          const d1 = Math.abs(points[1].x - points[0].x) + Math.abs(points[1].y - points[0].y)
          const d2 = Math.abs(points[2].x - points[1].x) + Math.abs(points[2].y - points[1].y)
          if (d1 > d2) {
            midX = (points[0].x + points[1].x) / 2
            midY = (points[0].y + points[1].y) / 2
          } else {
            midX = (points[1].x + points[2].x) / 2
            midY = (points[1].y + points[2].y) / 2
          }
        }

        ctx.beginPath()
        ctx.moveTo(points[0].x, points[0].y)

        for (let i = 1; i < points.length; i++) {
          const curr = points[i]
          const prev = points[i - 1]
          const next = points[i + 1]

          if (borderRadius > 0 && next) {
            // Draw line to start of corner
            const dPrevX = curr.x - prev.x
            const dPrevY = curr.y - prev.y
            const dPrevLen = Math.sqrt(dPrevX * dPrevX + dPrevY * dPrevY)
            const r = Math.min(borderRadius, dPrevLen / 2)

            ctx.lineTo(curr.x - (dPrevX / dPrevLen) * r, curr.y - (dPrevY / dPrevLen) * r)

            // Draw arc
            const dNextX = next.x - curr.x
            const dNextY = next.y - curr.y
            const dNextLen = Math.sqrt(dNextX * dNextX + dNextY * dNextY)
            const rNext = Math.min(borderRadius, dNextLen / 2)

            ctx.arcTo(curr.x, curr.y, curr.x + (dNextX / dNextLen) * rNext, curr.y + (dNextY / dNextLen) * rNext, r)
          } else {
            ctx.lineTo(curr.x, curr.y)
          }
        }
        ctx.stroke()

        // Arrows for step/smoothstep should align with final segment
        const last = points[points.length - 1]
        const prev = points[points.length - 2]
        finalAngleT = Math.atan2(last.y - prev.y, last.x - prev.x)

        const first = points[0]
        const firstNext = points[1]
        finalAngleS = Math.atan2(first.y - firstNext.y, first.x - firstNext.x)
      }

      // ── Arrow heads ───────────────────────────────────────────────
      const visualTargetScreenW = effWTarget * zoom
      const visualSourceScreenW = effWSource * zoom

      // Scale arrow with node size, but cap it at 14px
      // And hide if node is too small
      const ARROW_SIZE_BASE = 10
      const MIN_NODE_W_FOR_ARROW = 120

      if (dir === 'forward' || dir === 'both' || dir === 'bidirectional') {
        if (visualTargetScreenW > MIN_NODE_W_FOR_ARROW) {
          const arrowScreenSize = Math.min(ARROW_SIZE_BASE, visualTargetScreenW * 0.2)
          drawArrowHead(ctx, tH.x, tH.y, finalAngleT, arrowScreenSize / zoom, accent)
        }
      }
      if (dir === 'backward' || dir === 'both' || dir === 'bidirectional') {
        if (visualSourceScreenW > MIN_NODE_W_FOR_ARROW) {
          const arrowScreenSize = Math.min(ARROW_SIZE_BASE, visualSourceScreenW * 0.2)
          drawArrowHead(ctx, sH.x, sH.y, finalAngleS, arrowScreenSize / zoom, accent)
        }
      }

      // ── Edge Label ───────────────────────────────────────────
      if (edge.label && zoom * 11 > 4) {
        const fontSize = 11 / zoom
        ctx.font = `${fontSize}px Inter, system-ui, sans-serif`
        const textMetrics = ctx.measureText(edge.label)
        const textW = textMetrics.width
        const textH = fontSize
        const labelPos = pickEdgeLabelPosition(
          ctx.getTransform(),
          midX,
          midY,
          textW,
          textH,
          tH.x - sH.x,
          tH.y - sH.y,
          occupiedLabelRects,
        )

        ctx.save()
        ctx.globalAlpha = alpha * 0.95
        ctx.fillStyle = labelBg
        const px = 4 / zoom, py = 2 / zoom
        ctx.beginPath()
        ctx.roundRect(labelPos.x - textW / 2 - px, labelPos.y - textH / 2 - py, textW + px * 2, textH + py * 2, 4 / zoom)
        ctx.fill()
        ctx.restore()

        ctx.fillStyle = accent
        ctx.textAlign = 'center'
        ctx.textBaseline = 'middle'
        ctx.fillText(edge.label, labelPos.x, labelPos.y)
      }

      ctx.restore()
    }
  }
}

// ── Diagram group label ────────────────────────────────────────────

function drawGroupLabel(
  ctx: CanvasRenderingContext2D,
  group: DiagramGroupLayout,
  view: ZUIViewState,
  canvasW: number,
  canvasH: number,
  accent: string,
): void {
  const screenW = group.worldW * view.zoom
  if (screenW < 30) return

  const fontSize = clamp(13 / view.zoom, 3 / view.zoom, 24 / view.zoom)
  const labelX = group.worldX + group.diagramX + group.diagramW / 2
  const labelY = group.worldY + group.diagramY - 22 / view.zoom

  // Ensure label is within viewport
  const screenY = labelY * view.zoom + view.y
  if (screenY < -20 || screenY > canvasH + 20) return

  ctx.save()
  const levelText = group.levelLabel || `Level ${group.level}`

  // ── Level indicator (e.g. "Level 1" or "System Context")
  const levelFontSize = fontSize * 0.8
  ctx.font = `600 ${levelFontSize}px Inter, system-ui, sans-serif`
  ctx.fillStyle = accent
  ctx.globalAlpha = 0.8
  ctx.textAlign = 'center'
  ctx.textBaseline = 'bottom'
  ctx.fillText(levelText.toUpperCase(), labelX, group.worldY + group.diagramY - 30 / view.zoom)

  // ── Diagram Name
  ctx.font = `600 ${fontSize}px Inter, system-ui, sans-serif`
  ctx.fillStyle = 'rgba(255, 255, 255, 0.95)'
  const nameText = group.nodes.length === 0 ? `${group.label} (Empty)` : group.label
  ctx.fillText(nameText, labelX, group.worldY + group.diagramY - 10 / view.zoom)

  // ── Empty State Indicator inside the box
  if (group.nodes.length === 0 && view.zoom * group.diagramW > 100) {
    ctx.save()
    ctx.globalAlpha = 0.3
    ctx.font = `${fontSize * 0.7}px Inter, system-ui, sans-serif`
    ctx.textAlign = 'center'
    ctx.textBaseline = 'middle'
    ctx.fillText('This diagram has no elements.', labelX, group.worldY + group.diagramY + group.diagramH / 2)
    ctx.restore()
  }
  ctx.restore()
}


// ── Public: render one frame ───────────────────────────────────────

/**
 * Render a complete frame onto `ctx`.
 * Call this from a `requestAnimationFrame` loop.
 * The caller must set `ctx.setTransform(dpr,0,0,dpr,0,0)` before calling;
 * `canvasW/canvasH` are CSS-pixel dimensions (the transform handles HiDPI).
 */
export function renderFrame(
  ctx: CanvasRenderingContext2D,
  groups: DiagramGroupLayout[],
  view: ZUIViewState,
  canvasW: number,
  canvasH: number,
): ScreenRect[] {
  const { canvasBg, nodeBg, accent, labelBg } = getThemeVars()

  ctx.clearRect(0, 0, canvasW, canvasH)

  // Background matches the app's --bg-main
  ctx.fillStyle = canvasBg
  ctx.fillRect(0, 0, canvasW, canvasH)


  // Apply world transform
  ctx.save()
  ctx.translate(view.x, view.y)
  ctx.scale(view.zoom, view.zoom)

  const thresholds = getExpandThresholds(canvasW)
  const occupiedLabelRects = frameLabelRects
  occupiedLabelRects.length = 0

  for (const group of groups) {
    if (!isVisible(group.worldX, group.worldY, group.worldW, group.worldH, view, canvasW, canvasH)) {
      continue
    }

    drawGroupLabel(ctx, group, view, canvasW, canvasH, accent)

    // ── Group box (diagram elements container) ──────────────────────────
    const borderAlpha = clamp(0.5 - view.zoom * 0.05, 0.15, 0.5)

    ctx.save()
    ctx.globalAlpha = borderAlpha
    ctx.strokeStyle = accent
    ctx.lineWidth = 2 / view.zoom
    ctx.setLineDash([2, 2])
    // Only draw the border around the diagram part (not portals)
    ctx.strokeRect(group.worldX + group.diagramX, group.worldY + group.diagramY, group.diagramW, group.diagramH)
    ctx.setLineDash([])
    ctx.restore()

    // ── Squiggly edges to portal nodes ────────────────────────────────
    ctx.save()
    ctx.strokeStyle = accent
    ctx.setLineDash([])
    ctx.lineWidth = 2 / view.zoom
    ctx.globalAlpha = 0.6
    for (const node of group.nodes) {
      if (node.isPortal) {
        // Draw squiggle/dash from diagram box boundary to portal box boundary
        const cx = group.worldX + group.diagramX + group.diagramW / 2
        const cy = group.worldY + group.diagramY + group.diagramH / 2
        const px = node.worldX + node.worldW / 2
        const py = node.worldY + node.worldH / 2

        const dx = px - cx
        const dy = py - cy

        const getBBoxIntersection = (boxW: number, boxH: number, targetDX: number, targetDY: number) => {
          const hw = boxW / 2 + 10 // pad
          const hh = boxH / 2 + 10 // pad
          if (Math.abs(targetDX * hh) > Math.abs(targetDY * hw)) {
            return { x: Math.sign(targetDX) * hw, y: targetDY * (hw / Math.abs(targetDX)) }
          } else {
            return { x: targetDX * (hh / Math.abs(targetDY)), y: Math.sign(targetDY) * hh }
          }
        }

        const start = getBBoxIntersection(group.diagramW, group.diagramH, dx, dy)
        const end = getBBoxIntersection(node.worldW, node.worldH, -dx, -dy)

        drawSquigglyLine(ctx, cx + start.x, cy + start.y, px + end.x, py + end.y, view.zoom)
      }
    }
    ctx.restore()

    // Edges in this group
    drawEdges(ctx, group.nodes, 0.7, view.zoom, thresholds, accent, labelBg, occupiedLabelRects)

    // Nodes in this group
    for (const node of group.nodes) {
      if (!isVisible(node.worldX, node.worldY, node.worldW, node.worldH, view, canvasW, canvasH)) {
        continue
      }
      const screenW = node.worldW * view.zoom
      drawNode(ctx, node, screenW, thresholds, 1, view.zoom, nodeBg, canvasBg, view, canvasW, canvasH, accent, labelBg, node.worldX, node.worldY, 1, occupiedLabelRects)
    }
  }

  ctx.restore()
  return occupiedLabelRects
}
