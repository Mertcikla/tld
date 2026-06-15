import { expect, test } from '../../fixtures'
import { readFile } from 'node:fs/promises'
import {
  createAndLoadDiagramWithNodes,
  createConnector,
  reactFlowPaneBox,
  uniqueName,
} from '../../helpers/vieweditor'


test('exported Mermaid download contains node names and edge syntax', async ({ page }) => {
  const { diagram, elements } = await createAndLoadDiagramWithNodes(page, 2, 'Export Content')
  await createConnector(page, diagram.id, elements[0].id, elements[1].id, { label: 'exports-to' })
  await page.reload()

  await page.getByTestId('vieweditor-toolbar-extras').click()
  await page.getByTestId('vieweditor-toolbar-export').click()
  await page.getByTestId('export-modal').getByText('Mermaid Markdown').click()
  await page.getByTestId('export-filename-input').fill(uniqueName('export-content'))

  const downloadPromise = page.waitForEvent('download')
  await page.getByTestId('export-submit').click()
  const download = await downloadPromise
  const path = await download.path()
  if (!path) throw new Error('Expected download path')
  const content = await readFile(path, 'utf8')

  expect(content).toContain(elements[0].name)
  expect(content).toContain(elements[1].name)
  expect(content).toContain('flowchart LR')
  expect(content).toMatch(/node_\d+ -- "exports-to" --> node_\d+/)
})

test('canvas context menu copies Mermaid directly', async ({ page }) => {
  const { diagram, elements } = await createAndLoadDiagramWithNodes(page, 2, 'Context Export Content')
  await createConnector(page, diagram.id, elements[0].id, elements[1].id)
  await page.reload()
  await page.evaluate(() => {
    Object.defineProperty(navigator, 'clipboard', {
      configurable: true,
      value: {
        writeText: async (text: string) => {
          (window as typeof window & { __tldCopiedText?: string }).__tldCopiedText = text
        },
      },
    })
  })

  const box = await reactFlowPaneBox(page)
  await page.mouse.click(box.x + box.width * 0.5, box.y + box.height * 0.15, { button: 'right' })

  await page.getByTestId('vieweditor-canvas-context-copy-mermaid').click()
  await expect(page.getByText('Copied Mermaid').first()).toBeVisible()
  const content = await page.evaluate(() => (window as typeof window & { __tldCopiedText?: string }).__tldCopiedText ?? '')
  expect(content).toContain('flowchart LR')
  expect(content).toContain(elements[0].name)
  expect(content).toContain(elements[1].name)
})

test('export cancel closes the modal without downloading', async ({ page }) => {
  await createAndLoadDiagramWithNodes(page, 1, 'Export Cancel')

  await page.getByTestId('vieweditor-toolbar-extras').click()
  await page.getByTestId('vieweditor-toolbar-export').click()
  await expect(page.getByTestId('export-modal')).toBeVisible()
  await page.getByTestId('export-cancel').click()

  await expect(page.getByTestId('export-modal')).toHaveCount(0)
})

test('import parse error keeps the import modal open', async ({ page }) => {
  await createAndLoadDiagramWithNodes(page, 0, 'Import Error')

  await page.getByTestId('vieweditor-toolbar-extras').click()
  await page.getByTestId('vieweditor-toolbar-import').click()
  await page.getByTestId('import-mermaid-textarea').fill('this is not a diagram')
  await page.getByTestId('import-next').click()

  await expect(page.getByTestId('import-modal')).toBeVisible()
  await expect(page.getByText(/Failed to parse|Unsupported|Invalid|Unable to detect/i)).toBeVisible()
})

test('import back preserves the current Mermaid text for editing', async ({ page }) => {
  await createAndLoadDiagramWithNodes(page, 0, 'Import Back Preserve')
  const source = 'flowchart LR\n  PreserveA --> PreserveB'

  await page.getByTestId('vieweditor-toolbar-extras').click()
  await page.getByTestId('vieweditor-toolbar-import').click()
  await page.getByTestId('import-mermaid-textarea').fill(source)
  await page.getByTestId('import-next').click()
  await expect(page.getByText('Elements: 2')).toBeVisible()
  await page.getByTestId('import-back').click()

  await expect(page.getByTestId('import-mermaid-textarea')).toHaveValue(source)
})
