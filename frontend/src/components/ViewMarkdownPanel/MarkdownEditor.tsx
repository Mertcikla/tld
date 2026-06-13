import { useCallback, useImperativeHandle, useRef, useState, type MutableRefObject } from 'react'
import { Box, Button, ButtonGroup, HStack, IconButton, Tooltip } from '@chakra-ui/react'
import { AddIcon, CheckCircleIcon, ExternalLinkIcon, RepeatIcon } from '@chakra-ui/icons'
import { markdown, markdownLanguage } from '@codemirror/lang-markdown'
import { languages } from '@codemirror/language-data'
import { oneDark } from '@codemirror/theme-one-dark'
import { EditorView } from '@codemirror/view'
import CodeMirror, { type ReactCodeMirrorRef } from '@uiw/react-codemirror'
import type { MermaidMarkdownSyncStatus } from '../../api/client'
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
  editorRef: MutableRefObject<MarkdownEditorHandle | null>
  handleSyncMermaidBlock: () => Promise<void> | void
  isDirty: boolean
  isLoading: boolean
  isSaving: boolean
  markdown: ViewMarkdownDocument | null
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
  if (status === 'synced') return 'Current view Mermaid block is synced'
  if (status === 'stale') return 'Update current view Mermaid block'
  return 'Insert current view as Mermaid block'
}

function syncButtonLabel(status: MermaidMarkdownSyncStatus | null) {
  if (status === 'synced') return 'Mermaid synced'
  if (status === 'stale') return 'Update Mermaid'
  return 'Insert Mermaid'
}

function syncButtonIcon(status: MermaidMarkdownSyncStatus | null) {
  if (status === 'synced') return <CheckCircleIcon />
  if (status === 'stale') return <RepeatIcon />
  return <AddIcon />
}

export function MarkdownEditor({
  canEditDocument,
  content,
  editorRef,
  handleSyncMermaidBlock,
  isDirty,
  isLoading,
  isSaving,
  markdown: markdownDocument,
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

  return (
    <MermaidMarkdownContext.Provider value={mermaidContextValue}>
      <Box className={`tld-markdown-editor tld-markdown-editor--${mode}`} data-testid="markdown-editor">
        <HStack className="tld-markdown-toolbar" spacing={1.5}>
          <ButtonGroup className="tld-markdown-mode" size="xs" isAttached>
            <Button aria-pressed={mode === 'source'} onClick={() => setMode('source')}>Source</Button>
            <Button aria-pressed={mode === 'split'} onClick={() => setMode('split')}>Split</Button>
            <Button aria-pressed={mode === 'preview'} onClick={() => setMode('preview')}>Preview</Button>
          </ButtonGroup>

          <Box className="tld-markdown-toolbar-center">
            <Tooltip label={syncLabel(mermaidSyncStatus)} hasArrow openDelay={200}>
              <Box as="span" minW={0}>
                <Button
                  data-testid="markdown-insert-view-mermaid-button"
                  aria-label={syncLabel(mermaidSyncStatus)}
                  size="xs"
                  variant="ghost"
                  className={`tld-markdown-sync-button tld-markdown-sync-button-${mermaidSyncStatus ?? 'missing'}`}
                  leftIcon={syncButtonIcon(mermaidSyncStatus)}
                  onClick={() => { void handleSyncMermaidBlock() }}
                  isDisabled={!canEditDocument || isLoading || !markdownDocument || !viewId || mermaidSyncStatus === 'synced'}
                >
                  {syncButtonLabel(mermaidSyncStatus)}
                </Button>
              </Box>
            </Tooltip>
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
            <Box className="tld-markdown-preview-pane" data-testid="markdown-preview-pane">
              <MarkdownPreview markdown={content} />
            </Box>
          )}
        </Box>
      </Box>
    </MermaidMarkdownContext.Provider>
  )
}
