import { AnimatePresence, motion } from 'framer-motion'
import { Box, FocusLock, useBreakpointValue } from '@chakra-ui/react'
import { type ReactNode, useRef, useEffect } from 'react'
import { useViewEditorContext } from '../pages/ViewEditor/context'
import { floatingSurfaceStyles, panelSurfaceStyles } from '../styles/surfaces'

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
  autoFocus?: boolean
  noFocusLock?: boolean
  'data-testid'?: string
  isInline?: boolean
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
  autoFocus = false,
  noFocusLock = false,
  'data-testid': dataTestId,
  isInline = false,
}: Props) {
  // Prevent wheel events from propagating to the canvas (React Flow would pan/zoom)
  const boxRef = useRef<HTMLDivElement>(null)
  useEffect(() => {
    const el = boxRef.current
    if (!el) return
    const handler = (e: WheelEvent) => e.stopPropagation()
    el.addEventListener('wheel', handler, { passive: true })
    return () => el.removeEventListener('wheel', handler)
  }, [isOpen])

  const context = useViewEditorContext()
  const isMarkdownOpen = context?.isMarkdownOpen ?? false
  const markdownPaneWidth = context?.markdownPaneWidth ?? 0
  const isFlat = context?.sidePanelStyle === 'flat'

  const isMobileLayout = useBreakpointValue({ base: true, md: false }) ?? false
  const rightOffset = (!isMobileLayout && isMarkdownOpen && side === 'right')
    ? `${markdownPaneWidth + 10}px`
    : '0px'
  const focusLockDisabled = !isOpen || noFocusLock || !hasBackdrop

  if (isInline) {
    return (
      <Box
        data-testid={dataTestId}
        ref={boxRef}
        position="relative"
        w="full"
        h="full"
        overflow="hidden"
        display="flex"
        flexDir="column"
        sx={panelSurfaceStyles}
      >
        <Box display="flex" flexDirection="column" flex="1 1 auto" minHeight={0} height="100%">
          {children}
        </Box>
      </Box>
    )
  }

  // Use width if it's a fixed value, otherwise default to a safe offscreen distance
  const resolvedWidth = typeof width === 'string' ? width : '320px'
  const isFixed = resolvedWidth.endsWith('px')
  const widthVal = isFixed ? parseInt(resolvedWidth) : 320
  const offset = side === 'right' ? widthVal + 24 : -(widthVal + 24)

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
              position: isFlat ? 'absolute' : 'fixed',
              left: side === 'left' ? (isFlat ? 0 : '1rem') : undefined,
              right: side === 'right' ? (isFlat ? rightOffset : `calc(1rem + ${rightOffset})`) : undefined,
              top: 0,
              bottom: 0,
              display: 'flex',
              alignItems: isFlat ? 'stretch' : 'center',
              zIndex,
              pointerEvents: 'none',
            }}
          >
            <Box
              data-testid={dataTestId}
              ref={boxRef}
              position="relative"
              pointerEvents="auto"
              w={width}
              minW={minWidth}
              maxW={isFlat ? '100%' : 'calc(100vw - 24px)'}
              h={isFlat ? '100%' : height}
              maxH={isFlat ? '100%' : maxHeight}
              overflow="hidden"
              display="flex"
              flexDir="column"
              border={isFlat ? 'none' : '1px solid'}
              borderLeft={isFlat && side === 'right' ? '1px solid' : undefined}
              borderRight={isFlat && side === 'left' ? '1px solid' : undefined}
              rounded={isFlat ? 'none' : 'xl'}
              shadow={isFlat ? 'none' : 'panel'}
              sx={{
                ...(isFlat ? panelSurfaceStyles : floatingSurfaceStyles),
                // Keep flat panels attached to their layout without floating geometry.
                ...(isFlat ? { borderRadius: 0, boxShadow: 'none' } : {}),
                '& > div:not([data-focus-guard])': {
                  display: 'flex',
                  flexDirection: 'column',
                  flex: '1 1 auto',
                  minHeight: 0,
                  height: '100%',
                },
              }}
            >
              <FocusLock
                isDisabled={focusLockDisabled}
                autoFocus={autoFocus}
                restoreFocus
              >
                {children}
              </FocusLock>
            </Box>
          </motion.div>
        )}
      </AnimatePresence>
    </>
  )
}
