import type { Connector, PlacedElement } from '../types'

export const FLOW_MAX_STEPS = 1000
export const FLOW_MAX_PATHS = 26
export const FLOW_MAX_BRANCH_DEPTH = 8

export interface FlowStep {
  id: string
  title: string
  description: string
  elementIds: number[]
  connectorIds: number[]
  branches?: FlowBranches
  goToFlowId?: string
}
export interface FlowBranch { id: string; name: string; steps: FlowStep[] }
export interface FlowBranches { mode: string; paths: FlowBranch[] }
export interface FlowCatalog { flows: StoryFlow[]; views: { id: number; name: string }[] }
export interface StoryFlow { id: string; viewId: number; name: string; description: string; steps: FlowStep[]; revision: number }
export interface FlowDiagram {
  elements: (Pick<PlacedElement, 'element_id' | 'name'> & Partial<Pick<PlacedElement, 'kind' | 'description' | 'technology' | 'url' | 'logo_url' | 'technology_connectors' | 'tags' | 'position_x' | 'position_y'>>)[]
  connectors: Connector[]
}
export interface FlowFrame { id: string; steps: FlowStep[]; path?: FlowBranch; choices?: Record<string, string> }
export interface FlowFocus {
  viewId: number
  elementIds: number[]
  connectorIds: number[]
}

export function newStep(diagram: FlowDiagram, selected?: { elementId?: number; connectorId?: number }): FlowStep {
  return {
    id: crypto.randomUUID(), title: '', description: '',
    elementIds: diagram.elements.filter(e => e.element_id === selected?.elementId).map(e => e.element_id),
    connectorIds: diagram.connectors.filter(c => c.id === selected?.connectorId).map(c => c.id),
  }
}

/** Derive display text without persisting a generated title over the author's choice. */
export function stepLabel(step: FlowStep, diagram: FlowDiagram): string {
  if (step.title.trim()) return step.title
  const elementName = (id: number) => diagram.elements.find(element => element.element_id === id)?.name.trim() || `Element #${id}`
  const labels = [
    ...step.elementIds.map(elementName),
    ...step.connectorIds.map(id => {
      const connector = diagram.connectors.find(connector => connector.id === id)
      if (!connector) return `Connection #${id}`
      if (connector.label?.trim()) return connector.label.trim()
      const source = elementName(connector.source_element_id)
      const target = elementName(connector.target_element_id)
      return `${source} → ${target}`
    }),
  ]
  return labels.join(', ') || (step.branches ? step.branches.mode === 'parallel' ? 'Parallel paths' : 'Alternate paths' : step.goToFlowId ? 'Go to flow' : 'Text-only step')
}

export function allSteps(steps: FlowStep[]): FlowStep[] {
  return steps.flatMap(step => [step, ...(step.branches?.paths ?? []).flatMap(path => allSteps(path.steps))])
}

/** Position labels are derived from order; persisted step/path UUIDs remain stable. */
export function flowIdentifiers(steps: FlowStep[]): Map<string, string> {
  const identifiers = new Map<string, string>()
  function visit(items: FlowStep[], prefix: string) {
    items.forEach((step, index) => {
      const identifier = `${prefix}${index + 1}`
      identifiers.set(step.id, identifier)
      step.branches?.paths.forEach((path, pathIndex) => {
        const pathIdentifier = `${identifier}.${String.fromCharCode(97 + pathIndex)}`
        identifiers.set(path.id, pathIdentifier)
        visit(path.steps, `${pathIdentifier}.`)
      })
    })
  }
  visit(steps, '')
  return identifiers
}

export function updateStep(steps: FlowStep[], id: string, update: (step: FlowStep) => FlowStep): FlowStep[] {
  return steps.map(step => {
    if (step.id === id) return update(step)
    if (!step.branches) return step
    const paths = step.branches.paths.map(path => {
      const children = updateStep(path.steps, id, update)
      return children.every((child, index) => child === path.steps[index]) ? path : { ...path, steps: children }
    })
    return paths.every((path, index) => path === step.branches!.paths[index]) ? step : { ...step, branches: { ...step.branches, paths } }
  })
}

export function duplicateSteps(steps: FlowStep[]): FlowStep[] {
  return steps.map(step => ({ ...step, id: crypto.randomUUID(), elementIds: [...step.elementIds], connectorIds: [...step.connectorIds],
    ...(step.branches ? { branches: { ...step.branches, paths: step.branches.paths.map(path => ({ ...path, id: crypto.randomUUID(), steps: duplicateSteps(path.steps) })) } } : {}),
  }))
}

export function playbackFrames(steps: FlowStep[], choices: Record<string, string> = {}): FlowFrame[] {
  return steps.flatMap(step => {
    const path = step.branches?.paths.find(path => path.id === choices[step.id]) ?? step.branches?.paths[0]
    return [{ id: step.id, steps: [step], ...(path ? { path, choices } : {}) }, ...(path ? playbackFrames(path.steps, choices) : [])]
  })
}

/** Selecting a nested step also selects each of its ancestor paths. */
export function choicesForStep(steps: FlowStep[], id: string): Record<string, string> | undefined {
  for (const step of steps) {
    if (step.id === id) return {}
    for (const path of step.branches?.paths ?? []) {
      const found = choicesForStep(path.steps, id)
      if (found) return { [step.id]: path.id, ...found }
    }
  }
}

function initialHighlights(step: FlowStep, choices: Record<string, string> = {}): FlowStep[] {
  const paths = step.branches?.paths ?? []
  const preview = step.branches?.mode === 'alternate'
    ? [paths.find(path => path.id === choices[step.id]) ?? paths[0]].filter((path): path is FlowBranch => !!path)
    : paths
  return [step, ...preview.flatMap(path => path.steps[0] ? initialHighlights(path.steps[0], choices) : [])]
}

export function resolveFocus(flow: StoryFlow, frame: FlowFrame | undefined, diagram: FlowDiagram): FlowFocus {
  const elements = new Set<number>()
  const connectorIds = new Set<number>()
  const steps = !frame
    ? allSteps(flow.steps)
    : frame.steps.flatMap(step => step.branches?.mode === 'parallel' ? initialHighlights(step, frame.choices) : step.branches?.mode === 'alternate' ? [step, ...allSteps(frame.path?.steps ?? step.branches.paths[0]?.steps ?? [])] : [step])
  for (const step of steps) {
    for (const id of step.elementIds) if (diagram.elements.some(e => e.element_id === id)) elements.add(id)
    for (const id of step.connectorIds) {
      const connector = diagram.connectors.find(c => c.id === id)
      if (connector && [connector.source_element_id, connector.target_element_id].every(id => diagram.elements.some(e => e.element_id === id))) {
        elements.add(connector.source_element_id)
        elements.add(connector.target_element_id)
        connectorIds.add(connector.id)
      }
    }
  }
  return { viewId: flow.viewId, elementIds: [...elements], connectorIds: [...connectorIds] }
}

export function missingReference(step: FlowStep, diagram: FlowDiagram): boolean {
  return step.elementIds.some(id => !diagram.elements.some(e => e.element_id === id)) || step.connectorIds.some(id => {
    const connector = diagram.connectors.find(c => c.id === id)
    return !connector || ![connector.source_element_id, connector.target_element_id].every(id => diagram.elements.some(e => e.element_id === id))
  })
}

export interface FlowMinimapNode {
  id: string
  step?: FlowStep
  kind: 'step' | 'waypoint'
  col: number
  row: number
  x: number
  y: number
  label: string
  frameIndex: number
  onActivePath: boolean
  isActive: boolean
  isVisited: boolean
  isBranch: boolean
  branchMode?: string
  hasLink: boolean
  hasMissingRef: boolean
  choicePathId?: string
  branchStepId?: string
}

export interface FlowMinimapEdge {
  id: string
  fromId: string
  toId: string
  x1: number
  y1: number
  x2: number
  y2: number
  d: string
  onActivePath: boolean
  isVisited: boolean
  isActiveEdge: boolean
}

export interface FlowMinimapLayout {
  nodes: FlowMinimapNode[]
  edges: FlowMinimapEdge[]
  width: number
  height: number
  cols: number
  lanes: number
}

interface MeasuredMinimapSeq {
  cols: number
  lanes: number
  items: {
    step: FlowStep
    branchCols: number
    branchLanes: number
    paths: { branch: FlowBranch; measured: MeasuredMinimapSeq; cols: number; lanes: number }[]
  }[]
}

function measureMinimapSeq(steps: FlowStep[]): MeasuredMinimapSeq {
  const items = steps.map(step => {
    const rawPaths = step.branches?.paths ?? []
    if (!rawPaths.length) return { step, branchCols: 0, branchLanes: 1, paths: [] }
    const paths = rawPaths.map(branch => {
      const measured = measureMinimapSeq(branch.steps)
      return { branch, measured, cols: Math.max(1, measured.cols), lanes: Math.max(1, measured.lanes) }
    })
    const branchCols = Math.max(1, ...paths.map(path => path.cols))
    const branchLanes = Math.max(1, paths.reduce((sum, path) => sum + path.lanes, 0))
    return { step, branchCols, branchLanes, paths }
  })
  return {
    cols: items.reduce((sum, item) => sum + 1 + item.branchCols, 0),
    lanes: items.reduce((max, item) => Math.max(max, item.branchLanes), 1),
    items,
  }
}

export function minimapEdgePath(x1: number, y1: number, x2: number, y2: number, bendWidth = x2 - x1, bendAt: 'start' | 'end' = 'start'): string {
  if (Math.abs(y1 - y2) < 0.5) return `M ${x1} ${y1} L ${x2} ${y2}`
  const round = (value: number) => Math.round(value * 10) / 10
  const span = Math.min(bendWidth, x2 - x1)
  // Bend beside the fork/join, keeping sibling curves equal despite different path lengths.
  const start = bendAt === 'start' ? x1 : round(x2 - span)
  const end = bendAt === 'start' ? round(x1 + span) : x2
  const control = round((start + end) / 2)
  const lead = start === x1 ? '' : ` L ${start} ${y1}`
  const tail = end === x2 ? '' : ` L ${x2} ${y2}`
  return `M ${x1} ${y1}${lead} C ${control} ${y1} ${control} ${y2} ${end} ${y2}${tail}`
}

/** Lay out flow steps and branches as a left-to-right series-parallel chart. */
export function layoutFlowMinimap(
  steps: FlowStep[],
  diagram: FlowDiagram,
  choices: Record<string, string> = {},
  activeFrameId: string | null = null,
): FlowMinimapLayout {
  const minWidth = 304
  if (!steps.length) return { nodes: [], edges: [], width: minWidth, height: 34, cols: 0, lanes: 1 }

  const frames = playbackFrames(steps, choices)
  const frameIndexById = new Map(frames.map((frame, index) => [frame.id, index]))
  const activeIndex = activeFrameId ? (frameIndexById.get(activeFrameId) ?? -1) : -1
  const measured = measureMinimapSeq(steps)
  const totalCols = Math.max(1, measured.cols)
  const totalLanes = Math.max(1, measured.lanes)

  const padX = 18
  const laneGap = 20
  const height = totalLanes === 1 ? 34 : totalLanes * laneGap + 14
  const minColStep = 30
  const maxColStep = 84
  const innerMinWidth = minWidth - padX * 2
  const colStep = totalCols <= 1 ? 0 : Math.min(maxColStep, Math.max(minColStep, innerMinWidth / (totalCols - 1)))
  const contentWidth = (totalCols - 1) * colStep
  const width = Math.max(minWidth, Math.ceil(contentWidth + padX * 2))
  const startX = (width - contentWidth) / 2
  const toX = (col: number) => Math.round((startX + col * colStep) * 10) / 10
  const toY = (row: number) => Math.round(((height - totalLanes * laneGap) / 2 + row * laneGap) * 10) / 10

  const nodes: FlowMinimapNode[] = []
  const edges: FlowMinimapEdge[] = []

  function place(
    seq: MeasuredMinimapSeq,
    startCol: number,
    topRow: number,
    allocatedLanes: number,
    incoming: FlowMinimapNode[],
    parentOnActivePath: boolean,
    hasFollowingStep: boolean,
  ): FlowMinimapNode[] {
    const centerRow = topRow + allocatedLanes / 2
    let col = startCol
    let currentIncoming = incoming

    seq.items.forEach((item, index) => {
      const { step } = item
      const hasNext = index < seq.items.length - 1 || hasFollowingStep
      const frameIndex = frameIndexById.get(step.id) ?? -1
      const onActivePath = parentOnActivePath && frameIndex >= 0
      const isActive = activeIndex >= 0 && frameIndex === activeIndex
      const isVisited = activeIndex >= 0 && frameIndex >= 0 && frameIndex < activeIndex
      const node: FlowMinimapNode = {
        id: step.id,
        step,
        kind: 'step',
        col,
        row: centerRow,
        x: toX(col),
        y: toY(centerRow),
        label: stepLabel(step, diagram),
        frameIndex,
        onActivePath,
        isActive,
        isVisited,
        isBranch: item.paths.length > 0,
        branchMode: step.branches?.mode,
        hasLink: !!step.goToFlowId,
        hasMissingRef: missingReference(step, diagram),
      }
      nodes.push(node)

      for (const prev of currentIncoming) {
        const edgeOnActive = prev.onActivePath && node.onActivePath
        const edgeVisited = edgeOnActive && prev.isVisited && (node.isVisited || node.isActive)
        const isActiveEdge = edgeOnActive && prev.isVisited && node.isActive
        edges.push({
          id: `${prev.id}->${node.id}`,
          fromId: prev.id,
          toId: node.id,
          x1: prev.x,
          y1: prev.y,
          x2: node.x,
          y2: node.y,
          d: minimapEdgePath(prev.x, prev.y, node.x, node.y, colStep, prev.isBranch ? 'start' : 'end'),
          onActivePath: edgeOnActive,
          isVisited: edgeVisited,
          isActiveEdge,
        })
      }

      col += 1
      if (!item.paths.length) {
        currentIncoming = [node]
        return
      }

      const chosenPathId = choices[step.id] ?? step.branches?.paths[0]?.id
      let pathTop = centerRow - item.branchLanes / 2
      const exits: FlowMinimapNode[] = []
      for (const path of item.paths) {
        const isChosenPath = node.onActivePath && path.branch.id === chosenPathId
        const pathCenterRow = pathTop + path.lanes / 2
        if (path.measured.items.length > 0) {
          const offset = hasNext ? (item.branchCols - path.cols) / 2 : 0
          exits.push(...place(path.measured, col + offset, pathTop, path.lanes, [node], isChosenPath, hasNext))
        } else {
          const wpCol = col + (item.branchCols - 1) / 2
          const waypoint: FlowMinimapNode = {
            id: `${step.id}:${path.branch.id}`,
            kind: 'waypoint',
            col: wpCol,
            row: pathCenterRow,
            x: toX(wpCol),
            y: toY(pathCenterRow),
            label: path.branch.name || 'Path',
            frameIndex: -1,
            onActivePath: isChosenPath,
            isActive: false,
            isVisited: isChosenPath && node.isVisited,
            isBranch: false,
            hasLink: false,
            hasMissingRef: false,
            choicePathId: path.branch.id,
            branchStepId: step.id,
          }
          nodes.push(waypoint)
          edges.push({
            id: `${node.id}->${waypoint.id}`,
            fromId: node.id,
            toId: waypoint.id,
            x1: node.x,
            y1: node.y,
            x2: waypoint.x,
            y2: waypoint.y,
            d: minimapEdgePath(node.x, node.y, waypoint.x, waypoint.y, colStep),
            onActivePath: isChosenPath,
            isVisited: waypoint.isVisited,
            isActiveEdge: false,
          })
          exits.push(waypoint)
        }
        pathTop += path.lanes
      }
      currentIncoming = exits
      col += item.branchCols
    })

    return currentIncoming
  }

  place(measured, 0, 0, totalLanes, [], true, false)
  return { nodes, edges, width, height, cols: totalCols, lanes: totalLanes }
}
