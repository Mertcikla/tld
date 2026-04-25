import { expect, test } from '@playwright/test'
import {
  createAndLoadDiagramWithNodes,
  createElement,
  createPlacedElement,
  currentViewId,
  expectPlacement,
  libraryItemByName,
  nodeByName,
  prepareStorage,
  uniqueName,
} from '../../helpers/vieweditor'

test.beforeEach(async ({ page }) => {
  await prepareStorage(page)
})

test('searches the library and adds an existing catalog element to the canvas', async ({ page }) => {
  const { diagram } = await createAndLoadDiagramWithNodes(page, 0, 'Library Add')
  const element = await createElement(page, { name: uniqueName('Library Reusable'), kind: 'service' })
  await page.reload()

  await expect(page.getByTestId('element-library-panel')).toBeVisible()
  await page.getByTestId('element-library-search').fill(element.name)
  await expect(libraryItemByName(page, element.name)).toBeVisible()
  await libraryItemByName(page, element.name).getByTestId('element-library-add').click()

  await expect(nodeByName(page, element.name)).toBeVisible()
  await expectPlacement(page, element.name, true, diagram.id)
})

test('hide existing filters placed elements and find recenters an existing node', async ({ page }) => {
  const diagram = await createAndLoadDiagramWithNodes(page, 1, 'Library Existing')
  const existing = diagram.elements[0]
  const available = await createElement(page, { name: uniqueName('Library Available'), kind: 'database' })
  await page.reload()

  await page.getByTestId('element-library-search').fill('Library')
  await expect(libraryItemByName(page, existing.name)).toBeVisible()
  await expect(libraryItemByName(page, available.name)).toBeVisible()

  await page.getByTestId('element-library-hide-existing').click()
  await expect(libraryItemByName(page, existing.name)).toHaveCount(0)
  await expect(libraryItemByName(page, available.name)).toBeVisible()

  await page.getByTestId('element-library-hide-existing').click()
  await libraryItemByName(page, existing.name).getByTestId('element-library-find').click()
  await expect(nodeByName(page, existing.name)).toBeVisible()
})

test('dragging a library element to the canvas creates a placement', async ({ page }) => {
  await createAndLoadDiagramWithNodes(page, 0, 'Library Drag')
  const element = await createElement(page, { name: uniqueName('Library Drag Source'), kind: 'service' })
  await page.reload()
  await page.getByTestId('element-library-search').fill(element.name)

  const item = libraryItemByName(page, element.name)
  const box = await page.locator('.react-flow__pane').boundingBox()
  if (!box) throw new Error('Missing pane')
  const dataTransfer = await page.evaluateHandle(() => new DataTransfer())
  await item.dispatchEvent('dragstart', { dataTransfer })
  await page.locator('.react-flow__pane').dispatchEvent('dragover', {
    dataTransfer,
    clientX: box.x + box.width * 0.55,
    clientY: box.y + box.height * 0.45,
  })
  await page.locator('.react-flow__pane').dispatchEvent('drop', {
    dataTransfer,
    clientX: box.x + box.width * 0.55,
    clientY: box.y + box.height * 0.45,
  })

  await expect(nodeByName(page, element.name)).toBeVisible()
  await expectPlacement(page, element.name, true, currentViewId(page))
})

test('new element action opens an inline creator from the library', async ({ page }) => {
  await createAndLoadDiagramWithNodes(page, 0, 'Library New')
  const name = uniqueName('Library New Node')

  await page.getByTestId('element-library-new').click()
  await page.getByTestId('inline-element-adder-input').fill(name)
  await page.getByTestId('inline-element-adder-input').press('Enter')

  await expect(nodeByName(page, name)).toBeVisible()
  await expectPlacement(page, name, true)
})
