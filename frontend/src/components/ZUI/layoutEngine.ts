import type { SceneGraph, SceneNode } from './sceneGraph'
import type { ZUIViewState } from './types'

export interface ZUICameraRebase {
  originX: number
  originY: number
  view: ZUIViewState
}

export interface ZUITransitionRebase {
  preserveChildAlphaNodeIds: Set<string>
}

function clamp(v: number, min: number, max: number): number {
  return v < min ? min : v > max ? max : v
}

export function transitionT(screenW: number, start: number, end: number): number {
  return clamp((screenW - start) / (end - start), 0, 1)
}

export function getExpandThresholds(canvasW: number) {
  return {
    start: clamp(canvasW * 0.25, 80, 450),
    end: clamp(canvasW * 0.4, 200, 640),
  }
}

export function viewOriginX(view: ZUIViewState): number {
  return view.originX ?? 0
}

export function viewOriginY(view: ZUIViewState): number {
  return view.originY ?? 0
}

export function screenToWorldX(screenX: number, view: ZUIViewState): number {
  return viewOriginX(view) + (screenX - view.x) / view.zoom
}

export function screenToWorldY(screenY: number, view: ZUIViewState): number {
  return viewOriginY(view) + (screenY - view.y) / view.zoom
}

export function worldToScreenX(worldX: number, view: ZUIViewState): number {
  return (worldX - viewOriginX(view)) * view.zoom + view.x
}

export function worldToScreenY(worldY: number, view: ZUIViewState): number {
  return (worldY - viewOriginY(view)) * view.zoom + view.y
}

export function rawCameraView(view: ZUIViewState): ZUIViewState {
  return {
    x: view.x - viewOriginX(view) * view.zoom,
    y: view.y - viewOriginY(view) * view.zoom,
    zoom: view.zoom,
  }
}

export function isVisible(
  worldX: number, worldY: number, worldW: number, worldH: number,
  view: ZUIViewState, canvasW: number, canvasH: number,
): boolean {
  const sx = worldToScreenX(worldX, view)
  const sy = worldToScreenY(worldY, view)
  const sw = worldW * view.zoom
  const sh = worldH * view.zoom
  return sx + sw > 0 && sy + sh > 0 && sx < canvasW && sy < canvasH
}

export function isFullyVisible(
  worldX: number, worldY: number, worldW: number, worldH: number,
  view: ZUIViewState, canvasW: number, canvasH: number,
): boolean {
  const sx = worldToScreenX(worldX, view)
  const sy = worldToScreenY(worldY, view)
  const sw = worldW * view.zoom
  const sh = worldH * view.zoom
  return sx >= 0 && sy >= 0 && sx + sw <= canvasW && sy + sh <= canvasH
}

export function getCameraRebase(view: ZUIViewState, canvasW: number, canvasH: number): ZUICameraRebase {
  const zoom = Math.max(0.0001, view.zoom)
  return {
    originX: screenToWorldX(canvasW / 2, { ...view, zoom }),
    originY: screenToWorldY(canvasH / 2, { ...view, zoom }),
    view: {
      x: canvasW / 2,
      y: canvasH / 2,
      zoom: view.zoom,
    },
  }
}

function buildCameraTransitionRebase(
  graph: SceneGraph,
  view: ZUIViewState,
  canvasW: number,
  canvasH: number,
  thresholds: { start: number; end: number },
): ZUITransitionRebase {
  if (canvasW <= 0 || canvasH <= 0 || view.zoom <= 0) {
    return { preserveChildAlphaNodeIds: new Set() }
  }

  const worldCenterX = screenToWorldX(canvasW / 2, view)
  const worldCenterY = screenToWorldY(canvasH / 2, view)
  const path: Array<{ id: string; t: number }> = []

  for (const group of graph.groups) {
    if (
      worldCenterX < group.layout.worldX ||
      worldCenterX > group.layout.worldX + group.layout.worldW ||
      worldCenterY < group.layout.worldY ||
      worldCenterY > group.layout.worldY + group.layout.worldH
    ) {
      continue
    }

    let currentX = worldCenterX
    let currentY = worldCenterY
    let currentNodes = group.nodes
    let cumulativeScale = 1

    while (true) {
      const match = currentNodes.find((candidate) =>
        currentX >= candidate.layout.worldX &&
        currentX <= candidate.layout.worldX + candidate.layout.worldW &&
        currentY >= candidate.layout.worldY &&
        currentY <= candidate.layout.worldY + candidate.layout.worldH
      )

      if (!match) break

      const hasChildren = match.layout.children.length > 0
      const screenW = match.layout.worldW * view.zoom * cumulativeScale
      const t = hasChildren ? transitionT(screenW, thresholds.start, thresholds.end) : 0
      path.push({ id: match.layout.id, t })

      if (!hasChildren || t <= 0.05 || match.layout.childScale <= 0) break

      currentX = (currentX - match.layout.worldX) / match.layout.childScale + match.layout.childOffsetX
      currentY = (currentY - match.layout.worldY) / match.layout.childScale + match.layout.childOffsetY
      currentNodes = match.children
      cumulativeScale *= match.layout.childScale
    }

    break
  }

  const activeTransitionIndexes = path
    .map((entry, index) => ({ ...entry, index }))
    .filter((entry) => entry.t > 0.05 && entry.t < 0.95)

  if (activeTransitionIndexes.length <= 1) {
    return { preserveChildAlphaNodeIds: new Set() }
  }

  const deepestActiveIndex = activeTransitionIndexes[activeTransitionIndexes.length - 1].index
  return {
    preserveChildAlphaNodeIds: new Set(path.slice(0, deepestActiveIndex).map((entry) => entry.id)),
  }
}

function updateNodeStateRecursive(
  node: SceneNode,
  parentAbsScale: number,
  originX: number,
  originY: number,
  zoom: number,
  canvasCenterX: number,
  canvasCenterY: number,
  canvasW: number,
  canvasH: number,
  thresholds: { start: number; end: number },
  inheritedAlpha: number,
  preserveChildAlphaNodeIds: ReadonlySet<string>,
  parentAbsX: number,
  parentAbsY: number,
  parentChildOffsetX: number,
  parentChildOffsetY: number,
): void {
  const { layout } = node
  inheritedAlpha *= layout.transitionOpacity ?? 1

  const absX = parentAbsX + (layout.worldX - parentChildOffsetX) * parentAbsScale
  const absY = parentAbsY + (layout.worldY - parentChildOffsetY) * parentAbsScale
  const absW = layout.worldW * parentAbsScale
  const absH = layout.worldH * parentAbsScale
  const screenW = absW * zoom

  const rebasedWorldX = absX - originX
  const rebasedWorldY = absY - originY

  const screenXCheck = rebasedWorldX * zoom + canvasCenterX
  const screenYCheck = rebasedWorldY * zoom + canvasCenterY

  const visible = screenXCheck + screenW > 0 && screenYCheck + absH * zoom > 0 && screenXCheck < canvasW && screenYCheck < canvasH

  const hasChildren = layout.children.length > 0
  const t = hasChildren ? transitionT(screenW, thresholds.start, thresholds.end) : 0

  const parentAlpha = inheritedAlpha * (1 - t)
  const childAlpha = preserveChildAlphaNodeIds.has(layout.id) ? inheritedAlpha : inheritedAlpha * t

  let drawZoom = zoom * parentAbsScale
  let drawScreenW = screenW
  let isLeafCapped = false
  let leafCapScale = 1

  if (!hasChildren && screenW > thresholds.end) {
    leafCapScale = thresholds.end / screenW
    drawZoom = drawZoom * leafCapScale
    drawScreenW = thresholds.end
    isLeafCapped = true
  }

  const state = node.state
  state.worldX = rebasedWorldX
  state.worldY = rebasedWorldY
  state.screenW = drawScreenW
  state.drawZoom = drawZoom
  state.parentAlpha = parentAlpha
  state.childAlpha = childAlpha
  state.inheritedAlpha = inheritedAlpha
  state.t = t
  state.isVisible = visible
  state.isLeafCapped = isLeafCapped
  state.leafCapScale = leafCapScale

  if (childAlpha > 0.01 && layout.children.length > 0) {
    const childAbsScale = parentAbsScale * (layout.childScale > 0 ? layout.childScale : 1)

    for (const child of node.children) {
      updateNodeStateRecursive(
        child,
        childAbsScale,
        originX, originY, zoom, canvasCenterX, canvasCenterY,
        canvasW, canvasH,
        thresholds,
        childAlpha,
        preserveChildAlphaNodeIds,
        absX, absY,
        layout.childOffsetX,
        layout.childOffsetY,
      )
    }
  }
}

export function updateScene(
  graph: SceneGraph,
  view: ZUIViewState,
  canvasW: number,
  canvasH: number,
  thresholds: { start: number; end: number },
): ZUITransitionRebase {
  const rebase = getCameraRebase(view, canvasW, canvasH)
  const originX = rebase.originX
  const originY = rebase.originY
  const zoom = rebase.view.zoom
  const canvasCenterX = canvasW / 2
  const canvasCenterY = canvasH / 2

  const transitionRebase = buildCameraTransitionRebase(graph, view, canvasW, canvasH, thresholds)
  const preserveIds = transitionRebase.preserveChildAlphaNodeIds

  for (const group of graph.groups) {
    const { layout } = group
    const gsx = (layout.worldX - originX) * zoom + canvasCenterX
    const gsy = (layout.worldY - originY) * zoom + canvasCenterY
    const gsw = layout.worldW * zoom
    const gsh = layout.worldH * zoom
    group.isVisible = gsx + gsw > 0 && gsy + gsh > 0 && gsx < canvasW && gsy < canvasH
    group.groupLabelScreenX = gsx + layout.diagramX * zoom
    group.groupLabelScreenY = gsy + layout.diagramY * zoom - 12

    for (const node of group.nodes) {
      updateNodeStateRecursive(
        node,
        1,
        originX, originY, zoom, canvasCenterX, canvasCenterY,
        canvasW, canvasH,
        thresholds,
        1,
        preserveIds,
        0, 0, 0, 0,
      )
    }
  }

  return transitionRebase
}
