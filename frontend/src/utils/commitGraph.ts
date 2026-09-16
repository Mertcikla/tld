import type { ImpactCommit } from '../api/client'

export interface GraphEdge {
  fromRow: number
  fromLane: number
  /** Null when the parent is outside the loaded list: render a downward stub. */
  toRow: number | null
  toLane: number
  /** Lane reserved for the branch while this edge is in flight. */
  railLane: number
}

export interface GraphRow {
  commit: ImpactCommit
  lane: number
  isMerge: boolean
}

export interface CommitGraphLayout {
  rows: GraphRow[]
  edges: GraphEdge[]
  laneCount: number
}

export const GRAPH_LANE_COLORS = [
  '#63B3ED',
  '#68D391',
  '#F6AD55',
  '#B794F4',
  '#4FD1C5',
  '#F6E05E',
  '#F687B3',
  '#76E4F7',
]

export interface ParsedCommitRefs {
  isHead: boolean
  branches: string[]
  remotes: string[]
  tags: string[]
}

/** Split `git log --decorate=short` refs into head/branch/remote/tag buckets. */
export function parseCommitRefs(refs: string[]): ParsedCommitRefs {
  const out: ParsedCommitRefs = { isHead: false, branches: [], remotes: [], tags: [] }
  for (const ref of refs) {
    const trimmed = ref.trim()
    if (!trimmed) continue
    if (trimmed === 'HEAD') {
      out.isHead = true
      continue
    }
    if (trimmed.startsWith('HEAD -> ')) {
      out.isHead = true
      const branch = trimmed.slice('HEAD -> '.length).trim()
      if (branch) out.branches.push(branch)
      continue
    }
    if (trimmed.startsWith('tag: ')) {
      const tag = trimmed.slice('tag: '.length).trim()
      if (tag) out.tags.push(tag)
      continue
    }
    if (trimmed.includes('/')) {
      out.remotes.push(trimmed)
      continue
    }
    out.branches.push(trimmed)
  }
  return out
}

/**
 * Assign lanes to newest-first commits and derive rail edges.
 *
 * A commit reuses the lane reserved for it by an already-placed child.
 * Otherwise it takes the first free lane. The first parent inherits the
 * commit's lane; extra parents (fork points) take free lanes. Edges run from
 * each commit to the row of each parent, or to a downward stub when the
 * parent is not in the list.
 */
export function layoutCommitGraph(commits: ImpactCommit[]): CommitGraphLayout {
  const rows: GraphRow[] = commits.map((commit) => ({
    commit,
    lane: 0,
    isMerge: commit.parents.length > 1,
  }))
  const indexBySha = new Map<string, number>()
  commits.forEach((commit, index) => {
    if (!indexBySha.has(commit.sha)) indexBySha.set(commit.sha, index)
  })

  const lanes: (string | null)[] = []
  const rails: number[][] = []
  const takeLane = (sha: string): number => {
    const reserved = lanes.indexOf(sha)
    if (reserved >= 0) return reserved
    const free = lanes.indexOf(null)
    if (free >= 0) {
      lanes[free] = sha
      return free
    }
    lanes.push(sha)
    return lanes.length - 1
  }

  commits.forEach((commit, row) => {
    const reserved = lanes.indexOf(commit.sha)
    const lane = reserved >= 0 ? reserved : takeLane(commit.sha)
    rows[row].lane = lane
    // A branch can reserve more than one incoming rail. Release every rail
    // that arrives at this commit together so a live connection cannot be
    // reused by an unrelated commit before it reaches its parent.
    lanes.forEach((sha, index) => {
      if (sha === commit.sha) lanes[index] = null
    })
    rails[row] = []
    commit.parents.forEach((parent, parentIndex) => {
      if (parentIndex === 0) {
        lanes[lane] = parent
        rails[row].push(lane)
      } else {
        rails[row].push(takeLane(parent))
      }
    })
  })

  const edges: GraphEdge[] = []
  commits.forEach((commit, row) => {
    const fromLane = rows[row].lane
    commit.parents.forEach((parent, parentIndex) => {
      const toRow = indexBySha.get(parent) ?? null
      const railLane = rails[row][parentIndex]
      const toLane = toRow !== null ? rows[toRow].lane : railLane
      edges.push({ fromRow: row, fromLane, toRow, toLane, railLane })
    })
  })

  return { rows, edges, laneCount: lanes.length }
}
