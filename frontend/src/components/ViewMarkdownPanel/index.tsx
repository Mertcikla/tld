import { memo, useCallback, useEffect, useMemo, useRef, useState } from 'react'
import { Box } from '@chakra-ui/react'
import { isWailsApp } from '../../config/runtime'
import { defaultRepoMarkdownPath } from './metadata'
import { MarkdownEditor, type MarkdownEditorHandle } from './MarkdownEditor'
import { LoadingMarkdownState, MarkdownSetupState, MissingMarkdownState } from './MarkdownPanelStates'
import { markdownPanelBodySx } from './styles'
import type { ViewMarkdownPanelProps } from './types'
import { useMermaidMarkdownSync } from './hooks/useMermaidMarkdownSync'

function ViewMarkdownPanel({
  isOpen,
  onClose,
  viewName,
  markdown,
  content,
  viewId = null,
  mermaidIntegrationEnabled = false,
  canEdit = true,
  isLoading = false,
  isSaving = false,
  isDirty = false,
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
  onNavigateToView,
}: ViewMarkdownPanelProps) {
  const editorRef = useRef<MarkdownEditorHandle>(null)
  const latestContentRef = useRef(content)
  latestContentRef.current = content

  const suggestedRepoPath = useMemo(() => defaultRepoMarkdownPath(viewName), [viewName])
  const [repoPath, setRepoPath] = useState(suggestedRepoPath)
  const [attachPath, setAttachPath] = useState('')
  const canEditDocument = canEdit && !!markdown?.can_edit
  const showNativeAttachPicker = isWailsApp && !!onPickMarkdownFile
  const showAttachPathInput = !showNativeAttachPicker

  const currentEditorMarkdown = useCallback(() => editorRef.current?.getMarkdown() ?? latestContentRef.current, [])
  const replaceEditorMarkdown = useCallback((nextMarkdown: string) => {
    latestContentRef.current = nextMarkdown
    if (editorRef.current) {
      editorRef.current.setMarkdown(nextMarkdown)
      return
    }
    onChange(nextMarkdown)
  }, [onChange])

  const {
    currentMermaidBlock,
    mermaidContextValue,
    mermaidSyncStatus,
    handleSyncMermaidBlock,
  } = useMermaidMarkdownSync({
    canEditDocument,
    content,
    enabled: mermaidIntegrationEnabled,
    getMarkdown: currentEditorMarkdown,
    isOpen,
    replaceMarkdown: replaceEditorMarkdown,
    viewId,
    viewName,
    onNavigateToView,
  })

  useEffect(() => {
    setRepoPath(suggestedRepoPath)
  }, [suggestedRepoPath])

  const handleAttachPickedFile = useCallback(async () => {
    if (!onPickMarkdownFile || !onAttachMarkdown) return
    const path = await onPickMarkdownFile()
    if (!path) return
    setAttachPath(path)
    await onAttachMarkdown(path)
  }, [onAttachMarkdown, onPickMarkdownFile])

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
        position="relative"
        bg="var(--bg-canvas)"
        color="gray.100"
        sx={markdownPanelBodySx}
      >
        {isLoading ? (
          <LoadingMarkdownState />
        ) : markdown && !markdown.exists ? (
          <MissingMarkdownState
            attachPath={attachPath}
            canEdit={canEdit}
            handleAttachPickedFile={handleAttachPickedFile}
            markdown={markdown}
            onAttachMarkdown={onAttachMarkdown}
            onForceSave={onForceSave}
            onReload={onReload}
            onUnlinkMarkdown={onUnlinkMarkdown}
            setAttachPath={setAttachPath}
            showAttachPathInput={showAttachPathInput}
            showNativeAttachPicker={showNativeAttachPicker}
            viewName={viewName}
          />
        ) : markdown ? (
          <MarkdownEditor
            canEditDocument={canEditDocument}
            content={content}
            editorRef={editorRef}
            handleSyncMermaidBlock={handleSyncMermaidBlock}
            currentMermaidBlock={currentMermaidBlock}
            isDirty={isDirty}
            isLoading={isLoading}
            isSaving={isSaving}
            markdown={markdown}
            mermaidIntegrationEnabled={mermaidIntegrationEnabled}
            mermaidContextValue={mermaidContextValue}
            mermaidSyncStatus={mermaidSyncStatus}
            onChange={onChange}
            onClose={onClose}
            onOpenInEditor={onOpenInEditor}
            onReload={onReload}
            onSave={onSave}
            onSaveAs={onSaveAs}
            viewId={viewId}
          />
        ) : (
          <MarkdownSetupState
            attachPath={attachPath}
            canEdit={canEdit}
            handleAttachPickedFile={handleAttachPickedFile}
            onAttachMarkdown={onAttachMarkdown}
            onCreateMarkdown={onCreateMarkdown}
            repoPath={repoPath}
            setAttachPath={setAttachPath}
            setRepoPath={setRepoPath}
            showAttachPathInput={showAttachPathInput}
            suggestedRepoPath={suggestedRepoPath}
          />
        )}
      </Box>
    </Box>
  )
}

export default memo(ViewMarkdownPanel)
