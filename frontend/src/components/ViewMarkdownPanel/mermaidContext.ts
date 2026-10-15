import { createContext, type ReactNode } from 'react'
import type { MermaidMarkdownSyncStatus } from '../../api/client'

export const MermaidHeaderActionsContext = createContext<ReactNode>(null)

export interface MermaidMarkdownContextValue {
  blockStatusByCode: Map<string, MermaidMarkdownSyncStatus>
  canEdit: boolean
  currentViewId: number | null
  currentViewName?: string | null
  viewNameById?: Map<number, string>
  onNavigateToView?: (viewId: number) => void
  onSyncCurrentViewMermaidBlock?: () => Promise<void> | void
  onImportMermaidBlock?: (source: string) => Promise<void> | void
}

export const MermaidMarkdownContext = createContext<MermaidMarkdownContextValue>({
  blockStatusByCode: new Map(),
  canEdit: false,
  currentViewId: null,
})
