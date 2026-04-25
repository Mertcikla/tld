import { expect, test } from '@playwright/test'
import {
  createAndLoadDiagramWithNodes,
  createLayer,
  getElement,
  listLayers,
  nodeByName,
  openElementPanel,
  prepareStorage,
  uniqueName,
} from '../../helpers/vieweditor'

test.beforeEach(async ({ page }) => {
  await prepareStorage(page)
})

test('creates a tag in the explorer and applies it to an element from the panel', async ({ page }) => {
  const { elements } = await createAndLoadDiagramWithNodes(page, 1, 'Tag Toggle')
  const tag = uniqueName('qa-tag')

  await page.getByTestId('tag-manager-add-tag').click()
  await page.getByTestId('tag-manager-new-tag-name').fill(tag)
  await page.getByTestId('tag-manager-new-tag-submit').click()
  await expect(page.getByTestId('tag-manager-tag').filter({ hasText: tag })).toBeVisible()

  await openElementPanel(page, elements[0].name)
  await page.getByTestId('tag-upsert-input').fill(tag)
  await page.keyboard.press('ArrowDown')
  await page.keyboard.press('Enter')

  await expect.poll(async () => {
    const element = await getElement(page, elements[0].id)
    return (element.tags ?? []).includes(tag)
  }).toBeTruthy()
})

test('layer visibility hides and shows nodes with matching tags', async ({ page }) => {
  const tag = uniqueName('layer-tag')
  const { diagram, elements } = await createAndLoadDiagramWithNodes(page, 1, 'Layer Visibility')

  await openElementPanel(page, elements[0].name)
  await page.getByTestId('tag-upsert-input').fill(tag)
  await page.getByTestId('tag-upsert-input').press('Enter')
  await page.getByTestId('panel-close').click()
  await expect.poll(async () => (await getElement(page, elements[0].id)).tags?.includes(tag)).toBeTruthy()

  const layer = await createLayer(page, diagram.id, { name: uniqueName('QA Layer'), tags: [tag] })
  await page.reload()
  await expect(page.getByTestId('tag-manager-layer').filter({ hasText: layer.name })).toBeVisible()

  await page.getByTestId('tag-manager-layer').filter({ hasText: layer.name }).getByTestId('tag-manager-layer-visibility').click()
  await expect.poll(async () => nodeByName(page, elements[0].name).evaluate((node) => {
    const wrapper = node.closest('.react-flow__node')
    return wrapper ? getComputedStyle(wrapper).opacity : getComputedStyle(node).opacity
  })).toBe('0.1')
  await page.getByTestId('tag-manager-layer').filter({ hasText: layer.name }).getByTestId('tag-manager-layer-visibility').click()
  await expect.poll(async () => nodeByName(page, elements[0].name).evaluate((node) => {
    const wrapper = node.closest('.react-flow__node')
    return wrapper ? getComputedStyle(wrapper).opacity : getComputedStyle(node).opacity
  })).toBe('1')
})

test('deletes a layer from the explorer tag manager', async ({ page }) => {
  const { diagram } = await createAndLoadDiagramWithNodes(page, 0, 'Layer Delete')
  const layer = await createLayer(page, diagram.id, { name: uniqueName('Delete Layer'), tags: ['temporary'] })
  await page.reload()

  const layerItem = page.getByTestId('tag-manager-layer').filter({ hasText: layer.name })
  await layerItem.click()
  await layerItem.getByTestId('tag-manager-layer-delete').click()

  await expect.poll(async () => {
    const layers = await listLayers(page, diagram.id)
    return layers.some((candidate) => candidate.id === layer.id)
  }).toBeFalsy()
})
