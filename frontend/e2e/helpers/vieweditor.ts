import { expect, type APIResponse, type Browser, type BrowserContext, type Locator, type Page } from '@playwright/test'

const e2eOrgId = '11111111-1111-1111-1111-111111111111'

export const onboardingStorage = {
  editor: 'diagrameditor_tutorial_v1_core',
  explore: 'explore_tutorial_v1_core',
  explorePage: 'explore_page_tutorial_v1_core',
  viewGrid: 'viewgrid_tutorial_v2_core',
  shown: 'onboarding_shown',
  sharedZoom: 'shared_zoom_onboarding_dismissed',
}

export type CollaborationIdentity = {
  client_id: string
  user_id: string
  username: string
}

export type RealtimeFrame = {
  type?: string
  [key: string]: unknown
}

export type CollaborationSession = {
  context: BrowserContext
  page: Page
  identity: CollaborationIdentity
  receivedFrames: RealtimeFrame[]
  sentFrames: RealtimeFrame[]
}

export type CollaborationPair = {
  alice: CollaborationSession
  bob: CollaborationSession
  close: () => Promise<void>
}

export async function disableAnimations(page: Page) {
  await page.addInitScript(() => {
    const styleText = `
      *, *::before, *::after {
        animation-delay: 0s !important;
        animation-duration: 0.001s !important;
        scroll-behavior: auto !important;
        transition-delay: 0s !important;
        transition-duration: 0s !important;
      }
    `
    const install = () => {
      const style = document.createElement('style')
      style.dataset.tldE2e = 'disable-animations'
      style.textContent = styleText
      document.head.appendChild(style)
    }
    if (document.readyState === 'loading') {
      document.addEventListener('DOMContentLoaded', install, { once: true })
    } else {
      install()
    }
  })
}

export async function prepareStorage(page: Page) {
  await page.addInitScript((keys) => {
    localStorage.setItem(keys.editor, '1')
    localStorage.setItem(keys.explore, '1')
    localStorage.setItem(keys.explorePage, '1')
    localStorage.setItem(keys.viewGrid, '1')
    localStorage.setItem(keys.shown, '1')
    localStorage.setItem(keys.sharedZoom, 'true')
    localStorage.setItem('diag:libraryOpen', 'true')
    localStorage.setItem('diag:explorerOpen', 'true')
    localStorage.setItem('diag:snapToGrid', 'false')
    localStorage.setItem('tld:experimental', JSON.stringify({ watchEnabled: true }))
  }, onboardingStorage)
}

export async function prepareE2EPage(page: Page, identity?: CollaborationIdentity) {
  await disableAnimations(page)
  await prepareStorage(page)
  if (identity) {
    await page.addInitScript((storedIdentity) => {
      localStorage.setItem('tld.collaboration.identity.v1', JSON.stringify(storedIdentity))
    }, identity)
  }
}

export function uniqueName(prefix: string) {
  return `${prefix} ${Date.now()} ${Math.random().toString(36).slice(2, 8)}`
}

function parseRealtimeFrame(payload: unknown): RealtimeFrame {
  const text = typeof payload === 'string' ? payload : String(payload)
  try {
    const parsed = JSON.parse(text) as unknown
    return parsed && typeof parsed === 'object'
      ? parsed as RealtimeFrame
      : { raw: text }
  } catch {
    return { raw: text }
  }
}

function frameUserIds(frame: RealtimeFrame): Set<string> {
  const users = new Set<string>()
  if (frame.type === 'presence_join') {
    const viewer = frame.viewer as { user_id?: unknown } | undefined
    if (typeof viewer?.user_id === 'string') users.add(viewer.user_id)
  }
  for (const key of ['viewers', 'collaborators']) {
    const items = frame[key]
    if (!Array.isArray(items)) continue
    for (const item of items) {
      const userId = (item as { user_id?: unknown } | undefined)?.user_id
      if (typeof userId === 'string') users.add(userId)
    }
  }
  return users
}

function sawRealtimeUser(session: CollaborationSession, userId: string) {
  return session.receivedFrames.some((frame) => frameUserIds(frame).has(userId))
}

export async function waitForRealtimeFrame(session: CollaborationSession, type: string) {
  await expect.poll(() => session.receivedFrames.some((frame) => frame.type === type)).toBe(true)
}

export async function createCollaborationSession(browser: Browser, viewId: number, identity: CollaborationIdentity, baseURL?: string) {
  const context = await browser.newContext({
    baseURL,
    viewport: { width: 1440, height: 1000 },
  })
  const page = await context.newPage()
  await prepareE2EPage(page, identity)

  const session: CollaborationSession = {
    context,
    page,
    identity,
    receivedFrames: [],
    sentFrames: [],
  }
  let sawViewSocket = false

  page.on('websocket', (socket) => {
    if (!socket.url().includes(`/api/views/${viewId}/ws`)) return
    sawViewSocket = true
    socket.on('framereceived', (frame) => {
      session.receivedFrames.push(parseRealtimeFrame(frame.payload))
    })
    socket.on('framesent', (frame) => {
      session.sentFrames.push(parseRealtimeFrame(frame.payload))
    })
  })

  await gotoView(page, viewId)
  await expect.poll(() => sawViewSocket).toBe(true)
  await waitForRealtimeFrame(session, 'presence_snapshot')
  return session
}

export async function closeCollaborationSessions(...sessions: CollaborationSession[]) {
  await Promise.all(sessions.map((session) => session.context.close().catch(() => {})))
}

export async function createCollaborationPair(browser: Browser, viewId: number, baseURL?: string, prefix = uniqueName('Collab User')): Promise<CollaborationPair> {
  const sessions: CollaborationSession[] = []
  try {
    const alice = await createCollaborationSession(browser, viewId, {
      client_id: `${prefix}-alice-client`.replace(/\s+/g, '-').toLowerCase(),
      user_id: `${prefix}-alice`.replace(/\s+/g, '-').toLowerCase(),
      username: `${prefix} Alice`,
    }, baseURL)
    sessions.push(alice)

    const bob = await createCollaborationSession(browser, viewId, {
      client_id: `${prefix}-bob-client`.replace(/\s+/g, '-').toLowerCase(),
      user_id: `${prefix}-bob`.replace(/\s+/g, '-').toLowerCase(),
      username: `${prefix} Bob`,
    }, baseURL)
    sessions.push(bob)

    await expect.poll(() => sawRealtimeUser(alice, bob.identity.user_id)).toBe(true)
    await expect.poll(() => sawRealtimeUser(bob, alice.identity.user_id)).toBe(true)

    return {
      alice,
      bob,
      close: () => closeCollaborationSessions(alice, bob),
    }
  } catch (error) {
    await closeCollaborationSessions(...sessions)
    throw error
  }
}

export async function createDiagram(page: Page, name = uniqueName('E2E Diagram')) {
  const view = await createApiView(page, name)
  await gotoView(page, view.id)
  return { name, id: view.id }
}

export async function createDiagramFromGrid(page: Page, name = uniqueName('E2E Diagram')) {
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

export async function pasteTextOnCanvas(page: Page, text: string) {
  await page.getByTestId('vieweditor-canvas').click()
  await page.evaluate((pasteText) => {
    const data = new DataTransfer()
    data.setData('text/plain', pasteText)
    const event = new Event('paste', { bubbles: true, cancelable: true })
    Object.defineProperty(event, 'clipboardData', { value: data })
    window.dispatchEvent(event)
  }, text)
}

export async function addNodeWithToolbar(page: Page, name = uniqueName('Toolbar Node')) {
  return addNodeWithKeyboard(page, name)
}

export async function addNodeWithKeyboard(page: Page, name = uniqueName('Keyboard Node')) {
  await page.getByTestId('vieweditor-canvas').click({ position: { x: 600, y: 100 } })
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
  await page.getByTestId('vieweditor-canvas').click({ position: { x: 600, y: 100 } })
  await page.keyboard.press('c')
  const input = page.getByTestId('pending-element-label-input')
  await input.fill(name)
  await expect(page.getByTestId('pending-element-existing-option').filter({ hasText: name }).first()).toBeVisible()
  await page.keyboard.press('ArrowDown')
  await page.keyboard.press('Enter')
  await expect(nodeByName(page, name)).toBeVisible()
}

export async function confirmInlineNewElement(page: Page, name: string) {
  const input = page.getByTestId('pending-element-label-input')
  await expect(input).toBeVisible()
  await input.fill(name)
  await expect(page.getByTestId('pending-element-create-option').filter({ hasText: name }).first()).toBeVisible()
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

export async function dragNodeByName(page: Page, name: string, deltaX: number, deltaY: number) {
  const node = nodeByName(page, name)
  const box = await node.boundingBox()
  if (!box) throw new Error(`Node "${name}" is not visible`)
  const center = { x: box.x + box.width / 2, y: box.y + box.height / 2 }
  await page.mouse.move(center.x, center.y)
  await page.mouse.down()
  await page.mouse.move(center.x + deltaX, center.y + deltaY, { steps: 12 })
  await page.mouse.up()
}

export async function createConnectorWithKeyboard(page: Page, sourceName: string, targetName: string) {
  await nodeByName(page, sourceName).click()
  await page.keyboard.press('e')
  await expect(nodeByName(page, sourceName).getByText(/tap element to connect/i)).toBeVisible()
  await nodeByName(page, targetName).click()
  await expect(page.locator('.react-flow__edge').first()).toBeAttached()
}

export async function listPlacements(page: Page, viewId = currentViewId(page)) {
  const response = await page.request.post('/api/diag.v1.WorkspaceService/ListPlacements', {
    data: { viewId },
  })
  expect(response.ok()).toBeTruthy()
  const json = await response.json()
  return (json.placements ?? []) as Array<{
    id: number
    viewId: number
    elementId: number
    positionX?: number
    positionY?: number
    position_x?: number
    position_y?: number
    name: string
  }>
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
    data: { orgId: e2eOrgId, name, ownerElementId },
  })
  await expectResponseOk(response, 'create view')
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

export async function updateView(page: Page, viewId: number, data: { name?: string; description?: string; levelLabel?: string; tags?: string[] }) {
  const response = await page.request.post('/api/diag.v1.WorkspaceService/UpdateView', {
    data: { viewId, ...data },
  })
  expect(response.ok()).toBeTruthy()
  const json = await response.json()
  return json.view as { id: number; name: string; levelLabel?: string; level_label?: string }
}

export async function deleteView(page: Page, viewId: number) {
  const response = await page.request.post('/api/diag.v1.WorkspaceService/DeleteView', {
    data: { orgId: e2eOrgId, viewId },
  })
  expect(response.ok()).toBeTruthy()
}

export async function deleteTag(page: Page, tag: string) {
  const response = await page.request.delete(`/api/tags/${encodeURIComponent(tag)}`)
  expect(response.ok()).toBeTruthy()
}

export async function mergeElements(page: Page, sourceId: number, survivorId: number, resolved: Record<string, string | null> = {}) {
  const response = await page.request.post('/api/elements/merge', {
    data: { source_id: sourceId, survivor_id: survivorId, resolved },
  })
  expect(response.ok()).toBeTruthy()
  const json = await response.json()
  return json as { survivor: { id: number; name: string; tags?: string[] }; deleted_id: number }
}

export async function getViewMarkdown(page: Page, viewId: number) {
  const response = await page.request.post('/api/diag.v1.WorkspaceService/GetViewMarkdown', {
    data: { viewId },
  })
  if (response.status() === 404) return null
  expect(response.ok()).toBeTruthy()
  const json = await response.json()
  return json as { markdown?: { path?: string; isManaged?: boolean; is_managed?: boolean; updatedAt?: string; updated_at?: string }; content?: string }
}

export async function createViewMarkdown(page: Page, viewId: number, fileName: string, initialContent = '') {
  const response = await page.request.post('/api/diag.v1.WorkspaceService/CreateViewMarkdown', {
    data: { viewId, fileName, initialContent },
  })
  expect(response.ok()).toBeTruthy()
}

export async function saveViewMarkdown(page: Page, viewId: number, content: string) {
  const response = await page.request.post('/api/diag.v1.WorkspaceService/SaveViewMarkdown', {
    data: { viewId, content },
  })
  expect(response.ok()).toBeTruthy()
}

export async function unlinkViewMarkdown(page: Page, viewId: number, deleteManagedFile = true) {
  const response = await page.request.post('/api/diag.v1.WorkspaceService/UnlinkViewMarkdown', {
    data: { viewId, deleteManagedFile },
  })
  expect(response.ok()).toBeTruthy()
}

async function expectResponseOk(response: APIResponse, action: string) {
  if (!response.ok()) {
    throw new Error(`${action} failed with ${response.status()}: ${await response.text()}`)
  }
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
    sourceHandle?: string | null
    targetHandle?: string | null
    source_handle?: string | null
    target_handle?: string | null
    tags?: string[]
  }>
}

export async function createConnector(page: Page, viewId: number, sourceElementId: number, targetElementId: number, data: {
  label?: string
  description?: string
  relationship?: string
  direction?: string
  style?: string
  url?: string
  sourceHandle?: string | null
  targetHandle?: string | null
  source_handle?: string | null
  target_handle?: string | null
  tags?: string[]
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
      sourceHandle: data.sourceHandle ?? data.source_handle ?? undefined,
      targetHandle: data.targetHandle ?? data.target_handle ?? undefined,
      tags: data.tags ?? [],
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

export async function createTag(page: Page, name: string, color = '#A0AEC0', description = '') {
  const response = await page.request.post('/api/diag.v1.OrgService/UpdateTag', {
    data: { tag: name, color, description },
  })
  expect(response.ok()).toBeTruthy()
}

export async function openElementPanel(page: Page, name: string) {
  await nodeByName(page, name).click()
  await expect(page.getByTestId('element-panel')).toBeVisible()
}

export async function openConnectorPanelFromFirstEdge(page: Page) {
  const edge = page.locator('.react-flow__edge').first()
  await expect(edge).toBeAttached()
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

export async function createConnectorGraph(page: Page, prefix = 'Connector') {
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
