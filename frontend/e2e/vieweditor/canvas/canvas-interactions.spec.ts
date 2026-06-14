import { expect, test } from '../../fixtures'
import {
  createAndLoadDiagramWithNodes,
  expectConnector,
  expectPlacement,
  listPlacements,
  nodeByName,
  reactFlowPaneBox,
  uniqueName,
} from '../../helpers/vieweditor'


test('clicking the canvas deselects nodes and closes the element panel', async ({ page }) => {
  const { elements } = await createAndLoadDiagramWithNodes(page, 1, 'Canvas Select')
  await nodeByName(page, elements[0].name).click()
  await expect(page.getByTestId('element-panel')).toBeVisible()

  const box = await reactFlowPaneBox(page)
  await page.mouse.click(box.x + 40, box.y + 40)
  await expect(page.getByTestId('element-panel')).toHaveCount(0)
})

test('dragging a node persists its position after debounce and reload', async ({ page }) => {
  const { diagram, elements } = await createAndLoadDiagramWithNodes(page, 1, 'Canvas Drag')
  const node = nodeByName(page, elements[0].name)
  const before = (await listPlacements(page, diagram.id)).find((placement) => placement.elementId === elements[0].id)
  expect(before).toBeTruthy()

  const box = await node.boundingBox()
  expect(box).toBeTruthy()
  await page.mouse.move(box!.x + box!.width / 2, box!.y + box!.height / 2)
  await page.mouse.down()
  await page.mouse.move(box!.x + box!.width / 2 + 280, box!.y + box!.height / 2 + 180, { steps: 12 })
  await page.mouse.up()

  await expect.poll(async () => {
    const after = (await listPlacements(page, diagram.id)).find((placement) => placement.elementId === elements[0].id)
    return Boolean(after && (Math.abs(after.positionX - before!.positionX) > 20 || Math.abs(after.positionY - before!.positionY) > 20))
  }).toBeTruthy()

  await page.reload()
  await expect(nodeByName(page, elements[0].name)).toBeVisible()
})

test('connector handle drag previews a pending placeholder only over empty canvas', async ({ page }, testInfo) => {
  test.skip(
    testInfo.project.name === 'webkit',
    'WebKit does not reliably expose the pending placeholder during synthetic mouse drag',
  )

  const { elements } = await createAndLoadDiagramWithNodes(page, 2, 'Connector Placeholder')
  const sourceNode = page.locator(`.react-flow__node[data-id="${elements[0].id}"]`)
  const targetNode = page.locator(`.react-flow__node[data-id="${elements[1].id}"]`)
  const sourceHandle = sourceNode.locator('.react-flow__handle[data-handleid="right-2"]')
  const pendingNode = page.locator('.react-flow__node[data-id="pending-element"]')

  await expect(sourceHandle).toBeVisible()
  await expect(targetNode).toBeVisible()

  const sourceBox = await sourceHandle.boundingBox()
  const targetBox = await targetNode.boundingBox()
  const paneBox = await reactFlowPaneBox(page)
  expect(sourceBox).toBeTruthy()
  expect(targetBox).toBeTruthy()
  const emptyCanvasPoint = {
    x: paneBox.x + paneBox.width * 0.36,
    y: paneBox.y + paneBox.height * 0.82,
  }

  await page.mouse.move(sourceBox!.x + sourceBox!.width / 2, sourceBox!.y + sourceBox!.height / 2)
  await page.mouse.down()
  await page.mouse.move(emptyCanvasPoint.x, emptyCanvasPoint.y, { steps: 10 })

  await expect(pendingNode).toBeVisible()
  await expect(page.getByTestId('pending-element-label-input')).toHaveCount(0)
  const connectionPathStyle = await page.locator('.react-flow__connection path').evaluate((path) => {
    const style = getComputedStyle(path)
    const accentProbe = document.createElement('span')
    accentProbe.style.color = 'var(--accent)'
    document.body.appendChild(accentProbe)
    const accent = getComputedStyle(accentProbe).color
    accentProbe.remove()
    return {
      accent,
      stroke: style.stroke,
      strokeDasharray: style.strokeDasharray,
      opacity: style.opacity,
    }
  })
  expect(connectionPathStyle.stroke).toBe(connectionPathStyle.accent)
  expect(connectionPathStyle.strokeDasharray).toBe('6px, 5px')
  expect(connectionPathStyle.opacity).toBe('0')

  const pendingPreviewPathStyle = await page.locator('.vieweditor-temporary-connector .react-flow__edge-path').first().evaluate((path) => {
    const style = getComputedStyle(path)
    const accentProbe = document.createElement('span')
    accentProbe.style.color = 'var(--accent)'
    document.body.appendChild(accentProbe)
    const accent = getComputedStyle(accentProbe).color
    accentProbe.remove()
    return {
      accent,
      stroke: style.stroke,
      strokeDasharray: style.strokeDasharray,
      opacity: style.opacity,
    }
  })
  expect(pendingPreviewPathStyle.stroke).toBe(pendingPreviewPathStyle.accent)
  expect(pendingPreviewPathStyle.strokeDasharray).toBe('6px, 5px')
  expect(pendingPreviewPathStyle.opacity).toBe('1')

  await page.mouse.move(targetBox!.x + targetBox!.width / 2, targetBox!.y + targetBox!.height / 2, { steps: 10 })
  await expect(pendingNode).toHaveCount(0)

  await page.mouse.move(emptyCanvasPoint.x, emptyCanvasPoint.y, { steps: 10 })
  await expect(pendingNode).toBeVisible()

  await page.mouse.up()
  await expect(page.getByTestId('pending-element-label-input')).toBeVisible()
})

test('connector handle drag released over a node body creates a snapped connector', async ({ page }, testInfo) => {
  const { diagram, elements } = await createAndLoadDiagramWithNodes(page, 2, 'Connector Body Drop')
  const sourceNode = page.locator(`.react-flow__node[data-id="${elements[0].id}"]`)
  const targetNode = page.locator(`.react-flow__node[data-id="${elements[1].id}"]`)
  const sourceHandle = sourceNode.locator('.react-flow__handle[data-handleid="right-2"]')

  await expect(sourceHandle).toBeVisible()
  await expect(targetNode).toBeVisible()

  const sourceBox = await sourceHandle.boundingBox()
  const targetBox = await targetNode.boundingBox()
  expect(sourceBox).toBeTruthy()
  expect(targetBox).toBeTruthy()

  await page.mouse.move(sourceBox!.x + sourceBox!.width / 2, sourceBox!.y + sourceBox!.height / 2)
  await page.mouse.down()
  await page.mouse.move(targetBox!.x + targetBox!.width / 2, targetBox!.y + targetBox!.height / 2, { steps: 12 })
  if (testInfo.project.name !== 'webkit') {
    await expect(page.locator('.react-flow__node[data-id="pending-element"]')).toHaveCount(0)
  }
  await page.mouse.up()

  await expectConnector(page, {
    sourceElementId: elements[0].id,
    targetElementId: elements[1].id,
  }, true, diagram.id)
  await expect(page.getByTestId('pending-element-label-input')).toHaveCount(0)
})

test('canvas context menu snap-to-grid toggle persists in local storage', async ({ page }) => {
  await createAndLoadDiagramWithNodes(page, 0, 'Snap Toggle')
  const box = await reactFlowPaneBox(page)
  await page.mouse.click(box.x + box.width * 0.5, box.y + box.height * 0.5, { button: 'right' })
  await page.getByRole('button', { name: /Snap to Grid/ }).click()

  await expect.poll(async () => page.evaluate(() => localStorage.getItem('diag:snapToGrid'))).toBe('true')
})

test('Escape cancels an inline add operation without creating a placement', async ({ page }) => {
  await createAndLoadDiagramWithNodes(page, 0, 'Cancel Inline')
  const name = uniqueName('Canceled Node')
  await page.getByTestId('vieweditor-canvas').click()
  await page.keyboard.press('c')
  await page.getByTestId('pending-element-label-input').fill(name)
  await page.keyboard.press('Escape')

  await expect(page.getByTestId('pending-element-label-input')).toHaveCount(0)
  await expectPlacement(page, name, false)
})

test('single empty-canvas click cancels a focused pending element', async ({ page }) => {
  await createAndLoadDiagramWithNodes(page, 0, 'Cancel Pending Click')
  const name = uniqueName('Single Click Canceled Node')
  const canvas = page.getByTestId('vieweditor-canvas')
  const paneBox = await reactFlowPaneBox(page)

  await canvas.click()
  await page.keyboard.press('c')
  await page.getByTestId('pending-element-label-input').fill(name)
  await expect(page.getByTestId('pending-element-label-input')).toBeFocused()

  await page.mouse.click(paneBox.x + paneBox.width * 0.75, paneBox.y + paneBox.height * 0.72)

  await expect(page.getByTestId('pending-element-label-input')).toHaveCount(0)
  await expectPlacement(page, name, false)
})
