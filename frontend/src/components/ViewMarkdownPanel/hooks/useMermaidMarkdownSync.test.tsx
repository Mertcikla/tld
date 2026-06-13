import React from 'react'
import { act, create } from 'react-test-renderer'
import { describe, expect, it, vi } from 'vitest'
import { api } from '../../../api/client'
import { useMermaidMarkdownSync } from './useMermaidMarkdownSync'

vi.mock('../../../api/client', () => ({
  api: {
    mermaid: {
      inspectMarkdown: vi.fn(),
      upsertMarkdownBlock: vi.fn(),
    },
  },
}))

describe('useMermaidMarkdownSync', () => {
  it('upserts the current markdown without formatting it first', async () => {
    const exactMarkdown = '# Note\n\n\n```mermaid\nflowchart TD\n  A --> B\n```\n'
    const replaceMarkdown = vi.fn()
    const getMarkdown = vi.fn(() => exactMarkdown)
    vi.mocked(api.mermaid.upsertMarkdownBlock).mockResolvedValue({
      markdown: `${exactMarkdown}\nupdated`,
      previousStatus: 'stale',
      warnings: [],
    })
    let syncMarkdownBlock: (() => Promise<void> | void) | undefined

    function Harness() {
      const { handleSyncMermaidBlock } = useMermaidMarkdownSync({
        canEditDocument: true,
        content: exactMarkdown,
        enabled: false,
        getMarkdown,
        isOpen: true,
        replaceMarkdown,
        viewId: 6,
      })
      syncMarkdownBlock = handleSyncMermaidBlock
      return null
    }

    create(<Harness />)
    await act(async () => {
      await syncMarkdownBlock?.()
    })

    expect(api.mermaid.upsertMarkdownBlock).toHaveBeenCalledWith(6, exactMarkdown, true)
    expect(replaceMarkdown).toHaveBeenCalledWith(`${exactMarkdown}\nupdated`)
  })
})
