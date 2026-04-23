import { useCallback, useEffect, useRef, useState } from 'react'
import type { DrawingPath } from '../../../components/DrawingCanvas'
import { DRAWING_COLORS } from '../../../constants/colors'

export function useDrawingEngine(viewId: number | null) {
  const [drawingMode, setDrawingMode] = useState(false)
  const [drawingVisible, setDrawingVisible] = useState(true)
  const [drawingPaths, setDrawingPaths] = useState<DrawingPath[]>([])
  const [drawingTool, setDrawingTool] = useState<'pencil' | 'eraser' | 'text' | 'select'>('pencil')
  const [drawingColor, setDrawingColor] = useState(DRAWING_COLORS[0])
  const [drawingWidth, setDrawingWidth] = useState(3)
  const [textEditorState, setTextEditorState] = useState<{
    canvasX: number; canvasY: number; flowX: number; flowY: number
  } | null>(null)

  const drawingHistoryRef = useRef<DrawingPath[][]>([])
  const drawingRedoStackRef = useRef<DrawingPath[][]>([])

  const lastViewIdRef = useRef<number | null>(null)

  // Reset drawing state only when viewId actually changes to a new value
  useEffect(() => {
    if (lastViewIdRef.current !== viewId) {
      setDrawingPaths([])
      setDrawingMode(false)
      setTextEditorState(null)
      lastViewIdRef.current = viewId
    }
  }, [viewId])

  const handleUndo = useCallback(() => {
    if (drawingHistoryRef.current.length === 0) return
    const prevState = drawingHistoryRef.current.pop()!
    setDrawingPaths((current) => {
      drawingRedoStackRef.current.push([...current])
      return prevState
    })
  }, [])

  const handleRedo = useCallback(() => {
    if (drawingRedoStackRef.current.length === 0) return
    const nextState = drawingRedoStackRef.current.pop()!
    setDrawingPaths((current) => {
      drawingHistoryRef.current.push([...current])
      return nextState
    })
  }, [])

  const commitDrawingText = useCallback(
    (value: string, state: { canvasX: number; canvasY: number; flowX: number; flowY: number }) => {
      setTextEditorState(null)
      if (!value.trim()) return
      const path: DrawingPath = {
        id: `text-${Date.now()}-${Math.random().toString(36).slice(2)}`,
        points: [{ x: state.flowX, y: state.flowY }],
        color: drawingColor,
        width: drawingWidth,
        text: value,
        fontSize: Math.max(14, drawingWidth * 5),
      }
      setDrawingPaths((prev) => {
        drawingHistoryRef.current.push([...prev])
        drawingRedoStackRef.current = []
        return [...prev, path]
      })
    },
    [drawingColor, drawingWidth],
  )

  const onPathComplete = useCallback(
    (path: DrawingPath) => {
      setDrawingPaths((prev) => {
        drawingHistoryRef.current.push([...prev])
        drawingRedoStackRef.current = []
        return [...prev, path]
      })
    },
    [],
  )

  const onPathDelete = useCallback(
    (pathId: string) => {
      setDrawingPaths((prev) => {
        drawingHistoryRef.current.push([...prev])
        drawingRedoStackRef.current = []
        return prev.filter((p) => p.id !== pathId)
      })
    },
    [],
  )

  const onPathUpdate = useCallback(
    (path: DrawingPath) => {
      setDrawingPaths((prev) => {
        drawingHistoryRef.current.push([...prev])
        drawingRedoStackRef.current = []
        return prev.map(p => p.id === path.id ? path : p)
      })
    },
    [],
  )

  return {
    drawingMode,
    setDrawingMode,
    drawingVisible,
    setDrawingVisible,
    drawingPaths,
    setDrawingPaths,
    drawingTool,
    setDrawingTool,
    drawingColor,
    setDrawingColor,
    drawingWidth,
    setDrawingWidth,
    textEditorState,
    setTextEditorState,
    drawingHistoryRef,
    drawingRedoStackRef,
    handleUndo,
    handleRedo,
    commitDrawingText,
    onPathComplete,
    onPathDelete,
    onPathUpdate,
  }
}
