import { expect, test } from '../../fixtures'
import {
  addNodeWithToolbar,
  createAndLoadDiagramWithNodes,
  createDiagram,
  expectPlacement,
  nodeByName,
  openElementLibrary,
  openViewExplorer,
  uniqueName,
} from '../../helpers/vieweditor'


test('empty editor shows a usable canvas and primary panels', async ({ page }) => {
  await createDiagram(page, uniqueName('Empty Editor'))

  await expect(page.getByTestId('vieweditor-canvas')).toBeVisible()
  await openElementLibrary(page)
  await openViewExplorer(page)
  await expect(page.getByTestId('element-library-panel')).toBeVisible()
  await expect(page.getByTestId('view-explorer-panel')).toBeVisible()
})

test('toolbar extras open import and export actions', async ({ page }) => {
  await createDiagram(page, uniqueName('Toolbar Extras'))

  await page.getByTestId('vieweditor-toolbar-extras').click()
  await expect(page.getByTestId('vieweditor-toolbar-import')).toBeVisible()
  await expect(page.getByTestId('vieweditor-toolbar-export')).toBeVisible()
})

test('keyboard add creates a node with a durable placement', async ({ page }) => {
  await createDiagram(page, uniqueName('Undo Redo Node'))
  const name = await addNodeWithToolbar(page, uniqueName('Undoable Node'))

  await expect(nodeByName(page, name)).toBeVisible()
  await expectPlacement(page, name, true)
})

test('element panel can be closed without clearing the node placement', async ({ page }) => {
  const { elements } = await createAndLoadDiagramWithNodes(page, 1, 'Panel Close')

  await nodeByName(page, elements[0].name).click()
  await expect(page.getByTestId('element-panel')).toBeVisible()
  await page.getByTestId('panel-close').click()

  await expect(page.getByTestId('element-panel')).toHaveCount(0)
  await expect(nodeByName(page, elements[0].name)).toBeVisible()
})

test('new node survives a reload after creation', async ({ page }) => {
  await createDiagram(page, uniqueName('Reload Placement'))
  const name = await addNodeWithToolbar(page, uniqueName('Reloaded Node'))

  await page.reload()

  await expect(page.getByTestId('vieweditor-canvas')).toBeVisible()
  await expect(nodeByName(page, name)).toBeVisible()
})

test('clicking a second node switches panel selection', async ({ page }) => {
  const { elements } = await createAndLoadDiagramWithNodes(page, 2, 'Panel Switch')

  await nodeByName(page, elements[0].name).click()
  await expect(page.getByTestId('element-panel-name-input')).toHaveValue(elements[0].name)
  await nodeByName(page, elements[1].name).click()

  await expect(page.getByTestId('element-panel-name-input')).toHaveValue(elements[1].name)
})

test('canceling inline add from keyboard leaves no new placement', async ({ page }) => {
  await createDiagram(page, uniqueName('Inline Cancel'))

  await page.getByTestId('vieweditor-canvas').click()
  await page.keyboard.press('c')
  await page.getByTestId('pending-element-label-input').fill('Should Not Exist')
  await page.keyboard.press('Escape')

  await expect(page.getByTestId('pending-element-label-input')).toHaveCount(0)
  await expect(nodeByName(page, 'Should Not Exist')).toHaveCount(0)
})
