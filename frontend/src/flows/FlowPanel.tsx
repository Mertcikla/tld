import { useEffect, useId, useMemo, useRef, useState, type ReactElement, type ReactNode } from 'react'
import { Box, Button, Checkbox, Collapse, HStack, IconButton, Input, Menu, MenuButton, MenuItem, MenuList, Portal, Select, Tag, TagCloseButton, TagLabel, Text, Textarea, Tooltip, VStack } from '@chakra-ui/react'
import { useQuery, useQueryClient } from '@tanstack/react-query'
import { AddIcon, CopyIcon } from '@chakra-ui/icons'
import { motion } from 'framer-motion'
import { faBackwardStep, faForwardStep, faPause, faPlay, faRepeat, faStairs } from '@fortawesome/free-solid-svg-icons'
import FontAwesomeIcon from '../components/FontAwesomeIcon'
import { SortableFlowList } from './SortableFlowStep'
import { ChevronDownIcon, CloseIcon, ReloadIcon, SaveIcon, TrashIcon } from '../components/Icons'
import { flowApi } from './api'
import { api } from '../api/client'
import { enterFlow, returnFromFlow, type FlowJourney, type FlowVisit } from './navigation'
import { allSteps, updateStep, duplicateSteps, layoutFlowMinimap, missingReference, newStep, resolveFocus, stepLabel, type FlowBranch, type FlowCatalog, type FlowDiagram, type FlowFocus, type FlowStep, type StoryFlow } from './model'
import { useFlowPlayback } from './useFlowPlayback'
import { FlowYamlControls } from './FlowYamlControls'
import { addCanvasHighlight, captureCanvasStep, type FlowCanvasRecorder } from './canvasInteraction'
import { saveButtonStyles } from '../styles/saveButton'
import { panelSurfaceStyles } from '../styles/surfaces'

const emptyDiagram: FlowDiagram = { elements: [], connectors: [] }
function stepSummary(step: FlowStep): string {
  const behavior = step.branches ? `${step.branches.mode === 'parallel' ? 'Parallel' : 'Alternate'} paths` : step.goToFlowId ? 'Linked flow' : step.overview ? 'Overview' : ''
  const highlights = [
    step.elementIds.length ? `${step.elementIds.length} ${step.elementIds.length === 1 ? 'element' : 'elements'}` : '',
    step.connectors.length ? `${step.connectors.length} ${step.connectors.length === 1 ? 'connection' : 'connections'}` : '',
  ]
  return [behavior, ...highlights].filter(Boolean).join(' · ') || 'Text only'
}

function StepCount({ count, label = `${count} ${count === 1 ? 'step' : 'steps'}`, accent = false }: { count: number; label?: string; accent?: boolean }) {
  return <Tooltip label={label} openDelay={300} closeOnPointerDown closeOnScroll>
    <Box as="span" aria-label={label} display="inline-flex" alignItems="center" justifyContent="flex-end" gap={1.5} flexShrink={0} fontSize="sm" fontWeight="medium" color={accent ? 'var(--accent)' : 'text.muted'}>
      <Box as="span">{count}</Box><FontAwesomeIcon icon={faStairs} size={16} />
    </Box>
  </Tooltip>
}

function FlowOption({ flow, selected, onSelect }: { flow: StoryFlow; selected: boolean; onSelect?: () => void }) {
  const name = flow.name || 'Untitled flow'
  const total = allSteps(flow.steps).length
  const count = `${total} ${total === 1 ? 'step' : 'steps'}`
  return <MenuItem aria-label={`${name}, ${count}`} aria-current={selected ? 'true' : undefined} isDisabled={selected} onClick={onSelect}
    bg={selected ? 'rgba(var(--accent-rgb), 0.12)' : undefined} color={selected ? 'var(--accent)' : undefined} _disabled={{ opacity: 1, cursor: 'default' }}>
    <HStack w="full" minW={0} spacing={3}>
      <Text flex={1} minW={0} isTruncated>{name}</Text>
      <StepCount count={total} accent={selected} />
    </HStack>
  </MenuItem>
}

function FlowAction({ label, hint = label, icon, disabled, loading, primary, destructive, onClick }: {
  label: string; hint?: string; icon: ReactElement; disabled?: boolean; loading?: boolean; primary?: boolean; destructive?: boolean; onClick: () => void
}) {
  return <Tooltip label={hint} openDelay={200} closeOnPointerDown closeOnScroll>
    <Box as="span" display="inline-flex">
      <IconButton aria-label={label} icon={icon} size="xs" w={primary ? undefined : 7} h={primary ? undefined : 7} variant="ghost" colorScheme={destructive ? 'red' : 'gray'} sx={primary ? saveButtonStyles : undefined} isDisabled={disabled} isLoading={loading} onClick={onClick} />
    </Box>
  </Tooltip>
}

function FlowPlaybackMinimap({
  steps,
  choices,
  activeFrameId,
  diagram,
  disabled,
  onSelectStep,
  onChoosePath,
}: {
  steps: FlowStep[]
  choices: Record<string, string>
  activeFrameId: string | null
  diagram: FlowDiagram
  disabled: boolean
  onSelectStep: (stepId: string | null) => void
  onChoosePath: (stepId: string, pathId: string) => void
}) {
  const scrollRef = useRef<HTMLDivElement>(null)
  const layout = useMemo(() => layoutFlowMinimap(steps, diagram, choices, activeFrameId), [steps, diagram, choices, activeFrameId])
  const stepNodes = layout.nodes.filter(node => node.kind === 'step')
  const waypoints = layout.nodes.filter(node => node.kind === 'waypoint')
  const activeNode = stepNodes.find(node => node.isActive)
  const overviewActive = !activeFrameId && stepNodes.length > 0
  const centerY = Math.round((layout.height / 2) * 10) / 10
  const firstX = stepNodes[0]?.x ?? 18
  const lastX = stepNodes[stepNodes.length - 1]?.x ?? layout.width - 18

  useEffect(() => {
    const el = scrollRef.current
    if (!el || !activeNode || el.scrollWidth <= el.clientWidth) return
    const targetLeft = Math.max(0, activeNode.x - el.clientWidth / 2)
    el.scrollLeft = targetLeft
  }, [activeNode])

  return (
    <Box
      ref={scrollRef}
      data-testid="flow-playback-minimap-container"
      bg="whiteAlpha.50"
      border="1px solid"
      borderColor="whiteAlpha.100"
      rounded="lg"
      px={2}
      py={1}
      maxH="84px"
      overflowX="auto"
      overflowY="auto"
      sx={{
        scrollbarWidth: 'thin',
        '&::-webkit-scrollbar': { height: '4px', width: '4px' },
        '&::-webkit-scrollbar-thumb': { background: 'rgba(255, 255, 255, 0.16)', borderRadius: '999px' },
      }}
    >
      <svg
        data-testid="flow-playback-minimap"
        role="img"
        aria-label="Flow minimap"
        viewBox={`0 0 ${layout.width} ${layout.height}`}
        width={layout.width > 304 ? layout.width : '100%'}
        height={layout.height}
        style={{ display: 'block', minWidth: layout.width > 304 ? `${layout.width}px` : '100%' }}
      >
        <line
          x1={10}
          y1={centerY}
          x2={stepNodes.length ? firstX : layout.width - 10}
          y2={centerY}
          stroke="rgba(255, 255, 255, 0.08)"
          strokeWidth={1}
          strokeDasharray={stepNodes.length ? undefined : '3 3'}
        />
        {stepNodes.length > 0 && (
          <line
            x1={lastX}
            y1={centerY}
            x2={layout.width - 10}
            y2={centerY}
            stroke="rgba(255, 255, 255, 0.08)"
            strokeWidth={1}
          />
        )}
        {layout.edges.filter(edge => !edge.onActivePath).map(edge => (
          <path
            key={edge.id}
            data-testid="flow-minimap-edge"
            data-active-path="false"
            d={edge.d}
            fill="none"
            stroke="rgba(255, 255, 255, 0.16)"
            strokeWidth={1.25}
            strokeDasharray="3 3"
            strokeLinecap="round"
          />
        ))}
        {layout.edges.filter(edge => edge.onActivePath && !edge.isVisited).map(edge => (
          <path
            key={edge.id}
            data-testid="flow-minimap-edge"
            data-active-path="true"
            d={edge.d}
            fill="none"
            stroke={overviewActive ? 'rgba(var(--accent-rgb, 99, 179, 237), 0.55)' : 'rgba(255, 255, 255, 0.3)'}
            strokeWidth={1.75}
            strokeLinecap="round"
          />
        ))}
        {layout.edges.filter(edge => edge.isVisited).map(edge => (
          <g key={edge.id}>
            <path
              d={edge.d}
              fill="none"
              stroke="rgba(var(--accent-rgb, 99, 179, 237), 0.22)"
              strokeWidth={4.5}
              strokeLinecap="round"
            />
            <path
              data-testid="flow-minimap-edge"
              data-active-path="true"
              data-visited="true"
              d={edge.d}
              fill="none"
              stroke="var(--accent, #63B3ED)"
              strokeWidth={2.25}
              strokeLinecap="round"
            />
          </g>
        ))}
        {waypoints.map(wp => (
          <g
            key={wp.id}
            data-testid="flow-minimap-waypoint"
            aria-label={wp.label}
            style={{ cursor: disabled ? 'not-allowed' : 'pointer' }}
            onClick={() => {
              if (!disabled && wp.branchStepId && wp.choicePathId) {
                onSelectStep(wp.branchStepId)
                onChoosePath(wp.branchStepId, wp.choicePathId)
              }
            }}
          >
            <title>{wp.label}</title>
            <circle
              cx={wp.x}
              cy={wp.y}
              r={2.5}
              fill={wp.onActivePath ? 'var(--accent, #63B3ED)' : 'rgba(255, 255, 255, 0.22)'}
            />
          </g>
        ))}
        {!stepNodes.length && (
          <circle
            cx={layout.width / 2}
            cy={centerY}
            r={4}
            fill="var(--bg-main, #0d121e)"
            stroke="rgba(255, 255, 255, 0.2)"
            strokeWidth={1.25}
          />
        )}
        {stepNodes.map(node => {
          const stepTitle = node.frameIndex >= 0 ? `${node.frameIndex + 1}. ${node.label}` : node.label
          const stroke = node.isActive
            ? '#ffffff'
            : node.hasMissingRef
              ? '#F6AD55'
              : node.isVisited || overviewActive
                ? 'var(--accent, #63B3ED)'
                : node.onActivePath
                  ? 'rgba(255, 255, 255, 0.48)'
                  : 'rgba(255, 255, 255, 0.22)'
          const fill = node.isActive
            ? 'var(--accent, #63B3ED)'
            : node.isVisited
              ? 'rgba(var(--accent-rgb, 99, 179, 237), 0.3)'
              : overviewActive && node.onActivePath
                ? 'rgba(var(--accent-rgb, 99, 179, 237), 0.2)'
                : 'var(--bg-main, #0d121e)'
          const radius = node.isActive ? 7.5 : node.isVisited ? 5.5 : node.onActivePath ? 5 : 4.25
          return (
            <g
              key={node.id}
              data-testid="flow-minimap-step"
              data-step-id={node.id}
              data-active={node.isActive ? 'true' : undefined}
              data-visited={node.isVisited ? 'true' : undefined}
              data-on-active-path={node.onActivePath ? 'true' : undefined}
              aria-label={stepTitle}
              aria-current={node.isActive ? 'step' : undefined}
              style={{ cursor: disabled ? 'not-allowed' : 'pointer' }}
              onClick={() => { if (!disabled) onSelectStep(node.id) }}
            >
              <title>{stepTitle}</title>
              {node.isActive && (
                <>
                  <circle
                    data-testid="flow-minimap-active-halo"
                    cx={node.x}
                    cy={node.y}
                    r={11.5}
                    fill="rgba(var(--accent-rgb, 99, 179, 237), 0.18)"
                    stroke="rgba(var(--accent-rgb, 99, 179, 237), 0.45)"
                    strokeWidth={1}
                  />
                  <circle
                    cx={node.x}
                    cy={node.y}
                    r={9.25}
                    fill="none"
                    stroke="var(--accent, #63B3ED)"
                    strokeWidth={1}
                  />
                </>
              )}
              {node.hasLink && !node.isActive && (
                <circle
                  cx={node.x}
                  cy={node.y}
                  r={radius + 2.25}
                  fill="none"
                  stroke={stroke}
                  strokeWidth={1}
                  strokeDasharray="2 2"
                />
              )}
              {node.isBranch ? (
                <rect
                  x={node.x - radius * 0.85}
                  y={node.y - radius * 0.85}
                  width={radius * 1.7}
                  height={radius * 1.7}
                  rx={2}
                  transform={`rotate(45 ${node.x} ${node.y})`}
                  fill={fill}
                  stroke={stroke}
                  strokeWidth={node.isActive ? 1.5 : 1.35}
                />
              ) : (
                <circle
                  cx={node.x}
                  cy={node.y}
                  r={radius}
                  fill={fill}
                  stroke={stroke}
                  strokeWidth={node.isActive ? 1.5 : 1.35}
                />
              )}
              {node.isActive && node.frameIndex >= 0 ? (
                <text
                  x={node.x}
                  y={node.y}
                  textAnchor="middle"
                  dominantBaseline="central"
                  fontSize="8.5"
                  fontWeight="700"
                  fill="var(--accent-foreground, #0d121e)"
                  style={{ pointerEvents: 'none', userSelect: 'none' }}
                >
                  {node.frameIndex + 1}
                </text>
              ) : (node.isVisited || (node.onActivePath && !node.isBranch)) && (
                <circle
                  cx={node.x}
                  cy={node.y}
                  r={node.isVisited ? 2 : 1.5}
                  fill={node.isVisited || overviewActive ? 'var(--accent, #63B3ED)' : 'rgba(255, 255, 255, 0.45)'}
                />
              )}
            </g>
          )
        })}
      </svg>
    </Box>
  )
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
  onNavigateView?: (viewId: number) => void
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
  return <VStack align="stretch" spacing={2} minW={0}>
    <HStack justify="space-between" minW={0} spacing={1} flexWrap="wrap">
      <Text fontSize="xs" fontWeight="semibold" color="text.muted">What to highlight</Text>
      {onPick && <Button size="xs" h={7} px={2} variant={picking ? 'primary' : 'ghost'} color={picking ? undefined : 'var(--accent)'} aria-pressed={!!picking} onClick={onPick}>{picking ? 'Done picking' : 'Pick from canvas'}</Button>}
    </HStack>
    <Box border="1px solid" borderColor="whiteAlpha.200" rounded="md" overflow="hidden">
    <HStack p={2} spacing={1} flexWrap="wrap" aria-label="Highlight type">
      <Button flex="1 1 100px" minW={0} px={2} size="sm" variant={tab === 'elements' ? 'solid' : 'ghost'} aria-label="Show elements" aria-pressed={tab === 'elements'} onClick={() => { setTab('elements'); setSearch('') }}>Elements ({step.elementIds.length})</Button>
      <Button flex="1 1 100px" minW={0} px={2} size="sm" variant={tab === 'connections' ? 'solid' : 'ghost'} aria-label="Show connections" aria-pressed={tab === 'connections'} onClick={() => { setTab('connections'); setSearch('') }}>Connections ({step.connectors.length})</Button>
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
    {!step.elementIds.length && !step.connectors.length && <Text fontSize="xs" color="text.muted">Leave empty for a text-only step.</Text>}
  </VStack>
}

function StepOptions({ step, onChange, catalog, disabled, depth }: {
  step: FlowStep; onChange: (changes: Partial<FlowStep>) => void; catalog: FlowCatalog; disabled: boolean; depth: number
}) {
  const [linking, setLinking] = useState(!!step.goToFlowId)
  if (step.overview) return null
  if (step.branches) return <VStack align="stretch" spacing={2} borderTop="1px solid" borderColor="whiteAlpha.100" pt={3}>
    <HStack spacing={1}>
      {(['alternate', 'parallel'] as const).map(mode => <Button key={mode} size="xs" h={7} flex={1} minW={0} px={1} variant={step.branches!.mode === mode ? 'solid' : 'ghost'} aria-label={`${mode === 'alternate' ? 'Alternate' : 'Parallel'} paths`} aria-pressed={step.branches!.mode === mode} isDisabled={disabled} onClick={() => onChange({ branches: { ...step.branches!, mode } })}>{mode === 'alternate' ? 'Alternate' : 'Parallel'}</Button>)}
      <FlowAction label="Remove paths" icon={<CloseIcon />} disabled={disabled} onClick={() => { if (!step.branches!.paths.some(path => path.steps.length) || window.confirm('Remove these paths and their steps?')) onChange({ branches: undefined }) }} />
    </HStack>
    <Text fontSize="xs" color="text.muted">{step.branches.mode === 'alternate' ? 'Choose one path.' : 'Concurrent paths. Preview one at a time.'}</Text>
  </VStack>
  if (step.goToFlowId || linking) return <VStack align="stretch" spacing={2}>
    <HStack justify="space-between">
      <Text fontSize="xs" fontWeight="semibold" color="text.muted">Linked flow</Text>
      <FlowAction label="Remove flow link" icon={<CloseIcon />} disabled={disabled} onClick={() => { setLinking(false); onChange({ goToFlowId: undefined }) }} />
    </HStack>
    <Select size="sm" aria-label="Go to flow" isDisabled={disabled} value={step.goToFlowId ?? ''} onChange={event => onChange({ goToFlowId: event.target.value || undefined })}>
      <option value="">Choose a flow…</option>
      {step.goToFlowId && !catalog.flows.some(flow => flow.id === step.goToFlowId) && <option value={step.goToFlowId}>Unavailable linked flow</option>}
      {catalog.flows.map(flow => <option key={flow.id} value={flow.id}>{flow.name} · {catalog.views.find(view => view.id === flow.viewId)?.name ?? 'Diagram unavailable'}</option>)}
    </Select>
    {!catalog.flows.length && !step.goToFlowId && <Text fontSize="xs" color="text.muted">Save another flow to link it here.</Text>}
  </VStack>
  return <Menu placement="bottom-end" isLazy>
    <MenuButton as={Button} size="xs" variant="ghost" alignSelf="flex-start" leftIcon={<AddIcon boxSize="10px" />} isDisabled={disabled}>Add to step</MenuButton>
    <Portal><MenuList minW="200px" maxW="calc(100vw - 32px)">
      {(['alternate', 'parallel'] as const).map(mode => <MenuItem key={mode} aria-label={`Add ${mode} paths`} isDisabled={depth >= 8} onClick={() => onChange({ branches: { mode, paths: ['Path A', 'Path B'].map(name => ({ id: crypto.randomUUID(), name, steps: [] })) } })}>{mode === 'alternate' ? 'Alternate paths' : 'Parallel paths'}</MenuItem>)}
      <MenuItem aria-label="Link to flow" onClick={() => setLinking(true)}>Link to flow</MenuItem>
    </MenuList></Portal>
  </Menu>
}

function FlowEditorCard({ index, title, summary, handle, active, expanded, onToggle, actions, notice, children }: {
  index: number; title: string; summary: string; handle: ReactNode; active: boolean; expanded: boolean; onToggle: () => void; actions: ReactNode; notice?: ReactNode; children: ReactNode
}) {
  return <Box role="group" position="relative" minW={0} border="1px solid" borderColor={active ? 'var(--accent)' : 'whiteAlpha.100'} bg="whiteAlpha.50" borderRadius="lg" _hover={{ bg: 'whiteAlpha.100', borderColor: active ? 'var(--accent)' : 'whiteAlpha.300' }} sx={{
    '> [data-flow-step-header] .flow-step-actions': { opacity: 0, pointerEvents: 'none' },
    '&:hover > [data-flow-step-header] .flow-step-actions, &:focus-within > [data-flow-step-header] .flow-step-actions': { opacity: 1, pointerEvents: 'auto' },
    '@media (hover: none), (pointer: coarse)': { '> [data-flow-step-header] .flow-step-actions': { opacity: 1, pointerEvents: 'auto' } },
  }}>
    <Box position="relative" data-flow-step-header>
      {handle}
      <Button variant="unstyled" display="flex" alignItems="center" gap={2} size="sm" w="full" minW={0} h="54px" textAlign="left" p={2} pr="108px" rounded="lg" fontWeight="normal" aria-expanded={expanded} onClick={onToggle}>
        <Box data-flow-step-number display="flex" w="24px" h="24px" alignItems="center" justifyContent="center" flexShrink={0} bg="whiteAlpha.100" rounded="md" fontSize="10px" fontWeight="bold">{index + 1}</Box>
        <Box flex={1} minW={0}>
          <Text fontSize="sm" fontWeight="medium" noOfLines={1}>{title}</Text>
          <Text mt={0.5} fontSize="10px" fontWeight="normal" color="gray.400" noOfLines={1}>{summary}</Text>
        </Box>
        <Box className="flow-step-toggle" position="absolute" right={2} top="50%" transform="translateY(-50%)" w={7} h={7} display="flex" alignItems="center" justifyContent="center" aria-hidden>
          <Box display="flex" transform={expanded ? 'rotate(180deg)' : undefined} transition="transform 150ms ease" sx={{ '@media (prefers-reduced-motion: reduce)': { transition: 'none' } }}><ChevronDownIcon /></Box>
        </Box>
      </Button>
      <HStack className="flow-step-actions" position="absolute" right={10} top={0} bottom={0} alignItems="center" spacing={1}>{actions}</HStack>
    </Box>
    {notice && <Box px={3} pb={2}>{notice}</Box>}
    <Collapse in={expanded} animateOpacity unmountOnExit>{children}</Collapse>
  </Box>
}

function StepPaths({ step, onChange, disabled, available, expanded, onSelect, renderSteps }: {
  step: FlowStep; onChange: (paths: FlowBranch[]) => void; disabled: boolean; available: number; expanded: string | null; onSelect: (id: string | null) => void; renderSteps: (path: FlowBranch) => ReactNode
}) {
  const [collapsed, setCollapsed] = useState<Record<string, boolean>>({})
  const paths = step.branches!.paths
  return <VStack align="stretch" spacing={3}>
    <SortableFlowList items={paths} onChange={onChange} disabled={disabled} kind="path">{(path, index, handle) => {
      const count = allSteps(path.steps).length
      const active = allSteps(path.steps).some(child => child.id === expanded)
      const isExpanded = !collapsed[path.id] || active
      return <FlowEditorCard index={index} title={path.name} summary={`${count} ${count === 1 ? 'step' : 'steps'}`} handle={handle} active={active} expanded={isExpanded} onToggle={() => { setCollapsed(current => ({ ...current, [path.id]: isExpanded })); if (active) onSelect(step.id) }} actions={<>
        <FlowAction label={`Duplicate path ${path.name}`} hint="Duplicate path" icon={<CopyIcon />} disabled={disabled || paths.length >= 10 || count > available} onClick={() => onChange([...paths.slice(0, index + 1), { ...path, id: crypto.randomUUID(), name: `${path.name.slice(0, 190)} copy`, steps: duplicateSteps(path.steps) }, ...paths.slice(index + 1)])} />
        <FlowAction label={`Remove path ${path.name}`} hint="Remove path" icon={<TrashIcon />} destructive disabled={disabled || paths.length <= 2} onClick={() => { if (window.confirm('Remove this path and its steps?')) onChange(paths.filter(p => p.id !== path.id)) }} />
      </>}>
        <VStack align="stretch" spacing={3} p={3} pt={0}>
          <Input aria-label="Path name" size="sm" minW={0} isDisabled={disabled} maxLength={200} value={path.name} onChange={event => onChange(paths.map(p => p.id === path.id ? { ...p, name: event.target.value } : p))} />
          {renderSteps(path)}
        </VStack>
      </FlowEditorCard>
    }}</SortableFlowList>
    <Button size="sm" variant="ghost" leftIcon={<AddIcon boxSize="10px" />} isDisabled={disabled || paths.length >= 10} onClick={() => onChange([...paths, { id: crypto.randomUUID(), name: `Path ${paths.length + 1}`, steps: [] }])}>Add path</Button>
  </VStack>
}

function StepList({ steps, onChange, diagram, selection, onSelect, expanded, actions, onPick, picking, disabled, depth = 0, available = 500, catalog = { flows: [], views: [] } }: { depth?: number; available?: number; catalog?: FlowCatalog; disabled: boolean; onPick?: () => void; picking?: boolean; actions?: ReactNode; steps: FlowStep[]; onChange: (steps: FlowStep[]) => void; diagram: FlowDiagram; selection?: FlowPanelProps['selection']; onSelect: (id: string | null) => void; expanded: string | null }) {
  const patch = (id: string, changes: Partial<FlowStep>) => onChange(steps.map(step => step.id === id ? { ...step, ...changes } : step))
  return <VStack align="stretch" spacing={3}>
    <HStack aria-label="Flow steps" justify="space-between" flexWrap="wrap" gap={2}>
      <HStack spacing={2}>
        {actions}
        <Button size="sm" isDisabled={disabled || available <= 0} onClick={() => { const step = newStep(diagram, selection); onChange([...steps, step]); onSelect(step.id) }}>+ Step</Button>
      </HStack>
      <Box ml="auto"><StepCount count={allSteps(steps).length} /></Box>
    </HStack>
    {!steps.length && <Text fontSize={depth ? 'xs' : 'sm'} color="gray.400">{depth ? 'No steps yet.' : 'Add a step to describe what happens and choose what to highlight.'}</Text>}
    <SortableFlowList items={steps} onChange={onChange} disabled={disabled}>{(step, index, handle) =>
      <FlowEditorCard index={index} title={stepLabel(step, diagram)} summary={stepSummary(step)} handle={handle} active={expanded === step.id} expanded={expanded === step.id || allSteps((step.branches?.paths ?? []).flatMap(path => path.steps)).some(child => child.id === expanded)} onToggle={() => onSelect(expanded === step.id ? null : step.id)} actions={<>
        <FlowAction label={`Duplicate step ${index + 1}`} hint="Duplicate step" icon={<CopyIcon />} disabled={disabled || allSteps([step]).length > available} onClick={() => { const [copy] = duplicateSteps([step]); onChange([...steps.slice(0, index + 1), copy, ...steps.slice(index + 1)]); onSelect(copy.id) }} />
        <FlowAction label={`Remove step ${index + 1}`} hint="Remove step" icon={<TrashIcon />} destructive disabled={disabled} onClick={() => onChange(steps.filter(s => s.id !== step.id))} />
      </>} notice={missingReference(step, diagram) ? <Text fontSize="xs" color="orange.300">A highlight is unavailable. Remove it or select a replacement.</Text> : undefined}>
        <VStack align="stretch" spacing={3} p={3} pt={0}>
          <VStack align="stretch" spacing={0} minW={0}>
            <Text fontSize="xs" fontWeight="semibold" color="text.muted" mb={1}>Story</Text>
            <Input aria-label="Step title" variant="unstyled" minW={0} h="32px" px={2} rounded="md" fontSize="sm" fontWeight="semibold" placeholder="Step title" maxLength={500} value={step.title}
              _hover={{ bg: 'whiteAlpha.50' }} _focusVisible={{ bg: 'whiteAlpha.50', boxShadow: 'inset 0 0 0 1px var(--accent)' }} onChange={e => patch(step.id, { title: e.target.value })} />
            <Textarea aria-label="Step description" variant="unstyled" minW={0} fontSize="xs" color="gray.400" px={2} py={1.5} rounded="md" rows={1} minH="32px" maxH="120px" resize="none" placeholder="Add a description…" maxLength={10000} value={step.description}
              _hover={{ bg: 'whiteAlpha.50' }} _focus={{ minH: '64px', bg: 'whiteAlpha.50', outline: '1px solid', outlineColor: 'var(--accent)' }} onChange={e => patch(step.id, { description: e.target.value })} />
          </VStack>
          <StepHighlights step={step} diagram={diagram} onChange={changes => patch(step.id, changes)} onPick={expanded === step.id ? onPick : undefined} picking={expanded === step.id && picking} />
          <StepOptions step={step} onChange={changes => patch(step.id, changes)} catalog={catalog} disabled={disabled} depth={depth} />
          {step.branches && <StepPaths step={step} onChange={paths => patch(step.id, { branches: { ...step.branches!, paths } })} disabled={disabled} available={available} expanded={expanded} onSelect={onSelect} renderSteps={path =>
            <StepList steps={path.steps} onChange={steps => patch(step.id, { branches: { ...step.branches!, paths: step.branches!.paths.map(p => p.id === path.id ? { ...p, steps } : p) } })} diagram={diagram} selection={selection} onSelect={onSelect} expanded={expanded} onPick={onPick} picking={picking} disabled={disabled} depth={depth + 1} available={available} catalog={catalog} />
          } />}
        </VStack>
      </FlowEditorCard>
    }</SortableFlowList>
  </VStack>
}

export function FlowPanel({ open, viewId, editMode: requestedEditMode = false, diagram, diagrams, selection, docked = false, compact = false, expanded = false, onExpandedChange, onCanvasRecorderChange, onEditModeChange, onNavigateView, onClose, onFocus }: FlowPanelProps) {
  const queryClient = useQueryClient()
  const queryKey = useMemo(() => ['flows', viewId ?? 0], [viewId])
  const query = useQuery({ queryKey, queryFn: () => flowApi.list(viewId), enabled: open, retry: false })
  const navigationScope = viewId === undefined ? 'explorer' : 'editor'
  const navigationKey = useMemo(() => ['flow-navigation', navigationScope], [navigationScope])
  const initialJourney = useRef(queryClient.getQueryData<FlowJourney>(navigationKey))
  const initial = initialJourney.current?.current && (viewId === undefined || initialJourney.current.current.flow.viewId === viewId) ? initialJourney.current : undefined
  const [journey, setJourney] = useState<FlowJourney | null>(() => initial?.stack.length ? initial : null)
  const pendingRestore = useRef<FlowVisit | null>(initial?.current ?? null)
  const selectionStorageKey = `diag:lastFlow:${viewId ?? 'explorer'}`
  const [selectedId, setSelectedId] = useState(() => {
    if (initial) return initial.current.flow.id
    try { return window.localStorage.getItem(selectionStorageKey) ?? '' }
    catch { return '' }
  })
  const editMode = (requestedEditMode || !!onEditModeChange) && !journey
  const catalogQuery = useQuery({ queryKey: ['flows', 0], queryFn: () => flowApi.list(), enabled: open, retry: false })
  const viewsQuery = useQuery({ queryKey: ['flow-link-views'], queryFn: () => api.workspace.views.list(), enabled: open, retry: false })
  const catalogFlows = catalogQuery.data ?? []
  const catalog = { flows: catalogFlows, views: viewsQuery.data ?? [] }
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
  const [editPlaybackOpen, setEditPlaybackOpen] = useState(true)
  const playbackId = useId()
  useEffect(() => { if (editMode) setEditPlaybackOpen(true) }, [editMode])
  const [canvasMode, setCanvasMode] = useState<'record' | 'pick' | null>(null)
  const flows = query.data ?? []
  const flow = journey?.current.flow ?? draft ?? flows.find(flow => flow.id === selectedId) ?? flows[0] ?? null
  useEffect(() => {
    if (!open || journey || draft || !flow) return
    setSelectedId(flow.id)
    try { window.localStorage.setItem(selectionStorageKey, flow.id) }
    catch { /* Selection still works when browser storage is unavailable. */ }
  }, [open, journey, draft, flow, selectionStorageKey])
  const canEditFlow = !journey && (!!onEditModeChange || editMode)
  const currentDiagram = diagram ?? (flow ? diagrams?.[flow.viewId] : undefined) ?? emptyDiagram
  const playback = useFlowPlayback(flow)
  const focus = useMemo(() => open && flow && (!editMode || playback.frame) ? resolveFocus(flow, playback.frame, currentDiagram) : null, [open, flow, editMode, playback.frame, currentDiagram])
  useEffect(() => { onFocus(focus) }, [focus, onFocus])
  useEffect(() => () => onFocus(null), [onFocus])
  const { pause, select: selectStep, restore } = playback
  useEffect(() => {
    const pending = pendingRestore.current
    if (pending && flow?.id === pending.flow.id && flow.viewId === pending.flow.viewId) {
      restore(pending.stepId, pending.choices); pendingRestore.current = null
      if (!journey) queryClient.removeQueries({ queryKey: navigationKey, exact: true })
    }
  }, [flow, journey, restore, navigationKey, queryClient])
  const activeStep = playback.frame?.steps[0]
  const recorderActive = canvasMode === 'record' && open && editMode && !busy && !!flow && allSteps(flow.steps).length < 500
  const pickerActive = canvasMode === 'pick' && open && editMode && !busy && !!activeStep
  const captureActive = recorderActive || pickerActive
  useEffect(() => { setCanvasMode(null) }, [open, editMode, flow?.id])
  useEffect(() => { setCanvasMode(mode => mode === 'pick' ? null : mode) }, [activeStep?.id])
  useEffect(() => { if (flow && allSteps(flow.steps).length >= 500) setCanvasMode(mode => mode === 'record' ? null : mode) }, [flow])
  useEffect(() => {
    if (!onCanvasRecorderChange) return
    if (!captureActive || !flow) { onCanvasRecorderChange(null); return }
    onCanvasRecorderChange({
      mode: pickerActive ? 'pick' : 'record',
      capture: target => {
        if (pickerActive && activeStep) {
          setDraft(current => {
            const base = current ?? flow
            const steps = updateStep(base.steps, activeStep.id, step => addCanvasHighlight(step, target, currentDiagram))
            return steps.every((step, index) => step === base.steps[index]) ? current : { ...base, steps }
          })
          return
        }
        const step = captureCanvasStep(target, currentDiagram)
        if (!step) return
        setDraft(current => {
          const base = current ?? flow
          return allSteps(base.steps).length < 500 ? { ...base, steps: [...base.steps, step] } : base
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
  }
  function selectFlow(id: string) {
    if (busy || !discardAllowed()) return
    playback.pause(); setJourney(null); pendingRestore.current = null; queryClient.removeQueries({ queryKey: navigationKey, exact: true })
    setDraft(null); setSelectedId(id); setError(''); setDeletePending(false)
  }
  function goToFlow() {
    if (!flow || !activeStep?.goToFlowId || busy) return
    const target = catalogFlows.find(candidate => candidate.id === activeStep.goToFlowId)
    if (!target) { setError('The linked flow is unavailable. Reload flows or skip this step.'); return }
    if (viewId !== undefined && target.viewId !== viewId && !onNavigateView) { setError('This host cannot open another diagram.'); return }
    try {
      const next = enterFlow({ flow, stepId: activeStep.id, choices: playback.choices, draft: !journey && !!draft }, journey?.stack ?? [], target)
      playback.pause(); setJourney(next); pendingRestore.current = next.current
      queryClient.setQueryData(navigationKey, next); onEditModeChange?.(false)
      if (viewId !== undefined && target.viewId !== viewId) onNavigateView?.(target.viewId)
    } catch (error) { setError(error instanceof Error ? error.message : 'Could not open linked flow.') }
  }
  function returnToFlow() {
    if (!journey || busy) return
    const next = returnFromFlow(journey)
    if (!next) return
    playback.pause(); pendingRestore.current = next.current
    queryClient.setQueryData(navigationKey, next)
    setJourney(next.stack.length ? next : null)
    setSelectedId(next.current.flow.id)
    if (next.current.draft && (viewId === undefined || next.current.flow.viewId === viewId)) setDraft(next.current.flow)
    if (viewId !== undefined && next.current.flow.viewId !== viewId) onNavigateView?.(next.current.flow.viewId)
  }
  function nextStep() {
    if (activeStep?.goToFlowId) goToFlow()
    else if (playback.index === playback.frames.length - 1 && journey) returnToFlow()
    else playback.next()
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
      if (event.key === 'ArrowRight') { event.preventDefault(); nextStep() }
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
              <HStack spacing={0} minW={0} rounded="md" bg="whiteAlpha.50" border="1px solid" borderColor="var(--border-main)" overflow="hidden" _hover={{ borderColor: 'whiteAlpha.300' }} _focusWithin={{ borderColor: 'var(--accent)' }}>

                {editMode && flow ? <>
                  <Input aria-label="Flow name" isDisabled={busy} variant="unstyled" flex={1} minW={0} h="32px" px={2} fontSize="sm" fontWeight="semibold" placeholder="Flow name" value={flow.name} maxLength={200}
                    _focusVisible={{ bg: 'whiteAlpha.50', boxShadow: 'inset 0 0 0 1px var(--accent)' }} onChange={e => patch({ name: e.target.value })} />
                  <MenuButton as={IconButton} aria-label="Choose flow" icon={<ChevronDownIcon />} size="sm" variant="ghost" color="var(--accent)" isDisabled={busy} flexShrink={0} />
                </> : <MenuButton as={Button} aria-label="Choose flow" size="sm" variant="ghost" w="full" minW={0} px={2} textAlign="left" fontWeight="semibold" isDisabled={busy || !flows.length} rightIcon={<ChevronDownIcon />}>
                  <Text as="span" display="block" isTruncated>{flow ? flow.name || 'Untitled flow' : 'No flows'}</Text>
                </MenuButton>}
              </HStack>
              <MenuList minW="240px" maxH="min(320px, 50vh)" overflowY="auto" maxW="calc(100vw - 32px)">
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
      {journey && <Button size="sm" aria-label="Return to previous flow" onClick={returnToFlow}>Return to {journey.stack[journey.stack.length - 1]?.flow.name}</Button>}
      {!(compact && captureActive) && flow && <HStack data-testid="flow-details-header" spacing={2} align="start" minH="32px">

        {editMode ? <Textarea aria-label="Flow description" isDisabled={busy} flex={1} minW={0} variant="unstyled" fontSize="xs" color="gray.400" pl={2.5} py={1.5} rounded="md" rows={1} minH="32px" maxH="120px" resize="none" placeholder="Add a description…" maxLength={10000} value={flow.description}
          _hover={{ bg: 'whiteAlpha.50' }} _focus={{ minH: '64px', bg: 'whiteAlpha.50', outline: '1px solid', outlineColor: 'var(--accent)' }} onChange={e => patch({ description: e.target.value })} />
          : <Text flex={1} minW={0} fontSize="xs" color="gray.400" pl={2.5} py={1.5} noOfLines={1} title={flow.description || undefined}>{flow.description || 'No description'}</Text>}
        <HStack aria-label="Flow actions" spacing={1} flexShrink={0}>

            {!journey && (editMode || draft) && <FlowAction label="Discard" hint="Discard unsaved changes" icon={<ReloadIcon />} disabled={busy || !draft} onClick={() => { if (discardAllowed()) { setDraft(null); setError('') } }} />}
            {editMode && <FlowAction label="Save flow" hint="Save flow (Cmd/Ctrl+S)" icon={<SaveIcon />} primary loading={busy} disabled={!draft || !flow.name.trim() || allSteps(flow.steps).length > 500} onClick={() => void save()} />}

          </HStack>
      </HStack>}
    </VStack>
      {(query.error || error) && <Box role="alert" p={3} flexShrink={0} maxH="35%" overflowY="auto" bg="orange.900" rounded="md">
        <Text fontSize="sm">{error ? 'Could not complete the action. Your draft is still available.' : 'Flows could not be loaded. Try again to reconnect.'}</Text>
        <Box as="details" mt={2}><Box as="summary" fontSize="xs" cursor="pointer">Technical details</Box><Text fontSize="xs" mt={1} overflowWrap="anywhere">{error || query.error?.message}</Text></Box>
        {query.error && <Button size="sm" mt={2} isLoading={query.isFetching} onClick={() => { void query.refetch() }}>Retry loading</Button>}
      </Box>}
    <VStack as={motion.div} layoutScroll data-flow-scroll align="stretch" spacing={4} p={query.isLoading || !flow || editMode || !flow.steps.length ? 3 : 0} display={compact && captureActive ? "none" : "flex"} minH={0} overflowY="auto" overscrollBehavior="contain" flex={query.isLoading || !flow || editMode || !flow.steps.length ? 1 : 0}>
      {query.isLoading && <Text role="status" fontSize="sm">Loading flows…</Text>}

      {!flow && !query.isLoading && <Text fontSize="sm" color="gray.400">{flows.length ? 'Choose a story to follow through the diagram.' : editMode ? 'Create a flow, then add steps to highlight parts of this diagram.' : onEditModeChange ? 'No flows yet. Choose New flow to create your first story.' : 'No flows in this diagram yet.'}</Text>}
      {flow && <>
        {editMode && <Box as="fieldset" disabled={busy} border={0} minW={0} p={0}>
          <VStack align="stretch" spacing={3}>
            {allSteps(flow.steps).length >= 500 && <Text fontSize="xs" color="gray.400">Maximum of 500 steps reached.</Text>}
            <StepList disabled={busy} available={500 - allSteps(flow.steps).length} catalog={catalog}
              onPick={onCanvasRecorderChange ? () => { playback.pause(); setCanvasMode(pickerActive ? null : 'pick') } : undefined} picking={pickerActive}
              actions={onCanvasRecorderChange && <Button size="sm" leftIcon={<Box as="span" w="8px" h="8px" rounded="full" bg={recorderActive ? 'red.300' : 'currentColor'} />} aria-pressed={recorderActive} isDisabled={busy || allSteps(flow.steps).length >= 500} colorScheme={recorderActive ? 'red' : 'gray'} onClick={() => { playback.pause(); setCanvasMode(recorderActive ? null : 'record') }}>{recorderActive ? 'Stop recording' : 'Record steps'}</Button>}
              expanded={recorderActive ? null : activeStep?.id ?? null} steps={flow.steps} onChange={steps => patch({ steps })} diagram={currentDiagram} selection={selection} onSelect={playback.select} />
          </VStack>
        </Box>}
        {!editMode && !flow.steps.length && <Text fontSize="sm" color="gray.400">This flow has no steps yet.</Text>}

      </>}
    </VStack>
    {flow && !captureActive && <Box data-testid="flow-playback-dock" role="region" aria-label="Flow playback" borderTop={editMode || !flow.steps.length || query.isLoading ? "1px solid" : undefined} borderColor="var(--border-main)" flexShrink={0} minH={0} maxH={editMode ? "55%" : undefined} display="flex" flexDirection="column" overflow="hidden">
      {editMode && <Button size="xs" h={7} variant="ghost" w="full" flexShrink={0} justifyContent="space-between" px={3} rounded={0} aria-label="Toggle playback controls" aria-expanded={editPlaybackOpen} aria-controls={playbackId}
        onClick={() => { if (editPlaybackOpen) playback.pause(); setEditPlaybackOpen(!editPlaybackOpen) }}>
        <Text fontSize="xs" fontWeight="semibold">Preview playback</Text>
        <Box transform={editPlaybackOpen ? 'rotate(180deg)' : undefined}><ChevronDownIcon /></Box>
      </Button>}
      {(!editMode || editPlaybackOpen) && <Box id={playbackId} data-testid="flow-playback-controls" px={3} py={2.5} minH={0} display="flex" flexDirection="column" gap={2} overflow="hidden">
        {playback.frame && <VStack key={playback.frame.id} data-testid="flow-playback-details" align="stretch" spacing={1.5} minH={0} maxH="140px" overflowY="auto" overscrollBehavior="contain" aria-live="polite" sx={{ '& > *': { flexShrink: 0 } }}>
          <HStack data-testid="flow-playback-heading" justify="space-between" minW={0} spacing={2}>
            <Tooltip label={playback.frame.steps.map(step => stepLabel(step, currentDiagram)).join(' + ')} openDelay={300} closeOnPointerDown closeOnScroll>
              <Text flex={1} minW={0} fontWeight="semibold" fontSize="sm" isTruncated>{playback.frame.steps.map(step => stepLabel(step, currentDiagram)).join(' + ')}</Text>
            </Tooltip>
            {activeStep?.branches && <Select size="xs" h={6} w="auto" maxW="140px" flexShrink={0} aria-label="Choose flow path" value={playback.choices[activeStep.id] ?? activeStep.branches.paths[0]?.id ?? ''} onChange={event => playback.choose(activeStep.id, event.target.value)}>
              {activeStep.branches.paths.map(path => <option key={path.id} value={path.id}>{path.name}</option>)}
            </Select>}
            <StepCount count={playback.index + 1} label={`Step ${playback.index + 1} of ${playback.frames.length}`} accent />
          </HStack>
          {activeStep?.goToFlowId && <HStack spacing={1.5}><Button size="xs" variant="primary" isDisabled={busy || !catalogFlows.some(candidate => candidate.id === activeStep.goToFlowId)} onClick={goToFlow}>Go to flow</Button><Button size="xs" onClick={playback.next}>Skip flow</Button>
            {!catalogFlows.some(candidate => candidate.id === activeStep.goToFlowId) && <Text fontSize="xs" color="gray.400">Linked flow unavailable.</Text>}
          </HStack>}
          {playback.frame.steps.map(step => (step.description || missingReference(step, currentDiagram) || Object.keys(step.metadata).length > 0) && <Box key={step.id}>
            {step.description && <Text fontSize="xs" color="gray.300" whiteSpace="pre-wrap" overflowWrap="anywhere">{step.description}</Text>}
            {missingReference(step, currentDiagram) && <Text color="orange.300" fontSize="xs">Reference unavailable. Use Next to skip this step.</Text>}
            {Object.entries(step.metadata).map(([key, value]) => <Text key={key} fontSize="xs" color="gray.400" whiteSpace="pre-wrap" overflowWrap="anywhere"><strong>{key}:</strong> {value}</Text>)}
          </Box>)}
        </VStack>}
        <VStack data-testid="flow-playback-navigation" align="stretch" spacing={2} flexShrink={0}>
          <FlowPlaybackMinimap
            steps={flow.steps}
            choices={playback.choices}
            activeFrameId={playback.frame?.id ?? null}
            diagram={currentDiagram}
            disabled={busy}
            onSelectStep={playback.select}
            onChoosePath={playback.choose}
          />
          <HStack spacing={1.5} align="center" minW={0}>
            <HStack spacing={1} flexShrink={0}>
              <Tooltip label="Previous flow step" openDelay={200} closeOnPointerDown closeOnScroll>
                <Box as="span" display="inline-flex"><IconButton size="xs" w={7} h={7} aria-label="Previous flow step" icon={<FontAwesomeIcon icon={faBackwardStep} size={12} />} _hover={{ transform: 'none' }} isDisabled={busy || playback.index < 0} onClick={playback.previous} /></Box>
              </Tooltip>
              <Tooltip label={playback.playing ? 'Pause' : 'Auto-play'} openDelay={200} closeOnPointerDown closeOnScroll>
                <Box as="span" display="inline-flex"><IconButton size="xs" w={8} h={7} variant="primary" aria-label={playback.playing ? 'Pause' : 'Auto-play'} icon={<FontAwesomeIcon icon={playback.playing ? faPause : faPlay} size={13} />} isDisabled={busy || !playback.frames.length || !!activeStep?.goToFlowId} onClick={playback.toggle} /></Box>
              </Tooltip>
              <Tooltip label="Next flow step" openDelay={200} closeOnPointerDown closeOnScroll>
                <Box as="span" display="inline-flex"><IconButton size="xs" w={7} h={7} aria-label="Next flow step" icon={<FontAwesomeIcon icon={faForwardStep} size={12} />} _hover={{ transform: 'none' }} isDisabled={busy || !playback.frames.length || (playback.index === playback.frames.length - 1 && !journey && !activeStep?.goToFlowId)} onClick={nextStep} /></Box>
              </Tooltip>
            </HStack>
            <Box flex={1} minW={0}>
              <Select size="xs" h={7} rounded="md" aria-label="Current flow step" isDisabled={busy} value={playback.frame?.id ?? ''} onChange={e => playback.select(e.target.value || null)}>
                <option value="">Overview</option>
                {playback.frames.map((frame, i) => <option key={frame.id} value={frame.id}>{i + 1}. {frame.steps.map(step => stepLabel(step, currentDiagram)).join(' + ')}</option>)}
              </Select>
            </Box>
            <HStack spacing={1} flexShrink={0}>
              <Select size="xs" h={7} w="64px" rounded="md" aria-label="Autoplay interval" title="Autoplay interval" isDisabled={busy} value={playback.seconds} onChange={e => playback.setSeconds(Number(e.target.value))}>
                {[1, 2, 3, 5, 8].map(seconds => <option key={seconds} value={seconds}>{seconds}s</option>)}
              </Select>
              <Tooltip label={playback.loop ? 'Repeat on' : 'Repeat off'} openDelay={200} closeOnPointerDown closeOnScroll>
                <Box as="span" display="inline-flex"><IconButton size="xs" w={7} h={7} variant={playback.loop ? 'primary' : 'ghost'} aria-label="Repeat flow" aria-pressed={playback.loop} icon={<FontAwesomeIcon icon={faRepeat} size={12} />} isDisabled={busy} onClick={() => playback.setLoop(!playback.loop)} /></Box>
              </Tooltip>
            </HStack>
          </HStack>
        </VStack>
      </Box>}
    </Box>}
    {!(compact && captureActive) && <Box as="footer" data-testid="flow-yaml-footer" px={3} py={2} borderTop="1px solid" borderColor="var(--border-main)" flexShrink={0}>
      {canEditFlow && deletePending && <HStack mb={2}><Text flex={1} fontSize="xs">Delete this flow?</Text><Button size="xs" colorScheme="red" isDisabled={busy} onClick={() => void remove()}>Delete</Button><Button size="xs" isDisabled={busy} onClick={() => setDeletePending(false)}>Cancel</Button></HStack>}
      <HStack justify="space-between">
        <FlowYamlControls catalog={catalog} flow={flow} diagram={currentDiagram} viewId={viewId} canImport={canEditFlow} disabled={busy} onImport={imported => {
          if (!canEditFlow || busy || !discardAllowed()) return false
          playback.pause(); setSelectedId(''); setDraft(imported); setError(''); setDeletePending(false)
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
