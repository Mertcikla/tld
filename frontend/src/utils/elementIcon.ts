import type { TechnologyConnector } from '../types'
import { resolveIconPath } from './url'

export function resolveElementIconUrl(
  logoUrl: string | null | undefined,
  technologyConnectors: TechnologyConnector[] | null | undefined,
): string | null {
  if (logoUrl != null) {
    return logoUrl === '' ? null : resolveIconPath(logoUrl)
  }

  const catalogLinks = technologyConnectors?.filter((link) => link.type === 'catalog' && !!link.slug) ?? []
  const selected = catalogLinks.find((link) => (
    link.type === 'catalog' &&
    !!(link.is_primary_icon ?? link.isPrimaryIcon) &&
    !!link.slug
  )) ?? catalogLinks[0]
  if (!selected?.slug) return null
  return resolveIconPath(`/icons/${selected.slug}.png`)
}
