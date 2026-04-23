import { createContext, useContext } from 'react'
import type { LibraryElement, Connector } from '../../types'

export interface ViewEditorContextValue {
  viewId: number | null
  canEdit: boolean
  isOwner: boolean
  isFreePlan: boolean
  snapToGrid: boolean
  setSnapToGrid: (snap: boolean) => void
  selectedElement: LibraryElement | null
  selectedConnector: Connector | null
}

export const ViewEditorContext = createContext<ViewEditorContextValue | null>(null)

export function useViewEditorContext(): ViewEditorContextValue {
  const ctx = useContext(ViewEditorContext)
  if (!ctx) throw new Error('useViewEditorContext must be used inside ViewEditor')
  return ctx
}
