import { expect, test } from '../fixtures'
import {
  closeViewEditorPanels,
  createAndLoadDiagramWithNodes,
  createApiView,
  createConnector,
  createElement,
  createLayer,
  createPlacedElement,
  createTag,
  currentViewId,
  dispatchSafariGestureOnLocator,
  dispatchWheelOnLocator,
  dragNodeByName,
  getElement,
  gotoView,
  isMobileLayout,
  libraryItemByName,
  listPlacements,
  nodeByName,
  openElementLibrary,
  openViewExplorer,
  pasteTextOnCanvas,
  reactFlowPaneBox,
  reactFlowViewport,
  reloadView,
  uniqueName,
} from '../helpers/vieweditor'

function placementX(placement: Awaited<ReturnType<typeof listPlacements>>[number]) {
  return Math.round(placement.positionX ?? placement.position_x ?? 0)
}

function placementY(placement: Awaited<ReturnType<typeof listPlacements>>[number]) {
  return Math.round(placement.positionY ?? placement.position_y ?? 0)
}

async function dragMouse(page: Parameters<typeof currentViewId>[0], from: { x: number; y: number }, to: { x: number; y: number }) {
  await page.mouse.move(from.x, from.y)
  await page.mouse.down()
  await page.mouse.move(to.x, to.y, { steps: 12 })
  await page.mouse.up()
}

test('selects, marquee-selects, drags nodes, and persists multi-select moves', async ({ page }) => {
  const { diagram, elements } = await createAndLoadDiagramWithNodes(page, 2, 'Pointer Select Drag')
  await closeViewEditorPanels(page)
  const first = nodeByName(page, elements[0].name)
  const second = nodeByName(page, elements[1].name)

  await first.click()
  await expect(first).toHaveAttribute('aria-selected', 'true')
  await expect(page.getByTestId('element-panel')).toBeVisible()
  await page.keyboard.press('Escape')

  if (await isMobileLayout(page)) {
    return
  }

  const beforeSingle = new Map((await listPlacements(page, diagram.id)).map((placement) => [
    placement.elementId,
    { x: placementX(placement), y: placementY(placement) },
  ]))
  await dragNodeByName(page, elements[0].name, 120, 80)
  await expect.poll(async () => {
    const moved = (await listPlacements(page, diagram.id)).find((placement) => placement.elementId === elements[0].id)
    const previous = beforeSingle.get(elements[0].id)
    return Boolean(moved && previous && (
      Math.abs(placementX(moved) - previous.x) > 20 ||
      Math.abs(placementY(moved) - previous.y) > 20
    ))
  }).toBe(true)

  const firstMarqueeBox = await first.boundingBox()
  const secondMarqueeBox = await second.boundingBox()
  if (!firstMarqueeBox || !secondMarqueeBox) throw new Error('Expected both nodes to be visible')
  await dragMouse(
    page,
    { x: Math.min(firstMarqueeBox.x, secondMarqueeBox.x) - 24, y: Math.min(firstMarqueeBox.y, secondMarqueeBox.y) - 24 },
    {
      x: Math.max(firstMarqueeBox.x + firstMarqueeBox.width, secondMarqueeBox.x + secondMarqueeBox.width) + 24,
      y: Math.max(firstMarqueeBox.y + firstMarqueeBox.height, secondMarqueeBox.y + secondMarqueeBox.height) + 24,
    },
  )
  await expect(page.getByTestId('vieweditor-selection-bulk-bar')).toContainText('2 selected')

  const prefix = uniqueName('Pointer Bulk Drag')
  const bulkView = await createApiView(page, `${prefix} Diagram`)
  await gotoView(page, bulkView.id)
  await pasteTextOnCanvas(page, `flowchart LR
  A[${prefix} A] --> B[${prefix} B]`)
  await expect(page.getByTestId('vieweditor-selection-bulk-bar')).toContainText('2 selected')
  const bulkBefore = new Map((await listPlacements(page, bulkView.id))
    .filter((placement) => placement.name.startsWith(prefix))
    .map((placement) => [placement.name, { x: placementX(placement), y: placementY(placement) }]))
  const bulkDragBox = await nodeByName(page, `${prefix} A`).boundingBox()
  if (!bulkDragBox) throw new Error('Expected selected bulk node to be visible')
  await dragMouse(
    page,
    { x: bulkDragBox.x + bulkDragBox.width / 2, y: bulkDragBox.y + bulkDragBox.height / 2 },
    { x: bulkDragBox.x + bulkDragBox.width / 2 + 160, y: bulkDragBox.y + bulkDragBox.height / 2 + 110 },
  )

  await expect.poll(async () => {
    const after = (await listPlacements(page, bulkView.id)).filter((placement) => placement.name.startsWith(prefix))
    return after.every((placement) => {
      const previous = bulkBefore.get(placement.name)
      if (!previous) return false
      return Math.abs(placementX(placement) - previous.x) > 20 || Math.abs(placementY(placement) - previous.y) > 20
    })
  }).toBe(true)

  const paneBox = await reactFlowPaneBox(page)
  await page.mouse.click(paneBox.x + paneBox.width - 30, paneBox.y + paneBox.height - 30)
  await expect(page.getByTestId('vieweditor-selection-bulk-bar')).toHaveCount(0)
})

test('pans with middle mouse and separates physical wheel zoom from trackpad pan and pinch', async ({ page }) => {
  await createAndLoadDiagramWithNodes(page, 0, 'Pointer Wheel')
  await closeViewEditorPanels(page)
  const canvas = page.getByTestId('vieweditor-canvas')
  const pane = page.locator('.react-flow__pane')

  const start = await reactFlowViewport(page)
  await dispatchWheelOnLocator(canvas, { deltaY: -120, deltaMode: 1, cancelable: false })
  await expect.poll(async () => (await reactFlowViewport(page)).zoom).toBeGreaterThan(start.zoom + 0.02)

  const afterWheel = await reactFlowViewport(page)
  await dispatchWheelOnLocator(canvas, { deltaY: -80, deltaMode: 1, ctrlKey: true, cancelable: false })
  await expect.poll(async () => Math.abs((await reactFlowViewport(page)).zoom - afterWheel.zoom)).toBeLessThan(0.03)

  const afterCtrlWheel = await reactFlowViewport(page)
  await dispatchWheelOnLocator(pane, { deltaX: 72, deltaY: 18, deltaMode: 0, cancelable: false })
  const isMobileLayout = await page.evaluate(() => window.matchMedia('(max-width: 767px)').matches)
  if (isMobileLayout) {
    const next = await reactFlowViewport(page)
    expect(Math.abs(next.x - afterCtrlWheel.x) + Math.abs(next.y - afterCtrlWheel.y)).toBeLessThan(2)
  } else {
    await expect.poll(async () => {
      const next = await reactFlowViewport(page)
      return Math.abs(next.x - afterCtrlWheel.x) + Math.abs(next.y - afterCtrlWheel.y)
    }).toBeGreaterThan(10)
  }
  const afterTrackpad = await reactFlowViewport(page)
  expect(Math.abs(afterTrackpad.zoom - afterCtrlWheel.zoom)).toBeLessThan(0.03)

  const beforeMiddlePan = await reactFlowViewport(page)
  const paneBox = await reactFlowPaneBox(page)
  await page.mouse.move(paneBox.x + paneBox.width * 0.55, paneBox.y + paneBox.height * 0.55)
  await page.mouse.down({ button: 'middle' })
  await page.mouse.move(paneBox.x + paneBox.width * 0.55 + 130, paneBox.y + paneBox.height * 0.55 + 90, { steps: 10 })
  await page.mouse.up({ button: 'middle' })
  await expect.poll(async () => {
    const next = await reactFlowViewport(page)
    return Math.abs(next.x - beforeMiddlePan.x) + Math.abs(next.y - beforeMiddlePan.y)
  }).toBeGreaterThan(20)
})

test('zooms ViewEditor with Safari gesture events around the canvas point', async ({ page }) => {
  await createAndLoadDiagramWithNodes(page, 0, 'Pointer Safari Gesture')
  await closeViewEditorPanels(page)
  const canvas = page.getByTestId('vieweditor-canvas')
  const paneBox = await reactFlowPaneBox(page)
  const before = await reactFlowViewport(page)

  await dispatchSafariGestureOnLocator(canvas, {
    clientX: paneBox.x + paneBox.width * 0.52,
    clientY: paneBox.y + paneBox.height * 0.48,
    startScale: 1,
    changeScale: 1.35,
  })

  await expect.poll(async () => (await reactFlowViewport(page)).zoom).toBeGreaterThan(before.zoom + 0.05)
})

test('opens canvas and connector context menus without leaking right-clicks into connector selection', async ({ page }) => {
  const { diagram, elements } = await createAndLoadDiagramWithNodes(page, 2, 'Pointer Context Menu')
  await createConnector(page, diagram.id, elements[0].id, elements[1].id, { label: 'right-click-me' })
  await reloadView(page)
  await closeViewEditorPanels(page)

  const paneBox = await reactFlowPaneBox(page)
  await page.mouse.click(paneBox.x + paneBox.width * 0.5, paneBox.y + paneBox.height * 0.72, { button: 'right' })
  await expect(page.getByTestId('vieweditor-canvas-context-add-element')).toBeVisible()
  await expect(page.getByTestId('vieweditor-canvas-context-copy-mermaid')).toBeVisible()
  await page.mouse.click(paneBox.x + paneBox.width * 0.75, paneBox.y + paneBox.height * 0.75)

  const edge = page.locator('.react-flow__edge').first()
  await edge.click({ button: 'right', force: true })
  await expect(page.getByTestId('connector-context-edit')).toBeVisible()
  await expect(page.getByTestId('connector-context-move-source')).toBeVisible()
  await edge.click({ force: true })
  await expect(page.getByTestId('connector-panel')).toHaveCount(0)
  await edge.click({ force: true })
  await expect(page.getByTestId('connector-panel')).toBeVisible()
})

test('opens source links in code preview without selecting the node', async ({ page }) => {
  await page.route('https://api.github.com/repos/diag', async (route) => {
    await route.fulfill({
      status: 200,
      contentType: 'application/json',
      body: JSON.stringify({ private: false }),
    })
  })
  await page.route('https://raw.githubusercontent.com/diag/refs/heads/main/frontend/src/App.tsx', async (route) => {
    await route.fulfill({
      status: 200,
      contentType: 'text/plain',
      body: 'export function App() { return null }\n',
    })
  })

  const root = await createApiView(page, uniqueName('Pointer Zoom Root'))
  const codeElement = await createPlacedElement(page, root.id, {
    name: uniqueName('Pointer Code Link'),
    kind: 'service',
    repo: 'diag',
    branch: 'main',
    filePath: 'frontend/src/App.tsx',
    language: 'typescript',
  }, 460, 180)
  await gotoView(page, root.id)
  await closeViewEditorPanels(page)
  const codeNode = nodeByName(page, codeElement.name)
  await codeNode.getByTestId('vieweditor-node-source-link').click()
  await expect(page.getByTestId('code-preview-panel')).toBeVisible()
  await expect(page.getByTestId('element-panel')).toHaveCount(0)
  await page.getByTestId('code-preview-close').click()
  await expect(page.getByTestId('code-preview-panel')).toBeHidden()
})

test('navigates with node zoom buttons', async ({ page }) => {
  const root = await createApiView(page, uniqueName('Pointer Zoom Root'))
  const parent = await createPlacedElement(page, root.id, {
    name: uniqueName('Pointer Zoom Parent'),
    kind: 'service',
  }, 120, 140)
  await gotoView(page, root.id)
  await closeViewEditorPanels(page)

  const parentNode = nodeByName(page, parent.name)
  if (!await isMobileLayout(page)) {
    await parentNode.getByTestId('vieweditor-node-zoom-in').hover()
    await expect(parentNode).toHaveCSS('outline-color', /rgb|rgba|var/)
  }
  await parentNode.getByTestId('vieweditor-node-zoom-in').click({ force: true })
  await expect.poll(async () => currentViewId(page)).not.toBe(root.id)
  const childViewId = currentViewId(page)
  const childElement = await createPlacedElement(page, childViewId, {
    name: uniqueName('Pointer Child Node'),
    kind: 'api',
  }, 180, 160)
  await reloadView(page)
  await expect(nodeByName(page, childElement.name)).toBeVisible()

  await nodeByName(page, childElement.name).getByTestId('vieweditor-node-zoom-out').click({ force: true })
  await expect(page).toHaveURL(new RegExp(`/views/${root.id}`))
})

test('drops HTML library items plus tag and layer drags onto canvas nodes', async ({ page }) => {
  await createAndLoadDiagramWithNodes(page, 0, 'Pointer Library')
  await closeViewEditorPanels(page)
  const element = await createElement(page, { name: uniqueName('Pointer Library Drag Source'), kind: 'service' })
  await reloadView(page)
  await openElementLibrary(page)
  await page.getByTestId('element-library-search').fill(element.name)

  const item = libraryItemByName(page, element.name)
  const paneBox = await reactFlowPaneBox(page)
  const dataTransfer = await page.evaluateHandle(() => new DataTransfer())
  await item.dispatchEvent('dragstart', { dataTransfer })
  await page.locator('.react-flow__pane').dispatchEvent('dragover', {
    dataTransfer,
    clientX: paneBox.x + paneBox.width * 0.56,
    clientY: paneBox.y + paneBox.height * 0.46,
  })
  await page.locator('.react-flow__pane').dispatchEvent('drop', {
    dataTransfer,
    clientX: paneBox.x + paneBox.width * 0.56,
    clientY: paneBox.y + paneBox.height * 0.46,
  })

  await expect(nodeByName(page, element.name)).toBeVisible()

  const tag = uniqueName('pointer-drop-tag')
  const layerTagA = uniqueName('pointer-layer-a')
  const layerTagB = uniqueName('pointer-layer-b')
  await createTag(page, tag, '#F6AD55')
  await createTag(page, layerTagA, '#68D391')
  await createTag(page, layerTagB, '#63B3ED')
  const layer = await createLayer(page, currentViewId(page), { name: uniqueName('Pointer Drop Layer'), tags: [layerTagA, layerTagB] })
  await reloadView(page)
  await openViewExplorer(page)

  const otherTags = page.getByRole('button', { name: /Other tags/ })
  await expect(otherTags).toBeVisible()
  await otherTags.click()
  const target = nodeByName(page, element.name)
  const targetBox = await target.boundingBox()
  if (!targetBox) throw new Error('Missing tag-drop target node')
  const dropPoint = {
    clientX: targetBox.x + targetBox.width / 2,
    clientY: targetBox.y + targetBox.height / 2,
  }

  const tagTransfer = await page.evaluateHandle(() => new DataTransfer())
  await page.getByTestId('tag-manager-tag').filter({ hasText: tag }).dispatchEvent('dragstart', { dataTransfer: tagTransfer })
  await target.dispatchEvent('dragover', { dataTransfer: tagTransfer, ...dropPoint })
  await target.dispatchEvent('drop', { dataTransfer: tagTransfer, ...dropPoint })

  const layerTransfer = await page.evaluateHandle(() => new DataTransfer())
  await page.getByTestId('tag-manager-layer').filter({ hasText: layer.name }).dispatchEvent('dragstart', { dataTransfer: layerTransfer })
  await target.dispatchEvent('dragover', { dataTransfer: layerTransfer, ...dropPoint })
  await target.dispatchEvent('drop', { dataTransfer: layerTransfer, ...dropPoint })

  await expect.poll(async () => (await getElement(page, element.id)).tags ?? [])
    .toEqual(expect.arrayContaining([tag, layerTagA, layerTagB]))
})
