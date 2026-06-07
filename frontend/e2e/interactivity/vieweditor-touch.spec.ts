import { expect, test } from '../fixtures'
import type { Page } from '@playwright/test'
import {
  closeViewEditorPanels,
  createAndLoadDiagramWithNodes,
  createElement,
  currentViewId,
  expectConnector,
  expectPlacement,
  libraryItemByName,
  locatorCenter,
  longPressLocator,
  nodeByName,
  openElementLibrary,
  reactFlowPaneBox,
  reloadView,
  uniqueName,
} from '../helpers/vieweditor'

async function dispatchDocumentPointer(page: Page, type: 'pointermove' | 'pointerup' | 'pointercancel', options: {
  clientX: number
  clientY: number
  pointerId?: number
  pointerType?: 'touch' | 'pen'
  buttons?: number
}) {
  await page.evaluate((payload) => {
    const init = {
      bubbles: true,
      cancelable: true,
      clientX: payload.clientX,
      clientY: payload.clientY,
      pointerId: payload.pointerId,
      pointerType: payload.pointerType,
      button: 0,
      buttons: payload.buttons,
    }
    const event = typeof PointerEvent === 'function'
      ? new PointerEvent(payload.type, init)
      : new MouseEvent(payload.type, init)
    document.dispatchEvent(event)
  }, {
    type,
    clientX: options.clientX,
    clientY: options.clientY,
    pointerId: options.pointerId ?? 41,
    pointerType: options.pointerType ?? 'touch',
    buttons: options.buttons ?? (type === 'pointerup' || type === 'pointercancel' ? 0 : 1),
  })
}

async function touchDragLibraryItem(page: Page, name: string, delta: { x: number; y: number }, release: { x: number; y: number }) {
  const item = libraryItemByName(page, name)
  await expect(item).toBeVisible()
  const start = await locatorCenter(item)
  await item.dispatchEvent('pointerdown', {
    bubbles: true,
    cancelable: true,
    clientX: start.x,
    clientY: start.y,
    pointerId: 41,
    pointerType: 'touch',
    button: 0,
    buttons: 1,
  })
  await dispatchDocumentPointer(page, 'pointermove', {
    clientX: start.x + delta.x,
    clientY: start.y + delta.y,
  })
  await dispatchDocumentPointer(page, 'pointerup', {
    clientX: release.x,
    clientY: release.y,
  })
}

async function clickLibraryAction(page: Page, name: string, actionTestId: 'element-library-add' | 'element-library-find') {
  await libraryItemByName(page, name).getByTestId(actionTestId).evaluate((element) => {
    (element as HTMLElement).click()
  })
}

test('long-presses canvas and nodes, then taps through click-connect mode', async ({ page }) => {
  const { diagram, elements } = await createAndLoadDiagramWithNodes(page, 2, 'Touch Long Press')
  await closeViewEditorPanels(page)
  const paneBox = await reactFlowPaneBox(page)
  await longPressLocator(page.getByTestId('vieweditor-canvas'), {
    clientX: paneBox.x + paneBox.width * 0.55,
    clientY: paneBox.y + paneBox.height * 0.68,
    durationMs: 650,
  })
  await expect(page.getByTestId('vieweditor-canvas-context-add-element')).toBeVisible()
  await page.mouse.click(paneBox.x + paneBox.width * 0.8, paneBox.y + paneBox.height * 0.78)

  await nodeByName(page, elements[0].name).click()
  await expect(page.getByTestId('element-panel')).toBeVisible()
  await page.keyboard.press('Escape')

  await longPressLocator(nodeByName(page, elements[0].name), { durationMs: 560 })
  await expect(nodeByName(page, elements[0].name).getByText(/tap element to connect/i)).toBeVisible()
  await nodeByName(page, elements[1].name).click()
  await expectConnector(page, {
    sourceElementId: elements[0].id,
    targetElementId: elements[1].id,
  }, true, diagram.id)
})

test('drags mobile library items horizontally, cancels vertical scroll gestures, and taps add/find actions', async ({ page }) => {
  await createAndLoadDiagramWithNodes(page, 0, 'Touch Library')
  await closeViewEditorPanels(page)
  const horizontal = await createElement(page, { name: uniqueName('Touch Horizontal Drop'), kind: 'service' })
  const vertical = await createElement(page, { name: uniqueName('Touch Vertical Cancel'), kind: 'database' })
  const addable = await createElement(page, { name: uniqueName('Touch Tap Add'), kind: 'api' })
  await reloadView(page)

  await openElementLibrary(page)
  const hideExisting = page.getByRole('checkbox', { name: 'Hide existing' })
  if (await hideExisting.isChecked().catch(() => false)) {
    await hideExisting.uncheck({ force: true })
  }
  await page.getByTestId('element-library-search').fill(horizontal.name)
  const paneBox = await reactFlowPaneBox(page)
  await touchDragLibraryItem(page, horizontal.name, { x: 80, y: 4 }, {
    x: paneBox.x + paneBox.width * 0.58,
    y: paneBox.y + paneBox.height * 0.46,
  })
  await expectPlacement(page, horizontal.name, true, currentViewId(page))
  await expect(nodeByName(page, horizontal.name)).toBeVisible()

  await reloadView(page)
  await openElementLibrary(page)
  await page.getByTestId('element-library-search').fill(vertical.name)
  await touchDragLibraryItem(page, vertical.name, { x: 4, y: 90 }, {
    x: paneBox.x + paneBox.width * 0.6,
    y: paneBox.y + paneBox.height * 0.55,
  })
  await expectPlacement(page, vertical.name, false, currentViewId(page))

  await page.getByTestId('element-library-search').fill(addable.name)
  await clickLibraryAction(page, addable.name, 'element-library-add')
  await expectPlacement(page, addable.name, true, currentViewId(page))
  await expect(nodeByName(page, addable.name)).toBeVisible()

  await openElementLibrary(page)
  if (await hideExisting.isChecked().catch(() => false)) {
    await hideExisting.uncheck({ force: true })
  }
  await page.getByTestId('element-library-search').fill(horizontal.name)
  if (await libraryItemByName(page, horizontal.name).count() === 0) {
    await expect(nodeByName(page, horizontal.name)).toBeVisible()
    return
  }
  await expect(libraryItemByName(page, horizontal.name)).toBeVisible()
  await clickLibraryAction(page, horizontal.name, 'element-library-find')
  await expect(nodeByName(page, horizontal.name)).toBeVisible()
})
