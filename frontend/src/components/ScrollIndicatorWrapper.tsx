import { Box, type BoxProps } from '@chakra-ui/react'
import { useCallback, useEffect, useRef, useState, type ReactNode, forwardRef, useImperativeHandle } from 'react'

interface ScrollIndicatorWrapperProps extends BoxProps {
  children: ReactNode
  scrollId?: string
}

const ScrollIndicatorWrapper = forwardRef<HTMLDivElement, ScrollIndicatorWrapperProps>(({
  children,
  scrollId,
  onScroll,
  ...props
}, ref) => {
  const [scrollIndicators, setScrollIndicators] = useState({ showTop: false, showBottom: false })
  const internalRef = useRef<HTMLDivElement>(null)

  // Use the provided ref if any, otherwise use our internal ref
  useImperativeHandle(ref, () => internalRef.current as HTMLDivElement)

  const updateScrollIndicators = useCallback(() => {
    const el = internalRef.current
    if (!el) return

    const { scrollTop, scrollHeight, clientHeight } = el
    const isScrollable = scrollHeight > clientHeight
    const isAtBottom = Math.abs(scrollHeight - clientHeight - scrollTop) < 1.5
    const isAtTop = scrollTop < 1.5

    const nextShowTop = isScrollable && !isAtTop
    const nextShowBottom = isScrollable && !isAtBottom

    setScrollIndicators((prev) => (
      prev.showTop === nextShowTop && prev.showBottom === nextShowBottom
        ? prev
        : { showTop: nextShowTop, showBottom: nextShowBottom }
    ))
  }, [])

  useEffect(() => {
    const el = internalRef.current
    if (!el) return

    // Initial check
    updateScrollIndicators()

    // Observe size changes (e.g. content loading, filtering)
    const observer = new ResizeObserver(updateScrollIndicators)
    observer.observe(el)
    if (el.firstElementChild) {
      observer.observe(el.firstElementChild)
    }

    return () => observer.disconnect()
  }, [updateScrollIndicators])

  return (
    <Box position="relative" flex={1} minH={0} overflow="hidden" display="flex" flexDir="column">
      {/* Top Scroll indicator shadow */}
      <Box
        position="absolute"
        top={0}
        left={0}
        right={0}
        h="24px"
        bgGradient="linear(to-b, rgba(0,0,0,0.1), transparent)"
        pointerEvents="none"
        transition="opacity 0.2s ease"
        opacity={scrollIndicators.showTop ? 1 : 0}
        zIndex={2}
      />

      <Box
        ref={internalRef}
        id={scrollId}
        overflowY="auto"
        minH={0}
        onScroll={(e) => {
          updateScrollIndicators()
          onScroll?.(e)
        }}
        css={{
          '&::-webkit-scrollbar': { width: '4px' },
          '&::-webkit-scrollbar-track': { background: 'transparent' },
          '&::-webkit-scrollbar-thumb': {
            background: 'rgba(255,255,255,0.1)',
            borderRadius: '10px',
          },
          '&::-webkit-scrollbar-thumb:hover': {
            background: 'rgba(255,255,255,0.2)',
          },
        }}
        {...props}
      >
        {children}
      </Box>

      {/* Bottom Scroll indicator shadow */}
      <Box
        position="absolute"
        bottom={0}
        left={0}
        right={0}
        h="40px"
        bgGradient="linear(to-t, rgba(0,0,0,0.15), transparent)"
        pointerEvents="none"
        transition="opacity 0.2s ease"
        opacity={scrollIndicators.showBottom ? 1 : 0}
        zIndex={2}
      />
    </Box>
  )
})

ScrollIndicatorWrapper.displayName = 'ScrollIndicatorWrapper'

export default ScrollIndicatorWrapper
