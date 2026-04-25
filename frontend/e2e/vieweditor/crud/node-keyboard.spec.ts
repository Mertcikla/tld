import { test } from '@playwright/test'
import {
  addNodeWithKeyboard,
  createDiagram,
  expectPlacement,
  prepareStorage,
  removeNodeFromPanel,
  uniqueName,
} from '../../helpers/vieweditor'

test.beforeEach(async ({ page }) => {
  await prepareStorage(page)
})

test('adds a node with the C shortcut and removes it from the element panel', async ({ page }) => {
  await createDiagram(page, uniqueName('Keyboard CRUD Diagram'))
  const nodeName = await addNodeWithKeyboard(page, uniqueName('Keyboard CRUD Node'))
  await expectPlacement(page, nodeName, true)

  await removeNodeFromPanel(page, nodeName)
  await expectPlacement(page, nodeName, false)
})
