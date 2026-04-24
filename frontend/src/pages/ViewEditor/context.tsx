import { createContext } from 'react'
import type { LibraryElement, Connector } from '../../types'
import { useStore } from '../../store/useStore'

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
  const viewId = useStore((state) => state.viewId)
  const canEdit = useStore((state) => state.canEdit)
  const isOwner = useStore((state) => state.isOwner)
  const isFreePlan = useStore((state) => state.isFreePlan)
  const snapToGrid = useStore((state) => state.snapToGrid)
  const setSnapToGrid = useStore((state) => state.setSnapToGrid)
  const selectedElement = useStore((state) => state.selectedElement)
  const selectedConnector = useStore((state) => state.selectedConnector)

  return { viewId, canEdit, isOwner, isFreePlan, snapToGrid, setSnapToGrid, selectedElement, selectedConnector }
}
