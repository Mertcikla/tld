import { describe, expect, it } from 'vitest'
import {
  extractTldMermaidViewId,
  findMermaidMarkdownBlocks,
  getMermaidMarkdownSyncStatus,
  upsertMermaidMarkdownBlock,
} from './markdown'

const mermaidCode = `flowchart LR
%% tld/v1 view=42
  node_1["API"]
`

describe('Mermaid markdown blocks', () => {
  it('extracts view ids from tld/v1 comments inside Mermaid blocks', () => {
    expect(extractTldMermaidViewId(mermaidCode)).toBe(42)
    expect(extractTldMermaidViewId('flowchart LR\n%% tld/v1\n  A --> B')).toBeNull()
  })

  it('finds fenced Mermaid blocks and keeps unrelated markdown untouched', () => {
    const markdown = `# Notes

\`\`\`mermaid
${mermaidCode}\`\`\`

\`\`\`ts
console.log('not mermaid')
\`\`\`
`

    const blocks = findMermaidMarkdownBlocks(markdown)

    expect(blocks).toHaveLength(1)
    expect(blocks[0]?.viewId).toBe(42)
    expect(blocks[0]?.code).toContain('node_1')
  })

  it('replaces the block for the current view and appends when missing', () => {
    const nextCode = `flowchart LR
%% tld/v1 view=42
  node_2["Worker"]
`
    const updated = upsertMermaidMarkdownBlock(`# Notes\n\n\`\`\`mermaid\n${mermaidCode}\`\`\`\n`, 42, nextCode)

    expect(updated).toContain('node_2["Worker"]')
    expect(updated).not.toContain('node_1["API"]')

    const appended = upsertMermaidMarkdownBlock('# Shared notes\n', 42, nextCode)
    expect(appended).toBe(`# Shared notes

\`\`\`mermaid
${nextCode.trim()}
\`\`\`
`)
  })

  it('reports sync status against the exported current view', () => {
    expect(getMermaidMarkdownSyncStatus('', 42, mermaidCode)).toBe('missing')
    expect(getMermaidMarkdownSyncStatus(`\`\`\`mermaid\n${mermaidCode}\`\`\``, 42, mermaidCode)).toBe('synced')
    expect(getMermaidMarkdownSyncStatus(`\`\`\`mermaid\n${mermaidCode.replace('API', 'Old API')}\`\`\``, 42, mermaidCode)).toBe('stale')
  })
})
