import { forwardRef, useCallback, useEffect, useImperativeHandle, useMemo, useRef, useState } from 'react'
import type { ReactNode, Ref } from 'react'
import { useLocation, useNavigate, useParams } from 'react-router-dom'
import { Box, Center, Spinner, useDisclosure } from '@chakra-ui/react'
import ExploreOnboarding from '../components/ExploreOnboarding'
import MiniZoomOnboarding from '../components/MiniZoomOnboarding'
import { ZUICanvas, type ZUICameraFrame, type ZUICanvasHandle } from '../components/ZUI'
import { useCrossBranchContextSettings } from '../crossBranch/settings'
import { useWorkspaceVersionPreview } from '../context/WorkspaceVersionContext'
import { ExploreDiffPanel, ExploreEmptyState, ExploreToolbar, ExploreUnplacedDiffPanel } from './explore/ExploreComponents'
import { useExploreData } from './explore/useExploreData'
import { useExploreDiffMode } from './explore/useExploreDiffMode'
import { useExploreTags } from './explore/useExploreTags'

interface Props {
  sharedToken?: string
  shareSlot?: ReactNode
}

export interface InfiniteZoomHandle {
  focusDiagram(viewId: number): boolean
  focusElement(viewId: number, elementId: number): boolean
  setCameraFrame(frame: ZUICameraFrame): boolean
}

const MINI_ONBOARDING_KEY = 'shared_zoom_onboarding_dismissed'

function ExplorePage({ sharedToken, shareSlot }: Props, ref?: Ref<InfiniteZoomHandle>) {
  const navigate = useNavigate()
  const location = useLocation()
  const zuiRef = useRef<ZUICanvasHandle>(null)
  const [canvasReady, setCanvasReady] = useState(false)
  const [showMiniOnboarding, setShowMiniOnboarding] = useState(false)
  const [miniOnboardingInteractionSeen, setMiniOnboardingInteractionSeen] = useState(false)
  const { isOpen: isTagsOpen, onClose: onTagsClose, onToggle: onTagsToggle } = useDisclosure()
  const [isCrossBranchControlsOpen, setIsCrossBranchControlsOpen] = useState(false)
  const dataState = useExploreData(sharedToken)
  const { data, loading, error, hasPlacements, reload } = dataState
  const tags = useExploreTags(data, sharedToken)
  const crossBranchSurface = sharedToken ? 'zui-shared' : 'zui'
  const {
    settings: crossBranchSettings,
    setEnabled: setCrossBranchEnabled,
    setConnectorBudget: setCrossBranchConnectorBudget,
    setConnectorPriority: setCrossBranchConnectorPriority,
  } = useCrossBranchContextSettings(crossBranchSurface)
  const { preview: versionPreview, followTarget: versionFollowTarget } = useWorkspaceVersionPreview()
  const diffMode = useExploreDiffMode({
    data,
    sharedToken,
    location,
    navigate,
    canvasReady,
    zuiRef,
  })

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
    focusElement(viewId: number, elementId: number) {
      return zuiRef.current?.focusElement(viewId, elementId) ?? false
    },
    setCameraFrame(frame: ZUICameraFrame) {
      return zuiRef.current?.setCameraFrame(frame) ?? false
    },
  }), [])

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

  useEffect(() => {
    if (!sharedToken) return

    const handleMessage = (event: MessageEvent) => {
      const payload = event.data as { type?: unknown; progress?: unknown; profile?: unknown } | null
      if (!payload || payload.type !== 'tldiagram-zui-camera') return
      if (payload.profile !== 'detail-to-overview') return

      const progress = Number(payload.progress)
      if (!Number.isFinite(progress)) return

      zuiRef.current?.setCameraFrame({ profile: 'detail-to-overview', progress })
    }

    window.addEventListener('message', handleMessage)
    return () => window.removeEventListener('message', handleMessage)
  }, [sharedToken])

  const noDiagrams = !data || (data.tree ?? []).length === 0
  const goToEditor = useCallback(() => {
    const root = (data?.tree ?? []).find((node) => node.parent_view_id === null) ?? data?.tree?.[0]
    navigate(root ? `/views/${root.id}` : '/views?view=edit')
  }, [data, navigate])
  if (!loading && (error || noDiagrams || !hasPlacements)) {
    return (
      <ExploreEmptyState
        noDiagrams={noDiagrams}
        sharedToken={sharedToken}
        error={error}
        onRetry={reload}
        onGoToViews={goToEditor}
      />
    )
  }

  const showContent = !loading && !!data && canvasReady

  return (
    <Box position="relative" w="full" h="full" overflow="hidden">
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
            onReady={() => setCanvasReady(true)}
            onZoom={handleCanvasZoom}
            onPan={showMiniOnboardingAfterCanvasInteraction}
            initialCameraFrame={initialCameraFrame}
            highlightedTags={tags.highlightedTags}
            highlightColor={tags.highlightColor}
            hiddenTags={tags.hiddenTags}
            groupLayers={tags.layers}
            versionPreview={versionPreview}
            versionFollowTarget={versionFollowTarget}
            diffLens={diffMode.diffLens}
            crossBranchSettings={crossBranchSettings}
            hoverLocked={isTagsOpen || isCrossBranchControlsOpen}
          />

          {!sharedToken && <ExploreOnboarding hasLinkedNodes={!!(data.navigations?.length > 0)} />}
          <MiniZoomOnboarding isVisible={showMiniOnboarding} onClose={dismissMiniOnboarding} />

          {diffMode.diffVersionId > 0 && (
            <ExploreDiffPanel
              diffLens={diffMode.diffLens}
              diffLoading={diffMode.diffLoading}
              activeDiffTarget={diffMode.activeDiffTarget}
              activeDiffTargetIndex={diffMode.activeDiffTargetIndex}
              showContent={showContent}
              onExit={diffMode.exitDiffMode}
              onNavigate={diffMode.navigateDiffTarget}
            />
          )}

          {diffMode.diffLens && (
            <ExploreUnplacedDiffPanel
              diffLens={diffMode.diffLens}
              onOpenDiffSource={diffMode.openDiffSource}
            />
          )}

          <ExploreToolbar
            showContent={showContent}
            shareSlot={shareSlot}
            crossBranchSettings={crossBranchSettings}
            onCrossBranchEnabledChange={setCrossBranchEnabled}
            onCrossBranchBudgetChange={setCrossBranchConnectorBudget}
            onCrossBranchPriorityChange={setCrossBranchConnectorPriority}
            onCrossBranchOpenChange={setIsCrossBranchControlsOpen}
            allTags={tags.allTags}
            tagColors={tags.tagColors}
            layers={tags.layers}
            layerElementCounts={tags.layerElementCounts}
            tagCounts={tags.tagCounts}
            hiddenTags={tags.hiddenTags}
            isTagsOpen={isTagsOpen}
            onTagsClose={onTagsClose}
            onTagsToggle={onTagsToggle}
            setHighlightedTags={tags.setHighlightedTags}
            setHighlightColor={tags.setHighlightColor}
            toggleLayerVisibility={tags.toggleLayerVisibility}
            toggleTagVisibility={tags.toggleTagVisibility}
            onFitView={() => zuiRef.current?.fitView()}
          />
        </>
      )}
    </Box>
  )
}

const InfiniteZoom = forwardRef<InfiniteZoomHandle, Props>(ExplorePage)
export default InfiniteZoom

export const SharedInfiniteZoom = forwardRef<InfiniteZoomHandle, Props>((props, ref) => {
  const { token } = useParams()
  const effectiveToken = props.sharedToken ?? token
  return <InfiniteZoom {...props} ref={ref} sharedToken={effectiveToken} />
})
