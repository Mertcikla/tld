import { expect, test } from '../../fixtures'
import { readFile } from 'node:fs/promises'
import {
  createApiView,
  createAndLoadDiagramWithNodes,
  createConnector,
  createPlacedElement,
  getElement,
  gotoView,
  listConnectors,
  listPlacements,
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

test('round-trips TLD metadata through Mermaid export and import', async ({ page }) => {
  const prefix = uniqueName('Round Trip')
  const sourceView = await createApiView(page, `${prefix} Source`)
  const api = await createPlacedElement(page, sourceView.id, {
    name: `${prefix} API`,
    kind: 'service',
    description: 'Handles checkout requests',
    technology: 'Go',
    url: 'https://example.test/api',
    tags: ['backend', 'round-trip'],
    repo: 'github.com/example/shop',
    branch: 'main',
    filePath: 'cmd/api/main.go',
    language: 'go',
    technologyLinks: [{ type: 'catalog', slug: 'go', label: 'Go', isPrimaryIcon: true }],
  }, 120, 80)
  const db = await createPlacedElement(page, sourceView.id, {
    name: `${prefix} DB`,
    kind: 'database',
    description: 'Stores orders',
    technology: 'Postgres',
    tags: ['data'],
    technologyLinks: [{ type: 'catalog', slug: 'postgresql', label: 'PostgreSQL', isPrimaryIcon: true }],
  }, 460, 240)
  await createApiView(page, `${prefix} API Internals`, api.id)
  await createConnector(page, sourceView.id, api.id, db.id, {
    label: 'SQL reads',
    description: 'Read path',
    relationship: 'SQL',
    direction: 'both',
    style: 'smoothstep',
    url: 'https://example.test/runbook',
    sourceHandle: 'bottom',
    targetHandle: 'top',
  })

  await gotoView(page, sourceView.id)
  await page.getByTestId('vieweditor-toolbar-extras').click()
  await page.getByTestId('vieweditor-toolbar-export').click()
  await page.getByTestId('export-modal').getByText('Mermaid Markdown').click()
  await expect(page.getByRole('checkbox', { name: 'Include tlDiagram metadata' })).toBeChecked()
  await page.getByTestId('export-filename-input').fill(uniqueName('round-trip-metadata'))

  const downloadPromise = page.waitForEvent('download')
  await page.getByTestId('export-submit').click()
  const download = await downloadPromise
  const path = await download.path()
  if (!path) throw new Error('Expected download path')
  const content = await readFile(path, 'utf8')
  expect(content).toContain('%% tld/v1')
  expect(content).toContain('x=120 y=80')
  expect(content).toContain('tags=backend,round-trip')
  expect(content).toContain('techLinks=catalog:go:Go:1')
  expect(content).toContain('hasView=1')
  expect(content).toContain('sourceHandle=bottom')
  expect(content).toContain('targetHandle=top')

  const targetView = await createApiView(page, `${prefix} Clean Import Target`)
  await gotoView(page, targetView.id)
  await importMermaidMarkdown(page, content)

  await expect.poll(async () => (await listPlacements(page, targetView.id)).length).toBe(2)
  await expect.poll(async () => (await listConnectors(page, targetView.id)).length).toBe(1)

  const placements = await listPlacements(page, targetView.id)
  const importedApiPlacement = placements.find((placement) => placement.name === api.name)
  const importedDbPlacement = placements.find((placement) => placement.name === db.name)
  expect(importedApiPlacement).toBeDefined()
  expect(importedDbPlacement).toBeDefined()
  expect(positionX(importedApiPlacement!)).toBe(120)
  expect(positionY(importedApiPlacement!)).toBe(80)
  expect(positionX(importedDbPlacement!)).toBe(460)
  expect(positionY(importedDbPlacement!)).toBe(240)

  const importedApi = await getElement(page, importedApiPlacement!.elementId)
  expect(importedApi.id).toBe(api.id)
  expect(importedApi.kind).toBe('service')
  expect(importedApi.description).toBe('Handles checkout requests')
  expect(importedApi.technology).toBe('Go')
  expect(importedApi.url).toBe('https://example.test/api')
  expect(importedApi.tags).toEqual(['backend', 'round-trip'])
  expect(technologyLinks(importedApi)).toEqual([{ type: 'catalog', slug: 'go', label: 'Go', isPrimaryIcon: true }])
  expect(importedApi.repo).toBe('github.com/example/shop')
  expect(importedApi.branch).toBe('main')
  expect(importedApi.filePath).toBe('cmd/api/main.go')
  expect(importedApi.language).toBe('go')
  expect(importedApi.hasView).toBe(true)

  const connectors = await listConnectors(page, targetView.id)
  expect(connectors[0]).toMatchObject({
    sourceElementId: api.id,
    targetElementId: db.id,
    label: 'SQL reads',
    description: 'Read path',
    relationship: 'SQL',
    direction: 'both',
    style: 'smoothstep',
    url: 'https://example.test/runbook',
  })
  expect(connectors[0].sourceHandle).toBe('bottom')
  expect(connectors[0].targetHandle).toBe('top')

  await importMermaidMarkdown(page, content)
  await expect.poll(async () => (await listPlacements(page, targetView.id)).length).toBe(2)
  await expect.poll(async () => (await listConnectors(page, targetView.id)).length).toBe(1)
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

async function importMermaidMarkdown(page: Parameters<typeof gotoView>[0], source: string) {
  const selectionBar = page.getByTestId('vieweditor-selection-bulk-bar')
  if (await selectionBar.isVisible().catch(() => false)) {
    const box = await reactFlowPaneBox(page)
    await page.mouse.click(box.x + box.width - 24, box.y + 24)
    await expect(selectionBar).toBeHidden()
  }
  const importButton = page.getByTestId('vieweditor-toolbar-import')
  if (!await importButton.isVisible().catch(() => false)) {
    await page.getByTestId('vieweditor-toolbar-extras').click()
    await expect(importButton).toBeVisible()
  }
  await importButton.click()
  await page.getByTestId('import-mermaid-textarea').fill(source)
  await page.getByTestId('import-next').click()
  await expect(page.getByText('Elements: 2')).toBeVisible()
  await expect(page.getByText('Connectors: 1')).toBeVisible()
  await page.getByTestId('import-confirm').click()
  await expect(page.getByTestId('import-modal')).toHaveCount(0)
}

function positionX(placement: Awaited<ReturnType<typeof listPlacements>>[number]) {
  return placement.positionX
}

function positionY(placement: Awaited<ReturnType<typeof listPlacements>>[number]) {
  return placement.positionY
}

function technologyLinks(element: Awaited<ReturnType<typeof getElement>>) {
  return (element.technologyLinks ?? []).map((link) => ({
    type: link.type,
    slug: link.slug,
    label: link.label,
    isPrimaryIcon: link.isPrimaryIcon ?? false,
  }))
}
