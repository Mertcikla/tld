import { useCallback, useImperativeHandle, useRef, useState, type MutableRefObject } from 'react'
import { Box, Button, ButtonGroup, HStack, IconButton, Tooltip } from '@chakra-ui/react'
import { AddIcon, CheckCircleIcon, ExternalLinkIcon, RepeatIcon, WarningIcon } from '@chakra-ui/icons'
import { markdown, markdownLanguage } from '@codemirror/lang-markdown'
import { languages } from '@codemirror/language-data'
import { oneDark } from '@codemirror/theme-one-dark'
import { EditorView } from '@codemirror/view'
import CodeMirror, { type ReactCodeMirrorRef } from '@uiw/react-codemirror'
import type { MermaidMarkdownBlock, MermaidMarkdownSyncStatus } from '../../api/client'
import { isWailsApp } from '../../config/runtime'
import type { ViewMarkdownDocument } from '../../types'
import { CloseIcon, ReloadIcon, SaveIcon } from '../Icons'
import { MarkdownPreview } from './MarkdownPreview'
import { MermaidMarkdownContext, type MermaidMarkdownContextValue } from './mermaidContext'

export interface MarkdownEditorHandle {
  getMarkdown: () => string
  setMarkdown: (markdown: string) => void
  focus: () => void
}

interface MarkdownEditorProps {
  canEditDocument: boolean
  content: string
  currentMermaidBlock: MermaidMarkdownBlock | null
  editorRef: MutableRefObject<MarkdownEditorHandle | null>
  handleSyncMermaidBlock: () => Promise<void> | void
  isDirty: boolean
  isLoading: boolean
  isSaving: boolean
  markdown: ViewMarkdownDocument | null
  mermaidIntegrationEnabled: boolean
  mermaidContextValue: MermaidMarkdownContextValue
  mermaidSyncStatus: MermaidMarkdownSyncStatus | null
  onChange: (markdown: string) => void
  onClose: () => void
  onOpenInEditor?: () => void
  onReload?: () => Promise<void> | void
  onSave: (markdown: string) => Promise<void> | void
  onSaveAs?: (markdown: string) => Promise<void> | void
  viewId: number | null
}

type MarkdownPanelMode = 'source' | 'split' | 'preview'

const markdownExtensions = [
  markdown({ base: markdownLanguage, codeLanguages: languages }),
  EditorView.lineWrapping,
]

function syncLabel(status: MermaidMarkdownSyncStatus | null) {
  if (status === 'synced') return 'Current view Mermaid block is synced. Click to scroll to it.'
  if (status === 'stale') return 'Current view Mermaid block is stale. Click to scroll to it.'
  if (status === null) return 'Checking current view Mermaid block'
  return 'Insert current view as Mermaid block'
}

function syncButtonLabel(status: MermaidMarkdownSyncStatus | null) {
  if (status === 'synced') return 'Mermaid synced'
  if (status === 'stale') return 'Mermaid stale'
  if (status === null) return 'Checking Mermaid'
  return 'Insert Mermaid'
}

function syncButtonIcon(status: MermaidMarkdownSyncStatus | null) {
  if (status === 'synced') return <CheckCircleIcon />
  if (status === 'stale') return <WarningIcon />
  if (status === null) return <RepeatIcon />
  return <AddIcon />
}

export function MarkdownEditor({
  canEditDocument,
  content,
  currentMermaidBlock,
  editorRef,
  handleSyncMermaidBlock,
  isDirty,
  isLoading,
  isSaving,
  markdown: markdownDocument,
  mermaidIntegrationEnabled,
  mermaidContextValue,
  mermaidSyncStatus,
  onChange,
  onClose,
  onOpenInEditor,
  onReload,
  onSave,
  onSaveAs,
  viewId,
}: MarkdownEditorProps) {
  const codeMirrorRef = useRef<ReactCodeMirrorRef>(null)
  const previewPaneRef = useRef<HTMLDivElement>(null)
  const contentRef = useRef(content)
  const [mode, setMode] = useState<MarkdownPanelMode>('split')
  contentRef.current = content

  const getMarkdown = useCallback(() => (
    codeMirrorRef.current?.view?.state.doc.toString() ?? contentRef.current
  ), [])

  const focusEditor = useCallback(() => {
    codeMirrorRef.current?.view?.focus()
  }, [])

  const setEditorMarkdown = useCallback((nextMarkdown: string) => {
    const view = codeMirrorRef.current?.view
    if (!view) {
      onChange(nextMarkdown)
      return
    }
    view.dispatch({
      changes: { from: 0, to: view.state.doc.length, insert: nextMarkdown },
      scrollIntoView: true,
    })
  }, [onChange])

  useImperativeHandle(editorRef, () => ({
    getMarkdown,
    setMarkdown: setEditorMarkdown,
    focus: focusEditor,
  }), [focusEditor, getMarkdown, setEditorMarkdown])

  const handleEditorChange = useCallback((nextMarkdown: string) => {
    if (nextMarkdown === contentRef.current) return
    onChange(nextMarkdown)
  }, [onChange])

  const scrollToCurrentMermaidBlock = useCallback(() => {
    if (!currentMermaidBlock) return
    const view = codeMirrorRef.current?.view
    if (view && mode !== 'preview') {
      const position = Math.max(0, Math.min(currentMermaidBlock.start, view.state.doc.length))
      view.dispatch({
        effects: EditorView.scrollIntoView(position, { y: 'center' }),
      })
      view.focus()
    }

    if (mode !== 'source' && currentMermaidBlock.viewId != null) {
      previewPaneRef.current
        ?.querySelector<HTMLElement>(`[data-tld-view-id="${currentMermaidBlock.viewId}"]`)
        ?.scrollIntoView({ block: 'center', behavior: 'smooth' })
    }
  }, [currentMermaidBlock, mode])

  const mermaidPrimaryAction = mermaidSyncStatus === 'missing' ? 'insert' : currentMermaidBlock ? 'scroll' : 'checking'
  const isMermaidPrimaryDisabled = isLoading ||
    !markdownDocument ||
    !viewId ||
    mermaidPrimaryAction === 'checking' ||
    (mermaidPrimaryAction === 'insert' && !canEditDocument)

  const handleMermaidPrimaryAction = useCallback(() => {
    if (mermaidPrimaryAction === 'insert') {
      void handleSyncMermaidBlock()
      return
    }
    if (mermaidPrimaryAction === 'scroll') scrollToCurrentMermaidBlock()
  }, [handleSyncMermaidBlock, mermaidPrimaryAction, scrollToCurrentMermaidBlock])

  return (
    <MermaidMarkdownContext.Provider value={mermaidContextValue}>
      <Box className={`tld-markdown-editor tld-markdown-editor--${mode}`} data-testid="markdown-editor">
        <HStack className="tld-markdown-toolbar" spacing={1.5}>
          <ButtonGroup className="tld-markdown-mode" size="xs" variant="ghost" spacing={1}>
            <Button aria-pressed={mode === 'source'} onClick={() => setMode('source')}>Source</Button>
            <Button aria-pressed={mode === 'split'} onClick={() => setMode('split')}>Split</Button>
            <Button aria-pressed={mode === 'preview'} onClick={() => setMode('preview')}>Preview</Button>
          </ButtonGroup>

          <Box className="tld-markdown-toolbar-center">
            {mermaidIntegrationEnabled && (
              <HStack className="tld-markdown-sync-group" spacing={1}>
                <Tooltip label={syncLabel(mermaidSyncStatus)} hasArrow openDelay={200}>
                  <Box as="span" minW={0}>
                    <Button
                      data-testid="markdown-insert-view-mermaid-button"
                      aria-label={syncLabel(mermaidSyncStatus)}
                      size="xs"
                      variant="ghost"
                      className={`tld-markdown-sync-button tld-markdown-sync-button-${mermaidSyncStatus ?? 'checking'}`}
                      leftIcon={syncButtonIcon(mermaidSyncStatus)}
                      onClick={handleMermaidPrimaryAction}
                      isDisabled={isMermaidPrimaryDisabled}
                    >
                      {syncButtonLabel(mermaidSyncStatus)}
                    </Button>
                  </Box>
                </Tooltip>
                {mermaidSyncStatus === 'stale' && (
                  <Tooltip label="Update current view Mermaid block" hasArrow openDelay={200}>
                    <Box as="span">
                      <IconButton
                        data-testid="markdown-update-view-mermaid-button"
                        aria-label="Update current view Mermaid block"
                        size="xs"
                        variant="ghost"
                        className="tld-markdown-sync-update"
                        icon={<RepeatIcon />}
                        onClick={() => { void handleSyncMermaidBlock() }}
                        isDisabled={!canEditDocument || isLoading || !markdownDocument || !viewId}
                      />
                    </Box>
                  </Tooltip>
                )}
              </HStack>
            )}
          </Box>

          <HStack className="tld-markdown-toolbar-actions" spacing={1.5}>
            <Tooltip label="Reload" hasArrow openDelay={200}>
              <Box as="span">
                <IconButton
                  aria-label="Reload"
                  size="xs"
                  variant="ghost"
                  className="tld-markdown-toolbar-action"
                  icon={<ReloadIcon />}
                  onClick={() => { void onReload?.() }}
                  isDisabled={isLoading || !markdownDocument}
                />
              </Box>
            </Tooltip>
            <Tooltip label="Save" hasArrow openDelay={200}>
              <Box as="span">
                <IconButton
                  data-testid="markdown-save-button"
                  aria-label="Save"
                  size="xs"
                  className="tld-markdown-toolbar-action tld-markdown-toolbar-action-save"
                  icon={<SaveIcon />}
                  onClick={() => { void onSave(getMarkdown()) }}
                  isLoading={isSaving}
                  isDisabled={!canEditDocument || isLoading || !markdownDocument || !isDirty}
                />
              </Box>
            </Tooltip>
            {isWailsApp && onSaveAs && (
              <Tooltip label="Save As" hasArrow openDelay={200}>
                <Box as="span">
                  <IconButton
                    aria-label="Save As"
                    size="xs"
                    variant="ghost"
                    className="tld-markdown-toolbar-action"
                    icon={<SaveIcon />}
                    onClick={() => { void onSaveAs(getMarkdown()) }}
                    isDisabled={isLoading || !markdownDocument}
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
                    isDisabled={isLoading || !markdownDocument}
                  />
                </Box>
              </Tooltip>
            )}
            <Tooltip label="Close" hasArrow openDelay={200}>
              <IconButton
                aria-label="Close"
                size="xs"
                variant="ghost"
                className="tld-markdown-toolbar-action"
                icon={<CloseIcon />}
                onClick={onClose}
              />
            </Tooltip>
          </HStack>
        </HStack>

        <Box className="tld-markdown-workspace">
          {mode !== 'preview' && (
            <Box className="tld-markdown-source-pane" data-testid="markdown-source-pane">
              <CodeMirror
                ref={codeMirrorRef}
                value={content}
                height="100%"
                theme={oneDark}
                extensions={markdownExtensions}
                readOnly={!canEditDocument}
                editable={canEditDocument}
                basicSetup={{
                  lineNumbers: true,
                  foldGutter: true,
                  highlightActiveLineGutter: true,
                }}
                className="tld-markdown-codemirror"
                onChange={handleEditorChange}
              />
            </Box>
          )}
          {mode !== 'source' && (
            <Box ref={previewPaneRef} className="tld-markdown-preview-pane" data-testid="markdown-preview-pane">
              <MarkdownPreview markdown={content} mermaidEnabled={mermaidIntegrationEnabled} />
            </Box>
          )}
        </Box>
      </Box>
    </MermaidMarkdownContext.Provider>
  )
}
