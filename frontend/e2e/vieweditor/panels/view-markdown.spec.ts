import { expect, test } from '../../fixtures'
import {
  createApiView,
  createViewMarkdown,
  getViewMarkdown,
  gotoView,
  saveViewMarkdown,
  uniqueName,
} from '../../helpers/vieweditor'

test('notes panel can attach reload and detach markdown notes', async ({ page }) => {
  const view = await createApiView(page, uniqueName('Markdown Notes Diagram'))
  const sourceView = await createApiView(page, uniqueName('Markdown Notes Source'))
  const fileName = `${uniqueName('notes').replace(/\s+/g, '-').toLowerCase()}.md`
  const nextContent = `# Updated notes\n\nThis content was saved from the E2E markdown editor.`
  await createViewMarkdown(page, sourceView.id, fileName)
  const sourceMarkdown = await getViewMarkdown(page, sourceView.id)
  const markdownPath = sourceMarkdown?.markdown?.path
  expect(markdownPath).toBeTruthy()

  await gotoView(page, view.id)

  await page.getByTestId('vieweditor-toolbar-markdown').click()
  await expect(page.getByTestId('view-markdown-panel')).toBeVisible()
  await page.getByTestId('view-markdown-attach-path').fill(markdownPath!)
  await page.getByTestId('view-markdown-attach').click()

  await expect.poll(async () => {
    const markdown = await getViewMarkdown(page, view.id)
    return markdown?.markdown?.path === markdownPath
  }).toBe(true)

  await expect(page.getByTestId('view-markdown-status')).toContainText('Attached file')
  await expect(page.getByRole('textbox', { name: 'editable markdown' })).toBeVisible()
  await saveViewMarkdown(page, view.id, nextContent)
  await page.getByTestId('view-markdown-panel').getByRole('button', { name: 'Reload' }).click()
  await expect(page.getByRole('textbox', { name: 'editable markdown' })).toContainText('Updated notes')

  await page.getByTestId('view-markdown-panel').getByRole('button', { name: 'Close' }).click()
  await page.getByText(view.name).click()
  await expect(page.getByTestId('view-panel')).toBeVisible()
  await page.getByTestId('view-panel-markdown-unlink').click()
  await expect.poll(async () => await getViewMarkdown(page, view.id)).toBeNull()
})
