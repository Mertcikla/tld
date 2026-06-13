import { createContext } from 'react'
import type { MermaidMarkdownSyncStatus } from '../../api/client'

export interface MermaidMarkdownContextValue {
  blockStatusByCode: Map<string, MermaidMarkdownSyncStatus>
  canEdit: boolean
  currentViewId: number | null
  currentViewName?: string | null
}

export const MermaidMarkdownContext = createContext<MermaidMarkdownContextValue>({
  blockStatusByCode: new Map(),
  canEdit: false,
  currentViewId: null,
})
