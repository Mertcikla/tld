import type { ViewTreeNode } from '../../types'
import type { TreeNode } from './types'

export const SWATCH_COLORS = [
  '#F56565', '#ED8936', '#ECC94B', '#48BB78', '#38B2AC',
  '#4299E1', '#667EEA', '#9F7AEA', '#ED64A6', '#A0AEC0',
]

export function pickUnusedColor(usedColors: string[]): string {
  const used = new Set(usedColors.map((c) => c.toLowerCase()))
  const pool = SWATCH_COLORS.filter((c) => !used.has(c.toLowerCase()))
  const source = pool.length > 0 ? pool : SWATCH_COLORS
  return source[Math.floor(Math.random() * source.length)]
}

export function buildTree(nodes: ViewTreeNode[]): TreeNode[] {
  const allNodes: ViewTreeNode[] = []
  const traverseFlatten = (n: ViewTreeNode) => {
    allNodes.push(n)
    if (n.children) n.children.forEach(traverseFlatten)
  }
  nodes.forEach(traverseFlatten)

  const map = new Map<number, TreeNode>()
  allNodes.forEach((n) => map.set(n.id, { ...n, children: [], depth: 0 }))

  const roots: TreeNode[] = []
  map.forEach((node) => {
    const pid = node.parent_view_id
    if (pid !== null && map.has(pid)) {
      map.get(pid)!.children.push(node)
    } else {
      roots.push(node)
    }
  })

  const assignDepthAndSort = (node: TreeNode, depth: number) => {
    node.depth = depth
    node.children.sort((a, b) => new Date(a.created_at).getTime() - new Date(b.created_at).getTime())
    node.children.forEach((c) => assignDepthAndSort(c, depth + 1))
  }
  roots.forEach((r) => assignDepthAndSort(r, 0))
  roots.sort((a, b) => new Date(a.created_at).getTime() - new Date(b.created_at).getTime())

  return roots
}

export function flattenTree(roots: TreeNode[]): TreeNode[] {
  const result: TreeNode[] = []
  const traverse = (node: TreeNode) => {
    result.push(node)
    node.children.forEach(traverse)
  }
  roots.forEach(traverse)
  return result
}
