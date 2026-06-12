import { expect, test } from '../../fixtures'
import {
  createAndLoadDiagramWithNodes,
  expectConnector,
  handleLocator,
  listConnectors,
  nodeByName,
} from '../../helpers/vieweditor'


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

test('clicking the source handle cancels keyboard connector creation', async ({ page }) => {
  const { diagram, elements } = await createAndLoadDiagramWithNodes(page, 2, 'Keyboard Connector Cancel')
  const sourceNode = nodeByName(page, elements[0].name)

  await sourceNode.click()
  await page.keyboard.press('e')
  await expect(sourceNode.getByText(/tap element to connect/)).toBeVisible()
  const handleBox = await sourceNode.locator('.react-flow__handle').first().boundingBox()
  expect(handleBox).toBeTruthy()
  await page.mouse.click(handleBox!.x + handleBox!.width / 2, handleBox!.y + handleBox!.height / 2)
  await expect(sourceNode.getByText(/tap element to connect/)).toHaveCount(0)
  await nodeByName(page, elements[1].name).click()

  await expect.poll(async () => (await listConnectors(page, diagram.id)).length).toBe(0)
})

test('Escape cancels source-handle connector creation', async ({ page }) => {
  const { diagram, elements } = await createAndLoadDiagramWithNodes(page, 2, 'Handle Connector Escape')
  const sourceNode = nodeByName(page, elements[0].name)
  const sourceHandle = handleLocator(page, elements[0].id, 'right-2')

  await sourceNode.hover()
  await sourceHandle.click()
  await expect(sourceNode.getByText(/tap element to connect/)).toBeVisible()
  await page.keyboard.press('Escape')
  await expect(sourceNode.getByText(/tap element to connect/)).toHaveCount(0)
  await nodeByName(page, elements[1].name).click()

  await expect.poll(async () => (await listConnectors(page, diagram.id)).length).toBe(0)
})
