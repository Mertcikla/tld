import { expect, test } from '../fixtures'
import type { Page } from '@playwright/test'
import {
  createConnectorGraph,
  uniqueName,
} from '../helpers/vieweditor'

test.beforeEach(async ({ page }) => {
  await page.addInitScript(() => {
    localStorage.setItem('diag:InventoryDrawer-expanded', 'true')
  })
})

function neighbourCard(page: Page, name: string) {
  const token = name.slice(0, 28)
  return page.locator('[data-testid="inventory-connector-card"], [data-testid="dependencies-neighbour-card"]').filter({ hasText: token }).first()
}

function connectorCards(page: Page) {
  return page.locator('[data-testid="inventory-connector-card"], [data-testid="dependencies-neighbour-card"]')
}

async function expectConnectorGraphCards(page: Page, graph: Awaited<ReturnType<typeof createConnectorGraph>>) {
  await expect(selectedCard(page)).toContainText(graph.center.name.slice(0, 28))
  await expect(neighbourCard(page, graph.incoming.name)).toBeVisible()
  await expect(neighbourCard(page, graph.outgoing.name)).toBeVisible()
  await expect(neighbourCard(page, graph.both.name)).toBeVisible()
  await expect(neighbourCard(page, graph.undirected.name)).toBeVisible()
  await expect(connectorCards(page)).toHaveCount(4)
}

function selectedCard(page: Page) {
  return page.getByTestId('inventory-selected-card')
}

test('search empty state can be cleared in inventory', async ({ page }) => {
  await page.goto('/inventory')
  await page.getByTestId('inventory-search').fill(uniqueName('no-match'))

  await expect(page.getByText('No matching objects')).toBeVisible()
  await expect(page.getByText(/^0 \//)).toBeVisible()
  await page.getByTestId('inventory-search').fill('')
  await expect(page.getByTestId('inventory-search')).toHaveValue('')
})

test('lists elements, totals, and connector counts from the workspace', async ({ page }) => {
  const graph = await createConnectorGraph(page, 'Connector Totals')

  await page.goto(`/inventory?object=element:${graph.center.id}`)

  await expect(page.getByTestId('inventory-row').filter({ hasText: graph.center.name }).first()).toBeVisible()
  await expect(page.getByText(graph.center.name, { exact: false }).first()).toBeVisible()
  await expectConnectorGraphCards(page, graph)
})

test('query param selects an element and renders its connector graph', async ({ page }) => {
  const graph = await createConnectorGraph(page, 'Connector Query')

  // Tests redirect from /dependencies to /inventory?object=element:ID
  await page.goto(`/dependencies?element=${graph.center.id}`)

  await expect(page).toHaveURL(new RegExp(`/inventory\\?object=element:${graph.center.id}`))
  await expect(page.getByText(graph.center.name, { exact: false }).first()).toBeVisible()
  await expectConnectorGraphCards(page, graph)
})

test('connector graph groups incoming outgoing bidirectional and undirected connectors', async ({ page }) => {
  const graph = await createConnectorGraph(page, 'Connector Directions')

  await page.goto(`/inventory?object=element:${graph.center.id}`)

  const center = selectedCard(page)
  const incoming = neighbourCard(page, graph.incoming.name)
  const outgoing = neighbourCard(page, graph.outgoing.name)
  const both = neighbourCard(page, graph.both.name)
  const undirected = neighbourCard(page, graph.undirected.name)

  await expect(center).toBeVisible()
  await expect(center).toContainText(graph.center.name.slice(0, 28))
  await expect(incoming).toBeVisible()
  await expect(outgoing).toBeVisible()
  await expect(both).toBeVisible()
  await expect(undirected).toBeVisible()

  const centerBox = await center.boundingBox()
  const incomingBox = await incoming.boundingBox()
  const outgoingBox = await outgoing.boundingBox()
  const bothBox = await both.boundingBox()
  const undirectedBox = await undirected.boundingBox()

  expect(centerBox).not.toBeNull()
  expect(incomingBox).not.toBeNull()
  expect(outgoingBox).not.toBeNull()
  expect(bothBox).not.toBeNull()
  expect(undirectedBox).not.toBeNull()

  expect((incomingBox?.x ?? 0) + (incomingBox?.width ?? 0)).toBeLessThan((centerBox?.x ?? 0) + 4)
  expect(outgoingBox?.x ?? 0).toBeGreaterThan((centerBox?.x ?? 0) + (centerBox?.width ?? 0) - 4)
  expect((bothBox?.y ?? 0) + (bothBox?.height ?? 0)).toBeLessThan((centerBox?.y ?? 0) + 4)
  expect(undirectedBox?.y ?? 0).toBeGreaterThan((centerBox?.y ?? 0) + (centerBox?.height ?? 0) - 4)
})

test('selecting a connector card recenters the graph on that element', async ({ page }) => {
  const graph = await createConnectorGraph(page, 'Connector Neighbor Select')

  await page.goto(`/dependencies?element=${graph.center.id}`)
  await neighbourCard(page, graph.outgoing.name).click({ force: true })

  await expect(page).toHaveURL(/\/inventory\?/)
  await expect.poll(() => new URL(page.url()).searchParams.get('object')).toBe(`element:${graph.outgoing.id}`)
  await expect(page.getByText(graph.outgoing.name, { exact: false }).first()).toBeVisible()
})
