import type { ImpactChangeType, ImpactFile } from '../api/client'

export interface ImpactFileTreeNode {
  name: string
  path: string
  depth: number
  isDir: boolean
  added: number
  removed: number
  change?: ImpactChangeType
  children: ImpactFileTreeNode[]
}

interface MutableNode {
  name: string
  path: string
  isDir: boolean
  added: number
  removed: number
  change?: ImpactChangeType
  children: Map<string, MutableNode>
}

function finalize(children: Map<string, MutableNode>, depth: number): ImpactFileTreeNode[] {
  const nodes = [...children.values()].map((node) => {
    const sub = finalize(node.children, depth + 1)
    const added = node.isDir ? sub.reduce((sum, child) => sum + child.added, 0) : node.added
    const removed = node.isDir ? sub.reduce((sum, child) => sum + child.removed, 0) : node.removed
    return {
      name: node.name,
      path: node.path,
      depth,
      isDir: node.isDir,
      added,
      removed,
      change: node.change,
      children: sub,
    }
  })
  nodes.sort((a, b) => {
    if (a.isDir !== b.isDir) return a.isDir ? -1 : 1
    return a.name.localeCompare(b.name)
  })
  return nodes
}

export function buildImpactFileTree(files: ImpactFile[]): ImpactFileTreeNode[] {
  const root: MutableNode = { name: '', path: '', isDir: true, added: 0, removed: 0, children: new Map() }
  for (const file of files) {
    const segments = file.path.split('/').filter(Boolean)
    if (segments.length === 0) continue
    let current = root
    segments.forEach((segment, index) => {
      const isFile = index === segments.length - 1
      const childPath = current.path ? `${current.path}/${segment}` : segment
      let child = current.children.get(segment)
      if (!child) {
        child = { name: segment, path: childPath, isDir: !isFile, added: 0, removed: 0, children: new Map() }
        current.children.set(segment, child)
      }
      if (isFile) {
        child.isDir = false
        child.change = file.change
        child.added = file.added
        child.removed = file.removed
      }
      current = child
    })
  }
  return finalize(root.children, 0)
}

export function flattenImpactFileTree(nodes: ImpactFileTreeNode[]): ImpactFileTreeNode[] {
  const out: ImpactFileTreeNode[] = []
  const walk = (list: ImpactFileTreeNode[]) => {
    for (const node of list) {
      out.push(node)
      walk(node.children)
    }
  }
  walk(nodes)
  return out
}
