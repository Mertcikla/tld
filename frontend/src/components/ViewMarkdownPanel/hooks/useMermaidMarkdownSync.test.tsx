import React from 'react'
import { act, create } from 'react-test-renderer'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import type { MermaidMarkdownBlock } from '../../../api/client'
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

function mermaidBlock(overrides: Partial<MermaidMarkdownBlock> = {}): MermaidMarkdownBlock {
  return {
    index: 0,
    start: 0,
    end: 54,
    codeStart: 12,
    codeEnd: 43,
    lineStart: 1,
    lineEnd: 4,
    fence: '```',
    code: 'flowchart TD\n%% tld/v1 view=6\n  A --> B',
    viewId: 6,
    hasTldMetadata: true,
    preview: 'flowchart TD',
    syncStatusValue: 'stale',
    ...overrides,
  } as MermaidMarkdownBlock
}

async function flushPromises() {
  await Promise.resolve()
  await Promise.resolve()
}

describe('useMermaidMarkdownSync', () => {
  beforeEach(() => {
    vi.useFakeTimers()
    vi.mocked(api.mermaid.inspectMarkdown).mockReset()
    vi.mocked(api.mermaid.upsertMarkdownBlock).mockReset()
  })

  afterEach(() => {
    vi.clearAllTimers()
    vi.useRealTimers()
  })

  it('does not inspect markdown while Mermaid integration is disabled', async () => {
    function Harness() {
      useMermaidMarkdownSync({
        canEditDocument: true,
        content: '# Note',
        enabled: false,
        getMarkdown: () => '# Note',
        isOpen: true,
        replaceMarkdown: vi.fn(),
        viewId: 6,
      })
      return null
    }

    await act(async () => {
      create(<Harness />)
      await flushPromises()
    })
    await act(async () => {
      vi.advanceTimersByTime(300)
      await flushPromises()
    })

    expect(api.mermaid.inspectMarkdown).not.toHaveBeenCalled()
  })

  it('derives current and other TLD Mermaid blocks from inspection results', async () => {
    const currentBlock = mermaidBlock({ viewId: 6, syncStatusValue: 'stale' })
    const otherBlock = mermaidBlock({
      index: 1,
      start: 70,
      end: 122,
      code: 'flowchart TD\n%% tld/v1 view=9\n  X --> Y',
      viewId: 9,
      syncStatusValue: 'other',
    })
    vi.mocked(api.mermaid.inspectMarkdown).mockResolvedValue({
      blocks: [otherBlock, currentBlock],
      syncStatus: 'stale',
      warnings: [],
    })
    let hookResult: ReturnType<typeof useMermaidMarkdownSync> | undefined

    function Harness() {
      hookResult = useMermaidMarkdownSync({
        canEditDocument: true,
        content: '# Note',
        enabled: true,
        getMarkdown: () => '# Note',
        isOpen: true,
        replaceMarkdown: vi.fn(),
        viewId: 6,
      })
      return null
    }

    await act(async () => {
      create(<Harness />)
      await flushPromises()
    })
    await act(async () => {
      vi.advanceTimersByTime(250)
      await flushPromises()
    })

    expect(api.mermaid.inspectMarkdown).toHaveBeenCalledWith('# Note', 6)
    expect(hookResult?.mermaidSyncStatus).toBe('stale')
    expect(hookResult?.currentMermaidBlock).toBe(currentBlock)
    expect(hookResult?.otherViewMermaidBlocks).toEqual([otherBlock])
    expect(hookResult?.mermaidContextValue.blockStatusByCode.get(currentBlock.code)).toBe('stale')
    expect(hookResult?.mermaidContextValue.blockStatusByCode.get(otherBlock.code)).toBe('other')
  })

  it('upserts the current markdown without formatting it first', async () => {
    const exactMarkdown = '# Note\n\n\n```mermaid\nflowchart TD\n  A --> B\n```\n'
    const updatedMarkdown = `${exactMarkdown}\nupdated`
    const syncedBlock = mermaidBlock({ syncStatusValue: 'synced' })
    const replaceMarkdown = vi.fn()
    const getMarkdown = vi.fn(() => exactMarkdown)
    vi.mocked(api.mermaid.upsertMarkdownBlock).mockResolvedValue({
      markdown: updatedMarkdown,
      previousStatus: 'stale',
      warnings: [],
    })
    vi.mocked(api.mermaid.inspectMarkdown).mockResolvedValue({
      blocks: [syncedBlock],
      syncStatus: 'synced',
      warnings: [],
    })
    let syncMarkdownBlock: (() => Promise<void> | void) | undefined
    let hookResult: ReturnType<typeof useMermaidMarkdownSync> | undefined

    function Harness() {
      hookResult = useMermaidMarkdownSync({
        canEditDocument: true,
        content: exactMarkdown,
        enabled: true,
        getMarkdown,
        isOpen: true,
        replaceMarkdown,
        viewId: 6,
      })
      syncMarkdownBlock = hookResult.handleSyncMermaidBlock
      return null
    }

    create(<Harness />)
    await act(async () => {
      await syncMarkdownBlock?.()
    })

    expect(api.mermaid.upsertMarkdownBlock).toHaveBeenCalledWith(6, exactMarkdown, true)
    expect(replaceMarkdown).toHaveBeenCalledWith(updatedMarkdown)
    expect(api.mermaid.inspectMarkdown).toHaveBeenCalledWith(updatedMarkdown, 6)
    expect(hookResult?.mermaidSyncStatus).toBe('synced')
    expect(hookResult?.currentMermaidBlock).toBe(syncedBlock)
  })
})
