import { useEffect, useId, useMemo, useRef, useState, type ReactElement, type ReactNode } from 'react'
import { Badge, Box, Button, Checkbox, FormControl, FormLabel, HStack, IconButton, Input, Menu, MenuButton, MenuItem, MenuList, Select, Tag, TagCloseButton, TagLabel, Text, Textarea, Tooltip, VStack } from '@chakra-ui/react'
import { useQuery, useQueryClient } from '@tanstack/react-query'
import { AddIcon, CopyIcon } from '@chakra-ui/icons'
import { motion, Reorder } from 'framer-motion'
import { faBackwardStep, faForwardStep, faPause, faPlay, faRepeat } from '@fortawesome/free-solid-svg-icons'
import FontAwesomeIcon from '../components/FontAwesomeIcon'
import { SortableFlowStep } from './SortableFlowStep'
import { ChevronDownIcon, CloseIcon, EditorIcon, EyeIcon, ReloadIcon, SaveIcon, TrashIcon } from '../components/Icons'
import { flowApi } from './api'
import { duplicateSteps, missingReference, newStep, resolveFocus, stepLabel, type FlowDiagram, type FlowFocus, type FlowStep, type StoryFlow } from './model'
import { useFlowPlayback } from './useFlowPlayback'
import { FlowYamlControls } from './FlowYamlControls'
import { addCanvasHighlight, captureCanvasStep, type FlowCanvasRecorder } from './canvasInteraction'
import { saveButtonStyles } from '../styles/saveButton'
import { panelSurfaceStyles } from '../styles/surfaces'

const emptyDiagram: FlowDiagram = { elements: [], connectors: [] }
function FlowOption({ flow, selected, onSelect }: { flow: StoryFlow; selected: boolean; onSelect?: () => void }) {
  const name = flow.name || 'Untitled flow'
  const count = `${flow.steps.length} ${flow.steps.length === 1 ? 'step' : 'steps'}`
  return <MenuItem aria-label={`${name}, ${count}`} aria-current={selected ? 'true' : undefined} isDisabled={selected} onClick={onSelect}
    bg={selected ? 'rgba(var(--accent-rgb), 0.12)' : undefined} color={selected ? 'var(--accent)' : undefined} _disabled={{ opacity: 1, cursor: 'default' }}>
    <HStack w="full" minW={0} spacing={3}>
      <Text flex={1} minW={0} isTruncated>{name}</Text>
      <Badge flexShrink={0} fontSize="10px" fontWeight="medium" textTransform="none" px={1.5} rounded="md"
        bg={selected ? 'rgba(var(--accent-rgb), 0.16)' : 'whiteAlpha.100'} color={selected ? 'var(--accent)' : 'text.muted'}>{count}</Badge>
    </HStack>
  </MenuItem>
}

function FlowAction({ label, hint = label, icon, disabled, loading, primary, destructive, modeSwitch, onClick }: {
  label: string; hint?: string; icon: ReactElement; disabled?: boolean; loading?: boolean; primary?: boolean; destructive?: boolean; modeSwitch?: boolean; onClick: () => void
}) {
  return <Tooltip label={hint} openDelay={200} closeOnPointerDown closeOnScroll>
    <Box as="span" display="inline-flex" ml={modeSwitch ? 1 : undefined}>
      <IconButton aria-label={label} icon={icon} size={modeSwitch ? 'sm' : 'xs'} w={modeSwitch || primary ? undefined : 7} h={modeSwitch || primary ? undefined : 7} variant={modeSwitch ? 'elevated' : 'ghost'} colorScheme={destructive ? 'red' : 'gray'} sx={primary ? saveButtonStyles : undefined} isDisabled={disabled} isLoading={loading} onClick={onClick} />
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
    {onPick && <Box px={2} pt={2}><Button size="sm" w="full" variant="primary" aria-pressed={!!picking} onClick={onPick}>{picking ? 'Done picking' : 'Pick from canvas'}</Button></Box>}
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

function StepList({ steps, onChange, diagram, selection, onSelect, expanded, actions, onPick, picking, disabled }: { disabled: boolean; onPick?: () => void; picking?: boolean; actions?: ReactNode; steps: FlowStep[]; onChange: (steps: FlowStep[]) => void; diagram: FlowDiagram; selection?: FlowPanelProps['selection']; onSelect: (id: string | null) => void; expanded: string | null }) {
  const listRef = useRef<HTMLDivElement>(null)
  useEffect(() => {
    const card = Array.from(listRef.current?.querySelectorAll<HTMLElement>('[data-flow-step-id]') ?? []).find(row => row.dataset.flowStepId === expanded)
    const body = card?.closest<HTMLElement>('[data-flow-scroll]')
    if (card && body) body.scrollTop += card.getBoundingClientRect().top - body.getBoundingClientRect().top
  }, [expanded])
  const patch = (id: string, changes: Partial<FlowStep>) => onChange(steps.map(step => step.id === id ? { ...step, ...changes } : step))
  function move(index: number, delta: number) {
    if (disabled || index + delta < 0 || index + delta >= steps.length) return
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
    <Reorder.Group as="div" ref={listRef} axis="y" values={steps} onReorder={onChange} style={{ display: 'flex', flexDirection: 'column', gap: 12, paddingLeft: 26 }}>
    {steps.map((step, index) => <SortableFlowStep key={step.id} step={step} index={index} disabled={disabled || steps.length < 2} onMove={delta => move(index, delta)}>{handle =>
      <Box role="group" position="relative" border="1px solid" borderColor={expanded === step.id ? 'var(--accent)' : 'whiteAlpha.100'} bg="whiteAlpha.50" borderRadius="lg" _hover={{ bg: 'whiteAlpha.100', borderColor: expanded === step.id ? 'var(--accent)' : 'whiteAlpha.300' }} sx={{
        '.flow-step-actions': { opacity: 0, pointerEvents: 'none' },
        '&:hover .flow-step-actions, &:focus-within .flow-step-actions': { opacity: 1, pointerEvents: 'auto' },
        '@media (hover: none), (pointer: coarse)': { '.flow-step-actions': { opacity: 1, pointerEvents: 'auto' } },
      }}>
      <Box position="relative" data-flow-step-header>
        {handle}
        <Button variant="unstyled" display="flex" alignItems="center" gap={2} size="sm" w="full" minW={0} h="54px" textAlign="left" p={2} pr="108px" rounded="lg" fontWeight="normal" aria-expanded={expanded === step.id} onClick={() => { onSelect(expanded === step.id ? null : step.id) }}>
          <Box data-flow-step-number display="flex" w="24px" h="24px" alignItems="center" justifyContent="center" flexShrink={0} bg="whiteAlpha.100" rounded="md" fontSize="10px" fontWeight="bold">{index + 1}</Box>
          <Box flex={1} minW={0}>
            <Text fontSize="sm" fontWeight="medium" noOfLines={1}>{stepLabel(step, diagram)}</Text>
            <Text mt={0.5} fontSize="10px" fontWeight="normal" color="gray.400" noOfLines={1}>{step.elementIds.length + step.connectors.length === 0 ? 'Text only' : `${step.elementIds.length} elements · ${step.connectors.length} connections`}</Text>
          </Box>
          <Box className="flow-step-toggle" position="absolute" right={2} top="50%" transform="translateY(-50%)" w={7} h={7} display="flex" alignItems="center" justifyContent="center" aria-hidden>
            <Box display="flex" transform={expanded === step.id ? 'rotate(180deg)' : undefined} transition="transform 150ms ease" sx={{ '@media (prefers-reduced-motion: reduce)': { transition: 'none' } }}><ChevronDownIcon /></Box>
          </Box>
        </Button>
        <HStack className="flow-step-actions" position="absolute" right={10} top={0} bottom={0} alignItems="center" spacing={1}>
          <FlowAction label={`Duplicate step ${index + 1}`} hint="Duplicate step" icon={<CopyIcon />} disabled={disabled || steps.length >= 500} onClick={() => { const [copy] = duplicateSteps([step]); onChange([...steps.slice(0, index + 1), copy, ...steps.slice(index + 1)]); onSelect(copy.id) }} />
          <FlowAction label={`Remove step ${index + 1}`} hint="Remove step" icon={<TrashIcon />} destructive disabled={disabled} onClick={() => onChange(steps.filter(s => s.id !== step.id))} />
        </HStack>
      </Box>
      {missingReference(step, diagram) && <Text px={3} pb={2} fontSize="xs" color="orange.300">A highlight is unavailable. Remove it or select a replacement.</Text>}
      {expanded === step.id && <VStack align="stretch" spacing={3} p={3} pt={0}>
        <FormControl><FormLabel fontSize="xs">Step title</FormLabel><Input size="sm" aria-label="Step title" placeholder="What happens in this step?" maxLength={500} value={step.title} onChange={e => patch(step.id, { title: e.target.value })} /></FormControl>
        <FormControl><FormLabel fontSize="xs">Description (optional)</FormLabel><Textarea size="sm" rows={2} minH="64px" aria-label="Step description" placeholder="Explain this part of the story" maxLength={10000} value={step.description} onChange={e => patch(step.id, { description: e.target.value })} /></FormControl>
        <Box><Text fontSize="sm" fontWeight="semibold">Highlights</Text><Text fontSize="xs" color="gray.400" mt={1}>Choose elements or connections. Leave empty for a text-only step.</Text></Box>
        <StepHighlights step={step} diagram={diagram} onChange={changes => patch(step.id, changes)} onPick={onPick} picking={picking} />
      </VStack>}
      </Box>
    }</SortableFlowStep>)}
    </Reorder.Group>
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
  const [editPlaybackOpen, setEditPlaybackOpen] = useState(false)
  const playbackId = useId()
  useEffect(() => { if (editMode) setEditPlaybackOpen(false) }, [editMode])
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
  useEffect(() => { if (!open || editMode || captureActive || busy) pause() }, [open, editMode, captureActive, busy, pause])
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
    if (!flow || !canEditFlow || busy) return
    setBusy(true); playback.pause(); setCanvasMode(null); setError('')
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
    display="flex" flexDirection="column" overflow="hidden" border="1px solid" sx={panelSurfaceStyles} rounded={docked ? 0 : "xl"} boxShadow={docked ? undefined : "panel"}
    onKeyDown={event => {
      event.stopPropagation()
      if (event.defaultPrevented) return
      if (event.key === 'Escape') { event.preventDefault(); if (captureActive) setCanvasMode(null); else close(); return }
      if ((event.ctrlKey || event.metaKey) && event.key.toLowerCase() === 's') { event.preventDefault(); void save(); return }
      if (event.target !== event.currentTarget || editMode) return
      if (event.key === 'ArrowRight') { event.preventDefault(); playback.next() }
      if (event.key === 'ArrowLeft') { event.preventDefault(); playback.previous() }
      if (event.key === ' ') { event.preventDefault(); playback.toggle() }
    }} onPointerDown={event => event.stopPropagation()} onWheel={event => event.stopPropagation()}>
    <VStack as="header" data-testid="flow-header" align="stretch" spacing={2} p={3} borderBottom="1px solid" borderColor="var(--border-main)" flexShrink={0}>
      <HStack spacing={2} minW={0}>
        {compact && captureActive ? <>
          <Text flex={1} minW={0} fontSize="sm" fontWeight="semibold">{pickerActive ? 'Picking highlights' : 'Recording steps'}</Text>
          <Button size="sm" onClick={() => setCanvasMode(null)}>{pickerActive ? 'Done picking' : 'Stop recording'}</Button>
        </> : <>
          <Box flex={1} minW={0}>
            <Menu placement="bottom-end" isLazy>
              <HStack spacing={0} minW={0} rounded="md" bg="whiteAlpha.50" border="1px solid" borderColor="var(--border-main)" overflow="hidden" _focusWithin={{ borderColor: 'var(--accent)' }}>
                {editMode && flow ? <>
                  <Input aria-label="Flow name" isDisabled={busy} variant="unstyled" flex={1} minW={0} h="32px" px={2} fontSize="sm" fontWeight="semibold" placeholder="Flow name" value={flow.name} maxLength={200}
                    _focusVisible={{ bg: 'whiteAlpha.50', boxShadow: 'inset 0 0 0 1px var(--accent)' }} onChange={e => patch({ name: e.target.value })} />
                  <MenuButton as={IconButton} aria-label="Choose flow" icon={<ChevronDownIcon />} size="sm" variant="ghost" isDisabled={busy} flexShrink={0} />
                </> : <MenuButton as={Button} aria-label="Choose flow" size="sm" variant="ghost" w="full" minW={0} px={2} textAlign="left" fontWeight="semibold" isDisabled={busy} rightIcon={<ChevronDownIcon />}>
                  <Text as="span" display="block" isTruncated>{flow ? flow.name || 'Untitled flow' : 'Choose a flow'}</Text>
                </MenuButton>}
              </HStack>
              <MenuList minW="240px" maxH="min(320px, 50vh)" overflowY="auto" maxW="calc(100vw - 32px)">
                <MenuItem onClick={() => selectFlow('')}>Choose a flow</MenuItem>
                {draft?.id === '' && <FlowOption flow={draft} selected />}
                {flows.map(candidate => <FlowOption key={candidate.id} flow={candidate.id === flow?.id ? flow : candidate} selected={candidate.id === flow?.id} onSelect={() => selectFlow(candidate.id)} />)}
              </MenuList>
            </Menu>
          </Box>
          {canEditFlow && viewId && <FlowAction label="+ New flow" hint="New flow" icon={<AddIcon boxSize="14px" />} disabled={busy} onClick={createFlow} />}
          {compact && onExpandedChange && <IconButton size="sm" variant="ghost" aria-label={expanded ? 'Reduce flow editor' : 'Expand flow editor'} onClick={() => onExpandedChange(!expanded)} icon={<Text as="span">{expanded ? '↓' : '↑'}</Text>} />}
        </>}
        <IconButton size="sm" aria-label="Close flows" icon={<CloseIcon />} isDisabled={busy} onClick={close} flexShrink={0} />
      </HStack>
      {!(compact && captureActive) && flow && <HStack data-testid="flow-details-header" spacing={2} align="start" minH="32px">

        {editMode ? <Textarea aria-label="Flow description" isDisabled={busy} flex={1} minW={0} variant="unstyled" fontSize="xs" color="gray.400" pl={2.5} py={1.5} rounded="md" rows={1} minH="32px" maxH="120px" resize="none" placeholder="Add a description…" maxLength={10000} value={flow.description}
          _hover={{ bg: 'whiteAlpha.50' }} _focus={{ minH: '64px', bg: 'whiteAlpha.50', outline: '1px solid', outlineColor: 'var(--accent)' }} onChange={e => patch({ description: e.target.value })} />
          : <Text flex={1} minW={0} fontSize="xs" color="gray.400" pl={2.5} py={1.5} noOfLines={1} title={flow.description || undefined}>{flow.description || 'No description'}</Text>}
        <HStack aria-label="Flow actions" spacing={1} flexShrink={0}>

            {(editMode || draft) && <FlowAction label="Discard" hint="Discard unsaved changes" icon={<ReloadIcon />} disabled={busy || !draft} onClick={() => { if (discardAllowed()) { setDraft(null); setError('') } }} />}
            {editMode ? <>
              <FlowAction label="Save flow" hint="Save flow (Cmd/Ctrl+S)" icon={<SaveIcon />} primary loading={busy} disabled={!draft || !flow.name.trim() || flow.steps.length > 500} onClick={() => void save()} />
              {onEditModeChange && <FlowAction label="Preview" hint="Preview flow" icon={<EyeIcon />} modeSwitch disabled={busy} onClick={() => { playback.pause(); onEditModeChange(false) }} />}
            </> : onEditModeChange && <FlowAction label="Edit flow" icon={<EditorIcon size={14} />} modeSwitch disabled={busy} onClick={() => { playback.pause(); onEditModeChange(true) }} />}
          </HStack>
      </HStack>}
    </VStack>
      {(query.error || error) && <Box role="alert" p={3} flexShrink={0} maxH="35%" overflowY="auto" bg="orange.900" rounded="md">
        <Text fontSize="sm">{error ? 'Could not complete the action. Your draft is still available.' : 'Flows could not be loaded. Try again to reconnect.'}</Text>
        <Box as="details" mt={2}><Box as="summary" fontSize="xs" cursor="pointer">Technical details</Box><Text fontSize="xs" mt={1} overflowWrap="anywhere">{error || query.error?.message}</Text></Box>
        {query.error && <Button size="sm" mt={2} isLoading={query.isFetching} onClick={() => { void query.refetch() }}>Retry loading</Button>}
      </Box>}
    <VStack as={motion.div} layoutScroll data-flow-scroll align="stretch" spacing={4} p={3} display={compact && captureActive ? "none" : "flex"} minH={0} overflowY="auto" overscrollBehavior="contain" flex={1}>
      {query.isLoading && <Text role="status" fontSize="sm">Loading flows…</Text>}

      {!flow && !query.isLoading && <Text fontSize="sm" color="gray.400">{flows.length ? 'Choose a story to follow through the diagram.' : editMode ? 'Create a flow, then add steps to highlight parts of this diagram.' : onEditModeChange ? 'No flows yet. Choose New flow to create your first story.' : 'No flows in this diagram yet.'}</Text>}
      {flow && <>
        {editMode && <Box as="fieldset" disabled={busy} border={0} minW={0} p={0}>
          <VStack align="stretch" spacing={3}>
            {flow.steps.length >= 500 && <Text fontSize="xs" color="gray.400">Maximum of 500 steps reached.</Text>}
            <StepList disabled={busy}
              onPick={onCanvasRecorderChange ? () => { playback.pause(); setCanvasMode(pickerActive ? null : 'pick') } : undefined} picking={pickerActive}
              actions={onCanvasRecorderChange && <Button size="sm" leftIcon={<Box as="span" w="8px" h="8px" rounded="full" bg={recorderActive ? 'red.300' : 'currentColor'} />} aria-pressed={recorderActive} isDisabled={busy || flow.steps.length >= 500} colorScheme={recorderActive ? 'red' : 'gray'} onClick={() => { playback.pause(); setCanvasMode(recorderActive ? null : 'record') }}>{recorderActive ? 'Stop recording' : 'Record steps'}</Button>}
              expanded={recorderActive ? null : activeStep?.id ?? null} steps={flow.steps} onChange={steps => patch({ steps })} diagram={currentDiagram} selection={selection} onSelect={playback.select} />
          </VStack>
        </Box>}
        {!editMode && !flow.steps.length && <Text fontSize="sm" color="gray.400">{onEditModeChange ? 'This flow has no steps yet. Choose Edit flow to add your story.' : 'This flow has no steps yet.'}</Text>}

      </>}
    </VStack>
    {flow && !captureActive && <Box data-testid="flow-playback-dock" role="region" aria-label="Flow playback" borderTop="1px solid" borderColor="var(--border-main)" flexShrink={0} minH={0} maxH="50%" display="flex" flexDirection="column" overflow="hidden">
      {editMode && <Button size="sm" variant="ghost" w="full" flexShrink={0} justifyContent="space-between" px={3} rounded={0} aria-label="Toggle playback controls" aria-expanded={editPlaybackOpen} aria-controls={playbackId}
        onClick={() => { if (editPlaybackOpen) playback.pause(); setEditPlaybackOpen(!editPlaybackOpen) }}>
        <Text fontSize="xs" fontWeight="semibold">Preview playback</Text>
        <Box transform={editPlaybackOpen ? 'rotate(180deg)' : undefined}><ChevronDownIcon /></Box>
      </Button>}
      {(!editMode || editPlaybackOpen) && <Box id={playbackId} data-testid="flow-playback-controls" p={3} minH={0} display="flex" flexDirection="column" overflow="hidden">
        {playback.frame && <VStack key={playback.frame.id} data-testid="flow-playback-details" align="stretch" spacing={2} mb={3} minH={0} maxH="160px" overflowY="auto" overscrollBehavior="contain" aria-live="polite" sx={{ '& > *': { flexShrink: 0 } }}>
          <Text fontSize="xs" color="var(--accent)">Step {playback.index + 1} of {playback.frames.length}</Text>
          {playback.frame.steps.map(step => <Box key={step.id}>
            <Text fontWeight="semibold" fontSize="sm" overflowWrap="anywhere">{stepLabel(step, currentDiagram)}</Text>
            {step.description && <Text fontSize="sm" whiteSpace="pre-wrap" overflowWrap="anywhere">{step.description}</Text>}
            {missingReference(step, currentDiagram) && <Text color="orange.300" fontSize="xs">Reference unavailable. Use Next to skip this step.</Text>}
            {Object.entries(step.metadata).map(([key, value]) => <Text key={key} fontSize="xs" whiteSpace="pre-wrap" overflowWrap="anywhere"><strong>{key}:</strong> {value}</Text>)}
          </Box>)}
        </VStack>}
        <Box data-testid="flow-playback-navigation" flexShrink={0}>
          <HStack justify="center" align="center" spacing={4} mb={2}>
            <Tooltip label="Previous flow step" openDelay={200} closeOnPointerDown closeOnScroll>
              <Box as="span" display="inline-flex"><IconButton size="sm" aria-label="Previous flow step" icon={<FontAwesomeIcon icon={faBackwardStep} size={14} />} _hover={{ transform: 'none' }} isDisabled={busy || playback.index < 0} onClick={playback.previous} /></Box>
            </Tooltip>
            <Tooltip label={playback.playing ? 'Pause' : 'Auto-play'} openDelay={200} closeOnPointerDown closeOnScroll>
              <Box as="span" display="inline-flex"><IconButton size="lg" variant="primary" aria-label={playback.playing ? 'Pause' : 'Auto-play'} icon={<FontAwesomeIcon icon={playback.playing ? faPause : faPlay} size={24} />} isDisabled={busy || !playback.frames.length} onClick={playback.toggle} /></Box>
            </Tooltip>
            <Tooltip label="Next flow step" openDelay={200} closeOnPointerDown closeOnScroll>
              <Box as="span" display="inline-flex"><IconButton size="sm" aria-label="Next flow step" icon={<FontAwesomeIcon icon={faForwardStep} size={14} />} _hover={{ transform: 'none' }} isDisabled={busy || !playback.frames.length || playback.index === playback.frames.length - 1} onClick={playback.next} /></Box>
            </Tooltip>
          </HStack>
          <Select size="sm" aria-label="Current flow step" isDisabled={busy} value={playback.frame?.id ?? ''} onChange={e => playback.select(e.target.value || null)}>
            <option value="">Overview</option>
            {playback.frames.map((frame, i) => <option key={frame.id} value={frame.id}>{i + 1}. {frame.steps.map(step => stepLabel(step, currentDiagram)).join(' + ')}</option>)}
          </Select>

          <HStack mt={2} justify="space-between">
            <Select size="xs" w="120px" aria-label="Autoplay interval" isDisabled={busy} value={playback.seconds} onChange={e => playback.setSeconds(Number(e.target.value))}>
              {[1, 2, 3, 5, 8].map(seconds => <option key={seconds} value={seconds}>{seconds}s per step</option>)}
            </Select>
            <Tooltip label={playback.loop ? 'Repeat on' : 'Repeat off'} openDelay={200} closeOnPointerDown closeOnScroll>
              <Box as="span" display="inline-flex"><IconButton size="xs" variant={playback.loop ? 'primary' : 'ghost'} aria-label="Repeat flow" aria-pressed={playback.loop} icon={<FontAwesomeIcon icon={faRepeat} size={14} />} isDisabled={busy} onClick={() => playback.setLoop(!playback.loop)} /></Box>
            </Tooltip>
          </HStack>
        </Box>
      </Box>}
    </Box>}
    {!(compact && captureActive) && <Box as="footer" data-testid="flow-yaml-footer" px={3} py={2} borderTop="1px solid" borderColor="var(--border-main)" flexShrink={0}>
      {canEditFlow && deletePending && <HStack mb={2}><Text flex={1} fontSize="xs">Delete this flow?</Text><Button size="xs" colorScheme="red" isDisabled={busy} onClick={() => void remove()}>Delete</Button><Button size="xs" isDisabled={busy} onClick={() => setDeletePending(false)}>Cancel</Button></HStack>}
      <HStack justify="space-between">
        <FlowYamlControls flow={flow} diagram={currentDiagram} viewId={viewId} canImport={canEditFlow} disabled={busy} onImport={imported => {
          if (!canEditFlow || busy || !discardAllowed()) return false
          playback.pause(); setSelectedId(''); setDraft(imported); setError(''); setDeletePending(false)
          onEditModeChange?.(false)
          return true
        }} />
        {canEditFlow && flow && <HStack aria-label="Flow management" spacing={1}>
              <FlowAction label="Duplicate flow" hint={draft ? 'Save or discard changes before duplicating' : 'Duplicate flow'} icon={<CopyIcon boxSize="14px" />} disabled={busy || !!draft} onClick={() => {
                playback.pause(); setDeletePending(false); setError(''); setDraft({ ...flow, id: '', revision: 0, name: `${flow.name.slice(0, 190)} copy`, steps: duplicateSteps(flow.steps) }); onEditModeChange?.(true)
              }} />
              <FlowAction label="Delete flow" hint={flow.id ? 'Delete flow' : 'Save the flow before deleting'} icon={<TrashIcon />} destructive disabled={busy || !flow.id} onClick={() => { playback.pause(); setDeletePending(true) }} />
        </HStack>}
      </HStack>
    </Box>}
  </Box>
}
