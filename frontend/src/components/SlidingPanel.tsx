import { AnimatePresence, motion } from 'framer-motion'
import { Box } from '@chakra-ui/react'
import { type ReactNode, useRef, useEffect } from 'react'

const EASE = [0.25, 0.46, 0.45, 0.94]

interface Props {
  isOpen: boolean
  onClose: () => void
  panelKey: string
  side?: 'left' | 'right'
  width?: string | Record<string, string>
  minWidth?: string | Record<string, string>
  maxHeight?: string
  height?: string
  hasBackdrop?: boolean
  zIndex?: number
  children: ReactNode
}

export default function SlidingPanel({
  isOpen,
  onClose,
  panelKey,
  side = 'right',
  width = '300px',
  minWidth,
  maxHeight = 'calc(90vh - 7rem)',
  height = 'calc(90vh - 7rem)',
  hasBackdrop = true,
  zIndex = 1000,
  children,
}: Props) {
  // Use width if it's a fixed value, otherwise default to a safe offscreen distance
  const resolvedWidth = typeof width === 'string' ? width : '320px'
  const isFixed = resolvedWidth.endsWith('px')
  const widthVal = isFixed ? parseInt(resolvedWidth) : 320
  const offset = side === 'right' ? widthVal + 24 : -(widthVal + 24)

  // Prevent wheel events from propagating to the canvas (React Flow would pan/zoom)
  const boxRef = useRef<HTMLDivElement>(null)
  useEffect(() => {
    const el = boxRef.current
    if (!el) return
    const handler = (e: WheelEvent) => e.stopPropagation()
    el.addEventListener('wheel', handler, { passive: true })
    return () => el.removeEventListener('wheel', handler)
  }, [isOpen])

  return (
    <>
      {hasBackdrop && (
        <AnimatePresence>
          {isOpen && (
            <motion.div
              key={`${panelKey}-backdrop`}
              initial={{ opacity: 0 }}
              animate={{ opacity: 1 }}
              exit={{ opacity: 0 }}
              transition={{ duration: 0.15 }}
              style={{ position: 'fixed', inset: 0, zIndex: zIndex - 1 }}
              onClick={onClose}
            />
          )}
        </AnimatePresence>
      )}
      <AnimatePresence>
        {isOpen && (
          <motion.div
            key={`${panelKey}-panel`}
            initial={{ x: offset, opacity: 0 }}
            animate={{ x: 0, opacity: 1 }}
            exit={{ x: offset, opacity: 0 }}
            transition={{ duration: 0.2, ease: EASE }}
            style={{
              position: 'fixed',
              [side]: '1rem',
              top: 0,
              bottom: 0,
              display: 'flex',
              alignItems: 'center',
              zIndex,
              pointerEvents: 'none',
            }}
          >
            <Box
              ref={boxRef}
              pointerEvents="auto"
              w={width}
              minW={minWidth}
              maxW="calc(100vw - 24px)"
              h={height}
              maxH={maxHeight}
              overflow="hidden"
              display="flex"
              flexDir="column"
              bg="var(--bg-panel)"
              bgImage="var(--grad-panel)"
              backdropFilter="blur(24px)"
              border="1px solid"
              borderColor="whiteAlpha.100"
              borderTop="2px solid"
              borderTopColor="var(--accent)"
              rounded="xl"
              shadow="panel"
            >
              {children}
            </Box>
          </motion.div>
        )}
      </AnimatePresence>
    </>
  )
}
