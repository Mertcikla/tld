import { expect, test } from '../../fixtures'
import {
  createAndLoadDiagramWithNodes,
} from '../../helpers/vieweditor'

type CanvasBox = { x: number; y: number; width: number; height: number }
type Point = { x: number; y: number }

async function drawStroke(page: import('@playwright/test').Page, from: Point, to: Point) {
  await page.mouse.move(from.x, from.y)
  await page.mouse.down()
  await page.mouse.move((from.x + to.x) / 2, (from.y + to.y) / 2, { steps: 4 })
  await page.mouse.move(to.x, to.y, { steps: 4 })
  await page.mouse.up()
}

function canvasPoint(box: CanvasBox, xRatio: number, yRatio: number) {
  return { x: box.x + box.width * xRatio, y: box.y + box.height * yRatio }
}

async function drawStylusStroke(page: import('@playwright/test').Page, from: { x: number; y: number }, to: { x: number; y: number }) {
  const canvas = page.getByTestId('drawing-canvas')
  await canvas.dispatchEvent('pointerdown', {
    clientX: from.x,
    clientY: from.y,
    pointerId: 1,
    pointerType: 'pen',
    pressure: 0.2,
    button: 0,
    buttons: 1,
  })
  await canvas.dispatchEvent('pointermove', {
    clientX: (from.x + to.x) / 2,
    clientY: (from.y + to.y) / 2,
    pointerId: 1,
    pointerType: 'pen',
    pressure: 0.75,
    button: 0,
    buttons: 1,
  })
  await canvas.dispatchEvent('pointermove', {
    clientX: to.x,
    clientY: to.y,
    pointerId: 1,
    pointerType: 'pen',
    pressure: 0.45,
    button: 0,
    buttons: 1,
  })
  await canvas.dispatchEvent('pointerup', {
    clientX: to.x,
    clientY: to.y,
    pointerId: 1,
    pointerType: 'pen',
    pressure: 0.1,
    button: 0,
    buttons: 0,
  })
}


test('draws a pencil path, hides it, shows it, and exits drawing mode', async ({ page }) => {
  await createAndLoadDiagramWithNodes(page, 0, 'Drawing Basic')
  await page.getByTestId('vieweditor-toolbar-draw').click()
  await expect(page.getByTestId('draw-menu')).toBeVisible()

  const canvas = page.getByTestId('drawing-canvas')
  const box = await canvas.boundingBox()
  if (!box) throw new Error('Drawing canvas is not visible')
  await drawStroke(page, canvasPoint(box, 0.62, 0.34), canvasPoint(box, 0.72, 0.44))

  await expect(canvas).toHaveAttribute('data-path-count', '1')
  await expect(page.getByTestId('vieweditor-toolbar-draw-visibility')).toBeVisible()
  await page.getByTestId('vieweditor-toolbar-draw-visibility').click()
  await expect(canvas).toHaveAttribute('data-drawing-visible', 'false')
  await page.getByTestId('vieweditor-toolbar-draw-visibility').click()
  await expect(canvas).toHaveAttribute('data-drawing-visible', 'true')

  await page.getByTestId('draw-menu-done').click()
  await expect(page.getByTestId('draw-menu')).toHaveCount(0)
})

test('supports drawing undo and redo shortcuts', async ({ page }) => {
  await createAndLoadDiagramWithNodes(page, 0, 'Drawing Undo')
  await page.getByTestId('vieweditor-toolbar-draw').click()
  const canvas = page.getByTestId('drawing-canvas')
  const box = await canvas.boundingBox()
  if (!box) throw new Error('Drawing canvas is not visible')

  await drawStroke(page, canvasPoint(box, 0.58, 0.30), canvasPoint(box, 0.68, 0.38))
  await expect(canvas).toHaveAttribute('data-path-count', '1')

  await page.keyboard.press(process.platform === 'darwin' ? 'Meta+Z' : 'Control+Z')
  await expect(canvas).toHaveAttribute('data-path-count', '0')
  await page.keyboard.press(process.platform === 'darwin' ? 'Meta+Shift+Z' : 'Control+Shift+Z')
  await expect(canvas).toHaveAttribute('data-path-count', '1')
})

test('records stylus pressure for pen input', async ({ page }) => {
  await createAndLoadDiagramWithNodes(page, 0, 'Drawing Stylus Pressure')
  await page.getByTestId('vieweditor-toolbar-draw').click()
  const canvas = page.getByTestId('drawing-canvas')
  const box = await canvas.boundingBox()
  if (!box) throw new Error('Drawing canvas is not visible')

  await drawStylusStroke(page, { x: box.x + 200, y: box.y + 290 }, { x: box.x + 340, y: box.y + 330 })

  await expect(canvas).toHaveAttribute('data-path-count', '1')
  await expect(canvas).toHaveAttribute('data-pressure-point-count', '4')
})

test('changes drawing tool, color, and width from the draw menu', async ({ page }) => {
  await createAndLoadDiagramWithNodes(page, 0, 'Drawing Tools')
  await page.getByTestId('vieweditor-toolbar-draw').click()

  await page.locator('[data-testid="draw-menu-width"][data-width="12"]').click()
  await page.locator('[data-testid="draw-menu-color"][data-color="#f56565"]').click()
  await page.getByTestId('draw-menu-eraser-e').click()
  await expect(page.getByTestId('draw-menu-eraser-e')).toBeVisible()
  await page.getByTestId('draw-menu-pen-p').click()

  const canvas = page.getByTestId('drawing-canvas')
  const box = await canvas.boundingBox()
  if (!box) throw new Error('Drawing canvas is not visible')
  await drawStroke(page, canvasPoint(box, 0.60, 0.52), canvasPoint(box, 0.74, 0.52))
  await expect(canvas).toHaveAttribute('data-path-count', '1')
})
