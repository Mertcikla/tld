import { expect, test } from '../../fixtures'
import { readFile } from 'node:fs/promises'
import {
  addExistingFromLibrary,
  addNodeWithToolbar,
  createAndLoadDiagramWithNodes,
  createApiView,
  createConnector,
  createElement,
  expectConnector,
  getElement,
  listElements,
  nodeByName,
  openElementPanel,
  uniqueName,
} from '../../helpers/vieweditor'


test('creates edits connects tags reloads and exports a small diagram', async ({ page }) => {
  const { diagram, elements } = await createAndLoadDiagramWithNodes(page, 1, 'Full Workflow')
  const secondName = await addNodeWithToolbar(page, uniqueName('Full Workflow Added'))
  await expect.poll(async () => (await listElements(page, secondName)).length).toBeGreaterThan(0)
  const second = (await listElements(page, secondName))[0]

  await createConnector(page, diagram.id, elements[0].id, second.id, { label: 'full-flow' })
  await openElementPanel(page, secondName)
  await page.getByTestId('element-panel-description-input').fill('Edited inside a multi-action E2E workflow')
  await page.getByTestId('tag-upsert-input').fill('workflow-tag')
  await page.getByTestId('tag-upsert-input').press('Enter')
  await expect(page.getByTestId('element-panel-tag-chip').filter({ hasText: 'workflow-tag' }).first()).toBeVisible()
  await expect.poll(async () => (await getElement(page, second.id)).tags?.includes('workflow-tag')).toBeTruthy()
  await page.getByTestId('element-panel-url-input').blur()
  await page.reload()

  await expect(nodeByName(page, secondName)).toBeVisible()
  await expectConnector(page, { label: 'full-flow' }, true, diagram.id)
  await expect.poll(async () => (await getElement(page, second.id)).tags?.includes('workflow-tag')).toBeTruthy()

  await page.getByTestId('vieweditor-toolbar-extras').click()
  await page.getByTestId('vieweditor-toolbar-export').click()
  await page.getByTestId('export-modal').getByText('Mermaid Markdown').click()
  await page.getByTestId('export-filename-input').fill(uniqueName('full-flow-export'))
  const downloadPromise = page.waitForEvent('download')
  await page.getByTestId('export-submit').click()
  const download = await downloadPromise
  const path = await download.path()
  if (!path) throw new Error('Expected download path')
  const content = await readFile(path, 'utf8')
  expect(content).toContain(secondName)
  expect(content).toContain('flowchart LR')
  expect(content).toMatch(/node_\d+ -- "full-flow" --> node_\d+/)
})

test('builds parent and child views and navigates through editor controls', async ({ page }) => {
  const { diagram, elements } = await createAndLoadDiagramWithNodes(page, 1, 'Parent Child Flow')
  const child = await createApiView(page, uniqueName('Parent Child Detail'), elements[0].id)
  await page.reload()

  await nodeByName(page, elements[0].name).getByTestId('vieweditor-node-zoom-in').click()
  await expect(page).toHaveURL(new RegExp(`/views/${child.id}$`))
  await page.keyboard.press('w')
  await expect(page).toHaveURL(new RegExp(`/views/${diagram.id}$`))
})

test('places an existing imported element in another diagram and finds it from the library', async ({ page }) => {
  await createAndLoadDiagramWithNodes(page, 0, 'Imported Placement Source')
  await page.getByTestId('vieweditor-toolbar-extras').click()
  await page.getByTestId('vieweditor-toolbar-import').click()
  await page.getByTestId('import-mermaid-textarea').fill('flowchart LR\n  CrossImportA --> CrossImportB')
  await page.getByTestId('import-next').click()
  await page.getByTestId('import-confirm').click()
  await expect(page.getByText('CrossImportA').first()).toBeVisible()
  const importedViewURL = page.url()

  const target = await createApiView(page, uniqueName('Imported Placement Target'))
  await page.goto(`/views/${target.id}`)
  await addExistingFromLibrary(page, 'CrossImportA')

  await expect(nodeByName(page, 'CrossImportA')).toBeVisible()
  await page.goto(importedViewURL)
  await expect(page.getByText('CrossImportA').first()).toBeVisible()
})

test('created connector relationship appears in Inventory inspect graph', async ({ page }) => {
  const diagram = await createApiView(page, uniqueName('Workflow Inventory'))
  const source = await createElement(page, { name: uniqueName('Workflow Connector Source'), kind: 'service' })
  const target = await createElement(page, { name: uniqueName('Workflow Connector Target'), kind: 'database' })
  await page.request.post('/api/diag.v1.WorkspaceService/CreatePlacement', { data: { viewId: diagram.id, elementId: source.id, positionX: 140, positionY: 140 } })
  await page.request.post('/api/diag.v1.WorkspaceService/CreatePlacement', { data: { viewId: diagram.id, elementId: target.id, positionX: 420, positionY: 140 } })
  await createConnector(page, diagram.id, source.id, target.id, { label: 'feeds' })

  await page.goto(`/dependencies?element=${source.id}`)

  await expect(page).toHaveURL(/\/inventory\?/) 
  await expect.poll(() => new URL(page.url()).searchParams.get('object')).toBe(`element:${source.id}`)
  const connectorCard = page.locator('[data-testid="inventory-connector-card"], [data-testid="dependencies-neighbour-card"]').filter({ hasText: 'database' }).first()
  await expect(connectorCard).toBeVisible()
})
