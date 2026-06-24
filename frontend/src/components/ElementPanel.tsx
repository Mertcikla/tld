import { memo, useEffect, useRef, useState, useCallback } from 'react'
import type { ReactNode } from 'react'
import type { ElementPanelSlots } from '../slots'
import { useNavigate } from 'react-router-dom'
import {
  Badge,
  Box,
  Button,
  CloseButton,
  Flex,
  FormControl,
  FormLabel,
  HStack,
  Input,
  InputGroup,
  InputRightElement,
  Popover,
  PopoverArrow,
  PopoverBody,
  PopoverContent,
  PopoverTrigger,
  Slider,
  SliderFilledTrack,
  SliderThumb,
  SliderTrack,
  Switch,
  Tag,
  TagCloseButton,
  TagLabel,
  Text,
  Textarea,
  useBreakpointValue,
  useDisclosure,
  useToast,
  VStack,
  Wrap,
  WrapItem,
} from '@chakra-ui/react'

import { api } from '../api/client'
import { ELEMENT_TYPES, type LibraryElement, type ViewConnector, type TechnologyCatalogItem, type TechnologyConnector } from '../types'
import ConfirmDialog from './ConfirmDialog'
import SlidingPanel from './SlidingPanel'
import PanelHeader from './PanelHeader'
import GitSourceLinker from './GitSourceLinker'
import { getTechnologyCatalogIndex, getTechnologyCatalogItemBySlug, invalidateTechnologyCatalog, resolveWithBase, searchTechnologyCatalog } from '../utils/technologyCatalog'
import { canonicalTechnologySlug } from '../utils/technologyIcon'
import { resolveTechnologyConnectorIconUrl, technologyConnectorIconKey } from '../utils/elementIcon'
import { matchFontAwesomeTechnologyIconQuery } from '../utils/fontAwesomeIcon'
import { ChevronDownIcon, ImageUploadIcon, ZoomInIcon, ZoomOutIcon } from './Icons'
import ScrollIndicatorWrapper from './ScrollIndicatorWrapper'
import TagUpsert from './TagUpsert'
import { openExternalUrl } from '../lib/desktop'

import { useViewEditorContext } from '../pages/ViewEditor/context'

function normalizeTechnologyLabel(value: string): string {
  return value.trim().replace(/\s+/g, ' ').toLowerCase()
}

function splitTechnologyLabel(value: string): string[] {
  return value.split(',').map((part) => part.trim()).filter(Boolean)
}

function parseCustomTechnologyAliases(value: string): string[] {
  const aliases = value
    .split(/[\n,]/)
    .map((part) => part.trim())
    .filter(Boolean)
  return Array.from(new Set(aliases))
}

function mediaTypeForIconFile(file: File): string {
  if (file.type) return file.type
  const lowerName = file.name.toLowerCase()
  if (lowerName.endsWith('.svg')) return 'image/svg+xml'
  if (lowerName.endsWith('.png')) return 'image/png'
  return ''
}

function defaultTechnologyNameFromFile(file: File): string {
  const withoutExt = file.name.replace(/\.[^.]+$/, '')
  return withoutExt.replace(/[-_]+/g, ' ').replace(/\s+/g, ' ').trim()
}

function findCatalogItemByLabel(index: Awaited<ReturnType<typeof getTechnologyCatalogIndex>>, label: string): TechnologyCatalogItem | null {
  const normalized = normalizeTechnologyLabel(label)
  if (!normalized) return null

  const bySlugMatch = index.bySlug.get(canonicalTechnologySlug(label))
  if (bySlugMatch) return bySlugMatch

  return index.items.find((item) => (
    normalizeTechnologyLabel(item.name) === normalized ||
    normalizeTechnologyLabel(item.nameShort) === normalized ||
    normalizeTechnologyLabel(item.defaultSlug) === normalized ||
    (item.aliases ?? []).some((alias) => normalizeTechnologyLabel(alias) === normalized)
  )) ?? null
}

function dedupeTechnologyLinks(links: TechnologyConnector[]): TechnologyConnector[] {
  const seenCatalog = new Set<string>()
  const seenCustom = new Set<string>()
  const result: TechnologyConnector[] = []
  let primarySet = false

  // Sort links to process primary ones first, ensuring they are preserved during deduping
  const sortedLinks = [...links].sort((a, b) => {
    const aPrimary = !!(a.is_primary_icon ?? a.isPrimaryIcon)
    const bPrimary = !!(b.is_primary_icon ?? b.isPrimaryIcon)
    if (aPrimary && !bPrimary) return -1
    if (!aPrimary && bPrimary) return 1
    return 0
  })

  for (const link of sortedLinks) {
    const label = link.label.trim()
    if (!label) continue

    const isPrimary = !!(link.is_primary_icon ?? link.isPrimaryIcon)

    if (link.type === 'catalog' && link.slug) {
      const slug = canonicalTechnologySlug(link.slug)
      if (!slug) continue
      const key = slug
      if (seenCatalog.has(key)) continue
      seenCatalog.add(key)
      result.push({
        type: 'catalog',
        slug,
        label,
        is_primary_icon: !primarySet && isPrimary,
      })
      if (isPrimary) primarySet = true
      continue
    }

    const key = normalizeTechnologyLabel(label)
    if (seenCustom.has(key)) continue
    seenCustom.add(key)
    const customLink: TechnologyConnector = { type: 'custom', label }
    const canUseAsPrimaryIcon = !!technologyConnectorIconKey(customLink)
    result.push({
      ...customLink,
      is_primary_icon: canUseAsPrimaryIcon && !primarySet && isPrimary,
    })
    if (canUseAsPrimaryIcon && isPrimary) primarySet = true
  }

  return result.slice(0, 3)
}

async function normalizeInitialTechnologyLinks(element: LibraryElement): Promise<TechnologyConnector[]> {
  const rawLinks = element.technology_connectors ?? []
  const legacyLabels = splitTechnologyLabel(element.technology ?? '')

  if (rawLinks.length === 0 && legacyLabels.length === 0) return []

  const index = await getTechnologyCatalogIndex()
  const normalized: TechnologyConnector[] = []

  const pushLabel = (label: string, isPrimaryIcon = false) => {
    const match = findCatalogItemByLabel(index, label)
    if (match) {
      normalized.push({
        type: 'catalog',
        slug: match.defaultSlug,
        label: match.name,
        is_primary_icon: isPrimaryIcon,
      })
    } else {
      const customLink: TechnologyConnector = { type: 'custom', label: label.trim() }
      normalized.push({
        ...customLink,
        is_primary_icon: !!isPrimaryIcon && !!technologyConnectorIconKey(customLink),
      })
    }
  }

  if (rawLinks.length > 0) {
    for (const link of rawLinks) {
      if (link.type === 'catalog') {
        const match = link.slug ? index.bySlug.get(canonicalTechnologySlug(link.slug)) : null
        if (match) {
          normalized.push({
            type: 'catalog',
            slug: match.defaultSlug,
            label: match.name,
            is_primary_icon: !!(link.is_primary_icon ?? link.isPrimaryIcon),
          })
        } else if (link.label.trim()) {
          const customLink: TechnologyConnector = { type: 'custom', label: link.label.trim() }
          normalized.push({
            ...customLink,
            is_primary_icon: !!(link.is_primary_icon ?? link.isPrimaryIcon) && !!technologyConnectorIconKey(customLink),
          })
        }
      } else {
        const parts = splitTechnologyLabel(link.label)
        if (parts.length > 1) {
          for (const part of parts) pushLabel(part)
        } else {
          pushLabel(link.label, !!(link.is_primary_icon ?? link.isPrimaryIcon))
        }
      }
    }
  } else {
    for (const label of legacyLabels) {
      pushLabel(label)
    }
  }

  // If no catalog item is primary, try to match against element.logo_url. An
  // explicit empty logo_url means the user deselected technology icons.
  const deduped = dedupeTechnologyLinks(normalized)
  const hasPrimary = deduped.some(l => !!technologyConnectorIconKey(l) && l.is_primary_icon)
  if (!hasPrimary && element.logo_url !== '') {
    let bestMatchIndex = -1
    if (element.logo_url) {
      bestMatchIndex = deduped.findIndex(l => l.type === 'catalog' && l.slug && element.logo_url?.toLowerCase().includes(l.slug.toLowerCase()))
    }

    if (bestMatchIndex !== -1) {
      deduped[bestMatchIndex].is_primary_icon = true
    }
  }

  return deduped
}

function buildTechnologyFingerprintPayload(
  element: LibraryElement,
  links: TechnologyConnector[],
  type: string,
) {
  const normalizedLinks = links.map(serializeTechnologyLinkForSave)
  const normalizedType = type.trim().toLowerCase()
  const technology = links.map((link) => link.label).join(', ')

  return {
    name: element.name,
    description: element.description ?? '',
    kind: normalizedType,
    technology,
    url: element.url ?? '',
    logo_url: element.logo_url ?? '',
    technology_connectors: normalizedLinks,
    tags: element.tags ?? [],
    bypass_noise_gate: element.bypass_noise_gate ?? false,
    repo: element.repo,
    branch: element.branch,
    file_path: element.file_path,
    language: element.language,
  }
}

function serializeTechnologyLinkForSave(link: TechnologyConnector): TechnologyConnector {
  const isPrimaryIcon = !!(link.is_primary_icon ?? link.isPrimaryIcon)
  const iconKey = technologyConnectorIconKey(link)
  if (link.type === 'custom' && isPrimaryIcon && iconKey?.startsWith('fa:')) {
    return {
      type: 'catalog',
      slug: iconKey,
      label: link.label,
      is_primary_icon: true,
    }
  }

  return {
    type: link.type,
    slug: link.type === 'catalog' ? link.slug : undefined,
    label: link.label,
    is_primary_icon: link.type === 'catalog' && isPrimaryIcon,
  }
}

type AutoSaveOverrides = {
  tags?: string[]
  technologyLinks?: TechnologyConnector[]
  explicitLogoClear?: boolean
}

const NOISE_GATE_STOPS = [
  { value: -2, label: 'Quiet' },
  { value: -1, label: 'Lean' },
  { value: 0, label: 'Normal' },
  { value: 1, label: 'Rich' },
  { value: 2, label: 'Full' },
] as const

const AUTO_SAVE_DELAY_MS = 150

function clampNoiseGateLevel(level: number) {
  return Math.max(-2, Math.min(2, level))
}

function noiseGateLevelFromVisibilityDelta(delta: number) {
  return clampNoiseGateLevel(-delta)
}

function visibilityDeltaFromNoiseGateLevel(level: number) {
  return -clampNoiseGateLevel(level)
}

export interface ElementPanelProps extends ElementPanelSlots {
  isOpen: boolean
  onClose: () => void
  element?: LibraryElement | null
  onSave: (obj: LibraryElement) => void
  autoSave?: boolean
  onDelete?: (id: number) => void
  onPermanentDelete?: (id: number) => void
  onMerge?: (id: number) => void
  visibilityOverrideDelta?: number
  onVisibilityOverrideDeltaChange?: (id: number, delta: number) => Promise<void> | void
  onPromoteVisibility?: (id: number) => Promise<void> | void
  onDemoteVisibility?: (id: number) => Promise<void> | void
  onResetVisibility?: (id: number) => Promise<void> | void
  orgId?: string
  links?: ViewConnector[]
  parentLinks?: ViewConnector[]
  hasBackdrop?: boolean
  availableTags?: string[]
  noFocusLock?: boolean
  isInline?: boolean
  actions?: ReactNode
}

/**
 * Name: Edit Element Panel
 * Role: Opens when clicked on an element and displays its fields, allowing for editing.
 * Location: Right side of the screen on desktop. Overlays screen on mobile.
 * Aliases: Element Properties, Element Details.
 */
function ElementPanel({
  isOpen,
  onClose,
  element,
  onSave,
  autoSave = false,
  onDelete,
  onPermanentDelete,
  onMerge,
  visibilityOverrideDelta = 0,
  onVisibilityOverrideDeltaChange,
  onPromoteVisibility,
  onDemoteVisibility,
  onResetVisibility,
  orgId,
  links = [],
  parentLinks = [],
  hasBackdrop = true,
  availableTags = [],
  noFocusLock,
  elementPanelAfterContentSlot,
  isInline = false,
  actions,
}: ElementPanelProps) {
  const { canEdit, viewId } = useViewEditorContext()
  const isEdit = !!element
  const isReadOnly = !canEdit
  const autoSaveEdit = autoSave && isEdit && !isReadOnly
  const navigate = useNavigate()
  const toast = useToast()
  const [name, setName] = useState('')
  const [description, setDescription] = useState('')
  const [type, setType] = useState('')
  const [typeQuery, setTypeQuery] = useState('')
  const [typeResults, setTypeResults] = useState<string[]>([])
  const [url, setUrl] = useState('')
  const [technologyLinks, setTechnologyConnectors] = useState<TechnologyConnector[]>([])
  const [technologyQuery, setTechnologyQuery] = useState('')
  const [technologyResults, setTechnologyResults] = useState<TechnologyCatalogItem[]>([])
  const [technologyMeta, setTechnologyMeta] = useState<Record<string, TechnologyCatalogItem>>({})
  const [technologySearchLoading, setTechnologySearchLoading] = useState(false)
  const [technologySearchSettledQuery, setTechnologySearchSettledQuery] = useState('')
  const [tags, setTags] = useState<string[]>([])
  const [bypassNoiseGate, setBypassNoiseGate] = useState(false)
  const [loading, setLoading] = useState(false)
  const [explicitLogoClear, setExplicitLogoClear] = useState(false)
  const [draftNoiseGateLevel, setDraftNoiseGateLevel] = useState(() => noiseGateLevelFromVisibilityDelta(visibilityOverrideDelta))
  const typeInputRef = useRef<HTMLInputElement>(null)
  const techInputRef = useRef<HTMLInputElement>(null)
  const suppressTypeBlurRef = useRef(false)
  const lastSavedFingerprintRef = useRef<string>('')
  const savingRef = useRef(false)
  const pendingSaveRef = useRef(false)
  const pendingSaveOverridesRef = useRef<AutoSaveOverrides | undefined>(undefined)
  const [techResultIndex, setTechResultIndex] = useState(-1)
  const confirmPermanentDelete = useDisclosure()
  const customTechnologyFileInputRef = useRef<HTMLInputElement>(null)
  const [customTechnologyShortName, setCustomTechnologyShortName] = useState('')
  const [customTechnologyAliases, setCustomTechnologyAliases] = useState('')
  const [customTechnologyFile, setCustomTechnologyFile] = useState<File | null>(null)
  const [customTechnologyPreviewUrl, setCustomTechnologyPreviewUrl] = useState('')
  const [customTechnologyError, setCustomTechnologyError] = useState('')
  const [customTechnologySaving, setCustomTechnologySaving] = useState(false)
  const [customTechnologyExpanded, setCustomTechnologyExpanded] = useState(false)
  const [customTechnologyOptionsOpen, setCustomTechnologyOptionsOpen] = useState(false)
  const isMobile = useBreakpointValue({ base: true, md: false }) ?? false
  const initializedElementIdRef = useRef<number | null>(null)

  useEffect(() => {
    setTechResultIndex(-1)
    setTechnologySearchSettledQuery('')
    setCustomTechnologyExpanded(false)
    setCustomTechnologyOptionsOpen(false)
  }, [technologyQuery])

  useEffect(() => {
    return () => {
      if (customTechnologyPreviewUrl.startsWith('blob:')) {
        URL.revokeObjectURL(customTechnologyPreviewUrl)
      }
    }
  }, [customTechnologyPreviewUrl])

  useEffect(() => {
    let cancelled = false

    if (!isOpen) {
      initializedElementIdRef.current = null
      return () => {
        cancelled = true
      }
    }

    if (element) {
      if (initializedElementIdRef.current === element.id) {
        return () => {
          cancelled = true
        }
      }
      initializedElementIdRef.current = element.id
      setName(element.name)
      setDescription(element.description ?? '')
      setType(element.kind ?? '')
      setTypeQuery('')
      setTypeResults([])
      setUrl(element.url ?? '')
      setTags(element.tags ?? [])
      setBypassNoiseGate(element.bypass_noise_gate ?? false)
      setExplicitLogoClear(element.logo_url === '')

      const linksFromElement = (element.technology_connectors ?? []).map(tl => ({
        ...tl,
        is_primary_icon: !!(tl.is_primary_icon ?? tl.isPrimaryIcon),
      }))
      const fallbackLinks: TechnologyConnector[] = linksFromElement.length > 0
        ? linksFromElement
        : (element.technology ? [{ type: 'custom', label: element.technology, is_primary_icon: false }] : [])
      setTechnologyConnectors(fallbackLinks)
      lastSavedFingerprintRef.current = JSON.stringify(buildTechnologyFingerprintPayload(
        element,
        fallbackLinks,
        element.kind ?? '',
      ))

      normalizeInitialTechnologyLinks(element)
        .then((initialLinks) => {
          if (cancelled) return
          setTechnologyConnectors(initialLinks)
          lastSavedFingerprintRef.current = JSON.stringify(buildTechnologyFingerprintPayload(
            element,
            initialLinks,
            element.kind ?? '',
          ))
        })
        .catch(() => {
          if (cancelled) return
          setTechnologyConnectors(fallbackLinks)
          lastSavedFingerprintRef.current = JSON.stringify(buildTechnologyFingerprintPayload(
            element,
            fallbackLinks,
            element.kind ?? '',
          ))
        })
    } else {
      initializedElementIdRef.current = null
      setName('')
      setDescription('')
      setType('')
      setTypeQuery('')
      setTypeResults([])
      setUrl('')
      setTechnologyConnectors([])
      setTechnologyQuery('')
      setTechnologyResults([])
      setTechnologyMeta({})
      setTags([])
      setBypassNoiseGate(false)
      setExplicitLogoClear(false)
      lastSavedFingerprintRef.current = ''
    }

    return () => {
      cancelled = true
    }
  }, [element, isOpen])

  const resolveTechnologyLogoUrlForSave = useCallback(async (link: TechnologyConnector | undefined): Promise<string> => {
    if (!link) return ''

    const directIconUrl = resolveTechnologyConnectorIconUrl(link)
    if (link.type !== 'catalog' || !link.slug) return directIconUrl ?? ''

    const slug = link.slug
    const cached = technologyMeta[slug]
    if (cached?.iconUrl) return cached.iconUrl

    try {
      const item = await getTechnologyCatalogItemBySlug(slug)
      if (item) {
        setTechnologyMeta((prev) => ({ ...prev, [slug]: item }))
        return item.iconUrl || directIconUrl || ''
      }
    } catch {
      // ignore
    }

    return directIconUrl ?? ''
  }, [technologyMeta])

  const buildPayloadAndFingerprint = useCallback(async (overrides?: AutoSaveOverrides) => {
    const tagsForSave = overrides?.tags ?? tags
    const linksForSave = overrides?.technologyLinks ?? technologyLinks
    const explicitLogoClearForSave = overrides?.explicitLogoClear ?? explicitLogoClear

    const primaryLink = linksForSave.find((link) => (
      !!technologyConnectorIconKey(link) && !!(link.is_primary_icon ?? link.isPrimaryIcon)
    ))

    const normalizedLinks = linksForSave.map(serializeTechnologyLinkForSave)

    const normalizedType = type.trim().toLowerCase()

    let logoUrl = element?.logo_url ?? ''
    if (explicitLogoClearForSave) {
      logoUrl = ''
    }
    if (!explicitLogoClearForSave && primaryLink) {
      logoUrl = await resolveTechnologyLogoUrlForSave(primaryLink)
    }

    const payload = {
      name,
      description,
      kind: normalizedType,
      technology: linksForSave.map((link) => link.label).join(', '),
      url,
      logo_url: logoUrl,
      technology_connectors: normalizedLinks,
      tags: tagsForSave,
      bypass_noise_gate: bypassNoiseGate,
      repo: element?.repo,
      branch: element?.branch,
      file_path: element?.file_path,
      language: element?.language,
    }
    return { payload, fingerprint: JSON.stringify(payload) }
  }, [technologyLinks, explicitLogoClear, type, element, name, description, url, tags, bypassNoiseGate, resolveTechnologyLogoUrlForSave])

  const saveIfDirty = useCallback(async (overrides?: AutoSaveOverrides) => {
    if (!autoSaveEdit || !element) return
    if (!name.trim()) return

    if (savingRef.current) {
      pendingSaveRef.current = true
      pendingSaveOverridesRef.current = overrides ?? pendingSaveOverridesRef.current
      return
    }

    savingRef.current = true
    try {
      const { payload, fingerprint } = await buildPayloadAndFingerprint(overrides)
      if (fingerprint === lastSavedFingerprintRef.current) return
      const saved = await api.elements.update(element.id, payload)
      lastSavedFingerprintRef.current = fingerprint
      onSave(saved)
    } catch {
      // ignore
    } finally {
      savingRef.current = false
      if (pendingSaveRef.current) {
        const pendingOverrides = pendingSaveOverridesRef.current
        pendingSaveRef.current = false
        pendingSaveOverridesRef.current = undefined
        window.setTimeout(() => {
          void saveIfDirtyRef.current?.(pendingOverrides)
        }, AUTO_SAVE_DELAY_MS)
      }
    }
  }, [autoSaveEdit, element, name, buildPayloadAndFingerprint, onSave])

  const saveIfDirtyRef = useRef<((overrides?: AutoSaveOverrides) => Promise<void>) | null>(null)
  useEffect(() => { saveIfDirtyRef.current = saveIfDirty }, [saveIfDirty])

  const scheduleAutoSave = (overrides?: AutoSaveOverrides) => {
    if (!autoSaveEdit) return
    window.setTimeout(() => {
      void saveIfDirtyRef.current?.(overrides)
    }, AUTO_SAVE_DELAY_MS)
  }

  useEffect(() => {
    if (!autoSaveEdit || !element) return
    const timer = window.setTimeout(() => {
      void saveIfDirtyRef.current?.()
    }, AUTO_SAVE_DELAY_MS)
    return () => window.clearTimeout(timer)
  }, [autoSaveEdit, element, name, description, type, url, tags, technologyLinks, explicitLogoClear, bypassNoiseGate])

  const handleClose = useCallback(async () => {
    if (autoSaveEdit) {
      await saveIfDirtyRef.current?.()
    }
    onClose()
  }, [autoSaveEdit, onClose])

  useEffect(() => {
    setDraftNoiseGateLevel(noiseGateLevelFromVisibilityDelta(visibilityOverrideDelta))
  }, [element?.id, visibilityOverrideDelta])

  const handleNoiseGateChange = useCallback(async (nextLevel: number) => {
    if (!element) return

    const nextDelta = visibilityDeltaFromNoiseGateLevel(nextLevel)
    const currentDelta = visibilityDeltaFromNoiseGateLevel(noiseGateLevelFromVisibilityDelta(visibilityOverrideDelta))

    try {
      if (onVisibilityOverrideDeltaChange) {
        await onVisibilityOverrideDeltaChange(element.id, nextDelta)
        return
      }

      if (nextDelta === currentDelta) return

      let delta = currentDelta
      while (delta < nextDelta) {
        if (!onPromoteVisibility) break
        await onPromoteVisibility(element.id)
        delta += 1
      }
      while (delta > nextDelta) {
        if (!onDemoteVisibility) break
        await onDemoteVisibility(element.id)
        delta -= 1
      }
      if (delta !== nextDelta && nextDelta === 0 && onResetVisibility) {
        await onResetVisibility(element.id)
      }
    } catch {
      setDraftNoiseGateLevel(noiseGateLevelFromVisibilityDelta(visibilityOverrideDelta))
    }
  }, [element, onDemoteVisibility, onPromoteVisibility, onResetVisibility, onVisibilityOverrideDeltaChange, visibilityOverrideDelta])

  useEffect(() => {
    if (!isOpen) return
    const query = typeQuery.trim()
    if (!query) {
      setTypeResults([])
      return
    }

    const allTypes = Array.from(new Set([
      ...ELEMENT_TYPES,
      ...(type ? [type] : []),
    ]))

    try {
      const regex = new RegExp(query, 'i')
      setTypeResults(allTypes.filter((t) => regex.test(t)).slice(0, 12))
    } catch {
      const needle = query.toLowerCase()
      setTypeResults(allTypes.filter((t) => t.toLowerCase().includes(needle)).slice(0, 12))
    }
  }, [isOpen, typeQuery, type])

  useEffect(() => {
    if (!isOpen) return
    const slugs = technologyLinks
      .filter((link) => link.type === 'catalog' && !!link.slug)
      .map((link) => link.slug as string)

    if (slugs.length === 0) return

    getTechnologyCatalogIndex().then((index) => {
      setTechnologyMeta((prev) => {
        const next = { ...prev }
        for (const slug of slugs) {
          const item = index.bySlug.get(canonicalTechnologySlug(slug))
          if (item) {
            next[slug] = item
            next[item.defaultSlug] = item
          }
        }
        return next
      })
    }).catch(() => { /* intentionally empty */ })
  }, [isOpen, technologyLinks])

  useEffect(() => {
    if (!isOpen) return
    const query = technologyQuery.trim()
    if (!query) {
      setTechnologyResults([])
      setTechnologySearchLoading(false)
      setTechnologySearchSettledQuery('')
      return
    }

    let cancelled = false
    const timer = setTimeout(() => {
      setTechnologySearchLoading(true)
      searchTechnologyCatalog(query)
        .then((results) => {
          if (cancelled) return
          setTechnologyResults(results)
          setTechnologySearchSettledQuery(query)
          setTechnologyMeta((prev) => {
            const next = { ...prev }
            for (const item of results) {
              next[item.defaultSlug] = item
            }
            return next
          })
        })
        .catch(() => {
          if (cancelled) return
          setTechnologyResults([])
          setTechnologySearchSettledQuery(query)
        })
        .finally(() => {
          if (!cancelled) setTechnologySearchLoading(false)
        })
    }, 140)

    return () => {
      cancelled = true
      clearTimeout(timer)
    }
  }, [isOpen, technologyQuery])

  const handleSave = useCallback(async () => {
    if (isReadOnly || !name.trim()) return
    setLoading(true)
    try {
      const primaryLink = technologyLinks.find((link) => (
        !!technologyConnectorIconKey(link) && !!(link.is_primary_icon ?? link.isPrimaryIcon)
      ))
      const logoUrl = explicitLogoClear ? '' : await resolveTechnologyLogoUrlForSave(primaryLink)

      const normalizedLinks = technologyLinks.map((link) => {
        const serialized = serializeTechnologyLinkForSave(link)
        return {
          ...serialized,
          slug: serialized.type === 'catalog' ? canonicalTechnologySlug(serialized.slug) : undefined,
        }
      })

      const normalizedType = type.trim().toLowerCase()

      const payload = {
        name,
        description,
        kind: normalizedType,
        technology: technologyLinks.map((link) => link.label).join(', '),
        url,
        logo_url: logoUrl,
        technology_connectors: normalizedLinks,
        tags,
        bypass_noise_gate: bypassNoiseGate,
      }
      const saved = isEdit
        ? await api.elements.update(element!.id, payload)
        : await api.elements.create(payload)
      onSave(saved)
      handleClose()
    } catch {
      // intentionally empty
    } finally {
      setLoading(false)
    }
  }, [isReadOnly, name, technologyLinks, type, explicitLogoClear, tags, bypassNoiseGate, isEdit, element, onSave, handleClose, description, url, resolveTechnologyLogoUrlForSave])

  useEffect(() => {
    if (!isOpen) return
    const handler = (e: KeyboardEvent) => {
      const target = e.target as HTMLElement
      const isInput = target?.tagName === 'INPUT' || target?.tagName === 'TEXTAREA' || target.isContentEditable

      if (e.key === 'Escape' && !isInput) handleClose()

      if (e.key === 'Enter' && (e.metaKey || e.ctrlKey)) {
        e.preventDefault()
        if (!autoSaveEdit) {
          handleSave()
        } else {
          handleClose()
        }
      }

      if (e.key.toLowerCase() === 't' && !isInput && !e.ctrlKey && !e.metaKey && !e.altKey) {
        e.preventDefault()
        techInputRef.current?.focus()
      }
    }
    window.addEventListener('keydown', handler)
    return () => window.removeEventListener('keydown', handler)
  }, [isOpen, handleClose, autoSaveEdit, handleSave])

  const addCatalogTechnology = (item: TechnologyCatalogItem) => {
    if (technologyLinks.length >= 3) return
    if (technologyLinks.some((link) => link.type === 'catalog' && link.slug === item.defaultSlug)) return

    const hasPrimaryIcon = technologyLinks.some((link) => (
      !!technologyConnectorIconKey(link) && !!(link.is_primary_icon ?? link.isPrimaryIcon)
    ))

    const nextLinks: TechnologyConnector[] = [
      ...technologyLinks,
      {
        type: 'catalog',
        slug: item.defaultSlug,
        label: item.name,
        is_primary_icon: !explicitLogoClear && !hasPrimaryIcon,
      },
    ]
    setTechnologyConnectors(nextLinks)
    setTechnologyQuery('')
    setTechnologyResults([])
    resetCustomTechnologyForm()
    setTechnologyMeta((prev) => ({ ...prev, [item.defaultSlug]: item }))
    scheduleAutoSave({ technologyLinks: nextLinks })
  }

  const addCustomTechnology = (labelOverride?: string) => {
    const value = (labelOverride ?? technologyQuery).trim()
    if (!value || technologyLinks.length >= 3) return

    const link: TechnologyConnector = { type: 'custom', label: value }
    const iconKey = technologyConnectorIconKey(link)
    if (technologyLinks.some((item) => (
      (item.type === 'custom' && item.label.toLowerCase() === value.toLowerCase()) ||
      (!!iconKey && technologyConnectorIconKey(item) === iconKey)
    ))) return

    const canUseAsPrimaryIcon = !!technologyConnectorIconKey(link)
    const hasPrimaryIcon = technologyLinks.some((item) => (
      !!technologyConnectorIconKey(item) && !!(item.is_primary_icon ?? item.isPrimaryIcon)
    ))
    const nextLinks: TechnologyConnector[] = [
      ...technologyLinks,
      {
        ...link,
        is_primary_icon: canUseAsPrimaryIcon && !explicitLogoClear && !hasPrimaryIcon,
      },
    ]
    setTechnologyConnectors(nextLinks)
    setTechnologyQuery('')
    setTechnologyResults([])
    resetCustomTechnologyForm()
    scheduleAutoSave({ technologyLinks: nextLinks })
  }

  const resetCustomTechnologyForm = () => {
    setCustomTechnologyShortName('')
    setCustomTechnologyAliases('')
    setCustomTechnologyFile(null)
    setCustomTechnologyPreviewUrl('')
    setCustomTechnologyError('')
    setCustomTechnologySaving(false)
    setCustomTechnologyExpanded(false)
    setCustomTechnologyOptionsOpen(false)
  }

  const openCustomTechnologyFilePicker = () => {
    if (customTechnologySaving) return
    customTechnologyFileInputRef.current?.click()
  }

  const setCustomTechnologyIconFile = (file: File | null) => {
    setCustomTechnologyError('')
    setCustomTechnologyFile(null)
    setCustomTechnologyPreviewUrl('')

    if (!file) return

    const mediaType = mediaTypeForIconFile(file)
    if (mediaType !== 'image/svg+xml' && mediaType !== 'image/png') {
      setCustomTechnologyError('Choose an SVG or PNG file.')
      return
    }
    if (file.size > 2 * 1024 * 1024) {
      setCustomTechnologyError('Choose a file under 2 MB.')
      return
    }

    setCustomTechnologyFile(file)
    if (!technologyQuery.trim()) {
      setTechnologyQuery(defaultTechnologyNameFromFile(file))
    }
    if (typeof URL !== 'undefined' && URL.createObjectURL) {
      setCustomTechnologyPreviewUrl(URL.createObjectURL(file))
    }
  }

  const handleCustomTechnologyFileChange = (event: React.ChangeEvent<HTMLInputElement>) => {
    setCustomTechnologyIconFile(event.target.files?.[0] ?? null)
  }

  const handleCustomTechnologyFileDrop = (event: React.DragEvent<HTMLButtonElement>) => {
    event.preventDefault()
    if (customTechnologySaving) return
    setCustomTechnologyIconFile(event.dataTransfer.files?.[0] ?? null)
  }

  const clearCustomTechnologyFile = () => {
    setCustomTechnologyIconFile(null)
    if (customTechnologyFileInputRef.current) {
      customTechnologyFileInputRef.current.value = ''
    }
  }

  const handleCreateCustomTechnology = async () => {
    if (isReadOnly || customTechnologySaving) return
    const trimmedName = technologyQuery.trim() || (customTechnologyFile ? defaultTechnologyNameFromFile(customTechnologyFile) : '')
    if (!trimmedName) {
      setCustomTechnologyError('Name is required.')
      return
    }
    if (!customTechnologyFile) {
      setCustomTechnologyError('Icon file is required.')
      return
    }
    if (technologyLinks.length >= 3) {
      setCustomTechnologyError('Remove a technology before attaching another one.')
      return
    }

    setCustomTechnologySaving(true)
    setCustomTechnologyError('')
    try {
      const icon = new Uint8Array(await customTechnologyFile.arrayBuffer())
      const item = await api.technology.createCustom({
        name: trimmedName,
        name_short: customTechnologyShortName.trim() || undefined,
        aliases: parseCustomTechnologyAliases(customTechnologyAliases),
        icon,
        media_type: mediaTypeForIconFile(customTechnologyFile),
      })
      invalidateTechnologyCatalog()
      setTechnologyMeta((prev) => ({ ...prev, [item.defaultSlug]: item }))
      setTechnologyConnectors((prev) => {
        if (prev.length >= 3) return prev
        if (prev.some((link) => link.type === 'catalog' && link.slug === item.defaultSlug)) return prev
        const hasPrimaryIcon = prev.some((link) => (
          !!technologyConnectorIconKey(link) && !!(link.is_primary_icon ?? link.isPrimaryIcon)
        ))
        const shouldBePrimary = !explicitLogoClear && !hasPrimaryIcon
        return [
          ...prev,
          {
            type: 'catalog',
            slug: item.defaultSlug,
            label: item.name,
            is_primary_icon: shouldBePrimary,
          },
        ]
      })
      setTechnologyQuery('')
      setTechnologyResults([])
      resetCustomTechnologyForm()
      scheduleAutoSave()
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Create failed'
      setCustomTechnologyError(message)
      toast({ status: 'error', title: 'Create failed', description: message })
    } finally {
      setCustomTechnologySaving(false)
    }
  }

  const removeTechnology = (linkToRemove: TechnologyConnector) => {
    const shouldClearLogo = !!technologyConnectorIconKey(linkToRemove) && !!(linkToRemove.is_primary_icon ?? linkToRemove.isPrimaryIcon)
    const nextExplicitLogoClear = shouldClearLogo ? true : explicitLogoClear
    const nextLinks = technologyLinks.filter((link) => (
      !(link.type === linkToRemove.type && link.slug === linkToRemove.slug && link.label === linkToRemove.label)
    ))
    if (shouldClearLogo) {
      setExplicitLogoClear(true)
    }
    setTechnologyConnectors(nextLinks)
    scheduleAutoSave({ technologyLinks: nextLinks, explicitLogoClear: nextExplicitLogoClear })
  }

  const togglePrimaryIcon = (selectedIconKey: string) => {
    const isDeselecting = selectedPrimaryIconKey === selectedIconKey
    const nextLinks = technologyLinks.map((link) => {
      return {
        ...link,
        is_primary_icon: !isDeselecting && technologyConnectorIconKey(link) === selectedIconKey,
      }
    })
    setTechnologyConnectors(nextLinks)
    setExplicitLogoClear(isDeselecting)
    scheduleAutoSave({ technologyLinks: nextLinks, explicitLogoClear: isDeselecting })
  }

  const selectedPrimaryIconLink = technologyLinks.find((link) => (
    !!technologyConnectorIconKey(link) && !!(link.is_primary_icon ?? link.isPrimaryIcon)
  ))
  const selectedPrimaryIconKey = selectedPrimaryIconLink ? technologyConnectorIconKey(selectedPrimaryIconLink) ?? '' : ''
  const inlineCustomTechnologyName = technologyQuery.trim() || (customTechnologyFile ? defaultTechnologyNameFromFile(customTechnologyFile) : '')
  const customTechnologyCanCreate = !!inlineCustomTechnologyName && !!customTechnologyFile && technologyLinks.length < 3 && !customTechnologySaving
  const normalizedTechnologyQuery = technologyQuery.trim()
  const fontAwesomeQueryMatch = matchFontAwesomeTechnologyIconQuery(normalizedTechnologyQuery)
  const fontAwesomeQueryIconUrl = fontAwesomeQueryMatch?.iconUrl ?? null
  const fontAwesomeQueryLabel = fontAwesomeQueryMatch?.label ?? ''
  const fontAwesomeQueryIconKey = fontAwesomeQueryMatch ? `fa:${fontAwesomeQueryMatch.iconName}` : ''
  const showFontAwesomeTechnologyCreate = (
    !!fontAwesomeQueryMatch &&
    technologyLinks.length < 3 &&
    !technologyLinks.some((link) => (
      technologyConnectorIconKey(link) === fontAwesomeQueryIconKey ||
      (link.type === 'custom' && normalizeTechnologyLabel(link.label) === normalizeTechnologyLabel(fontAwesomeQueryLabel))
    ))
  )
  const showCustomTechnologyCreate = (
    !!normalizedTechnologyQuery &&
    technologyLinks.length < 3 &&
    !fontAwesomeQueryIconUrl &&
    !technologySearchLoading &&
    technologySearchSettledQuery === normalizedTechnologyQuery &&
    technologyResults.length === 0
  )

  const commitTypeFromQuery = () => {
    if (isReadOnly) return
    const value = typeQuery.trim().toLowerCase()
    if (!value) return
    setType(value)
    setTypeQuery('')
    setTypeResults([])
  }

  const clearTypeAndFocus = () => {
    if (isReadOnly) return
    setType('')
    setTypeQuery('')
    setTypeResults([])
    requestAnimationFrame(() => typeInputRef.current?.focus())
  }

  const handleDelete = async () => {
    if (isReadOnly || !element) return
    try {
      if (viewId != null) {
        await api.workspace.views.placements.remove(viewId, element.id)
      } else if (orgId !== undefined) {
        await api.elements.delete(orgId, element.id)
      }
      onDelete?.(element.id)
      onClose()
    } catch { /* intentionally empty */ }
  }

  const handlePermanentDelete = async () => {
    if (isReadOnly || !element) return
    try {
      await api.elements.delete(orgId ?? '', element.id)
      onPermanentDelete?.(element.id)
      confirmPermanentDelete.onClose()
      onClose()
    } catch { /* intentionally empty */ }
  }

  const showNoiseGateControls = !!element && !!(onVisibilityOverrideDeltaChange || onPromoteVisibility || onDemoteVisibility || onResetVisibility)

  return (
    <>
      <SlidingPanel
        data-testid="element-panel"
        isOpen={isOpen}
        onClose={handleClose}
        panelKey="element"
        side={isMobile ? 'left' : 'right'}
        width="300px"
        hasBackdrop={hasBackdrop}
        autoFocus={true}
        noFocusLock={noFocusLock}
        isInline={isInline}
      >
        <PanelHeader title={isEdit ? 'Edit Element' : 'New Element'} onClose={handleClose} hasCloseButton={!isInline} isInline={isInline} actions={actions} />

        {/* Body */}
        <ScrollIndicatorWrapper px={4} py={4}>
          <VStack spacing={4} align="stretch">
            <FormControl isRequired isDisabled={isReadOnly}>
              <FormLabel>Name</FormLabel>
              <Input
                data-testid="element-panel-name-input"
                size="sm"
                value={name}
                onChange={(e) => setName(e.target.value)}
                onBlur={() => scheduleAutoSave()}
                placeholder="Payment Service"
              />
            </FormControl>
            <FormControl isDisabled={isReadOnly}>
              <FormLabel>Type</FormLabel>
              <VStack align="stretch" spacing={2}>
                <HStack align="flex-start">
                  <InputGroup>
                    <Input
                      data-testid="element-panel-type-input"
                      ref={typeInputRef}
                      size="sm"
                      value={typeQuery || type}
                      onFocus={() => {
                        if (isReadOnly) return
                        if (type && !typeQuery) setTypeQuery(type)
                      }}
                      onChange={(e) => setTypeQuery(e.target.value)}
                      onBlur={() => {
                        if (isReadOnly) return
                        // If the user is clicking a result, the mousedown handler will
                        // set suppression so we don't prematurely commit the typed query
                        // (which would happen before the click handler runs).
                        if (suppressTypeBlurRef.current) {
                          suppressTypeBlurRef.current = false
                          return
                        }
                        if (typeQuery.trim()) commitTypeFromQuery()
                        scheduleAutoSave()
                      }}
                      onKeyDown={(e) => {
                        if (e.key === 'Enter') {
                          e.preventDefault()
                          commitTypeFromQuery()
                        }
                      }}
                      placeholder="type to search or create"
                      isDisabled={isReadOnly}
                    />
                    {!!type && (
                      <InputRightElement h="full">
                        <CloseButton
                          size="sm"
                          onClick={(e) => {
                            e.preventDefault()
                            e.stopPropagation()
                            clearTypeAndFocus()
                          }}
                        />
                      </InputRightElement>
                    )}
                  </InputGroup>
                </HStack>

                {!isReadOnly && typeQuery.trim() && typeQuery.trim().toLowerCase() !== (type || '').trim().toLowerCase() && (
                  <Box border="1px solid" borderColor="whiteAlpha.200" rounded="md" bg="blackAlpha.300" maxH="140px" overflowY="auto">
                    <VStack spacing={0} align="stretch">
                      {typeResults.map((t) => (
                        <Box
                          data-testid="element-panel-type-option"
                          key={t}
                          px={2}
                          py={2}
                          cursor="pointer"
                          _hover={{ bg: 'whiteAlpha.100' }}
                          onMouseDown={() => { suppressTypeBlurRef.current = true }}
                          onClick={() => {
                            setType(t)
                            setTypeQuery('')
                            setTypeResults([])
                            // release suppression after handling click
                            setTimeout(() => { suppressTypeBlurRef.current = false }, 0)
                            scheduleAutoSave()
                          }}
                        >
                          <Text fontSize="sm" color="white" letterSpacing="0.05em">{t}</Text>
                        </Box>
                      ))}
                      {typeResults.length === 0 && (
                        <Box
                          px={2}
                          py={2}
                          cursor="pointer"
                          _hover={{ bg: 'whiteAlpha.100' }}
                          onMouseDown={() => { suppressTypeBlurRef.current = true }}
                          onClick={() => {
                            commitTypeFromQuery()
                            setTimeout(() => { suppressTypeBlurRef.current = false }, 0)
                            scheduleAutoSave()
                          }}
                        >
                          <Text fontSize="xs" color="gray.300">No match. Press Enter to set “{typeQuery.trim()}”.</Text>
                        </Box>
                      )}
                    </VStack>
                  </Box>
                )}
              </VStack>
            </FormControl>
            <FormControl isDisabled={isReadOnly}>
              <FormLabel>Description</FormLabel>
              <Textarea
                data-testid="element-panel-description-input"
                size="sm"
                value={description}
                onChange={(e) => setDescription(e.target.value)}
                onBlur={() => scheduleAutoSave()}
                placeholder="What does this element do?"
                rows={3}
              />
            </FormControl>
            <FormControl isDisabled={isReadOnly}>
              <FormLabel>Technology</FormLabel>
              <VStack align="stretch" spacing={2}>
                <HStack align="flex-start">
                  <Input
                    data-testid="element-panel-technology-input"
                    ref={techInputRef}
                    size="sm"
                    value={technologyQuery}
                    onChange={(e) => setTechnologyQuery(e.target.value)}
                    onKeyDown={(e) => {
                      if (e.key === 'ArrowDown') {
                        e.preventDefault()
                        setTechResultIndex((prev) => Math.min(prev + 1, technologyResults.length - 1))
                      } else if (e.key === 'ArrowUp') {
                        e.preventDefault()
                        setTechResultIndex((prev) => Math.max(prev - 1, -1))
                      } else if (e.key === 'Enter' || e.key === 'Tab') {
                        if (techResultIndex >= 0 && technologyResults[techResultIndex]) {
                          e.preventDefault()
                          addCatalogTechnology(technologyResults[techResultIndex])
                        } else if (e.key === 'Enter' && showFontAwesomeTechnologyCreate && fontAwesomeQueryMatch) {
                          e.preventDefault()
                          addCustomTechnology(fontAwesomeQueryLabel)
                        } else if (e.key === 'Enter' && technologyQuery.trim()) {
                          e.preventDefault()
                          addCustomTechnology()
                        }
                      } else if (e.key === 'Escape') {
                        e.preventDefault()
                        e.stopPropagation()
                        setTechnologyQuery('')
                        setTechResultIndex(-1)
                        resetCustomTechnologyForm()
                        techInputRef.current?.blur()
                      }
                    }}
                    placeholder="Regex or text (e.g. kafka|rabbitmq)"
                    isDisabled={isReadOnly || technologyLinks.length >= 3}
                  />
                  <Button
                    data-testid="element-panel-technology-add"
                    size="sm"
                    onClick={addCustomTechnology}
                    isDisabled={isReadOnly || technologyLinks.length >= 3 || !technologyQuery.trim()}
                  >
                    Add
                  </Button>
                </HStack>

                {!isReadOnly && technologyQuery.trim() && technologyLinks.length < 3 && (
                  <Box border="1px solid" borderColor="whiteAlpha.200" rounded="md" bg="blackAlpha.300" maxH="280px" overflowY="auto">
                    <VStack spacing={0} align="stretch">
                      {technologyResults.map((item, idx) => (
                        <Box
                          data-testid="element-panel-technology-option"
                          key={item.defaultSlug}
                          px={2}
                          py={2}
                          cursor="pointer"
                          bg={idx === techResultIndex ? 'whiteAlpha.200' : 'transparent'}
                          _hover={{ bg: 'whiteAlpha.100' }}
                          onClick={() => addCatalogTechnology(item)}
                        >
                          <HStack justify="space-between" align="center">
                            <HStack spacing={2} minW={0}>
                              <Box as="img" src={resolveWithBase(item.iconUrl)} alt={item.name} boxSize="18px" objectFit="contain" />
                              <Text fontSize="sm" color="white" noOfLines={1}>{item.name}</Text>
                            </HStack>
                            {item.provider && (
                              <Badge variant="subtle" colorScheme="blue" fontSize="8px">{item.provider}</Badge>
                            )}
                          </HStack>
                        </Box>
                      ))}
                      {technologySearchLoading && (
                        <Text px={2} py={2} fontSize="xs" color="gray.400">Searching...</Text>
                      )}
                      {showFontAwesomeTechnologyCreate && fontAwesomeQueryIconUrl && (
                        <Box
                          data-testid="element-panel-fontawesome-technology-option"
                          borderColor="whiteAlpha.100"
                          px={2}
                          py={2}
                          cursor="pointer"
                          _hover={{ bg: 'whiteAlpha.100' }}
                          onClick={() => addCustomTechnology(fontAwesomeQueryLabel)}
                        >
                          <HStack justify="space-between" align="center">
                            <HStack spacing={2} minW={0}>
                              <Flex w="18px" h="18px" align="center" justify="center" flexShrink={0}>
                                <Box as="img" src={fontAwesomeQueryIconUrl} alt="" boxSize="15px" objectFit="contain" />
                              </Flex>
                              <Text fontSize="sm" color="white" noOfLines={1}>{fontAwesomeQueryLabel}</Text>
                            </HStack>
                            <Badge variant="subtle" colorScheme="purple" fontSize="8px">Font Awesome</Badge>
                          </HStack>
                        </Box>
                      )}
                      {showCustomTechnologyCreate && (
                        <Box
                          data-testid="element-panel-custom-technology-create"
                          borderColor="whiteAlpha.100"
                          px={2}
                          py={2}
                          cursor={customTechnologyExpanded ? 'default' : 'pointer'}
                          _hover={customTechnologyExpanded ? undefined : { bg: 'whiteAlpha.100' }}
                          onClick={() => {
                            if (!customTechnologyExpanded) setCustomTechnologyExpanded(true)
                          }}
                        >
                          <VStack align="stretch" spacing={customTechnologyExpanded ? 3 : 0}>
                            <HStack justify="space-between" align="center" spacing={3} minH="24px">
                              <HStack spacing={2} minW={0}>
                                <Flex w="18px" h="18px" align="center" justify="center" color="gray.500" flexShrink={0}>
                                  <ImageUploadIcon size={14} />
                                </Flex>
                                <Text fontSize="sm" color="white" noOfLines={1}>
                                  Create custom technology
                                </Text>
                              </HStack>
                              <Text fontSize="xs" color="gray.500" flexShrink={0}>
                                Add icon
                              </Text>
                            </HStack>
                            {customTechnologyExpanded && (
                              <VStack align="stretch" spacing={3} pt={2}>
                                <Box
                                  data-testid="custom-technology-icon-dropzone"
                                  as="button"
                                  type="button"
                                  aria-label="Choose custom technology icon"
                                  disabled={customTechnologySaving}
                                  w="full"
                                  minH="72px"
                                  rounded="md"
                                  border="1px"
                                  borderStyle={customTechnologyPreviewUrl ? 'solid' : 'dashed'}
                                  borderColor={customTechnologyError ? 'red.300' : (customTechnologyPreviewUrl ? 'blue.300' : 'whiteAlpha.300')}
                                  bg={customTechnologyPreviewUrl ? 'whiteAlpha.100' : 'blackAlpha.200'}
                                  color="inherit"
                                  cursor={customTechnologySaving ? 'not-allowed' : 'pointer'}
                                  px={3}
                                  py={2}
                                  textAlign="left"
                                  transition="border-color 120ms ease, background 120ms ease, box-shadow 120ms ease"
                                  _hover={customTechnologySaving ? undefined : { borderColor: 'blue.300', bg: 'whiteAlpha.100' }}
                                  _focusVisible={{ outline: 'none', boxShadow: '0 0 0 2px var(--accent)' }}
                                  _disabled={{ opacity: 0.55 }}
                                  onClick={(event: React.MouseEvent<HTMLButtonElement>) => {
                                    event.stopPropagation()
                                    openCustomTechnologyFilePicker()
                                  }}
                                  onDragOver={(event: React.DragEvent<HTMLButtonElement>) => event.preventDefault()}
                                  onDrop={handleCustomTechnologyFileDrop}
                                >
                                  <HStack spacing={3} align="center">
                                    <Flex
                                      w="38px"
                                      h="38px"
                                      align="center"
                                      justify="center"
                                      rounded="md"
                                      bg="blackAlpha.300"
                                      border="1px solid"
                                      borderColor="whiteAlpha.100"
                                      flexShrink={0}
                                    >
                                      {customTechnologyPreviewUrl ? (
                                        <Box
                                          data-testid="custom-technology-preview-icon"
                                          as="img"
                                          src={customTechnologyPreviewUrl}
                                          alt=""
                                          maxW="28px"
                                          maxH="28px"
                                          objectFit="contain"
                                          opacity={0.95}
                                        />
                                      ) : (
                                        <Box color="gray.500">
                                          <ImageUploadIcon size={16} />
                                        </Box>
                                      )}
                                    </Flex>
                                    <Box minW={0}>
                                      <Text fontSize="sm" color="gray.100" noOfLines={1}>
                                        {customTechnologyFile ? customTechnologyFile.name : 'Upload icon'}
                                      </Text>
                                      <Text fontSize="xs" color="gray.500" lineHeight="1.35">
                                        {customTechnologyFile ? 'Click to replace, or drop another file.' : 'Drop or choose SVG/PNG, max 2 MB.'}
                                      </Text>
                                    </Box>
                                  </HStack>
                                </Box>
                                <Input
                                  data-testid="custom-technology-file"
                                  ref={customTechnologyFileInputRef}
                                  type="file"
                                  display="none"
                                  accept=".svg,.png,image/svg+xml,image/png"
                                  onChange={handleCustomTechnologyFileChange}
                                />
                                {customTechnologyFile && (
                                  <HStack justify="space-between" spacing={2}>
                                    <Text fontSize="xs" color="gray.400" noOfLines={1}>
                                      Name: {inlineCustomTechnologyName}
                                    </Text>
                                    <Button
                                      size="xs"
                                      variant="ghost"
                                      color="gray.400"
                                      onClick={(event) => {
                                        event.stopPropagation()
                                        clearCustomTechnologyFile()
                                      }}
                                      isDisabled={customTechnologySaving}
                                    >
                                      Clear
                                    </Button>
                                  </HStack>
                                )}
                                <Box
                                  as="button"
                                  type="button"
                                  data-testid="custom-technology-options-toggle"
                                  display="flex"
                                  alignItems="center"
                                  justifyContent="center"
                                  gap="6px"
                                  w="full"
                                  py={1.5}
                                  rounded="md"
                                  bg="transparent"
                                  color="gray.400"
                                  fontSize="xs"
                                  fontWeight="medium"
                                  letterSpacing="0.02em"
                                  cursor="pointer"
                                  transition="color 150ms ease, background 150ms ease"
                                  _hover={{ color: 'gray.200', bg: 'whiteAlpha.50' }}
                                  onClick={(event: React.MouseEvent) => {
                                    event.stopPropagation()
                                    setCustomTechnologyOptionsOpen((value) => !value)
                                  }}
                                >
                                  <Box
                                    flex={1}
                                    h="1px"
                                    bg="whiteAlpha.100"
                                  />
                                  <HStack spacing="4px" flexShrink={0}>
                                    <Text fontSize="xs" lineHeight="1">
                                      {customTechnologyOptionsOpen ? 'Hide optional fields' : 'Show optional fields'}
                                    </Text>
                                    <Box
                                      as="span"
                                      display="inline-flex"
                                      transition="transform 200ms ease"
                                      transform={customTechnologyOptionsOpen ? 'rotate(180deg)' : 'rotate(0deg)'}
                                    >
                                      <ChevronDownIcon size={10} strokeWidth={2.5} />
                                    </Box>
                                  </HStack>
                                  <Box
                                    flex={1}
                                    h="1px"
                                    bg="whiteAlpha.100"
                                  />
                                </Box>
                                {customTechnologyOptionsOpen && (
                                  <VStack
                                    align="stretch"
                                    spacing={3}
                                    p={3}
                                    rounded="md"
                                    bg="whiteAlpha.50"
                                    border="1px solid"
                                    borderColor="whiteAlpha.100"
                                  >
                                    <Box>
                                      <Text fontSize="xs" color="gray.400" mb={1} fontWeight="medium">Short name</Text>
                                      <Input
                                        data-testid="custom-technology-short-name"
                                        size="sm"
                                        value={customTechnologyShortName}
                                        onChange={(event) => setCustomTechnologyShortName(event.target.value)}
                                        placeholder="e.g. K8s"
                                        isDisabled={customTechnologySaving}
                                      />
                                    </Box>
                                    <Box>
                                      <Text fontSize="xs" color="gray.400" mb={1} fontWeight="medium">Aliases</Text>
                                      <Input
                                        data-testid="custom-technology-aliases"
                                        size="sm"
                                        value={customTechnologyAliases}
                                        onChange={(event) => setCustomTechnologyAliases(event.target.value)}
                                        placeholder="Comma separated, e.g. k8s, kube"
                                        isDisabled={customTechnologySaving}
                                      />
                                    </Box>
                                  </VStack>
                                )}
                                <Button
                                  data-testid="custom-technology-save"
                                  size="sm"
                                  colorScheme="blue"
                                  isLoading={customTechnologySaving}
                                  isDisabled={!customTechnologyCanCreate}
                                  w="full"
                                  onClick={(event) => {
                                    event.stopPropagation()
                                    void handleCreateCustomTechnology()
                                  }}
                                >
                                  Create and attach
                                </Button>
                                {customTechnologyError && (
                                  <Text data-testid="custom-technology-error" fontSize="xs" color="red.300">
                                    {customTechnologyError}
                                  </Text>
                                )}
                              </VStack>
                            )}
                          </VStack>
                        </Box>
                      )}
                    </VStack>
                  </Box>
                )}

                <Wrap>
                  {technologyLinks.map((link) => {
                    const meta = link.slug ? technologyMeta[link.slug] : undefined
                    const sourceUrl = meta?.websiteUrl || meta?.docsUrl
                    const iconKey = technologyConnectorIconKey(link)
                    const iconUrl = resolveTechnologyConnectorIconUrl(link, meta?.iconUrl)
                    const isSelectable = !!iconKey && !isReadOnly
                    const isPrimaryIcon = !!iconKey && !!(link.is_primary_icon ?? link.isPrimaryIcon)
                    return (
                      <WrapItem key={`${link.type}:${link.slug ?? link.label}`}>
                        <Popover trigger={isMobile ? 'click' : 'hover'} placement="top" closeOnBlur>
                          <PopoverTrigger>
                            <Tag
                              data-testid="element-panel-technology-chip"
                              size="sm"
                              variant="subtle"
                              bg={isPrimaryIcon ? 'blue.500' : 'whiteAlpha.100'}
                              border="1px solid"
                              borderColor={isPrimaryIcon ? 'blue.300' : 'whiteAlpha.200'}
                              color={isPrimaryIcon ? 'white' : undefined}
                              cursor={isSelectable ? 'pointer' : 'default'}
                              onClick={() => {
                                if (isSelectable && iconKey) togglePrimaryIcon(iconKey)
                              }}
                            >
                              <TagLabel color="white">
                                {iconUrl && (
                                  <Box as="img" src={resolveWithBase(iconUrl)} alt={link.label} boxSize="12px" objectFit="contain" display="inline-block" mr={1.5} verticalAlign="middle" />
                                )}
                                {link.label}
                              </TagLabel>
                              {!isReadOnly && (
                                <TagCloseButton
                                  data-testid="element-panel-technology-remove"
                                  onClick={(e) => {
                                    e.preventDefault()
                                    e.stopPropagation()
                                    removeTechnology(link)
                                  }}
                                />
                              )}
                            </Tag>
                          </PopoverTrigger>
                          <PopoverContent bg="var(--bg-panel)" borderColor="whiteAlpha.300" maxW="260px">
                            <PopoverArrow bg="var(--bg-panel)" />
                            <PopoverBody>
                              <VStack align="stretch" spacing={1}>
                                <Text fontSize="sm" color="white" fontWeight="semibold">{meta?.name || link.label}</Text>
                                <Text fontSize="xs" color="gray.400">{iconKey?.startsWith('fa:') ? 'Font Awesome icon' : (link.type === 'custom' ? 'Custom technology' : (meta?.provider || 'General'))}</Text>
                                {sourceUrl && (
                                  <Text as="button" type="button" onClick={() => openExternalUrl(sourceUrl)} fontSize="xs" color="blue.300" textDecoration="underline" pointerEvents="auto" textAlign="left">
                                    {sourceUrl}
                                  </Text>
                                )}
                              </VStack>
                            </PopoverBody>
                          </PopoverContent>
                        </Popover>
                      </WrapItem>
                    )
                  })}
                </Wrap>
              </VStack>
            </FormControl>
            <FormControl isDisabled={isReadOnly}>
              <FormLabel>URL</FormLabel>
              <Input
                data-testid="element-panel-url-input"
                size="sm"
                value={url}
                onChange={(e) => setUrl(e.target.value)}
                onBlur={() => scheduleAutoSave()}
                placeholder="https://…"
              />
            </FormControl>
            <FormControl isDisabled={isReadOnly}>
              <FormLabel>Tags</FormLabel>
              <TagUpsert
                currentTags={tags}
                availableTags={availableTags}
                onAddTag={(tag) => {
                  if (!tags.includes(tag)) {
                    const nextTags = [...tags, tag]
                    setTags(nextTags)
                    scheduleAutoSave({ tags: nextTags })
                  }
                }}
                isReadOnly={isReadOnly}
              />
              <Wrap mt={3}>
                {tags.map((tag) => (
                  <WrapItem key={tag}>
                    <Tag data-testid="element-panel-tag-chip" size="sm" variant="subtle" bg="whiteAlpha.100" border="1px solid" borderColor="whiteAlpha.200">
                      <TagLabel color="white">{tag}</TagLabel>
                      {!isReadOnly && (
                        <TagCloseButton data-testid="element-panel-tag-remove" onClick={() => {
                          const nextTags = tags.filter((t) => t !== tag)
                          setTags(nextTags)
                          scheduleAutoSave({ tags: nextTags })
                        }} />
                      )}
                    </Tag>
                  </WrapItem>
                ))}
              </Wrap>
            </FormControl>
            {showNoiseGateControls || (isEdit && canEdit && onMerge) ? (
              <Box >
                {showNoiseGateControls && (
                  <>
                    <HStack justify="space-between" align="center" mb={3}>
                      <Box>
                        <FormLabel fontSize="sm" fontFamily="var(--chakra-fonts-heading)" mb={0.5}>
                          Noise Gate
                        </FormLabel>
                      </Box>
                      <Switch
                        data-testid="element-panel-bypass-noise-gate"
                        aria-label="Noise gate"
                        isChecked={!bypassNoiseGate}
                        isDisabled={isReadOnly}
                        colorScheme="blue"
                        onChange={(event) => {
                          setBypassNoiseGate(!event.target.checked)
                          scheduleAutoSave()
                        }}
                      />
                    </HStack>
                    {!bypassNoiseGate && (
                      <>
                        <HStack justify="space-between" align="flex-start" mb={2.5}>
                          <Box>
                            <Text fontSize="xs" color="gray.500">Choose when this element starts appearing when filtering is enabled.</Text>
                          </Box>
                        </HStack>
                        <Box px={1} pt={1} pb={0.5} mb={isEdit && canEdit && onMerge ? 3 : 0}>
                          <Slider
                            aria-label="Element noise gate"
                            min={-2}
                            max={2}
                            step={1}
                            value={draftNoiseGateLevel}
                            onChange={setDraftNoiseGateLevel}
                            onChangeEnd={(value) => {
                              setDraftNoiseGateLevel(value)
                              void handleNoiseGateChange(value)
                            }}
                            focusThumbOnChange={false}
                            isDisabled={isReadOnly}
                          >
                            <SliderTrack h="4px" bg="whiteAlpha.200">
                              <SliderFilledTrack bg="var(--accent)" />
                            </SliderTrack>
                            {NOISE_GATE_STOPS.map((stop) => (
                              <Box
                                key={stop.value}
                                position="absolute"
                                left={`${((stop.value + 2) / 4) * 100}%`}
                                top="50%"
                                transform="translate(-50%, -50%)"
                                w={stop.value === draftNoiseGateLevel ? '6px' : '2px'}
                                h={stop.value === draftNoiseGateLevel ? '6px' : '10px'}
                                rounded="full"
                                bg={draftNoiseGateLevel >= stop.value ? 'var(--accent)' : 'whiteAlpha.500'}
                                pointerEvents="none"
                              />
                            ))}
                            <SliderThumb boxSize="14px" bg="white" border="2px solid" borderColor="var(--accent)" />
                          </Slider>
                          <HStack justify="space-between" mt={2} px={0.5}>
                            {NOISE_GATE_STOPS.map((stop) => (
                              <Text
                                key={stop.value}
                                fontSize="9px"
                                fontWeight={stop.value === draftNoiseGateLevel ? 'bold' : 'medium'}
                                color={stop.value === draftNoiseGateLevel ? 'whiteAlpha.900' : 'whiteAlpha.500'}
                              >
                                {stop.label}
                              </Text>
                            ))}
                          </HStack>
                        </Box>
                      </>
                    )}
                  </>
                )}

              </Box>
            ) : null}

            {isEdit && element && (
              <GitSourceLinker
                element={element}
                isReadOnly={isReadOnly}
                onUpdate={(updates) => {
                  Object.assign(element, updates)
                  // Trigger a save with new updates by rebuilding payload in saveIfDirty
                  if (!isReadOnly) {
                    scheduleAutoSave()
                  }
                }}
              />
            )}

            {isEdit && (links.length > 0 || parentLinks.length > 0) && (
              <Box>
                <FormLabel fontSize="sm" fontWeight="bold" color="gray.400" mb={2}>Drill Down</FormLabel>
                <VStack align="stretch" spacing={2}>
                  {parentLinks.map((link: ViewConnector) => (
                    <HStack
                      key={link.id}
                      as="button"
                      w="full"
                      px={2}
                      py={1.5}
                      rounded="md"
                      bg="whiteAlpha.50"
                      _hover={{ bg: 'whiteAlpha.100' }}
                      onClick={() => {
                        navigate(`/views/${link.from_view_id}`)
                        onClose()
                      }}
                      align="center"
                    >
                      <Box color="blue.400" flexShrink={0}>
                        <ZoomOutIcon size={12} />
                      </Box>
                      <HStack align="baseline" spacing={2} flex={1} overflow="hidden">
                        <Text fontSize="xs" color="gray.400" whiteSpace="nowrap">Parent View</Text>
                        <Text fontSize="sm" color="white" isTruncated>{link.to_view_name}</Text>
                      </HStack>
                    </HStack>
                  ))}

                  {links.map((link: ViewConnector) => (
                    <HStack
                      key={link.id}
                      as="button"
                      w="full"
                      px={2}
                      py={1.5}
                      rounded="md"
                      bg="whiteAlpha.50"
                      _hover={{ bg: 'whiteAlpha.100' }}
                      onClick={() => {
                        navigate(`/views/${link.to_view_id}`)
                        onClose()
                      }}
                      align="center"
                    >
                      <Box color="teal.400" flexShrink={0}>
                        <ZoomInIcon size={12} />
                      </Box>
                      <HStack align="baseline" spacing={2} flex={1} overflow="hidden">
                        <Text fontSize="xs" color="gray.400" whiteSpace="nowrap">Sub-view</Text>
                        <Text fontSize="sm" color="white" isTruncated>{link.to_view_name}</Text>
                      </HStack>
                    </HStack>
                  ))}
                </VStack>
              </Box>
            )}

            {elementPanelAfterContentSlot}


            <Box>
              {isEdit && canEdit && onMerge && (
                <Button
                  data-testid="element-panel-merge"
                  variant="outline"
                  size="sm"
                  borderColor="teal.700"
                  color="teal.300"
                  _hover={{ bg: 'teal.900', borderColor: 'teal.500', color: 'teal.100' }}
                  onClick={() => onMerge(element.id)}
                  w="full"
                >
                  Merge
                </Button>
              )}

            </Box>
            {isEdit && canEdit && (
              <HStack borderTop="1px solid" borderColor="whiteAlpha.100" pt={4} pb={1} spacing={2}>
                {viewId != null && (
                  <Button
                    data-testid="element-panel-remove"
                    variant="ghost"
                    size="sm"
                    color="gray.400"
                    _hover={{ bg: 'whiteAlpha.100', color: 'white' }}
                    onClick={handleDelete}
                    flex={1}
                  >
                    Remove
                  </Button>
                )}
                <Button
                  data-testid="element-panel-delete-permanent"
                  variant="ghost"
                  size="sm"
                  color="red.400"
                  _hover={{ bg: 'red.900', color: 'red.200' }}
                  onClick={confirmPermanentDelete.onOpen}
                  flex={1}
                >
                  Delete Element
                </Button>
              </HStack>
            )}
          </VStack>
        </ScrollIndicatorWrapper>

        {/* Footer */}
        {!autoSaveEdit && !isInline && (
          <HStack px={4} py={3} justify="flex-end" flexShrink={0}>
            <Button variant="ghost" size="sm" onClick={handleClose}>
              Cancel
            </Button>
            {canEdit && (
              <Button size="sm" px={5} colorScheme="blue" onClick={handleSave} isLoading={loading}>
                Save
              </Button>
            )}
          </HStack>
        )}
      </SlidingPanel >

      <ConfirmDialog
        isOpen={confirmPermanentDelete.isOpen}
        onClose={confirmPermanentDelete.onClose}
        onConfirm={handlePermanentDelete}
        title="Delete Element"
        body="This permanently deletes the element from the library and cannot be reverted."
        confirmLabel="Delete Permanently"
      />
    </>
  )
}

export default memo(ElementPanel)
