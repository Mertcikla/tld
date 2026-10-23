import { expect, test } from '../fixtures'
import { createApiView, createConnector, createPlacedElement, uniqueName } from '../helpers/vieweditor'
import { waitForStableZuiCamera, waitForZuiReady, zuiScreenGeometry } from '../helpers/zui'

test('uses matching connector cards in the editor panel and explore popover', async ({ page }, testInfo) => {
  const root = await createApiView(page, uniqueName('Connector Details'))
  const source = await createPlacedElement(page, root.id, { name: 'API', kind: 'service' }, 120, 140)
  const target = await createPlacedElement(page, root.id, { name: 'Database', kind: 'database' }, 520, 140)
  const sourceView = await createApiView(page, 'API internals', source.id)
  const targetView = await createApiView(page, 'Database internals', target.id)
  const worker = await createPlacedElement(page, sourceView.id, { name: 'Worker', kind: 'component' }, 120, 140)
  const store = await createPlacedElement(page, targetView.id, { name: 'Store', kind: 'component' }, 120, 140)
  await createConnector(page, root.id, source.id, target.id, { label: 'calls', style: 'straight' })
  await createConnector(page, sourceView.id, worker.id, store.id, {
    label: 'loads', relationship: 'SQL', description: 'Loads account records', style: 'straight',
  })

  await page.goto(`/views/${root.id}`)
  const badge = page.getByRole('button', { name: '+1', exact: true })
  // Labels mount before the initial viewport is ready; opacity does not gate Playwright clicks.
  await expect(page.locator('.react-flow__edgelabel-renderer').filter({ has: badge })).toHaveCSS('opacity', '1')
  await badge.hover()
  await badge.click()
  await expect(page.getByText('Underlying Connectors · 1', { exact: true })).toBeVisible()
  await expect(page.getByText('loads · SQL', { exact: true })).toBeVisible()
  await expect(page.getByText('Loads account records', { exact: true })).toBeVisible()
  await expect(page.getByRole('button', { name: 'Edit connector', exact: true })).toBeVisible()
  await expect(page.getByTestId('proxy-connector-panel').locator('..')).toHaveCSS('transform', 'none')
  await page.screenshot({ path: testInfo.outputPath('connector-panel.png') })

  await page.goto(`/views?view=explore&debugZuiTest=1&focus=${root.id}`)
  await waitForZuiReady(page)
  await waitForStableZuiCamera(page)
  const { connector } = await zuiScreenGeometry(page, {
    connector: { sourceElementId: source.id, targetElementId: target.id },
  }) as { connector: { midPoint: { x: number; y: number } } }
  await page.mouse.move(connector.midPoint.x, connector.midPoint.y)
  const popover = page.getByTestId('zui-hover-popover')
  await expect(popover).toBeVisible()
  await expect(popover).toContainText('Underlying Connectors · 1')
  await expect(popover).toContainText('loads · SQL')
  await expect(popover).toContainText('Loads account records')
  await expect(popover.getByRole('link', { name: 'Worker', exact: true })).toHaveAttribute('href', `/views/${sourceView.id}?element=${worker.id}`)
  const storeLink = popover.getByRole('link', { name: 'Store', exact: true })
  await expect(storeLink).toHaveAttribute('href', `/views/${targetView.id}?element=${store.id}`)
  await expect(popover).toHaveCSS('opacity', '1')
  await page.screenshot({ path: testInfo.outputPath('connector-popover.png') })
  await storeLink.click()
  await expect(page.getByTestId('element-panel-name-input')).toHaveValue('Store')
  await expect(page).toHaveURL(new RegExp(`/views/${targetView.id}$`))
})
