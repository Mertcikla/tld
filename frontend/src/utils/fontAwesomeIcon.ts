import { fas } from '@fortawesome/free-solid-svg-icons'

type FontAwesomeSvgPath = string | string[]

interface FontAwesomeIconDefinition {
  icon: [number, number, string[], string, FontAwesomeSvgPath]
}

export interface FontAwesomeTechnologyIconMatch {
  iconName: string
  label: string
  iconUrl: string
}

const FONT_AWESOME_ICON_FILL = '#E2E8F0'
const FONT_AWESOME_ICON_VIEWBOX_PADDING_RATIO = 0.08
const FONT_AWESOME_PREFIX_PATTERN = /^(?:fa|fas):(.+)$/i
const FONT_AWESOME_STYLE_CLASSES = new Set([
  'fa-solid',
  'fa-regular',
  'fa-brands',
  'fa-light',
  'fa-thin',
  'fa-duotone',
  'fas',
  'far',
  'fab',
  'fal',
  'fat',
  'fad',
  'fa',
])

const solidIcons = fas as Record<string, FontAwesomeIconDefinition | undefined>
const iconUrlCache = new Map<string, string | null>()

function exportKeyForIconName(iconName: string): string {
  const suffix = iconName
    .split('-')
    .filter(Boolean)
    .map((part) => part.charAt(0).toUpperCase() + part.slice(1))
    .join('')
  return `fa${suffix}`
}

function iconNameForExportKey(exportKey: string): string {
  return exportKey
    .replace(/^fa/, '')
    .replace(/([a-z0-9])([A-Z])/g, '$1-$2')
    .toLowerCase()
}

function toKebabFontAwesomeIconName(rawIconName: string): string {
  return rawIconName
    .trim()
    .toLowerCase()
    .replace(/_/g, '-')
    .split(/\s+/)
    .filter((token) => token && !FONT_AWESOME_STYLE_CLASSES.has(token))
    .map((token) => token.replace(/^fa-/, ''))
    .join('-')
    .replace(/[^a-z0-9-]/g, '-')
    .replace(/-+/g, '-')
    .replace(/^-+|-+$/g, '')
}

function cleanFontAwesomeIconName(rawIconName: string): string {
  const normalized = rawIconName.trim().toLowerCase().replace(/_/g, '-')
  const tokens = normalized.split(/\s+/).filter(Boolean)
  const iconToken = [...tokens].reverse().find((token) => (
    token.startsWith('fa-') && !FONT_AWESOME_STYLE_CLASSES.has(token)
  )) ?? tokens[tokens.length - 1] ?? ''

  return iconToken
    .replace(/^fa-/, '')
    .replace(/[^a-z0-9-]/g, '')
    .replace(/^-+|-+$/g, '')
}

function fontAwesomeIconNameCandidates(value: string | null | undefined): string[] {
  const trimmed = (value ?? '').trim()
  if (!trimmed) return []

  const prefixedMatch = trimmed.match(FONT_AWESOME_PREFIX_PATTERN)
  const body = prefixedMatch ? prefixedMatch[1] : trimmed
  const candidates = [
    toKebabFontAwesomeIconName(body),
    cleanFontAwesomeIconName(body),
  ]

  return Array.from(new Set(candidates.filter(Boolean)))
}

function findFontAwesomeIconNameByPrefix(candidates: string[]): string | null {
  const searchableCandidates = candidates.filter((candidate) => candidate.length >= 4)
  if (searchableCandidates.length === 0) return null

  for (const candidate of searchableCandidates) {
    let bestMatch: string | null = null
    for (const exportKey of Object.keys(solidIcons)) {
      const iconName = iconNameForExportKey(exportKey)
      if (!iconName.startsWith(candidate)) continue
      if (!bestMatch || iconName.length < bestMatch.length || (iconName.length === bestMatch.length && iconName < bestMatch)) {
        bestMatch = iconName
      }
    }
    if (bestMatch) return bestMatch
  }
  return null
}

function escapeSvgAttribute(value: string): string {
  return value
    .replace(/&/g, '&amp;')
    .replace(/"/g, '&quot;')
}

function formatSvgNumber(value: number): string {
  return Number.isInteger(value)
    ? String(value)
    : value.toFixed(3).replace(/\.?0+$/, '')
}

function paddedSvgViewBox(width: number, height: number): string {
  const padX = width * FONT_AWESOME_ICON_VIEWBOX_PADDING_RATIO
  const padY = height * FONT_AWESOME_ICON_VIEWBOX_PADDING_RATIO

  return [
    -padX,
    -padY,
    width + padX * 2,
    height + padY * 2,
  ].map(formatSvgNumber).join(' ')
}

export function parseFontAwesomeTechnologyIconName(value: string | null | undefined): string | null {
  const match = (value ?? '').trim().match(FONT_AWESOME_PREFIX_PATTERN)
  if (!match) return null

  const iconName = cleanFontAwesomeIconName(match[1])
  return iconName || null
}

export function fontAwesomeIconUrlForName(iconName: string | null | undefined): string | null {
  const cleanName = cleanFontAwesomeIconName(iconName ?? '')
  if (!cleanName) return null

  if (iconUrlCache.has(cleanName)) {
    return iconUrlCache.get(cleanName) ?? null
  }

  const definition = solidIcons[exportKeyForIconName(cleanName)]
  if (!definition) {
    iconUrlCache.set(cleanName, null)
    return null
  }

  const [width, height, , , svgPathData] = definition.icon
  const paths = (Array.isArray(svgPathData) ? svgPathData : [svgPathData])
    .map((path) => `<path fill="${FONT_AWESOME_ICON_FILL}" d="${escapeSvgAttribute(path)}"/>`)
    .join('')
  const svg = `<svg xmlns="http://www.w3.org/2000/svg" width="${width}" height="${height}" viewBox="${paddedSvgViewBox(width, height)}">${paths}</svg>`
  const iconUrl = `data:image/svg+xml;charset=utf-8,${encodeURIComponent(svg)}`
  iconUrlCache.set(cleanName, iconUrl)
  return iconUrl
}

export function fontAwesomeIconUrlForTechnologyLabel(label: string | null | undefined): string | null {
  return fontAwesomeIconUrlForName(parseFontAwesomeTechnologyIconName(label))
}

export function matchFontAwesomeTechnologyIconQuery(value: string | null | undefined): FontAwesomeTechnologyIconMatch | null {
  const candidates = fontAwesomeIconNameCandidates(value)
  for (const candidate of candidates) {
    const iconUrl = fontAwesomeIconUrlForName(candidate)
    if (iconUrl) {
      return { iconName: candidate, label: `fa:${candidate}`, iconUrl }
    }
  }

  const prefixMatch = findFontAwesomeIconNameByPrefix(candidates)
  if (!prefixMatch) return null

  const iconUrl = fontAwesomeIconUrlForName(prefixMatch)
  return iconUrl ? { iconName: prefixMatch, label: `fa:${prefixMatch}`, iconUrl } : null
}
