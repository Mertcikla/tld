import { expect, test } from '@playwright/test'
import {
  createAndLoadDiagramWithNodes,
  prepareStorage,
} from '../../helpers/vieweditor'

async function drawStroke(page: import('@playwright/test').Page, from: { x: number; y: number }, to: { x: number; y: number }) {
  const canvas = page.getByTestId('drawing-canvas')
  await canvas.dispatchEvent('pointerdown', {
    clientX: from.x,
    clientY: from.y,
    pointerId: 1,
    pointerType: 'mouse',
    button: 0,
    buttons: 1,
  })
  await canvas.dispatchEvent('pointermove', {
    clientX: (from.x + to.x) / 2,
    clientY: (from.y + to.y) / 2,
    pointerId: 1,
    pointerType: 'mouse',
    button: 0,
    buttons: 1,
  })
  await canvas.dispatchEvent('pointermove', {
    clientX: to.x,
    clientY: to.y,
    pointerId: 1,
    pointerType: 'mouse',
    button: 0,
    buttons: 1,
  })
  await canvas.dispatchEvent('pointerup', {
    clientX: to.x,
    clientY: to.y,
    pointerId: 1,
    pointerType: 'mouse',
    button: 0,
    buttons: 0,
  })
}

test.beforeEach(async ({ page }) => {
  await prepareStorage(page)
})

test('draws a pencil path, hides it, shows it, and exits drawing mode', async ({ page }) => {
  await createAndLoadDiagramWithNodes(page, 0, 'Drawing Basic')
  await page.getByTestId('vieweditor-toolbar-draw').click()
  await expect(page.getByTestId('draw-menu')).toBeVisible()

  const canvas = page.getByTestId('drawing-canvas')
  const box = await canvas.boundingBox()
  if (!box) throw new Error('Drawing canvas is not visible')
  await drawStroke(page, { x: box.x + 260, y: box.y + 260 }, { x: box.x + 340, y: box.y + 320 })

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

  await drawStroke(page, { x: box.x + 180, y: box.y + 210 }, { x: box.x + 260, y: box.y + 250 })
  await expect(canvas).toHaveAttribute('data-path-count', '1')

  await page.keyboard.press(process.platform === 'darwin' ? 'Meta+Z' : 'Control+Z')
  await expect(canvas).toHaveAttribute('data-path-count', '0')
  await page.keyboard.press(process.platform === 'darwin' ? 'Meta+Shift+Z' : 'Control+Shift+Z')
  await expect(canvas).toHaveAttribute('data-path-count', '1')
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
  await drawStroke(page, { x: box.x + 220, y: box.y + 360 }, { x: box.x + 360, y: box.y + 360 })
  await expect(canvas).toHaveAttribute('data-path-count', '1')
})
