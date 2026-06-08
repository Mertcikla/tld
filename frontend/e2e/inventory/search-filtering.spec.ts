import { expect, test } from '../fixtures'
import type { Page } from '@playwright/test'
import {
  createApiView,
  createConnector,
  createPlacedElement,
  uniqueName,
  updateView,
} from '../helpers/vieweditor'

test.beforeEach(async ({ page }) => {
  await page.addInitScript(() => {
    localStorage.setItem('diag:InventoryDrawer-expanded', 'true')
  })
})

function rowByKey(page: Page, key: string) {
  return page.locator(`[data-inventory-key="${key}"]`)
}

async function expandFilterSection(page: Page, title: string) {
  await page.getByTestId(`inventory-filter-section-${title.toLowerCase()}`).getByText(title, { exact: true }).click()
}

function isTouchDeviceProject(projectName: string) {
  return ['mobile-safari', 'mobile-chrome', 'tablet-touch'].includes(projectName)
}

async function seedInventory(page: Page) {
  const prefix = uniqueName('Inventory Filter')
  const tag = `inv-${Date.now()}-${Math.random().toString(36).slice(2, 7)}`
  const diagram = await createApiView(page, `${prefix} Revenue Map`)
  await updateView(page, diagram.id, {
    name: `${prefix} Revenue Map`,
    description: `${prefix} money movement map`,
    levelLabel: 'Context',
    tags: [tag, 'view-scope'],
  })
  const api = await createPlacedElement(page, diagram.id, {
    name: `${prefix} Payments API`,
    kind: 'service',
    technology: 'Go',
    tags: [tag, 'payments'],
  })
  const worker = await createPlacedElement(page, diagram.id, {
    name: `${prefix} Billing Worker`,
    kind: 'worker',
    description: `${prefix} invoice jobs`,
    technology: 'Node',
    tags: [tag, 'billing'],
  }, 360, 140)
  const db = await createPlacedElement(page, diagram.id, {
    name: `${prefix} Ledger DB`,
    kind: 'database',
    description: `${prefix} durable storage`,
    technology: 'Postgres',
    tags: ['storage'],
  }, 600, 140)
  const sqlConnector = await createConnector(page, diagram.id, api.id, db.id, {
    label: `${prefix} writes ledger`,
    description: `${prefix} settlement edge`,
    relationship: 'SQL',
    direction: 'forward',
    style: 'step',
    tags: [tag, 'edge'],
  })
  const unlabeledConnector = await createConnector(page, diagram.id, worker.id, db.id, {
    relationship: 'AMQP',
    direction: 'forward',
    style: 'bezier',
    tags: [tag, 'needs-label'],
  })

  return { prefix, tag, diagram, api, worker, db, sqlConnector, unlabeledConnector }
}

test('global search matches element metadata and connector context', async ({ page }) => {
  const data = await seedInventory(page)

  await page.goto('/inventory')
  await page.getByTestId('inventory-search').fill(data.api.name)
  await expect(rowByKey(page, `element:${data.api.id}`)).toBeVisible()

  await page.getByTestId('inventory-search').fill(' invoice jobs ')
  await expect(rowByKey(page, `element:${data.worker.id}`)).toBeVisible()
  await expect(rowByKey(page, `element:${data.api.id}`)).toHaveCount(0)

  await page.getByTestId('inventory-search').fill('settlement edge')
  await expect(rowByKey(page, `connector:${data.sqlConnector.id}`)).toBeVisible()
  await expect(rowByKey(page, `element:${data.worker.id}`)).toHaveCount(0)

  await page.getByTestId('inventory-search').fill(data.db.name.toUpperCase())
  await expect(rowByKey(page, `connector:${data.sqlConnector.id}`)).toBeVisible()
  await expect(rowByKey(page, `connector:${data.unlabeledConnector.id}`)).toBeVisible()
  await expect(rowByKey(page, `element:${data.db.id}`)).toBeVisible()

  await page.getByTestId('inventory-search').fill(data.diagram.name)
  await expect(rowByKey(page, `connector:${data.sqlConnector.id}`)).toBeVisible()
  await expect(rowByKey(page, `view:${data.diagram.id}`)).toBeVisible()
})

test('type tag kind and quality filters compose and reset URL state', async ({ page }, testInfo) => {
  test.skip(isTouchDeviceProject(testInfo.project.name), 'filter sidebar is not rendered in touch device inventory layouts')

  const data = await seedInventory(page)

  await page.goto(`/inventory?page=2&object=element:${data.api.id}`)
  await expandFilterSection(page, 'Tags')
  await page.getByTestId(`inventory-tag-filter-${data.tag}`).click()

  await expect.poll(() => new URL(page.url()).searchParams.get('tags')).toBe(data.tag)
  expect(new URL(page.url()).searchParams.get('page')).toBeNull()
  expect(new URL(page.url()).searchParams.get('object')).toBeNull()
  await expect(rowByKey(page, `element:${data.api.id}`)).toBeVisible()
  await expect(rowByKey(page, `element:${data.worker.id}`)).toBeVisible()
  await expect(rowByKey(page, `element:${data.db.id}`)).toHaveCount(0)
  await expect(page.getByText(`tag: ${data.tag}`)).toBeVisible()

  await expandFilterSection(page, 'Kind')
  await page.getByTestId('inventory-kind-filter-service').click()
  await expect.poll(() => new URL(page.url()).searchParams.get('kind')).toBe('service')
  await expect(page.getByText('kind: service')).toBeVisible()
  await expect(rowByKey(page, `element:${data.api.id}`)).toBeVisible()
  await expect(rowByKey(page, `element:${data.worker.id}`)).toHaveCount(0)

  await expandFilterSection(page, 'Quality')
  await page.getByTestId('inventory-quality-filter-missing description').click()
  await expect.poll(() => new URL(page.url()).searchParams.get('qualities')).toBe('missing description')
  await expect(page.getByText('missing description').first()).toBeVisible()
  await expect(rowByKey(page, `element:${data.api.id}`)).toBeVisible()
  await expect(rowByKey(page, `element:${data.worker.id}`)).toHaveCount(0)

  await page.getByRole('button', { name: 'Clear all' }).click()
  await expect.poll(() => new URL(page.url()).searchParams.toString()).toBe('')
})

test('object type tab keeps search scoped and records type in the URL', async ({ page }, testInfo) => {
  test.skip(isTouchDeviceProject(testInfo.project.name), 'filter sidebar tabs are not rendered in touch device inventory layouts')

  const data = await seedInventory(page)

  await page.goto('/inventory')
  await page.getByTestId('inventory-search').fill(data.prefix)
  await page.getByTestId('inventory-tab-connectors').click()

  await expect.poll(() => new URL(page.url()).searchParams.get('type')).toBe('connectors')
  await expect(new URL(page.url()).searchParams.get('q')).toBe(data.prefix)
  await expect(rowByKey(page, `connector:${data.sqlConnector.id}`)).toBeVisible()
  await expect(rowByKey(page, `element:${data.api.id}`)).toHaveCount(0)

  await page.getByTestId('inventory-tab-elements').click()
  await expect.poll(() => new URL(page.url()).searchParams.get('type')).toBe('elements')
  await expect(rowByKey(page, `element:${data.api.id}`)).toBeVisible()
  await expect(rowByKey(page, `connector:${data.sqlConnector.id}`)).toHaveCount(0)
})

test('no-result search can be cleared and stale selected object params are removed', async ({ page }) => {
  const data = await seedInventory(page)

  await page.goto(`/inventory?page=2&object=element:${data.api.id}`)
  await page.getByTestId('inventory-search').fill(uniqueName('no-match'))
  await expect(page.getByText('No matching objects')).toBeVisible()
  await expect(page.getByText('0 /')).toBeVisible()
  expect(new URL(page.url()).searchParams.get('page')).toBeNull()
  expect(new URL(page.url()).searchParams.get('object')).toBeNull()

  await page.getByRole('button', { name: 'Clear search' }).click()
  await expect(page.getByTestId('inventory-search')).toHaveValue('')
  await expect(page.getByText('No matching objects')).toHaveCount(0)
  await expect(page.getByTestId('inventory-row').first()).toBeVisible()

  await page.getByTestId('inventory-search').fill(data.worker.name)
  await expect(rowByKey(page, `element:${data.worker.id}`)).toBeVisible()
  await expect(rowByKey(page, `element:${data.api.id}`)).toHaveCount(0)
  expect(new URL(page.url()).searchParams.get('object')).toBeNull()
})
