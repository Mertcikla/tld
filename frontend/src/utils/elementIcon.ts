import type { TechnologyConnector } from '../types'
import { fontAwesomeIconUrlForTechnologyLabel, parseFontAwesomeTechnologyIconName } from './fontAwesomeIcon'
import { canonicalTechnologySlug, catalogIconUrlForSlug, normalizeCatalogIconPath } from './technologyIcon'
import { resolveIconPath } from './url'

export function elementTechnologyText(technology: string | null | undefined, links: TechnologyConnector[] | null | undefined, hasIcon: boolean): string | undefined {
  const connectors = links ?? []
  const linkCount = connectors.filter(link => !!link.label).length
  const count = linkCount || (technology ?? '').split(',').map(part => part.trim()).filter(Boolean).length
  return !hasIcon || count > 1 ? technology || (linkCount > 1 ? connectors.map(link => link.label).join(', ') : undefined) : undefined
}

export function technologyConnectorIconKey(link: TechnologyConnector): string | null {
  if (link.type === 'catalog' && link.slug) {
    return `catalog:${canonicalTechnologySlug(link.slug)}`
  }

  if (link.type === 'custom') {
    const iconName = parseFontAwesomeTechnologyIconName(link.label)
    if (iconName && fontAwesomeIconUrlForTechnologyLabel(link.label)) {
      return `fa:${iconName}`
    }
  }

  return null
}

export function resolveTechnologyConnectorIconUrl(
  link: TechnologyConnector,
  catalogIconUrl?: string | null,
): string | null {
  if (link.type === 'catalog' && link.slug) {
    return catalogIconUrl ?? catalogIconUrlForSlug(link.slug)
  }

  if (link.type === 'custom') {
    return fontAwesomeIconUrlForTechnologyLabel(link.label)
  }

  return null
}

export function resolveElementIconUrl(
  logoUrl: string | null | undefined,
  technologyConnectors: TechnologyConnector[] | null | undefined,
): string | null {
  if (logoUrl != null) {
    return logoUrl === '' ? null : resolveIconPath(normalizeCatalogIconPath(logoUrl))
  }

  const iconLinks = (technologyConnectors ?? [])
    .map((link) => ({ link, iconUrl: resolveTechnologyConnectorIconUrl(link) }))
    .filter((entry): entry is { link: TechnologyConnector; iconUrl: string } => !!entry.iconUrl)
  const selected = iconLinks.find(({ link }) => !!(link.is_primary_icon ?? link.isPrimaryIcon)) ?? iconLinks[0]
  return selected ? resolveIconPath(selected.iconUrl) : null
}
