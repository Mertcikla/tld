import { useEffect, useMemo, useRef, useState, type ReactElement, type ReactNode } from 'react'
import { Box, Button, Checkbox, FormControl, FormLabel, HStack, IconButton, Input, Select, Tag, TagCloseButton, TagLabel, Text, Textarea, Tooltip, VStack } from '@chakra-ui/react'
import { useQuery, useQueryClient } from '@tanstack/react-query'
import { CopyIcon } from '@chakra-ui/icons'
import { CloseIcon, EditorIcon, ReloadIcon, SaveIcon, TrashIcon } from '../components/Icons'
import { flowApi } from './api'
import { duplicateSteps, missingReference, newStep, resolveFocus, stepLabel, type FlowDiagram, type FlowFocus, type FlowStep, type StoryFlow } from './model'
import { useFlowPlayback } from './useFlowPlayback'
import { FlowYamlControls } from './FlowYamlControls'
import { addCanvasHighlight, captureCanvasStep, type FlowCanvasRecorder } from './canvasInteraction'

const emptyDiagram: FlowDiagram = { elements: [], connectors: [] }
function FlowAction({ label, hint = label, icon, disabled, loading, primary, destructive, modeSwitch, onClick }: {
  label: string; hint?: string; icon: ReactElement; disabled?: boolean; loading?: boolean; primary?: boolean; destructive?: boolean; modeSwitch?: boolean; onClick: () => void
}) {
  return <Tooltip label={hint} openDelay={200} closeOnPointerDown closeOnScroll>
    <Box as="span" display="inline-flex" ml={modeSwitch ? 1 : undefined}>
      <IconButton aria-label={label} icon={icon} size={modeSwitch ? 'sm' : 'xs'} w={modeSwitch ? undefined : 7} h={modeSwitch ? undefined : 7} variant={modeSwitch ? 'elevated' : primary ? 'solid' : 'ghost'} colorScheme={destructive ? 'red' : primary ? 'blue' : 'gray'} isDisabled={disabled} isLoading={loading} onClick={onClick} />
    </Box>
  </Tooltip>
}

export interface FlowPanelProps {
  open: boolean
  viewId?: number
  editMode?: boolean
  diagram?: FlowDiagram
  diagrams?: Record<string, FlowDiagram>
  selection?: { elementId?: number; connectorId?: number }
  docked?: boolean
  compact?: boolean
  expanded?: boolean
  onExpandedChange?: (expanded: boolean) => void
  onCanvasRecorderChange?: (recorder: FlowCanvasRecorder | null) => void
  onEditModeChange?: (editing: boolean) => void
  onClose: () => void
  onFocus: (focus: FlowFocus | null) => void
}

function StepHighlights({ step, diagram, onChange, onPick, picking }: { onPick?: () => void; picking?: boolean; step: FlowStep; diagram: FlowDiagram; onChange: (changes: Partial<FlowStep>) => void }) {
  const [search, setSearch] = useState('')
  const [tab, setTab] = useState<'elements' | 'connections'>(step.connectors.length && !step.elementIds.length ? 'connections' : 'elements')
  const term = search.trim().toLocaleLowerCase()
  const elements = term ? diagram.elements.filter(element => element.name.toLocaleLowerCase().includes(term)) : []
  const connectors = diagram.connectors.map(connector => {
    const endpoints = `${diagram.elements.find(e => e.element_id === connector.source_element_id)?.name ?? connector.source_element_id} → ${diagram.elements.find(e => e.element_id === connector.target_element_id)?.name ?? connector.target_element_id}`
    const label = connector.label?.trim()
    return { ...connector, name: label || endpoints, details: label ? `${endpoints} · ${label}` : endpoints }
  })
  const matchingConnectors = term ? connectors.filter(connector => connector.details.toLocaleLowerCase().includes(term)) : []
  return <Box border="1px solid" borderColor="whiteAlpha.200" rounded="md" overflow="hidden">
    {onPick && <Box px={2} pt={2}><Button size="sm" w="full" variant="solid" colorScheme="blue" aria-pressed={!!picking} onClick={onPick}>{picking ? 'Done picking' : 'Pick from canvas'}</Button></Box>}
    <HStack p={2} spacing={1} aria-label="Highlight type">
      <Button flex={1} size="sm" variant={tab === 'elements' ? 'solid' : 'ghost'} aria-label="Show elements" aria-pressed={tab === 'elements'} onClick={() => { setTab('elements'); setSearch('') }}>Elements ({step.elementIds.length})</Button>
      <Button flex={1} size="sm" variant={tab === 'connections' ? 'solid' : 'ghost'} aria-label="Show connections" aria-pressed={tab === 'connections'} onClick={() => { setTab('connections'); setSearch('') }}>Connections ({step.connectors.length})</Button>
    </HStack>
    <Box px={2} pb={2}><Input size="sm" aria-label="Search highlights" placeholder={tab === 'elements' ? 'Search elements…' : 'Search connections…'} value={search} onChange={event => setSearch(event.target.value)} /></Box>
    {(tab === 'elements' ? step.elementIds.length > 0 : step.connectors.length > 0) && <HStack px={2} pb={2} spacing={1} gap={1} flexWrap="wrap" aria-label={tab === 'elements' ? 'Selected elements' : 'Selected connections'}>
      {tab === 'elements' ? step.elementIds.map(id => {
        const name = diagram.elements.find(element => element.element_id === id)?.name ?? `Unavailable element #${id}`
        return <Tag key={id} size="sm" maxW="full">
          <TagLabel isTruncated title={name}>{name}</TagLabel>
          <TagCloseButton aria-label={`Remove element ${name}`} onClick={() => onChange({ elementIds: step.elementIds.filter(value => value !== id) })} />
        </Tag>
      }) : step.connectors.map(highlight => {
        const connector = connectors.find(connector => connector.id === highlight.id)
        const name = connector?.name ?? `Unavailable connection #${highlight.id}`
        return <Tag key={highlight.id} size="sm" maxW="full">
          <TagLabel isTruncated title={connector?.details ?? name}>{name}</TagLabel>
          <Button size="xs" variant="ghost" minW="24px" px={1} h="24px" aria-label={`Reverse ${connector?.details ?? name}`} aria-pressed={highlight.reverse} title="Reverse direction" onClick={() => onChange({ connectors: step.connectors.map(c => c.id === highlight.id ? { ...c, reverse: !c.reverse } : c) })}>{highlight.reverse ? '←' : '→'}</Button>
          <TagCloseButton aria-label={`Remove connection ${connector?.details ?? name}`} onClick={() => onChange({ connectors: step.connectors.filter(value => value.id !== highlight.id) })} />
        </Tag>
      })}
    </HStack>}
    {term && <VStack data-testid="highlight-search-results" align="stretch" spacing={0} maxH="240px" overflowY="auto" overscrollBehavior="contain" px={2} pb={2}>
      {tab === 'elements' ? <>
        {elements.slice(0, 5).map(element => <Checkbox key={element.element_id} size="sm" py={2} alignItems="flex-start" isChecked={step.elementIds.includes(element.element_id)} onChange={e => onChange({ elementIds: e.target.checked ? [...step.elementIds, element.element_id] : step.elementIds.filter(id => id !== element.element_id) })}>{element.name}</Checkbox>)}
        {!elements.length && <Text fontSize="sm" color="gray.400" py={2}>No matching elements.</Text>}
      </> : <>
        {matchingConnectors.slice(0, 5).map(connector => <Box key={connector.id} py={2}>
          {/* Checkbox forwards its ref to the hidden input; use a DOM wrapper for pointer exit. */}
          <Tooltip label={connector.details} openDelay={300} placement="right" hasArrow closeOnPointerDown closeOnScroll>
            <Box as="span" display="block">
              <Checkbox size="sm" w="full" aria-label={connector.details} sx={{ '.chakra-checkbox__label': { minWidth: 0, overflow: 'hidden', textOverflow: 'ellipsis', whiteSpace: 'nowrap' } }} isChecked={step.connectors.some(highlight => highlight.id === connector.id)} onChange={e => onChange({ connectors: e.target.checked ? [...step.connectors, { id: connector.id, reverse: false }] : step.connectors.filter(c => c.id !== connector.id) })}>{connector.name}</Checkbox>
            </Box>
          </Tooltip>
        </Box>)}
        {!matchingConnectors.length && <Text fontSize="sm" color="gray.400" py={2}>No matching connections.</Text>}
      </>}
      {(tab === 'elements' ? elements.length : matchingConnectors.length) > 5 && <Text fontSize="xs" color="gray.400" pt={1}>Showing 5 matches. Refine your search for more.</Text>}
    </VStack>}

  </Box>
}

function StepList({ steps, onChange, diagram, selection, onSelect, expanded, actions, onPick, picking }: { onPick?: () => void; picking?: boolean; actions?: ReactNode; steps: FlowStep[]; onChange: (steps: FlowStep[]) => void; diagram: FlowDiagram; selection?: FlowPanelProps['selection']; onSelect: (id: string | null) => void; expanded: string | null }) {
  const activeStepRef = useRef<HTMLDivElement>(null)
  useEffect(() => {
    const card = activeStepRef.current
    const body = card?.closest<HTMLElement>('[data-flow-scroll]')
    if (card && body) body.scrollTop += card.getBoundingClientRect().top - body.getBoundingClientRect().top
  }, [expanded])
  const patch = (id: string, changes: Partial<FlowStep>) => onChange(steps.map(step => step.id === id ? { ...step, ...changes } : step))
  function move(index: number, delta: number) {
    const next = [...steps]
    const [step] = next.splice(index, 1)
    next.splice(index + delta, 0, step)
    onChange(next)
  }
  return <VStack align="stretch" spacing={3}>
    <HStack justify="space-between" flexWrap="wrap" gap={2}>
      <Text fontSize="sm" fontWeight="semibold" flexShrink={0}>Steps ({steps.length})</Text>
      <HStack spacing={2}>
        {actions}
        <Button size="sm" isDisabled={steps.length >= 500} onClick={() => { const step = newStep(diagram, selection); onChange([...steps, step]); onSelect(step.id) }}>+ Step</Button>
      </HStack>
    </HStack>
    {!steps.length && <Text fontSize="sm" color="gray.400">Add a step to describe what happens and choose what to highlight.</Text>}
    {steps.map((step, index) => <Box key={step.id} ref={expanded === step.id ? activeStepRef : undefined} border="1px solid" borderColor={expanded === step.id ? 'blue.300' : 'whiteAlpha.200'} borderRadius="lg" overflow="hidden">
      <Button variant="ghost" size="sm" w="full" justifyContent="space-between" whiteSpace="normal" h="auto" minH="52px" textAlign="left" p={3} aria-expanded={expanded === step.id} onClick={() => { onSelect(expanded === step.id ? null : step.id) }}>
        <Box minW={0}><Text overflowWrap="anywhere">{index + 1}. {stepLabel(step, diagram)}</Text><Text mt={1} fontSize="xs" fontWeight="normal" color="gray.400">{step.elementIds.length + step.connectors.length === 0 ? 'Text only' : `${step.elementIds.length} elements · ${step.connectors.length} connections`}</Text></Box>
        <Text ml={2} aria-hidden>{expanded === step.id ? '−' : '+'}</Text>
      </Button>
      {missingReference(step, diagram) && <Text px={3} pb={2} fontSize="xs" color="orange.300">A highlight is unavailable. Remove it or select a replacement.</Text>}
      {expanded === step.id && <VStack align="stretch" spacing={3} p={3} pt={0}>
        <HStack spacing={1} flexWrap="wrap">
          <Button size="sm" aria-label="Move step up" isDisabled={index === 0} onClick={() => move(index, -1)}>↑</Button>
          <Button size="sm" aria-label="Move step down" isDisabled={index === steps.length - 1} onClick={() => move(index, 1)}>↓</Button>
          <Button size="sm" variant="ghost" onClick={() => { const [copy] = duplicateSteps([step]); onChange([...steps.slice(0, index + 1), copy, ...steps.slice(index + 1)]); onSelect(copy.id) }}>Duplicate</Button>
          <Button size="sm" colorScheme="red" variant="ghost" onClick={() => onChange(steps.filter(s => s.id !== step.id))}>Remove</Button>
        </HStack>
        <FormControl><FormLabel fontSize="xs">Step title</FormLabel><Input size="sm" aria-label="Step title" placeholder="What happens in this step?" maxLength={500} value={step.title} onChange={e => patch(step.id, { title: e.target.value })} /></FormControl>
        <FormControl><FormLabel fontSize="xs">Description (optional)</FormLabel><Textarea size="sm" rows={2} minH="64px" aria-label="Step description" placeholder="Explain this part of the story" maxLength={10000} value={step.description} onChange={e => patch(step.id, { description: e.target.value })} /></FormControl>
        <Box><Text fontSize="sm" fontWeight="semibold">Highlights</Text><Text fontSize="xs" color="gray.400" mt={1}>Choose elements or connections. Leave empty for a text-only step.</Text></Box>
        <StepHighlights step={step} diagram={diagram} onChange={changes => patch(step.id, changes)} onPick={onPick} picking={picking} />
      </VStack>}
    </Box>)}
  </VStack>
}

export function FlowPanel({ open, viewId, editMode = false, diagram, diagrams, selection, docked = false, compact = false, expanded = false, onExpandedChange, onCanvasRecorderChange, onEditModeChange, onClose, onFocus }: FlowPanelProps) {
  const queryClient = useQueryClient()
  const queryKey = useMemo(() => ['flows', viewId ?? 0], [viewId])
  const query = useQuery({ queryKey, queryFn: () => flowApi.list(viewId), enabled: open, retry: false })
  const [selectedId, setSelectedId] = useState('')
  const draftKey = useMemo(() => ['flow-draft', viewId ?? 0], [viewId])
  const [draft, setDraft] = useState<StoryFlow | null>(() => queryClient.getQueryData<StoryFlow>(draftKey) ?? null)
  useEffect(() => {
    if (!viewId) return
    // Keep unsaved work across canvas navigation for this app session only.
    queryClient.setQueryDefaults(draftKey, { gcTime: Infinity })
    if (draft) queryClient.setQueryData(draftKey, draft)
    else queryClient.removeQueries({ queryKey: draftKey, exact: true })
  }, [draft, draftKey, queryClient, viewId])
  const [busy, setBusy] = useState(false)
  const [error, setError] = useState('')
  const [deletePending, setDeletePending] = useState(false)
  const [canvasMode, setCanvasMode] = useState<'record' | 'pick' | null>(null)
  const flows = query.data ?? []
  const flow = draft ?? flows.find(flow => flow.id === selectedId) ?? null
  const canEditFlow = !!onEditModeChange || editMode
  const currentDiagram = diagram ?? (flow ? diagrams?.[flow.viewId] : undefined) ?? emptyDiagram
  const playback = useFlowPlayback(flow)
  const focus = useMemo(() => open && flow && (!editMode || playback.frame) ? resolveFocus(flow, playback.frame, currentDiagram) : null, [open, flow, editMode, playback.frame, currentDiagram])
  useEffect(() => { onFocus(focus) }, [focus, onFocus])
  useEffect(() => () => onFocus(null), [onFocus])
  const { pause, select: selectStep } = playback
  const activeStep = playback.frame?.steps[0]
  const recorderActive = canvasMode === 'record' && open && editMode && !busy && !!flow && flow.steps.length < 500
  const pickerActive = canvasMode === 'pick' && open && editMode && !busy && !!activeStep
  const captureActive = recorderActive || pickerActive
  useEffect(() => { setCanvasMode(null) }, [open, editMode, flow?.id])
  useEffect(() => { setCanvasMode(mode => mode === 'pick' ? null : mode) }, [activeStep?.id])
  useEffect(() => { if (flow && flow.steps.length >= 500) setCanvasMode(mode => mode === 'record' ? null : mode) }, [flow])
  useEffect(() => {
    if (!onCanvasRecorderChange) return
    if (!captureActive || !flow) { onCanvasRecorderChange(null); return }
    onCanvasRecorderChange({
      mode: pickerActive ? 'pick' : 'record',
      capture: target => {
        if (pickerActive && activeStep) {
          setDraft(current => {
            const base = current ?? flow
            const steps = base.steps.map(step => step.id === activeStep.id ? addCanvasHighlight(step, target, currentDiagram) : step)
            return steps.every((step, index) => step === base.steps[index]) ? current : { ...base, steps }
          })
          return
        }
        const step = captureCanvasStep(target, currentDiagram)
        if (!step) return
        setDraft(current => {
          const base = current ?? flow
          return base.steps.length < 500 ? { ...base, steps: [...base.steps, step] } : base
        })
        selectStep(step.id)
      },
      stop: () => setCanvasMode(null),
    })
    return () => onCanvasRecorderChange(null)
  }, [onCanvasRecorderChange, captureActive, pickerActive, activeStep, flow, currentDiagram, selectStep])
  useEffect(() => {
    if (!captureActive) return
    const cancel = (event: KeyboardEvent) => {
      if (event.key === 'Escape') { event.preventDefault(); setCanvasMode(null) }
    }
    window.addEventListener('keydown', cancel)
    return () => window.removeEventListener('keydown', cancel)
  }, [captureActive])
  useEffect(() => { if (!open || editMode) pause() }, [open, editMode, pause])
  useEffect(() => {
    if (playback.frame?.steps.some(step => missingReference(step, currentDiagram))) pause()
  }, [playback.frame, playback.playing, currentDiagram, pause])
  useEffect(() => {
    if (!draft) return
    const warn = (event: BeforeUnloadEvent) => { event.preventDefault(); event.returnValue = '' }
    window.addEventListener('beforeunload', warn)
    return () => window.removeEventListener('beforeunload', warn)
  }, [draft])

  function discardAllowed() { return !draft || window.confirm('Discard unsaved flow changes?') }
  function patch(changes: Partial<StoryFlow>) { if (flow && editMode && !busy) { playback.pause(); setDraft({ ...flow, ...changes }) } }
  async function save() {
    if (!draft || !editMode || busy) return
    setBusy(true); setCanvasMode(null); setError('')
    try {
      const saved = await flowApi.save(draft)
      queryClient.setQueryData<StoryFlow[]>(queryKey, old => [...(old ?? []).filter(f => f.id !== saved.id), saved])
      queryClient.removeQueries({ queryKey: draftKey, exact: true })
      setSelectedId(saved.id); setDraft(null)
      void queryClient.invalidateQueries({ queryKey: ['flows'] })
    } catch (error) { setError(error instanceof Error ? error.message : 'Could not save flow.') }
    finally { setBusy(false) }
  }
  async function remove() {
    if (!flow || !editMode || busy) return
    setBusy(true); setCanvasMode(null); setError('')
    try {
      await flowApi.remove(flow)
      queryClient.removeQueries({ queryKey: draftKey, exact: true })
      setDraft(null); setSelectedId(''); setDeletePending(false)
      queryClient.setQueryData<StoryFlow[]>(queryKey, old => (old ?? []).filter(f => f.id !== flow.id))
      void queryClient.invalidateQueries({ queryKey: ['flows'] })
    } catch (error) { setError(error instanceof Error ? error.message : 'Could not delete flow.') }
    finally { setBusy(false) }
  }
  function createFlow() {
    if (!viewId || !canEditFlow || busy || !discardAllowed()) return
    playback.pause()
    setSelectedId('')
    setDraft({ id: '', viewId, name: 'New flow', description: '', revision: 0, steps: [] })
    setError(''); setDeletePending(false)
    onEditModeChange?.(false)
  }
  function selectFlow(id: string) {
    if (busy || !discardAllowed()) return
    playback.pause()
    setDraft(null); setSelectedId(id); setError(''); setDeletePending(false)
    onEditModeChange?.(false)
  }
  function close() { if (!busy) { playback.pause(); onClose() } }
  if (!open) return null
  return <Box data-testid="flow-panel" role="region" aria-label="Flows" tabIndex={0} position={docked ? "relative" : "absolute"} zIndex={30}
    left={docked ? undefined : { base: 2, md: '64px' }} top={docked ? undefined : { base: 'auto', md: '64px' }} bottom={docked ? undefined : { base: '96px', md: 'auto' }}
    w={docked ? '100%' : { base: 'calc(100% - 16px)', md: 'min(400px, calc(100% - 80px))' }} h={docked ? '100%' : undefined} minH={0}
    maxH={docked ? '100%' : { base: 'min(65%, 760px)', md: 'min(760px, calc(100% - 168px))' }}
    display="flex" flexDirection="column" overflow="hidden" bg="var(--bg-main)" border="1px solid" borderColor="whiteAlpha.200" rounded={docked ? 0 : "xl"} boxShadow={docked ? undefined : "xl"}
    onKeyDown={event => {
      event.stopPropagation()
      if (event.key === 'Escape') { event.preventDefault(); if (captureActive) setCanvasMode(null); else close(); return }
      if ((event.ctrlKey || event.metaKey) && event.key.toLowerCase() === 's') { event.preventDefault(); void save(); return }
      if (event.target !== event.currentTarget || editMode) return
      if (event.key === 'ArrowRight') { event.preventDefault(); playback.next() }
      if (event.key === 'ArrowLeft') { event.preventDefault(); playback.previous() }
      if (event.key === ' ') { event.preventDefault(); playback.toggle() }
    }} onPointerDown={event => event.stopPropagation()} onWheel={event => event.stopPropagation()}>
    <HStack justify="space-between" p={3} borderBottom="1px solid" borderColor="whiteAlpha.200" flexShrink={0}>
      <Box><Text fontWeight="bold">{editMode ? 'Edit flow' : 'Flows'}</Text><Text fontSize="xs" color="gray.400">{pickerActive ? 'Picking step highlights' : recorderActive ? 'Recording canvas clicks' : draft ? 'Unsaved changes' : editMode && flow ? 'All changes saved' : 'Tell a story through your diagram'}</Text></Box>
      <HStack spacing={1}>
        {compact && captureActive && <Button size="sm" onClick={() => setCanvasMode(null)}>{pickerActive ? 'Done picking' : 'Stop recording'}</Button>}
        {compact && !captureActive && onExpandedChange && <Button size="sm" aria-label={expanded ? 'Reduce flow editor' : 'Expand flow editor'} onClick={() => onExpandedChange(!expanded)}>{expanded ? '↓' : '↑'}</Button>}
        <Button size="sm" aria-label="Close flows" isDisabled={busy} onClick={close}>×</Button>
      </HStack>
    </HStack>
    {!(compact && captureActive) && <HStack align="start" p={3} flexShrink={0} borderBottom="1px solid" borderColor="whiteAlpha.100">
        <Select size="sm" aria-label="Choose flow" value={draft?.id === '' ? '__new' : flow?.id ?? ''} isDisabled={busy} onChange={event => selectFlow(event.target.value)}>
          <option value="">Choose a flow</option>
          {draft?.id === '' && <option value="__new">{draft.name || 'New flow'}</option>}
          {flows.map(flow => <option key={flow.id} value={flow.id}>{flow.name}</option>)}
        </Select>
        {canEditFlow && viewId && <Button size="sm" flexShrink={0} isDisabled={busy} onClick={createFlow}>+ New flow</Button>}
    </HStack>}
      {(query.error || error) && <Box role="alert" p={3} flexShrink={0} maxH="35%" overflowY="auto" bg="orange.900" rounded="md">
        <Text fontSize="sm">{error ? 'Could not complete the action. Your draft is still available.' : 'Flows could not be loaded. Try again to reconnect.'}</Text>
        <Box as="details" mt={2}><Box as="summary" fontSize="xs" cursor="pointer">Technical details</Box><Text fontSize="xs" mt={1} overflowWrap="anywhere">{error || query.error?.message}</Text></Box>
        {query.error && <Button size="sm" mt={2} isLoading={query.isFetching} onClick={() => { void query.refetch() }}>Retry loading</Button>}
      </Box>}
    <VStack data-flow-scroll align="stretch" spacing={4} p={3} display={compact && captureActive ? "none" : "flex"} minH={0} overflowY="auto" overscrollBehavior="contain" flex={1}>
      {query.isLoading && <Text role="status" fontSize="sm">Loading flows…</Text>}

      {!flow && !query.isLoading && <Text fontSize="sm" color="gray.400">{flows.length ? 'Choose a story to follow through the diagram.' : editMode ? 'Create a flow, then add steps to highlight parts of this diagram.' : onEditModeChange ? 'No flows yet. Choose New flow to create your first story.' : 'No flows in this diagram yet.'}</Text>}
      {flow && <>
        <HStack data-testid="flow-details-header" justify="space-between" align="start" spacing={2}>
          {editMode ? <VStack flex={1} minW={0} align="stretch" spacing={0}>
            <Input aria-label="Flow name" isDisabled={busy} variant="unstyled" fontSize="sm" fontWeight="semibold" px={1} py={1} rounded="md" placeholder="Flow name" value={flow.name} maxLength={200}
              _hover={{ bg: 'whiteAlpha.50' }} _focusVisible={{ bg: 'whiteAlpha.50', outline: '1px solid', outlineColor: 'blue.300' }} onChange={e => patch({ name: e.target.value })} />
            <Textarea aria-label="Flow description" isDisabled={busy} variant="unstyled" fontSize="xs" color="gray.400" px={1} py={1} rounded="md" rows={1} minH="28px" maxH="160px" resize="none" placeholder="Add a description…" maxLength={10000} value={flow.description}
              _hover={{ bg: 'whiteAlpha.50' }} _focus={{ minH: '64px', bg: 'whiteAlpha.50', outline: '1px solid', outlineColor: 'blue.300' }} onChange={e => patch({ description: e.target.value })} />
          </VStack> : <Box flex={1} minW={0}><Text fontSize="sm" fontWeight="semibold" overflowWrap="anywhere">{flow.name}</Text>{flow.description && <Text mt={2} fontSize="sm" whiteSpace="pre-wrap">{flow.description}</Text>}</Box>}
          {!(compact && captureActive) && <HStack aria-label="Flow actions" spacing={1} flexShrink={0}>
            {(editMode || draft) && <FlowAction label="Discard" hint="Discard unsaved changes" icon={<ReloadIcon />} disabled={busy || !draft} onClick={() => { if (discardAllowed()) { setDraft(null); setError('') } }} />}
            {editMode ? <>
              <FlowAction label="Save flow" hint="Save flow (Cmd/Ctrl+S)" icon={<SaveIcon />} primary loading={busy} disabled={!draft || !flow.name.trim() || flow.steps.length > 500} onClick={() => void save()} />
              {!draft && <FlowAction label="Duplicate flow" icon={<CopyIcon boxSize="14px" />} disabled={busy} onClick={() => setDraft({ ...flow, id: '', revision: 0, name: `${flow.name.slice(0, 190)} copy`, steps: duplicateSteps(flow.steps) })} />}
              {flow.id && <FlowAction label="Delete flow" icon={<TrashIcon />} destructive disabled={busy} onClick={() => setDeletePending(true)} />}
              {onEditModeChange && <FlowAction label="Cancel edit" hint="Exit edit mode" icon={<CloseIcon />} modeSwitch disabled={busy} onClick={() => { playback.pause(); onEditModeChange(false) }} />}
            </> : onEditModeChange && <FlowAction label="Edit flow" icon={<EditorIcon size={14} />} modeSwitch disabled={busy} onClick={() => { playback.pause(); onEditModeChange(true) }} />}
          </HStack>}
        </HStack>
        {editMode && deletePending && <HStack><Text fontSize="xs">Delete this flow?</Text><Button size="xs" colorScheme="red" isDisabled={busy} onClick={() => void remove()}>Delete</Button><Button size="xs" isDisabled={busy} onClick={() => setDeletePending(false)}>Cancel</Button></HStack>}
        {editMode && <Box as="fieldset" disabled={busy} border={0} minW={0} p={0}>
          <VStack align="stretch" spacing={3}>
            {flow.steps.length >= 500 && <Text fontSize="xs" color="gray.400">Maximum of 500 steps reached.</Text>}
            <StepList
              onPick={onCanvasRecorderChange ? () => { playback.pause(); setCanvasMode(pickerActive ? null : 'pick') } : undefined} picking={pickerActive}
              actions={onCanvasRecorderChange && <Button size="sm" leftIcon={<Box as="span" w="8px" h="8px" rounded="full" bg={recorderActive ? 'red.300' : 'currentColor'} />} aria-pressed={recorderActive} isDisabled={busy || flow.steps.length >= 500} colorScheme={recorderActive ? 'red' : 'gray'} onClick={() => { playback.pause(); setCanvasMode(recorderActive ? null : 'record') }}>{recorderActive ? 'Stop recording' : 'Record steps'}</Button>}
              expanded={recorderActive ? null : activeStep?.id ?? null} steps={flow.steps} onChange={steps => patch({ steps })} diagram={currentDiagram} selection={selection} onSelect={playback.select} />
          </VStack>
        </Box>}
        {!editMode && !flow.steps.length && <Text fontSize="sm" color="gray.400">{onEditModeChange ? 'This flow has no steps yet. Choose Edit flow to add your story.' : 'This flow has no steps yet.'}</Text>}
        {!editMode && <Box borderTop="1px solid" borderColor="whiteAlpha.200" pt={3}>
          <HStack justify="space-between" mb={2}>
            <Button size="sm" aria-label="Previous flow step" isDisabled={playback.index < 0} onClick={playback.previous}>←</Button>
            <Button size="sm" colorScheme="blue" isDisabled={!playback.frames.length} onClick={playback.toggle}>{playback.playing ? 'Pause' : 'Auto-play'}</Button>
            <Button size="sm" aria-label="Next flow step" isDisabled={!playback.frames.length || playback.index === playback.frames.length - 1} onClick={playback.next}>→</Button>
          </HStack>
          <Select size="sm" aria-label="Current flow step" value={playback.frame?.id ?? ''} onChange={e => playback.select(e.target.value || null)}>
            <option value="">Overview</option>
            {playback.frames.map((frame, i) => <option key={frame.id} value={frame.id}>{i + 1}. {frame.steps.map(step => stepLabel(step, currentDiagram)).join(' + ')}</option>)}
          </Select>
          {playback.frame && <VStack align="stretch" spacing={2} my={3} aria-live="polite">
            <Text fontSize="xs" color="var(--accent)">Step {playback.index + 1} of {playback.frames.length}</Text>
            {playback.frame.steps.map(step => <Box key={step.id}>
              <Text fontWeight="semibold" fontSize="sm" overflowWrap="anywhere">{stepLabel(step, currentDiagram)}</Text>
              {step.description && <Text fontSize="sm" whiteSpace="pre-wrap">{step.description}</Text>}
              {missingReference(step, currentDiagram) && <Text color="orange.300" fontSize="xs">Reference unavailable. Use Next to skip this step.</Text>}
              {Object.entries(step.metadata).map(([key, value]) => <Text key={key} fontSize="xs" whiteSpace="pre-wrap"><strong>{key}:</strong> {value}</Text>)}
            </Box>)}
          </VStack>}
          <HStack mt={2} justify="space-between">
            <Select size="xs" w="120px" aria-label="Autoplay interval" value={playback.seconds} onChange={e => playback.setSeconds(Number(e.target.value))}>
              {[1, 2, 3, 5, 8].map(seconds => <option key={seconds} value={seconds}>{seconds}s per step</option>)}
            </Select>
            <Checkbox size="sm" isChecked={playback.loop} onChange={e => playback.setLoop(e.target.checked)}>Loop</Checkbox>
          </HStack>
        </Box>}
      </>}
    </VStack>
    {!(compact && captureActive) && <Box as="footer" data-testid="flow-yaml-footer" px={3} py={2} borderTop="1px solid" borderColor="whiteAlpha.200" flexShrink={0}>
        <FlowYamlControls flow={flow} diagram={currentDiagram} viewId={viewId} canImport={canEditFlow} disabled={busy} onImport={imported => {
          if (!canEditFlow || busy || !discardAllowed()) return false
          playback.pause(); setSelectedId(''); setDraft(imported); setError(''); setDeletePending(false)
          onEditModeChange?.(false)
          return true
        }} />
    </Box>}
  </Box>
}
