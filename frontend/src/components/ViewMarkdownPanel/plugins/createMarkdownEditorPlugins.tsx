import { Box, HStack, IconButton, Tooltip } from '@chakra-ui/react'
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
  quotePlugin,
  type RealmPlugin,
  thematicBreakPlugin,
  toolbarPlugin,
  UndoRedo,
} from '@mdxeditor/editor'
import { isWailsApp } from '../../../config/runtime'
import type { MermaidMarkdownSyncStatus } from '../../../api/client'
import type { ViewMarkdownDocument } from '../../../types'
import { CloseIcon, ReloadIcon, SaveIcon } from '../../Icons'
import { MERMAID_CODE_BLOCK_DESCRIPTOR } from './mermaidCodeBlock'

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

export interface MarkdownEditorPluginContext {
  canEditDocument: boolean
  currentEditorMarkdown: () => string
  handleSyncMermaidBlock: () => Promise<void> | void
  isDirty: boolean
  isLoading: boolean
  isSaving: boolean
  markdown: ViewMarkdownDocument | null
  mermaidIntegrationEnabled: boolean
  mermaidSyncStatus: MermaidMarkdownSyncStatus | null
  onClose: () => void
  onOpenInEditor?: () => void
  onReload?: () => Promise<void> | void
  onSave: (markdown: string) => Promise<void> | void
  onSaveAs?: (markdown: string) => Promise<void> | void
  viewId: number | null
}

export function createMarkdownEditorPlugins(ctx: MarkdownEditorPluginContext): RealmPlugin[] {
  const baseMarkdownPlugins = [
    headingsPlugin(),
    listsPlugin(),
    quotePlugin(),
    thematicBreakPlugin(),
  ]

  const codePlugins = [
    codeBlockPlugin({
      defaultCodeBlockLanguage: 'mermaid',
      codeBlockEditorDescriptors: ctx.mermaidIntegrationEnabled ? [MERMAID_CODE_BLOCK_DESCRIPTOR] : [],
    }),
    codeMirrorPlugin({
      codeBlockLanguages: CODE_BLOCK_LANGUAGES,
      autoLoadLanguageSupport: false,
    }),
  ]

  const editingPlugins = [
    markdownShortcutPlugin(),
    linkDialogPlugin(),
  ]

  const toolbarPlugins = [
    toolbarPlugin({
      toolbarClassName: 'tld-markdown-toolbar',
      toolbarContents: () => (
        <>
          {ctx.canEditDocument && (
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
            {ctx.mermaidIntegrationEnabled && (
              <Tooltip
                label={ctx.mermaidSyncStatus === 'synced' ? 'Diagram block is synced' : ctx.mermaidSyncStatus === 'stale' ? 'Update diagram block from view' : 'Insert diagram block from view'}
                hasArrow
                openDelay={200}
              >
                <Box as="span">
                  <IconButton
                    aria-label="Insert/update diagram"
                    size="xs"
                    variant="ghost"
                    className={`tld-markdown-toolbar-action tld-markdown-toolbar-action-mermaid tld-markdown-toolbar-action-mermaid-${ctx.mermaidSyncStatus ?? 'missing'}`}
                    icon={<ReloadIcon />}
                    onClick={() => { void ctx.handleSyncMermaidBlock() }}
                    isDisabled={!ctx.canEditDocument || ctx.isLoading || !ctx.markdown || !ctx.viewId || ctx.mermaidSyncStatus === 'synced'}
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
                  onClick={() => { void ctx.onReload?.() }}
                  isDisabled={ctx.isLoading || !ctx.markdown}
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
                  onClick={() => { void ctx.onSave(ctx.currentEditorMarkdown()) }}
                  isLoading={ctx.isSaving}
                  isDisabled={!ctx.canEditDocument || ctx.isLoading || !ctx.markdown || !ctx.isDirty}
                />
              </Box>
            </Tooltip>
            {isWailsApp && ctx.onSaveAs && (
              <Tooltip label="Save As" hasArrow openDelay={200}>
                <Box as="span">
                  <IconButton
                    aria-label="Save As"
                    size="xs"
                    className="tld-markdown-toolbar-action"
                    icon={<SaveIcon />}
                    onClick={() => { void ctx.onSaveAs?.(ctx.currentEditorMarkdown()) }}
                    isDisabled={ctx.isLoading || !ctx.markdown}
                  />
                </Box>
              </Tooltip>
            )}
            {ctx.onOpenInEditor && (
              <Tooltip label="Open in Editor" hasArrow openDelay={200}>
                <Box as="span">
                  <IconButton
                    aria-label="Open in Editor"
                    size="xs"
                    variant="ghost"
                    className="tld-markdown-toolbar-action"
                    icon={<ExternalLinkIcon />}
                    onClick={ctx.onOpenInEditor}
                    isDisabled={ctx.isLoading || !ctx.markdown}
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
                  onClick={ctx.onClose}
                />
              </Box>
            </Tooltip>
          </HStack>
        </>
      ),
    }),
  ]

  return [
    ...baseMarkdownPlugins,
    ...codePlugins,
    ...editingPlugins,
    ...toolbarPlugins,
  ]
}
