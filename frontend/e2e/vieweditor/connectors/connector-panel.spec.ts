import { expect, test } from '../../fixtures'
import {
  createAndLoadDiagramWithNodes,
  createConnector,
  expectConnector,
  listConnectors,
  openConnectorPanelFromFirstEdge,
} from '../../helpers/vieweditor'


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

test('clears a connector label and keeps it empty after reload', async ({ page }) => {
  const { diagram, elements } = await createAndLoadDiagramWithNodes(page, 2, 'Connector Clear Label')
  const connector = await createConnector(page, diagram.id, elements[0].id, elements[1].id, { label: 'initial' })
  await page.reload()

  await openConnectorPanelFromFirstEdge(page)
  await page.getByTestId('connector-panel-label-input').fill('')
  await page.getByTestId('connector-panel-label-input').blur()

  await expectConnector(page, { id: connector.id, label: '' }, true, diagram.id)

  await page.reload()
  await openConnectorPanelFromFirstEdge(page)
  await expect(page.getByTestId('connector-panel-label-input')).toHaveValue('')
  await expect(page.getByText('initial')).toHaveCount(0)
})

test('keeps URL input while an earlier connector autosave completes', async ({ page }) => {
  const { diagram, elements } = await createAndLoadDiagramWithNodes(page, 2, 'Connector URL Autosave')
  const connector = await createConnector(page, diagram.id, elements[0].id, elements[1].id, { label: 'initial' })
  await page.reload()
  await openConnectorPanelFromFirstEdge(page)

  let releaseSave!: () => void
  const saveGate = new Promise<void>((resolve) => { releaseSave = resolve })
  let markSaving!: () => void
  const saving = new Promise<void>((resolve) => { markSaving = resolve })
  await page.route('**/diag.v1.WorkspaceService/UpdateConnector', async (route) => {
    const response = await route.fetch()
    markSaving()
    await saveGate
    await route.fulfill({ response })
  }, { times: 1 })

  try {
    await page.getByTestId('connector-panel-label-input').fill('calls')
    await page.getByTestId('connector-panel-label-input').blur()
    await saving

    const url = page.getByTestId('connector-panel-url-input')
    const nextURL = 'https://example.com/connector'
    // Exercise input delivery even when the controlled input's value tracker
    // already contains the browser-filled value.
    await url.evaluate((element, value) => {
      const input = element as HTMLInputElement
      input.value = value
      input.dispatchEvent(new InputEvent('input', { bubbles: true, inputType: 'insertText', data: value }))
    }, nextURL)
    releaseSave()
    await expect(url).toHaveValue(nextURL)
    await expectConnector(page, { id: connector.id, label: 'calls', url: nextURL }, true, diagram.id)

    await page.reload()
    await openConnectorPanelFromFirstEdge(page)
    await expect(page.getByTestId('connector-panel-url-input')).toHaveValue(nextURL)
  } finally {
    releaseSave()
  }
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
