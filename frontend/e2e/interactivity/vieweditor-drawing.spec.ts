import { expect, test } from '../fixtures'
import type { Locator, Page } from '@playwright/test'
import {
  createAndLoadDiagramWithNodes,
  reactFlowViewport,
  reloadView,
} from '../helpers/vieweditor'

type Point = { x: number; y: number }
type CanvasBox = { x: number; y: number; width: number; height: number }

function canvasPoint(box: CanvasBox, xRatio: number, yRatio: number): Point {
  return { x: box.x + box.width * xRatio, y: box.y + box.height * yRatio }
}

async function drawingCanvasBox(page: Page) {
  const canvas = page.getByTestId('drawing-canvas')
  const box = await canvas.boundingBox()
  if (!box) throw new Error('Drawing canvas is not visible')
  return { canvas, box }
}

async function drawMouseStroke(page: Page, from: Point, to: Point) {
  await page.mouse.move(from.x, from.y)
  await page.mouse.down()
  await page.mouse.move((from.x + to.x) / 2, (from.y + to.y) / 2, { steps: 4 })
  await page.mouse.move(to.x, to.y, { steps: 4 })
  await page.mouse.up()
}

async function clickDrawingControl(page: Page, testId: string) {
  await page.getByTestId(testId).evaluate((element) => {
    (element as HTMLElement).click()
  })
}

async function dispatchPenPointer(canvas: Locator, type: 'pointerdown' | 'pointermove' | 'pointerup', points: Array<Point & { pressure: number }>) {
  await canvas.evaluate((element, payload) => {
    const point = payload.points[payload.points.length - 1]
    const event = new PointerEvent(payload.type, {
      bubbles: true,
      cancelable: true,
      clientX: point.x,
      clientY: point.y,
      pointerId: 51,
      pointerType: 'pen',
      pressure: point.pressure,
      button: 0,
      buttons: payload.type === 'pointerup' ? 0 : 1,
    })
    Object.defineProperty(event, 'getCoalescedEvents', {
      value: () => payload.points.map((coalesced) => new PointerEvent(payload.type, {
        bubbles: true,
        cancelable: true,
        clientX: coalesced.x,
        clientY: coalesced.y,
        pointerId: 51,
        pointerType: 'pen',
        pressure: coalesced.pressure,
        button: 0,
        buttons: payload.type === 'pointerup' ? 0 : 1,
      })),
    })
    element.dispatchEvent(event)
  }, { type, points })
}

test('erases an existing drawing path', async ({ page }) => {
  await createAndLoadDiagramWithNodes(page, 0, 'Drawing Eraser')
  await page.getByTestId('vieweditor-toolbar-draw').click()
  const { canvas, box } = await drawingCanvasBox(page)
  const from = canvasPoint(box, 0.55, 0.36)
  const to = canvasPoint(box, 0.70, 0.44)
  await drawMouseStroke(page, from, to)
  await expect(canvas).toHaveAttribute('data-path-count', '1')

  await clickDrawingControl(page, 'draw-menu-eraser-e')
  await drawMouseStroke(page, from, to)
  await expect(canvas).toHaveAttribute('data-path-count', '0')
})

test('selects and moves drawings, persists them, and blocks underlying canvas selection or pan', async ({ page }) => {
  await createAndLoadDiagramWithNodes(page, 2, 'Drawing Select Move')
  await page.getByTestId('vieweditor-toolbar-draw').click()
  const { canvas, box } = await drawingCanvasBox(page)
  const from = canvasPoint(box, 0.38, 0.36)
  const to = canvasPoint(box, 0.52, 0.40)
  await drawMouseStroke(page, from, to)
  await expect(canvas).toHaveAttribute('data-path-count', '1')

  await clickDrawingControl(page, 'draw-menu-select-move-v')
  await drawMouseStroke(page, canvasPoint(box, 0.45, 0.38), canvasPoint(box, 0.58, 0.50))
  await expect(canvas).toHaveAttribute('data-path-count', '1')

  const beforeOverlayDrag = await reactFlowViewport(page)
  await clickDrawingControl(page, 'draw-menu-pen-p')
  await drawMouseStroke(page, canvasPoint(box, 0.64, 0.64), canvasPoint(box, 0.74, 0.70))
  await expect(canvas).toHaveAttribute('data-path-count', '2')
  await expect(page.getByTestId('vieweditor-selection-bulk-bar')).toHaveCount(0)
  const afterOverlayDrag = await reactFlowViewport(page)
  expect(Math.abs(afterOverlayDrag.x - beforeOverlayDrag.x) + Math.abs(afterOverlayDrag.y - beforeOverlayDrag.y)).toBeLessThan(4)

  await reloadView(page)
  await expect(page.getByTestId('drawing-canvas')).toHaveAttribute('data-path-count', '2')
})

test('commits text with Enter and cancels text placement with Escape', async ({ page }) => {
  await createAndLoadDiagramWithNodes(page, 0, 'Drawing Text')
  await page.getByTestId('vieweditor-toolbar-draw').click()
  const { canvas, box } = await drawingCanvasBox(page)
  await clickDrawingControl(page, 'draw-menu-text-t')

  await page.mouse.click(box.x + box.width * 0.42, box.y + box.height * 0.42)
  await expect(page.getByTestId('drawing-text-input')).toBeVisible()
  await page.getByTestId('drawing-text-input').fill('cancel me')
  await page.keyboard.press('Escape')
  await expect(page.getByTestId('drawing-text-input')).toHaveCount(0)
  await expect(canvas).toHaveAttribute('data-path-count', '0')

  await page.mouse.click(box.x + box.width * 0.52, box.y + box.height * 0.50)
  await expect(page.getByTestId('drawing-text-input')).toBeVisible()
  await page.getByTestId('drawing-text-input').fill('kept note')
  await page.keyboard.press('Enter')
  await expect(page.getByTestId('drawing-text-input')).toHaveCount(0)
  await expect(canvas).toHaveAttribute('data-path-count', '1')
})

test('records pen pressure from coalesced stylus pointer events', async ({ page }) => {
  await createAndLoadDiagramWithNodes(page, 0, 'Drawing Coalesced Pen')
  await page.getByTestId('vieweditor-toolbar-draw').click()
  const { canvas, box } = await drawingCanvasBox(page)
  const start = canvasPoint(box, 0.34, 0.58)
  const mid = canvasPoint(box, 0.42, 0.64)
  const end = canvasPoint(box, 0.52, 0.60)

  await dispatchPenPointer(canvas, 'pointerdown', [{ ...start, pressure: 0.2 }])
  await dispatchPenPointer(canvas, 'pointermove', [
    { x: mid.x - 14, y: mid.y - 6, pressure: 0.45 },
    { ...mid, pressure: 0.8 },
  ])
  await dispatchPenPointer(canvas, 'pointermove', [
    { x: end.x - 12, y: end.y + 4, pressure: 0.65 },
    { ...end, pressure: 0.35 },
  ])
  await dispatchPenPointer(canvas, 'pointerup', [{ ...end, pressure: 0.1 }])

  await expect(canvas).toHaveAttribute('data-path-count', '1')
  await expect(canvas).toHaveAttribute('data-pressure-point-count', '6')
})
