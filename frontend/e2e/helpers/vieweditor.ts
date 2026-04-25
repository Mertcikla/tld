import { expect, type Locator, type Page } from '@playwright/test'

export const onboardingStorage = {
  editor: 'diagrameditor_tutorial_v1_core',
  viewGrid: 'viewgrid_tutorial_v2_core',
  shown: 'onboarding_shown',
}

export async function prepareStorage(page: Page) {
  await page.addInitScript((keys) => {
    localStorage.setItem(keys.editor, '1')
    localStorage.setItem(keys.viewGrid, '1')
    localStorage.setItem(keys.shown, '1')
    localStorage.setItem('diag:libraryOpen', 'true')
    localStorage.setItem('diag:explorerOpen', 'true')
    localStorage.setItem('diag:snapToGrid', 'false')
  }, onboardingStorage)
}

export function uniqueName(prefix: string) {
  return `${prefix} ${Date.now()} ${Math.random().toString(36).slice(2, 8)}`
}

export async function createDiagram(page: Page, name = uniqueName('E2E Diagram')) {
  await page.goto('/views?view=hierarchy')
  await page.getByTestId('views-new-diagram-button').click()
  await page.getByTestId('views-new-diagram-name-input').fill(name)
  await page.getByTestId('views-create-diagram-submit').click()
  await expect(page).toHaveURL(/\/views\/\d+$/)
  await expect(page.getByTestId('vieweditor-canvas')).toBeVisible()
  return { name, id: currentViewId(page) }
}

export function currentViewId(page: Page) {
  const match = page.url().match(/\/views\/(\d+)/)
  if (!match) throw new Error(`Expected /views/:id URL, got ${page.url()}`)
  return Number(match[1])
}

export function nodeByName(page: Page, name: string): Locator {
  return page.getByTestId('vieweditor-node').filter({ hasText: name })
}

export function libraryItemByName(page: Page, name: string): Locator {
  return page.getByTestId('element-library-item').filter({ hasText: name }).first()
}

export async function reactFlowPaneBox(page: Page) {
  const pane = page.locator('.react-flow__pane')
  const box = await pane.boundingBox()
  if (!box) throw new Error('React Flow pane is not visible')
  return box
}

export async function addNodeWithToolbar(page: Page, name = uniqueName('Toolbar Node')) {
  await page.getByTestId('vieweditor-toolbar-add-element').click()
  await confirmInlineNewElement(page, name)
  await expect(nodeByName(page, name)).toBeVisible()
  return name
}

export async function addNodeWithKeyboard(page: Page, name = uniqueName('Keyboard Node')) {
  await page.getByTestId('vieweditor-canvas').click()
  await page.keyboard.press('c')
  await confirmInlineNewElement(page, name)
  await expect(nodeByName(page, name)).toBeVisible()
  return name
}

export async function addNodeWithCanvasContextMenu(page: Page, name = uniqueName('Context Node')) {
  const box = await reactFlowPaneBox(page)

  await page.mouse.click(box.x + box.width * 0.52, box.y + box.height * 0.42, { button: 'right' })
  await page.getByTestId('vieweditor-canvas-context-add-element').click()
  await confirmInlineNewElement(page, name)
  await expect(nodeByName(page, name)).toBeVisible()
  return name
}

export async function addExistingNodeWithInlineSearch(page: Page, name: string) {
  await page.getByTestId('vieweditor-toolbar-add-element').click()
  const input = page.getByTestId('inline-element-adder-input')
  await input.fill(name)
  await expect(page.getByTestId('inline-element-adder-existing-option').filter({ hasText: name }).first()).toBeVisible()
  await page.keyboard.press('ArrowDown')
  await page.keyboard.press('Enter')
  await expect(nodeByName(page, name)).toBeVisible()
}

export async function confirmInlineNewElement(page: Page, name: string) {
  const input = page.getByTestId('inline-element-adder-input')
  await expect(input).toBeVisible()
  await input.fill(name)
  await expect(page.getByTestId('inline-element-adder-create-option').filter({ hasText: name }).first()).toBeVisible()
  await input.press('Enter')
}

export async function deleteSelectedNodeWithKeyboard(page: Page, name: string) {
  await nodeByName(page, name).click()
  await page.keyboard.press('Delete')
  await expect(nodeByName(page, name)).toHaveCount(0)
}

export async function removeNodeFromPanel(page: Page, name: string) {
  await nodeByName(page, name).click()
  await expect(page.getByTestId('element-panel')).toBeVisible()
  await page.getByTestId('element-panel-remove').click()
  await expect(nodeByName(page, name)).toHaveCount(0)
}

export async function removeSelectedNodeWithBackspace(page: Page, name: string) {
  await nodeByName(page, name).click()
  await page.keyboard.press('Backspace')
  await expect(nodeByName(page, name)).toHaveCount(0)
}

export async function listPlacements(page: Page, viewId = currentViewId(page)) {
  const response = await page.request.post('/api/diag.v1.WorkspaceService/ListPlacements', {
    data: { viewId },
  })
  expect(response.ok()).toBeTruthy()
  const json = await response.json()
  return (json.placements ?? []) as Array<{ id: number; viewId: number; elementId: number; name: string }>
}

export async function expectPlacement(page: Page, name: string, visible: boolean, viewId = currentViewId(page)) {
  await expect.poll(async () => {
    const placements = await listPlacements(page, viewId)
    return placements.some((placement) => placement.name === name)
  }).toBe(visible)
}

export async function createElement(page: Page, data: {
  name: string
  kind?: string
  description?: string
  technology?: string
  url?: string
  tags?: string[]
}) {
  const response = await page.request.post('/api/diag.v1.WorkspaceService/CreateElement', {
    data: {
      name: data.name,
      kind: data.kind ?? '',
      description: data.description ?? '',
      technology: data.technology ?? '',
      url: data.url ?? '',
      tags: data.tags ?? [],
    },
  })
  expect(response.ok()).toBeTruthy()
  const json = await response.json()
  return json.element as { id: number; name: string; kind?: string; tags?: string[] }
}

export async function getElement(page: Page, elementId: number) {
  const response = await page.request.post('/api/diag.v1.WorkspaceService/GetElement', {
    data: { elementId },
  })
  expect(response.ok()).toBeTruthy()
  const json = await response.json()
  return json.element as {
    id: number
    name: string
    kind?: string
    description?: string
    technology?: string
    url?: string
    tags?: string[]
    technology_connectors?: Array<{ type: string; slug?: string; label: string; is_primary_icon?: boolean }>
  }
}

export async function addPlacement(page: Page, viewId: number, elementId: number, x = 120, y = 140) {
  const response = await page.request.post('/api/diag.v1.WorkspaceService/CreatePlacement', {
    data: { viewId, elementId, positionX: x, positionY: y },
  })
  expect(response.ok()).toBeTruthy()
  const json = await response.json()
  return json.placement as { id: number; viewId: number; elementId: number; positionX?: number; positionY?: number }
}

export async function createPlacedElement(page: Page, viewId: number, data: Parameters<typeof createElement>[1], x = 120, y = 140) {
  const element = await createElement(page, data)
  await addPlacement(page, viewId, element.id, x, y)
  return element
}

export async function createApiView(page: Page, name = uniqueName('API Diagram'), ownerElementId?: number) {
  const response = await page.request.post('/api/diag.v1.WorkspaceService/CreateView', {
    data: { name, ownerElementId },
  })
  expect(response.ok()).toBeTruthy()
  const json = await response.json()
  return json.view as { id: number; name: string; ownerElementId?: number | null }
}

export async function gotoView(page: Page, viewId: number) {
  await page.goto(`/views/${viewId}`)
  await expect(page.getByTestId('vieweditor-canvas')).toBeVisible()
}

export async function reloadView(page: Page) {
  await page.reload()
  await expect(page.getByTestId('vieweditor-canvas')).toBeVisible()
}

export async function listConnectors(page: Page, viewId = currentViewId(page)) {
  const response = await page.request.post('/api/diag.v1.WorkspaceService/ListConnectors', {
    data: { viewId },
  })
  expect(response.ok()).toBeTruthy()
  const json = await response.json()
  return (json.connectors ?? []) as Array<{
    id: number
    viewId: number
    sourceElementId: number
    targetElementId: number
    label?: string
    description?: string
    relationship?: string
    direction?: string
    style?: string
    url?: string
  }>
}

export async function createConnector(page: Page, viewId: number, sourceElementId: number, targetElementId: number, data: {
  label?: string
  description?: string
  relationship?: string
  direction?: string
  style?: string
  url?: string
} = {}) {
  const response = await page.request.post('/api/diag.v1.WorkspaceService/CreateConnector', {
    data: {
      viewId,
      sourceElementId,
      targetElementId,
      direction: data.direction ?? 'forward',
      style: data.style ?? 'bezier',
      label: data.label ?? '',
      description: data.description ?? '',
      relationship: data.relationship ?? '',
      url: data.url ?? '',
    },
  })
  expect(response.ok()).toBeTruthy()
  const json = await response.json()
  return json.connector as Awaited<ReturnType<typeof listConnectors>>[number]
}

export async function expectConnector(page: Page, matcher: Partial<Awaited<ReturnType<typeof listConnectors>>[number]>, visible = true, viewId = currentViewId(page)) {
  await expect.poll(async () => {
    const connectors = await listConnectors(page, viewId)
    return connectors.some((connector) =>
      Object.entries(matcher).every(([key, value]) => connector[key as keyof typeof connector] === value)
    )
  }).toBe(visible)
}

export async function listLayers(page: Page, viewId = currentViewId(page)) {
  const response = await page.request.post('/api/diag.v1.WorkspaceService/ListViewLayers', {
    data: { viewId },
  })
  expect(response.ok()).toBeTruthy()
  const json = await response.json()
  return (json.layers ?? []) as Array<{ id: number; viewId: number; name: string; tags: string[]; color: string }>
}

export async function createLayer(page: Page, viewId: number, data: { name: string; tags: string[]; color?: string }) {
  const response = await page.request.post('/api/diag.v1.WorkspaceService/CreateViewLayer', {
    data: { viewId, name: data.name, tags: data.tags, color: data.color ?? '#38BDF8' },
  })
  expect(response.ok()).toBeTruthy()
  const json = await response.json()
  return json.layer as { id: number; viewId: number; name: string; tags: string[]; color: string }
}

export async function openElementPanel(page: Page, name: string) {
  await nodeByName(page, name).click()
  await expect(page.getByTestId('element-panel')).toBeVisible()
}

export async function openConnectorPanelFromFirstEdge(page: Page) {
  const edge = page.locator('.react-flow__edge').first()
  await expect(edge).toBeVisible()
  await edge.click({ force: true })
  await edge.click({ force: true })
  await expect(page.getByTestId('connector-panel')).toBeVisible()
}

export async function addExistingFromLibrary(page: Page, name: string) {
  await expect(page.getByTestId('element-library-panel')).toBeVisible()
  await page.getByTestId('element-library-search').fill(name)
  const item = libraryItemByName(page, name)
  await expect(item).toBeVisible()
  await item.getByTestId('element-library-add').click()
  await expect(nodeByName(page, name)).toBeVisible()
}

export async function createAndLoadDiagramWithNodes(page: Page, count: number, prefix = 'Node') {
  const diagram = await createDiagram(page, uniqueName(`${prefix} Diagram`))
  const elements = []
  for (let i = 0; i < count; i += 1) {
    elements.push(await createPlacedElement(page, diagram.id, {
      name: uniqueName(`${prefix} ${i + 1}`),
      kind: i % 2 === 0 ? 'service' : 'database',
    }, 120 + i * 260, 150 + (i % 2) * 160))
  }
  await reloadView(page)
  for (const element of elements) {
    await expect(nodeByName(page, element.name)).toBeVisible()
  }
  return { diagram, elements }
}
