import { expect, test } from '../../fixtures'
import {
  createAndLoadDiagramWithNodes,
  createConnector,
  currentViewId,
  nodeByName,
  pasteTextOnCanvas,
  uniqueName,
} from '../../helpers/vieweditor'


test('imports a Mermaid diagram through the import modal', async ({ page }) => {
  await createAndLoadDiagramWithNodes(page, 0, 'Import Host')

  await page.getByTestId('vieweditor-toolbar-extras').click()
  await page.getByTestId('vieweditor-toolbar-import').click()
  await expect(page.getByTestId('import-modal')).toBeVisible()

  await page.getByTestId('import-mermaid-textarea').fill(`flowchart LR
  E2EA[Import API] --> E2EB[Import DB]`)
  await page.getByTestId('import-next').click()
  await expect(page.getByText('Elements: 2')).toBeVisible()
  await expect(page.getByText('Connectors: 1')).toBeVisible()
  await page.getByTestId('import-confirm').click()

  await expect(page).toHaveURL(/\/views\/\d+$/)
  await expect(page.getByText('Import API').first()).toBeVisible()
  await expect(page.getByText('Import DB').first()).toBeVisible()
})

test('export modal changes options and downloads Mermaid output', async ({ page }) => {
  const { diagram, elements } = await createAndLoadDiagramWithNodes(page, 2, 'Export Diagram')
  await createConnector(page, diagram.id, elements[0].id, elements[1].id)
  await page.reload()

  await page.getByTestId('vieweditor-toolbar-extras').click()
  await page.getByTestId('vieweditor-toolbar-export').click()
  await expect(page.getByTestId('export-modal')).toBeVisible()
  await page.getByText('Mermaid').click()
  const filename = uniqueName('export-mermaid')
  await page.getByTestId('export-filename-input').fill(filename)

  const downloadPromise = page.waitForEvent('download')
  await page.getByTestId('export-submit').click()
  const download = await downloadPromise
  expect(download.suggestedFilename()).toBe(`${filename}.mermaid`)
})

test('pasting fenced Mermaid imports into the open view', async ({ page }) => {
  await createAndLoadDiagramWithNodes(page, 0, 'Paste Import Host')
  const viewId = currentViewId(page)
  const source = `Some markdown before.

\`\`\`mermaid
flowchart LR
  PasteA[Paste API] --> PasteB[Paste DB]
\`\`\`
`

  await pasteTextOnCanvas(page, source)

  await expect(page).toHaveURL(new RegExp(`/views/${viewId}$`))
  await expect(nodeByName(page, 'Paste API')).toBeVisible()
  await expect(nodeByName(page, 'Paste DB')).toBeVisible()
  await expect(page.getByTestId('vieweditor-selection-bulk-bar')).toContainText('2 selected')
  await expect.poll(async () => nodeByName(page, 'Paste API').evaluate((node) => node.closest('.react-flow__node')?.classList.contains('selected'))).toBe(true)
  await expect.poll(async () => nodeByName(page, 'Paste DB').evaluate((node) => node.closest('.react-flow__node')?.classList.contains('selected'))).toBe(true)
})

test('pasting architecture-beta Mermaid imports into the open view', async ({ page }) => {
  await createAndLoadDiagramWithNodes(page, 0, 'Architecture Paste Host')
  const viewId = currentViewId(page)
  const source = `architecture-beta
    service left_disk(disk)[Disk]
    service top_disk(disk)[Disk]
    service bottom_disk(disk)[Disk]
    service top_gateway(internet)[Gateway]
    service bottom_gateway(internet)[Gateway]
    junction junctionCenter
    junction junctionRight

    left_disk:R -- L:junctionCenter
    top_disk:B -- T:junctionCenter
    bottom_disk:T -- B:junctionCenter
    junctionCenter:R -- L:junctionRight
    top_gateway:B -- T:junctionRight
    bottom_gateway:T -- B:junctionRight`

  await pasteTextOnCanvas(page, source)

  await expect(page).toHaveURL(new RegExp(`/views/${viewId}$`))
  await expect(page.getByTestId('vieweditor-node').filter({ hasText: 'Disk' })).toHaveCount(3)
  await expect(page.getByTestId('vieweditor-node').filter({ hasText: 'Gateway' })).toHaveCount(2)
  await expect(nodeByName(page, 'junctionCenter')).toBeVisible()
  await expect(nodeByName(page, 'junctionRight')).toBeVisible()
  await expect(page.getByTestId('vieweditor-selection-bulk-bar')).toContainText('7 selected')
})

test('pasting non-Mermaid text is ignored by canvas import', async ({ page }) => {
  await createAndLoadDiagramWithNodes(page, 0, 'Paste Ignore Host')
  const viewId = currentViewId(page)

  await pasteTextOnCanvas(page, 'this is not a diagram')

  await expect(page).toHaveURL(new RegExp(`/views/${viewId}$`))
  await expect(page.getByTestId('vieweditor-node')).toHaveCount(0)
})

test('import modal can parse invalid text without creating resources by canceling', async ({ page }) => {
  await createAndLoadDiagramWithNodes(page, 0, 'Import Cancel')
  await page.getByTestId('vieweditor-toolbar-extras').click()
  await page.getByTestId('vieweditor-toolbar-import').click()

  await page.getByTestId('import-mermaid-textarea').fill('flowchart LR\n  A --> B')
  await page.getByTestId('import-next').click()
  await expect(page.getByText('Elements: 2')).toBeVisible()
  await page.getByTestId('import-back').click()
  await expect(page.getByTestId('import-mermaid-textarea')).toBeVisible()
  await page.getByTestId('import-cancel').click()
  await expect(page.getByTestId('import-modal')).toHaveCount(0)
})
