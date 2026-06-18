import type { ViewMarkdownDocument } from '../../types'

export function markdownSourceLabel(markdown: ViewMarkdownDocument | null) {
  if (!markdown) return 'No notes'
  if (!markdown.exists) return 'Missing file'
  switch (markdown.source_kind) {
    case 'PRIVATE_WORKSPACE':
    case 'PRIVATE_APP':
      return 'Private note'
    case 'REPO':
      return 'Repo note'
    case 'ATTACHED':
      return 'Attached file'
    default:
      return markdown.is_managed ? 'Private note' : 'Attached file'
  }
}

export function markdownStatusLabel(markdown: ViewMarkdownDocument | null) {
  if (!markdown) return 'No notes'
  const source = markdownSourceLabel(markdown)
  if (!markdown.exists) return source
  if (!markdown.can_edit) return `${source} · read-only`
  if (markdown.source_kind === 'REPO' && markdown.git_state && markdown.git_state !== 'unknown') {
    return `${source} · ${markdown.git_state.replace(/_/g, ' ')}`
  }
  return source
}

export function defaultRepoMarkdownPath(viewName?: string | null) {
  const slug = (viewName || 'view-notes')
    .trim()
    .toLowerCase()
    .replace(/[^a-z0-9]+/g, '-')
    .replace(/^-+|-+$/g, '') || 'view-notes'
  return `docs/diagrams/${slug}.md`
}
