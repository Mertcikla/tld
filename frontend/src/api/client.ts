import { createClient, ConnectError } from '@connectrpc/connect'
import { toJson } from '@bufbuild/protobuf'
import type {
  Connector,
  DependencyConnector,
  DependencyElement,
  ElementPlacement,
  ElementReactionSummary,
  ExploreData,
  LibraryElement,
  NoiseGateInitialization,
  PlacedElement,
  Tag,
  TechnologyCatalogItem,
  ThreadResolveEvent,
  View,
  ViewComment,
  ViewConnector,
  ViewMarkdownDocument,
  ViewLayer,
  ViewPlacement,
  ViewThread,
  ViewTreeNode,
  VisibilityOverride,
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
  CreateCustomTechnologyResponseSchema,
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
  type ViewContent,
} from '@buf/tldiagramcom_diagram.bufbuild_es/diag/v1/workspace_service_pb'
import {
  DependencyService,
  ListDependenciesResponseSchema,
} from '@buf/tldiagramcom_diagram.bufbuild_es/diag/v1/dependency_service_pb'
import {
  ImportService,
} from '@buf/tldiagramcom_diagram.bufbuild_es/diag/v1/import_service_pb'
import {
  MermaidDirection as MermaidDirectionProto,
  MermaidMarkdownSyncStatus as MermaidMarkdownSyncStatusProto,
  MermaidService,
  type MermaidImportSummary as ProtoMermaidImportSummary,
  type MermaidMarkdownBlockInfo,
} from '@buf/tldiagramcom_diagram.bufbuild_es/diag/v1/mermaid_service_pb'
import {
  WorkspaceVersionService,
  type WorkspaceVersionInfo,
} from '@buf/tldiagramcom_diagram.bufbuild_es/diag/v1/workspace_version_service_pb'
import {
  OrgService,
  ListTagColorsResponseSchema,
} from '@buf/tldiagramcom_diagram.bufbuild_es/diag/v1/org_service_pb'
import {
  CollaborationService,
  ListThreadsResponseSchema,
  CreateThreadResponseSchema,
  AddCommentResponseSchema,
  ListReactionsResponseSchema,
} from '@buf/tldiagramcom_diagram.bufbuild_es/diag/v1/collaboration_service_pb'
import { transport } from './transport'
import { apiUrl, fetchApiAsset, isWailsApp } from '../config/runtime'
import {
  normalizeConnectorRouteStyle,
  normalizeLogoUrl,
  normalizeTechnologyConnectors,
} from './client-normalize'

export {
  normalizeConnectorRouteStyle,
  normalizeLogoUrl,
  normalizeTechnologyConnectors,
} from './client-normalize'

const localWorkspaceOrgId = '11111111-1111-1111-1111-111111111111'
const orgIdOrLocal = (orgId?: string | null) => orgId || localWorkspaceOrgId

export function watchWebSocketUrl(): string {
  const baseUrl = isWailsApp ? window.__TLD_SERVER_URL__ : window.location.href
  if (isWailsApp && !baseUrl) {
    throw new Error('Desktop server URL is not configured')
  }
  const url = new URL(apiUrl('/watch/ws'), baseUrl)
  url.protocol = url.protocol === 'https:' ? 'wss:' : 'ws:'
  return url.toString()
}

async function responseError(res: Response, fallback: string): Promise<Error> {
  const body = await res.json().catch(() => null) as { error?: string; message?: string } | null
  return new Error(body?.message || body?.error || `${fallback}: ${res.statusText}`)
}

const WORKSPACE_CONNECT_SERVICE = 'diag.v1.WorkspaceService'

async function connectJsonRpc<T>(
  method: string,
  body: Record<string, unknown>,
  options: { allowNotFound?: boolean } = {},
): Promise<T | null> {
  const res = await fetch(apiUrl(`/${WORKSPACE_CONNECT_SERVICE}/${method}`), {
    method: 'POST',
    headers: {
      'Content-Type': 'application/json',
      'Connect-Protocol-Version': '1',
    },
    body: JSON.stringify(body),
  })
  if (options.allowNotFound && res.status === 404) return null
  if (!res.ok) throw await responseError(res, `Failed to ${method}`)
  return await res.json() as T
}

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
  connected_clients?: number
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

export interface WatchContextActionResponse {
  repository_id: number
  action: 'show' | 'hide' | 'clean' | string
  policies_created: number
  policies_updated: number
  policies_deactivated: number
  owners_affected: number
  tier_before: number
  tier_after: number
  max_tier: number
  elements_added: number
  connectors_added: number
  views_added: number
  elements_removed: number
  connectors_removed: number
  views_removed: number
  representation: {
    repository_id: number
    representation_run_id: number
    filter_run_id: number
    raw_graph_hash: string
    filter_settings_hash: string
    representation_hash: string
  }
  summary: WatchRepresentationSummary
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
export type MermaidDirection = 'TB' | 'TD' | 'BT' | 'RL' | 'LR'
export type MermaidImportFormat = 'mermaid' | 'structurizr'
export type MermaidMarkdownSyncStatus = 'missing' | 'synced' | 'stale' | 'other' | 'unlinked'

export interface ParsedImport {
  format: MermaidImportFormat
  elements: PlanElement[]
  connectors: PlanConnector[]
  warnings: string[]
  direction: MermaidDirection
  source: string
}

export interface MermaidImportSummary {
  resolvedElementCount: number
  createdElementCount: number
  resolvedConnectorCount: number
  createdConnectorCount: number
  importedElementIds: Set<number>
  resolvedElementIds: number[]
  createdElementIds: number[]
  resolvedConnectorIds: number[]
  createdConnectorIds: number[]
}

export interface MermaidImportResult {
  summary: MermaidImportSummary
  warnings: string[]
  content?: ViewContent
}

export interface MermaidMarkdownBlock extends MermaidMarkdownBlockInfo {
  syncStatusValue: MermaidMarkdownSyncStatus
}

// ─── RPC clients ─────────────────────────────────────────────────────────────

const workspaceClient = createClient(WorkspaceService, transport)
const dependencyClient = createClient(DependencyService, transport)
const importClient = createClient(ImportService, transport)
const mermaidClient = createClient(MermaidService, transport)
const workspaceVersionClient = createClient(WorkspaceVersionService, transport)
const orgClient = createClient(OrgService, transport)
const collaborationClient = createClient(CollaborationService, transport)

// ─── Helpers ─────────────────────────────────────────────────────────────────

export async function rpc<T>(call: () => Promise<T>): Promise<T> {
  try {
    return await call()
  } catch (e) {
    if (e instanceof ConnectError) throw new Error(e.message)
    throw e
  }
}

export function j<T>(schema: Parameters<typeof toJson>[0], msg: Parameters<typeof toJson>[1]): T {
  return toJson(schema, msg, { useProtoFieldName: true, emitDefaultValues: true }) as unknown as T
}

function timestampToISOString(value?: WorkspaceVersionInfo['createdAt'] | null): string {
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

function mapViewComment(raw: Record<string, unknown>): ViewComment {
  const viewId = Number(raw.view_id ?? 0)
  return {
    id: Number(raw.id ?? 0),
    org_id: String(raw.org_id ?? ''),
    view_id: viewId,
    diagram_id: viewId,
    thread_id: Number(raw.thread_id ?? 0),
    author_id: String(raw.author_id ?? ''),
    author_username: String(raw.author_username ?? ''),
    body: String(raw.body ?? ''),
    created_at: String(raw.created_at ?? ''),
    updated_at: String(raw.updated_at ?? ''),
  }
}

function mapViewThread(raw: Record<string, unknown>): ViewThread {
  const viewId = Number(raw.view_id ?? 0)
  return {
    id: Number(raw.id ?? 0),
    org_id: String(raw.org_id ?? ''),
    view_id: viewId,
    diagram_id: viewId,
    element_id: raw.element_id != null ? Number(raw.element_id) : null,
    connector_id: raw.connector_id != null ? Number(raw.connector_id) : null,
    created_by: String(raw.created_by ?? ''),
    created_by_username: String(raw.created_by_username ?? ''),
    status: raw.status === 'resolved' ? 'resolved' : 'open',
    created_at: String(raw.created_at ?? ''),
    resolved_at: raw.resolved_at ? String(raw.resolved_at) : null,
    comments: Array.isArray(raw.comments)
      ? raw.comments.map((item) => mapViewComment((item ?? {}) as Record<string, unknown>))
      : [],
  }
}

function mapReactionSummary(raw: Record<string, unknown>): ElementReactionSummary {
  return {
    element_id: Number(raw.element_id ?? 0),
    emoji: String(raw.emoji ?? ''),
    count: Number(raw.count ?? 0),
    reacted_by_me: Boolean(raw.reacted_by_me ?? false),
  }
}

function mapMermaidDirection(value: MermaidDirectionProto): MermaidDirection {
  if (value === MermaidDirectionProto.TB) return 'TB'
  if (value === MermaidDirectionProto.TD) return 'TD'
  if (value === MermaidDirectionProto.BT) return 'BT'
  if (value === MermaidDirectionProto.RL) return 'RL'
  return 'LR'
}

function mapMermaidMarkdownSyncStatus(value: MermaidMarkdownSyncStatusProto): MermaidMarkdownSyncStatus {
  if (value === MermaidMarkdownSyncStatusProto.SYNCED) return 'synced'
  if (value === MermaidMarkdownSyncStatusProto.STALE) return 'stale'
  if (value === MermaidMarkdownSyncStatusProto.OTHER_VIEW) return 'other'
  if (value === MermaidMarkdownSyncStatusProto.UNLINKED) return 'unlinked'
  return 'missing'
}

function mapMermaidImportSummary(summary?: ProtoMermaidImportSummary): MermaidImportSummary {
  return {
    resolvedElementCount: summary?.resolvedElementCount ?? 0,
    createdElementCount: summary?.createdElementCount ?? 0,
    resolvedConnectorCount: summary?.resolvedConnectorCount ?? 0,
    createdConnectorCount: summary?.createdConnectorCount ?? 0,
    importedElementIds: new Set(summary?.importedElementIds ?? []),
    resolvedElementIds: summary?.resolvedElementIds ?? [],
    createdElementIds: summary?.createdElementIds ?? [],
    resolvedConnectorIds: summary?.resolvedConnectorIds ?? [],
    createdConnectorIds: summary?.createdConnectorIds ?? [],
  }
}

function mapMermaidMarkdownBlock(block: MermaidMarkdownBlockInfo): MermaidMarkdownBlock {
  return {
    ...block,
    syncStatusValue: mapMermaidMarkdownSyncStatus(block.syncStatus),
  }
}

// ─── Proto → frontend type mappers ───────────────────────────────────────────

export interface ProtoDiagram {
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
  tags?: string[]
  parent_view_id?: number | null
  parentViewId?: number | null
  markdown?: ProtoViewMarkdownDocument | null
  children?: ProtoDiagram[]
}

interface ProtoViewMarkdownDocument {
  path?: string
  isManaged?: boolean
  is_managed?: boolean
  updatedAt?: string
  updated_at?: string
  sourceKind?: string
  source_kind?: string
  exists?: boolean
  writable?: boolean
  canEdit?: boolean
  can_edit?: boolean
  gitState?: string
  git_state?: string
  repoRelativePath?: string
  repo_relative_path?: string
  linkedViewCount?: number
  linked_view_count?: number
  fileVersion?: string
  file_version?: string
}

export function mapViewMarkdown(doc: ProtoViewMarkdownDocument | null | undefined): ViewMarkdownDocument | null {
  if (!doc?.path) return null
  return {
    path: String(doc.path),
    is_managed: Boolean(doc.isManaged ?? doc.is_managed),
    updated_at: String(doc.updatedAt ?? doc.updated_at ?? ''),
    source_kind: String(doc.sourceKind ?? doc.source_kind ?? ''),
    exists: Boolean(doc.exists ?? true),
    writable: Boolean(doc.writable ?? true),
    can_edit: Boolean(doc.canEdit ?? doc.can_edit ?? true),
    git_state: String(doc.gitState ?? doc.git_state ?? 'unknown'),
    repo_relative_path: doc.repoRelativePath ?? doc.repo_relative_path,
    linked_view_count: Number(doc.linkedViewCount ?? doc.linked_view_count ?? 0),
    file_version: String(doc.fileVersion ?? doc.file_version ?? ''),
  }
}

export function mapDiagram(d: ProtoDiagram): ViewTreeNode {
  return {
    id: Number(d.id),
    owner_element_id: d.ownerElementId != null || d.owner_element_id != null
      ? Number(d.ownerElementId ?? d.owner_element_id)
      : null,
    name: d.name,
    description: d.description ?? null,
    level_label: d.levelLabel ?? d.level_label ?? null,
    tags: (d.tags ?? []) as string[],
    level: d.level ?? 0,
    depth: d.depth ?? 0,
    created_at: d.createdAt ?? d.created_at ?? '',
    updated_at: d.updatedAt ?? d.updated_at ?? '',
    parent_view_id: d.parentViewId != null || d.parent_view_id != null
      ? Number(d.parentViewId ?? d.parent_view_id)
      : null,
    markdown: mapViewMarkdown(d.markdown),
    children: (d.children ?? []).map(mapDiagram),
  }
}

function findViewPath(nodes: ViewTreeNode[], viewId: number, path: ViewTreeNode[] = []): ViewTreeNode[] | null {
  for (const node of nodes) {
    const nextPath = [...path, node]
    if (node.id === viewId) return nextPath
    const found = findViewPath(node.children ?? [], viewId, nextPath)
    if (found) return found
  }
  return null
}

function pruneDescendants(node: ViewTreeNode, remainingDepth: number): ViewTreeNode {
  return {
    ...node,
    children: remainingDepth <= 0
      ? []
      : (node.children ?? []).map((child) => pruneDescendants(child, remainingDepth - 1)),
  }
}

function pruneTreeAround(nodes: ViewTreeNode[], viewId: number, ancestorLevels: number, descendantLevels: number): ViewTreeNode[] {
  const path = findViewPath(nodes, viewId)
  if (!path) return []

  const start = Math.max(0, path.length - 1 - ancestorLevels)
  let scoped = pruneDescendants(path[path.length - 1], descendantLevels)
  for (let index = path.length - 2; index >= start; index -= 1) {
    scoped = { ...path[index], children: [scoped] }
  }
  return [scoped]
}

export function diagramToView(d: ProtoDiagram): View {
  return {
    id: Number(d.id),
    owner_element_id: d.ownerElementId != null || d.owner_element_id != null
      ? Number(d.ownerElementId ?? d.owner_element_id)
      : null,
    name: d.name,
    label: d.levelLabel ?? d.level_label ?? null,
    tags: (d.tags ?? []) as string[],
    is_root: (d.parentViewId ?? d.parent_view_id ?? null) === null,
    created_at: d.createdAt ?? d.created_at ?? new Date().toISOString(),
    updated_at: d.updatedAt ?? d.updated_at ?? new Date().toISOString(),
  }
}

export function protoElementToLibrary(e: Record<string, unknown>): LibraryElement {
  const technologyConnectors = normalizeTechnologyConnectors(e.technology_connectors ?? e.technologyLinks)
  return {
    id: Number(e.id ?? 0),
    name: String(e.name ?? ''),
    kind: (e.kind ?? null) as string | null,
    description: (e.description ?? null) as string | null,
    technology: (e.technology ?? null) as string | null,
    url: (e.url ?? null) as string | null,
    logo_url: normalizeLogoUrl(e.logo_url ?? e.logoUrl, technologyConnectors),
    technology_connectors: technologyConnectors,
    tags: (e.tags ?? []) as string[],
    repo: (e.repo ?? null) as string | null,
    branch: (e.branch ?? null) as string | null,
    file_path: (e.file_path ?? null) as string | null,
    language: (e.language ?? null) as string | null,
    created_at: String(e.created_at ?? e.createdAt ?? new Date().toISOString()),
    updated_at: String(e.updated_at ?? e.updatedAt ?? new Date().toISOString()),
    has_view: Boolean(e.has_view ?? e.hasView ?? false),
    view_label: (e.view_label ?? e.viewLabel ?? null) as string | null,
    bypass_noise_gate: Boolean(e.bypass_noise_gate ?? e.bypassNoiseGate ?? false),
  }
}

export function libraryElementToDependency(element: LibraryElement): DependencyElement {
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
    bypass_noise_gate: element.bypass_noise_gate ?? false,
    repo: element.repo,
    branch: element.branch,
    language: element.language,
    file_path: element.file_path,
    created_at: element.created_at,
    updated_at: element.updated_at,
  }
}

export function protoPlacedElement(p: Record<string, unknown>): PlacedElement {
  const technologyConnectors = normalizeTechnologyConnectors(p.technology_connect_ors ?? p.technology_connectors ?? p.technologyLinks)
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
    logo_url: normalizeLogoUrl(p.logo_url ?? p.logoUrl, technologyConnectors),
    technology_connectors: technologyConnectors,
    tags: (p.tags ?? []) as string[],
    repo: (p.repo ?? null) as string | null,
    branch: (p.branch ?? null) as string | null,
    file_path: (p.file_path ?? null) as string | null,
    language: (p.language ?? null) as string | null,
    has_view: Boolean(p.has_view ?? p.hasView ?? false),
    view_label: (p.view_label ?? p.viewLabel ?? null) as string | null,
    bypass_noise_gate: Boolean(p.bypass_noise_gate ?? p.bypassNoiseGate ?? false),
  }
}

export function protoConnector(e: Record<string, unknown>): Connector {
  return {
    id: Number(e.id ?? 0),
    view_id: Number(e.view_id ?? e.viewId ?? 0),
    source_element_id: Number(e.source_element_id ?? e.sourceElementId ?? 0),
    target_element_id: Number(e.target_element_id ?? e.targetElementId ?? 0),
    label: (e.label ?? null) as string | null,
    description: (e.description ?? null) as string | null,
    relationship: (e.relationship ?? null) as string | null,
    direction: String(e.direction ?? 'forward'),
    style: normalizeConnectorRouteStyle(e.style),
    url: (e.url ?? null) as string | null,
    source_handle: (e.source_handle ?? e.sourceHandle ?? null) as string | null,
    target_handle: (e.target_handle ?? e.targetHandle ?? null) as string | null,
    tags: (e.tags ?? []) as string[],
    created_at: String(e.created_at ?? e.createdAt ?? new Date().toISOString()),
    updated_at: String(e.updated_at ?? e.updatedAt ?? new Date().toISOString()),
  }
}

export function normalizeFrontendImportElements(elements: PlanElement[]): PlanElement[] {
  return elements.map((element) => {
    const raw = element as Record<string, unknown>
    if (raw.bypassNoiseGate != null || raw.bypass_noise_gate != null) {
      return element
    }
    return { ...element, bypassNoiseGate: false } as PlanElement
  })
}

function normalizeVisibilityOverride(value: Record<string, unknown>, fallback?: { viewId: number; resourceType: VisibilityOverride['resource_type']; resourceId: number; levelDelta: number }): VisibilityOverride {
  const rawLevelDelta = value.level_delta ?? value.levelDelta
  return {
    view_id: Number(value.view_id ?? value.viewId ?? fallback?.viewId ?? 0),
    resource_type: (value.resource_type ?? value.resourceType ?? fallback?.resourceType ?? 'element') as VisibilityOverride['resource_type'],
    resource_id: Number(value.resource_id ?? value.resourceId ?? fallback?.resourceId ?? 0),
    level_delta: rawLevelDelta != null ? Number(rawLevelDelta) : (fallback?.levelDelta ?? 0),
    created_at: (value.created_at ?? value.createdAt) as string | undefined,
    updated_at: (value.updated_at ?? value.updatedAt) as string | undefined,
  }
}

export function protoDependencyConnector(e: Record<string, unknown>): DependencyConnector {
  return {
    id: String(e.id ?? 0),
    view_id: String(e.view_id ?? e.viewId ?? 0),
    source_element_id: String(e.source_element_id ?? e.sourceElementId ?? 0),
    target_element_id: String(e.target_element_id ?? e.targetElementId ?? 0),
    label: (e.label ?? null) as string | null,
    description: (e.description ?? null) as string | null,
    relationship_type: (e.relationship_type ?? e.relationshipType ?? e.relationship ?? null) as string | null,
    direction: String(e.direction ?? 'forward'),
    connector_type: String(e.connector_type ?? e.connectorType ?? e.style ?? 'bezier'),
    url: (e.url ?? null) as string | null,
    source_handle: (e.source_handle ?? e.sourceHandle ?? null) as string | null,
    target_handle: (e.target_handle ?? e.targetHandle ?? null) as string | null,
    tags: (e.tags ?? []) as string[],
    created_at: String(e.created_at ?? e.createdAt ?? ''),
    updated_at: String(e.updated_at ?? e.updatedAt ?? ''),
  }
}

export function protoNavigation(n: Record<string, unknown>): ViewConnector {
  return {
    id: Number(n.id ?? 0),
    element_id: (n.element_id ?? null) as number | null,
    from_view_id: Number(n.from_view_id ?? 0),
    to_view_id: Number(n.to_view_id ?? 0),
    to_view_name: String(n.to_view_name ?? ''),
    relation_type: String(n.relation_type ?? 'child'),
  }
}

export function protoDiagramPlacement(p: Record<string, unknown>): ViewPlacement {
  return {
    view_id: Number(p.view_id ?? 0),
    view_name: String(p.view_name ?? ''),
  }
}

function protoTechnologyCatalogItem(raw: Record<string, unknown>): TechnologyCatalogItem {
  return {
    iconUrl: String(raw.icon_url ?? raw.iconUrl ?? ''),
    name: String(raw.name ?? ''),
    provider: typeof (raw.provider ?? undefined) === 'string' ? String(raw.provider) : undefined,
    docsUrl: typeof (raw.docs_url ?? raw.docsUrl ?? undefined) === 'string' ? String(raw.docs_url ?? raw.docsUrl) : undefined,
    description: typeof (raw.description ?? undefined) === 'string' ? String(raw.description) : undefined,
    websiteUrl: typeof (raw.website_url ?? raw.websiteUrl ?? undefined) === 'string' ? String(raw.website_url ?? raw.websiteUrl) : undefined,
    nameShort: String(raw.name_short ?? raw.nameShort ?? ''),
    defaultSlug: String(raw.default_slug ?? raw.defaultSlug ?? ''),
    aliases: Array.isArray(raw.aliases) ? raw.aliases.map(String) : undefined,
  }
}

export function protoLayer(l: Record<string, unknown>): ViewLayer {
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

  technology: {
    createCustom: (data: {
      name: string
      name_short?: string
      aliases?: string[]
      icon: Uint8Array
      media_type: string
      preferred_slug?: string
    }): Promise<TechnologyCatalogItem> =>
      rpc(async () => {
        const res = await workspaceClient.createCustomTechnology({
          name: data.name,
          nameShort: data.name_short || undefined,
          aliases: data.aliases ?? [],
          icon: data.icon,
          mediaType: data.media_type,
          preferredSlug: data.preferred_slug || undefined,
        } as Parameters<typeof workspaceClient.createCustomTechnology>[0])
        const json = j<{ item?: Record<string, unknown> }>(CreateCustomTechnologyResponseSchema, res)
        if (!json.item) throw new Error('Custom technology response was empty')
        return protoTechnologyCatalogItem(json.item)
      }),
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
        const request = {
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
          bypassNoiseGate: data.bypass_noise_gate ?? false,
        }
        const res = await workspaceClient.createElement(request as Parameters<typeof workspaceClient.createElement>[0])
        const json = j<{ element: Record<string, unknown> }>(CreateElementResponseSchema, res)
        return protoElementToLibrary(json.element ?? {})
      }),

    update: (id: number, data: Partial<LibraryElement>): Promise<LibraryElement> =>
      rpc(async () => {
        const request = {
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
          bypassNoiseGate: data.bypass_noise_gate,
        }
        const res = await workspaceClient.updateElement(request as Parameters<typeof workspaceClient.updateElement>[0])
        const json = j<{ element: Record<string, unknown> }>(UpdateElementResponseSchema, res)
        return protoElementToLibrary(json.element ?? {})
      }),

    delete: (orgId: string, id: number): Promise<void> =>
      rpc(async () => { await workspaceClient.deleteElement({ orgId: orgIdOrLocal(orgId), elementId: id }) }),

    merge: (sourceId: number, survivorId: number, resolved: Partial<{
      kind: string | null
      description: string | null
      repo: string | null
      branch: string | null
      file_path: string | null
      language: string | null
    }>): Promise<{ survivor: LibraryElement; deleted_id: number }> =>
      rpc(async () => {
        const res = await fetch(apiUrl('/elements/merge'), {
          method: 'POST',
          headers: { 'Content-Type': 'application/json' },
          body: JSON.stringify({ source_id: sourceId, survivor_id: survivorId, resolved }),
        })
        if (!res.ok) {
          throw await responseError(res, 'Merge failed')
        }
        const json = await res.json() as { survivor: Record<string, unknown>; deleted_id: number }
        return { survivor: protoElementToLibrary(json.survivor), deleted_id: json.deleted_id }
      }),

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
        list: (): Promise<Record<string, Tag>> =>
          rpc(async () => {
            const res = await orgClient.listTagColors({})
            const json = j<{ tags?: Record<string, { color?: string; description?: string | null }> }>(ListTagColorsResponseSchema, res)
            const tags: Record<string, Tag> = {}
            Object.entries(json.tags ?? {}).forEach(([name, tag]) => {
              tags[name] = { name, color: tag.color ?? '#A0AEC0', description: tag.description ?? null }
            })
            return tags
          }),
        update: (name: string, color: string, description?: string | null): Promise<void> =>
          rpc(async () => {
            await orgClient.updateTag({ tag: name, color, description: description ?? undefined })
          }),
        delete: (name: string): Promise<void> =>
          rpc(async () => {
            const res = await fetch(apiUrl(`/tags/${encodeURIComponent(name)}`), { method: 'DELETE' })
            if (!res.ok && res.status !== 204) throw await responseError(res, `Failed to delete tag`)
          }),
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
            tags: (v.tags ?? []) as string[],
            is_root: Boolean(v.is_root ?? false),
            created_at: String(v.created_at ?? new Date().toISOString()),
            updated_at: String(v.updated_at ?? new Date().toISOString()),
          }))
        }),

      content: (id: number): Promise<{ view?: ViewTreeNode; placements: PlacedElement[]; connectors: Connector[] }> =>
        rpc(async () => {
          const res = await workspaceClient.getView({ viewId: id, includeContent: true })
          const json = j<{ view?: ProtoDiagram; content?: { placements?: Record<string, unknown>[]; connectors?: Record<string, unknown>[] } }>(GetViewResponseSchema, res)
          return {
            view: json.view ? mapDiagram(json.view) : undefined,
            placements: (json.content?.placements ?? []).map(protoPlacedElement),
            connectors: (json.content?.connectors ?? []).map(protoConnector),
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
        const tree = await api.workspace.views.tree()
        return pruneTreeAround(tree, viewId, ancestorLevels, descendantLevels)
      },

      gridData: (): Promise<{
        views: ViewTreeNode[]
        content: Record<number, { placements: PlacedElement[]; connectors: Connector[] }>
      }> =>
        rpc(async () => {
          const res = await workspaceClient.getWorkspace({
            includeContent: true,
            hasView: true,
          })
          const json = j<{
            views?: ProtoDiagram[]
            content?: Record<string, { placements?: Record<string, unknown>[]; connectors?: Record<string, unknown>[] }>
          }>(GetWorkspaceResponseSchema, res)
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
            orgId: localWorkspaceOrgId,
            name: data.name,
            levelLabel: data.label ?? undefined,
            ownerElementId: data.parent_view_id ?? undefined,
          })
          const json = j<{ view: ProtoDiagram }>(CreateViewResponseSchema, res)
          return diagramToView(json.view)
        }),

      update: (id: number, data: { name: string; description?: string; label?: string; tags?: string[] }): Promise<View> =>
        rpc(async () => {
          const res = await workspaceClient.updateView({ viewId: id, name: data.name, description: data.description ?? undefined, levelLabel: data.label ?? undefined, tags: data.tags })
          const json = j<{ view: ProtoDiagram }>(UpdateViewResponseSchema, res)
          return diagramToView(json.view)
        }),

      markdown: {
        get: async (id: number): Promise<{ markdown: ViewMarkdownDocument; content: string } | null> => {
          const json = await connectJsonRpc<{
            markdown?: ProtoViewMarkdownDocument
            content?: string
          }>('GetViewMarkdown', { viewId: id }, { allowNotFound: true })
          if (!json) return null
          const markdown = mapViewMarkdown(json.markdown)
          if (!markdown) return null
          return {
            markdown,
            content: String(json.content ?? ''),
          }
        },

        create: async (
          id: number,
          data: { fileName?: string; initialContent?: string; targetKind?: string; path?: string } = {},
        ): Promise<ViewTreeNode> => {
          const json = await connectJsonRpc<{ view?: ProtoDiagram }>('CreateViewMarkdown', {
            viewId: id,
            fileName: data.fileName ?? undefined,
            initialContent: data.initialContent ?? undefined,
            targetKind: data.targetKind ?? undefined,
            path: data.path ?? undefined,
          })
          if (!json?.view) throw new Error('View markdown was created without an updated view response')
          return mapDiagram(json.view)
        },

        link: async (id: number, path: string): Promise<ViewTreeNode> => {
          const json = await connectJsonRpc<{ view?: ProtoDiagram }>('LinkViewMarkdown', {
            viewId: id,
            path,
          })
          if (!json?.view) throw new Error('View markdown link was saved without an updated view response')
          return mapDiagram(json.view)
        },

        save: async (
          id: number,
          content: string,
          options: { expectedFileVersion?: string; force?: boolean } = {},
        ): Promise<ViewMarkdownDocument> => {
          const json = await connectJsonRpc<{ markdown?: ProtoViewMarkdownDocument }>('SaveViewMarkdown', {
            viewId: id,
            content,
            expectedFileVersion: options.expectedFileVersion ?? undefined,
            force: options.force ?? false,
          })
          const markdown = mapViewMarkdown(json?.markdown)
          if (!markdown) throw new Error('View markdown save returned no markdown metadata')
          return markdown
        },

        unlink: async (id: number, deleteManagedFile = false): Promise<ViewTreeNode> => {
          const json = await connectJsonRpc<{ view?: ProtoDiagram }>('UnlinkViewMarkdown', {
            viewId: id,
            deleteManagedFile,
          })
          if (!json?.view) throw new Error('View markdown unlink returned no updated view response')
          return mapDiagram(json.view)
        },
      },

      threads: {
        listForElement: (viewId: number, elementId: number): Promise<ViewThread[]> =>
          rpc(async () => {
            const res = await collaborationClient.listThreads({ viewId, elementId })
            const json = j<{ threads?: Record<string, unknown>[] }>(ListThreadsResponseSchema, res)
            return (json.threads ?? []).map(mapViewThread)
          }),
        listForConnector: (viewId: number, connectorId: number): Promise<ViewThread[]> =>
          rpc(async () => {
            const res = await collaborationClient.listThreads({ viewId, connectorId })
            const json = j<{ threads?: Record<string, unknown>[] }>(ListThreadsResponseSchema, res)
            return (json.threads ?? []).map(mapViewThread)
          }),
        createForElement: (viewId: number, elementId: number, body: string): Promise<ViewThread> =>
          rpc(async () => {
            const res = await collaborationClient.createThread({ viewId, elementId, body })
            const json = j<{ thread?: Record<string, unknown> }>(CreateThreadResponseSchema, res)
            return mapViewThread(json.thread ?? {})
          }),
        createForConnector: (viewId: number, connectorId: number, body: string): Promise<ViewThread> =>
          rpc(async () => {
            const res = await collaborationClient.createThread({ viewId, connectorId, body })
            const json = j<{ thread?: Record<string, unknown> }>(CreateThreadResponseSchema, res)
            return mapViewThread(json.thread ?? {})
          }),
        addComment: (viewId: number, threadId: number, body: string): Promise<ViewComment> =>
          rpc(async () => {
            const res = await collaborationClient.addComment({ viewId, threadId, body })
            const json = j<{ comment?: Record<string, unknown> }>(AddCommentResponseSchema, res)
            return mapViewComment(json.comment ?? {})
          }),
        resolve: (viewId: number, threadId: number, resolved: boolean): Promise<ThreadResolveEvent> =>
          rpc(async () => {
            await collaborationClient.resolveThread({ viewId, threadId, resolved })
            return { thread_id: threadId, resolved }
          }),
      },

      reactions: {
        list: (viewId: number): Promise<ElementReactionSummary[]> =>
          rpc(async () => {
            const res = await collaborationClient.listReactions({ viewId })
            const json = j<{ reactions?: Record<string, unknown>[] }>(ListReactionsResponseSchema, res)
            return (json.reactions ?? []).map(mapReactionSummary)
          }),
        toggleForElement: (viewId: number, elementId: number, emoji: string): Promise<{ active: boolean }> =>
          rpc(async () => {
            const res = await collaborationClient.toggleReaction({ viewId, elementId, emoji })
            return { active: res.active }
          }),
      },

      rename: (id: number, name: string): Promise<View> =>
        rpc(async () => {
          const res = await workspaceClient.updateView({ viewId: id, name })
          const json = j<{ view: ProtoDiagram }>(UpdateViewResponseSchema, res)
          return diagramToView(json.view)
        }),

      setLevel: (id: number, level: number): Promise<void> =>
        rpc(async () => { await workspaceClient.setViewLevel({ viewId: id, level }) }),

      density: {
        get: async (id: number): Promise<number> => {
          const res = await fetch(apiUrl(`/views/${id}/density`))
          if (!res.ok) throw new Error('Failed to load density')
          const json = await res.json() as { density_level?: number }
          return Number(json.density_level ?? 0)
        },
        set: async (id: number, densityLevel: number): Promise<number> => {
          const res = await fetch(apiUrl(`/views/${id}/density`), {
            method: 'PUT',
            headers: { 'Content-Type': 'application/json' },
            body: JSON.stringify({ density_level: densityLevel }),
          })
          if (!res.ok) throw new Error('Failed to save density')
          const json = await res.json() as { density_level?: number }
          return Number(json.density_level ?? densityLevel)
        },
      },

      noiseGate: {
        initialize: async (id: number, densityLevel?: number): Promise<NoiseGateInitialization> => {
          const res = await fetch(apiUrl(`/views/${id}/noise-gate/initialize`), {
            method: 'POST',
            headers: { 'Content-Type': 'application/json' },
            body: JSON.stringify(densityLevel == null ? {} : { density_level: densityLevel }),
          })
          if (!res.ok) throw new Error('Failed to initialize noise gate')
          const json = await res.json() as Partial<NoiseGateInitialization>
          return {
            view_id: Number(json.view_id ?? id),
            density_level: Number(json.density_level ?? densityLevel ?? 0),
            elements_enabled: Number(json.elements_enabled ?? 0),
            overrides_created: Number(json.overrides_created ?? 0),
          }
        },
      },

      populate: {
        getQuery: async (id: number): Promise<{ query: string; enriched_query: string }> => {
          const res = await fetch(apiUrl(`/views/${id}/populate-query`))
          if (!res.ok) throw new Error('Failed to load populate query')
          const json = await res.json() as { query: string; enriched_query?: string }
          return { query: json.query, enriched_query: json.enriched_query ?? json.query }
        },
        search: async (id: number, q: string, limit: number): Promise<Array<LibraryElement & { similarity_score: number; match_kind?: string; match_reason?: string }>> => {
          const params = new URLSearchParams({ q, limit: String(limit) })
          const res = await fetch(apiUrl(`/views/${id}/populate?${params}`))
          if (!res.ok) throw new Error('Failed to run similarity search')
          const json = await res.json() as { results: Array<Record<string, unknown> & { similarity_score: number; match_kind?: string; match_reason?: string }> }
          return (json.results ?? []).map(r => ({
            ...protoElementToLibrary(r),
            similarity_score: r.similarity_score,
            match_kind: r.match_kind,
            match_reason: r.match_reason,
          }))
        },
      },

      visibilityOverrides: {
        list: async (id: number): Promise<VisibilityOverride[]> => {
          const res = await fetch(apiUrl(`/views/${id}/visibility-overrides`))
          if (!res.ok) throw new Error('Failed to load visibility overrides')
          const json = await res.json() as { overrides?: Record<string, unknown>[] }
          return (json.overrides ?? []).map((override) => normalizeVisibilityOverride(override))
        },
        set: async (id: number, resourceType: VisibilityOverride['resource_type'], resourceId: number, levelDelta: number): Promise<VisibilityOverride> => {
          const res = await fetch(apiUrl(`/views/${id}/visibility-overrides`), {
            method: 'PUT',
            headers: { 'Content-Type': 'application/json' },
            body: JSON.stringify({
              resource_type: resourceType,
              resource_id: resourceId,
              level_delta: levelDelta,
            }),
          })
          if (!res.ok) throw new Error('Failed to save visibility override')
          const json = await res.json() as { override?: Record<string, unknown> }
          return normalizeVisibilityOverride(json.override ?? {}, { viewId: id, resourceType, resourceId, levelDelta })
        },
        promote: async (id: number, resourceType: VisibilityOverride['resource_type'], resourceId: number): Promise<VisibilityOverride> => {
          const res = await fetch(apiUrl(`/views/${id}/visibility-overrides/${resourceType}/${resourceId}/promote`), { method: 'POST' })
          if (!res.ok) throw new Error('Failed to promote visibility')
          const json = await res.json() as { override?: Record<string, unknown> }
          return normalizeVisibilityOverride(json.override ?? {}, { viewId: id, resourceType, resourceId, levelDelta: 1 })
        },
        demote: async (id: number, resourceType: VisibilityOverride['resource_type'], resourceId: number): Promise<VisibilityOverride> => {
          const res = await fetch(apiUrl(`/views/${id}/visibility-overrides/${resourceType}/${resourceId}/demote`), { method: 'POST' })
          if (!res.ok) throw new Error('Failed to demote visibility')
          const json = await res.json() as { override?: Record<string, unknown> }
          return normalizeVisibilityOverride(json.override ?? {}, { viewId: id, resourceType, resourceId, levelDelta: -1 })
        },
        reset: async (id: number, resourceType: VisibilityOverride['resource_type'], resourceId: number): Promise<void> => {
          const res = await fetch(apiUrl(`/views/${id}/visibility-overrides/${resourceType}/${resourceId}`), { method: 'DELETE' })
          if (!res.ok) throw new Error('Failed to reset visibility override')
        },
      },

      delete: (orgId: string, id: number): Promise<void> =>
        rpc(async () => { await workspaceClient.deleteView({ orgId: orgIdOrLocal(orgId), viewId: id }) }),

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
      list: (diagramId: number, params?: { limit?: number; offset?: number }): Promise<Connector[]> =>
        rpc(async () => {
          const res = await workspaceClient.listConnectors({
            viewId: diagramId,
            limit: params?.limit ?? 0,
            offset: params?.offset ?? 0,
          })
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
          tags?: string[]
        },
      ): Promise<Connector> =>
        rpc(async () => {
          const request: Record<string, unknown> = {
            viewId: diagramId,
            sourceElementId: data.source_element_id,
            targetElementId: data.target_element_id,
            label: data.label ?? undefined,
            description: data.description ?? undefined,
            relationship: data.relationship ?? undefined,
            direction: data.direction ?? undefined,
            style: normalizeConnectorRouteStyle(data.style),
            url: data.url ?? undefined,
            sourceHandle: data.source_handle ?? undefined,
            targetHandle: data.target_handle ?? undefined,
          }
          if (data.tags !== undefined) request.tags = data.tags
          const res = await workspaceClient.createConnector(request as Parameters<typeof workspaceClient.createConnector>[0])
          const json = j<{ connector: Record<string, unknown> }>(CreateConnectorResponseSchema, res)
          return protoConnector(json.connector ?? {})
        }),

      update: (
        diagramId: number,
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
          tags?: string[]
        },
      ): Promise<Connector> =>
        rpc(async () => {
          const request: Record<string, unknown> = {
            viewId: diagramId,
            connectorId,
            sourceElementId: data.source_element_id ?? undefined,
            targetElementId: data.target_element_id ?? undefined,
            label: data.label ?? undefined,
            description: data.description ?? undefined,
            relationship: data.relationship ?? undefined,
            direction: data.direction ?? undefined,
            style: data.style === undefined ? undefined : normalizeConnectorRouteStyle(data.style),
            url: data.url ?? undefined,
            sourceHandle: data.source_handle ?? undefined,
            targetHandle: data.target_handle ?? undefined,
          }
          if (data.tags !== undefined) request.tags = data.tags
          const res = await workspaceClient.updateConnector(request as Parameters<typeof workspaceClient.updateConnector>[0])
          const json = j<{ connector: Record<string, unknown> }>(UpdateConnectorResponseSchema, res)
          return protoConnector(json.connector ?? {})
        }),

      delete: (orgId: string, connectorId: number): Promise<void> =>
        rpc(async () => { await workspaceClient.deleteConnector({ orgId: orgIdOrLocal(orgId), connectorId }) }),
    },
  },

  dependencies: {
    list: (params?: { limit?: number; offset?: number; search?: string }): Promise<DependenciesResponse> =>
      rpc(async () => {
        if (params) {
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
            workspaceClient.listConnectors({
              viewId: 0,
              limit: params.limit ?? 0,
              offset: params.offset ?? 0,
            })
              .then((res) => {
                const connectorJson = j<{ connectors: Record<string, unknown>[] }>(ListConnectorsResponseSchema, res)
                return (connectorJson.connectors ?? []).map(protoDependencyConnector)
              }),
          ])
          return {
            elements: elements.elements.map(libraryElementToDependency),
            connectors,
            totalCount: elements.totalCount,
          }
        }
        const res = await dependencyClient.listDependencies({})
        const json = j<Partial<DependenciesResponse> & { total_count?: number }>(ListDependenciesResponseSchema, res)
        return {
          elements: json.elements ?? [],
          connectors: json.connectors ?? [],
          totalCount: json.totalCount ?? json.total_count,
        }
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
          orgId: orgIdOrLocal(orgId),
          elements: normalizeFrontendImportElements(data.elements),
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

  mermaid: {
    parse: (source: string): Promise<ParsedImport> =>
      rpc(async () => {
        const res = await mermaidClient.parseMermaid({ source })
        return {
          format: 'mermaid',
          elements: res.elements,
          connectors: res.connectors,
          warnings: res.warnings,
          direction: mapMermaidDirection(res.direction),
          source: res.source,
        }
      }),

    importIntoView: (
      viewId: number,
      source: string,
      center: { x: number; y: number },
      dryRun = false,
    ): Promise<MermaidImportResult> =>
      rpc(async () => {
        const res = await mermaidClient.importMermaidIntoView({
          orgId: orgIdOrLocal(''),
          viewId,
          source,
          centerX: center.x,
          centerY: center.y,
          dryRun,
        })
        return {
          summary: mapMermaidImportSummary(res.summary),
          warnings: res.warnings,
          content: res.content,
        }
      }),

    exportView: (
      viewId: number,
      options: {
        includeTldMetadata: boolean
        markdownBlock?: boolean
        densityOverride?: number
      },
    ): Promise<{ code: string; markdown: string; warnings: string[] }> =>
      rpc(async () => {
        const res = await mermaidClient.exportMermaidView({
          orgId: orgIdOrLocal(''),
          viewId,
          includeTldMetadata: options.includeTldMetadata,
          markdownBlock: options.markdownBlock ?? false,
          densityOverride: options.densityOverride,
        })
        return {
          code: res.code,
          markdown: res.markdown,
          warnings: res.warnings,
        }
      }),

    inspectMarkdown: (markdown: string, viewId?: number | null): Promise<{ blocks: MermaidMarkdownBlock[]; syncStatus: MermaidMarkdownSyncStatus; warnings: string[] }> =>
      rpc(async () => {
        const res = await mermaidClient.inspectMermaidMarkdown({
          orgId: orgIdOrLocal(''),
          markdown,
          viewId: viewId ?? undefined,
        })
        return {
          blocks: res.blocks.map(mapMermaidMarkdownBlock),
          syncStatus: mapMermaidMarkdownSyncStatus(res.syncStatus),
          warnings: res.warnings,
        }
      }),

    upsertMarkdownBlock: (
      viewId: number,
      markdown: string,
      includeTldMetadata = true,
    ): Promise<{ markdown: string; previousStatus: MermaidMarkdownSyncStatus; warnings: string[] }> =>
      rpc(async () => {
        const res = await mermaidClient.upsertMermaidMarkdownBlock({
          orgId: orgIdOrLocal(''),
          viewId,
          markdown,
          includeTldMetadata,
        })
        return {
          markdown: res.markdown,
          previousStatus: mapMermaidMarkdownSyncStatus(res.previousStatus),
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
    websocketUrl: watchWebSocketUrl,
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
    cleanContext: async (repositoryId: number, input: { resource_type: 'element' | 'view'; resource_id: number }): Promise<WatchContextActionResponse> => {
      const res = await fetch(apiUrl(`/watch/repositories/${repositoryId}/context/clean`), {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify(input),
      })
      if (!res.ok) throw await responseError(res, 'Failed to clean watch context')
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
        throw await responseError(res, 'Failed to open editor')
      }
    },
  },
}
