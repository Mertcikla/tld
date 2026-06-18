import { fas } from '@fortawesome/free-solid-svg-icons'

type FontAwesomeSvgPath = string | string[]

interface FontAwesomeIconDefinition {
  icon: [number, number, string[], string, FontAwesomeSvgPath]
}

const FONT_AWESOME_ICON_FILL = '#E2E8F0'
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

function escapeSvgAttribute(value: string): string {
  return value
    .replace(/&/g, '&amp;')
    .replace(/"/g, '&quot;')
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
  const svg = `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 ${width} ${height}">${paths}</svg>`
  const iconUrl = `data:image/svg+xml;charset=utf-8,${encodeURIComponent(svg)}`
  iconUrlCache.set(cleanName, iconUrl)
  return iconUrl
}

export function fontAwesomeIconUrlForTechnologyLabel(label: string | null | undefined): string | null {
  return fontAwesomeIconUrlForName(parseFontAwesomeTechnologyIconName(label))
}
