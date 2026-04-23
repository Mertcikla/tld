import { useState, useEffect, useCallback } from 'react'
import { useSearchParams } from 'react-router-dom'
import { Box, Flex, Button, Text, Spinner, Center } from '@chakra-ui/react'
import { motion, AnimatePresence } from 'framer-motion'
import ViewsGrid from './ViewsGrid'
import InfiniteZoom from './InfiniteZoom'
import { api } from '../api/client'

interface Props {
  shareSlot?: React.ReactNode
  onShareView?: (viewId: number) => void
}

type ViewType = 'explore' | 'hierarchy'

const MotionBox = motion.create(Box)

export default function ViewsPage({ shareSlot, onShareView }: Props) {
  const [searchParams, setSearchParams] = useSearchParams()
  const requestedView = searchParams.get('view')
  const initialView: ViewType = requestedView === 'edit' ? 'hierarchy' : ((requestedView as ViewType) || 'explore')
  const [view, setView] = useState<ViewType>(initialView)
  const [initializing, setInitializing] = useState(true)

  const handleViewChange = useCallback((newView: ViewType) => {
    setView(newView)
    const newParams = new URLSearchParams(searchParams)
    newParams.set('view', newView)
    setSearchParams(newParams, { replace: true })
  }, [searchParams, setSearchParams])

  // Sync state with search params
  useEffect(() => {
    const v = searchParams.get('view')
    if (v === 'explore' || v === 'hierarchy') {
      setView(v)
    }
    if (v === 'edit') {
      setView('hierarchy')
    }
  }, [searchParams])

  useEffect(() => {
    let mounted = true
    api.workspace.views.tree()
      .then((tree) => {
        if (!mounted) return
        if (!tree || tree.length === 0) {
          // Only auto-switch to edit if no view is explicitly set in URL
          if (!searchParams.get('view')) {
            handleViewChange('hierarchy')
          }
        }
      })
      .catch(() => {
        // Fallback to explore on error
      })
      .finally(() => {
        if (mounted) setInitializing(false)
      })
    return () => { mounted = false }
  }, [searchParams, handleViewChange])

  // Colors for the switch
  const bgColor = 'var(--bg)'
  const activeColor = 'clay.bg'
  const inactiveColor = 'gray.400'

  if (initializing) {
    return (
      <Center h="full">
        <Spinner size="xl" color="var(--accent)" />
      </Center>
    )
  }

  return (
    <Box position="relative" w="full" h="full" overflow="hidden">
      {/* Floating Switch */}
      <Flex
        position="absolute"
        top={4}
        left="50%"
        transform="translateX(-50%)"
        zIndex={1000}
        bg={bgColor}
        backdropFilter="blur(12px)"
        border="1px solid"
        borderColor="whiteAlpha.100"
        borderRadius="full"
        p={1}
        gap={1}
        boxShadow="0 4px 20px rgba(0, 0, 0, 0.4)"
      >
        <Button
          size="sm"
          variant="ghost"
          borderRadius="full"
          position="relative"
          px={6}
          h="32px"
          onClick={() => handleViewChange('explore')}
          _hover={{ bg: 'transparent' }}
          _active={{ bg: 'transparent' }}
          color={view === 'explore' ? 'white' : inactiveColor}
          transition="color 0.2s"
        >
          {view === 'explore' && (
            <MotionBox
              layoutId="active-pill"
              position="absolute"
              inset={0}
              bg={activeColor}
              borderRadius="full"
              zIndex={-1}
              transition={{ type: 'spring', bounce: 0.2, duration: 0.6 }}
            />
          )}
          <Text fontSize="xs" fontWeight="bold" zIndex={1}>Explore</Text>
        </Button>
        <Button
          size="sm"
          variant="ghost"
          borderRadius="full"
          position="relative"
          px={6}
          h="32px"
          onClick={() => handleViewChange('hierarchy')}
          _hover={{ bg: 'transparent' }}
          _active={{
            bg: 'transparent'
          }}
          color={view === 'hierarchy' ? 'white' : inactiveColor}
          transition="color 0.2s"
        >
          {view === 'hierarchy' && (
            <MotionBox
              layoutId="active-pill"
              position="absolute"
              inset={0}
              bg={activeColor}
              borderRadius="full"
              zIndex={-1}
              transition={{ type: 'spring', bounce: 0.2, duration: 0.6 }}
            />
          )}
          <Text fontSize="xs" fontWeight="bold" zIndex={1}>Hierarchy</Text>
        </Button>
      </Flex>

      {/* Page Content */}
      <AnimatePresence mode="wait">
        <MotionBox
          key={view}
          initial={{ opacity: 0, scale: 0.98 }}
          animate={{ opacity: 1, scale: 1 }}
          exit={{ opacity: 0, scale: 1.02 }}
          transition={{ duration: 0.3, ease: 'easeInOut' }}
          w="full"
          h="full"
        >
          {view === 'explore' ? (
            <InfiniteZoom shareSlot={shareSlot} />
          ) : (
            <ViewsGrid onShare={onShareView} />
          )}
        </MotionBox>
      </AnimatePresence>
    </Box>
  )
}
