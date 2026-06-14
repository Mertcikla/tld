export interface TechnologyConnector {
  type: 'catalog' | 'custom'
  slug?: string
  label: string
  is_primary_icon?: boolean
  isPrimaryIcon?: boolean
}

export interface TechnologyCatalogItem {
  iconUrl: string
  name: string
  provider?: string
  docsUrl?: string
  description?: string
  websiteUrl?: string
  nameShort: string
  defaultSlug: string
  aliases?: string[]
}

export interface Tag {
  name: string
  color: string
  description: string | null
}

export interface LibraryElement {
  id: number
  name: string
  kind: string | null
  description: string | null
  technology: string | null
  url: string | null
  logo_url: string | null
  technology_connectors: TechnologyConnector[]
  tags: string[]
  repo?: string | null
  branch?: string | null
  file_path?: string | null
  language?: string | null
  bypass_noise_gate?: boolean
  created_at: string
  updated_at: string
  has_view: boolean
  view_label: string | null
}

export interface View {
  id: number
  owner_element_id: number | null
  name: string
  label: string | null
  tags: string[]
  is_root: boolean
  created_at: string
  updated_at: string
}

export interface ElementPlacement {
  id: number
  view_id: number
  element_id: number
  position_x: number
  position_y: number
}

export interface PlacedElement {
  id: number
  view_id: number
  element_id: number
  position_x: number
  position_y: number
  name: string
  description: string | null
  kind: string | null
  technology: string | null
  url: string | null
  logo_url: string | null
  technology_connectors: TechnologyConnector[]
  tags: string[]
  repo?: string | null
  branch?: string | null
  file_path?: string | null
  language?: string | null
  bypass_noise_gate?: boolean
  has_view: boolean
  view_label: string | null
}

export interface VisibilityOverride {
  view_id: number
  resource_type: 'element' | 'connector'
  resource_id: number
  level_delta: number
  created_at?: string
  updated_at?: string
}

export interface NoiseGateInitialization {
  view_id: number
  density_level: number
  elements_enabled: number
  overrides_created: number
}

export interface NavigationConnector {
  id: number
  element_id: number | null
  from_view_id: number
  to_view_id: number
  to_view_name: string
  relation_type: string
}

export interface Connector {
  id: number
  view_id: number
  source_element_id: number
  target_element_id: number
  label: string | null
  description: string | null
  relationship: string | null
  direction: string
  style: string
  url: string | null
  source_handle: string | null
  target_handle: string | null
  tags?: string[]
  created_at: string
  updated_at: string
}

export interface ViewComment {
  id: number
  org_id: string
  view_id: number
  diagram_id: number
  thread_id: number
  author_id: string
  author_username: string
  body: string
  created_at: string
  updated_at: string
}

export interface ViewThread {
  id: number
  org_id: string
  view_id: number
  diagram_id: number
  element_id: number | null
  connector_id: number | null
  created_by: string
  created_by_username: string
  status: 'open' | 'resolved'
  created_at: string
  resolved_at: string | null
  comments: ViewComment[]
}

export interface ThreadResolveEvent {
  thread_id: number
  resolved: boolean
}

export interface ElementReactionSummary {
  element_id: number
  emoji: string
  count: number
  reacted_by_me: boolean
}

export interface ViewTreeNode {
  id: number
  owner_element_id?: number | null
  name: string
  description: string | null
  level_label: string | null
  tags?: string[]
  level: number
  depth: number
  created_at: string
  updated_at: string
  parent_view_id: number | null
  markdown?: ViewMarkdownDocument | null
  children: ViewTreeNode[]
}

export interface ViewMarkdownDocument {
  path: string
  is_managed: boolean
  updated_at: string
  source_kind: 'PRIVATE_WORKSPACE' | 'PRIVATE_APP' | 'REPO' | 'ATTACHED' | 'LEGACY' | string
  exists: boolean
  writable: boolean
  can_edit: boolean
  git_state: 'outside_repo' | 'ignored' | 'untracked' | 'modified' | 'deleted' | 'tracked' | 'unknown' | string
  repo_relative_path?: string
  linked_view_count: number
  file_version: string
}

export interface ViewLayer {
  id: number
  diagram_id: number
  name: string
  tags: string[]
  color?: string
  created_at?: string
  updated_at?: string
}

export interface DependencyElement {
  id: string
  name: string
  description?: string | null
  type?: string | null
  technology?: string | null
  url?: string | null
  logo_url?: string | null
  technology_connectors: TechnologyConnector[]
  tags: string[]
  repo?: string | null
  branch?: string | null
  language?: string | null
  file_path?: string | null
  bypass_noise_gate?: boolean
  created_at: string
  updated_at: string
}

export interface DependencyConnector {
  id: string
  view_id: string
  source_element_id: string
  target_element_id: string
  label?: string | null
  description?: string | null
  relationship_type?: string | null
  direction: string
  connector_type: string
  url?: string | null
  source_handle?: string | null
  target_handle?: string | null
  tags: string[]
  created_at: string
  updated_at: string
}

export interface ViewConnector {
  id: number
  element_id: number | null
  from_view_id: number
  to_view_id: number
  to_view_name: string
  relation_type: string
}

export interface ViewConnectorsResult {
  child_links: ViewConnector[]
  parent_links: ViewConnector[]
}

export interface IncomingViewConnector {
  id: number
  element_id: number
  element_name: string
  from_view_id: number
  from_view_name: string
  to_view_id: number
}

export interface ViewPlacement {
  view_id: number
  view_name: string
}

export interface ExploreViewData {
  placements: PlacedElement[]
  connectors: Connector[]
}

export interface ExploreData {
  tree: ViewTreeNode[]
  views: Record<string, ExploreViewData>
  navigations: ViewConnector[]
  password_required?: boolean
}

export const ELEMENT_TYPES = [
  'person',
  'system',
  'container',
  'component',
  'database',
  'queue',
  'api',
  'service',
  'external',
] as const

export type ElementType = (typeof ELEMENT_TYPES)[number]

export const TYPE_COLORS: Record<string, string> = {
  person: 'teal',
  system: 'blue',
  container: 'purple',
  component: 'orange',
  database: 'cyan',
  queue: 'yellow',
  api: 'green',
  service: 'pink',
  external: 'gray',
}
