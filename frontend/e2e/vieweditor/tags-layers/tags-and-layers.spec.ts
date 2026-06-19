import { expect, test } from '../../fixtures'
import {
  createAndLoadDiagramWithNodes,
  createTag,
  createLayer,
  getElement,
  listLayers,
  nodeByName,
  openElementPanel,
  openViewExplorer,
  uniqueName,
} from '../../helpers/vieweditor'


test('shows a tag in the explorer and applies it to an element from the panel', async ({ page }) => {
  const { elements } = await createAndLoadDiagramWithNodes(page, 1, 'Tag Toggle')
  const tag = uniqueName('qa-tag')

  await createTag(page, tag)
  await page.reload()
  await openViewExplorer(page)
  const otherTags = page.getByRole('button', { name: /Other tags/ })
  await expect(otherTags).toBeVisible()
  await otherTags.click()
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
  await openViewExplorer(page)
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
  await openViewExplorer(page)

  const layerItem = page.getByTestId('tag-manager-layer').filter({ hasText: layer.name })
  await layerItem.click()
  await layerItem.getByTestId('tag-manager-layer-delete').click()

  await expect.poll(async () => {
    const layers = await listLayers(page, diagram.id)
    return layers.some((candidate) => candidate.id === layer.id)
  }).toBeFalsy()
})

test('dragging tags and layers onto a canvas node updates element tags', async ({ page }) => {
  const tag = uniqueName('drop-tag')
  const layerTagA = uniqueName('layer-a')
  const layerTagB = uniqueName('layer-b')
  const { diagram, elements } = await createAndLoadDiagramWithNodes(page, 1, 'Tag Drop')

  await createTag(page, tag, '#F6AD55')
  await createTag(page, layerTagA, '#68D391')
  await createTag(page, layerTagB, '#63B3ED')
  const layer = await createLayer(page, diagram.id, { name: uniqueName('Drop Layer'), tags: [layerTagA, layerTagB] })
  await page.reload()
  await openViewExplorer(page)

  const otherTags = page.getByRole('button', { name: /Other tags/ })
  await expect(otherTags).toBeVisible()
  await otherTags.click()
  const tagItem = page.getByTestId('tag-manager-tag').filter({ hasText: tag })
  await expect(tagItem).toBeVisible()

  const target = nodeByName(page, elements[0].name)
  const targetBox = await target.boundingBox()
  if (!targetBox) throw new Error('Missing node target')
  const dropPoint = {
    clientX: targetBox.x + targetBox.width / 2,
    clientY: targetBox.y + targetBox.height / 2,
  }

  const tagTransfer = await page.evaluateHandle(() => new DataTransfer())
  await tagItem.dispatchEvent('dragstart', { dataTransfer: tagTransfer })
  await target.dispatchEvent('dragover', { dataTransfer: tagTransfer, ...dropPoint })
  await target.dispatchEvent('drop', { dataTransfer: tagTransfer, ...dropPoint })

  await expect.poll(async () => {
    const element = await getElement(page, elements[0].id)
    return element.tags ?? []
  }).toContain(tag)

  const layerTransfer = await page.evaluateHandle(() => new DataTransfer())
  await page.getByTestId('tag-manager-layer').filter({ hasText: layer.name }).dispatchEvent('dragstart', { dataTransfer: layerTransfer })
  await target.dispatchEvent('dragover', { dataTransfer: layerTransfer, ...dropPoint })
  await target.dispatchEvent('drop', { dataTransfer: layerTransfer, ...dropPoint })

  await expect.poll(async () => {
    const element = await getElement(page, elements[0].id)
    return element.tags ?? []
  }).toEqual(expect.arrayContaining([tag, layerTagA, layerTagB]))
})
