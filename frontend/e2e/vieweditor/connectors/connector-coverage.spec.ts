import { expect, test } from '@playwright/test'
import {
  createAndLoadDiagramWithNodes,
  createConnector,
  expectConnector,
  listConnectors,
  nodeByName,
  openConnectorPanelFromFirstEdge,
  prepareStorage,
} from '../../helpers/vieweditor'

test.beforeEach(async ({ page }) => {
  await prepareStorage(page)
})

test('delete confirmation can be canceled without removing the connector', async ({ page }) => {
  const { diagram, elements } = await createAndLoadDiagramWithNodes(page, 2, 'Connector Cancel Delete')
  const connector = await createConnector(page, diagram.id, elements[0].id, elements[1].id, { label: 'keep-me' })
  await page.reload()

  await openConnectorPanelFromFirstEdge(page)
  await page.getByTestId('connector-panel-delete').click()
  await page.getByTestId('confirm-dialog-cancel').click()

  await expectConnector(page, { id: connector.id }, true, diagram.id)
})

test('style buttons persist each supported connector route style', async ({ page }) => {
  const { diagram, elements } = await createAndLoadDiagramWithNodes(page, 2, 'Connector Style Matrix')
  const connector = await createConnector(page, diagram.id, elements[0].id, elements[1].id, { label: 'style-me' })
  await page.reload()

  await openConnectorPanelFromFirstEdge(page)
  for (const style of ['smoothstep', 'bezier', 'straight', 'step']) {
    await page.getByTestId(`connector-panel-style-${style}`).click()
    await page.getByTestId('connector-panel-description-input').blur()
    await expectConnector(page, { id: connector.id, style }, true, diagram.id)
  }
})

test('direction buttons persist each supported connector direction', async ({ page }) => {
  const { diagram, elements } = await createAndLoadDiagramWithNodes(page, 2, 'Connector Direction Matrix')
  const connector = await createConnector(page, diagram.id, elements[0].id, elements[1].id, { label: 'direction-me' })
  await page.reload()

  await openConnectorPanelFromFirstEdge(page)
  for (const direction of ['forward', 'backward', 'both', 'none']) {
    await page.getByTestId(`connector-panel-direction-${direction}`).click()
    await page.getByTestId('connector-panel-description-input').blur()
    await expectConnector(page, { id: connector.id, direction }, true, diagram.id)
  }
})

test('Escape cancels keyboard connector creation', async ({ page }) => {
  const { diagram, elements } = await createAndLoadDiagramWithNodes(page, 2, 'Connector Escape')

  await nodeByName(page, elements[0].name).click()
  await page.keyboard.press('e')
  await expect(nodeByName(page, elements[0].name).getByText(/tap element to connect/)).toBeVisible()
  await page.keyboard.press('Escape')
  await expect(nodeByName(page, elements[0].name).getByText(/tap element to connect/)).toHaveCount(0)
  await nodeByName(page, elements[1].name).click()

  await expect.poll(async () => (await listConnectors(page, diagram.id)).length).toBe(0)
})

test('connector metadata remains visible after reload', async ({ page }) => {
  const { diagram, elements } = await createAndLoadDiagramWithNodes(page, 2, 'Connector Reload')
  await createConnector(page, diagram.id, elements[0].id, elements[1].id, { label: 'reload label', relationship: 'HTTP' })
  await page.reload()

  await expect(page.getByText('reload label')).toBeVisible()
  await openConnectorPanelFromFirstEdge(page)
  await expect(page.getByTestId('connector-panel-label-input')).toHaveValue('reload label')
  await expect(page.getByTestId('connector-panel-relationship-input')).toHaveValue('HTTP')
})
