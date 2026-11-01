import { expect, test } from '../../fixtures'
import {
  createAndLoadDiagramWithNodes,
  createConnector,
  expectConnector,
  listConnectors,
  nodeByName,
  openConnectorPanelFromFirstEdge,
  reloadView,
} from '../../helpers/vieweditor'


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

test('connector context menu can move source and target endpoints', async ({ page }) => {
  const { diagram, elements } = await createAndLoadDiagramWithNodes(page, 3, 'Connector Rewire')
  const connector = await createConnector(page, diagram.id, elements[0].id, elements[1].id, { label: 'rewire-me' })
  await reloadView(page)

  await page.locator('.react-flow__edge').first().click({ button: 'right', force: true })
  await page.getByTestId('connector-context-move-target').click()
  await expect(page.getByText(/Tap a node to set as new target/)).toBeVisible()
  await nodeByName(page, elements[2].name).click()

  await expectConnector(page, {
    id: connector.id,
    sourceElementId: elements[0].id,
    targetElementId: elements[2].id,
  }, true, diagram.id)

  await reloadView(page)
  await page.locator('.react-flow__edge').first().click({ button: 'right', force: true })
  await page.getByTestId('connector-context-move-source').click()
  await expect(page.getByText(/Tap a node to set as new source/)).toBeVisible()
  await nodeByName(page, elements[1].name).click()

  await expect.poll(async () => {
    const connectors = await listConnectors(page, diagram.id)
    const updated = connectors.find((candidate) => candidate.id === connector.id)
    return updated ? [updated.sourceElementId, updated.targetElementId] : []
  }).toEqual([elements[1].id, elements[2].id])

  await reloadView(page)
  await expectConnector(page, {
    id: connector.id,
    sourceElementId: elements[1].id,
    targetElementId: elements[2].id,
  }, true, diagram.id)
})

test('used handles stay hidden at rest and reveal without moving endpoints on connector hover or selection', async ({ page }) => {
  const { diagram, elements } = await createAndLoadDiagramWithNodes(page, 2, 'Handle Visibility')
  await createConnector(page, diagram.id, elements[0].id, elements[1].id, { sourceHandle: 'right-2', targetHandle: 'left-2' })
  await reloadView(page)
  const handles = page.locator('.react-flow__handle.handle-connected')
  const stroke = page.locator('.vieweditor-connector > .react-flow__edge-path').first()
  const interaction = page.locator('.vieweditor-connector .react-flow__edge-interaction').first()
  await expect(handles).toHaveCount(2)
  await page.mouse.move(10, 10)
  for (const handle of await handles.all()) await expect(handle).toHaveCSS('opacity', '0')
  const restingPath = await stroke.getAttribute('d')
  await interaction.hover({ force: true })
  for (const handle of await handles.all()) await expect(handle).toHaveCSS('opacity', '1')
  await expect(stroke).toHaveAttribute('d', restingPath!)
  await page.mouse.move(10, 10)
  for (const handle of await handles.all()) await expect(handle).toHaveCSS('opacity', '0')
  await expect(stroke).toHaveAttribute('d', restingPath!)
  await openConnectorPanelFromFirstEdge(page)
  await page.mouse.move(10, 10)
  for (const handle of await handles.all()) await expect(handle).toHaveCSS('opacity', '1')
  await expect(stroke).toHaveAttribute('d', restingPath!)
})
