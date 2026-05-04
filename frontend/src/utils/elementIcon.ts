import type { TechnologyConnector } from '../types'
import { resolveIconPath } from './url'

export function resolveElementIconUrl(
  logoUrl: string | null | undefined,
  technologyConnectors: TechnologyConnector[] | null | undefined,
): string | null {
  if (logoUrl != null) {
    return logoUrl === '' ? null : resolveIconPath(logoUrl)
  }

  const selected = technologyConnectors?.find((link) => (
    link.type === 'catalog' &&
    !!(link.is_primary_icon ?? link.isPrimaryIcon) &&
    !!link.slug
  ))
  if (!selected?.slug) return null
  return resolveIconPath(`/icons/${selected.slug}.png`)
}
