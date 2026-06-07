import { expect, type Page } from '@playwright/test'
import {
  createApiView,
  createConnector,
  createPlacedElement,
  createLayer,
  createTag,
  dispatchTouchEventOnLocator,
  uniqueName,
} from './vieweditor'

export type CanvasPoint = { x: number; y: number }
export type CanvasRect = CanvasPoint & { width: number; height: number }
export type ZUIViewState = { x: number; y: number; zoom: number; originX?: number; originY?: number }
export type ZUITestNodeRect = CanvasRect & { elementId: number; diagramId: number }
export type ZUITestInteraction = {
  type: 'wheel' | 'mouse' | 'touch' | 'dblclick'
  mode: string
  deltaX?: number
  deltaY?: number
  deltaMode?: number
  ctrlKey?: boolean
}

declare global {
  interface Window {
    __TLD_ZUI_TEST_STATE__?: {
      viewState: ZUIViewState
      groups: Array<{ nodes: unknown[] }>
      lastInteraction?: ZUITestInteraction
    }
  }
}

export async function waitForZuiReady(page: Page) {
  await expect(page.locator('canvas')).toBeVisible()
  await expect.poll(async () => page.evaluate(() => !!window.__TLD_ZUI_TEST_STATE__)).toBe(true)
}

export async function zuiViewState(page: Page) {
  await waitForZuiReady(page)
  return page.evaluate(() => {
    const state = window.__TLD_ZUI_TEST_STATE__
    if (!state) throw new Error('Missing ZUI test state')
    return state.viewState
  })
}

export async function zuiLastInteraction(page: Page) {
  await waitForZuiReady(page)
  return page.evaluate(() => window.__TLD_ZUI_TEST_STATE__?.lastInteraction ?? null)
}

export async function waitForZuiFrame(page: Page) {
  await page.evaluate(() => new Promise<void>((resolve) => {
    requestAnimationFrame(() => requestAnimationFrame(() => resolve()))
  }))
}

function cameraDelta(left: ZUIViewState, right: ZUIViewState) {
  const zoom = Math.max(left.zoom, right.zoom, 1)
  return (
    Math.abs(left.x - right.x) +
    Math.abs(left.y - right.y) +
    Math.abs((left.originX ?? 0) - (right.originX ?? 0)) * zoom +
    Math.abs((left.originY ?? 0) - (right.originY ?? 0)) * zoom +
    Math.abs(left.zoom - right.zoom) * 100
  )
}

export async function waitForStableZuiCamera(page: Page, tolerance = 1) {
  await waitForZuiFrame(page)
  let previous = await zuiViewState(page)
  for (let attempt = 0; attempt < 8; attempt += 1) {
    await waitForZuiFrame(page)
    const next = await zuiViewState(page)
    if (cameraDelta(previous, next) <= tolerance) {
      return next
    }
    previous = next
  }
  return previous
}

export async function canvasBox(page: Page) {
  const box = await page.locator('canvas').boundingBox()
  if (!box) throw new Error('ZUI canvas is not visible')
  return box
}

export async function zuiScreenGeometry(page: Page, options: {
  nodeElementId?: number
  connector?: { sourceElementId: number; targetElementId: number }
}) {
  await waitForZuiReady(page)
  return page.locator('canvas').evaluate((canvas: HTMLCanvasElement, request) => {
    const state = window.__TLD_ZUI_TEST_STATE__
    if (!state) throw new Error('Missing ZUI test state')

    type LayoutNodeLike = {
      elementId: number
      diagramId: number
      worldX: number
      worldY: number
      worldW: number
      worldH: number
      childScale: number
      childOffsetX: number
      childOffsetY: number
      children: LayoutNodeLike[]
    }
    type AbsNode = ZUITestNodeRect & { worldX: number; worldY: number; worldW: number; worldH: number }

    const rect = canvas.getBoundingClientRect()
    const view = state.viewState
    const originX = view.originX ?? -view.x / view.zoom
    const originY = view.originY ?? -view.y / view.zoom
    const toScreen = (worldX: number, worldY: number): CanvasPoint => ({
      x: rect.left + (worldX - originX) * view.zoom + view.x,
      y: rect.top + (worldY - originY) * view.zoom + view.y,
    })

    const nodes: AbsNode[] = []
    const visit = (
      items: LayoutNodeLike[],
      parentAbsX: number,
      parentAbsY: number,
      parentScale: number,
      parentChildOffsetX: number,
      parentChildOffsetY: number,
    ) => {
      for (const node of items) {
        const worldX = parentAbsX + (node.worldX - parentChildOffsetX) * parentScale
        const worldY = parentAbsY + (node.worldY - parentChildOffsetY) * parentScale
        const worldW = node.worldW * parentScale
        const worldH = node.worldH * parentScale
        const topLeft = toScreen(worldX, worldY)
        nodes.push({
          elementId: node.elementId,
          diagramId: node.diagramId,
          worldX,
          worldY,
          worldW,
          worldH,
          x: topLeft.x,
          y: topLeft.y,
          width: worldW * view.zoom,
          height: worldH * view.zoom,
        })
        visit(
          node.children ?? [],
          worldX,
          worldY,
          parentScale * node.childScale,
          node.childOffsetX,
          node.childOffsetY,
        )
      }
    }

    for (const group of state.groups) {
      visit(group.nodes as LayoutNodeLike[], 0, 0, 1, 0, 0)
    }

    const nodeScore = (node: AbsNode) => {
      const canvasCenterX = rect.left + rect.width / 2
      const canvasCenterY = rect.top + rect.height / 2
      const nodeCenterX = node.x + node.width / 2
      const nodeCenterY = node.y + node.height / 2
      const overlapsCanvas =
        node.x + node.width >= rect.left &&
        node.x <= rect.right &&
        node.y + node.height >= rect.top &&
        node.y <= rect.bottom
      return (overlapsCanvas ? 0 : 1_000_000) + Math.hypot(nodeCenterX - canvasCenterX, nodeCenterY - canvasCenterY)
    }

    const findBestNode = (elementId: number) => {
      return nodes
        .filter((candidate) => candidate.elementId === elementId)
        .sort((a, b) => nodeScore(a) - nodeScore(b))[0]
    }

    if (request.nodeElementId) {
      const node = findBestNode(request.nodeElementId)
      if (!node) throw new Error(`Missing node ${request.nodeElementId}`)
      return { node }
    }

    if (request.connector) {
      const source = findBestNode(request.connector.sourceElementId)
      const target = findBestNode(request.connector.targetElementId)
      if (!source || !target) throw new Error('Missing connector endpoint')
      const sourcePoint = toScreen(source.worldX + source.worldW, source.worldY + source.worldH / 2)
      const targetPoint = toScreen(target.worldX, target.worldY + target.worldH / 2)
      return {
        connector: {
          sourcePoint,
          targetPoint,
          midPoint: {
            x: (sourcePoint.x + targetPoint.x) / 2,
            y: (sourcePoint.y + targetPoint.y) / 2,
          },
        },
      }
    }

    throw new Error('No ZUI geometry request')
  }, options)
}

export async function createNestedZuiFixture(page: Page) {
  const root = await createApiView(page, uniqueName('ZUI Interaction Root'))
  const parent = await createPlacedElement(page, root.id, {
    name: uniqueName('ZUI Parent Service'),
    kind: 'service',
    technology: 'go',
  }, 120, 140)
  const sibling = await createPlacedElement(page, root.id, {
    name: uniqueName('ZUI Sibling Store'),
    kind: 'database',
    technology: 'postgres',
  }, 520, 140)
  const child = await createApiView(page, uniqueName('ZUI Child Detail'), parent.id)
  const childSource = await createPlacedElement(page, child.id, {
    name: uniqueName('ZUI Child API'),
    kind: 'api',
    technology: 'typescript',
  }, 120, 120)
  const childTarget = await createPlacedElement(page, child.id, {
    name: uniqueName('ZUI Child Worker'),
    kind: 'service',
    technology: 'node',
  }, 380, 120)

  await createConnector(page, root.id, parent.id, sibling.id, { label: 'Parent Link', style: 'straight' })
  await createConnector(page, child.id, childSource.id, childTarget.id, { label: 'Child Link', style: 'straight' })

  return { root, parent, sibling, child, childSource, childTarget }
}

export async function createTaggedZuiFixture(page: Page) {
  const diagram = await createApiView(page, uniqueName('ZUI Tags Root'))
  const tagName = uniqueName('zui-tag')
  const layerName = uniqueName('ZUI Layer')
  await createTag(page, tagName, '#38BDF8')
  const source = await createPlacedElement(page, diagram.id, {
    name: uniqueName('ZUI Tagged API'),
    kind: 'api',
    technology: 'typescript',
    tags: [tagName],
  }, 120, 140)
  const target = await createPlacedElement(page, diagram.id, {
    name: uniqueName('ZUI Tagged DB'),
    kind: 'database',
    technology: 'postgres',
  }, 440, 180)
  await createConnector(page, diagram.id, source.id, target.id, { label: 'writes to' })
  await createLayer(page, diagram.id, { name: layerName, tags: [tagName], color: '#38BDF8' })
  return { diagram, source, target, tagName, layerName }
}

export async function dispatchZuiTouchPan(page: Page, from: CanvasPoint, to: CanvasPoint) {
  const canvas = page.locator('canvas')
  await dispatchTouchEventOnLocator(canvas, 'touchstart', [{ identifier: 1, clientX: from.x, clientY: from.y }])
  await dispatchTouchEventOnLocator(canvas, 'touchmove', [{ identifier: 1, clientX: to.x, clientY: to.y }])
  await dispatchTouchEventOnLocator(canvas, 'touchend', [], [{ identifier: 1, clientX: to.x, clientY: to.y }])
}

export async function dispatchZuiWheel(page: Page, point: CanvasPoint, options: {
  deltaX?: number
  deltaY?: number
  deltaMode?: number
  ctrlKey?: boolean
}) {
  await page.locator('canvas').evaluate((element, payload) => {
    const rect = element.getBoundingClientRect()
    const clientX = Math.min(Math.max(payload.point.x, rect.left + 1), rect.right - 1)
    const clientY = Math.min(Math.max(payload.point.y, rect.top + 1), rect.bottom - 1)
    element.dispatchEvent(new WheelEvent('wheel', {
      bubbles: true,
      cancelable: true,
      clientX,
      clientY,
      deltaX: payload.options.deltaX ?? 0,
      deltaY: payload.options.deltaY ?? 0,
      deltaMode: payload.options.deltaMode ?? 0,
      ctrlKey: payload.options.ctrlKey ?? false,
    }))
  }, { point, options })
}

export async function dispatchZuiMousePan(page: Page, from: CanvasPoint, to: CanvasPoint) {
  const canvas = page.locator('canvas')
  await canvas.evaluate((element, point) => {
    element.dispatchEvent(new MouseEvent('mousedown', {
      bubbles: true,
      cancelable: true,
      clientX: point.x,
      clientY: point.y,
      button: 0,
      buttons: 1,
    }))
  }, from)
  await page.evaluate((point) => {
    window.dispatchEvent(new MouseEvent('mousemove', {
      bubbles: true,
      cancelable: true,
      clientX: point.x,
      clientY: point.y,
      button: 0,
      buttons: 1,
    }))
  }, to)
  await page.evaluate((point) => {
    window.dispatchEvent(new MouseEvent('mouseup', {
      bubbles: true,
      cancelable: true,
      clientX: point.x,
      clientY: point.y,
      button: 0,
      buttons: 0,
    }))
  }, to)
}

export async function dispatchZuiDoubleClick(page: Page, point: CanvasPoint) {
  await page.locator('canvas').evaluate((element, clickPoint) => {
    element.dispatchEvent(new MouseEvent('dblclick', {
      bubbles: true,
      cancelable: true,
      clientX: clickPoint.x,
      clientY: clickPoint.y,
      button: 0,
      buttons: 0,
    }))
  }, point)
}

export async function dispatchZuiPinch(page: Page, center: CanvasPoint, startDistance: number, endDistance: number) {
  const canvas = page.locator('canvas')
  const start = [
    { identifier: 1, clientX: center.x - startDistance / 2, clientY: center.y },
    { identifier: 2, clientX: center.x + startDistance / 2, clientY: center.y },
  ]
  const moved = [
    { identifier: 1, clientX: center.x - endDistance / 2, clientY: center.y - 8 },
    { identifier: 2, clientX: center.x + endDistance / 2, clientY: center.y - 8 },
  ]
  await dispatchTouchEventOnLocator(canvas, 'touchstart', start)
  await dispatchTouchEventOnLocator(canvas, 'touchmove', moved, moved)
  await dispatchTouchEventOnLocator(canvas, 'touchend', [], moved)
}
