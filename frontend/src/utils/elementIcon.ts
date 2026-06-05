import type { TechnologyConnector } from '../types'
import { catalogIconUrlForSlug, normalizeCatalogIconPath } from './technologyIcon'
import { resolveIconPath } from './url'

export function resolveElementIconUrl(
  logoUrl: string | null | undefined,
  technologyConnectors: TechnologyConnector[] | null | undefined,
): string | null {
  if (logoUrl != null) {
    return logoUrl === '' ? null : resolveIconPath(normalizeCatalogIconPath(logoUrl))
  }

  const catalogLinks = technologyConnectors?.filter((link) => link.type === 'catalog' && !!link.slug) ?? []
  const selected = catalogLinks.find((link) => (
    link.type === 'catalog' &&
    !!(link.is_primary_icon ?? link.isPrimaryIcon) &&
    !!link.slug
  )) ?? catalogLinks[0]
  if (!selected?.slug) return null
  const iconUrl = catalogIconUrlForSlug(selected.slug)
  return iconUrl ? resolveIconPath(iconUrl) : null
}
