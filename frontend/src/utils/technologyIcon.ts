const LEGACY_SLUG_MAP: Record<string, string> = {
  golang: 'go',
  'c-plusplus': 'cplusplus',
  'json-javascript-object-notation': 'json',
  'tailwind-css': 'tailwindcss',
  tailwind: 'tailwindcss',
  postgres: 'postgresql',
  node: 'nodejs',
  ts: 'typescript',
  js: 'javascript',
  'next.js': 'nextjs',
  k8s: 'kubernetes',
  dockerfile: 'docker',
  python3: 'python',
  cpp: 'cplusplus',
  'c#': 'csharp',
  container: 'docker',
}

export function canonicalTechnologySlug(slug: string | null | undefined): string {
  const clean = (slug ?? '').trim().toLowerCase()
  return LEGACY_SLUG_MAP[clean] ?? clean
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
