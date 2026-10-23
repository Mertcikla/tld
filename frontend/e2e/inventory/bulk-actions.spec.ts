import { expect, test } from '../fixtures'
import {
  createConnectorGraph,
  createTag,
  getElement,
  listElements,
  uniqueName,
} from '../helpers/vieweditor'

test('bulk inventory actions add remove tags and delete only selected rows', async ({ page }) => {
  const prefix = uniqueName('Inventory Bulk')
  const tag = uniqueName('bulk-tag')
  await createTag(page, tag, '#38BDF8')
  const graph = await createConnectorGraph(page, prefix)
  const first = graph.incoming
  const second = graph.both
  const survivor = graph.outgoing

  await page.goto(`/inventory?type=elements&q=${encodeURIComponent(prefix)}`)
  await expect(page.getByTestId('inventory-page')).toBeVisible()
  await expect(page.locator(`[data-inventory-key="element:${first.id}"]`)).toBeVisible()
  await expect(page.locator(`[data-inventory-key="element:${second.id}"]`)).toBeVisible()
  await expect(page.locator(`[data-inventory-key="element:${survivor.id}"]`)).toBeVisible()

  await expect(page.getByTestId(`inventory-row-select-element:${first.id}`).locator('input')).toBeEnabled()
  await page.getByTestId(`inventory-row-select-element:${first.id}`).click({ force: true })
  await page.getByTestId(`inventory-row-select-element:${second.id}`).click({ force: true })
  await expect(page.getByText('2 selected', { exact: true })).toBeVisible()

  await page.getByTestId('inventory-bulk-add-tag').click()
  await page.getByTestId(`inventory-bulk-add-tag-${tag}`).click()

  await expect.poll(async () => {
    const [firstElement, secondElement, survivorElement] = await Promise.all([
      getElement(page, first.id),
      getElement(page, second.id),
      getElement(page, survivor.id),
    ])
    return [
      firstElement.tags?.includes(tag) ?? false,
      secondElement.tags?.includes(tag) ?? false,
      survivorElement.tags?.includes(tag) ?? false,
    ]
  }).toEqual([true, true, false])

  await expect(page.getByTestId(`inventory-row-select-element:${first.id}`).locator('input')).toBeEnabled()
  await page.getByTestId(`inventory-row-select-element:${first.id}`).click({ force: true })
  await page.getByTestId(`inventory-row-select-element:${second.id}`).click({ force: true })
  await page.getByTestId('inventory-bulk-remove-tag').click()
  await page.getByTestId(`inventory-bulk-remove-tag-${tag}`).click()

  await expect.poll(async () => {
    const [firstElement, secondElement] = await Promise.all([
      getElement(page, first.id),
      getElement(page, second.id),
    ])
    return [
      firstElement.tags?.includes(tag) ?? false,
      secondElement.tags?.includes(tag) ?? false,
    ]
  }).toEqual([false, false])

  await expect(page.getByTestId(`inventory-row-select-element:${first.id}`).locator('input')).toBeEnabled()
  await page.getByTestId(`inventory-row-select-element:${first.id}`).click({ force: true })
  await page.getByTestId(`inventory-row-select-element:${second.id}`).click({ force: true })
  await page.getByTestId('inventory-bulk-delete').click()
  await expect(page.getByTestId('inventory-bulk-delete-confirm')).toBeVisible()
  await page.getByRole('button', { name: 'Cancel' }).click()
  await expect(page.getByTestId('inventory-bulk-delete-confirm')).toBeHidden()
  await expect(page.getByTestId(`inventory-row-select-element:${first.id}`)).toBeVisible()

  await page.getByTestId('inventory-bulk-delete').click()
  await page.getByTestId('inventory-bulk-delete-submit').click()

  await expect.poll(async () => {
    const elements = await listElements(page, prefix)
    const names = elements.map((element) => element.name)
    return {
      hasFirst: names.includes(first.name),
      hasSecond: names.includes(second.name),
      hasSurvivor: names.includes(survivor.name),
    }
  }).toEqual({ hasFirst: false, hasSecond: false, hasSurvivor: true })
})

test('select all matching replaces selections outside the current filters', async ({ page }) => {
  const prefix = uniqueName('Inventory Selection')
  const graph = await createConnectorGraph(page, prefix)
  await page.goto(`/inventory?type=elements&q=${encodeURIComponent(prefix)}`)
  await page.getByTestId(`inventory-row-select-element:${graph.center.id}`).click()
  await page.getByTestId('inventory-search').fill(graph.incoming.name)
  await expect(page.getByText('1 outside current filters', { exact: true })).toBeVisible()
  await page.getByRole('button', { name: 'Select all 1 matching', exact: true }).click()
  await expect(page.getByText('1 outside current filters', { exact: true })).toHaveCount(0)
  await page.getByTestId('inventory-search').fill(prefix)
  await expect(page.getByTestId(`inventory-row-select-element:${graph.center.id}`).locator('input')).not.toBeChecked()
  await expect(page.getByTestId(`inventory-row-select-element:${graph.incoming.id}`).locator('input')).toBeChecked()
})
