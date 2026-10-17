import { expect, test } from '../../fixtures'
import {
  createApiView,
  createPlacedElement,
  createViewMarkdown,
  getViewMarkdown,
  gotoView,
  saveViewMarkdown,
  uniqueName,
} from '../../helpers/vieweditor'

test('opening linked Mermaid views retains the markdown document and saves to its source view', async ({ page }) => {
  const source = await createApiView(page, uniqueName('Mermaid Notes'))
  const linked = await createApiView(page, uniqueName('Linked Mermaid'))
  const nextLinked = await createApiView(page, uniqueName('Another Linked Mermaid'))
  await createPlacedElement(page, linked.id, { name: uniqueName('Linked Service'), kind: 'service' })
  const content = `# Source notes\n\n\`\`\`mermaid\nflowchart LR\n%% tld/v1 view=${linked.id}\n  A --> B\n\`\`\`\n\n\`\`\`mermaid\nflowchart LR\n%% tld/v1 view=${nextLinked.id}\n  C --> D\n\`\`\``
  await createViewMarkdown(page, source.id, 'linked-mermaid.md', content)
  await createViewMarkdown(page, linked.id, 'destination-notes.md', '# Destination notes')
  await gotoView(page, source.id)
  await page.getByTestId('vieweditor-toolbar-markdown').click()
  const panel = page.getByTestId('view-markdown-panel')
  await panel.getByRole('button', { name: 'Preview', exact: true }).click()
  const preview = (id: number) => panel.locator(`[data-testid="tld-mermaid-preview"][data-tld-view-id="${id}"]`)
  await preview(linked.id).getByTestId('tld-mermaid-navigate-view').click()
  await expect(page).toHaveURL(new RegExp(`/views/${linked.id}$`))
  await expect(panel).toBeVisible()
  await expect(panel).toContainText('Source notes')
  await expect(panel).not.toContainText('Destination notes')

  // Repeated navigation must continue to retain the original document.
  await preview(nextLinked.id).getByTestId('tld-mermaid-navigate-view').click()
  await expect(page).toHaveURL(new RegExp(`/views/${nextLinked.id}$`))
  await panel.getByRole('button', { name: 'Source', exact: true }).click()
  const editor = panel.getByRole('textbox', { name: 'editable markdown' })
  await editor.fill(`${content}\n\nUpdated from the linked view`)
  await panel.getByTestId('markdown-save-button').click()
  await expect.poll(async () => (await getViewMarkdown(page, source.id))?.content).toContain('Updated from the linked view')
  expect((await getViewMarkdown(page, linked.id))?.content).toBe('# Destination notes')
  expect(await getViewMarkdown(page, nextLinked.id)).toBeNull()

  await panel.getByRole('button', { name: 'Reload', exact: true }).click()
  await expect(editor).toContainText('Updated from the linked view')
  await panel.getByRole('button', { name: 'Close', exact: true }).click()
  await expect(panel).toHaveCount(0)
})

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
