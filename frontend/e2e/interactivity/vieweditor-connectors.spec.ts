import { expect, test } from '../fixtures'
import {
  closeViewEditorPanels,
  confirmInlineNewElement,
  createAndLoadDiagramWithNodes,
  createConnector,
  expectConnector,
  handleLocator,
  isMobileLayout,
  listConnectors,
  locatorCenter,
  longPressLocator,
  nodeByName,
  reactFlowPaneBox,
  reloadView,
  uniqueName,
} from '../helpers/vieweditor'

async function dragLocatorToPoint(
  page: Parameters<typeof handleLocator>[0],
  locator: ReturnType<typeof handleLocator>,
  point: { x: number; y: number },
  useTouchOnMobile = false,
) {
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

async function openEdgeContextMenu(page: Parameters<typeof handleLocator>[0]) {
  const edge = page.locator('.react-flow__edge').first()
  await edge.click({ button: 'right', force: true })
  if (await page.getByTestId('connector-context-move-target').isVisible({ timeout: 800 }).catch(() => false)) {
    return
  }
  await longPressLocator(edge, { durationMs: 650 })
}

test('starts, cancels, and creates click-connect flows from handles', async ({ page }) => {
  const { diagram, elements } = await createAndLoadDiagramWithNodes(page, 2, 'Connector Click Handles')
  await closeViewEditorPanels(page)
  const sourceHandle = handleLocator(page, elements[0].id, 'right-2')
  const targetHandle = handleLocator(page, elements[1].id, 'left-2')

  await sourceHandle.click({ force: true })
  await expect(nodeByName(page, elements[0].name).getByText(/tap element to connect/i)).toBeVisible()
  await sourceHandle.click({ force: true })
  await expect(nodeByName(page, elements[0].name).getByText(/tap element to connect/i)).toHaveCount(0)
  await expect.poll(async () => (await listConnectors(page, diagram.id)).length).toBe(0)

  await sourceHandle.click({ force: true })
  await targetHandle.click({ force: true })
  await expectConnector(page, {
    sourceElementId: elements[0].id,
    targetElementId: elements[1].id,
  }, true, diagram.id)
})

test('drags handles to target handles, node bodies, empty pending elements, and Shift-forced connect mode', async ({ page }) => {
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
  await dragLocatorToPoint(page, handleLocator(page, elements[2].id, 'right-2'), {
    x: paneBox.x + paneBox.width * 0.64,
    y: paneBox.y + paneBox.height * 0.78,
  })
  await expect(page.getByTestId('pending-element-label-input')).toBeVisible()
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
  if (!await moveSource.isVisible({ timeout: 1500 }).catch(() => false)) {
    return
  }
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
