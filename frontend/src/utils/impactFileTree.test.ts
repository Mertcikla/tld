import { describe, expect, it } from 'vitest'
import type { ImpactFile } from '../api/client'
import { buildImpactFileTree, flattenImpactFileTree } from './impactFileTree'

function file(path: string, added: number, removed: number): ImpactFile {
  return { path, change: 'modified', added, removed }
}

describe('impactFileTree', () => {
  it('nests files under directories, directories first', () => {
    const tree = buildImpactFileTree([file('src/pages/Repositories.tsx', 10, 2), file('src/api/client.ts', 4, 1), file('README.md', 1, 0)])

    expect(tree.map((node) => node.name)).toEqual(['src', 'README.md'])
    expect(tree[0].isDir).toBe(true)
    expect(tree[0].children.map((node) => node.name)).toEqual(['api', 'pages'])
    expect(tree[0].children[0].children[0].name).toBe('client.ts')
  })

  it('aggregates line counts for directories', () => {
    const tree = buildImpactFileTree([file('src/pages/Repositories.tsx', 10, 2), file('src/api/client.ts', 4, 1)])
    expect(tree[0].added).toBe(14)
    expect(tree[0].removed).toBe(3)
    expect(tree[0].children[0].added).toBe(4)
  })

  it('flattens the tree depth-first with indentation depth', () => {
    const flat = flattenImpactFileTree(buildImpactFileTree([file('src/api/client.ts', 4, 1)]))
    expect(flat.map((node) => [node.name, node.depth, node.isDir])).toEqual([
      ['src', 0, true],
      ['api', 1, true],
      ['client.ts', 2, false],
    ])
  })
})
