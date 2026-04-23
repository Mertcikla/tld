import type { ViewTreeNode } from '../../types'

export interface TreeNode extends ViewTreeNode {
  children: TreeNode[]
  depth: number
}

export interface NavItem {
  id: number
  name: string
  subtitle?: string
  elementId?: number
}
