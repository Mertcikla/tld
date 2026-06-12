import { createContext, memo, useCallback, useContext, useEffect, useMemo, useRef, useState, type FocusEvent, type MouseEvent, type PointerEvent as ReactPointerEvent } from 'react'
import {
  Badge,
  Box,
  Button,
  HStack,
  IconButton,
  Input,
  Menu,
  MenuButton,
  MenuItem,
  MenuList,
  Spinner,
  Text,
  Tooltip,
  VStack,
} from '@chakra-ui/react'
import { ChevronDownIcon, DragHandleIcon, ExternalLinkIcon, ViewIcon, ViewOffIcon } from '@chakra-ui/icons'
import {
  BlockTypeSelect,
  BoldItalicUnderlineToggles,
  codeBlockPlugin,
  codeMirrorPlugin,
  CreateLink,
  headingsPlugin,
  linkDialogPlugin,
  listsPlugin,
  ListsToggle,
  markdownShortcutPlugin,
  MDXEditor,
  type MDXEditorMethods,
  quotePlugin,
  thematicBreakPlugin,
  toolbarPlugin,
  type CodeBlockEditorDescriptor,
  type CodeBlockEditorProps,
  UndoRedo,
  useCodeBlockEditorContext,
} from '@mdxeditor/editor'
import '@mdxeditor/editor/style.css'
import { CloseIcon, ReloadIcon, SaveIcon } from './Icons'
import type { ViewMarkdownDocument } from '../types'
import { isWailsApp } from '../config/runtime'
import { api, type MermaidMarkdownSyncStatus } from '../api/client'

interface MermaidMarkdownContextValue {
  blockStatusByCode: Map<string, MermaidMarkdownSyncStatus>
  canEdit: boolean
}

const MermaidMarkdownContext = createContext<MermaidMarkdownContextValue>({
  blockStatusByCode: new Map(),
  canEdit: false,
})

const CODE_BLOCK_LANGUAGES = {
  mermaid: 'Mermaid',
  markdown: 'Markdown',
  ts: 'TypeScript',
  tsx: 'TypeScript (React)',
  js: 'JavaScript',
  jsx: 'JavaScript (React)',
  css: 'CSS',
  json: 'JSON',
  go: 'Go',
  shell: 'Shell',
}

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

function MermaidCollapsedCodeBlock({ code }: CodeBlockEditorProps) {
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

const MERMAID_CODE_BLOCK_DESCRIPTOR: CodeBlockEditorDescriptor = {
  priority: 10,
  match: (language) => (language ?? '').toLowerCase() === 'mermaid',
  Editor: MermaidCollapsedCodeBlock,
}

function markdownSourceLabel(markdown: ViewMarkdownDocument | null) {
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

function markdownStatusLabel(markdown: ViewMarkdownDocument | null) {
  if (!markdown) return 'No notes'
  const source = markdownSourceLabel(markdown)
  if (!markdown.exists) return source
  if (!markdown.can_edit) return `${source} · read-only`
  if (markdown.source_kind === 'REPO' && markdown.git_state && markdown.git_state !== 'unknown') {
    return `${source} · ${markdown.git_state.replace(/_/g, ' ')}`
  }
  return source
}

function defaultRepoMarkdownPath(viewName?: string | null) {
  const slug = (viewName || 'view-notes')
    .trim()
    .toLowerCase()
    .replace(/[^a-z0-9]+/g, '-')
    .replace(/^-+|-+$/g, '') || 'view-notes'
  return `docs/diagrams/${slug}.md`
}

const FLOATING_MENU_MARGIN = 8
const FLOATING_MENU_DEFAULT_Y = 56

interface FloatingMenuPosition {
  x: number | null
  y: number
}

interface ConcreteFloatingMenuPosition {
  x: number
  y: number
}

function sameFloatingMenuPosition(left: FloatingMenuPosition, right: FloatingMenuPosition) {
  return left.x === right.x && left.y === right.y
}

interface Props {
  isOpen: boolean
  onClose: () => void
  viewName?: string | null
  markdown: ViewMarkdownDocument | null
  content: string
  syncToken: number
  viewId?: number | null
  mermaidIntegrationEnabled?: boolean
  canEdit?: boolean
  isLoading?: boolean
  isSaving?: boolean
  isDirty?: boolean
  hasSaveConflict?: boolean
  onChange: (markdown: string) => void
  onSave: (markdown: string) => Promise<void> | void
  onForceSave?: (markdown: string) => Promise<void> | void
  onCreateMarkdown?: (targetKind: string, path?: string) => Promise<void> | void
  onAttachMarkdown?: (path: string) => Promise<void> | void
  onUnlinkMarkdown?: (options?: { deleteManagedFile: boolean }) => Promise<void> | void
  onPickMarkdownFile?: () => Promise<string | null>
  onSaveAs?: (markdown: string) => Promise<void> | void
  onOpenInEditor?: () => void
  onReload?: () => Promise<void> | void
}

function ViewMarkdownPanel({
  isOpen,
  onClose,
  viewName,
  markdown,
  content,
  syncToken,
  viewId = null,
  mermaidIntegrationEnabled = false,
  canEdit = true,
  isLoading = false,
  isSaving = false,
  isDirty = false,
  hasSaveConflict = false,
  onChange,
  onSave,
  onForceSave,
  onCreateMarkdown,
  onAttachMarkdown,
  onUnlinkMarkdown,
  onPickMarkdownFile,
  onSaveAs,
  onOpenInEditor,
  onReload,
}: Props) {
  const editorRef = useRef<MDXEditorMethods>(null)
  const panelBodyRef = useRef<HTMLDivElement | null>(null)
  const floatingMenuRef = useRef<HTMLDivElement | null>(null)
  const floatingMenuDragRef = useRef<{ pointerId: number; offsetX: number; offsetY: number } | null>(null)
  const latestContentRef = useRef(content)
  const lastSyncTokenRef = useRef(syncToken)
  const [mermaidSyncStatus, setMermaidSyncStatus] = useState<MermaidMarkdownSyncStatus | null>(null)
  const [mermaidBlockStatusByCode, setMermaidBlockStatusByCode] = useState<Map<string, MermaidMarkdownSyncStatus>>(() => new Map())
  const [floatingMenuPosition, setFloatingMenuPosition] = useState<FloatingMenuPosition>(() => ({ x: null, y: FLOATING_MENU_DEFAULT_Y }))
  const [isFloatingMenuHidden, setIsFloatingMenuHidden] = useState(false)
  const [isFloatingMenuDragging, setIsFloatingMenuDragging] = useState(false)
  latestContentRef.current = content
  const suggestedRepoPath = useMemo(() => defaultRepoMarkdownPath(viewName), [viewName])
  const [repoPath, setRepoPath] = useState(suggestedRepoPath)
  const [attachPath, setAttachPath] = useState('')
  const canEditDocument = canEdit && !!markdown?.can_edit
  const showNativeAttachPicker = isWailsApp && !!onPickMarkdownFile
  const showAttachPathInput = !showNativeAttachPicker

  useEffect(() => {
    setRepoPath(suggestedRepoPath)
  }, [suggestedRepoPath])

  const currentEditorMarkdown = useCallback(() => editorRef.current?.getMarkdown() ?? latestContentRef.current, [])

  const handleAttachPickedFile = useCallback(async () => {
    if (!onPickMarkdownFile || !onAttachMarkdown) return
    const path = await onPickMarkdownFile()
    if (!path) return
    setAttachPath(path)
    await onAttachMarkdown(path)
  }, [onAttachMarkdown, onPickMarkdownFile])

  const handleCopyPath = useCallback(async () => {
    if (!markdown?.path || typeof navigator === 'undefined' || !navigator.clipboard) return
    await navigator.clipboard.writeText(markdown.path)
  }, [markdown?.path])

  const clampFloatingMenuPosition = useCallback((position: ConcreteFloatingMenuPosition): ConcreteFloatingMenuPosition => {
    const panelRect = panelBodyRef.current?.getBoundingClientRect()
    if (!panelRect) return position

    const menuRect = floatingMenuRef.current?.getBoundingClientRect()
    const maxMenuWidth = Math.max(0, panelRect.width - FLOATING_MENU_MARGIN * 2)
    const maxMenuHeight = Math.max(0, panelRect.height - FLOATING_MENU_MARGIN * 2)
    const menuWidth = Math.min(menuRect?.width ?? 280, maxMenuWidth)
    const menuHeight = Math.min(menuRect?.height ?? 36, maxMenuHeight)
    const maxX = Math.max(FLOATING_MENU_MARGIN, panelRect.width - menuWidth - FLOATING_MENU_MARGIN)
    const maxY = Math.max(FLOATING_MENU_MARGIN, panelRect.height - menuHeight - FLOATING_MENU_MARGIN)

    return {
      x: Math.min(Math.max(position.x, FLOATING_MENU_MARGIN), maxX),
      y: Math.min(Math.max(position.y, FLOATING_MENU_MARGIN), maxY),
    }
  }, [])

  const handleFloatingMenuDragStart = useCallback((event: ReactPointerEvent<HTMLButtonElement>) => {
    if (event.button !== 0) return

    const panelRect = panelBodyRef.current?.getBoundingClientRect()
    const menuRect = floatingMenuRef.current?.getBoundingClientRect()
    if (!panelRect || !menuRect) return

    floatingMenuDragRef.current = {
      pointerId: event.pointerId,
      offsetX: event.clientX - menuRect.left,
      offsetY: event.clientY - menuRect.top,
    }
    setFloatingMenuPosition(clampFloatingMenuPosition({
      x: menuRect.left - panelRect.left,
      y: menuRect.top - panelRect.top,
    }))
    setIsFloatingMenuDragging(true)
    event.preventDefault()
    event.stopPropagation()
  }, [clampFloatingMenuPosition])

  useEffect(() => {
    if (!isFloatingMenuDragging) return

    const handlePointerMove = (event: PointerEvent) => {
      const dragState = floatingMenuDragRef.current
      const panelRect = panelBodyRef.current?.getBoundingClientRect()
      if (!dragState || dragState.pointerId !== event.pointerId || !panelRect) return

      event.preventDefault()
      setFloatingMenuPosition(clampFloatingMenuPosition({
        x: event.clientX - panelRect.left - dragState.offsetX,
        y: event.clientY - panelRect.top - dragState.offsetY,
      }))
    }

    const stopDragging = (event: PointerEvent) => {
      const dragState = floatingMenuDragRef.current
      if (dragState && dragState.pointerId !== event.pointerId) return
      floatingMenuDragRef.current = null
      setIsFloatingMenuDragging(false)
    }

    window.addEventListener('pointermove', handlePointerMove)
    window.addEventListener('pointerup', stopDragging)
    window.addEventListener('pointercancel', stopDragging)

    return () => {
      window.removeEventListener('pointermove', handlePointerMove)
      window.removeEventListener('pointerup', stopDragging)
      window.removeEventListener('pointercancel', stopDragging)
    }
  }, [clampFloatingMenuPosition, isFloatingMenuDragging])

  useEffect(() => {
    const clampCurrentPosition = () => {
      setFloatingMenuPosition((current) => {
        if (current.x === null) return current
        const next = clampFloatingMenuPosition({ x: current.x, y: current.y })
        return sameFloatingMenuPosition(current, next) ? current : next
      })
    }

    clampCurrentPosition()
    const observer = typeof ResizeObserver === 'undefined'
      ? null
      : new ResizeObserver(clampCurrentPosition)
    if (panelBodyRef.current) observer?.observe(panelBodyRef.current)
    if (floatingMenuRef.current) observer?.observe(floatingMenuRef.current)
    window.addEventListener('resize', clampCurrentPosition)
    return () => {
      observer?.disconnect()
      window.removeEventListener('resize', clampCurrentPosition)
    }
  }, [clampFloatingMenuPosition, isFloatingMenuHidden])

  useEffect(() => {
    if (!isOpen || !mermaidIntegrationEnabled || !viewId) {
      setMermaidSyncStatus(null)
      setMermaidBlockStatusByCode(new Map())
      return undefined
    }

    let canceled = false
    const timer = window.setTimeout(() => {
      void api.mermaid.inspectMarkdown(content, viewId).then((result) => {
        if (canceled) return
        setMermaidSyncStatus(result.syncStatus)
        setMermaidBlockStatusByCode(new Map(result.blocks.map((block) => [block.code, block.syncStatusValue])))
      }).catch(() => {
        if (canceled) return
        setMermaidSyncStatus(null)
        setMermaidBlockStatusByCode(new Map())
      })
    }, 250)

    return () => {
      canceled = true
      window.clearTimeout(timer)
    }
  }, [content, isOpen, mermaidIntegrationEnabled, viewId])

  const mermaidContextValue = useMemo(() => ({
    blockStatusByCode: mermaidBlockStatusByCode,
    canEdit: canEditDocument,
  }), [canEditDocument, mermaidBlockStatusByCode])

  const handleSyncMermaidBlock = useCallback(async () => {
    if (!viewId) return
    const currentMarkdown = editorRef.current?.getMarkdown() ?? latestContentRef.current
    const result = await api.mermaid.upsertMarkdownBlock(viewId, currentMarkdown, true)
    const nextMarkdown = result.markdown
    latestContentRef.current = nextMarkdown
    editorRef.current?.setMarkdown(nextMarkdown)
    setMermaidSyncStatus('synced')
    onChange(nextMarkdown)
  }, [onChange, viewId])

  useEffect(() => {
    if (!isOpen) return
    if (lastSyncTokenRef.current === syncToken) return
    lastSyncTokenRef.current = syncToken
    editorRef.current?.setMarkdown(content)
  }, [content, isOpen, syncToken])

  const plugins = useMemo(() => {
    const base = [
      headingsPlugin(),
      listsPlugin(),
      quotePlugin(),
      thematicBreakPlugin(),
      codeBlockPlugin({
        defaultCodeBlockLanguage: 'mermaid',
        codeBlockEditorDescriptors: mermaidIntegrationEnabled ? [MERMAID_CODE_BLOCK_DESCRIPTOR] : [],
      }),
      codeMirrorPlugin({
        codeBlockLanguages: CODE_BLOCK_LANGUAGES,
        autoLoadLanguageSupport: false,
      }),
      markdownShortcutPlugin(),
      linkDialogPlugin(),
      toolbarPlugin({
        toolbarClassName: 'tld-markdown-toolbar',
        toolbarContents: () => (
          <>
            {canEditDocument && (
              <Box className="tld-markdown-toolbar-formatting" display="flex" flex="0 1 auto" minW={0} overflow="hidden" gap={1.5} alignItems="center">
                <UndoRedo />
                <BoldItalicUnderlineToggles />
                <BlockTypeSelect />
                <ListsToggle />
                <CreateLink />
              </Box>
            )}
            <Box className="tld-markdown-toolbar-spacer" />
            <HStack className="tld-markdown-toolbar-actions" spacing={1.5}>
              {mermaidIntegrationEnabled && (
                <Tooltip
                  label={mermaidSyncStatus === 'synced' ? 'Diagram block is synced' : mermaidSyncStatus === 'stale' ? 'Update diagram block from view' : 'Insert diagram block from view'}
                  hasArrow
                  openDelay={200}
                >
                  <Box as="span">
                    <IconButton
                      aria-label="Insert/update diagram"
                      size="xs"
                      variant="ghost"
                      className={`tld-markdown-toolbar-action tld-markdown-toolbar-action-mermaid tld-markdown-toolbar-action-mermaid-${mermaidSyncStatus ?? 'missing'}`}
                      icon={<ReloadIcon />}
                      onClick={() => { void handleSyncMermaidBlock() }}
                      isDisabled={!canEditDocument || isLoading || !markdown || !viewId || mermaidSyncStatus === 'synced'}
                    />
                  </Box>
                </Tooltip>
              )}
              <Tooltip label="Reload" hasArrow openDelay={200}>
                <Box as="span">
                  <IconButton
                    aria-label="Reload"
                    size="xs"
                    variant="ghost"
                    className="tld-markdown-toolbar-action"
                    icon={<ReloadIcon />}
                    onClick={() => { void onReload?.() }}
                    isDisabled={isLoading || !markdown}
                  />
                </Box>
              </Tooltip>
              <Tooltip label="Save" hasArrow openDelay={200}>
                <Box as="span">
                  <IconButton
                    aria-label="Save"
                    size="xs"
                    className="tld-markdown-toolbar-action tld-markdown-toolbar-action-save"
                    icon={<SaveIcon />}
                    onClick={() => { void onSave(currentEditorMarkdown()) }}
                    isLoading={isSaving}
                    isDisabled={!canEditDocument || isLoading || !markdown || !isDirty}
                  />
                </Box>
              </Tooltip>
              {isWailsApp && onSaveAs && (
                <Tooltip label="Save As" hasArrow openDelay={200}>
                  <Box as="span">
                    <IconButton
                      aria-label="Save As"
                      size="xs"
                      className="tld-markdown-toolbar-action"
                      icon={<SaveIcon />}
                      onClick={() => { void onSaveAs(currentEditorMarkdown()) }}
                      isDisabled={isLoading || !markdown}
                    />
                  </Box>
                </Tooltip>
              )}
              {onOpenInEditor && (
                <Tooltip label="Open in Editor" hasArrow openDelay={200}>
                  <Box as="span">
                    <IconButton
                      aria-label="Open in Editor"
                      size="xs"
                      variant="ghost"
                      className="tld-markdown-toolbar-action"
                      icon={<ExternalLinkIcon />}
                      onClick={onOpenInEditor}
                      isDisabled={isLoading || !markdown}
                    />
                  </Box>
                </Tooltip>
              )}
              <Tooltip label="Close" hasArrow openDelay={200}>
                <Box as="span">
                  <IconButton
                    aria-label="Close"
                    size="xs"
                    variant="ghost"
                    className="tld-markdown-toolbar-action"
                    icon={<CloseIcon />}
                    onClick={onClose}
                  />
                </Box>
              </Tooltip>
            </HStack>
          </>
        ),
      }),
    ]

    return base
  }, [canEditDocument, currentEditorMarkdown, handleSyncMermaidBlock, isDirty, isLoading, isSaving, markdown, mermaidIntegrationEnabled, mermaidSyncStatus, onClose, onOpenInEditor, onReload, onSave, onSaveAs, viewId])

  if (!isOpen) return null

  return (
    <Box
      data-testid="view-markdown-panel"
      h="full"
      minH={0}
      minW={0}
      display="flex"
      flexDir="column"
      bg="var(--bg-panel)"
      bgImage="var(--grad-panel)"
    >
      <Box
        ref={panelBodyRef}
        flex="1 1 auto"
        minH={0}
        overflow="hidden"
        position="relative"
        bg="var(--bg-canvas)"
        color="gray.100"
        sx={{
          '.tld-markdown-editor': {
            '--basePageBg': 'var(--bg-canvas)',
            '--baseBase': '#0f172a',
            '--baseBgSubtle': '#111c31',
            '--baseBg': '#152238',
            '--baseBgHover': '#1a2b46',
            '--baseBgActive': '#203456',
            '--baseLine': 'rgba(148, 163, 184, 0.18)',
            '--baseBorder': 'rgba(148, 163, 184, 0.22)',
            '--baseBorderHover': 'rgba(148, 163, 184, 0.28)',
            '--baseSolid': '#22385a',
            '--baseSolidHover': '#2a4772',
            '--baseText': '#dbe6f5',
            '--baseTextContrast': '#f8fbff',
            '--accentBase': '#10233d',
            '--accentBgSubtle': '#123053',
            '--accentBg': '#153c68',
            '--accentBgHover': '#1a4a82',
            '--accentBgActive': '#205798',
            '--accentLine': 'rgba(96, 165, 250, 0.4)',
            '--accentBorder': 'rgba(96, 165, 250, 0.45)',
            '--accentBorderHover': 'rgba(96, 165, 250, 0.6)',
            '--accentSolid': '#3b82f6',
            '--accentSolidHover': '#60a5fa',
            '--accentText': '#93c5fd',
            '--accentTextContrast': '#eff6ff',
            display: 'flex',
            flexDirection: 'column',
            width: '100%',
            height: '100%',
            minHeight: 0,
            background: 'var(--bg-canvas)',
            color: '#dbe6f5',
          },
          '.tld-markdown-toolbar': {
            display: 'flex',
            flexWrap: 'nowrap',
            alignItems: 'center',
            borderBottom: '1px solid rgba(148, 163, 184, 0.18)',
            background: 'var(--basePageBg)',
            color: '#dbe6f5',
            paddingInline: '0.5rem',
            minHeight: '48px',
            flexShrink: 0,
          },
          '.tld-markdown-toolbar-formatting': {
            display: 'flex',
            flexWrap: 'nowrap',
            alignItems: 'center',
            flex: '0 1 auto',
            minWidth: 0,
            overflow: 'hidden',
          },
          '.tld-markdown-toolbar-spacer': {
            flex: '1 1 auto',
            minWidth: '0.5rem',
          },
          '.tld-markdown-toolbar-actions': {
            marginLeft: 'auto',
            paddingLeft: '0.5rem',
            borderLeft: '1px solid rgba(148, 163, 184, 0.12)',
            flexShrink: 0,
          },
          '.tld-markdown-toolbar-action': {
            height: '30px',
            minWidth: '30px',
            width: '30px',
            paddingInline: '0',
            borderRadius: '0.55rem',
            fontWeight: 600,
            background: 'transparent',
            color: '#dbe6f5',
          },
          '.tld-markdown-toolbar-action svg': {
            width: '15px',
            height: '15px',
          },
          '.tld-markdown-toolbar-action:hover': {
            background: 'rgba(96, 165, 250, 0.12)',
          },
          '.tld-markdown-toolbar-action-save': {
            background: 'rgba(59, 130, 246, 0.22)',
            color: '#eff6ff',
          },
          '.tld-markdown-toolbar-action-save:hover': {
            background: 'rgba(96, 165, 250, 0.3)',
          },
          '.tld-markdown-toolbar-action[data-disabled], .tld-markdown-toolbar-action:disabled': {
            opacity: 0.4,
            background: 'transparent',
          },
          '.tld-markdown-toolbar button': {
            color: '#dbe6f5',
          },
          '.tld-markdown-toolbar button:hover': {
            background: 'rgba(96, 165, 250, 0.14)',
          },
          '.tld-markdown-toolbar [data-state="on"]': {
            background: 'rgba(59, 130, 246, 0.24)',
            color: '#eff6ff',
          },
          '.tld-markdown-toolbar [disabled]': {
            opacity: 0.35,
          },
          '.tld-markdown-editor .mdxeditor-root-contenteditable': {
            position: 'relative',
            display: 'flex',
            flexDirection: 'column',
            flex: '1 1 auto',
            minHeight: 0,
            background: 'var(--bg-canvas)',
          },
          '.tld-markdown-editor .mdxeditor-root-contenteditable > div': {
            display: 'flex',
            flexDirection: 'column',
            flex: '1 1 auto',
            minHeight: 0,
          },
          '.tld-markdown-editor__content': {
            flex: '1 1 auto',
            minHeight: 0,
            width: '100%',
            padding: '1.25rem 1.5rem 2rem',
            fontSize: '0.95rem',
            lineHeight: 1.75,
            color: '#dbe6f5',
            background: 'var(--bg-canvas)',
            outline: 'none',
            boxShadow: 'none',
            overflowY: 'auto',
          },
          '.tld-markdown-editor__content p': {
            color: '#dbe6f5',
            marginBottom: '1rem',
          },
          '.tld-markdown-editor__content h1': {
            color: '#f8fbff',
            fontSize: '1.75em',
            fontWeight: 700,
            marginTop: '1.5rem',
            marginBottom: '0.75rem',
            lineHeight: 1.25,
          },
          '.tld-markdown-editor__content h2': {
            color: '#f8fbff',
            fontSize: '1.4em',
            fontWeight: 700,
            marginTop: '1.4rem',
            marginBottom: '0.6rem',
            lineHeight: 1.3,
          },
          '.tld-markdown-editor__content h3': {
            color: '#f8fbff',
            fontSize: '1.2em',
            fontWeight: 600,
            marginTop: '1.3rem',
            marginBottom: '0.5rem',
            lineHeight: 1.35,
          },
          '.tld-markdown-editor__content h4': {
            color: '#f8fbff',
            fontSize: '1.1em',
            fontWeight: 600,
            marginTop: '1.2rem',
            marginBottom: '0.4rem',
          },
          '.tld-markdown-editor__content h5': {
            color: '#f8fbff',
            fontSize: '1.05em',
            fontWeight: 600,
            marginTop: '1.1rem',
            marginBottom: '0.4rem',
          },
          '.tld-markdown-editor__content h6': {
            color: '#f8fbff',
            fontSize: '1em',
            fontWeight: 600,
            marginTop: '1rem',
            marginBottom: '0.4rem',
          },
          '.tld-markdown-editor__content a': {
            color: '#93c5fd',
          },
          '.tld-markdown-editor__content blockquote': {
            borderLeft: '4px solid',
            borderLeftColor: 'rgba(96, 165, 250, 0.45)',
            color: '#cbd5e1',
            paddingLeft: '1rem',
            marginInline: '0',
            marginBottom: '1rem',
            fontStyle: 'italic',
          },
          '.tld-markdown-editor__content code': {
            background: 'rgba(30, 41, 59, 0.9)',
            color: '#bfdbfe',
            padding: '0.2rem 0.4rem',
            borderRadius: '0.25rem',
            fontSize: '0.875em',
          },
          '.tld-markdown-editor__content pre': {
            background: '#020817',
            color: '#dbe6f5',
            padding: '1rem',
            borderRadius: '0.5rem',
            overflowX: 'auto',
            marginBottom: '1rem',
          },
          '.tld-markdown-editor__content pre code': {
            background: 'transparent',
            padding: 0,
            borderRadius: 0,
            color: 'inherit',
            fontSize: 'inherit',
          },
          '.tld-mermaid-markdown-block': {
            margin: '0.75rem 0 1rem',
          },
          '.tld-mermaid-markdown-block__summary': {
            cursor: 'pointer',
            fontFamily: 'ui-monospace, SFMono-Regular, Menlo, Monaco, Consolas, "Liberation Mono", "Courier New", monospace',
            fontSize: '0.875em',
            color: '#94a3b8',
            whiteSpace: 'nowrap',
          },
          '.tld-mermaid-markdown-block__meta': {
            marginLeft: '0.75rem',
            color: '#64748b',
          },
          '.tld-mermaid-markdown-block--stale .tld-mermaid-markdown-block__meta': {
            color: '#facc15',
          },
          '.tld-mermaid-markdown-block--other .tld-mermaid-markdown-block__meta': {
            opacity: 0.72,
          },
          '.tld-mermaid-markdown-block__textarea': {
            width: '100%',
            minHeight: '8rem',
            resize: 'vertical',
            border: 0,
            outline: 'none',
            color: 'inherit',
            background: 'transparent',
            font: 'inherit',
            lineHeight: 'inherit',
          },
          '.tld-markdown-editor__content ul, .tld-markdown-editor__content ol': {
            paddingLeft: '1.5rem',
            marginBottom: '1rem',
          },
          '.tld-markdown-editor__content li': {
            marginBottom: '0.25rem',
          },
        }}
      >
        <Box
          ref={floatingMenuRef}
          data-testid="view-markdown-floating-menu"
          position="absolute"
          top={`${floatingMenuPosition.y}px`}
          left={floatingMenuPosition.x === null ? undefined : `${floatingMenuPosition.x}px`}
          right={floatingMenuPosition.x === null ? 3 : undefined}
          zIndex={30}
          pointerEvents="none"
          maxW="calc(100% - 16px)"
        >
          {isFloatingMenuHidden ? (
            <Tooltip label="Show notes menu" hasArrow openDelay={200}>
              <Button
                data-testid="view-markdown-menu-show"
                pointerEvents="auto"
                size="xs"
                leftIcon={<ViewIcon />}
                bg="rgba(2, 8, 23, 0.86)"
                color="whiteAlpha.900"
                border="1px solid"
                borderColor="whiteAlpha.200"
                boxShadow="0 12px 28px rgba(0,0,0,0.36)"
                backdropFilter="blur(18px)"
                _hover={{ bg: 'rgba(15, 23, 42, 0.94)' }}
                onClick={() => setIsFloatingMenuHidden(false)}
                onPointerDown={(event) => event.stopPropagation()}
              >
                Notes
              </Button>
            </Tooltip>
          ) : (
            <HStack
              data-testid="view-markdown-floating-menu-expanded"
              pointerEvents="auto"
              spacing={1}
              minW={0}
              maxW="100%"
              px={1.5}
              py={1}
              bg="rgba(2, 8, 23, 0.86)"
              border="1px solid"
              borderColor={isFloatingMenuDragging ? 'blue.300' : 'whiteAlpha.200'}
              rounded="lg"
              boxShadow="0 12px 28px rgba(0,0,0,0.36)"
              backdropFilter="blur(18px)"
              onPointerDown={(event) => event.stopPropagation()}
            >
              <Tooltip label="Move notes menu" hasArrow openDelay={200}>
                <IconButton
                  data-testid="view-markdown-menu-drag-handle"
                  aria-label="Move notes menu"
                  size="xs"
                  variant="ghost"
                  icon={<DragHandleIcon />}
                  color="whiteAlpha.700"
                  cursor={isFloatingMenuDragging ? 'grabbing' : 'grab'}
                  style={{ touchAction: 'none' }}
                  _hover={{ bg: 'whiteAlpha.100', color: 'whiteAlpha.900' }}
                  onPointerDown={handleFloatingMenuDragStart}
                />
              </Tooltip>
              <Box minW={0} maxW={{ base: '150px', md: '240px' }} px={1}>
                <HStack spacing={1.5} minW={0}>
                  <Text fontSize="xs" fontWeight="semibold" color="whiteAlpha.900" flexShrink={0}>Notes</Text>
                  <Badge data-testid="view-markdown-status" colorScheme={markdown?.exists === false ? 'red' : markdown?.can_edit === false ? 'yellow' : 'blue'} variant="subtle" maxW={{ base: '90px', md: '150px' }} overflow="hidden" textOverflow="ellipsis" whiteSpace="nowrap">
                    {markdownStatusLabel(markdown)}
                  </Badge>
                </HStack>
                {markdown?.path && (
                  <Text fontSize="10px" color="gray.500" isTruncated title={markdown.repo_relative_path || markdown.path}>
                    {markdown.repo_relative_path || markdown.path}
                  </Text>
                )}
              </Box>
              {hasSaveConflict && markdown && (
                <HStack spacing={1} flexShrink={0}>
                  <Button size="xs" variant="outline" onClick={() => { void onReload?.() }}>Reload</Button>
                  <Button data-testid="view-markdown-overwrite" size="xs" colorScheme="blue" onClick={() => { void onForceSave?.(currentEditorMarkdown()) }}>
                    Overwrite
                  </Button>
                </HStack>
              )}
              <Menu placement="bottom-end">
                <MenuButton as={Button} size="xs" variant="ghost" rightIcon={<ChevronDownIcon boxSize={2.5} />} isDisabled={isLoading}>
                  File
                </MenuButton>
                <MenuList bg="var(--bg-panel)" borderColor="whiteAlpha.200" fontSize="sm">
                  <MenuItem bg="transparent" onClick={() => { void onReload?.() }} isDisabled={!markdown || isLoading}>Reload</MenuItem>
                  <MenuItem bg="transparent" onClick={() => { void onSave(currentEditorMarkdown()) }} isDisabled={!canEditDocument || !markdown || !isDirty || isLoading}>Save</MenuItem>
                  <MenuItem bg="transparent" onClick={() => { void onForceSave?.(currentEditorMarkdown()) }} isDisabled={!canEditDocument || !markdown || isLoading}>Overwrite from pane</MenuItem>
                  <MenuItem bg="transparent" onClick={() => { void handleCopyPath() }} isDisabled={!markdown?.path}>Copy path</MenuItem>
                  {onOpenInEditor && (
                    <MenuItem bg="transparent" onClick={onOpenInEditor} isDisabled={!markdown}>Open in editor</MenuItem>
                  )}
                  <MenuItem data-testid="view-markdown-detach" bg="transparent" color="red.200" onClick={() => { void onUnlinkMarkdown?.({ deleteManagedFile: false }) }} isDisabled={!canEdit || !markdown || isLoading}>Detach from view</MenuItem>
                  <MenuItem bg="transparent" onClick={onClose}>Close</MenuItem>
                </MenuList>
              </Menu>
              <Tooltip label="Hide notes menu" hasArrow openDelay={200}>
                <IconButton
                  data-testid="view-markdown-menu-hide"
                  aria-label="Hide notes menu"
                  size="xs"
                  variant="ghost"
                  icon={<ViewOffIcon />}
                  color="whiteAlpha.700"
                  _hover={{ bg: 'whiteAlpha.100', color: 'whiteAlpha.900' }}
                  onClick={() => setIsFloatingMenuHidden(true)}
                />
              </Tooltip>
            </HStack>
          )}
        </Box>
        {isLoading ? (
          <VStack justify="center" align="center" spacing={3} h="full" color="whiteAlpha.700">
            <Spinner size="md" color="blue.300" />
            <Text fontSize="sm">Loading markdown…</Text>
          </VStack>
        ) : markdown && !markdown.exists ? (
          <VStack justify="center" align="center" spacing={3} h="full" color="whiteAlpha.800" px={6} textAlign="center">
            <Text fontSize="sm" fontWeight="semibold">Markdown file is missing</Text>
            <Text fontSize="xs" color="gray.500" maxW="320px" noOfLines={2}>{markdown.path}</Text>
            <HStack spacing={2} flexWrap="wrap" justify="center">
              <Button size="sm" variant="outline" onClick={() => { void onReload?.() }}>Reload</Button>
              <Button
                data-testid="view-markdown-create-replacement"
                size="sm"
                colorScheme="blue"
                onClick={() => { void onForceSave?.(viewName?.trim() ? `# ${viewName.trim()}\n\n` : '') }}
                isDisabled={!canEdit || !onForceSave}
              >
                Create replacement
              </Button>
              <Button
                data-testid="view-markdown-missing-detach"
                size="sm"
                variant="outline"
                colorScheme="red"
                onClick={() => { void onUnlinkMarkdown?.({ deleteManagedFile: false }) }}
                isDisabled={!canEdit || !onUnlinkMarkdown}
              >
                Detach
              </Button>
            </HStack>
            {canEdit && onAttachMarkdown && (
              <VStack spacing={2} w="full" maxW="360px" pt={2}>
                {showAttachPathInput && (
                  <Input
                    data-testid="view-markdown-attach-path"
                    size="sm"
                    value={attachPath}
                    onChange={(event) => setAttachPath(event.target.value)}
                    placeholder="docs/overview.md or /absolute/path/overview.md"
                  />
                )}
                <HStack spacing={2} w="full">
                  {showAttachPathInput && (
                    <Button data-testid="view-markdown-relink" size="sm" variant="outline" flex={1} onClick={() => { void onAttachMarkdown(attachPath.trim()) }} isDisabled={!attachPath.trim()}>
                      Relink
                    </Button>
                  )}
                  {showNativeAttachPicker && (
                    <Button data-testid="view-markdown-choose-file" size="sm" variant="outline" flex={1} onClick={() => { void handleAttachPickedFile() }}>
                      Choose file
                    </Button>
                  )}
                </HStack>
              </VStack>
            )}
          </VStack>
        ) : markdown ? (
          <MermaidMarkdownContext.Provider value={mermaidContextValue}>
            <MDXEditor
              ref={editorRef}
              markdown={content}
              readOnly={!canEditDocument}
              spellCheck
              className="tld-markdown-editor"
              contentEditableClassName="tld-markdown-editor__content"
              placeholder="Start writing notes for this view…"
              plugins={plugins}
              onChange={(nextMarkdown) => onChange(nextMarkdown)}
            />
          </MermaidMarkdownContext.Provider>
        ) : (
          <VStack justify="center" align="stretch" spacing={4} h="full" color="whiteAlpha.800" px={6} textAlign="left">
            <Box textAlign="center">
              <Text fontSize="sm" fontWeight="semibold">No notes file for this view</Text>
              <Text fontSize="xs" color="gray.500" mt={1}>Choose where this view's markdown notes should live.</Text>
            </Box>
            <Button
              data-testid="view-markdown-create-private"
              size="sm"
              colorScheme="blue"
              onClick={() => { void onCreateMarkdown?.('PRIVATE_WORKSPACE') }}
              isDisabled={!canEdit || !onCreateMarkdown}
            >
              Create private note
            </Button>
            <VStack spacing={2} align="stretch">
              <Input
                data-testid="view-markdown-repo-path"
                size="sm"
                value={repoPath}
                onChange={(event) => setRepoPath(event.target.value)}
                placeholder={suggestedRepoPath}
              />
              <Button
                data-testid="view-markdown-create-repo"
                size="sm"
                variant="outline"
                onClick={() => { void onCreateMarkdown?.('REPO', repoPath.trim() || suggestedRepoPath) }}
                isDisabled={!canEdit || !onCreateMarkdown}
              >
                Create repo note
              </Button>
            </VStack>
            {showAttachPathInput ? (
              <VStack spacing={2} align="stretch">
                <Input
                  data-testid="view-markdown-attach-path"
                  size="sm"
                  value={attachPath}
                  onChange={(event) => setAttachPath(event.target.value)}
                  placeholder="docs/overview.md or /absolute/path/overview.md"
                />
                <Button
                  data-testid="view-markdown-attach"
                  size="sm"
                  variant="outline"
                  onClick={() => { void onAttachMarkdown?.(attachPath.trim()) }}
                  isDisabled={!canEdit || !onAttachMarkdown || !attachPath.trim()}
                >
                  Attach existing file
                </Button>
              </VStack>
            ) : (
              <Button
                data-testid="view-markdown-choose-file"
                size="sm"
                variant="outline"
                onClick={() => { void handleAttachPickedFile() }}
                isDisabled={!canEdit || !onAttachMarkdown}
              >
                Attach existing file
              </Button>
            )}
          </VStack>
        )}
      </Box>
    </Box>
  )
}

export default memo(ViewMarkdownPanel)
