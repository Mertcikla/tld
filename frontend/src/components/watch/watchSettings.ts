import type {
  WatchEmbeddingConfig,
  WatchSettings,
  WatchSettingsDescriptor,
} from '../../api/client'

export const WATCH_LANGUAGE_OPTIONS = ['c', 'cpp', 'go', 'java', 'javascript', 'python', 'rust', 'typescript']

export const WATCHER_OPTIONS = ['auto', 'fsnotify', 'poll']
export const SCALE_STRATEGY_OPTIONS = ['auto', 'full', 'limited', 'abort']
export const EMBEDDING_PROVIDER_OPTIONS = ['none', 'openai', 'ollama', 'local-lexical', 'local-deterministic-test']

export type WatchSettingsGroup = {
  id: string
  label: string
  description?: string
  keys: string[]
  // Advanced groups are hidden behind progressive disclosure on compact
  // surfaces because they tune detail rather than day-to-day behavior.
  advanced?: boolean
}

// Individual keys that belong to an otherwise primary group but are low-impact
// detail settings. They are relocated into the Advanced disclosure.
export const ADVANCED_WATCH_KEYS: ReadonlySet<string> = new Set([
  'watch.poll_interval',
  'watch.debounce',
  'watch.scale.max_tracked_files',
  'watch.scale.max_limited_files',
  'watch.scale.max_recent_files',
  'watch.scale.max_caller_depth',
  'watch.scale.max_blast_radius_hops',
  'watch.lsp.health_interval',
  'watch.lsp.memory_limit_bytes',
  'watch.embedding.dimension',
  'watch.embedding.max_tokens',
  'watch.embedding.health_threshold',
  'watch.embedding.runtime_path',
])

export function isAdvancedWatchKey(key: string): boolean {
  return ADVANCED_WATCH_KEYS.has(key) || key.startsWith('watch.lsp.commands.')
}

// Splits the flat registry into the primary settings shown by default and the
// advanced settings tucked behind the disclosure. Primary groups keep their
// identity; advanced keys pulled out of them become sibling groups so the
// disclosure still reads as labelled sections.
export function partitionWatchSettingsGroups(): { primary: WatchSettingsGroup[]; advanced: WatchSettingsGroup[] } {
  const primary: WatchSettingsGroup[] = []
  const advanced: WatchSettingsGroup[] = []
  for (const group of WATCH_SETTINGS_GROUPS) {
    if (group.advanced) {
      advanced.push(group)
      continue
    }
    const primaryKeys = group.keys.filter((key) => !isAdvancedWatchKey(key))
    const advancedKeys = group.keys.filter(isAdvancedWatchKey)
    if (primaryKeys.length > 0) {
      primary.push({ ...group, keys: primaryKeys })
    }
    if (advancedKeys.length > 0) {
      advanced.push({ ...group, id: `${group.id}-advanced`, keys: advancedKeys })
    }
  }
  return { primary, advanced }
}

// Groups the flat watch.* config registry into labelled sections. Keys not
// listed in any group are appended to a trailing "Other" group by the caller.
export const WATCH_SETTINGS_GROUPS: WatchSettingsGroup[] = [
  {
    id: 'discovery',
    label: 'Discovery',
    description: 'Which repositories and languages are scanned.',
    keys: ['watch.languages', 'watch.watcher', 'watch.poll_interval', 'watch.debounce'],
  },
  {
    id: 'scale',
    label: 'Scale',
    description: 'Huge-repository strategy and file thresholds.',
    keys: [
      'watch.scale.strategy',
      'watch.scale.max_tracked_files',
      'watch.scale.max_limited_files',
      'watch.scale.max_recent_files',
      'watch.scale.max_caller_depth',
      'watch.scale.max_blast_radius_hops',
    ],
  },
  {
    id: 'thresholds',
    label: 'Representation thresholds',
    description: 'Caps applied when materializing generated views.',
    advanced: true,
    keys: [
      'watch.thresholds.max_elements_per_view',
      'watch.thresholds.max_connectors_per_view',
      'watch.thresholds.max_incoming_per_element',
      'watch.thresholds.max_outgoing_per_element',
      'watch.thresholds.max_expanded_connectors_per_group',
    ],
  },
  {
    id: 'lsp',
    label: 'Language servers',
    description: 'Definition resolution quality and resource limits.',
    keys: [
      'watch.lsp.enabled',
      'watch.lsp.health_interval',
      'watch.lsp.memory_limit_bytes',
      'watch.lsp.commands.c',
      'watch.lsp.commands.cpp',
      'watch.lsp.commands.go',
      'watch.lsp.commands.java',
      'watch.lsp.commands.javascript',
      'watch.lsp.commands.python',
      'watch.lsp.commands.rust',
      'watch.lsp.commands.typescript',
    ],
  },
  {
    id: 'embedding',
    label: 'Embeddings',
    description: 'Identity matching and similarity provider.',
    keys: [
      'watch.embedding.provider',
      'watch.embedding.endpoint',
      'watch.embedding.model',
      'watch.embedding.dimension',
      'watch.embedding.max_tokens',
      'watch.embedding.health_threshold',
      'watch.embedding.runtime_path',
    ],
  },
  {
    id: 'dependencies',
    label: 'Dependencies',
    keys: ['watch.dependencies.enabled'],
  },
  {
    id: 'visibility',
    label: 'Visibility weights',
    advanced: true,
    keys: [
      'watch.visibility.core_threshold_enabled',
      'watch.visibility.core_threshold',
      'watch.visibility.tier_multiplier',
      'watch.visibility.max_expansion_multiplier',
      'watch.visibility.weights.changed',
      'watch.visibility.weights.selected',
      'watch.visibility.weights.user_show',
      'watch.visibility.weights.user_hide',
      'watch.visibility.weights.high_signal_fact',
      'watch.visibility.weights.relationship_proximity',
      'watch.visibility.weights.dependency_fact',
      'watch.visibility.weights.utility_noise',
      'watch.visibility.weights.high_degree_noise',
    ],
  },
  {
    id: 'layout',
    label: 'Layout',
    advanced: true,
    keys: [
      'watch.layout.link_distance',
      'watch.layout.charge_strength',
      'watch.layout.collide_radius',
      'watch.layout.gravity_strength',
    ],
  },
]

const GROUPED_KEYS = new Set(WATCH_SETTINGS_GROUPS.flatMap((group) => group.keys))

export function ungroupedDescriptors(descriptors: WatchSettingsDescriptor[]): WatchSettingsDescriptor[] {
  return descriptors.filter((descriptor) => !GROUPED_KEYS.has(descriptor.key))
}

export function descriptorsByKey(descriptors: WatchSettingsDescriptor[]): Record<string, WatchSettingsDescriptor> {
  const out: Record<string, WatchSettingsDescriptor> = {}
  for (const descriptor of descriptors) {
    out[descriptor.key] = descriptor
  }
  return out
}

// Go time.Duration values are serialized to JSON as an integer count of
// nanoseconds. These helpers convert to/from a human-readable seconds value.
export function durationSecondsToNanos(value: number): number {
  if (!Number.isFinite(value)) return 0
  return Math.max(0, Math.round(value * 1_000_000_000))
}

export function nanosToDurationSeconds(value: number): number {
  if (!Number.isFinite(value)) return 0
  return value / 1_000_000_000
}

export function formatDurationNanos(value: number): string {
  const seconds = nanosToDurationSeconds(value)
  if (seconds === 0) return '0s'
  if (seconds % 3600 === 0) return `${seconds / 3600}h`
  if (seconds % 60 === 0) return `${seconds / 60}m`
  if (Number.isInteger(seconds)) return `${seconds}s`
  return `${seconds.toFixed(1)}s`
}

export function describeWatchSetting(key: string, descriptors: Record<string, WatchSettingsDescriptor>): string {
  return descriptors[key]?.description ?? ''
}

export function cloneWatchSettings(settings: WatchSettings): WatchSettings {
  return JSON.parse(JSON.stringify(settings)) as WatchSettings
}

export function cloneWatchEmbedding(embedding: WatchEmbeddingConfig): WatchEmbeddingConfig {
  return JSON.parse(JSON.stringify(embedding)) as WatchEmbeddingConfig
}

// Summarizes the most important effective settings for compact surfaces.
export function summarizeWatchSettings(settings: WatchSettings, embedding: WatchEmbeddingConfig): string {
  const languages = settings.languages.length > 0 ? settings.languages.join(', ') : 'auto'
  return `${settings.watcher} · ${languages} · ${embedding.provider || 'none'}`
}
