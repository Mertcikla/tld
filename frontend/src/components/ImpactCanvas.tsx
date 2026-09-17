import { useEffect, useMemo, useRef } from 'react'
import { Box, Center, Spinner, Text } from '@chakra-ui/react'
import { useCrossBranchContextSettings } from '../crossBranch/settings'
import type { ImpactReport } from '../api/client'
import { buildImpactLens, filterExploreData, impactedConnectorIds, impactedElementIds } from '../utils/impactGraph'
import { useExploreData } from '../pages/explore/useExploreData'
import { ZUICanvas, type ZUICanvasHandle } from './ZUI'

interface Props {
  report: ImpactReport
  onOpenElement: (elementId: number) => void
}

export default function ImpactCanvas({ report, onOpenElement }: Props) {
  const zuiRef = useRef<ZUICanvasHandle>(null)
  const { data, loading, error } = useExploreData()
  const { settings: crossBranchSettings } = useCrossBranchContextSettings('zui')

  const elementIds = useMemo(() => impactedElementIds(report), [report])
  const connectorIds = useMemo(() => impactedConnectorIds(report), [report])
  const filtered = useMemo(
    () => (data ? filterExploreData(data, elementIds, connectorIds) : null),
    [data, elementIds, connectorIds],
  )
  const lens = useMemo(() => buildImpactLens(report), [report])

  useEffect(() => {
    if (!filtered || elementIds.size === 0) return
    const timer = setTimeout(() => zuiRef.current?.fitView(), 120)
    return () => clearTimeout(timer)
  }, [filtered, elementIds])

  if (loading) {
    return (
      <Center h={{ base: '460px', lg: '100%' }} minH="240px">
        <Spinner color="var(--accent)" />
      </Center>
    )
  }
  if (error || !filtered || elementIds.size === 0) {
    return (
      <Center h={{ base: '460px', lg: '100%' }} minH="240px">
        <Text fontSize="sm" color="gray.500">
          {elementIds.size === 0 ? 'No mapped elements to draw for this change.' : 'Architecture canvas unavailable.'}
        </Text>
      </Center>
    )
  }

  return (
    <Box
      position="relative"
      h={{ base: '460px', lg: '100%' }}
      minH="240px"
      bg="var(--bg-canvas)"
      overflow="hidden"
    >
      <ZUICanvas
        ref={zuiRef}
        data={filtered}
        diffLens={lens}
        crossBranchSettings={crossBranchSettings}
        onElementActivate={onOpenElement}
      />
      <Box position="absolute" bottom={2} left={3} fontSize="xs" color="gray.500" pointerEvents="none">
        Click a node to open it in Explore
      </Box>
    </Box>
  )
}
