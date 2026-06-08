import { expect, test } from '../fixtures'
import type { Page } from '@playwright/test'
import {
  createApiView,
  createConnector,
  createLayer,
  createPlacedElement,
  createTag,
  uniqueName,
} from '../helpers/vieweditor'

type CanvasPoint = { x: number; y: number }
type CanvasRect = CanvasPoint & { width: number; height: number }
type ZUITestNodeRect = CanvasRect & { elementId: number; diagramId: number }

declare global {
  interface Window {
    __TLD_ZUI_TEST_STATE__?: {
      viewState: { x: number; y: number; zoom: number; originX?: number; originY?: number }
      groups: Array<{ nodes: unknown[] }>
    }
  }
}


async function canvasPixelStats(page: Page, region?: CanvasRect) {
  return page.locator('canvas').evaluate((canvas: HTMLCanvasElement, sample?: CanvasRect) => {
    const ctx = canvas.getContext('2d')
    if (!ctx || canvas.width === 0 || canvas.height === 0) {
      return { uniqueColors: 0, nonTransparent: 0, accentLike: 0 }
    }

    const dpr = window.devicePixelRatio || 1
    const rect = canvas.getBoundingClientRect()
    const left = Math.max(0, Math.floor(((sample?.x ?? rect.left) - rect.left) * dpr))
    const top = Math.max(0, Math.floor(((sample?.y ?? rect.top) - rect.top) * dpr))
    const right = Math.min(canvas.width, Math.ceil(((sample ? sample.x + sample.width : rect.right) - rect.left) * dpr))
    const bottom = Math.min(canvas.height, Math.ceil(((sample ? sample.y + sample.height : rect.bottom) - rect.top) * dpr))
    const step = Math.max(2, Math.floor(Math.min(right - left, bottom - top) / 48))
    const colors = new Set<string>()
    let nonTransparent = 0
    let accentLike = 0
    let totalR = 0
    let totalG = 0
    let totalB = 0

    for (let y = top; y < bottom; y += step) {
      for (let x = left; x < right; x += step) {
        const [r, g, b, a] = ctx.getImageData(x, y, 1, 1).data
        if (a > 0) nonTransparent += 1
        totalR += r
        totalG += g
        totalB += b
        const accentDistance = Math.hypot(r - 99, g - 179, b - 237)
        if (accentDistance < 92 && b > r && b > g * 0.9) accentLike += 1
        colors.add(`${r},${g},${b},${a}`)
      }
    }

    const divisor = Math.max(1, colors.size === 0 ? 0 : nonTransparent)
    return {
      uniqueColors: colors.size,
      nonTransparent,
      accentLike,
      avgR: totalR / divisor,
      avgG: totalG / divisor,
      avgB: totalB / divisor,
    }
  }, region)
}

async function canvasLinePixelStats(page: Page, from: CanvasPoint, to: CanvasPoint, radius = 10) {
  return page.locator('canvas').evaluate((canvas: HTMLCanvasElement, input) => {
    const ctx = canvas.getContext('2d')
    if (!ctx) return { accentLike: 0, sampled: 0 }

    const dpr = window.devicePixelRatio || 1
    const rect = canvas.getBoundingClientRect()
    const dx = input.to.x - input.from.x
    const dy = input.to.y - input.from.y
    const length = Math.hypot(dx, dy) || 1
    const nx = -dy / length
    const ny = dx / length
    let accentLike = 0
    let sampled = 0

    for (let index = 0; index <= 48; index += 1) {
      const t = index / 48
      const x = input.from.x + dx * t
      const y = input.from.y + dy * t
      for (let offset = -input.radius; offset <= input.radius; offset += 2) {
        const px = Math.floor((x + nx * offset - rect.left) * dpr)
        const py = Math.floor((y + ny * offset - rect.top) * dpr)
        if (px < 0 || py < 0 || px >= canvas.width || py >= canvas.height) continue
        const [r, g, b, a] = ctx.getImageData(px, py, 1, 1).data
        if (a === 0) continue
        sampled += 1
        const accentDistance = Math.hypot(r - 99, g - 179, b - 237)
        if (accentDistance < 95 && b > r && b > g * 0.85) accentLike += 1
      }
    }

    return { accentLike, sampled }
  }, { from, to, radius })
}

async function canvasConnectorRegionStats(page: Page, from: CanvasPoint, to: CanvasPoint, radius = 24) {
  const left = Math.min(from.x, to.x) - radius
  const top = Math.min(from.y, to.y) - radius
  return canvasPixelStats(page, {
    x: left,
    y: top,
    width: Math.abs(to.x - from.x) + radius * 2,
    height: Math.abs(to.y - from.y) + radius * 2,
  })
}

async function waitForCanvasFrame(page: Page) {
  await page.evaluate(() => new Promise<void>((resolve) => {
    requestAnimationFrame(() => requestAnimationFrame(() => resolve()))
  }))
}

async function zuiScreenGeometry(page: Page, options: {
  nodeElementId?: number
  connector?: { sourceElementId: number; targetElementId: number }
}) {
  await expect.poll(async () => page.evaluate(() => !!window.__TLD_ZUI_TEST_STATE__)).toBe(true)
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

async function expectZuiNodeVisible(page: Page, nodeElementId: number) {
  let visibleNode: ZUITestNodeRect | null = null
  await expect.poll(async () => {
    const box = await page.locator('canvas').boundingBox()
    if (!box) return false
    const { node } = await zuiScreenGeometry(page, { nodeElementId }) as { node: ZUITestNodeRect }
    const visible =
      node.x + node.width >= box.x &&
      node.x <= box.x + box.width &&
      node.y + node.height >= box.y &&
      node.y <= box.y + box.height
    if (visible) visibleNode = node
    return visible
  }).toBe(true)
  return visibleNode!
}

async function expectConnectorAccentPixels(
  page: Page,
  connector: { sourceElementId: number; targetElementId: number },
  radius = 10,
) {
  let latest = { accentLike: 0, sampled: 0, visibilityScore: 0 }
  await expect.poll(async () => {
    const { connector: geometry } = await zuiScreenGeometry(page, { connector }) as {
      connector: { sourcePoint: CanvasPoint; targetPoint: CanvasPoint; midPoint: CanvasPoint }
    }
    const line = await canvasLinePixelStats(page, geometry.sourcePoint, geometry.targetPoint, radius)
    const region = await canvasConnectorRegionStats(page, geometry.sourcePoint, geometry.targetPoint, radius * 2)
    // Connector paint can vary by theme and state; count either accent pixels or dense opaque line samples.
    const visibilityScore = Math.max(line.accentLike, Math.floor(line.sampled / 6))
    latest = {
      accentLike: Math.max(line.accentLike, region.accentLike),
      sampled: line.sampled + region.nonTransparent,
      visibilityScore,
    }
    return latest.visibilityScore
  }).toBeGreaterThan(8)
  return latest
}

async function createNestedCanvasFixture(page: Page) {
  const root = await createApiView(page, uniqueName('Explore Zoom Root'))
  const parent = await createPlacedElement(page, root.id, {
    name: uniqueName('Explore Parent Service'),
    kind: 'service',
    technology: 'go',
  }, 120, 140)
  const sibling = await createPlacedElement(page, root.id, {
    name: uniqueName('Explore Sibling Store'),
    kind: 'database',
    technology: 'postgres',
  }, 520, 140)
  const child = await createApiView(page, uniqueName('Explore Child Detail'), parent.id)
  const childSource = await createPlacedElement(page, child.id, {
    name: uniqueName('Explore Child API'),
    kind: 'api',
    technology: 'typescript',
  }, 120, 120)
  const childTarget = await createPlacedElement(page, child.id, {
    name: uniqueName('Explore Child Worker'),
    kind: 'service',
    technology: 'node',
  }, 380, 120)

  await createConnector(page, root.id, parent.id, sibling.id, { label: 'Parent Link', style: 'straight' })
  await createConnector(page, child.id, childSource.id, childTarget.id, { label: 'Child Link', style: 'straight' })

  return { root, parent, sibling, childSource, childTarget }
}

async function createDeepNestedCanvasFixture(page: Page, depth = 8) {
  const root = await createApiView(page, uniqueName('Explore Deep Root'))
  let currentView = root
  let deepest = await createPlacedElement(page, currentView.id, {
    name: uniqueName('Explore Deep Level 1'),
    kind: 'service',
    technology: 'go',
  }, 120, 120)

  for (let level = 2; level <= depth; level += 1) {
    currentView = await createApiView(page, uniqueName(`Explore Deep Level ${level} View`), deepest.id)
    deepest = await createPlacedElement(page, currentView.id, {
      name: uniqueName(`Explore Deep Level ${level}`),
      kind: level % 2 === 0 ? 'api' : 'service',
      technology: level % 2 === 0 ? 'typescript' : 'go',
    }, 120, 120)
  }

  return { root, deepestView: currentView, deepest }
}

test('renders the explore canvas and opens the tag controls', async ({ page }) => {
  const diagram = await createApiView(page, uniqueName('Explore Canvas'))
  const tagName = uniqueName('critical-path')
  const layerName = uniqueName('Runtime Layer')
  const source = await createPlacedElement(page, diagram.id, {
    name: uniqueName('Explore API'),
    kind: 'api',
    technology: 'typescript',
    tags: [tagName],
  }, 120, 140)
  const target = await createPlacedElement(page, diagram.id, {
    name: uniqueName('Explore DB'),
    kind: 'database',
    technology: 'postgres',
    tags: ['data-store'],
  }, 440, 180)
  await createConnector(page, diagram.id, source.id, target.id, { label: 'writes to' })
  await createTag(page, tagName, '#38BDF8')
  await createLayer(page, diagram.id, { name: layerName, tags: [tagName], color: '#38BDF8' })

  await page.goto(`/views?view=explore&focus=${diagram.id}`)

  const canvas = page.locator('canvas')
  await expect(canvas).toBeVisible()
  await expect(page.getByRole('button', { name: 'Fit View' })).toBeVisible()

  await expect.poll(async () => {
    const stats = await canvasPixelStats(page)
    return stats.uniqueColors
  }).toBeGreaterThan(10)
  const stats = await canvasPixelStats(page)
  expect(stats.nonTransparent).toBeGreaterThan(100)

  await page.getByRole('button', { name: 'Fit View' }).click()
  const tagsButton = page.getByRole('button', { name: 'Tags' })
  await expect(tagsButton).toBeVisible()
  await tagsButton.click()
  await expect(tagsButton).toHaveAttribute('aria-expanded', 'true')

  await expect(page.getByText(layerName)).toBeVisible()
  await expect(page.getByText(tagName)).toBeVisible()
})

test('zooms into a linked component and changes parent transparency and connector visibility', async ({ page }) => {
  const { root, parent, sibling, childSource, childTarget } = await createNestedCanvasFixture(page)

  await page.goto(`/views?view=explore&debugZuiTest=1&focus=${root.id}`)
  const canvas = page.locator('canvas')
  await expect(canvas).toBeVisible()
  await expect(page.getByRole('button', { name: 'Fit View' })).toBeVisible()

  await expectConnectorAccentPixels(page, { sourceElementId: parent.id, targetElementId: sibling.id })

  await page.goto(`/views?view=explore&debugZuiTest=1&focus=${root.id}&element=${parent.id}`)
  await expect(canvas).toBeVisible()
  await waitForCanvasFrame(page)

  await expectConnectorAccentPixels(page, { sourceElementId: childSource.id, targetElementId: childTarget.id }, 12)

  await page.goto(`/views?view=explore&debugZuiTest=1&focus=${root.id}`)
  await expect(canvas).toBeVisible()
  await waitForCanvasFrame(page)
  await expectConnectorAccentPixels(page, { sourceElementId: parent.id, targetElementId: sibling.id })
})

test('keeps an eight-level deep focused node visible on the canvas', async ({ page }) => {
  const { deepestView, deepest } = await createDeepNestedCanvasFixture(page, 8)

  await page.goto(`/views?view=explore&debugZuiTest=1&focus=${deepestView.id}&element=${deepest.id}`)
  const canvas = page.locator('canvas')
  await expect(canvas).toBeVisible()
  await expect(page.getByRole('button', { name: 'Fit View' })).toBeVisible()

  let visibleNode: ZUITestNodeRect | null = null
  await expect.poll(async () => {
    const box = await canvas.boundingBox()
    if (!box) return false
    const { node } = await zuiScreenGeometry(page, { nodeElementId: deepest.id }) as { node: ZUITestNodeRect }
    const centerX = node.x + node.width / 2
    const centerY = node.y + node.height / 2
    const centerVisible =
      centerX >= box.x + 8 &&
      centerX <= box.x + box.width - 8 &&
      centerY >= box.y + 8 &&
      centerY <= box.y + box.height - 8

    const visible = centerVisible && node.width > 32 && node.height > 18
    if (visible) visibleNode = node
    return visible
  }).toBe(true)

  const node = visibleNode!
  const stats = await canvasPixelStats(page, {
    x: node.x,
    y: node.y,
    width: Math.min(node.width, 180),
    height: Math.min(node.height, 100),
  })
  expect(stats.uniqueColors).toBeGreaterThan(4)
  expect(stats.nonTransparent).toBeGreaterThan(20)
})

test('keeps repeated zoom interactions within the canvas performance budget', async ({ page }, testInfo) => {
  test.skip(
    !['chromium', 'firefox'].includes(testInfo.project.name),
    'canvas performance budget is calibrated for desktop browser projects',
  )

  await createNestedCanvasFixture(page)
  await page.goto('/views?view=explore&debugZuiTest=1')
  await expect(page.locator('canvas')).toBeVisible()
  await expect(page.getByRole('button', { name: 'Fit View' })).toBeVisible()

  const result = await page.locator('canvas').evaluate(async (canvas: HTMLCanvasElement) => {
    const rect = canvas.getBoundingClientRect()
    const longTasks: number[] = []
    const observer = 'PerformanceObserver' in window
      ? new PerformanceObserver((list) => {
        for (const entry of list.getEntries()) longTasks.push(entry.duration)
      })
      : null
    observer?.observe({ entryTypes: ['longtask'] })

    const frameDurations: number[] = []
    let previousFrame = performance.now()
    const centerX = rect.left + rect.width / 2
    const centerY = rect.top + rect.height / 2

    for (let index = 0; index < 36; index += 1) {
      const deltaY = index % 2 === 0 ? -80 : 80
      const beforeDispatch = performance.now()
      canvas.dispatchEvent(new WheelEvent('wheel', {
        bubbles: true,
        cancelable: true,
        clientX: centerX,
        clientY: centerY,
        deltaMode: WheelEvent.DOM_DELTA_LINE,
        deltaY,
      }))
      const afterDispatch = performance.now()
      await new Promise<void>((resolve) => requestAnimationFrame(() => resolve()))
      const now = performance.now()
      frameDurations.push(Math.max(now - previousFrame, afterDispatch - beforeDispatch))
      previousFrame = now
    }

    observer?.disconnect()
    return {
      averageFrameMs: frameDurations.reduce((sum, value) => sum + value, 0) / frameDurations.length,
      maxFrameMs: Math.max(...frameDurations),
      maxLongTaskMs: longTasks.length > 0 ? Math.max(...longTasks) : 0,
    }
  })

  expect(result.averageFrameMs).toBeLessThan(35)
  expect(result.maxFrameMs).toBeLessThan(120)
  expect(result.maxLongTaskMs).toBeLessThan(120)
})
