// src/pages/InfiniteZoom.tsx Explore page holds the ZUI feature
import { forwardRef, useCallback, useEffect, useImperativeHandle, useMemo, useRef, useState } from 'react'
import { useLocation, useNavigate, useParams } from 'react-router-dom'
import {
  Box,
  Button,
  Center,
  HStack,
  IconButton,
  Popover,
  PopoverBody,
  PopoverContent,
  PopoverTrigger,
  Portal,
  Spinner,
  Text,
  Tooltip,
  useDisclosure,
  VStack,
} from '@chakra-ui/react'
import { api } from '../api/client'
import type { ExploreData, ViewLayer } from '../types'
import { FitViewIcon as FitViewSvg, TagsIcon, EyeIcon, EyeOffIcon } from '../components/Icons'
import ExploreOnboarding from '../components/ExploreOnboarding'
import ExplorePageOnboarding from '../components/ExplorePageOnboarding'
import MiniZoomOnboarding from '../components/MiniZoomOnboarding'
import { ZUICanvas, type ZUICameraFrame, type ZUICanvasHandle } from '../components/ZUI'
import { useCrossBranchContextSettings } from '../crossBranch/settings'
import CrossBranchControls from '../components/CrossBranchControls'
import { primeWorkspaceGraphSnapshot } from '../crossBranch/store'
import { WATCH_REPRESENTATION_UPDATED_EVENT } from '../components/WorkspacePanel'
import { useWorkspaceVersionPreview } from '../context/WorkspaceVersionContext'

// ── Types ──────────────────────────────────────────────────────────
interface Props {
  sharedToken?: string
  shareSlot?: React.ReactNode
}

export interface InfiniteZoomHandle {
  focusDiagram(viewId: number): boolean
  setCameraFrame(frame: ZUICameraFrame): boolean
}

const MINI_ONBOARDING_KEY = 'shared_zoom_onboarding_dismissed'

// ── Inner component ────────────────────────────────────────────────
function InfiniteZoomInner({ sharedToken, shareSlot }: Props, ref?: React.Ref<InfiniteZoomHandle>) {
  const navigate = useNavigate()
  const location = useLocation()

  const [data, setData] = useState<ExploreData | null>(null)
  const [loading, setLoading] = useState(true)
  const [canvasReady, setCanvasReady] = useState(false)
  const [showMiniOnboarding, setShowMiniOnboarding] = useState(false)
  const [miniOnboardingInteractionSeen, setMiniOnboardingInteractionSeen] = useState(false)
  const [tagColors] = useState<Record<string, import('../types').Tag>>({})
  const [layers, setLayers] = useState<ViewLayer[]>([])
  const [highlightedTags, setHighlightedTags] = useState<string[]>([])
  const [highlightColor, setHighlightColor] = useState('')
  const [hiddenTags, setHiddenTags] = useState<string[]>([])
  const { isOpen: isTagsOpen, onClose: onTagsClose, onToggle: onTagsToggle } = useDisclosure()
  const zuiRef = useRef<ZUICanvasHandle>(null)
  const crossBranchSurface = sharedToken ? 'zui-shared' : 'zui'
  const { settings: crossBranchSettings, setEnabled: setCrossBranchEnabled, setDepth: setCrossBranchDepth } = useCrossBranchContextSettings(crossBranchSurface)
  const { preview: versionPreview } = useWorkspaceVersionPreview()

  const cameraProfile = useMemo(() => new URLSearchParams(location.search).get('profile'), [location.search])
  const isDetailToOverviewProfile = sharedToken && cameraProfile === 'detail-to-overview'

  const initialCameraFrame = useMemo<ZUICameraFrame | undefined>(() => {
    return isDetailToOverviewProfile
      ? { profile: 'detail-to-overview', progress: 0 }
      : undefined
  }, [isDetailToOverviewProfile])

  useImperativeHandle(ref, () => ({
    focusDiagram(viewId: number) {
      return zuiRef.current?.focusDiagram(viewId) ?? false
    },
    setCameraFrame(frame: ZUICameraFrame) {
      return zuiRef.current?.setCameraFrame(frame) ?? false
    },
  }), [])

  // ── No data or No content ────────────────────────────────────────
  const hasPlacements = useMemo(() => {
    if (!data || !data.views) return false
    return Object.values(data.views).some(d => (d && d.placements && d.placements.length > 0))
  }, [data])

  const allTags = useMemo(() => {
    if (!data || !data.views) return []
    const tagSet = new Set<string>()
    Object.values(data.views).forEach(d => {
      (d?.placements ?? []).forEach(p => { (p.tags ?? []).forEach(t => tagSet.add(t)) })
    })
    return Array.from(tagSet).sort()
  }, [data])

  const tagCounts = useMemo(() => {
    if (!data || !data.views) return {} as Record<string, number>
    const counts: Record<string, number> = {}
    Object.values(data.views).forEach(d => {
      (d?.placements ?? []).forEach(p => {
        (p.tags ?? []).forEach(t => { counts[t] = (counts[t] ?? 0) + 1 })
      })
    })
    return counts
  }, [data])

  const layerElementCounts = useMemo(() => {
    if (!data || !data.views) return {} as Record<number, number>
    const counts: Record<number, number> = {}
    for (const layer of layers) {
      let count = 0
      Object.values(data.views).forEach(d => {
        (d?.placements ?? []).forEach(p => {
          if ((p.tags ?? []).some(t => layer.tags.includes(t))) count++
        })
      })
      counts[layer.id] = count
    }
    return counts
  }, [data, layers])

  const toggleLayerVisibility = useCallback((layer: ViewLayer) => {
    if (layer.tags.length === 0) return
    setHiddenTags(prev => {
      const allHidden = layer.tags.every(t => prev.includes(t))
      return allHidden
        ? prev.filter(t => !layer.tags.includes(t))
        : Array.from(new Set([...prev, ...layer.tags]))
    })
  }, [])

  const toggleTagVisibility = useCallback((tag: string) => {
    setHiddenTags(prev => prev.includes(tag) ? prev.filter(t => t !== tag) : [...prev, tag])
  }, [])

  useEffect(() => {
    if (isDetailToOverviewProfile) return
    if (sharedToken && canvasReady && !localStorage.getItem(MINI_ONBOARDING_KEY)) {
      setShowMiniOnboarding(true)
    }
  }, [sharedToken, canvasReady, isDetailToOverviewProfile])

  const dismissMiniOnboarding = useCallback(() => {
    if (showMiniOnboarding) {
      setShowMiniOnboarding(false)
      if (!isDetailToOverviewProfile) {
        localStorage.setItem(MINI_ONBOARDING_KEY, 'true')
      }
    }
  }, [isDetailToOverviewProfile, showMiniOnboarding])

  const showMiniOnboardingAfterCanvasInteraction = useCallback(() => {
    if (!isDetailToOverviewProfile || miniOnboardingInteractionSeen) return
    setMiniOnboardingInteractionSeen(true)
    setShowMiniOnboarding(true)
  }, [isDetailToOverviewProfile, miniOnboardingInteractionSeen])

  const handleCanvasZoom = useCallback(() => {
    setMiniOnboardingInteractionSeen(true)
    dismissMiniOnboarding()
  }, [dismissMiniOnboarding])

  const loadExploreData = useCallback(() => {
    const loader = sharedToken ? api.explore.loadShared(sharedToken) : api.explore.load()
    return loader.then((d) => {
      if (d.password_required) {
        setLoading(false)
      } else {
        primeWorkspaceGraphSnapshot(d)
        setData(d)
        setLoading(false)
      }
    }).catch(() => setLoading(false))
  }, [sharedToken])

  useEffect(() => {
    void loadExploreData()
  }, [loadExploreData])

  useEffect(() => {
    if (sharedToken) return
    const refresh = () => {
      setLoading(true)
      void loadExploreData()
    }
    window.addEventListener(WATCH_REPRESENTATION_UPDATED_EVENT, refresh)
    return () => window.removeEventListener(WATCH_REPRESENTATION_UPDATED_EVENT, refresh)
  }, [loadExploreData, sharedToken])

  // Fetch tag colors and layers once data is loaded (authenticated users only).
  // Only fetch from root tree nodes child/nested diagrams would duplicate the same layers.
  useEffect(() => {
    if (!data || sharedToken) return
    let cancelled = false
    const rootIds = (data.tree ?? []).map(n => n.id)
    const fetchTagData = async () => {
      try {
        const diagramLayers = await Promise.all(
          rootIds.map(id => api.workspace.views.layers.list(id)),
        )
        if (!cancelled) {
          // Deduplicate by layer ID in case of any API overlap
          const seen = new Set<number>()
          const unique = diagramLayers.flat().filter(l => seen.has(l.id) ? false : (seen.add(l.id), true))
          setLayers(unique)
        }
      } catch {
        // intentionally empty: layers are not available for public shared pages
      }
    }
    void fetchTagData()
    return () => { cancelled = true }
  }, [data, sharedToken])

  const handleCanvasReady = useCallback(() => {
    setCanvasReady(true)
  }, [])

  useEffect(() => {
    if (!sharedToken) return

    const handleMessage = (event: MessageEvent) => {
      const data = event.data as { type?: unknown; progress?: unknown; profile?: unknown } | null
      if (!data || data.type !== 'tldiagram-zui-camera') return
      if (data.profile !== 'detail-to-overview') return

      const progress = Number(data.progress)
      if (!Number.isFinite(progress)) return

      zuiRef.current?.setCameraFrame({ profile: 'detail-to-overview', progress })
    }

    window.addEventListener('message', handleMessage)
    return () => window.removeEventListener('message', handleMessage)
  }, [sharedToken])

  if (!loading && (!data || (data.tree ?? []).length === 0 || !hasPlacements)) {
    const noDiagrams = !data || (data.tree ?? []).length === 0
    return (
      <Center h="100%" flexDir="column" gap={4} px={6} textAlign="center">
        <VStack spacing={2}>
          <Text color="gray.300" fontWeight="bold" fontSize="lg">
            {noDiagrams ? 'No diagrams to explore yet' : 'Your diagrams are empty'}
          </Text>
          <Text color="gray.500" fontSize="sm" maxW="400px">
            {noDiagrams 
              ? 'Start by creating your first diagram in the workspace.' 
              : 'Add elements to your diagrams in the editor to see them rendered on this infinite canvas.'}
          </Text>
        </VStack>
        
        {!sharedToken && (
          <Button size="sm" colorScheme="blue" onClick={() => navigate('/views')} borderRadius="full" px={6}>
            {noDiagrams ? 'Create First Diagram' : 'Go to Editor'}
          </Button>
        )}
        {!noDiagrams && !sharedToken && <ExplorePageOnboarding hasDiagrams={!noDiagrams} />}
      </Center>
    )
  }

  // ── Main view with loading overlay ────────────────────────────────
  const showContent = !loading && !!data && canvasReady

  return (
    <Box position="relative" w="full" h="full" overflow="hidden">
      {/* Loading overlay - stays until data and canvas are ready */}
      {(!loading && data && !canvasReady) || loading ? (
        <Center
          position="absolute"
          top={0} left={0} right={0} bottom={0}
          zIndex={100}
          bg="var(--bg-primary)"
        >
          <Spinner size="xl" color="var(--accent)" />
        </Center>
      ) : null}

      {data && (
        <>
          <ZUICanvas
            ref={zuiRef}
            data={data}
            onReady={handleCanvasReady}
            onZoom={handleCanvasZoom}
            onPan={showMiniOnboardingAfterCanvasInteraction}
            initialCameraFrame={initialCameraFrame}
            highlightedTags={highlightedTags}
            highlightColor={highlightColor}
            hiddenTags={hiddenTags}
            versionPreview={versionPreview}
            crossBranchSettings={crossBranchSettings}
            hoverLocked={isTagsOpen}
          />

          {/* Onboarding overlay */}
          {data && !sharedToken && <ExploreOnboarding hasLinkedNodes={!!(data.navigations?.length > 0)} />}
          <MiniZoomOnboarding isVisible={showMiniOnboarding} onClose={dismissMiniOnboarding} />

          {/* Bottom toolbar */}
          <Box
            position="absolute"
            bottom={4}
            left="50%"
            transform="translateX(-50%)"
            zIndex={10}
            className="glass"
            borderRadius="lg"
            px={2}
            py={1}
            opacity={showContent ? 1 : 0}
            transition="opacity 0.3s"
          >
            <HStack spacing={0}>
              <Tooltip label="Fit View" placement="top" openDelay={200}>
                <Button
                  variant="ghost" h="28px" px={2.5}
                  color="gray.300"
                  _hover={{ bg: 'rgba(var(--accent-rgb), 0.12)', color: 'var(--accent)' }}
                  onClick={() => zuiRef.current?.fitView()}
                >
                  <HStack spacing={1.5}>
                    <FitViewSvg />
                    <Text fontSize="11px" fontWeight="normal">Fit View</Text>
                  </HStack>
                </Button>
              </Tooltip>

              {shareSlot}

              <Box w="1px" h="16px" bg="whiteAlpha.100" flexShrink={0} mx={0.5} />
              <CrossBranchControls
                settings={crossBranchSettings}
                onEnabledChange={setCrossBranchEnabled}
                onDepthChange={setCrossBranchDepth}
                label="Branches"
              />

              {(allTags.length > 0 || layers.length > 0) && (
                <>
                  <Box w="1px" h="16px" bg="whiteAlpha.100" flexShrink={0} mx={0.5} />
                  <Popover
                    isOpen={isTagsOpen}
                    onClose={() => { onTagsClose(); setHighlightedTags([]); setHighlightColor('') }}
                    placement="top"
                    isLazy
                    closeOnBlur
                  >
                    <PopoverTrigger>
                      <Button
                        variant="ghost" h="28px" px={2.5}
                        color={isTagsOpen ? 'var(--accent)' : 'gray.300'}
                        _hover={{ bg: 'rgba(var(--accent-rgb), 0.12)', color: 'var(--accent)' }}
                        onClick={onTagsToggle}
                      >
                        <HStack spacing={1.5}>
                          <TagsIcon />
                          <Text fontSize="11px" fontWeight="normal">Tags</Text>
                        </HStack>
                      </Button>
                    </PopoverTrigger>
                    <Portal>
                      <PopoverContent
                        data-zui-native-wheel="true"
                        bg="glass.bg"
                        backdropFilter="blur(16px)"
                        borderColor="glass.border"
                        boxShadow="panel"
                        borderRadius="lg"
                        width="220px"
                        _focus={{ boxShadow: 'none' }}
                        onMouseLeave={() => { setHighlightedTags([]); setHighlightColor('') }}
                      >
                        <PopoverBody p={2} maxH="360px" overflowY="auto">
                          {layers.map(layer => {
                            const isHidden = layer.tags.length > 0 && layer.tags.every(t => hiddenTags.includes(t))
                            return (
                              <HStack
                                key={`layer-${layer.id}`}
                                px={2}
                                py={1}
                                spacing={2}
                                borderRadius="md"
                                _hover={{ bg: 'whiteAlpha.100' }}
                                onMouseEnter={() => { setHighlightedTags(layer.tags); setHighlightColor(layer.color || '') }}
                                opacity={isHidden ? 0.4 : 1}
                                transition="opacity 0.15s"
                              >
                                <Box w="10px" h="10px" rounded="full" bg={layer.color || 'gray.500'} flexShrink={0} />
                                <Text fontSize="xs" fontWeight="600" color="white" flex={1} isTruncated>
                                  {layer.name}
                                </Text>
                                <Text fontSize="10px" color="gray.600" flexShrink={0}>
                                  {layerElementCounts[layer.id] ?? 0}
                                </Text>
                                <IconButton
                                  aria-label={isHidden ? 'Show layer' : 'Hide layer'}
                                  icon={isHidden ? <EyeOffIcon size={12} /> : <EyeIcon size={12} />}
                                  size="xs"
                                  variant="ghost"
                                  color={isHidden ? 'whiteAlpha.300' : 'whiteAlpha.600'}
                                  _hover={{ color: 'white', bg: 'whiteAlpha.200' }}
                                  onClick={(e) => { e.stopPropagation(); toggleLayerVisibility(layer) }}
                                  flexShrink={0}
                                />
                              </HStack>
                            )
                          })}

                          {allTags.map(tag => {
                            const isHidden = hiddenTags.includes(tag)
                            return (
                              <HStack
                                key={`tag-${tag}`}
                                px={2}
                                py={1}
                                spacing={2}
                                borderRadius="md"
                                onMouseEnter={() => { setHighlightedTags([tag]); setHighlightColor(tagColors[tag]?.color || '') }}
                                opacity={isHidden ? 0.4 : 1}
                                transition="opacity 0.15s"
                              >
                                <Box w="8px" h="8px" rounded="full" bg={tagColors[tag]?.color || '#A0AEC0'} flexShrink={0} />
                                <Text fontSize="xs" fontWeight="600" color="gray.300" flex={1} isTruncated>
                                  {tag}
                                </Text>
                                <Text fontSize="10px" color="gray.600" flexShrink={0}>
                                  {tagCounts[tag] ?? 0}
                                </Text>
                                <IconButton
                                  aria-label={isHidden ? 'Show tag' : 'Hide tag'}
                                  icon={isHidden ? <EyeOffIcon size={12} /> : <EyeIcon size={12} />}
                                  size="xs"
                                  variant="ghost"
                                  color={isHidden ? 'whiteAlpha.300' : 'whiteAlpha.600'}
                                  _hover={{ color: 'white', bg: 'whiteAlpha.200' }}
                                  onClick={(e) => { e.stopPropagation(); toggleTagVisibility(tag) }}
                                  flexShrink={0}
                                />
                              </HStack>
                            )
                          })}
                        </PopoverBody>
                      </PopoverContent>
                    </Portal>
                  </Popover>
                </>
              )}
            </HStack>
          </Box>
    </>
  )}
</Box>
  )
}

// ── Exports ───────────────────────────────────────────────────────

const InfiniteZoom = forwardRef<InfiniteZoomHandle, Props>(InfiniteZoomInner)
export default InfiniteZoom

export const SharedInfiniteZoom = forwardRef<InfiniteZoomHandle, Props>((props, ref) => {
  const { token } = useParams()
  const effectiveToken = props.sharedToken ?? token
  return <InfiniteZoom {...props} ref={ref} sharedToken={effectiveToken} />
})
