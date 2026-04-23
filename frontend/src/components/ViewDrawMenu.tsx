import React from 'react'
import { Box } from '@chakra-ui/react'
import { DRAWING_COLORS } from '../constants/colors'

export function UndoSvg() {
  return (
    <svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2.5" strokeLinecap="round" strokeLinejoin="round">
      <polyline points="1 4 1 10 7 10" />
      <path d="M3.51 15a9 9 0 1 0 2.13-9.36L1 10" />
    </svg>
  )
}
export function RedoSvg() {
  return (
    <svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2.5" strokeLinecap="round" strokeLinejoin="round">
      <polyline points="23 4 23 10 17 10" />
      <path d="M20.49 15a9 9 0 1 1-2.13-9.36L23 10" />
    </svg>
  )
}
export function PencilSvg() {
  return (
    <svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2.5" strokeLinecap="round" strokeLinejoin="round">
      <path d="M17 3a2.828 2.828 0 1 1 4 4L7.5 20.5 2 22l1.5-5.5L17 3z" />
    </svg>
  )
}
export function EraserSvg() {
  return (
    <svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2.5" strokeLinecap="round" strokeLinejoin="round">
      <path d="M20 20H7L3 16C2 15 2 13 3 12L13 2L22 11L20 13L17 10L10 17L13 20" />
    </svg>
  )
}
export function TextSvg() {
  return (
    <svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2.5" strokeLinecap="round" strokeLinejoin="round">
      <polyline points="4 7 4 4 20 4 20 7" />
      <line x1="9" y1="20" x2="15" y2="20" />
      <line x1="12" y1="4" x2="12" y2="20" />
    </svg>
  )
}
export function MoveSvg() {
  return (
    <svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2.5" strokeLinecap="round" strokeLinejoin="round">
      <polyline points="5 9 2 12 5 15" />
      <polyline points="9 5 12 2 15 5" />
      <polyline points="15 19 12 22 9 19" />
      <polyline points="19 9 22 12 19 15" />
      <line x1="2" y1="12" x2="22" y2="12" />
      <line x1="12" y1="2" x2="12" y2="22" />
    </svg>
  )
}
export function CloseSvg() {
  return (
    <svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2.5" strokeLinecap="round" strokeLinejoin="round">
      <line x1="18" y1="6" x2="6" y2="18" />
      <line x1="6" y1="6" x2="18" y2="18" />
    </svg>
  )
}

function DrawingBtn({ active, disabled, onClick, title, children }: {
  active?: boolean
  disabled?: boolean
  onClick?: () => void
  title?: string
  children: React.ReactNode
}) {
  return (
    <Box
      as="button"
      title={title}
      onClick={disabled ? undefined : onClick}
      w="34px"
      h="34px"
      display="flex"
      alignItems="center"
      justifyContent="center"
      rounded="8px"
      position="relative"
      cursor={disabled ? 'default' : 'pointer'}
      opacity={disabled ? 0.3 : 1}
      outline="none"
      border="none"
      bg={active ? 'rgba(var(--accent-rgb, 99,179,237), 0.15)' : 'transparent'}
      color={active ? 'var(--accent)' : 'rgba(255,255,255,0.6)'}
      transition="all 0.1s"
      _hover={disabled ? {} : {
        bg: active ? 'rgba(var(--accent-rgb, 99,179,237), 0.22)' : 'rgba(255,255,255,0.07)',
        color: 'white',
      }}
      flexShrink={0}
    >
      {children}
      {active && (
        <Box
          position="absolute"
          bottom="4px"
          left="50%"
          transform="translateX(-50%)"
          w="4px"
          h="4px"
          rounded="full"
          bg="var(--accent)"
        />
      )}
    </Box>
  )
}

function DrawingDivider() {
  return <Box w="1px" h="20px" bg="rgba(255,255,255,0.08)" mx="4px" flexShrink={0} />
}

const DRAWING_WIDTHS = [1, 3, 6, 12]

export interface ViewDrawMenuProps {
  drawingMode: boolean
  drawingTool: 'pencil' | 'eraser' | 'text' | 'select'
  setDrawingTool: (tool: 'pencil' | 'eraser' | 'text' | 'select') => void
  drawingColor: string
  setDrawingColor: (color: string) => void
  drawingWidth: number
  setDrawingWidth: (width: number) => void
  onUndo: () => void
  onRedo: () => void
  canUndo: boolean
  canRedo: boolean
  setDrawingMode: (mode: boolean) => void
}

export default function ViewDrawMenu({
  drawingMode,
  drawingTool,
  setDrawingTool,
  drawingColor,
  setDrawingColor,
  drawingWidth,
  setDrawingWidth,
  onUndo,
  onRedo,
  canUndo,
  canRedo,
  setDrawingMode,
}: ViewDrawMenuProps) {
  if (!drawingMode) return null

  return (
    <Box
      position="absolute"
      top="12px"
      left="50%"
      transform="translateX(-50%)"
      zIndex={100}
      display="flex"
      alignItems="center"
      gap="2px"
      bg="var(--bg-panel)"
      backdropFilter="blur(20px)"
      border="1px solid rgba(255,255,255,0.1)"
      borderRadius="16px"
      boxShadow="0 8px 32px rgba(0,0,0,0.5)"
      px="6px"
      py="6px"
      userSelect="none"
    >
      {/* Selection / Move */}
      <DrawingBtn title="Select & Move (V)" active={drawingTool === 'select'} onClick={() => setDrawingTool('select')}>
        <MoveSvg />
      </DrawingBtn>

      <DrawingDivider />

      {/* Tools */}
      <DrawingBtn title="Pen (P)" active={drawingTool === 'pencil'} onClick={() => setDrawingTool('pencil')}>
        <PencilSvg />
      </DrawingBtn>
      <DrawingBtn title="Eraser (E)" active={drawingTool === 'eraser'} onClick={() => setDrawingTool('eraser')}>
        <EraserSvg />
      </DrawingBtn>
      <DrawingBtn title="Text (T)" active={drawingTool === 'text'} onClick={() => setDrawingTool('text')}>
        <TextSvg />
      </DrawingBtn>

      <DrawingDivider />

      {/* Color swatches */}
      <Box display="flex" gap="4px" px="4px">
        {DRAWING_COLORS.map(c => (
          <Box
            key={c}
            as="button"
            title={c}
            onClick={() => setDrawingColor(c)}
            w="18px"
            h="18px"
            rounded="full"
            bg={c}
            cursor="pointer"
            flexShrink={0}
            outline="none"
            border="none"
            boxShadow={
              drawingColor === c
                ? `0 0 0 2px var(--bg-panel), 0 0 0 4px ${c}`
                : '0 0 0 1px rgba(255,255,255,0.1)'
            }
            transition="all 0.15s cubic-bezier(0.4, 0, 0.2, 1)"
            _hover={{
              transform: 'scale(1.2)',
              boxShadow: drawingColor === c
                ? `0 0 0 2px var(--bg-panel), 0 0 0 4px ${c}`
                : `0 0 0 2px var(--bg-panel), 0 0 0 3px ${c}`
            }}
          />
        ))}
      </Box>

      <DrawingDivider />

      {/* Stroke width presets (horizontal lines) */}
      <Box display="flex" gap="2px" px="4px">
        {DRAWING_WIDTHS.map(size => (
          <Box
            key={size}
            as="button"
            title={drawingTool === 'text' ? `Font size ${size * 5}px` : `Width ${size}px`}
            onClick={() => setDrawingWidth(size)}
            w="30px"
            h="30px"
            display="flex"
            alignItems="center"
            justifyContent="center"
            rounded="8px"
            bg={drawingWidth === size ? 'rgba(255,255,255,0.1)' : 'transparent'}
            transition="all 0.15s"
            _hover={{ bg: 'rgba(255,255,255,0.05)' }}
          >
            <Box
              w="16px"
              h={`${Math.max(1, size / 1.5)}px`}
              rounded="full"
              bg={drawingWidth === size ? 'white' : 'whiteAlpha.400'}
            />
          </Box>
        ))}
      </Box>

      <DrawingDivider />

      {/* Undo / Redo */}
      <DrawingBtn title="Undo (Cmd+Z)" disabled={!canUndo} onClick={onUndo}>
        <UndoSvg />
      </DrawingBtn>
      <DrawingBtn title="Redo (Cmd+Shift+Z)" disabled={!canRedo} onClick={onRedo}>
        <RedoSvg />
      </DrawingBtn>

      <DrawingDivider />

      {/* Exit */}
      <DrawingBtn title="Done" onClick={() => setDrawingMode(false)}>
        <CloseSvg />
      </DrawingBtn>
    </Box>
  )
}
