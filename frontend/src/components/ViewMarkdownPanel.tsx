import { createContext, memo, useCallback, useContext, useEffect, useMemo, useRef, type FocusEvent, type MouseEvent } from 'react'
import {
  Box,
  HStack,
  IconButton,
  Spinner,
  Text,
  Tooltip,
  VStack,
} from '@chakra-ui/react'
import { ExternalLinkIcon } from '@chakra-ui/icons'
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
import {
  extractTldMermaidViewId,
  getMermaidMarkdownSyncStatus,
  mermaidCodeEquals,
  upsertMermaidMarkdownBlock,
} from '../pkg/mermaid/markdown'

interface MermaidMarkdownContextValue {
  currentViewId: number | null
  currentMermaidCode: string
  canEdit: boolean
}

const MermaidMarkdownContext = createContext<MermaidMarkdownContextValue>({
  currentViewId: null,
  currentMermaidCode: '',
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
  const { currentViewId, currentMermaidCode, canEdit } = useContext(MermaidMarkdownContext)
  const { setCode } = useCodeBlockEditorContext()
  const blockViewId = extractTldMermaidViewId(code)
  const isCurrentView = currentViewId !== null && blockViewId === currentViewId
  const isSynced = isCurrentView && currentMermaidCode ? mermaidCodeEquals(code, currentMermaidCode) : false
  const status = !blockViewId
    ? 'unlinked'
    : isCurrentView
      ? isSynced ? 'synced' : 'stale'
      : 'other'
  const statusLabel = status === 'synced'
    ? 'Synced'
    : status === 'stale'
      ? 'View changed'
      : status === 'other'
        ? `View #${blockViewId}`
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

interface Props {
  isOpen: boolean
  onClose: () => void
  viewName?: string | null
  markdown: ViewMarkdownDocument | null
  content: string
  syncToken: number
  viewId?: number | null
  mermaidIntegrationEnabled?: boolean
  currentMermaidCode?: string
  canEdit?: boolean
  isLoading?: boolean
  isSaving?: boolean
  isDirty?: boolean
  onChange: (markdown: string) => void
  onSave: (markdown: string) => Promise<void> | void
  onSaveAs?: (markdown: string) => Promise<void> | void
  onOpenInEditor?: () => void
  onReload?: () => Promise<void> | void
}

function ViewMarkdownPanel({
  isOpen,
  onClose,
  markdown,
  content,
  syncToken,
  viewId = null,
  mermaidIntegrationEnabled = false,
  currentMermaidCode = '',
  canEdit = true,
  isLoading = false,
  isSaving = false,
  isDirty = false,
  onChange,
  onSave,
  onSaveAs,
  onOpenInEditor,
  onReload,
}: Props) {
  const editorRef = useRef<MDXEditorMethods>(null)
  const latestContentRef = useRef(content)
  const lastSyncTokenRef = useRef(syncToken)
  latestContentRef.current = content

  const mermaidSyncStatus = useMemo(() => {
    if (!mermaidIntegrationEnabled || !viewId || !currentMermaidCode) return null
    return getMermaidMarkdownSyncStatus(content, viewId, currentMermaidCode)
  }, [content, currentMermaidCode, mermaidIntegrationEnabled, viewId])

  const mermaidContextValue = useMemo(() => ({
    currentViewId: viewId,
    currentMermaidCode,
    canEdit,
  }), [canEdit, currentMermaidCode, viewId])

  const handleSyncMermaidBlock = useCallback(() => {
    if (!viewId || !currentMermaidCode) return
    const currentMarkdown = editorRef.current?.getMarkdown() ?? latestContentRef.current
    const nextMarkdown = upsertMermaidMarkdownBlock(currentMarkdown, viewId, currentMermaidCode)
    latestContentRef.current = nextMarkdown
    editorRef.current?.setMarkdown(nextMarkdown)
    onChange(nextMarkdown)
  }, [currentMermaidCode, onChange, viewId])

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
            {canEdit && (
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
                  label={mermaidSyncStatus === 'synced' ? 'Mermaid block is synced' : mermaidSyncStatus === 'stale' ? 'Update Mermaid block from view' : 'Insert Mermaid block from view'}
                  hasArrow
                  openDelay={200}
                >
                  <Box as="span">
                    <IconButton
                      aria-label="Sync Mermaid block"
                      size="xs"
                      variant="ghost"
                      className={`tld-markdown-toolbar-action tld-markdown-toolbar-action-mermaid tld-markdown-toolbar-action-mermaid-${mermaidSyncStatus ?? 'missing'}`}
                      icon={<ReloadIcon />}
                      onClick={handleSyncMermaidBlock}
                      isDisabled={!canEdit || isLoading || !markdown || !viewId || !currentMermaidCode || mermaidSyncStatus === 'synced'}
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
                    onClick={() => { void onSave(editorRef.current?.getMarkdown() ?? latestContentRef.current) }}
                    isLoading={isSaving}
                    isDisabled={!canEdit || isLoading || !markdown || !isDirty}
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
                      onClick={() => { void onSaveAs(editorRef.current?.getMarkdown() ?? latestContentRef.current) }}
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
  }, [canEdit, currentMermaidCode, handleSyncMermaidBlock, isDirty, isLoading, isSaving, markdown, mermaidIntegrationEnabled, mermaidSyncStatus, onClose, onOpenInEditor, onReload, onSave, onSaveAs, viewId])

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
        flex="1 1 auto"
        minH={0}
        overflow="hidden"
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
        {isLoading ? (
          <VStack justify="center" align="center" spacing={3} h="full" color="whiteAlpha.700">
            <Spinner size="md" color="blue.300" />
            <Text fontSize="sm">Loading markdown…</Text>
          </VStack>
        ) : markdown ? (
          <MermaidMarkdownContext.Provider value={mermaidContextValue}>
            <MDXEditor
              ref={editorRef}
              markdown={content}
              readOnly={!canEdit}
              spellCheck
              className="tld-markdown-editor"
              contentEditableClassName="tld-markdown-editor__content"
              placeholder="Start writing notes for this view…"
              plugins={plugins}
              onChange={(nextMarkdown) => onChange(nextMarkdown)}
            />
          </MermaidMarkdownContext.Provider>
        ) : (
          <VStack justify="center" align="center" spacing={3} h="full" color="whiteAlpha.700" px={6} textAlign="center">
            <Text fontSize="sm" fontWeight="semibold">No markdown document linked</Text>
            <Text fontSize="xs">
              Create a managed file from the toolbar, or link an existing markdown file from the view details panel.
            </Text>
          </VStack>
        )}
      </Box>
    </Box>
  )
}

export default memo(ViewMarkdownPanel)
