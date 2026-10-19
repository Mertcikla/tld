import type { DiagramGroupLayout, LayoutNode, ZUILayout, BBox } from './types'

export interface NodeScreenState {
  worldX: number
  worldY: number
  screenW: number
  drawZoom: number
  parentAlpha: number
  childAlpha: number
  inheritedAlpha: number
  t: number
  isVisible: boolean
  isLeafCapped: boolean
  leafCapScale: number
}

export function createNodeScreenState(): NodeScreenState {
  return {
    worldX: 0,
    worldY: 0,
    screenW: 0,
    drawZoom: 0,
    parentAlpha: 0,
    childAlpha: 0,
    inheritedAlpha: 1,
    t: 0,
    isVisible: false,
    isLeafCapped: false,
    leafCapScale: 1,
  }
}

export interface SceneNode {
  layout: LayoutNode
  children: SceneNode[]
  state: NodeScreenState
}

export interface SceneGroup {
  layout: DiagramGroupLayout
  nodes: SceneNode[]
  isVisible: boolean
}

export interface SceneGraph {
  groups: SceneGroup[]
  bbox: BBox
  nodeById: Map<string, SceneNode>
}

function wrapNode(node: LayoutNode): SceneNode {
  const children = (node.children ?? []).map((child) => wrapNode(child))
  return {
    layout: node,
    children,
    state: createNodeScreenState(),
  }
}

export function buildSceneGraph(layout: ZUILayout): SceneGraph {
  const nodeById = new Map<string, SceneNode>()

  function indexNode(n: SceneNode): void {
    nodeById.set(n.layout.id, n)
    for (const child of n.children) {
      indexNode(child)
    }
  }

  const groups: SceneGroup[] = layout.groups.map((group) => {
    const nodes = group.nodes.map((node) => wrapNode(node))
    const sceneGroup: SceneGroup = {
      layout: group,
      nodes,
      isVisible: false,
    }

    for (const node of nodes) {
      indexNode(node)
    }

    return sceneGroup
  })

  return {
    groups,
    bbox: layout.bbox,
    nodeById,
  }
}
