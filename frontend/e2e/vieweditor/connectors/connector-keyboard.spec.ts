import { expect, test } from '../../fixtures'
import {
  createAndLoadDiagramWithNodes,
  createApiView,
  createPlacedElement,
  expectConnector,
  gotoView,
  handleLocator,
  listConnectors,
  locatorCenter,
  nodeByName,
  uniqueName,
} from '../../helpers/vieweditor'


test('creates a connector with the E shortcut and target click flow', async ({ page }) => {
  const { diagram, elements } = await createAndLoadDiagramWithNodes(page, 2, 'Keyboard Connector')

  await nodeByName(page, elements[0].name).click()
  await page.keyboard.press('e')
  await nodeByName(page, elements[1].name).click()

  await expectConnector(page, {
    sourceElementId: elements[0].id,
    targetElementId: elements[1].id,
  }, true, diagram.id)
  await expect(page.locator('.react-flow__edge')).toHaveCount(1)
})

test('E shortcut persists the preview source handle on target click', async ({ page }) => {
  const prefix = uniqueName('Keyboard Connector Source Handle')
  const diagram = await createApiView(page, `${prefix} Diagram`)
  // A slow setup drag can trigger long-press connector creation on CI WebKit.
  const elements = [
    await createPlacedElement(page, diagram.id, { name: `${prefix} Source`, kind: 'service' }, 120, 280),
    await createPlacedElement(page, diagram.id, { name: `${prefix} Target`, kind: 'database' }, 120, 80),
  ]
  await gotoView(page, diagram.id)
  const sourceNode = nodeByName(page, elements[0].name)
  const targetNode = nodeByName(page, elements[1].name)
  await expect(targetNode).toBeVisible()
  const expectedSourceHandle = 'top'

  await sourceNode.click()
  await page.keyboard.press('e')
  await expect(sourceNode.getByText(/tap element to connect/)).toBeVisible()
  const movedTargetCenter = await locatorCenter(targetNode)
  await page.mouse.move(movedTargetCenter.x, movedTargetCenter.y)
  await expect(page.getByTestId('click-connect-connector')).toHaveCount(1)
  const ghostPathStyle = await page.getByTestId('click-connect-connector').locator('.vieweditor-temporary-connector-path').evaluate((path) => {
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
  expect(ghostPathStyle.stroke).toBe(ghostPathStyle.accent)
  expect(ghostPathStyle.strokeDasharray).toBe('6px, 5px')
  expect(ghostPathStyle.opacity).toBe('1')
  await targetNode.click()

  await expect.poll(async () => {
    const connectors = await listConnectors(page, diagram.id)
    const connector = connectors.find((candidate) =>
      candidate.sourceElementId === elements[0].id && candidate.targetElementId === elements[1].id
    )
    return connector?.sourceHandle ?? connector?.source_handle ?? null
  }).toBe(expectedSourceHandle)
})

test('clicking the source handle cancels keyboard connector creation', async ({ page }) => {
  const { diagram, elements } = await createAndLoadDiagramWithNodes(page, 2, 'Keyboard Connector Cancel')
  const sourceNode = nodeByName(page, elements[0].name)

  await sourceNode.click()
  await page.keyboard.press('e')
  await expect(sourceNode.getByText(/tap element to connect/)).toBeVisible()
  const handleBox = await sourceNode.locator('.react-flow__handle').first().boundingBox()
  expect(handleBox).toBeTruthy()
  await page.mouse.click(handleBox!.x + handleBox!.width / 2, handleBox!.y + handleBox!.height / 2)
  await expect(sourceNode.getByText(/tap element to connect/)).toHaveCount(0)
  await nodeByName(page, elements[1].name).click()

  await expect.poll(async () => (await listConnectors(page, diagram.id)).length).toBe(0)
})

test('Escape cancels source-handle connector creation', async ({ page }) => {
  const { diagram, elements } = await createAndLoadDiagramWithNodes(page, 2, 'Handle Connector Escape')
  const sourceNode = nodeByName(page, elements[0].name)
  const sourceHandle = handleLocator(page, elements[0].id, 'right-2')

  await sourceNode.hover()
  await sourceHandle.click()
  await expect(sourceNode.getByText(/tap element to connect/)).toBeVisible()
  await page.keyboard.press('Escape')
  await expect(sourceNode.getByText(/tap element to connect/)).toHaveCount(0)
  await nodeByName(page, elements[1].name).click()

  await expect.poll(async () => (await listConnectors(page, diagram.id)).length).toBe(0)
})
