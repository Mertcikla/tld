import type { TechnologyConnector } from '../types'
import { catalogIconUrlForSlug, normalizeCatalogIconPath } from '../utils/technologyIcon'

const CONNECTOR_ROUTE_STYLES = new Set(['bezier', 'straight', 'step', 'smoothstep'])

type ProtoTechnologyLink = {
  type?: string
  slug?: string
  label?: string
  is_primary_icon?: boolean
  isPrimaryIcon?: boolean
}

export function normalizeConnectorRouteStyle(style: unknown): string {
  return typeof style === 'string' && CONNECTOR_ROUTE_STYLES.has(style) ? style : 'bezier'
}

export function normalizeTechnologyConnectors(value: unknown): TechnologyConnector[] {
  return ((value ?? []) as ProtoTechnologyLink[]).map((technologyLink) => ({
    type: (technologyLink.type ?? 'custom') as TechnologyConnector['type'],
    slug: technologyLink.slug,
    label: technologyLink.label ?? '',
    is_primary_icon: !!(technologyLink.is_primary_icon ?? technologyLink.isPrimaryIcon),
  }))
}

export function normalizeLogoUrl(
  logoUrl: unknown,
  technologyConnectors: TechnologyConnector[],
): string | null {
  if (logoUrl != null) {
    return typeof logoUrl === 'string' ? normalizeCatalogIconPath(logoUrl) : logoUrl as string
  }
  const primary = technologyConnectors.find((link) => (
    link.type === 'catalog' &&
    !!link.slug &&
    !!(link.is_primary_icon ?? link.isPrimaryIcon)
  )) ?? technologyConnectors.find((link) => link.type === 'catalog' && !!link.slug)
  return catalogIconUrlForSlug(primary?.slug)
}
