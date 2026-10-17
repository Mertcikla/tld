import { api } from '../../api/client'
import type { Connector, ViewLayer } from '../../types'
import { isElementGroupTag } from '../../utils/elementGroups'
import { isGroundingIgnoreTag } from '../../utils/groundingTags'

export function connectorTagsForDrag(source: { tag?: string; layerId?: number }, layers: readonly ViewLayer[]): string[] {
  const tags = source.tag ? [source.tag] : layers.find((layer) => layer.id === source.layerId)?.tags ?? []
  return tags.filter((tag) => tag.trim() !== '' && !isElementGroupTag(tag) && !isGroundingIgnoreTag(tag))
}

export function connectorTagsFromDrop(data: Pick<DataTransfer, 'getData'>, layers: readonly ViewLayer[]): string[] {
  return connectorTagsForDrag({ tag: data.getData('application/diag-tag'), layerId: Number(data.getData('application/diag-layer')) }, layers)
}

export function connectorAlreadyHasDraggedTags(connector: Pick<Connector, 'tags'>, tags: readonly string[] | null | undefined): boolean {
  return !!tags?.length && tags.every((tag) => (connector.tags ?? []).includes(tag))
}

export function resolveTagDropConnector(target: EventTarget | null, connectors: readonly Connector[]): Connector | null {
  if (typeof Element === 'undefined' || !(target instanceof Element)) return null
  const id = target.closest('[data-tag-drop-connector-id]')?.getAttribute('data-tag-drop-connector-id')
  return connectors.find((connector) => String(connector.id) === id) ?? null
}

/** Append tags with a narrow patch, preserving every other connector field. */
export async function saveConnectorTagDrop(connector: Connector, tags: readonly string[]): Promise<Connector | null> {
  if (tags.every((tag) => (connector.tags ?? []).includes(tag))) return null
  const nextTags = [...new Set([...(connector.tags ?? []), ...tags])]
  return api.workspace.connectors.update(connector.view_id, connector.id, { tags: nextTags })
}
