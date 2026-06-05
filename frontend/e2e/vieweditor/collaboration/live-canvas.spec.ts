import type { Browser } from '@playwright/test'
import { expect, test } from '../../fixtures'
import {
  addNodeWithKeyboard,
  createApiView,
  createCollaborationPair,
  createConnector,
  createConnectorWithKeyboard,
  createPlacedElement,
  dragNodeByName,
  getElement,
  listConnectors,
  listPlacements,
  nodeByName,
  openConnectorPanelFromFirstEdge,
  openElementPanel,
  removeNodeFromPanel,
  uniqueName,
  type CollaborationPair,
} from '../../helpers/vieweditor'

async function withCollaborationPair(
  browser: Browser,
  viewId: number,
  baseURL: string | undefined,
  run: (pair: CollaborationPair) => Promise<void>,
) {
  const pair = await createCollaborationPair(browser, viewId, baseURL, uniqueName('Collab User'))
  try {
    await run(pair)
  } finally {
    await pair.close()
  }
}

async function expectRenderedConnectorEdge(pair: CollaborationPair) {
  await expect(pair.bob.page.locator('.react-flow__edge')).toHaveCount(1)
  await expect.poll(async () => {
    const path = pair.bob.page.locator('.react-flow__edge path').first()
    const d = await path.getAttribute('d').catch(() => null)
    return Boolean(d && d.startsWith('M '))
  }).toBe(true)
}

test('element placement create appears on the second user canvas', async ({ page, browser, baseURL }) => {
  const diagram = await createApiView(page, uniqueName('Collab Element Create'))

  await withCollaborationPair(browser, diagram.id, baseURL, async ({ alice, bob }) => {
    const nodeName = await addNodeWithKeyboard(alice.page, uniqueName('Collab Created Node'))

    await expect(nodeByName(bob.page, nodeName)).toBeVisible()
  })
})

test('element move appears on the second user canvas', async ({ page, browser, baseURL }) => {
  const diagram = await createApiView(page, uniqueName('Collab Element Move'))
  const element = await createPlacedElement(page, diagram.id, { name: uniqueName('Collab Moved Node'), kind: 'service' }, 160, 180)

  await withCollaborationPair(browser, diagram.id, baseURL, async ({ alice, bob }) => {
    await expect(nodeByName(bob.page, element.name)).toBeVisible()
    const before = await nodeByName(bob.page, element.name).boundingBox()
    expect(before).toBeTruthy()

    await dragNodeByName(alice.page, element.name, 260, 170)

    await expect.poll(async () => {
      const after = await nodeByName(bob.page, element.name).boundingBox()
      if (!after || !before) return 0
      return Math.hypot(after.x - before.x, after.y - before.y)
    }).toBeGreaterThan(40)
  })
})

test('element rename appears on the second user canvas', async ({ page, browser, baseURL }) => {
  const diagram = await createApiView(page, uniqueName('Collab Element Rename'))
  const element = await createPlacedElement(page, diagram.id, { name: uniqueName('Collab Rename Source'), kind: 'service' }, 180, 180)
  const nextName = uniqueName('Collab Renamed Node')

  await withCollaborationPair(browser, diagram.id, baseURL, async ({ alice, bob }) => {
    await openElementPanel(alice.page, element.name)
    const panel = alice.page.getByTestId('element-panel').filter({ visible: true }).last()
    await panel.getByTestId('element-panel-name-input').fill(nextName)
    await panel.getByTestId('element-panel-name-input').blur()

    await expect.poll(async () => (await getElement(page, element.id)).name).toBe(nextName)
    await expect(nodeByName(bob.page, nextName)).toBeVisible()
  })
})

test('element removal disappears from the second user canvas', async ({ page, browser, baseURL }) => {
  const diagram = await createApiView(page, uniqueName('Collab Element Remove'))
  const element = await createPlacedElement(page, diagram.id, { name: uniqueName('Collab Removed Node'), kind: 'service' }, 180, 180)

  await withCollaborationPair(browser, diagram.id, baseURL, async ({ alice, bob }) => {
    await expect(nodeByName(bob.page, element.name)).toBeVisible()

    await removeNodeFromPanel(alice.page, element.name)

    await expect.poll(async () => {
      const placements = await listPlacements(page, diagram.id)
      return placements.some((placement) => placement.elementId === element.id)
    }).toBe(false)
    await expect(nodeByName(bob.page, element.name)).toHaveCount(0)
  })
})

test('connector create appears on the second user canvas', async ({ page, browser, baseURL }) => {
  const diagram = await createApiView(page, uniqueName('Collab Connector Create'))
  const source = await createPlacedElement(page, diagram.id, { name: uniqueName('Collab Connector Source'), kind: 'service' }, 160, 220)
  const target = await createPlacedElement(page, diagram.id, { name: uniqueName('Collab Connector Target'), kind: 'database' }, 520, 220)

  await withCollaborationPair(browser, diagram.id, baseURL, async (pair) => {
    const { alice } = pair
    await createConnectorWithKeyboard(alice.page, source.name, target.name)

    await expect.poll(async () => (await listConnectors(page, diagram.id)).length).toBe(1)
    await expectRenderedConnectorEdge(pair)
  })
})

test('connector metadata update appears on the second user canvas', async ({ page, browser, baseURL }) => {
  const diagram = await createApiView(page, uniqueName('Collab Connector Update'))
  const source = await createPlacedElement(page, diagram.id, { name: uniqueName('Collab Update Source'), kind: 'service' }, 160, 220)
  const target = await createPlacedElement(page, diagram.id, { name: uniqueName('Collab Update Target'), kind: 'database' }, 520, 220)
  const connector = await createConnector(page, diagram.id, source.id, target.id, { label: 'initial collaboration label' })
  const nextLabel = uniqueName('live connector label')

  await withCollaborationPair(browser, diagram.id, baseURL, async ({ alice, bob }) => {
    await expect(bob.page.locator('.react-flow__edge')).toHaveCount(1)

    await openConnectorPanelFromFirstEdge(alice.page)
    await alice.page.getByTestId('connector-panel-label-input').fill(nextLabel)
    await alice.page.getByTestId('connector-panel-description-input').blur()

    await expect.poll(async () => {
      const connectors = await listConnectors(page, diagram.id)
      return connectors.find((candidate) => candidate.id === connector.id)?.label
    }).toBe(nextLabel)
    await expect(bob.page.getByText(nextLabel)).toBeVisible()
  })
})

test('connector delete disappears from the second user canvas', async ({ page, browser, baseURL }) => {
  const diagram = await createApiView(page, uniqueName('Collab Connector Delete'))
  const source = await createPlacedElement(page, diagram.id, { name: uniqueName('Collab Delete Source'), kind: 'service' }, 160, 220)
  const target = await createPlacedElement(page, diagram.id, { name: uniqueName('Collab Delete Target'), kind: 'database' }, 520, 220)
  const connector = await createConnector(page, diagram.id, source.id, target.id, { label: 'delete collaboration label' })

  await withCollaborationPair(browser, diagram.id, baseURL, async ({ alice, bob }) => {
    await expect(bob.page.locator('.react-flow__edge')).toHaveCount(1)

    await openConnectorPanelFromFirstEdge(alice.page)
    await alice.page.getByTestId('connector-panel-delete').click()
    await alice.page.getByTestId('confirm-dialog-confirm').click()

    await expect.poll(async () => {
      const connectors = await listConnectors(page, diagram.id)
      return connectors.some((candidate) => candidate.id === connector.id)
    }).toBe(false)
    await expect(bob.page.locator('.react-flow__edge')).toHaveCount(0)
  })
})
