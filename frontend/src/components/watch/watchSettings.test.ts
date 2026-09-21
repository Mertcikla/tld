import { describe, expect, it } from 'vitest'
import {
  WATCH_SETTINGS_GROUPS,
  descriptorsByKey,
  durationSecondsToNanos,
  formatDurationNanos,
  isAdvancedWatchKey,
  nanosToDurationSeconds,
  partitionWatchSettingsGroups,
  ungroupedDescriptors,
} from './watchSettings'

describe('watchSettings helpers', () => {
  it('round-trips duration values between seconds and nanoseconds', () => {
    expect(durationSecondsToNanos(1.5)).toBe(1_500_000_000)
    expect(nanosToDurationSeconds(1_500_000_000)).toBe(1.5)
    expect(durationSecondsToNanos(0)).toBe(0)
  })

  it('formats duration nanoseconds compactly', () => {
    expect(formatDurationNanos(0)).toBe('0s')
    expect(formatDurationNanos(500_000_000)).toBe('0.5s')
    expect(formatDurationNanos(60_000_000_000)).toBe('1m')
    expect(formatDurationNanos(3_600_000_000_000)).toBe('1h')
  })

  it('keeps ungrouped descriptors separate from known groups', () => {
    const descriptors = [
      { key: 'watch.watcher', value: 'auto', description: '' },
      { key: 'watch.something.new', value: '1', description: '' },
    ]
    const grouped = new Set(WATCH_SETTINGS_GROUPS.flatMap((group) => group.keys))
    expect(grouped.has('watch.watcher')).toBe(true)
    const remaining = ungroupedDescriptors(descriptors)
    expect(remaining.map((d) => d.key)).toEqual(['watch.something.new'])
  })

  it('indexes descriptors by key', () => {
    const byKey = descriptorsByKey([
      { key: 'watch.watcher', value: 'auto', description: 'watcher backend' },
      { key: 'watch.debounce', value: '500ms', description: 'debounce' },
    ])
    expect(byKey['watch.watcher'].description).toBe('watcher backend')
    expect(byKey['watch.debounce'].value).toBe('500ms')
  })

  it('classifies low-impact detail settings as advanced', () => {
    expect(isAdvancedWatchKey('watch.debounce')).toBe(true)
    expect(isAdvancedWatchKey('watch.lsp.commands.go')).toBe(true)
    expect(isAdvancedWatchKey('watch.watcher')).toBe(false)
    expect(isAdvancedWatchKey('watch.languages')).toBe(false)
    expect(isAdvancedWatchKey('watch.embedding.provider')).toBe(false)
  })

  it('partitions groups without losing or duplicating keys', () => {
    const { primary, advanced } = partitionWatchSettingsGroups()
    const registryKeys = WATCH_SETTINGS_GROUPS.flatMap((group) => group.keys)
    const partitionedKeys = [...primary, ...advanced].flatMap((group) => group.keys)

    expect(partitionedKeys).toHaveLength(registryKeys.length)
    expect(new Set(partitionedKeys)).toEqual(new Set(registryKeys))

    const primaryKeys = primary.flatMap((group) => group.keys)
    expect(primaryKeys).toContain('watch.watcher')
    expect(primaryKeys).not.toContain('watch.debounce')
    expect(primaryKeys).not.toContain('watch.visibility.weights.changed')

    const advancedKeys = advanced.flatMap((group) => group.keys)
    expect(advancedKeys).toContain('watch.debounce')
    expect(advancedKeys).toContain('watch.visibility.weights.changed')
    expect(advancedKeys).toContain('watch.layout.link_distance')
    expect(advancedKeys).not.toContain('watch.watcher')
  })
})
