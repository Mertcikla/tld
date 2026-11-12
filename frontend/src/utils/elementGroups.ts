import type { ViewLayer } from '../types'

export const ELEMENT_GROUP_TAG_PREFIX = 'group:'
export const ELEMENT_GROUP_NODE_PREFIX = 'element-group-background:'
const ELEMENT_GROUP_TAG_PATTERN = /^group:[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$/i

export function isElementGroupTag(tag: string): boolean {
  return ELEMENT_GROUP_TAG_PATTERN.test(tag)
}

export function createElementGroupTag(): string {
  const randomUUID = globalThis.crypto?.randomUUID?.()
  const hex = Array.from({ length: 32 }, () => Math.floor(Math.random() * 16).toString(16)).join('')
  const id = randomUUID ?? `${hex.slice(0, 8)}-${hex.slice(8, 12)}-4${hex.slice(13, 16)}-a${hex.slice(17, 20)}-${hex.slice(20)}`
  return `${ELEMENT_GROUP_TAG_PREFIX}${id}`
}

export function elementGroupTagForLayer(layer: Pick<ViewLayer, 'tags'>): string | null {
  return layer.tags.length === 1 && isElementGroupTag(layer.tags[0]) ? layer.tags[0] : null
}

export function isElementGroupLayer(layer: Pick<ViewLayer, 'tags'>): boolean {
  return elementGroupTagForLayer(layer) !== null
}

/** Shared surfaces for element groups and symbol selection areas. */
export function groupHighlightColors(color: string, active = false, hidden = false) {
  return {
    borderColor: `color-mix(in srgb, ${color} ${hidden ? 20 : active ? 90 : 52}%, transparent)`,
    bg: `color-mix(in srgb, ${color} ${hidden ? 3 : active ? 18 : 10}%, transparent)`,
  }
}
