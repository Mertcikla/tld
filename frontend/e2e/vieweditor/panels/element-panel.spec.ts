import { expect, test } from '@playwright/test'
import {
  createAndLoadDiagramWithNodes,
  expectPlacement,
  getElement,
  nodeByName,
  openElementPanel,
  prepareStorage,
  uniqueName,
} from '../../helpers/vieweditor'

test.beforeEach(async ({ page }) => {
  await prepareStorage(page)
})

test('edits element fields and persists them after reload', async ({ page }) => {
  const { elements } = await createAndLoadDiagramWithNodes(page, 1, 'Panel Edit')
  const original = elements[0]
  const nextName = uniqueName('Panel Renamed')
  const tag = uniqueName('panel-tag')

  await openElementPanel(page, original.name)
  await page.getByTestId('element-panel-name-input').fill(nextName)
  await page.getByTestId('element-panel-type-input').fill('service')
  await page.getByTestId('element-panel-type-input').press('Enter')
  await page.getByTestId('element-panel-description-input').fill('Edited from Playwright')
  await page.getByTestId('element-panel-url-input').fill('https://example.com/element')
  await page.getByTestId('tag-upsert-input').fill(tag)
  await page.getByTestId('tag-upsert-input').press('Enter')
  await page.getByTestId('element-panel-url-input').blur()

  await expect.poll(async () => {
    const element = await getElement(page, original.id)
    return {
      name: element.name,
      kind: element.kind,
      description: element.description,
      url: element.url,
      hasTag: (element.tags ?? []).includes(tag),
    }
  }).toEqual({
    name: nextName,
    kind: 'service',
    description: 'Edited from Playwright',
    url: 'https://example.com/element',
    hasTag: true,
  })

  await page.reload()
  await expect(nodeByName(page, nextName)).toBeVisible()
  await expectPlacement(page, nextName, true)
})

test('permanently deletes an element from every diagram after confirmation', async ({ page }) => {
  const { elements } = await createAndLoadDiagramWithNodes(page, 1, 'Permanent Delete')
  const nodeName = elements[0].name

  await openElementPanel(page, nodeName)
  await page.getByTestId('element-panel-delete-permanent').click()
  await expect(page.getByTestId('confirm-dialog')).toBeVisible()
  await page.getByTestId('confirm-dialog-confirm').click()

  await expect(nodeByName(page, nodeName)).toHaveCount(0)
  await expectPlacement(page, nodeName, false)
  await expect.poll(async () => {
    const response = await page.request.post('/api/diag.v1.WorkspaceService/GetElement', {
      data: { elementId: elements[0].id },
    })
    return response.status()
  }).not.toBe(200)
})
