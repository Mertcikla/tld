import { useContext, type FocusEvent, type MouseEvent } from 'react'
import { type CodeBlockEditorProps, useCodeBlockEditorContext } from '@mdxeditor/editor'
import { MermaidMarkdownContext } from './mermaidContext'

function openMermaidDetails(event: FocusEvent<HTMLDetailsElement> | MouseEvent<HTMLDetailsElement>) {
  event.currentTarget.open = true
}

function closeMermaidDetailsOnMouseLeave(event: MouseEvent<HTMLDetailsElement>) {
  if (event.currentTarget.matches(':focus-within')) return
  const movedBelow = event.clientY >= event.currentTarget.getBoundingClientRect().bottom - 1
  if (movedBelow) event.currentTarget.open = false
}

function closeMermaidDetailsOnBlur(event: FocusEvent<HTMLDetailsElement>) {
  const nextTarget = event.relatedTarget
  if (nextTarget && event.currentTarget.contains(nextTarget as Node)) return
  event.currentTarget.open = false
}

export function MermaidCollapsedCodeBlock({ code }: CodeBlockEditorProps) {
  const { blockStatusByCode, canEdit } = useContext(MermaidMarkdownContext)
  const { setCode } = useCodeBlockEditorContext()
  const status = blockStatusByCode.get(code) ?? 'unlinked'
  const statusLabel = status === 'synced'
    ? 'Synced'
    : status === 'stale'
      ? 'View changed'
      : status === 'other'
        ? 'Other view'
        : 'Unlinked'
  const lineCount = code.trim() ? code.trim().split(/\r?\n/).length : 0
  const lineLabel = `${lineCount} line${lineCount === 1 ? '' : 's'}`

  return (
    <details
      contentEditable={false}
      className={`tld-mermaid-markdown-block tld-mermaid-markdown-block--${status}`}
      onMouseEnter={openMermaidDetails}
      onMouseLeave={closeMermaidDetailsOnMouseLeave}
      onFocusCapture={openMermaidDetails}
      onBlurCapture={closeMermaidDetailsOnBlur}
    >
      <summary className="tld-mermaid-markdown-block__summary">
        {'```mermaid'}
        <span className="tld-mermaid-markdown-block__meta">
          {lineLabel} · {statusLabel}
        </span>
      </summary>
      {canEdit ? (
        <pre>
          <textarea
            className="tld-mermaid-markdown-block__textarea"
            value={code}
            onChange={(event) => setCode(event.currentTarget.value)}
            rows={Math.min(16, Math.max(4, lineCount))}
            spellCheck={false}
          />
        </pre>
      ) : (
        <pre>
          <code>{code}</code>
        </pre>
      )}
    </details>
  )
}
