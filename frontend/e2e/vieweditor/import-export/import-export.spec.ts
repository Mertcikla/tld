import { expect, test } from '@playwright/test'
import {
  createAndLoadDiagramWithNodes,
  createConnector,
  prepareStorage,
  uniqueName,
} from '../../helpers/vieweditor'

test.beforeEach(async ({ page }) => {
  await prepareStorage(page)
})

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
