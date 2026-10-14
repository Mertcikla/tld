import { expect, test } from '../fixtures'
import type { Page } from '@playwright/test'
import { openInventory, searchInventory } from '../helpers/inventory'
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
  await page.getByTestId(`inventory-filter-section-${title.toLowerCase()}`).click()
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

  await openInventory(page)
  await searchInventory(page, data.api.name)
  await expect(rowByKey(page, `element:${data.api.id}`)).toBeVisible()
  // The matching row is also visible before search finishes rendering.
  await expect(rowByKey(page, `element:${data.worker.id}`)).toHaveCount(0)

  await searchInventory(page, ' invoice jobs ')
  await expect(rowByKey(page, `element:${data.worker.id}`)).toBeVisible()
  await expect(rowByKey(page, `element:${data.api.id}`)).toHaveCount(0)

  await searchInventory(page, 'settlement edge')
  await expect(rowByKey(page, `connector:${data.sqlConnector.id}`)).toBeVisible()
  await expect(rowByKey(page, `element:${data.worker.id}`)).toHaveCount(0)

  await searchInventory(page, data.db.name.toUpperCase())
  await expect(rowByKey(page, `connector:${data.sqlConnector.id}`)).toBeVisible()
  await expect(rowByKey(page, `connector:${data.unlabeledConnector.id}`)).toBeVisible()
  await expect(rowByKey(page, `element:${data.db.id}`)).toBeVisible()

  await searchInventory(page, data.diagram.name)
  await expect(rowByKey(page, `connector:${data.sqlConnector.id}`)).toBeVisible()
  await expect(rowByKey(page, `view:${data.diagram.id}`)).toBeVisible()
})

test('type tag kind and quality filters compose and reset URL state', async ({ page }) => {

  const data = await seedInventory(page)

  await openInventory(page, `/inventory?page=2&object=element:${data.api.id}`)
  await expect(rowByKey(page, `element:${data.api.id}`)).toBeVisible()
  if ((page.viewportSize()?.width ?? 0) >= 1280) {
    await expect(page.getByTestId('element-panel-name-input')).toHaveValue(data.api.name)
  }
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

  await page.getByRole('button', { name: 'Done', exact: true }).click()
  await page.getByRole('button', { name: 'Clear all' }).click()
  await expect.poll(() => new URL(page.url()).searchParams.toString()).toBe('')
})

test('object type tab keeps search scoped and records type in the URL', async ({ page }) => {

  const data = await seedInventory(page)

  await openInventory(page)
  await searchInventory(page, data.prefix)
  await expect(page.getByRole('button', { name: 'Clear search', exact: true })).toBeVisible()
  await page.getByTestId('inventory-tab-connectors').click()

  await expect.poll(() => new URL(page.url()).searchParams.get('type')).toBe('connectors')
  await expect.poll(() => new URL(page.url()).searchParams.get('q')).toBe(data.prefix)
  await expect(rowByKey(page, `connector:${data.sqlConnector.id}`)).toBeVisible()
  await expect(rowByKey(page, `element:${data.api.id}`)).toHaveCount(0)

  await page.getByTestId('inventory-tab-elements').click()
  await expect.poll(() => new URL(page.url()).searchParams.get('type')).toBe('elements')
  await expect(rowByKey(page, `element:${data.api.id}`)).toBeVisible()
  await expect(rowByKey(page, `connector:${data.sqlConnector.id}`)).toHaveCount(0)
})

test('no-result search can be cleared and stale selected object params are removed', async ({ page }) => {
  const data = await seedInventory(page)

  await openInventory(page, `/inventory?page=2&object=element:${data.api.id}`)
  await expect(rowByKey(page, `element:${data.api.id}`)).toBeVisible()
  await searchInventory(page, uniqueName('no-match'))
  await expect(page.getByText('No matching objects')).toBeVisible()
  await expect(page.getByTestId('inventory-pagination-summary')).toHaveText('0–0 of 0')
  expect(new URL(page.url()).searchParams.get('page')).toBeNull()
  expect(new URL(page.url()).searchParams.get('object')).toBeNull()

  await page.getByRole('button', { name: 'Clear search' }).click()
  await expect(page.getByTestId('inventory-search')).toHaveValue('')
  await expect(page.getByText('No matching objects')).toHaveCount(0)
  await expect(page.getByTestId('inventory-row').first()).toBeVisible()

  await searchInventory(page, data.worker.name)
  await expect(rowByKey(page, `element:${data.worker.id}`)).toBeVisible()
  await expect(rowByKey(page, `element:${data.api.id}`)).toHaveCount(0)
  expect(new URL(page.url()).searchParams.get('object')).toBeNull()
})

test('column filters keep sorting independent and remain accessible with hidden columns', async ({ page }) => {
  test.skip((page.viewportSize()?.width ?? 0) < 768, 'compact layouts use toolbar filters')
  const data = await seedInventory(page)
  await openInventory(page)
  const headers = page.locator('thead')
  await expect(headers.getByTestId('inventory-filter-section-tags')).toBeVisible()
  await expect(headers.getByTestId('inventory-filter-section-kind')).toBeVisible()
  await headers.getByRole('button', { name: 'Kind', exact: true }).click()
  await expect.poll(() => new URL(page.url()).searchParams.get('sort')).toBe('type')
  const kindHeader = headers.getByRole('columnheader').filter({
    has: page.getByRole('button', { name: 'Kind', exact: true }),
  })
  await expect(kindHeader).toHaveAttribute('aria-sort', /^(ascending|descending)$/)
  const sortParams = new URL(page.url()).searchParams
  expect(sortParams.get('sort')).toBe('type')
  await headers.getByTestId('inventory-filter-section-kind').click()
  await page.getByTestId('inventory-kind-filter-service').click()
  await expect.poll(() => new URL(page.url()).searchParams.get('kind')).toBe('service')
  expect(new URL(page.url()).searchParams.get('sort')).toBe('type')
  expect(new URL(page.url()).searchParams.get('dir')).toBe(sortParams.get('dir'))
  await page.getByRole('button', { name: 'Done', exact: true }).click()
  await page.getByRole('button', { name: 'Columns', exact: true }).click()
  await page.getByRole('menuitem', { name: 'Kind', exact: true }).click()
  await page.getByRole('menuitem', { name: 'Tags', exact: true }).click()
  await page.keyboard.press('Escape')
  await expect(headers.getByTestId('inventory-filter-section-kind')).toHaveCount(0)
  await expect(headers.getByTestId('inventory-filter-section-tags')).toHaveCount(0)
  await page.getByTestId('inventory-filter-section-tags').click()
  await page.getByRole('textbox', { name: 'Search tags filters', exact: true }).fill(data.tag)
  await page.getByTestId(`inventory-tag-filter-${data.tag}`).click()
  await page.getByRole('button', { name: 'Done', exact: true }).click()
  await expect(rowByKey(page, `element:${data.api.id}`)).toBeVisible()
  await expect(rowByKey(page, `element:${data.worker.id}`)).toHaveCount(0)
})

test('new quality checks filter metadata and stay scoped to the active type', async ({ page }) => {
  const prefix = uniqueName('Inventory Quality')
  const diagram = await createApiView(page, prefix)
  const incomplete = await createPlacedElement(page, diagram.id, { name: `${prefix} Incomplete`, kind: '', technology: '', description: '  ' })
  const documented = await createPlacedElement(page, diagram.id, { name: `${prefix} Documented`, kind: 'service', technology: 'Go', description: 'Documented service' })
  await openInventory(page, `/inventory?type=elements&q=${encodeURIComponent(prefix)}`)
  await page.getByTestId('inventory-filter-section-quality').click()
  await expect(page.getByTestId('inventory-quality-filter-empty view')).toHaveCount(0)
  await page.getByTestId('inventory-quality-filter-missing kind').click()
  await page.getByRole('button', { name: 'Done', exact: true }).click()
  await expect(rowByKey(page, `element:${incomplete.id}`)).toBeVisible()
  await expect(rowByKey(page, `element:${documented.id}`)).toHaveCount(0)
  await page.getByTestId('inventory-tab-views').click()
  expect(new URL(page.url()).searchParams.get('qualities')).toBeNull()
  await page.getByTestId('inventory-filter-section-quality').click()
  await expect(page.getByTestId('inventory-quality-filter-missing kind')).toHaveCount(0)
  await expect(page.getByTestId('inventory-quality-filter-missing level')).toBeVisible()
  await page.getByTestId('inventory-quality-filter-unconnected view').click()
  await page.getByRole('button', { name: 'Done', exact: true }).click()
  await expect(rowByKey(page, `view:${diagram.id}`)).toBeVisible()
  await page.getByTestId('inventory-tab-connectors').click()
  expect(new URL(page.url()).searchParams.get('qualities')).toBeNull()
  await page.getByTestId('inventory-filter-section-quality').click()
  await expect(page.getByTestId('inventory-quality-filter-unconnected view')).toHaveCount(0)
  await expect(page.getByTestId('inventory-quality-filter-missing relationship')).toBeVisible()
  await expect(page.getByTestId('inventory-quality-filter-self connection')).toBeVisible()
})
