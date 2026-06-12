import { memo, useCallback, useEffect, useMemo, useRef, useState } from 'react'
import { Box } from '@chakra-ui/react'
import type { MDXEditorMethods } from '@mdxeditor/editor'
import { isWailsApp } from '../../config/runtime'
import { defaultRepoMarkdownPath } from './metadata'
import { MarkdownEditor } from './MarkdownEditor'
import { MarkdownFloatingMenu } from './MarkdownFloatingMenu'
import { LoadingMarkdownState, MarkdownSetupState, MissingMarkdownState } from './MarkdownPanelStates'
import { markdownPanelBodySx } from './styles'
import type { ViewMarkdownPanelProps } from './types'
import { useFloatingMarkdownMenu } from './hooks/useFloatingMarkdownMenu'
import { useMermaidMarkdownSync } from './hooks/useMermaidMarkdownSync'

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
}: ViewMarkdownPanelProps) {
  const editorRef = useRef<MDXEditorMethods>(null)
  const latestContentRef = useRef(content)
  latestContentRef.current = content

  const suggestedRepoPath = useMemo(() => defaultRepoMarkdownPath(viewName), [viewName])
  const [repoPath, setRepoPath] = useState(suggestedRepoPath)
  const [attachPath, setAttachPath] = useState('')
  const canEditDocument = canEdit && !!markdown?.can_edit
  const showNativeAttachPicker = isWailsApp && !!onPickMarkdownFile
  const showAttachPathInput = !showNativeAttachPicker
  const {
    panelBodyRef,
    floatingMenuRef,
    floatingMenuPosition,
    isFloatingMenuHidden,
    setIsFloatingMenuHidden,
    isFloatingMenuDragging,
    handleFloatingMenuDragStart,
  } = useFloatingMarkdownMenu()

  const currentEditorMarkdown = useCallback(() => editorRef.current?.getMarkdown() ?? latestContentRef.current, [])

  const {
    mermaidContextValue,
    mermaidSyncStatus,
    handleSyncMermaidBlock,
  } = useMermaidMarkdownSync({
    canEditDocument,
    content,
    editorRef,
    enabled: mermaidIntegrationEnabled,
    isOpen,
    latestContentRef,
    onChange,
    viewId,
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

  const handleCopyPath = useCallback(async () => {
    if (!markdown?.path || typeof navigator === 'undefined' || !navigator.clipboard) return
    await navigator.clipboard.writeText(markdown.path)
  }, [markdown?.path])

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
        sx={markdownPanelBodySx}
      >
        <MarkdownFloatingMenu
          canEdit={canEdit}
          canEditDocument={canEditDocument}
          currentEditorMarkdown={currentEditorMarkdown}
          floatingMenuPosition={floatingMenuPosition}
          floatingMenuRef={floatingMenuRef}
          handleCopyPath={handleCopyPath}
          handleFloatingMenuDragStart={handleFloatingMenuDragStart}
          hasSaveConflict={hasSaveConflict}
          isDirty={isDirty}
          isFloatingMenuDragging={isFloatingMenuDragging}
          isFloatingMenuHidden={isFloatingMenuHidden}
          isLoading={isLoading}
          markdown={markdown}
          onClose={onClose}
          onForceSave={onForceSave}
          onOpenInEditor={onOpenInEditor}
          onReload={onReload}
          onSave={onSave}
          onUnlinkMarkdown={onUnlinkMarkdown}
          setIsFloatingMenuHidden={setIsFloatingMenuHidden}
        />

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
            currentEditorMarkdown={currentEditorMarkdown}
            editorRef={editorRef}
            handleSyncMermaidBlock={handleSyncMermaidBlock}
            isDirty={isDirty}
            isLoading={isLoading}
            isSaving={isSaving}
            markdown={markdown}
            mermaidContextValue={mermaidContextValue}
            mermaidIntegrationEnabled={mermaidIntegrationEnabled}
            mermaidSyncStatus={mermaidSyncStatus}
            onChange={onChange}
            onClose={onClose}
            onOpenInEditor={onOpenInEditor}
            onReload={onReload}
            onSave={onSave}
            onSaveAs={onSaveAs}
            syncToken={syncToken}
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
