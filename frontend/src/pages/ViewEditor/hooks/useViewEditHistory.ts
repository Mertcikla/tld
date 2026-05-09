import { useCallback, useState } from 'react'

const MAX_HISTORY_ITEMS = 20

export interface ViewEditHistoryAction {
  undo: () => Promise<void>
  redo: () => Promise<void>
}

export function useViewEditHistory() {
  const [undoStack, setUndoStack] = useState<ViewEditHistoryAction[]>([])
  const [redoStack, setRedoStack] = useState<ViewEditHistoryAction[]>([])
  const [isApplyingHistory, setIsApplyingHistory] = useState(false)

  const pushAction = useCallback((action: ViewEditHistoryAction) => {
    setUndoStack((stack) => [...stack, action].slice(-MAX_HISTORY_ITEMS))
    setRedoStack([])
  }, [])

  const clearHistory = useCallback(() => {
    setUndoStack([])
    setRedoStack([])
  }, [])

  const undo = useCallback(async () => {
    if (isApplyingHistory) return
    const action = undoStack[undoStack.length - 1]
    if (!action) return
    setIsApplyingHistory(true)
    try {
      await action.undo()
      setUndoStack((stack) => stack.slice(0, -1))
      setRedoStack((stack) => [...stack, action].slice(-MAX_HISTORY_ITEMS))
    } finally {
      setIsApplyingHistory(false)
    }
  }, [isApplyingHistory, undoStack])

  const redo = useCallback(async () => {
    if (isApplyingHistory) return
    const action = redoStack[redoStack.length - 1]
    if (!action) return
    setIsApplyingHistory(true)
    try {
      await action.redo()
      setRedoStack((stack) => stack.slice(0, -1))
      setUndoStack((stack) => [...stack, action].slice(-MAX_HISTORY_ITEMS))
    } finally {
      setIsApplyingHistory(false)
    }
  }, [isApplyingHistory, redoStack])

  return {
    canUndo: undoStack.length > 0,
    canRedo: redoStack.length > 0,
    isApplyingHistory,
    pushAction,
    clearHistory,
    undo,
    redo,
  }
}
