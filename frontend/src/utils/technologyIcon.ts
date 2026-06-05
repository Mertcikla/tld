export function canonicalTechnologySlug(slug: string | null | undefined): string {
  return (slug ?? '').trim().toLowerCase()
}

export function catalogIconUrlForSlug(slug: string | null | undefined): string | null {
  const canonicalSlug = canonicalTechnologySlug(slug)
  return canonicalSlug ? `/icons/${canonicalSlug}.svg` : null
}

export function normalizeCatalogIconPath(urlOrPath: string): string {
  const match = urlOrPath.match(/^(\/app)?\/icons\/([^/?#]+)\.(png|svg)([?#].*)?$/i)
  if (!match) return urlOrPath

  const appPrefix = match[1] ?? ''
  const canonicalSlug = canonicalTechnologySlug(match[2])
  const suffix = match[4] ?? ''
  if (!canonicalSlug) return urlOrPath
  return `${appPrefix}/icons/${canonicalSlug}.svg${suffix}`
}
