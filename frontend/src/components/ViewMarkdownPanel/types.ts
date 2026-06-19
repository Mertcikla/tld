import type { ViewMarkdownDocument } from '../../types'

export interface ViewMarkdownPanelProps {
  isOpen: boolean
  onClose: () => void
  viewName?: string | null
  markdown: ViewMarkdownDocument | null
  content: string
  syncToken: number
  viewId?: number | null
  viewNameById?: Map<number, string>
  mermaidIntegrationEnabled?: boolean
  dbOnlyNotes?: boolean
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
  onNavigateToView?: (viewId: number) => void
  onImportMermaidBlock?: (source: string) => Promise<void> | void
}
