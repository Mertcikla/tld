import { expect, type Locator, type Page } from '@playwright/test'

export const onboardingStorage = {
  editor: 'diagrameditor_tutorial_v1_core',
  explore: 'explore_tutorial_v1_core',
  explorePage: 'explore_page_tutorial_v1_core',
  viewGrid: 'viewgrid_tutorial_v2_core',
  shown: 'onboarding_shown',
  dependencies: 'dependencies_tutorial_v1_core',
  sharedZoom: 'shared_zoom_onboarding_dismissed',
}

export async function prepareStorage(page: Page) {
  await page.addInitScript((keys) => {
    localStorage.setItem(keys.editor, '1')
    localStorage.setItem(keys.explore, '1')
    localStorage.setItem(keys.explorePage, '1')
    localStorage.setItem(keys.viewGrid, '1')
    localStorage.setItem(keys.shown, '1')
    localStorage.setItem(keys.dependencies, '1')
    localStorage.setItem(keys.sharedZoom, 'true')
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
  repo?: string
  branch?: string
  filePath?: string
  language?: string
  technologyLinks?: Array<{ type: string; slug?: string; label: string; isPrimaryIcon?: boolean }>
}) {
  const response = await page.request.post('/api/diag.v1.WorkspaceService/CreateElement', {
    data: {
      name: data.name,
      kind: data.kind ?? '',
      description: data.description ?? '',
      technology: data.technology ?? '',
      url: data.url ?? '',
      tags: data.tags ?? [],
      repo: data.repo ?? '',
      branch: data.branch ?? '',
      filePath: data.filePath ?? '',
      language: data.language ?? '',
      technologyLinks: data.technologyLinks ?? [],
    },
  })
  expect(response.ok()).toBeTruthy()
  const json = await response.json()
  return json.element as { id: number; name: string; kind?: string; tags?: string[] }
}

export async function updateElement(page: Page, elementId: number, data: {
  name?: string
  kind?: string
  description?: string
  technology?: string
  url?: string
  tags?: string[]
  repo?: string
  branch?: string
  filePath?: string
  language?: string
  technologyLinks?: Array<{ type: string; slug?: string; label: string; isPrimaryIcon?: boolean }>
}) {
  const response = await page.request.post('/api/diag.v1.WorkspaceService/UpdateElement', {
    data: { elementId, ...data },
  })
  expect(response.ok()).toBeTruthy()
  const json = await response.json()
  return json.element as Awaited<ReturnType<typeof getElement>>
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
    repo?: string
    branch?: string
    file_path?: string
    language?: string
  }
}

export async function listElements(page: Page, search = '') {
  const response = await page.request.post('/api/diag.v1.WorkspaceService/ListElements', {
    data: { search },
  })
  expect(response.ok()).toBeTruthy()
  const json = await response.json()
  return (json.elements ?? []) as Array<{ id: number; name: string; kind?: string; technology?: string; tags?: string[] }>
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

export async function getView(page: Page, viewId: number) {
  const response = await page.request.post('/api/diag.v1.WorkspaceService/GetView', {
    data: { viewId },
  })
  expect(response.ok()).toBeTruthy()
  const json = await response.json()
  return json.view as { id: number; name: string; levelLabel?: string; level_label?: string; parentViewId?: number | null; parent_view_id?: number | null }
}

export async function updateView(page: Page, viewId: number, data: { name?: string; levelLabel?: string }) {
  const response = await page.request.post('/api/diag.v1.WorkspaceService/UpdateView', {
    data: { viewId, ...data },
  })
  expect(response.ok()).toBeTruthy()
  const json = await response.json()
  return json.view as { id: number; name: string; levelLabel?: string; level_label?: string }
}

export async function deleteView(page: Page, viewId: number) {
  const response = await page.request.post('/api/diag.v1.WorkspaceService/DeleteView', {
    data: { viewId },
  })
  expect(response.ok()).toBeTruthy()
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

export async function listVisibilityOverrides(page: Page, viewId: number) {
  const response = await page.request.get(`/api/views/${viewId}/visibility-overrides`)
  expect(response.ok()).toBeTruthy()
  const json = await response.json()
  return (json.overrides ?? []) as Array<{ view_id: number; resource_type: string; resource_id: number; level_delta: number }>
}

export async function setVisibilityOverride(page: Page, viewId: number, resourceType: 'element' | 'connector', resourceId: number, levelDelta: number) {
  const response = await page.request.put(`/api/views/${viewId}/visibility-overrides`, {
    data: { resource_type: resourceType, resource_id: resourceId, level_delta: levelDelta },
  })
  expect(response.ok()).toBeTruthy()
  const json = await response.json()
  return json.override as { view_id: number; resource_type: string; resource_id: number; level_delta: number }
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

export async function createDependencyGraph(page: Page, prefix = 'Dependency') {
  const diagram = await createApiView(page, uniqueName(`${prefix} Diagram`))
  const center = await createPlacedElement(page, diagram.id, { name: uniqueName(`${prefix} Center`), kind: 'service', technology: 'go' }, 480, 260)
  const incoming = await createPlacedElement(page, diagram.id, { name: uniqueName(`${prefix} Incoming`), kind: 'api', technology: 'typescript' }, 180, 260)
  const outgoing = await createPlacedElement(page, diagram.id, { name: uniqueName(`${prefix} Outgoing`), kind: 'database', technology: 'postgres' }, 780, 260)
  const both = await createPlacedElement(page, diagram.id, { name: uniqueName(`${prefix} Both`), kind: 'queue', technology: 'kafka' }, 480, 80)
  const undirected = await createPlacedElement(page, diagram.id, { name: uniqueName(`${prefix} Undirected`), kind: 'external', technology: 's3' }, 480, 480)
  await createConnector(page, diagram.id, incoming.id, center.id, { label: 'incoming', direction: 'forward' })
  await createConnector(page, diagram.id, center.id, outgoing.id, { label: 'outgoing', direction: 'forward' })
  await createConnector(page, diagram.id, center.id, both.id, { label: 'both', direction: 'both' })
  await createConnector(page, diagram.id, center.id, undirected.id, { label: 'none', direction: 'none' })
  return { diagram, center, incoming, outgoing, both, undirected }
}

export async function mockWatchRuntime(page: Page, options: {
  active?: boolean
  repositoryId?: number
  versionId?: number
  viewId?: number
  elementId?: number
  elementName?: string
} = {}) {
  const repositoryId = options.repositoryId ?? 1001
  const versionId = options.versionId ?? 2001
  const elementId = options.elementId ?? 1
  const elementName = options.elementName ?? 'Changed element'
  const active = options.active ?? true

  await page.addInitScript((payload) => {
    const sent: string[] = []
    class MockWebSocket extends EventTarget {
      static CONNECTING = 0
      static OPEN = 1
      static CLOSING = 2
      static CLOSED = 3
      readyState = MockWebSocket.CONNECTING
      url: string
      onopen: ((event: Event) => void) | null = null
      onclose: ((event: Event) => void) | null = null
      onmessage: ((event: MessageEvent) => void) | null = null
      onerror: ((event: Event) => void) | null = null

      constructor(url: string) {
        super()
        this.url = url
        ;(window as unknown as { __TLD_WATCH_SENT__: string[] }).__TLD_WATCH_SENT__ = sent
        window.setTimeout(() => {
          this.readyState = MockWebSocket.OPEN
          const openEvent = new Event('open')
          this.dispatchEvent(openEvent)
          this.onopen?.(openEvent)
          for (const event of payload.events) {
            const message = new MessageEvent('message', { data: JSON.stringify(event) })
            this.dispatchEvent(message)
            this.onmessage?.(message)
          }
        }, 20)
      }

      send(data: string) {
        sent.push(data)
      }

      close() {
        this.readyState = MockWebSocket.CLOSED
        const event = new Event('close')
        this.dispatchEvent(event)
        this.onclose?.(event)
      }
    }
    ;(window as unknown as { WebSocket: typeof MockWebSocket }).WebSocket = MockWebSocket
  }, {
    events: [
      { type: 'watch.connected', at: new Date().toISOString(), repository_id: repositoryId, watcher_mode: 'mock', languages: ['go'] },
      { type: 'scan.started', at: new Date().toISOString(), repository_id: repositoryId, changed_files: 2 },
      { type: 'source.changed', at: new Date().toISOString(), repository_id: repositoryId, data: { change: { path: 'internal/app/service.go', change_type: 'modified' }, representation_changed: true } },
      { type: 'scan.completed', at: new Date().toISOString(), repository_id: repositoryId },
    ],
  })

  const repo = {
    id: repositoryId,
    remote_url: null,
    repo_root: '/tmp/e2e-repo',
    display_name: 'e2e-repo',
    branch: 'main',
    head_commit: 'abcdef0',
    identity_status: 'associated',
  }
  const version = {
    id: versionId,
    repository_id: repositoryId,
    commit_hash: 'abcdef0',
    commit_message: 'E2E mocked watch version',
    branch: 'main',
    representation_hash: 'mock-hash',
    workspace_version_id: 3001,
    created_at: new Date().toISOString(),
  }
  const diff = {
    id: 4001,
    version_id: versionId,
    owner_type: 'file',
    owner_key: 'internal/app/service.go',
    change_type: 'updated',
    resource_type: 'element',
    resource_id: elementId,
    summary: elementName,
    added_lines: 12,
    removed_lines: 3,
  }

  await page.route('**/api/watch/status', async (route) => {
    await route.fulfill({
      contentType: 'application/json',
      body: JSON.stringify(active
        ? { active: true, repository: repo, lock: { id: 1, repository_id: repositoryId, pid: 123, started_at: new Date().toISOString(), heartbeat_at: new Date().toISOString(), status: 'active' } }
        : { active: false }),
    })
  })
  await page.route('**/api/watch/repositories', async (route) => {
    await route.fulfill({ contentType: 'application/json', body: JSON.stringify([repo]) })
  })
  await page.route(`**/api/watch/repositories/${repositoryId}/versions`, async (route) => {
    await route.fulfill({ contentType: 'application/json', body: JSON.stringify([version]) })
  })
  await page.route(`**/api/watch/versions/${versionId}/diffs**`, async (route) => {
    await route.fulfill({ contentType: 'application/json', body: JSON.stringify([diff]) })
  })
  await page.route('**/api/versions**', async (route) => {
    await route.fulfill({
      contentType: 'application/json',
      body: JSON.stringify({
        versions: [{
          id: '3001',
          version_id: String(versionId),
          source: 'watch',
          view_count: 1,
          element_count: 1,
          connector_count: 0,
          description: 'mock',
          workspace_hash: 'hash',
          created_at: new Date().toISOString(),
        }],
      }),
    })
  })
}
