import { expect, test } from '@playwright/test'
import {
  createAndLoadDiagramWithNodes,
  createConnector,
  expectConnector,
  listConnectors,
  openConnectorPanelFromFirstEdge,
  prepareStorage,
} from '../../helpers/vieweditor'

test.beforeEach(async ({ page }) => {
  await prepareStorage(page)
})

test('edits connector metadata, direction, and style from the connector panel', async ({ page }) => {
  const { diagram, elements } = await createAndLoadDiagramWithNodes(page, 2, 'Connector Edit')
  const connector = await createConnector(page, diagram.id, elements[0].id, elements[1].id, { label: 'initial' })
  await page.reload()
  await expect(page.getByText('initial')).toBeVisible()

  await openConnectorPanelFromFirstEdge(page)
  await page.getByTestId('connector-panel-label-input').fill('calls')
  await page.getByTestId('connector-panel-relationship-input').fill('gRPC')
  await page.getByTestId('connector-panel-url-input').fill('https://example.com/connector')
  await page.getByTestId('connector-panel-description-input').fill('Connector edited from Playwright')
  await page.getByTestId('connector-panel-direction-both').click()
  await page.getByTestId('connector-panel-style-straight').click()
  await page.getByTestId('connector-panel-description-input').blur()

  await expectConnector(page, {
    id: connector.id,
    label: 'calls',
    relationship: 'gRPC',
    direction: 'both',
    style: 'straight',
    url: 'https://example.com/connector',
    description: 'Connector edited from Playwright',
  })
})

test('deletes a connector from the connector panel', async ({ page }) => {
  const { diagram, elements } = await createAndLoadDiagramWithNodes(page, 2, 'Connector Delete')
  const connector = await createConnector(page, diagram.id, elements[0].id, elements[1].id, { label: 'remove-me' })
  await page.reload()

  await openConnectorPanelFromFirstEdge(page)
  await page.getByTestId('connector-panel-delete').click()
  await page.getByTestId('confirm-dialog-confirm').click()

  await expectConnector(page, { id: connector.id }, false)
  await expect.poll(async () => (await listConnectors(page, diagram.id)).length).toBe(0)
})
