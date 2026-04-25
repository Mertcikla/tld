import { test } from '@playwright/test'
import {
  addNodeWithToolbar,
  createDiagram,
  deleteSelectedNodeWithKeyboard,
  expectPlacement,
  prepareStorage,
  uniqueName,
} from '../../helpers/vieweditor'

test.beforeEach(async ({ page }) => {
  await prepareStorage(page)
})

test('creates a diagram, adds a node with the toolbar, and deletes it with the keyboard', async ({ page }) => {
  await createDiagram(page, uniqueName('Toolbar CRUD Diagram'))
  const nodeName = await addNodeWithToolbar(page, uniqueName('Toolbar CRUD Node'))
  await expectPlacement(page, nodeName, true)

  await deleteSelectedNodeWithKeyboard(page, nodeName)
  await expectPlacement(page, nodeName, false)
})
