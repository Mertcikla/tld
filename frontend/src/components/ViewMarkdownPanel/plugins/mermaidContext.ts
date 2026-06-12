import { createContext } from 'react'
import type { MermaidMarkdownSyncStatus } from '../../../api/client'

export interface MermaidMarkdownContextValue {
  blockStatusByCode: Map<string, MermaidMarkdownSyncStatus>
  canEdit: boolean
}

export const MermaidMarkdownContext = createContext<MermaidMarkdownContextValue>({
  blockStatusByCode: new Map(),
  canEdit: false,
})
