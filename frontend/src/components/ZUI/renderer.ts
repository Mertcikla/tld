import type { DiagramGroupLayout, LayoutNode, ZUIViewState } from './types'
import { positionForHandleSide, stepRoutePoints } from '../../utils/connectorRoute'
import type { SceneGraph, SceneNode } from './sceneGraph'
import type { ZUITransitionRebase } from './layoutEngine'
import {
  getCameraRebase,
  isVisible,
} from './layoutEngine'
import {
  DEFAULT_SOURCE_HANDLE_SIDE,
  DEFAULT_TARGET_HANDLE_SIDE,
  getHandleFlowPosition,
  getHandleSlotOffsetFromId,
  getLogicalHandleId,
  getVisualHandleIdForGroup,
} from '../../utils/edgeDistribution'

const MIN_LABEL_PX = 12
const MIN_DRAW_PX = 2
const BADGE_THRESHOLD = 100
const CONNECTOR_MIN_ALPHA = 0.32
const CONNECTOR_MAX_ALPHA = 0.95
const CONNECTOR_LINE_PX = 2
const CONNECTOR_ENDPOINT_MIN_ALPHA = 0.35
const MIN_NODE_W_FOR_CONNECTOR_DETAIL = 120

const MIN_FONT_HINT = 12
const MAX_FONT_HINT = 24

const VIEW_EDITOR_NODE_H = 85
const NAME_FONT_TO_NODE_H = 20 / VIEW_EDITOR_NODE_H
const TYPE_FONT_TO_NODE_H = 10 / VIEW_EDITOR_NODE_H
const RADIUS_TO_NODE_H = 8 / VIEW_EDITOR_NODE_H

export interface ScreenRect {
  left: number
  top: number
  right: number
  bottom: number
}

export interface EdgeLabelDrawRect {
  x: number
  y: number
  width: number
  height: number
}

const frameLabelRects: ScreenRect[] = []

function getClampedFontSize(baseWorldSize: number, minScreenSize: number, maxScreenSize: number, zoom: number): number {
  return clamp(baseWorldSize, minScreenSize / zoom, maxScreenSize / zoom)
}

const typeBorderColorCache = new Map<string, string>()
function typeBorderColor(_type: string, alpha = 0.5): string {
  const cacheKey = `${_type}:${alpha}`
  const cached = typeBorderColorCache.get(cacheKey)
  if (cached) return cached

  const hex = '#a0aec0'
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

interface NavigationHintDraw {
  matrix: DOMMatrix
  candidates: Array<{ x: number; y: number }>
  text: string
  textW: number
  textH: number
  fontSize: number
  color: string
  alpha: number
  priority: number
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

function portalTintColor(accent: string, alpha: number): string {
  const hex = accent
  const r = parseInt(hex.slice(1, 3), 16)
  const g = parseInt(hex.slice(3, 5), 16)
  const b = parseInt(hex.slice(5, 7), 16)
  return `rgba(${r},${g},${b},${alpha})`
}

export function getThemeVars(): RendererThemeVars {
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

export interface RenderContext {
  canvasBg: string
  nodeBg: string
  accent: string
  labelBg: string
  canvasW: number
  canvasH: number
  thresholds: { start: number; end: number }
  lowDetail: boolean
}

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

let currentVersionElementChanges: Map<number, string> = new Map()
let currentVersionConnectorChanges: Map<number, string> = new Map()
let currentVersionElementLineDeltas: Map<number, { added: number; removed: number }> = new Map()
let currentDiffContextElementIds: Set<number> = new Set()
let currentDiffContextConnectorIds: Set<number> = new Set()
let currentDiffLensActive = false
export function setVersionDiff(
  elementChanges: Map<number, string>,
  connectorChanges: Map<number, string>,
  elementLineDeltas: Map<number, { added: number; removed: number }> = new Map(),
  contextElementIds: Set<number> = new Set(),
  contextConnectorIds: Set<number> = new Set(),
  diffLensActive = false,
): void {
  currentVersionElementChanges = elementChanges
  currentVersionConnectorChanges = connectorChanges
  currentVersionElementLineDeltas = elementLineDeltas
  currentDiffContextElementIds = contextElementIds
  currentDiffContextConnectorIds = contextConnectorIds
  currentDiffLensActive = diffLensActive
}

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

function connectorAlpha(alpha: number, minAlpha = CONNECTOR_MIN_ALPHA): number {
  return clamp(alpha * 1.15, minAlpha, CONNECTOR_MAX_ALPHA)
}

function normalizeEdgeRouteType(type: string | null | undefined): 'bezier' | 'straight' | 'step' | 'smoothstep' {
  if (type === 'straight' || type === 'step' || type === 'smoothstep') return type
  return 'bezier'
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
): { x: number; y: number } {
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

export function edgeLabelDrawRectFromCenter(center: { x: number; y: number }, width: number, height: number): EdgeLabelDrawRect {
  return {
    x: center.x - width / 2,
    y: center.y - height / 2,
    width,
    height,
  }
}

function pickNavigationHintPosition(
  matrix: DOMMatrix,
  candidates: Array<{ x: number; y: number }>,
  textW: number,
  textH: number,
  occupiedLabelRects: ScreenRect[],
): { x: number; y: number } | null {
  const screenTextW = Math.max(1, textW * matrix.a)
  const screenTextH = Math.max(1, textH * matrix.d)
  const gap = 6

  for (const candidate of candidates) {
    const screenX = matrix.a * candidate.x + matrix.c * candidate.y + matrix.e
    const screenY = matrix.b * candidate.x + matrix.d * candidate.y + matrix.f
    const rect: ScreenRect = {
      left: screenX - screenTextW / 2 - gap,
      top: screenY - gap,
      right: screenX + screenTextW / 2 + gap,
      bottom: screenY + screenTextH + gap,
    }
    if (occupiedLabelRects.some((existing) => rectsOverlap(rect, existing))) continue
    occupiedLabelRects.push(rect)
    return candidate
  }

  return null
}

function isHiddenByTags(node: LayoutNode): boolean {
  return currentHiddenTags.size > 0 && node.tags.length > 0 && node.tags.some((t) => currentHiddenTags.has(t))
}

function drawNavigationHints(
  ctx: CanvasRenderingContext2D,
  hints: NavigationHintDraw[],
  occupiedLabelRects: ScreenRect[],
): void {
  hints.sort((a, b) => b.priority - a.priority)

  for (const hint of hints) {
    const hintPos = pickNavigationHintPosition(
      hint.matrix,
      hint.candidates,
      hint.textW,
      hint.textH,
      occupiedLabelRects,
    )
    if (!hintPos) continue

    ctx.save()
    ctx.setTransform(hint.matrix)
    ctx.globalAlpha = hint.alpha
    ctx.font = `${hint.fontSize}px Inter, system-ui, sans-serif`
    ctx.fillStyle = hint.color
    ctx.textAlign = 'center'
    ctx.textBaseline = 'top'
    ctx.fillText(hint.text, hintPos.x, hintPos.y)
    ctx.restore()
  }
}

function drawZoomInIcon(ctx: CanvasRenderingContext2D, x: number, y: number, size: number, strokeWidth: number): void {
  ctx.save()
  ctx.translate(x, y)
  const s = size / 24
  ctx.scale(s, s)
  ctx.beginPath()
  ctx.arc(11, 11, 8, 0, Math.PI * 2)
  ctx.moveTo(21, 21)
  ctx.lineTo(16.65, 16.65)
  ctx.lineWidth = strokeWidth
  ctx.stroke()
  ctx.beginPath()
  ctx.moveTo(11, 7)
  ctx.lineTo(11, 15)
  ctx.moveTo(7, 11)
  ctx.lineTo(15, 11)
  ctx.stroke()
  ctx.restore()
}

function drawCycleIcon(ctx: CanvasRenderingContext2D, x: number, y: number, size: number, strokeWidth: number, accent: string): void {
  ctx.save()
  ctx.translate(x, y)
  const s = size / 24
  ctx.scale(s, s)
  ctx.strokeStyle = accent
  ctx.lineWidth = strokeWidth
  ctx.beginPath()
  ctx.arc(11, 11, 8, Math.PI * 0.5, Math.PI * 1.8)
  ctx.stroke()
  ctx.beginPath()
  ctx.moveTo(19, 10)
  ctx.lineTo(18, 15)
  ctx.lineTo(13.5, 12.5)
  ctx.stroke()
  ctx.restore()
}

function drawArrowHead(ctx: CanvasRenderingContext2D, x: number, y: number, angle: number, size: number, color: string): void {
  ctx.save()
  ctx.translate(x, y)
  ctx.rotate(angle)
  ctx.beginPath()
  ctx.moveTo(0, 0)
  ctx.lineTo(-size * 1.6, -size * 0.6)
  ctx.lineTo(-size * 1.6, size * 0.6)
  ctx.closePath()
  ctx.fillStyle = color
  ctx.fill()
  ctx.restore()
}

export function shouldDrawConnectorDetailLabel(direction: string | null | undefined, visualSourceScreenW: number, visualTargetScreenW: number): boolean {
  const dir = direction ?? 'forward'
  const sourceVisible = visualSourceScreenW > MIN_NODE_W_FOR_CONNECTOR_DETAIL
  const targetVisible = visualTargetScreenW > MIN_NODE_W_FOR_CONNECTOR_DETAIL

  if (dir === 'forward') return targetVisible
  if (dir === 'backward') return sourceVisible
  if (dir === 'both' || dir === 'bidirectional') return sourceVisible || targetVisible

  return sourceVisible || targetVisible
}

function drawGroupLabel(
  ctx: CanvasRenderingContext2D,
  group: DiagramGroupLayout,
  renderView: ZUIViewState,
  canvasW: number,
  canvasH: number,
  accent: string,
  lowDetail: boolean,
  originX?: number,
  originY?: number,
): void {
  if (lowDetail) return
  if (!group.isPortal && !group.label) return

  const ox = originX ?? 0
  const oy = originY ?? 0
  const { worldX, worldY, diagramX, diagramY, diagramW } = group

  if (!isVisible(
    worldX + diagramX - ox, worldY + diagramY - 30 - oy,
    diagramW, 30,
    renderView, canvasW, canvasH,
  )) return

  ctx.save()
  ctx.font = `600 ${14}px Inter, system-ui, sans-serif`

  const text = group.label
  const textW = ctx.measureText(text).width
  const padH = 10 / renderView.zoom
  const padV = 4 / renderView.zoom

  const labelX = worldX + diagramX + diagramW / 2 - ox
  const labelY = worldY + diagramY - 8 / renderView.zoom - oy

  const boxLeft = labelX - textW / 2 - padH
  const boxTop = labelY - 14 - padV
  const boxW = textW + padH * 2
  const boxH = 14 + padV * 2

  if (group.isPortal) {
    ctx.fillStyle = accent
    ctx.globalAlpha = 0.12
    ctx.beginPath()
    ctx.roundRect(boxLeft, boxTop, boxW, boxH, 3 / renderView.zoom)
    ctx.fill()

    ctx.globalAlpha = 0.9
    ctx.fillStyle = accent
    ctx.textAlign = 'center'
    ctx.textBaseline = 'middle'
    ctx.fillText(text, labelX, labelY)
  }

  ctx.restore()
}

function drawGrid(
  ctx: CanvasRenderingContext2D,
  view: ZUIViewState,
  canvasW: number,
  canvasH: number,
): void {
  const gridSize = 24
  const dotSize = 1.0

  // Calculate screen-space offsets using camera translation.
  // Since we want the grid to shift when panning but ignore zoom,
  // we shift by view.x and view.y modulo the gridSize.
  const offsetX = ((view.x % gridSize) + gridSize) % gridSize
  const offsetY = ((view.y % gridSize) + gridSize) % gridSize

  ctx.save()

  // Use a passive/subtle dot color based on the canvas background brightness
  const { canvasBg } = getThemeVars()
  const isLight = (() => {
    if (canvasBg && canvasBg.startsWith('#') && canvasBg.length === 7) {
      const r = parseInt(canvasBg.slice(1, 3), 16)
      const g = parseInt(canvasBg.slice(3, 5), 16)
      const b = parseInt(canvasBg.slice(5, 7), 16)
      const brightness = (r * 299 + g * 587 + b * 114) / 1000
      return brightness > 128
    }
    return false
  })()

  ctx.fillStyle = isLight ? 'rgba(0, 0, 0, 0.06)' : 'rgba(255, 255, 255, 0.06)'

  // Batch draw the dot grid in a single compound path to optimize canvas drawing performance.
  ctx.beginPath()
  for (let sx = offsetX - gridSize; sx < canvasW + gridSize; sx += gridSize) {
    for (let sy = offsetY - gridSize; sy < canvasH + gridSize; sy += gridSize) {
      ctx.moveTo(sx + dotSize, sy)
      ctx.arc(sx, sy, dotSize, 0, Math.PI * 2)
    }
  }
  ctx.fill()
  ctx.restore()
}

function drawSceneNode(
  ctx: CanvasRenderingContext2D,
  node: SceneNode,
  renderCtx: RenderContext,
  view: ZUIViewState,
  _transitionRebase: ZUITransitionRebase,
  navigationHints: NavigationHintDraw[],
): void {
  const state = node.state
  const layout = node.layout

  if (state.screenW < MIN_DRAW_PX || (state.parentAlpha < 0.01 && state.inheritedAlpha < 0.01)) return
  if (isHiddenByTags(layout)) return

  const x = 0
  const y = 0
  const w = layout.worldW
  const h = layout.worldH

  const drawZoom = state.drawZoom
  const drawScreenW = state.screenW
  const r = h * RADIUS_TO_NODE_H
  const { accent, nodeBg, canvasBg, canvasW, canvasH } = renderCtx

  const hasChildren = layout.children.length > 0
  const t = state.t

  if (state.isLeafCapped) {
    const cx = x + w / 2
    const cy = y + h / 2
    ctx.translate(cx, cy)
    ctx.scale(state.leafCapScale, state.leafCapScale)
    ctx.translate(-cx, -cy)
  }

  const parentAlpha = state.parentAlpha
  const inheritedAlpha = state.inheritedAlpha

  const borderColor = typeBorderColor(layout.type)

  const traceShape = (ox = 0, oy = 0) => {
    ctx.beginPath()
    ctx.roundRect(x + ox, y + oy, w, h, r)
  }

  if (layout.isCircular && parentAlpha > 0.1) {
    ctx.save()
    ctx.globalAlpha = parentAlpha * 0.15
    ctx.fillStyle = accent
    traceShape()
    ctx.fill()
    ctx.restore()
  }

  if (hasChildren && parentAlpha > 0.1 && t < 0.5) {
    const stackT = 1 - (t / 0.5)
    ctx.save()
    ctx.globalAlpha = parentAlpha * stackT * 0.4
    ctx.fillStyle = nodeBg
    ctx.strokeStyle = borderColor
    ctx.lineWidth = 1 / drawZoom

    const offset1 = 4 / drawZoom
    const offset2 = 8 / drawZoom

    traceShape(offset2, offset2)
    ctx.fill()
    ctx.stroke()

    traceShape(offset1, offset1)
    ctx.fill()
    ctx.stroke()
    ctx.restore()
  }

  if (parentAlpha > 0.5 && state.screenW > 40) {
    ctx.save()
    ctx.globalAlpha = parentAlpha * 0.4
    ctx.shadowColor = 'rgba(0, 0, 0, 0.5)'
    ctx.shadowBlur = 12 / drawZoom
    ctx.shadowOffsetY = 4 / drawZoom
    traceShape()
    ctx.fill()
    ctx.restore()
  }

  if (parentAlpha > 0.01) {
    ctx.save()
    traceShape()

    ctx.globalAlpha = parentAlpha * 0.8
    ctx.fillStyle = canvasBg
    ctx.fill()

    ctx.fillStyle = nodeBg
    ctx.fill()

    if (layout.isPortal) {
      ctx.fillStyle = portalTintColor(accent, 0.10)
      ctx.fill()
    }

    ctx.restore()
  }

  if (layout.logoUrl && parentAlpha > 0.05 && drawScreenW > 60) {
    const img = getOrLoadImage(layout.logoUrl)
    if (img) {
      ctx.save()
      ctx.globalAlpha = parentAlpha * 1

      const logoMaxDim = h * 0.35
      const topOffset = h * 0.06

      const intrinsicW = img.naturalWidth || img.width
      const intrinsicH = img.naturalHeight || img.height
      const aspect = intrinsicH > 0 ? intrinsicW / intrinsicH : 1
      let drawW = logoMaxDim
      let drawH = drawW / aspect

      if (drawH > logoMaxDim) {
        drawH = logoMaxDim
        drawW = drawH * aspect
      }

      const iconX = x + (w - drawW) / 2
      const iconY = y + topOffset + (logoMaxDim - drawH) / 2

      ctx.drawImage(img, iconX, iconY, drawW, drawH)
      ctx.restore()
    }
  }

  if (inheritedAlpha > 0.01) {
    ctx.save()
    ctx.globalAlpha = inheritedAlpha
    traceShape()
    if (layout.isPortal) {
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
  }

  if (!renderCtx.lowDetail && state.screenW >= MIN_LABEL_PX && parentAlpha > 0.1) {
    const nameFontSize = h * NAME_FONT_TO_NODE_H
    const screenFontSize = nameFontSize * drawZoom

    if (screenFontSize >= 6) {
      ctx.save()
      ctx.globalAlpha = parentAlpha
      ctx.font = `600 ${nameFontSize}px Inter, system-ui, sans-serif`
      ctx.fillStyle = '#f7fafc'
      ctx.textAlign = 'center'
      ctx.textBaseline = 'middle'

      const worldPadding = w * 0.08
      const maxW = w - worldPadding
      let label = layout.label
      const totalW = ctx.measureText(label).width
      if (totalW > maxW) {
        const ratio = maxW / totalW
        label = label.slice(0, Math.max(3, Math.floor(label.length * ratio)))
        if (label.length < layout.label.length) label += '\u2026'
      }

      const showLogo = !!layout.logoUrl && drawScreenW > 60
      const baseOffset = showLogo ? 0.15 : 0
      const nameY = drawScreenW > BADGE_THRESHOLD ? y + h * (0.42 + baseOffset) : y + h * (0.5 + baseOffset)
      ctx.fillText(label, x + w / 2, nameY)

      if (drawScreenW > BADGE_THRESHOLD) {
        const badgeFontSize = h * TYPE_FONT_TO_NODE_H
        if (badgeFontSize * drawZoom >= 5) {
          ctx.font = `${badgeFontSize}px Inter, system-ui, sans-serif`
          ctx.fillStyle = '#a0aec0'
          const displayType = typeof layout.type === 'string' ? layout.type.toUpperCase() : 'UNKNOWN'
          ctx.fillText(displayType, x + w / 2, y + h * (0.62 + baseOffset))
        }
      }
      ctx.restore()
    }
  }

  if (!renderCtx.lowDetail && layout.linkedDiagramLabel && t > 0.05 && inheritedAlpha > 0.05) {
    const hintFontSize = getClampedFontSize(14, MIN_FONT_HINT, MAX_FONT_HINT, drawZoom)
    const screenFontSize = hintFontSize * drawZoom

    if (screenFontSize >= 6) {
      const hintPrefix = layout.isCircular ? '\u21ba ' : '\u203a '
      const hintSuffix = layout.isCircular ? ' (Circular)' : ''
      const hintText = hintPrefix + layout.linkedDiagramLabel + hintSuffix

      ctx.save()
      ctx.font = `${hintFontSize}px Inter, system-ui, sans-serif`
      const tw = ctx.measureText(hintText).width
      ctx.restore()

      const baseHintX = x + w / 2
      let baseHintY = y + h + 10
      let clampedHintX = baseHintX

      if (t > 0.8) {
        const m = ctx.getTransform()
        const dp = m.a / drawZoom
        const vwLocalB = ((canvasH - screenFontSize) * dp - m.f) / m.d
        baseHintY = Math.min(baseHintY, vwLocalB)
        baseHintY = Math.max(baseHintY, y + h / 2)

        const vwLocalL = -m.e / m.a
        const vwLocalR = (canvasW * dp - m.e) / m.a

        const pad = 30 / drawZoom
        clampedHintX = Math.max(clampedHintX, vwLocalL + tw / 2 + pad)
        clampedHintX = Math.min(clampedHintX, vwLocalR - tw / 2 - pad)
        clampedHintX = clamp(clampedHintX, x + tw / 2 + 10, x + w - tw / 2 - 10)
      }

      const lineH = hintFontSize * 1.2
      const gap = 10 / drawZoom
      const sideGap = 14 / drawZoom
      const lowerGap = lineH + gap
      navigationHints.push({
        matrix: ctx.getTransform(),
        candidates: [
          { x: clampedHintX, y: baseHintY },
          { x: clampedHintX, y: y - lineH - gap },
          { x: clampedHintX, y: baseHintY + lowerGap },
          { x: x + w + tw / 2 + sideGap, y: y + h / 2 - lineH / 2 },
          { x: x - tw / 2 - sideGap, y: y + h / 2 - lineH / 2 },
        ],
        text: hintText,
        textW: tw,
        textH: lineH,
        fontSize: hintFontSize,
        color: layout.isCircular ? accent : '#718096',
        alpha: inheritedAlpha * 0.7,
        priority: layout.pathElementIds.length,
      })
    }
  }

  if (!renderCtx.lowDetail && (hasChildren || layout.isCircular) && t < 0.9 && parentAlpha > 0.2 && drawScreenW > BADGE_THRESHOLD) {
    const iconSize = getClampedFontSize(12, 10, 16, drawZoom)
    const padding = 8 / drawZoom

    ctx.save()
    ctx.globalAlpha = parentAlpha * (1 - t) * 0.8
    ctx.strokeStyle = accent
    if (layout.isCircular) {
      drawCycleIcon(ctx, x + w - iconSize - padding, y + padding, iconSize, 3.5, accent)
    } else {
      drawZoomInIcon(ctx, x + w - iconSize - padding, y + padding, iconSize, 2.5)
    }
    ctx.restore()
  }

  if (currentHighlightedTags.size > 0 && parentAlpha > 0.05) {
    const isHighlighted = layout.tags.length > 0 && layout.tags.some((t2) => currentHighlightedTags.has(t2))
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

  if ((currentVersionElementChanges.size > 0 || currentVersionConnectorChanges.size > 0) && parentAlpha > 0.05) {
    const change = currentVersionElementChanges.get(layout.elementId)
    if (!change) {
      ctx.save()
      const isContext = currentDiffLensActive && currentDiffContextElementIds.has(layout.elementId)
      ctx.globalAlpha = parentAlpha * (isContext ? 0.45 : 0.9)
      ctx.fillStyle = canvasBg
      traceShape()
      ctx.fill()
      if (isContext && drawScreenW > 40) {
        ctx.globalAlpha = parentAlpha * 0.55
        ctx.strokeStyle = 'rgba(255, 255, 255, 0.18)'
        ctx.lineWidth = 1.5 / drawZoom
        ctx.setLineDash([4 / drawZoom, 4 / drawZoom])
        traceShape()
        ctx.stroke()
        ctx.setLineDash([])
      }
      ctx.restore()
    } else {
      const color = change === 'added' ? '#68d391' : change === 'deleted' ? '#fc8181' : '#f6e05e'
      ctx.save()
      ctx.globalAlpha = parentAlpha
      ctx.shadowColor = color
      ctx.shadowBlur = 8 / drawZoom
      ctx.strokeStyle = color
      ctx.lineWidth = 2.5 / drawZoom
      traceShape()
      ctx.stroke()
      ctx.restore()
    }
  }

  const delta = currentVersionElementLineDeltas.get(layout.elementId)
  if (!renderCtx.lowDetail && delta && (delta.added > 0 || delta.removed > 0) && drawScreenW > 52 && parentAlpha > 0.05) {
    const addText = delta.added > 0 ? `+${delta.added}` : ''
    const removeText = delta.removed > 0 ? `-${delta.removed}` : ''
    const badgeText = [addText, removeText].filter(Boolean).join(' ')
    const fontSize = getClampedFontSize(12, 8, 13, drawZoom)
    ctx.save()
    ctx.globalAlpha = parentAlpha
    ctx.font = `800 ${fontSize}px Inter, system-ui, sans-serif`
    const textWidth = ctx.measureText(badgeText).width
    const badgeW = textWidth + 12 / drawZoom
    const badgeH = 20 / drawZoom
    const badgeX = x + w - badgeW - 6 / drawZoom
    const badgeY = y + h - badgeH - 6 / drawZoom
    ctx.fillStyle = 'rgba(17, 24, 39, 0.9)'
    ctx.strokeStyle = 'rgba(255, 255, 255, 0.22)'
    ctx.lineWidth = 1 / drawZoom
    ctx.beginPath()
    ctx.roundRect(badgeX, badgeY, badgeW, badgeH, 5 / drawZoom)
    ctx.fill()
    ctx.stroke()
    ctx.textAlign = 'center'
    ctx.textBaseline = 'middle'
    ctx.fillStyle = delta.added > 0 && delta.removed === 0 ? '#68d391' : delta.removed > 0 && delta.added === 0 ? '#fc8181' : '#e2e8f0'
    ctx.fillText(badgeText, badgeX + badgeW / 2, badgeY + badgeH / 2)
    ctx.restore()
  }

  if (state.isLeafCapped) {
    ctx.restore()
  }
}

interface HandleUsage {
  edgeKey: string
  type: 'source' | 'target'
  otherNodeCoord: number
}

interface DrawEdgesLayoutMetadata {
  nodeMap: Map<string, LayoutNode>
  sceneNodeMap: Map<string, SceneNode>
  handleUsage: Record<string, HandleUsage[]>
  handleUsageIndex: Record<string, number>
}

const drawEdgesMetadataCache = new WeakMap<SceneNode[], DrawEdgesLayoutMetadata>()
const emptyHandleUsage: HandleUsage[] = []

function getDrawEdgesLayoutMetadata(sceneNodes: SceneNode[]): DrawEdgesLayoutMetadata {
  const cached = drawEdgesMetadataCache.get(sceneNodes)
  if (cached) return cached

  const nodeMap = new Map<string, LayoutNode>()
  const sceneNodeMap = new Map<string, SceneNode>()
  const handleUsage: Record<string, HandleUsage[]> = {}
  const handleUsageIndex: Record<string, number> = {}

  for (const sceneNode of sceneNodes) {
    nodeMap.set(sceneNode.layout.id, sceneNode.layout)
    sceneNodeMap.set(sceneNode.layout.id, sceneNode)
  }

  for (const sceneNode of sceneNodes) {
    const node = sceneNode.layout
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

  const metadata = { nodeMap, sceneNodeMap, handleUsage, handleUsageIndex }
  drawEdgesMetadataCache.set(sceneNodes, metadata)
  return metadata
}

function nodeSelfAlphaFromState(node: SceneNode): number {
  if (node.layout.children.length === 0) return 1
  return 1 - node.state.t
}

export function nodeConnectorEndpointAlphaFromState(node: SceneNode): number {
  const selfAlpha = nodeSelfAlphaFromState(node)
  if (node.layout.children.length === 0 || node.state.inheritedAlpha <= 0.01) return selfAlpha
  return Math.max(selfAlpha, Math.min(node.state.inheritedAlpha, CONNECTOR_ENDPOINT_MIN_ALPHA))
}

function getHandlePos(
  nodeX: number, nodeY: number, nodeW: number, nodeH: number,
  handleId: string | null, isSource: boolean, slotScale = 1,
): { x: number; y: number; pos: 'top' | 'bottom' | 'left' | 'right' } {
  const fallback = isSource ? DEFAULT_SOURCE_HANDLE_SIDE : DEFAULT_TARGET_HANDLE_SIDE
  if (slotScale === 1) {
    const { x, y, side } = getHandleFlowPosition(nodeX, nodeY, nodeW, nodeH, handleId, fallback)
    return { x, y, pos: side }
  }

  const side = getLogicalHandleId(handleId, fallback) ?? fallback
  const offset = getHandleSlotOffsetFromId(handleId) * slotScale
  switch (side) {
    case 'top':
      return { x: nodeX + nodeW / 2 + offset, y: nodeY, pos: side }
    case 'bottom':
      return { x: nodeX + nodeW / 2 + offset, y: nodeY + nodeH, pos: side }
    case 'left':
      return { x: nodeX, y: nodeY + nodeH / 2 + offset, pos: side }
    case 'right':
      return { x: nodeX + nodeW, y: nodeY + nodeH / 2 + offset, pos: side }
  }
}

function drawEdges(
  ctx: CanvasRenderingContext2D,
  sceneNodes: SceneNode[],
  alpha: number,
  zoom: number,
  thresholds: { start: number; end: number },
  accent: string,
  labelBg: string,
  occupiedLabelRects: ScreenRect[],
  lowDetail: boolean,
  originX?: number,
  originY?: number,
): void {
  if (alpha < 0.05) return

  const ox = originX ?? 0
  const oy = originY ?? 0

  const { nodeMap, sceneNodeMap, handleUsage, handleUsageIndex } = getDrawEdgesLayoutMetadata(sceneNodes)

  for (const sn of sceneNodes) {
    const node = sn.layout
    for (const [edgeIndex, edge] of node.edgesOut.entries()) {
      const target = nodeMap.get(edge.targetId)
      if (!target) continue

      const targetSn = sceneNodeMap.get(edge.targetId)

      if (currentHiddenTags.size > 0) {
        const srcHidden = node.tags.length > 0 && node.tags.some((t2) => currentHiddenTags.has(t2))
        const tgtHidden = target.tags.length > 0 && target.tags.some((t2) => currentHiddenTags.has(t2))
        if (srcHidden || tgtHidden) continue
      }

      const endpointAlphaFactor = Math.min(
        nodeConnectorEndpointAlphaFromState(sn),
        targetSn ? nodeConnectorEndpointAlphaFromState(targetSn) : 1,
      )
      const edgeAlpha = alpha * endpointAlphaFactor
      if (edgeAlpha < 0.05) continue

      const dir = edge.direction ?? 'forward'
      const type = normalizeEdgeRouteType(edge.type)

      const hasSourceChildren = node.children && node.children.length > 0
      const sourceScreenW = node.worldW * zoom
      const sSource = (!hasSourceChildren && sourceScreenW > thresholds.end) ? thresholds.end / sourceScreenW : 1
      const effWSource = node.worldW * sSource
      const effHSource = node.worldH * sSource
      const cxSource = node.worldX + node.worldW / 2 - ox
      const cySource = node.worldY + node.worldH / 2 - oy
      const effXSource = cxSource - effWSource / 2
      const effYSource = cySource - effHSource / 2

      const hasTargetChildren = target.children && target.children.length > 0
      const targetScreenW = target.worldW * zoom
      const sTarget = (!hasTargetChildren && targetScreenW > thresholds.end) ? thresholds.end / targetScreenW : 1
      const effWTarget = target.worldW * sTarget
      const effHTarget = target.worldH * sTarget
      const cxTarget = target.worldX + target.worldW / 2 - ox
      const cyTarget = target.worldY + target.worldH / 2 - oy
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
        effXSource, effYSource, effWSource, effHSource,
        getVisualHandleIdForGroup(sourceSide, sourceGroupIndex, Math.max(srcGroup.length, 1)),
        true, sSource,
      )
      const tH = getHandlePos(
        effXTarget, effYTarget, effWTarget, effHTarget,
        getVisualHandleIdForGroup(targetSide, targetGroupIndex, Math.max(tgtGroup.length, 1)),
        false, sTarget,
      )

      ctx.save()
      const edgeChange = currentVersionConnectorChanges.get(edge.id)
      const versionPreviewActive = currentVersionElementChanges.size > 0 || currentVersionConnectorChanges.size > 0
      const edgeContext = currentDiffLensActive && (
        currentDiffContextConnectorIds.has(edge.id) ||
        currentDiffContextElementIds.has(node.elementId) ||
        currentDiffContextElementIds.has(target.elementId) ||
        currentVersionElementChanges.has(node.elementId) ||
        currentVersionElementChanges.has(target.elementId)
      )
      ctx.globalAlpha = versionPreviewActive && !edgeChange
        ? edgeContext
          ? Math.max(edgeAlpha * 0.28, 0.12 * endpointAlphaFactor)
          : Math.max(edgeAlpha * 0.08, 0.04 * endpointAlphaFactor)
        : connectorAlpha(edgeAlpha, CONNECTOR_MIN_ALPHA * endpointAlphaFactor)
      ctx.strokeStyle = edgeChange === 'added'
        ? '#68d391'
        : edgeChange === 'deleted'
          ? '#fc8181'
          : edgeChange
            ? '#f6e05e'
            : accent
      ctx.lineWidth = (edgeChange ? CONNECTOR_LINE_PX * 1.35 : CONNECTOR_LINE_PX) / zoom

      let midX = (sH.x + tH.x) / 2
      let midY = (sH.y + tH.y) / 2
      let finalAngleS = 0
      let finalAngleT = 0

      if (type === 'bezier') {
        const curvature = 0.5
        let cp1x = sH.x, cp1y = sH.y, cp2x = tH.x, cp2y = tH.y
        const dx = Math.abs(tH.x - sH.x)
        const dy = Math.abs(tH.y - sH.y)

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

        const points = stepRoutePoints(
          sH.x,
          sH.y,
          tH.x,
          tH.y,
          positionForHandleSide(sH.pos),
          positionForHandleSide(tH.pos),
          effWSource,
          effHSource,
          effWTarget,
          effHTarget,
        )

        if (points.length > 1) {
          let maxLen = -1
          let bestIdx = 0
          const startIdx = points.length >= 4 ? 1 : 0
          const endIdx = points.length >= 4 ? points.length - 2 : points.length - 1

          for (let i = startIdx; i < endIdx; i++) {
            const p1 = points[i]
            const p2 = points[i + 1]
            const len = Math.hypot(p2.x - p1.x, p2.y - p1.y)
            if (len > maxLen) {
              maxLen = len
              bestIdx = i
            }
          }
          const p1 = points[bestIdx]
          const p2 = points[bestIdx + 1]
          midX = (p1.x + p2.x) / 2
          midY = (p1.y + p2.y) / 2
        }

        ctx.beginPath()
        ctx.moveTo(points[0].x, points[0].y)

        for (let i = 1; i < points.length; i++) {
          const curr = points[i]
          const prev = points[i - 1]
          const next = points[i + 1]

          if (borderRadius > 0 && next) {
            const dPrevX = curr.x - prev.x
            const dPrevY = curr.y - prev.y
            const dPrevLen = Math.sqrt(dPrevX * dPrevX + dPrevY * dPrevY)
            const r = Math.min(borderRadius, dPrevLen / 2)

            ctx.lineTo(curr.x - (dPrevX / dPrevLen) * r, curr.y - (dPrevY / dPrevLen) * r)

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

        const last = points[points.length - 1]
        const prev = points[points.length - 2]
        finalAngleT = Math.atan2(last.y - prev.y, last.x - prev.x)

        const first = points[0]
        const firstNext = points[1]
        finalAngleS = Math.atan2(first.y - firstNext.y, first.x - firstNext.x)
      }

      const visualTargetScreenW = effWTarget * zoom
      const visualSourceScreenW = effWSource * zoom

      const ARROW_SIZE_BASE = 10
      const sourceConnectorDetailVisible = visualSourceScreenW > MIN_NODE_W_FOR_CONNECTOR_DETAIL
      const targetConnectorDetailVisible = visualTargetScreenW > MIN_NODE_W_FOR_CONNECTOR_DETAIL

      if (dir === 'forward' || dir === 'both' || dir === 'bidirectional') {
        if (targetConnectorDetailVisible) {
          const arrowScreenSize = Math.min(ARROW_SIZE_BASE, visualTargetScreenW * 0.2)
          drawArrowHead(ctx, tH.x, tH.y, finalAngleT, arrowScreenSize / zoom, accent)
        }
      }
      if (dir === 'backward' || dir === 'both' || dir === 'bidirectional') {
        if (sourceConnectorDetailVisible) {
          const arrowScreenSize = Math.min(ARROW_SIZE_BASE, visualSourceScreenW * 0.2)
          drawArrowHead(ctx, sH.x, sH.y, finalAngleS, arrowScreenSize / zoom, accent)
        }
      }

      if (!lowDetail && edge.label && shouldDrawConnectorDetailLabel(dir, visualSourceScreenW, visualTargetScreenW)) {
        const screenFontSize = 12
        const worldFontSize = screenFontSize / zoom
        ctx.font = `${worldFontSize}px Inter, system-ui, sans-serif`
        ctx.fillStyle = '#cbd5e0'
        ctx.textAlign = 'center'
        ctx.textBaseline = 'middle'

        const textW = ctx.measureText(edge.label).width
        const padX = 8 / zoom
        const padY = 4 / zoom
        const labelW = textW + padX * 2
        const labelH = worldFontSize * 1.2 + padY * 2

        const labelCenter = pickEdgeLabelPosition(
          ctx.getTransform(),
          midX, midY,
          labelW, labelH,
          tH.x - sH.x,
          tH.y - sH.y,
          occupiedLabelRects,
        )
        const labelRect = edgeLabelDrawRectFromCenter(labelCenter, labelW, labelH)

        ctx.fillStyle = labelBg
        ctx.globalAlpha = Math.min(edgeAlpha, connectorAlpha(edgeAlpha * 1.1))
        ctx.beginPath()
        ctx.roundRect(labelRect.x, labelRect.y, labelRect.width, labelRect.height, 4 / zoom)
        ctx.fill()

        ctx.globalAlpha = edgeAlpha
        ctx.fillStyle = '#cbd5e0'
        ctx.fillText(edge.label, labelCenter.x, labelCenter.y)
      }

      ctx.restore()
    }
  }
}

function drawNodeTree(
  ctx: CanvasRenderingContext2D,
  node: SceneNode,
  renderCtx: RenderContext,
  view: ZUIViewState,
  transitionRebase: ZUITransitionRebase,
  effectiveZoom: number,
  occupiedLabelRects: ScreenRect[],
  navigationHints: NavigationHintDraw[],
  dpr: number,
): void {
  const nodeScreenX = dpr * (renderCtx.canvasW / 2 + node.state.worldX * view.zoom)
  const nodeScreenY = dpr * (renderCtx.canvasH / 2 + node.state.worldY * view.zoom)
  const capScale = node.state.isLeafCapped ? Math.max(node.state.leafCapScale, 0.0001) : 1
  const baseDrawZoom = node.state.drawZoom / capScale

  ctx.save()
  ctx.setTransform(dpr * baseDrawZoom, 0, 0, dpr * baseDrawZoom, nodeScreenX, nodeScreenY)

  drawSceneNode(ctx, node, renderCtx, view, transitionRebase, navigationHints)

  if (node.layout.children.length > 0 && node.state.childAlpha > 0.01) {
    const childScale = node.layout.childScale > 0 ? node.layout.childScale : 1
    const childZoom = effectiveZoom * childScale

    ctx.save()

    const r = node.layout.worldH * RADIUS_TO_NODE_H
    ctx.beginPath()
    ctx.roundRect(0, 0, node.layout.worldW, node.layout.worldH, r)
    ctx.clip()

    ctx.scale(childScale, childScale)
    ctx.translate(-node.layout.childOffsetX, -node.layout.childOffsetY)

    if (node.state.childAlpha > 0.2) {
      drawEdges(ctx, node.children, node.state.childAlpha * 0.8, childZoom, renderCtx.thresholds, renderCtx.accent, renderCtx.labelBg, occupiedLabelRects, renderCtx.lowDetail)
    }

    for (const child of node.children) {
      if (!child.state.isVisible) continue
      drawNodeTree(ctx, child, renderCtx, view, transitionRebase, childZoom, occupiedLabelRects, navigationHints, dpr)
    }

    ctx.restore()
  }

  ctx.restore()
}

export function renderFrame(
  ctx: CanvasRenderingContext2D,
  graph: SceneGraph,
  renderCtx: RenderContext,
  view: ZUIViewState,
  transitionRebase: ZUITransitionRebase,
): ScreenRect[] {
  const { canvasBg, canvasW, canvasH, accent } = renderCtx

  ctx.clearRect(0, 0, canvasW, canvasH)

  ctx.fillStyle = canvasBg
  ctx.fillRect(0, 0, canvasW, canvasH)

  drawGrid(ctx, view, canvasW, canvasH)

  const rebase = getCameraRebase(view, canvasW, canvasH)
  const renderView = rebase.view
  const dpr = ctx.getTransform().a

  ctx.save()
  ctx.translate(renderView.x, renderView.y)
  ctx.scale(renderView.zoom, renderView.zoom)

  const occupiedLabelRects = frameLabelRects
  occupiedLabelRects.length = 0
  const navigationHints: NavigationHintDraw[] = []

  for (const group of graph.groups) {
    if (!group.isVisible) continue

    drawGroupLabel(ctx, group.layout, renderView, canvasW, canvasH, accent, renderCtx.lowDetail, rebase.originX, rebase.originY)

    const borderAlpha = clamp(0.5 - renderView.zoom * 0.05, 0.15, 0.5)

    ctx.save()
    ctx.globalAlpha = borderAlpha
    ctx.strokeStyle = accent
    ctx.lineWidth = 2 / renderView.zoom
    ctx.setLineDash([2, 2])
    ctx.strokeRect(
      group.layout.worldX + group.layout.diagramX - rebase.originX,
      group.layout.worldY + group.layout.diagramY - rebase.originY,
      group.layout.diagramW,
      group.layout.diagramH,
    )
    ctx.setLineDash([])
    ctx.restore()

    drawEdges(ctx, group.nodes, 0.7, renderView.zoom, renderCtx.thresholds, accent, renderCtx.labelBg, occupiedLabelRects, renderCtx.lowDetail, rebase.originX, rebase.originY)

    for (const node of group.nodes) {
      if (!node.state.isVisible) continue
      drawNodeTree(ctx, node, renderCtx, view, transitionRebase, renderView.zoom, occupiedLabelRects, navigationHints, dpr)
    }
  }

  if (!renderCtx.lowDetail) {
    drawNavigationHints(ctx, navigationHints, occupiedLabelRects)
  }

  ctx.restore()
  return occupiedLabelRects
}
