import { createClient, ConnectError } from '@connectrpc/connect'
import { toJson } from '@bufbuild/protobuf'
import type {
  Connector,
  DependencyConnector,
  DependencyElement,
  ElementPlacement,
  ExploreData,
  LibraryElement,
  PlacedElement,
  Tag,
  TechnologyConnector,
  View,
  ViewConnector,
  ViewLayer,
  ViewPlacement,
  ViewTreeNode,
} from '../types'
import {
  WorkspaceService,
  CreateViewResponseSchema,
  UpdateViewResponseSchema,
  ListViewsResponseSchema,
  GetViewResponseSchema,
  GetWorkspaceResponseSchema,
  ListElementsResponseSchema,
  GetElementResponseSchema,
  CreateElementResponseSchema,
  UpdateElementResponseSchema,
  ListElementPlacementsResponseSchema,
  ListPlacementsResponseSchema,
  CreatePlacementResponseSchema,
  ListConnectorsResponseSchema,
  CreateConnectorResponseSchema,
  UpdateConnectorResponseSchema,
  ListElementNavigationsResponseSchema,
  ListViewLayersResponseSchema,
  CreateViewLayerResponseSchema,
  UpdateViewLayerResponseSchema,
  PlanElement,
  PlanConnector,
} from '@buf/tldiagramcom_diagram.bufbuild_es/diag/v1/workspace_service_pb'
import {
  DependencyService,
  ListDependenciesResponseSchema,
} from '@buf/tldiagramcom_diagram.bufbuild_es/diag/v1/dependency_service_pb'
import {
  ImportService,
} from '@buf/tldiagramcom_diagram.bufbuild_es/diag/v1/import_service_pb'
import {
  WorkspaceVersionService,
  type WorkspaceVersionInfo,
} from '@buf/tldiagramcom_diagram.bufbuild_es/diag/v1/workspace_version_service_pb'
import { transport } from './transport'
import { apiUrl, fetchApiAsset } from '../config/runtime'

export interface DependenciesResponse {
  elements: DependencyElement[]
  connectors: DependencyConnector[]
  totalCount?: number
}

export interface WatchRepository {
  id: number
  remote_url: string | null
  repo_root: string
  display_name: string
  branch: string | null
  head_commit: string | null
  identity_status: string
}

export interface WatchLock {
  id: number
  repository_id: number
  pid: number
  started_at: string
  heartbeat_at: string
  status: 'active' | 'paused' | 'stopping' | 'stale' | 'released' | string
}

export interface WatchStatus {
  active: boolean
  repository?: WatchRepository
  lock?: WatchLock
}

export interface WatchRepresentationSummary {
  repository_id: number
  raw_graph_hash?: string
  filter_settings_hash?: string
  representation_hash?: string
  last_status?: string
  last_started_at?: string
  last_finished_at?: string
  elements_created: number
  elements_updated: number
  connectors_created: number
  connectors_updated: number
  views_created: number
  diffs?: WatchDiff[]
}

export interface WatchEvent {
  type: string
  repository_id?: number
  message?: string
  at: string
  data?: unknown
  phase?: string
  watcher_mode?: string
  languages?: string[]
  changed_files?: number
  warnings?: string[]
}

export interface WatchVersion {
  id: number
  repository_id: number
  commit_hash: string
  commit_message?: string
  parent_commit_hash?: string
  branch?: string
  representation_hash: string
  workspace_version_id?: number
  created_at: string
}

export interface WatchDiff {
  id: number
  version_id: number
  owner_type: string
  owner_key: string
  change_type: string
  before_hash?: string
  after_hash?: string
  resource_type?: string
  resource_id?: number
  language?: string
  summary?: string
  added_lines?: number
  removed_lines?: number
}

export interface WorkspaceVersion {
  id: string
  version_id: string
  source: string
  parent_version_id?: string
  view_count: number
  element_count: number
  connector_count: number
  description?: string
  workspace_hash?: string
  created_at: string
}

export type SourceEditor = 'zed' | 'vscode'

// ─── RPC clients ─────────────────────────────────────────────────────────────

const workspaceClient = createClient(WorkspaceService, transport)
const dependencyClient = createClient(DependencyService, transport)
const importClient = createClient(ImportService, transport)
const workspaceVersionClient = createClient(WorkspaceVersionService, transport)
let dependencyConnectorsCache: Promise<DependencyConnector[]> | null = null

// ─── Helpers ─────────────────────────────────────────────────────────────────

async function rpc<T>(call: () => Promise<T>): Promise<T> {
  try {
    return await call()
  } catch (e) {
    if (e instanceof ConnectError) throw new Error(e.message)
    throw e
  }
}

function j<T>(schema: Parameters<typeof toJson>[0], msg: Parameters<typeof toJson>[1]): T {
  return toJson(schema, msg, { useProtoFieldName: true, emitDefaultValues: true }) as unknown as T
}

function timestampToISOString(value: WorkspaceVersionInfo['createdAt']): string {
  if (!value) return ''
  const seconds = typeof value.seconds === 'bigint' ? Number(value.seconds) : Number(value.seconds ?? 0)
  const nanos = Number(value.nanos ?? 0)
  return new Date(seconds * 1000 + Math.floor(nanos / 1_000_000)).toISOString()
}

function mapWorkspaceVersion(version: WorkspaceVersionInfo): WorkspaceVersion {
  return {
    id: version.id,
    version_id: version.versionId,
    source: version.source,
    parent_version_id: version.parentVersionId,
    view_count: version.viewCount,
    element_count: version.elementCount,
    connector_count: version.connectorCount,
    description: version.description,
    workspace_hash: version.workspaceHash,
    created_at: timestampToISOString(version.createdAt),
  }
}

async function fetchWorkspaceRaw(body: Record<string, unknown>) {
  const res = await fetchApiAsset(apiUrl('/diag.v1.WorkspaceService/GetWorkspace'), {
    method: 'POST',
    headers: {
      'Content-Type': 'application/json',
      'Connect-Protocol-Version': '1',
    },
    body: JSON.stringify(body),
  })
  if (!res.ok) {
    throw new Error(`GetWorkspace failed: ${res.statusText}`)
  }
  return res.json() as Promise<{
    views?: ProtoDiagram[]
    total_count?: number
    totalCount?: number
    content?: Record<string, { placements?: Record<string, unknown>[]; connectors?: Record<string, unknown>[] }>
    navigations?: Record<string, unknown>[]
  }>
}

// ─── Proto → frontend type mappers ───────────────────────────────────────────

interface ProtoDiagram {
  id: number
  ownerElementId?: number | null
  owner_element_id?: number | null
  name: string
  description?: string | null
  levelLabel?: string | null
  level_label?: string | null
  level?: number
  depth?: number
  createdAt?: string
  created_at?: string
  updatedAt?: string
  updated_at?: string
  parent_view_id?: number | null
  parentViewId?: number | null
  children?: ProtoDiagram[]
}

function mapDiagram(d: ProtoDiagram): ViewTreeNode {
  return {
    id: Number(d.id),
    owner_element_id: d.ownerElementId != null || d.owner_element_id != null
      ? Number(d.ownerElementId ?? d.owner_element_id)
      : null,
    name: d.name,
    description: d.description ?? null,
    level_label: d.levelLabel ?? d.level_label ?? null,
    level: d.level ?? 0,
    depth: d.depth ?? 0,
    created_at: d.createdAt ?? d.created_at ?? '',
    updated_at: d.updatedAt ?? d.updated_at ?? '',
    parent_view_id: d.parentViewId != null || d.parent_view_id != null
      ? Number(d.parentViewId ?? d.parent_view_id)
      : null,
    children: (d.children ?? []).map(mapDiagram),
  }
}

function diagramToView(d: ProtoDiagram): View {
  return {
    id: Number(d.id),
    owner_element_id: d.ownerElementId != null || d.owner_element_id != null
      ? Number(d.ownerElementId ?? d.owner_element_id)
      : null,
    name: d.name,
    label: d.levelLabel ?? d.level_label ?? null,
    is_root: (d.parent_view_id ?? null) === null,
    created_at: d.createdAt ?? d.created_at ?? new Date().toISOString(),
    updated_at: d.updatedAt ?? d.updated_at ?? new Date().toISOString(),
  }
}

function protoElementToLibrary(e: Record<string, unknown>): LibraryElement {
  return {
    id: Number(e.id ?? 0),
    name: String(e.name ?? ''),
    kind: (e.kind ?? null) as string | null,
    description: (e.description ?? null) as string | null,
    technology: (e.technology ?? null) as string | null,
    url: (e.url ?? null) as string | null,
    logo_url: (e.logo_url ?? e.logoUrl ?? null) as string | null,
    technology_connectors: ((e.technology_connectors ?? e.technologyLinks ?? []) as Array<{ type?: string; slug?: string; label?: string; is_primary_icon?: boolean; isPrimaryIcon?: boolean }>).map(tl => ({
      type: (tl.type ?? 'custom') as TechnologyConnector['type'],
      slug: tl.slug,
      label: tl.label ?? '',
      is_primary_icon: !!(tl.is_primary_icon ?? tl.isPrimaryIcon),
    })),
    tags: (e.tags ?? []) as string[],
    repo: (e.repo ?? null) as string | null,
    branch: (e.branch ?? null) as string | null,
    file_path: (e.file_path ?? null) as string | null,
    language: (e.language ?? null) as string | null,
    created_at: String(e.created_at ?? e.createdAt ?? new Date().toISOString()),
    updated_at: String(e.updated_at ?? e.updatedAt ?? new Date().toISOString()),
    has_view: Boolean(e.has_view ?? e.hasView ?? false),
    view_label: (e.view_label ?? e.viewLabel ?? null) as string | null,
  }
}

function libraryElementToDependency(element: LibraryElement): DependencyElement {
  return {
    id: String(element.id),
    name: element.name,
    type: element.kind,
    description: element.description,
    technology: element.technology,
    url: element.url,
    logo_url: element.logo_url,
    technology_connectors: element.technology_connectors,
    tags: element.tags,
    repo: element.repo,
    branch: element.branch,
    language: element.language,
    file_path: element.file_path,
    created_at: element.created_at,
    updated_at: element.updated_at,
  }
}

function protoPlacedElement(p: Record<string, unknown>): PlacedElement {
  return {
    id: Number(p.id ?? 0),
    view_id: Number(p.view_id ?? p.viewId ?? 0),
    element_id: Number(p.element_id ?? p.elementId ?? 0),
    position_x: Number(p.position_x ?? p.positionX ?? 0),
    position_y: Number(p.position_y ?? p.positionY ?? 0),
    name: String(p.name ?? ''),
    description: (p.description ?? null) as string | null,
    kind: (p.kind ?? null) as string | null,
    technology: (p.technology ?? null) as string | null,
    url: (p.url ?? null) as string | null,
    logo_url: (p.logo_url ?? p.logoUrl ?? null) as string | null,
    technology_connectors: ((p.technology_connect_ors ?? p.technology_connectors ?? p.technologyLinks ?? []) as Array<{ type?: string; slug?: string; label?: string; is_primary_icon?: boolean; isPrimaryIcon?: boolean }>).map(tl => ({
      type: (tl.type ?? 'custom') as TechnologyConnector['type'],
      slug: tl.slug,
      label: tl.label ?? '',
      is_primary_icon: !!(tl.is_primary_icon ?? tl.isPrimaryIcon),
    })),
    tags: (p.tags ?? []) as string[],
    repo: (p.repo ?? null) as string | null,
    branch: (p.branch ?? null) as string | null,
    file_path: (p.file_path ?? null) as string | null,
    language: (p.language ?? null) as string | null,
    has_view: Boolean(p.has_view ?? p.hasView ?? false),
    view_label: (p.view_label ?? p.viewLabel ?? null) as string | null,
  }
}

function protoConnector(e: Record<string, unknown>): Connector {
  return {
    id: Number(e.id ?? 0),
    view_id: Number(e.view_id ?? 0),
    source_element_id: Number(e.source_element_id ?? 0),
    target_element_id: Number(e.target_element_id ?? 0),
    label: (e.label ?? null) as string | null,
    description: (e.description ?? null) as string | null,
    relationship: (e.relationship ?? null) as string | null,
    direction: String(e.direction ?? 'forward'),
    style: String(e.style ?? 'bezier'),
    url: (e.url ?? null) as string | null,
    source_handle: (e.source_handle ?? null) as string | null,
    target_handle: (e.target_handle ?? null) as string | null,
    created_at: String(e.created_at ?? new Date().toISOString()),
    updated_at: String(e.updated_at ?? new Date().toISOString()),
  }
}

function protoDependencyConnector(e: Record<string, unknown>): DependencyConnector {
  return {
    id: String(e.id ?? 0),
    view_id: String(e.view_id ?? e.viewId ?? 0),
    source_element_id: String(e.source_element_id ?? e.sourceElementId ?? 0),
    target_element_id: String(e.target_element_id ?? e.targetElementId ?? 0),
    label: (e.label ?? null) as string | null,
    description: (e.description ?? null) as string | null,
    relationship_type: (e.relationship_type ?? e.relationshipType ?? e.relationship ?? null) as string | null,
    direction: String(e.direction ?? 'forward'),
    connector_type: String(e.connector_type ?? e.connectorType ?? e.style ?? 'solid'),
    url: (e.url ?? null) as string | null,
    source_handle: (e.source_handle ?? e.sourceHandle ?? null) as string | null,
    target_handle: (e.target_handle ?? e.targetHandle ?? null) as string | null,
    created_at: String(e.created_at ?? e.createdAt ?? ''),
    updated_at: String(e.updated_at ?? e.updatedAt ?? ''),
  }
}

function protoNavigation(n: Record<string, unknown>): ViewConnector {
  return {
    id: Number(n.id ?? 0),
    element_id: (n.element_id ?? null) as number | null,
    from_view_id: Number(n.from_view_id ?? 0),
    to_view_id: Number(n.to_view_id ?? 0),
    to_view_name: String(n.to_view_name ?? ''),
    relation_type: String(n.relation_type ?? 'child'),
  }
}

function protoDiagramPlacement(p: Record<string, unknown>): ViewPlacement {
  return {
    view_id: Number(p.view_id ?? 0),
    view_name: String(p.view_name ?? ''),
  }
}

function protoLayer(l: Record<string, unknown>): ViewLayer {
  return {
    id: Number(l.id ?? 0),
    diagram_id: Number(l.view_id ?? l.diagram_id ?? 0),
    name: String(l.name ?? ''),
    tags: (l.tags ?? []) as string[],
    color: String(l.color ?? ''),
    created_at: String(l.created_at ?? new Date().toISOString()),
    updated_at: String(l.updated_at ?? new Date().toISOString()),
  }
}

export const api = {
  system: {
    ready: (): Promise<{ ok: boolean }> =>
      rpc(() => workspaceClient.listViews({}).then(() => ({ ok: true }))),
  },

  user: {
    getPreferences: (): Promise<{ accent_color: string | null; background_color: string | null; element_color: string | null }> =>
      Promise.resolve({
        accent_color: localStorage.getItem('diag:accent-color'),
        background_color: localStorage.getItem('diag:background-color'),
        element_color: localStorage.getItem('diag:element-color'),
      }),
    updatePreferences: async (prefs: { accent_color?: string; background_color?: string; element_color?: string }): Promise<void> => {
      if (prefs.accent_color) localStorage.setItem('diag:accent-color', prefs.accent_color)
      if (prefs.background_color) localStorage.setItem('diag:background-color', prefs.background_color)
      if (prefs.element_color) localStorage.setItem('diag:element-color', prefs.element_color)
    },
  },

  elements: {
    list: (params?: { limit?: number; offset?: number; search?: string }): Promise<LibraryElement[]> =>
      rpc(async () => {
        const res = await workspaceClient.listElements({
          limit: params?.limit ?? 0,
          offset: params?.offset ?? 0,
          search: params?.search ?? '',
        })
        const json = j<{ elements: Record<string, unknown>[] }>(ListElementsResponseSchema, res)
        return (json.elements ?? []).map(protoElementToLibrary)
      }),

    get: (id: number): Promise<LibraryElement> =>
      rpc(async () => {
        const res = await workspaceClient.getElement({ elementId: id })
        const json = j<{ element: Record<string, unknown> }>(GetElementResponseSchema, res)
        return protoElementToLibrary(json.element ?? {})
      }),

    create: (data: Partial<LibraryElement>): Promise<LibraryElement> =>
      rpc(async () => {
        const res = await workspaceClient.createElement({
          name: data.name ?? '',
          kind: data.kind ?? '',
          description: data.description ?? undefined,
          technology: data.technology ?? undefined,
          url: data.url ?? undefined,
          logoUrl: data.logo_url ?? undefined,
          technologyLinks: (data.technology_connectors ?? []).map(tl => ({
            type: tl.type,
            slug: tl.slug ?? '',
            label: tl.label,
            isPrimaryIcon: tl.is_primary_icon ?? false,
          })),
          tags: data.tags ?? [],
          repo: data.repo ?? undefined,
          branch: data.branch ?? undefined,
          filePath: data.file_path ?? undefined,
          language: data.language ?? undefined,
        })
        const json = j<{ element: Record<string, unknown> }>(CreateElementResponseSchema, res)
        return protoElementToLibrary(json.element ?? {})
      }),

    update: (id: number, data: Partial<LibraryElement>): Promise<LibraryElement> =>
      rpc(async () => {
        const res = await workspaceClient.updateElement({
          elementId: id,
          name: data.name ?? undefined,
          kind: data.kind ?? undefined,
          description: data.description ?? undefined,
          technology: data.technology ?? undefined,
          url: data.url ?? undefined,
          logoUrl: data.logo_url ?? undefined,
          technologyLinks: (data.technology_connectors ?? []).map(tl => ({
            type: tl.type,
            slug: tl.slug ?? '',
            label: tl.label,
            isPrimaryIcon: tl.is_primary_icon ?? false,
          })),
          tags: data.tags ?? [],
          repo: data.repo ?? undefined,
          branch: data.branch ?? undefined,
          filePath: data.file_path ?? undefined,
          language: data.language ?? undefined,
        })
        const json = j<{ element: Record<string, unknown> }>(UpdateElementResponseSchema, res)
        return protoElementToLibrary(json.element ?? {})
      }),

    delete: (_orgId: string, id: number): Promise<void> =>
      rpc(async () => { await workspaceClient.deleteElement({ orgId: '', elementId: id }) }),

    placements: (id: number): Promise<ViewPlacement[]> =>
      rpc(async () => {
        const res = await workspaceClient.listElementPlacements({ elementId: id })
        const json = j<{ placements: Record<string, unknown>[] }>(ListElementPlacementsResponseSchema, res)
        return (json.placements ?? []).map(protoDiagramPlacement)
      }),
  },

  workspace: {
    orgs: {
      tagColors: {
        list: (): Promise<Tag[]> => Promise.resolve([]),
      },
    },

    elements: {
      list: (params?: { limit?: number; offset?: number; search?: string }): Promise<LibraryElement[]> =>
        api.elements.list(params),
      get: (id: number): Promise<LibraryElement> => api.elements.get(id),
      create: (data: Partial<LibraryElement>): Promise<LibraryElement> => api.elements.create(data),
      update: (id: number, data: Partial<LibraryElement>): Promise<LibraryElement> => api.elements.update(id, data),
      delete: (_orgId: string, id: number): Promise<void> => api.elements.delete('', id),
      placements: (id: number): Promise<ViewPlacement[]> => api.elements.placements(id),
      navigations: {
        list: (elementId: number, fromDiagramId: number): Promise<ViewConnector[]> =>
          rpc(async () => {
            const res = await workspaceClient.listElementNavigations({ elementId, fromViewId: fromDiagramId, toViewId: 0 })
            const json = j<{ navigations: Record<string, unknown>[] }>(ListElementNavigationsResponseSchema, res)
            return (json.navigations ?? []).map(protoNavigation)
          }),
        listParents: (elementId: number, toDiagramId: number): Promise<ViewConnector[]> =>
          rpc(async () => {
            const res = await workspaceClient.listElementNavigations({ elementId, fromViewId: 0, toViewId: toDiagramId })
            const json = j<{ navigations: Record<string, unknown>[] }>(ListElementNavigationsResponseSchema, res)
            return (json.navigations ?? []).map(protoNavigation)
          }),
      },
    },

    views: {
      list: (): Promise<View[]> =>
        rpc(async () => {
          const res = await workspaceClient.listViews({})
          const json = j<{ views: Record<string, unknown>[] }>(ListViewsResponseSchema, res)
          return (json.views ?? []).map(v => ({
            id: Number(v.id ?? 0),
            owner_element_id: v.owner_element_id != null ? Number(v.owner_element_id) : null,
            name: String(v.name ?? ''),
            label: (v.label ?? null) as string | null,
            is_root: Boolean(v.is_root ?? false),
            created_at: String(v.created_at ?? new Date().toISOString()),
            updated_at: String(v.updated_at ?? new Date().toISOString()),
          }))
        }),

      content: (id: number): Promise<{ placements: PlacedElement[]; connectors: Connector[] }> =>
        rpc(async () => {
          const [placementsRes, connectorsRes] = await Promise.all([
            workspaceClient.listPlacements({ viewId: id }),
            workspaceClient.listConnectors({ viewId: id }),
          ])
          const placementJson = j<{ placements: Record<string, unknown>[] }>(ListPlacementsResponseSchema, placementsRes)
          const connectorJson = j<{ connectors: Record<string, unknown>[] }>(ListConnectorsResponseSchema, connectorsRes)
          return {
            placements: (placementJson.placements ?? []).map(protoPlacedElement),
            connectors: (connectorJson.connectors ?? []).map(protoConnector),
          }
        }),

      tree: (): Promise<ViewTreeNode[]> =>
        rpc(async () => {
          const res = await workspaceClient.getWorkspace({ includeContent: false })
          const json = j<{ views: ProtoDiagram[] }>(GetWorkspaceResponseSchema, res)
          return (json.views ?? []).map(mapDiagram)
        }),

      // Lazy: root-level views only. Use for sidebar first render on huge workspaces.
      treeRoots: (opts: { limit?: number; offset?: number; search?: string } = {}): Promise<{ views: ViewTreeNode[]; totalCount: number }> =>
        rpc(async () => {
          const res = await workspaceClient.getWorkspace({
            includeContent: false,
            level: 0,
            limit: opts.limit ?? 0,
            offset: opts.offset ?? 0,
            search: opts.search ?? '',
          })
          const json = j<{ views: ProtoDiagram[]; total_count?: number }>(GetWorkspaceResponseSchema, res)
          return {
            views: (json.views ?? []).map(mapDiagram),
            totalCount: Number(json.total_count ?? 0),
          }
        }),

      // Lazy: direct children of a parent view. Used on tree node expand.
      treeChildren: (parentId: number, opts: { limit?: number; offset?: number } = {}): Promise<ViewTreeNode[]> =>
        rpc(async () => {
          const res = await workspaceClient.getWorkspace({
            includeContent: false,
            parentId,
            limit: opts.limit ?? 0,
            offset: opts.offset ?? 0,
          })
          const json = j<{ views: ProtoDiagram[] }>(GetWorkspaceResponseSchema, res)
          return (json.views ?? []).map(mapDiagram)
        }),

      treeAround: async (
        viewId: number,
        opts: { ancestorLevels?: number; descendantLevels?: number } = {},
      ): Promise<ViewTreeNode[]> => {
        const ancestorLevels = opts.ancestorLevels ?? 2
        const descendantLevels = opts.descendantLevels ?? 2
        const current = await api.workspace.views.get(viewId)

        const ancestors: ViewTreeNode[] = []
        let cursor: ViewTreeNode = current
        for (let depth = 0; depth < ancestorLevels && cursor.parent_view_id != null; depth += 1) {
          const parent = await api.workspace.views.get(cursor.parent_view_id)
          ancestors.unshift(parent)
          cursor = parent
        }

        const withDescendants = async (node: ViewTreeNode, remainingDepth: number): Promise<ViewTreeNode> => {
          const scoped: ViewTreeNode = { ...node, children: [] }
          if (remainingDepth <= 0) return scoped
          const children = await api.workspace.views.treeChildren(node.id)
          scoped.children = await Promise.all(children.map((child) => withDescendants(child, remainingDepth - 1)))
          return scoped
        }

        let scoped = await withDescendants(current, descendantLevels)
        for (let index = ancestors.length - 1; index >= 0; index -= 1) {
          scoped = { ...ancestors[index], children: [scoped] }
        }
        return [scoped]
      },

      gridData: (): Promise<{
        views: ViewTreeNode[]
        content: Record<number, { placements: PlacedElement[]; connectors: Connector[] }>
      }> =>
        rpc(async () => {
          const json = await fetchWorkspaceRaw({
            includeContent: true,
            hasView: true,
          })
          return {
            views: (json.views ?? []).map(mapDiagram),
            content: Object.fromEntries(
              Object.entries(json.content ?? {}).map(([key, value]) => [
                Number(key),
                {
                  placements: (value.placements ?? []).map(protoPlacedElement),
                  connectors: (value.connectors ?? []).map(protoConnector),
                },
              ])
            ),
          }
        }),

      get: (id: number): Promise<ViewTreeNode> =>
        rpc(async () => {
          const res = await workspaceClient.getView({ viewId: id })
          const json = j<{ view?: ProtoDiagram }>(GetViewResponseSchema, res)
          if (!json.view) throw new Error('View not found')
          return mapDiagram(json.view)
        }),

      create: (data: { name: string; label?: string; parent_view_id?: number | null }): Promise<View> =>
        rpc(async () => {
          const res = await workspaceClient.createView({
            name: data.name,
            levelLabel: data.label ?? undefined,
            ownerElementId: data.parent_view_id ?? undefined,
          })
          const json = j<{ view: ProtoDiagram }>(CreateViewResponseSchema, res)
          return diagramToView(json.view)
        }),

      update: (id: number, data: { name: string; label?: string }): Promise<View> =>
        rpc(async () => {
          const res = await workspaceClient.updateView({ viewId: id, name: data.name, levelLabel: data.label ?? undefined })
          const json = j<{ view: ProtoDiagram }>(UpdateViewResponseSchema, res)
          return diagramToView(json.view)
        }),

      rename: (id: number, name: string): Promise<View> =>
        rpc(async () => {
          const res = await workspaceClient.updateView({ viewId: id, name })
          const json = j<{ view: ProtoDiagram }>(UpdateViewResponseSchema, res)
          return diagramToView(json.view)
        }),

      setLevel: (id: number, level: number): Promise<void> =>
        rpc(async () => { await workspaceClient.setViewLevel({ viewId: id, level }) }),

      delete: (_orgId: string, id: number): Promise<void> =>
        rpc(async () => { await workspaceClient.deleteView({ orgId: '', viewId: id }) }),

      thumbnail: async (id: number): Promise<string | null> => {
        const res = await fetchApiAsset(apiUrl(`/views/${id}/thumbnail.svg`), {
          headers: { Accept: 'image/svg+xml' },
        })
        if (!res.ok) return null
        const svg = await res.text()
        return URL.createObjectURL(new Blob([svg], { type: 'image/svg+xml;charset=utf-8' }))
      },

      placements: {
        list: (diagramId: number): Promise<ElementPlacement[]> =>
          rpc(async () => {
            const res = await workspaceClient.listPlacements({ viewId: diagramId })
            const json = j<{ placements: Record<string, unknown>[] }>(ListPlacementsResponseSchema, res)
            return (json.placements ?? []).map(protoPlacedElement).map(pe => ({
              id: pe.id,
              view_id: pe.view_id,
              element_id: pe.element_id,
              position_x: pe.position_x,
              position_y: pe.position_y,
            }))
          }),

        add: (diagramId: number, elementId: number, x = 100, y = 100): Promise<ElementPlacement> =>
          rpc(async () => {
            const res = await workspaceClient.createPlacement({ viewId: diagramId, elementId, positionX: x, positionY: y })
            const json = j<{ placement: Record<string, unknown> }>(CreatePlacementResponseSchema, res)
            const pe = protoPlacedElement(json.placement ?? {})
            return { id: pe.id, view_id: pe.view_id, element_id: pe.element_id, position_x: pe.position_x, position_y: pe.position_y }
          }),

        updatePosition: (diagramId: number, elementId: number, x: number, y: number): Promise<void> =>
          rpc(async () => { await workspaceClient.updatePlacementPosition({ viewId: diagramId, elementId, positionX: x, positionY: y }) }),

        remove: (diagramId: number, elementId: number): Promise<void> =>
          rpc(async () => { await workspaceClient.deletePlacement({ viewId: diagramId, elementId }) }),
      },

      layers: {
        list: (diagramId: number): Promise<ViewLayer[]> =>
          rpc(async () => {
            const res = await workspaceClient.listViewLayers({ viewId: diagramId })
            const json = j<{ layers: Record<string, unknown>[] }>(ListViewLayersResponseSchema, res)
            return (json.layers ?? []).map(protoLayer)
          }),

        create: (diagramId: number, data: { name: string; tags: string[]; color?: string }): Promise<ViewLayer> =>
          rpc(async () => {
            const res = await workspaceClient.createViewLayer({ viewId: diagramId, name: data.name, tags: data.tags, color: data.color ?? '#888888' })
            const json = j<{ layer: Record<string, unknown> }>(CreateViewLayerResponseSchema, res)
            return protoLayer(json.layer ?? {})
          }),

        update: (_diagramId: number, layerId: number, data: Partial<ViewLayer>): Promise<ViewLayer> =>
          rpc(async () => {
            const res = await workspaceClient.updateViewLayer({ layerId, name: data.name ?? undefined, tags: data.tags ?? [], color: data.color ?? undefined })
            const json = j<{ layer: Record<string, unknown> }>(UpdateViewLayerResponseSchema, res)
            return protoLayer(json.layer ?? {})
          }),

        delete: (_diagramId: number, layerId: number): Promise<void> =>
          rpc(async () => { await workspaceClient.deleteViewLayer({ layerId }) }),
      },

    },

    connectors: {
      list: (diagramId: number): Promise<Connector[]> =>
        rpc(async () => {
          const res = await workspaceClient.listConnectors({ viewId: diagramId })
          const json = j<{ connectors: Record<string, unknown>[] }>(ListConnectorsResponseSchema, res)
          return (json.connectors ?? []).map(protoConnector)
        }),

      create: (
        diagramId: number,
        data: {
          source_element_id: number
          target_element_id: number
          label?: string
          description?: string
          relationship?: string
          direction?: string
          style?: string
          url?: string
          source_handle?: string | null
          target_handle?: string | null
        },
      ): Promise<Connector> =>
        rpc(async () => {
          const res = await workspaceClient.createConnector({
            viewId: diagramId,
            sourceElementId: data.source_element_id,
            targetElementId: data.target_element_id,
            label: data.label ?? undefined,
            description: data.description ?? undefined,
            relationship: data.relationship ?? undefined,
            direction: data.direction ?? undefined,
            style: data.style ?? undefined,
            url: data.url ?? undefined,
            sourceHandle: data.source_handle ?? undefined,
            targetHandle: data.target_handle ?? undefined,
          })
          const json = j<{ connector: Record<string, unknown> }>(CreateConnectorResponseSchema, res)
          return protoConnector(json.connector ?? {})
        }),

      update: (
        _diagramId: number,
        connectorId: number,
        data: {
          source_element_id?: number
          target_element_id?: number
          label?: string
          description?: string
          relationship?: string
          direction?: string
          style?: string
          url?: string
          source_handle?: string | null
          target_handle?: string | null
        },
      ): Promise<Connector> =>
        rpc(async () => {
          const res = await workspaceClient.updateConnector({
            connectorId,
            sourceElementId: data.source_element_id ?? undefined,
            targetElementId: data.target_element_id ?? undefined,
            label: data.label ?? undefined,
            description: data.description ?? undefined,
            relationship: data.relationship ?? undefined,
            direction: data.direction ?? undefined,
            style: data.style ?? undefined,
            url: data.url ?? undefined,
            sourceHandle: data.source_handle ?? undefined,
            targetHandle: data.target_handle ?? undefined,
          })
          const json = j<{ connector: Record<string, unknown> }>(UpdateConnectorResponseSchema, res)
          return protoConnector(json.connector ?? {})
        }),

      delete: (_orgId: string, connectorId: number): Promise<void> =>
        rpc(async () => { await workspaceClient.deleteConnector({ orgId: '', connectorId }) }),
    },
  },

  dependencies: {
    list: (params?: { limit?: number; offset?: number; search?: string }): Promise<DependenciesResponse> =>
      rpc(async () => {
        if (params) {
          if (!dependencyConnectorsCache) {
            dependencyConnectorsCache = workspaceClient.listConnectors({ viewId: 0 })
              .then((res) => {
                const connectorJson = j<{ connectors: Record<string, unknown>[] }>(ListConnectorsResponseSchema, res)
                return (connectorJson.connectors ?? []).map(protoDependencyConnector)
              })
          }
          const [elements, connectors] = await Promise.all([
            workspaceClient.listElements({
              limit: params.limit ?? 0,
              offset: params.offset ?? 0,
              search: params.search ?? '',
            }).then((res) => {
              const json = j<{ elements: Record<string, unknown>[] }>(ListElementsResponseSchema, res)
              return {
                elements: (json.elements ?? []).map(protoElementToLibrary),
                totalCount: res.pagination ? Number(res.pagination.totalCount) : undefined,
              }
            }),
            dependencyConnectorsCache,
          ])
          return {
            elements: elements.elements.map(libraryElementToDependency),
            connectors,
            totalCount: elements.totalCount,
          }
        }
        const res = await dependencyClient.listDependencies({})
        return j<DependenciesResponse>(ListDependenciesResponseSchema, res)
      }),
  },

  explore: {
    load: (): Promise<ExploreData & { password_required?: boolean }> =>
      rpc(async () => {
        const res = await workspaceClient.getWorkspace({ includeContent: true })
        const json = j<{
          views: ProtoDiagram[]
          content: Record<string, { placements: Record<string, unknown>[]; connectors: Record<string, unknown>[] }>
          navigations: Record<string, unknown>[]
        }>(GetWorkspaceResponseSchema, res)
        return {
          tree: (json.views ?? []).map(mapDiagram),
          views: Object.fromEntries(
            Object.entries(json.content ?? {}).map(([key, value]) => [
              key,
              {
                placements: (value.placements ?? []).map(protoPlacedElement),
                connectors: (value.connectors ?? []).map(protoConnector),
              },
            ])
          ),
          navigations: (json.navigations ?? []).map(protoNavigation),
          password_required: false,
        }
      }),

    loadShared: async (token: string, password?: string): Promise<ExploreData & { password_required?: boolean }> => {
      const init: RequestInit = {
        method: password ? 'POST' : 'GET',
        headers: { 'Content-Type': 'application/json' },
      }
      if (password) {
        init.body = JSON.stringify({ password })
      }
      const res = await fetch(apiUrl(`/shared/explore/${token}`), init)
      if (!res.ok) {
        throw new Error(`Failed to load shared diagram: ${res.statusText}`)
      }
      const data = await res.json() as {
        tree: ProtoDiagram[]
        views: Record<string, { elements: Record<string, unknown>[]; connectors: Record<string, unknown>[] }>
        password_required?: boolean
      }

      const tree = (data.tree ?? []).map(mapDiagram)
      const views = Object.fromEntries(
        Object.entries(data.views ?? {}).map(([key, value]) => [
          key,
          {
            placements: (value.elements ?? []).map(protoPlacedElement),
            connectors: (value.connectors ?? []).map(protoConnector),
          },
        ])
      )

      // Ensure that the share root is treated as a root (no parent) so that computeLayout
      // picks it up even if it was nested in the original workspace.
      const _sharedRoot = tree.find(n => String(n.id) === String(data.views[token]?.elements?.[0]?.view_id ?? ''))
      // Backend actually returns the shareToken.ViewID as the root of the tree it builds.
      // We should find the node in 'tree' that has no parent *within the returned set*.
      // For shared explore, the backend typically returns a tree starting at the shared view.
      tree.forEach(node => {
        // If the node's parent is not in our tree, it's a root for this shared view.
        const parentInTree = tree.find(n => n.id === node.parent_view_id)
        if (!parentInTree) {
          node.parent_view_id = null
        }
      })
      const navigations: ViewConnector[] = []
      const elementToChildView = new Map<number, ViewTreeNode>()
      const allViews: ViewTreeNode[] = []
      const flatTree = (nodes: ViewTreeNode[]) => {
        nodes.forEach(n => {
          allViews.push(n)
          if (n.owner_element_id) elementToChildView.set(n.owner_element_id, n)
          if (n.children) flatTree(n.children)
        })
      }
      flatTree(tree)

      Object.values(views).forEach((v) => {
        v.placements.forEach((p) => {
          const childView = elementToChildView.get(p.element_id)
          if (childView) {
            navigations.push({
              id: 0,
              element_id: p.element_id,
              from_view_id: p.view_id,
              to_view_id: childView.id,
              to_view_name: childView.name,
              relation_type: 'child',
            })
          }
        })
      })

      return {
        tree,
        views,
        navigations,
        password_required: data.password_required,
      }
    },
  },

  import: {
    resources: (orgId: string, data: { elements: PlanElement[]; connectors: PlanConnector[] }): Promise<{ view_id: number; view_url: string }> =>
      rpc(async () => {
        const res = await importClient.importResources({
          orgId,
          elements: data.elements,
          connectors: data.connectors,
        })
        return { view_id: res.viewId, view_url: res.viewUrl }
      }),
    parseStructurizr: (code: string): Promise<{ elements: PlanElement[]; connectors: PlanConnector[]; warnings: string[] }> =>
      rpc(async () => {
        const res = await importClient.parseStructurizr({ code })
        return {
          elements: res.elements,
          connectors: res.connectors,
          warnings: res.warnings,
        }
      }),
  },

  versions: {
    list: (limit = 50): Promise<WorkspaceVersion[]> =>
      rpc(async () => {
        const res = await workspaceVersionClient.listVersions({ limit })
        return (res.versions ?? []).map(mapWorkspaceVersion)
      }),
  },

  watch: {
    status: async (): Promise<WatchStatus> => {
      const res = await fetch(apiUrl('/watch/status'))
      if (!res.ok) throw new Error(`Failed to load watch status: ${res.statusText}`)
      return res.json()
    },
    websocketUrl: (): string => {
      const url = new URL(apiUrl('/watch/ws'), window.location.href)
      url.protocol = url.protocol === 'https:' ? 'wss:' : 'ws:'
      return url.toString()
    },
    repositories: async (): Promise<WatchRepository[]> => {
      const res = await fetch(apiUrl('/watch/repositories'))
      if (!res.ok) throw new Error(`Failed to load watch repositories: ${res.statusText}`)
      return res.json()
    },
    versions: async (repositoryId: number): Promise<WatchVersion[]> => {
      const res = await fetch(apiUrl(`/watch/repositories/${repositoryId}/versions`))
      if (!res.ok) throw new Error(`Failed to load watch versions: ${res.statusText}`)
      return res.json()
    },
    diffs: async (versionId: number, filters?: { owner_type?: string; change_type?: string; resource_type?: string; language?: string }): Promise<WatchDiff[]> => {
      const params = new URLSearchParams()
      if (filters?.owner_type) params.set('owner_type', filters.owner_type)
      if (filters?.change_type) params.set('change_type', filters.change_type)
      if (filters?.resource_type) params.set('resource_type', filters.resource_type)
      if (filters?.language) params.set('language', filters.language)
      const suffix = params.toString() ? `?${params}` : ''
      const res = await fetch(apiUrl(`/watch/versions/${versionId}/diffs${suffix}`))
      if (!res.ok) throw new Error(`Failed to load watch diffs: ${res.statusText}`)
      return res.json()
    },
  },

  editor: {
    open: async (input: { editor: SourceEditor; repo?: string | null; file_path: string; line?: number | null }): Promise<void> => {
      const res = await fetch(apiUrl('/editor/open'), {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({
          editor: input.editor,
          repo: input.repo ?? '',
          file_path: input.file_path,
          line: input.line ?? 0,
        }),
      })
      if (!res.ok) {
        const body = await res.json().catch(() => null) as { error?: string } | null
        throw new Error(body?.error || `Failed to open editor: ${res.statusText}`)
      }
    },
  },
}
