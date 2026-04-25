import { expect, test } from '@playwright/test'
import {
  createAndLoadDiagramWithNodes,
  expectConnector,
  nodeByName,
  prepareStorage,
} from '../../helpers/vieweditor'

test.beforeEach(async ({ page }) => {
  await prepareStorage(page)
})

test('creates a connector with the E shortcut and target click flow', async ({ page }) => {
  const { diagram, elements } = await createAndLoadDiagramWithNodes(page, 2, 'Keyboard Connector')

  await nodeByName(page, elements[0].name).click()
  await page.keyboard.press('e')
  await nodeByName(page, elements[1].name).click()

  await expectConnector(page, {
    sourceElementId: elements[0].id,
    targetElementId: elements[1].id,
  }, true, diagram.id)
  await expect(page.locator('.react-flow__edge')).toHaveCount(1)
})
