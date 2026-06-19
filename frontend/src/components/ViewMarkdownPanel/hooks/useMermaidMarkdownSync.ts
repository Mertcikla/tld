import { useCallback, useEffect, useMemo, useState } from 'react'
import { api, type MermaidMarkdownBlock, type MermaidMarkdownSyncStatus } from '../../../api/client'

interface UseMermaidMarkdownSyncParams {
  canEditDocument: boolean
  content: string
  enabled: boolean
  getMarkdown: () => string
  isOpen: boolean
  replaceMarkdown: (markdown: string) => void
  viewId: number | null
  viewName?: string | null
  viewNameById?: Map<number, string>
  onNavigateToView?: (viewId: number) => void
  onImportMermaidBlock?: (source: string) => Promise<void> | void
}

interface MermaidMarkdownInsertRange {
  from: number
  to: number
}

export interface MermaidMarkdownSyncOptions {
  insertRange?: MermaidMarkdownInsertRange
}

function clampPosition(position: number, markdownLength: number) {
  return Math.max(0, Math.min(position, markdownLength))
}

function insertMarkdownBlock(markdown: string, block: string, range: MermaidMarkdownInsertRange) {
  const from = clampPosition(Math.min(range.from, range.to), markdown.length)
  const to = clampPosition(Math.max(range.from, range.to), markdown.length)
  const before = markdown.slice(0, from)
  const after = markdown.slice(to)
  const normalizedBlock = block.trim()
  const beforeSeparator = before.length === 0 || before.endsWith('\n\n') ? '' : before.endsWith('\n') ? '\n' : '\n\n'
  const afterSeparator = after.length === 0 || after.startsWith('\n\n') ? '' : after.startsWith('\n') ? '\n' : '\n\n'

  return `${before}${beforeSeparator}${normalizedBlock}${afterSeparator}${after}`
}

export function useMermaidMarkdownSync({
  canEditDocument,
  content,
  enabled,
  getMarkdown,
  isOpen,
  replaceMarkdown,
  viewId,
  viewName,
  viewNameById,
  onNavigateToView,
  onImportMermaidBlock,
}: UseMermaidMarkdownSyncParams) {
  const [mermaidSyncStatus, setMermaidSyncStatus] = useState<MermaidMarkdownSyncStatus | null>(null)
  const [mermaidBlocks, setMermaidBlocks] = useState<MermaidMarkdownBlock[]>([])
  const [mermaidBlockStatusByCode, setMermaidBlockStatusByCode] = useState<Map<string, MermaidMarkdownSyncStatus>>(() => new Map())

  const applyInspectionResult = useCallback((result: { blocks: MermaidMarkdownBlock[]; syncStatus: MermaidMarkdownSyncStatus }) => {
    setMermaidSyncStatus(result.syncStatus)
    setMermaidBlocks(result.blocks)
    setMermaidBlockStatusByCode(new Map(result.blocks.map((block) => [block.code, block.syncStatusValue])))
  }, [])

  const resetInspectionResult = useCallback(() => {
    setMermaidSyncStatus(null)
    setMermaidBlocks([])
    setMermaidBlockStatusByCode(new Map())
  }, [])

  useEffect(() => {
    if (!isOpen || !enabled || !viewId) {
      resetInspectionResult()
      return undefined
    }

    let canceled = false
    const timer = globalThis.setTimeout(() => {
      void api.mermaid.inspectMarkdown(content, viewId).then((result) => {
        if (canceled) return
        applyInspectionResult(result)
      }).catch(() => {
        if (canceled) return
        resetInspectionResult()
      })
    }, 250)

    return () => {
      canceled = true
      globalThis.clearTimeout(timer)
    }
  }, [applyInspectionResult, content, enabled, isOpen, resetInspectionResult, viewId])

  const currentMermaidBlock = useMemo(() => {
    if (!viewId) return null
    return mermaidBlocks.find((block) => block.viewId === viewId) ?? null
  }, [mermaidBlocks, viewId])

  const otherViewMermaidBlocks = useMemo(() => {
    if (!viewId) return mermaidBlocks.filter((block) => block.viewId != null)
    return mermaidBlocks.filter((block) => block.viewId != null && block.viewId !== viewId)
  }, [mermaidBlocks, viewId])

  const handleSyncMermaidBlock = useCallback(async (options: MermaidMarkdownSyncOptions = {}) => {
    if (!enabled || !viewId) return
    const currentMarkdown = getMarkdown()
    let nextMarkdown: string

    if (options.insertRange) {
      const result = await api.mermaid.exportView(viewId, { includeTldMetadata: true, markdownBlock: true })
      nextMarkdown = insertMarkdownBlock(currentMarkdown, result.markdown, options.insertRange)
    } else {
      const result = await api.mermaid.upsertMarkdownBlock(viewId, currentMarkdown, true)
      nextMarkdown = result.markdown
    }

    replaceMarkdown(nextMarkdown)
    const inspection = await api.mermaid.inspectMarkdown(nextMarkdown, viewId)
    applyInspectionResult(inspection)
  }, [applyInspectionResult, enabled, getMarkdown, replaceMarkdown, viewId])

  const mermaidContextValue = useMemo(() => ({
    blockStatusByCode: mermaidBlockStatusByCode,
    canEdit: canEditDocument,
    currentViewId: viewId,
    currentViewName: viewName,
    viewNameById,
    onNavigateToView,
    onSyncCurrentViewMermaidBlock: handleSyncMermaidBlock,
    onImportMermaidBlock,
  }), [canEditDocument, handleSyncMermaidBlock, mermaidBlockStatusByCode, onImportMermaidBlock, onNavigateToView, viewId, viewName, viewNameById])

  return {
    currentMermaidBlock,
    mermaidBlocks,
    mermaidContextValue,
    mermaidSyncStatus,
    otherViewMermaidBlocks,
    handleSyncMermaidBlock,
  }
}
