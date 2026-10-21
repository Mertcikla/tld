import { expect, test } from '../fixtures'
import {
  closeViewEditorPanels,
  confirmInlineNewElement,
  createAndLoadDiagramWithNodes,
  createApiView,
  createConnector,
  createElement,
  createPlacedElement,
  expectConnector,
  expectPlacement,
  handleLocator,
  isMobileLayout,
  listConnectors,
  listPlacements,
  locatorCenter,
  longPressLocator,
  nodeByName,
  reactFlowPaneBox,
  reloadView,
  uniqueName,
} from '../helpers/vieweditor'

async function waitForStableLocator(
  locator: ReturnType<typeof handleLocator>,
) {
  let previous = await locator.boundingBox()
  let stableSamples = 0
  await expect.poll(async () => {
    const next = await locator.boundingBox()
    const stable =
      previous && next &&
      previous.x === next.x && previous.y === next.y &&
      previous.width === next.width && previous.height === next.height
    stableSamples = stable ? stableSamples + 1 : 0
    previous = next
    return stableSamples
  }, { intervals: [100] }).toBeGreaterThanOrEqual(3)
}

async function dragLocatorToPoint(
  page: Parameters<typeof handleLocator>[0],
  locator: ReturnType<typeof handleLocator>,
  point: { x: number; y: number },
  useTouchOnMobile = false,
) {
  await waitForStableLocator(locator)
  const start = await locatorCenter(locator)
  const isWebKit = await page.evaluate(() => (
    navigator.userAgent.includes('AppleWebKit') &&
    !navigator.userAgent.includes('Chrome') &&
    !navigator.userAgent.includes('CriOS')
  ))
  if (useTouchOnMobile && (await isMobileLayout(page) || isWebKit)) {
    await locator.dispatchEvent('pointerdown', {
      bubbles: true,
      cancelable: true,
      clientX: start.x,
      clientY: start.y,
      pointerId: 81,
      pointerType: 'touch',
      button: 0,
      buttons: 1,
    })
    await page.evaluate((payload) => {
      document.dispatchEvent(new PointerEvent('pointermove', {
        bubbles: true,
        cancelable: true,
        clientX: payload.x,
        clientY: payload.y,
        pointerId: 81,
        pointerType: 'touch',
        button: 0,
        buttons: 1,
      }))
      document.dispatchEvent(new PointerEvent('pointerup', {
        bubbles: true,
        cancelable: true,
        clientX: payload.x,
        clientY: payload.y,
        pointerId: 81,
        pointerType: 'touch',
        button: 0,
        buttons: 0,
      }))
    }, point)
    return
  }
  await page.mouse.move(start.x, start.y)
  await page.mouse.down()
  await page.mouse.move(point.x, point.y, { steps: 12 })
  await page.mouse.up()
}

async function dragLocatorToPointWithMidDragCheck(
  page: Parameters<typeof handleLocator>[0],
  locator: ReturnType<typeof handleLocator>,
  point: { x: number; y: number },
  check: () => Promise<void>,
) {
  await waitForStableLocator(locator)
  const start = await locatorCenter(locator)
  await page.mouse.move(start.x, start.y)
  await page.mouse.down()
  await page.mouse.move(point.x, point.y, { steps: 12 })
  await check()
  await page.mouse.up()
}

async function openEdgeContextMenu(page: Parameters<typeof handleLocator>[0]) {
  const edge = page.locator('.react-flow__edge').first()
  const path = edge.locator('.react-flow__edge-interaction').first()
  const moveTarget = page.getByTestId('connector-context-move-target')
  // Horizontal SVG paths have a zero-height bounding box even when their stroke is visible.
  await expect(path).toBeAttached()
  await waitForStableLocator(path)
  // The SVG bounding-box center can miss a curved edge and right-click the canvas.
  const findPoint = () => path.evaluate((element) => {
    const curve = element as SVGPathElement
    const transform = curve.getScreenCTM()
    if (!transform) return null
    const edge = curve.closest('.react-flow__edge')
    for (const fraction of [0.35, 0.65, 0.5, 0.2, 0.8]) {
      const point = curve.getPointAtLength(curve.getTotalLength() * fraction).matrixTransform(transform)
      if (document.elementFromPoint(point.x, point.y)?.closest('.react-flow__edge') === edge) {
        return { x: point.x, y: point.y }
      }
    }
    return null
  })
  await expect.poll(findPoint, {
    message: 'Connector interaction path must be reachable before opening its menu',
  }).not.toBeNull()
  const point = await findPoint()
  if (!point) throw new Error('Connector interaction path is not reachable')
  await page.mouse.click(point.x, point.y, { button: 'right' })
  try {
    await expect(moveTarget).toBeVisible({ timeout: 1000 })
  } catch {
    await longPressLocator(path, { clientX: point.x, clientY: point.y, durationMs: 650 })
    await expect(moveTarget).toBeVisible()
  }
}

async function clickRevealedHandle(
  node: ReturnType<typeof nodeByName>,
  handle: ReturnType<typeof handleLocator>,
) {
  await node.hover()
  await handle.click()
}

test('starts, cancels, and creates click-connect flows from handles', async ({ page }) => {
  const { diagram, elements } = await createAndLoadDiagramWithNodes(page, 2, 'Connector Click Handles')
  await closeViewEditorPanels(page)
  const sourceNode = nodeByName(page, elements[0].name)
  const sourceHandle = handleLocator(page, elements[0].id, 'right-2')
  const targetHandle = handleLocator(page, elements[1].id, 'left-2')

  await clickRevealedHandle(sourceNode, sourceHandle)
  await expect(sourceNode.getByText(/tap element to connect/i)).toBeVisible()
  await sourceHandle.click()
  await expect(sourceNode.getByText(/tap element to connect/i)).toHaveCount(0)
  await expect.poll(async () => (await listConnectors(page, diagram.id)).length).toBe(0)

  await clickRevealedHandle(sourceNode, sourceHandle)
  await targetHandle.click()
  await expectConnector(page, {
    sourceElementId: elements[0].id,
    targetElementId: elements[1].id,
  }, true, diagram.id)
})

test('drags handles to target handles, node bodies, and the inline connection picker', async ({ page }) => {
  const { diagram, elements } = await createAndLoadDiagramWithNodes(page, 4, 'Connector Drag Matrix')
  await closeViewEditorPanels(page)

  await dragLocatorToPoint(page, handleLocator(page, elements[0].id, 'right-2'), await locatorCenter(handleLocator(page, elements[1].id, 'left-2')))
  await expectConnector(page, {
    sourceElementId: elements[0].id,
    targetElementId: elements[1].id,
  }, true, diagram.id)

  await dragLocatorToPoint(page, handleLocator(page, elements[1].id, 'right-2'), await locatorCenter(nodeByName(page, elements[2].name)))
  await expectConnector(page, {
    sourceElementId: elements[1].id,
    targetElementId: elements[2].id,
  }, true, diagram.id)

  const paneBox = await reactFlowPaneBox(page)
  await dragLocatorToPointWithMidDragCheck(page, handleLocator(page, elements[2].id, 'right-2'), {
    x: paneBox.x + paneBox.width * 0.64,
    y: paneBox.y + paneBox.height * 0.78,
  }, async () => {
    await expect(page.getByTestId('click-connect-connector')).toHaveCount(0)
  })
  await expect(page.getByTestId('pending-element-label-input')).toBeVisible()
  await expect(page.getByTestId('pending-element-mode-add')).toHaveAttribute('aria-pressed', 'true')
  const pendingName = uniqueName('Connector Pending Target')
  await confirmInlineNewElement(page, pendingName)
  await expect(nodeByName(page, pendingName)).toBeVisible()
  await expect.poll(async () => (await listConnectors(page, diagram.id)).length).toBeGreaterThanOrEqual(3)

  const shiftFixture = await createAndLoadDiagramWithNodes(page, 1, 'Connector Shift Empty')
  await closeViewEditorPanels(page)
  const shiftPaneBox = await reactFlowPaneBox(page)
  await page.keyboard.down('Shift')
  await dragLocatorToPoint(page, handleLocator(page, shiftFixture.elements[0].id, 'right-2'), {
    x: shiftPaneBox.x + shiftPaneBox.width * 0.72,
    y: shiftPaneBox.y + shiftPaneBox.height * 0.78,
  })
  await page.keyboard.up('Shift')
  await expect(page.getByTestId('pending-element-label-input')).toBeVisible()
  await page.keyboard.press('Escape')
})

test('defaults to add and connect and can connect to off-view elements without adding them', async ({ page }) => {
  const { diagram, elements } = await createAndLoadDiagramWithNodes(page, 1, 'Off-view Picker')
  const externalView = await createApiView(page, uniqueName('External View'))
  const external = await createPlacedElement(page, externalView.id, { name: uniqueName('External Target') })
  const unplaced = await createElement(page, { name: uniqueName('Unplaced Target') })
  await reloadView(page)
  await closeViewEditorPanels(page)

  const openPicker = async () => {
    await clickRevealedHandle(nodeByName(page, elements[0].name), handleLocator(page, elements[0].id, 'right-2'))
    const paneBox = await reactFlowPaneBox(page)
    await page.mouse.click(paneBox.x + paneBox.width * 0.35, paneBox.y + paneBox.height * 0.72)
    await expect(page.getByTestId('pending-element-mode-add')).toHaveAttribute('aria-pressed', 'true')
    await page.getByTestId('pending-element-mode-connect').click()
  }

  await openPicker()
  const input = page.getByTestId('pending-element-label-input')
  await input.fill(unplaced.name)
  const unplacedOption = page.getByTestId('pending-element-existing-option').filter({ hasText: unplaced.name })
  await expect(unplacedOption).toHaveAttribute('aria-disabled', 'false')
  await expect(page.getByTestId('pending-element-create-option')).toHaveCount(0)
  await input.press('Enter')
  await expectConnector(page, {
    sourceElementId: elements[0].id,
    targetElementId: unplaced.id,
  }, true, 0)
  expect((await listPlacements(page, diagram.id)).map((placement) => placement.elementId)).toEqual([elements[0].id])
  await expect(page.locator('button[aria-label="Expand Unplaced elements"]')).toBeVisible()
  await reloadView(page)
  await closeViewEditorPanels(page)
  const expandUnplaced = page.locator('button[aria-label="Expand Unplaced elements"]')
  await expect(expandUnplaced).toBeVisible()
  await waitForStableLocator(expandUnplaced)
  await expandUnplaced.click()
  await expect(page.locator('button[aria-label="Collapse Unplaced elements"]')).toBeVisible()
  await page.keyboard.press('f')
  await expect(page.locator('.react-flow__node-contextNeighborNode').filter({ hasText: unplaced.name })).toBeVisible()

  await openPicker()

  await input.fill(external.name)
  const externalOption = page.getByTestId('pending-element-existing-option').filter({ hasText: external.name })
  await expect(externalOption).toHaveAttribute('aria-disabled', 'false')
  await input.press('Enter')
  await expectConnector(page, {
    sourceElementId: elements[0].id,
    targetElementId: external.id,
  }, true, 0)
  expect((await listPlacements(page, diagram.id)).map((placement) => placement.elementId)).toEqual([elements[0].id])
  await expect(page.locator('.react-flow__edge-proxyConnectorEdge').first()).toBeAttached()

  await openPicker()
  await input.fill(external.name)
  await page.getByTestId('pending-element-mode-add').click()
  await expect(input).toHaveValue(external.name)
  await expect(page.getByTestId('pending-element-mode-add')).toHaveAttribute('aria-pressed', 'true')
  await externalOption.click()
  await expectPlacement(page, external.name, true, diagram.id)
  await expect(nodeByName(page, external.name)).toBeVisible()
  await expect.poll(async () => (await listConnectors(page, diagram.id)).length).toBe(2)

  await page.keyboard.press('c')
  await expect(input).toBeVisible()
  await expect(page.getByTestId('pending-element-mode-connect')).toHaveCount(0)
  const newName = uniqueName('Standalone Element')
  await confirmInlineNewElement(page, newName)
  await expectPlacement(page, newName, true, diagram.id)
})

test('switches an unmatched connection search to add and connect only once per pending element', async ({ page }) => {
  const { elements } = await createAndLoadDiagramWithNodes(page, 1, 'Connection Search Fallback')
  await closeViewEditorPanels(page)
  const input = page.getByTestId('pending-element-label-input')
  const addMode = page.getByTestId('pending-element-mode-add')
  const connectMode = page.getByTestId('pending-element-mode-connect')

  const openPicker = async () => {
    await clickRevealedHandle(nodeByName(page, elements[0].name), handleLocator(page, elements[0].id, 'right-2'))
    const paneBox = await reactFlowPaneBox(page)
    await page.mouse.click(paneBox.x + paneBox.width * 0.35, paneBox.y + paneBox.height * 0.72)
    await expect(addMode).toHaveAttribute('aria-pressed', 'true')
    const modes = page.getByRole('group', { name: 'Target placement' }).getByRole('button')
    await expect(modes).toHaveText(['Add & connect', 'Connect only'])
    await connectMode.click()
    await expect(connectMode).toHaveAttribute('aria-pressed', 'true')
  }

  await openPicker()
  const unmatchedName = uniqueName('No Matching Element')
  let releaseSearch!: () => void
  const searchGate = new Promise<void>((resolve) => { releaseSearch = resolve })
  await page.route('**/ListElements', async (route) => {
    if (route.request().postDataJSON()?.search === unmatchedName) await searchGate
    await route.continue()
  })
  const searchStarted = page.waitForRequest((request) => request.url().endsWith('/ListElements') && request.postDataJSON()?.search === unmatchedName)
  await input.fill(unmatchedName)
  await searchStarted
  await expect(connectMode).toHaveAttribute('aria-pressed', 'true')
  releaseSearch()
  await expect(addMode).toHaveAttribute('aria-pressed', 'true')
  await expect(page.getByTestId('pending-element-create-option')).toBeVisible()

  await connectMode.click()
  await expect(page.getByText('No matching elements.', { exact: true })).toBeVisible()
  await input.fill(uniqueName('Still No Matching Element'))
  await expect(page.getByText('No matching elements.', { exact: true })).toBeVisible()
  await expect(connectMode).toHaveAttribute('aria-pressed', 'true')

  await input.press('Escape')
  await expect(input).toHaveCount(0)
  await openPicker()
  await input.fill(uniqueName('Another New Element'))
  await expect(addMode).toHaveAttribute('aria-pressed', 'true')
})

test('moves connector source and target through the connector context menu', async ({ page }) => {
  const { diagram, elements } = await createAndLoadDiagramWithNodes(page, 3, 'Connector Context Rewire')
  const connector = await createConnector(page, diagram.id, elements[0].id, elements[1].id, { label: 'context-rewire' })
  await reloadView(page)
  await closeViewEditorPanels(page)

  await openEdgeContextMenu(page)
  await page.getByTestId('connector-context-move-target').click()
  await expect(page.getByText(/Tap a node to set as new target/)).toBeVisible()
  await nodeByName(page, elements[2].name).click()
  await expectConnector(page, {
    id: connector.id,
    sourceElementId: elements[0].id,
    targetElementId: elements[2].id,
  }, true, diagram.id)

  await reloadView(page)
  await closeViewEditorPanels(page)
  await openEdgeContextMenu(page)
  const moveSource = page.getByTestId('connector-context-move-source')
  await expect(moveSource).toBeVisible()
  await moveSource.click()
  await expect(page.getByText(/Tap a node to set as new source/)).toBeVisible()
  await nodeByName(page, elements[1].name).click()
  await expectConnector(page, {
    id: connector.id,
    sourceElementId: elements[1].id,
    targetElementId: elements[2].id,
  }, true, diagram.id)
})

test('drags reconnect zones to a nearby handle and a nearby node body', async ({ page }) => {
  const { diagram, elements } = await createAndLoadDiagramWithNodes(page, 4, 'Connector Reconnect Zones')
  const connector = await createConnector(page, diagram.id, elements[0].id, elements[1].id, { label: 'zone-reconnect' })
  await reloadView(page)
  await closeViewEditorPanels(page)

  const edge = page.locator('.react-flow__edge').first()
  await edge.click({ force: true })
  const targetZone = page.locator(`[data-testid="vieweditor-node-reconnect-zone"][data-edge-id="${connector.id}"][data-endpoint="target"]`).first()
  await expect(targetZone).toBeVisible()
  await dragLocatorToPoint(page, targetZone, await locatorCenter(handleLocator(page, elements[2].id, 'left-2')), true)
  await expectConnector(page, {
    id: connector.id,
    sourceElementId: elements[0].id,
    targetElementId: elements[2].id,
  }, true, diagram.id)

  await reloadView(page)
  await closeViewEditorPanels(page)
  await page.locator('.react-flow__edge').first().click({ force: true })
  const movedTargetZone = page.locator(`[data-testid="vieweditor-node-reconnect-zone"][data-edge-id="${connector.id}"][data-endpoint="target"]`).first()
  await expect(movedTargetZone).toBeVisible()
  const bodyBox = await nodeByName(page, elements[3].name).boundingBox()
  if (!bodyBox) throw new Error('Expected reconnect body target to be visible')
  await dragLocatorToPoint(page, movedTargetZone, {
    x: bodyBox.x + bodyBox.width + 18,
    y: bodyBox.y + bodyBox.height / 2,
  }, true)
  await expectConnector(page, {
    id: connector.id,
    sourceElementId: elements[0].id,
    targetElementId: elements[3].id,
  }, true, diagram.id)
})
