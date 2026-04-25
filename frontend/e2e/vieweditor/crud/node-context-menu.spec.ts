import { test } from '@playwright/test'
import {
  addNodeWithCanvasContextMenu,
  createDiagram,
  expectPlacement,
  prepareStorage,
  removeSelectedNodeWithBackspace,
  uniqueName,
} from '../../helpers/vieweditor'

test.beforeEach(async ({ page }) => {
  await prepareStorage(page)
})

test('adds a node from the canvas context menu and removes it with Backspace', async ({ page }) => {
  await createDiagram(page, uniqueName('Context CRUD Diagram'))
  const nodeName = await addNodeWithCanvasContextMenu(page, uniqueName('Context CRUD Node'))
  await expectPlacement(page, nodeName, true)

  await removeSelectedNodeWithBackspace(page, nodeName)
  await expectPlacement(page, nodeName, false)
})
