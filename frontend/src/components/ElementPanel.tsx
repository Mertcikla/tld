import { memo, useEffect, useRef, useState, useCallback } from 'react'
import type { ElementPanelSlots } from '../slots'
import { useNavigate } from 'react-router-dom'
import {
  Badge,
  Box,
  Button,
  CloseButton,
  Divider,
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
  Tag,
  TagCloseButton,
  TagLabel,
  Text,
  Textarea,
  useBreakpointValue,
  useDisclosure,
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
import { getTechnologyCatalogIndex, getTechnologyCatalogItemBySlug, resolveWithBase, searchTechnologyCatalog } from '../utils/technologyCatalog'
import { ZoomInIcon, ZoomOutIcon } from './Icons'
import ScrollIndicatorWrapper from './ScrollIndicatorWrapper'
import TagUpsert from './TagUpsert'

import { useViewEditorContext } from '../pages/ViewEditor/context'

function normalizeTechnologyLabel(value: string): string {
  return value.trim().replace(/\s+/g, ' ').toLowerCase()
}

function splitTechnologyLabel(value: string): string[] {
  return value.split(',').map((part) => part.trim()).filter(Boolean)
}

function findCatalogItemByLabel(index: Awaited<ReturnType<typeof getTechnologyCatalogIndex>>, label: string): TechnologyCatalogItem | null {
  const normalized = normalizeTechnologyLabel(label)
  if (!normalized) return null

  const bySlugMatch = index.bySlug.get(label.trim())
  if (bySlugMatch) return bySlugMatch

  return index.items.find((item) => (
    normalizeTechnologyLabel(item.name) === normalized ||
    normalizeTechnologyLabel(item.nameShort) === normalized ||
    normalizeTechnologyLabel(item.defaultSlug) === normalized
  )) ?? null
}

function dedupeTechnologyLinks(links: TechnologyConnector[]): TechnologyConnector[] {
  const seenCatalog = new Set<string>()
  const seenCustom = new Set<string>()
  const result: TechnologyConnector[] = []
  let primarySet = false

  for (const link of links) {
    const label = link.label.trim()
    if (!label) continue

    if (link.type === 'catalog' && link.slug) {
      const slug = link.slug.trim()
      const key = slug.toLowerCase()
      if (seenCatalog.has(key)) continue
      seenCatalog.add(key)
      result.push({
        type: 'catalog',
        slug,
        label,
        is_primary_icon: !primarySet && !!link.is_primary_icon,
      })
      if (link.is_primary_icon) primarySet = true
      continue
    }

    const key = normalizeTechnologyLabel(label)
    if (seenCustom.has(key)) continue
    seenCustom.add(key)
    result.push({ type: 'custom', label, is_primary_icon: false })
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
      normalized.push({ type: 'custom', label: label.trim(), is_primary_icon: false })
    }
  }

  for (const link of rawLinks.length > 0 ? rawLinks : legacyLabels.map((label) => ({ type: 'custom' as const, label }))) {
    if (link.type === 'catalog') {
      const match = link.slug ? index.bySlug.get(link.slug) : null
      normalized.push({
        type: 'catalog',
        slug: link.slug,
        label: match?.name ?? link.label,
        is_primary_icon: !!link.is_primary_icon,
      })
      continue
    }

    const parts = splitTechnologyLabel(link.label)
    if (parts.length > 1) {
      for (const part of parts) pushLabel(part)
    } else {
      pushLabel(link.label)
    }
  }

  for (const label of legacyLabels) {
    pushLabel(label)
  }

  return dedupeTechnologyLinks(normalized)
}

function buildTechnologyFingerprintPayload(
  element: LibraryElement,
  links: TechnologyConnector[],
  type: string,
) {
  const normalizedLinks = links.map((link) => ({
    type: link.type,
    slug: link.type === 'catalog' ? link.slug : undefined,
    label: link.label,
    is_primary_icon: !!link.is_primary_icon,
  }))
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
    repo: element.repo,
    branch: element.branch,
    file_path: element.file_path,
    language: element.language,
  }
}

export interface ElementPanelProps extends ElementPanelSlots {
  isOpen: boolean
  onClose: () => void
  element?: LibraryElement | null
  onSave: (obj: LibraryElement) => void
  autoSave?: boolean
  onDelete?: (id: number) => void
  onPermanentDelete?: (id: number) => void
  orgId?: string
  links?: ViewConnector[]
  parentLinks?: ViewConnector[]
  hasBackdrop?: boolean
  availableTags?: string[]
}

/**
 * Name: Edit Element Panel
 * Role: Opens when clicked on an element and displays its fields, allowing for editing.
 * Location: Right side of the screen on desktop. Overlays screen on mobile.
 * Aliases: Element Properties, Element Details.
 */
function ElementPanel({ isOpen, onClose, element, onSave, autoSave = false, onDelete, onPermanentDelete, orgId, links = [], parentLinks = [], hasBackdrop = true, availableTags = [], elementPanelAfterContentSlot }: ElementPanelProps) {
  const { canEdit, viewId } = useViewEditorContext()
  const isEdit = !!element
  const isReadOnly = !canEdit
  const autoSaveEdit = autoSave && isEdit && !isReadOnly
  const navigate = useNavigate()
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
  const [tags, setTags] = useState<string[]>([])
  const [loading, setLoading] = useState(false)
  const [explicitLogoClear, setExplicitLogoClear] = useState(false)
  const typeInputRef = useRef<HTMLInputElement>(null)
  const techInputRef = useRef<HTMLInputElement>(null)
  const suppressTypeBlurRef = useRef(false)
  const lastSavedFingerprintRef = useRef<string>('')
  const savingRef = useRef(false)
  const pendingSaveRef = useRef(false)
  const [techResultIndex, setTechResultIndex] = useState(-1)
  const confirmPermanentDelete = useDisclosure()
  const isMobile = useBreakpointValue({ base: true, md: false }) ?? false

  useEffect(() => {
    setTechResultIndex(-1)
  }, [technologyQuery])

  useEffect(() => {
    let cancelled = false

    if (element) {
      setName(element.name)
      setDescription(element.description ?? '')
      setType(element.kind ?? '')
      setTypeQuery('')
      setTypeResults([])
      setUrl(element.url ?? '')
      setTags(element.tags ?? [])
      setExplicitLogoClear(false)

      const linksFromElement = element.technology_connectors ?? []
      const fallbackLinks: TechnologyConnector[] = linksFromElement.length > 0
        ? linksFromElement
        : (element.technology ? [{ type: 'custom', label: element.technology }] : [])
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
      setExplicitLogoClear(false)
      lastSavedFingerprintRef.current = ''
    }

    return () => {
      cancelled = true
    }
  }, [element, isOpen])

  const buildPayloadAndFingerprint = useCallback(async () => {
    const primaryLink = technologyLinks.find((link) => link.type === 'catalog' && link.is_primary_icon && link.slug)
    const primarySlug = primaryLink?.slug

    const normalizedLinks = technologyLinks.map((link) => ({
      type: link.type,
      slug: link.type === 'catalog' ? link.slug : undefined,
      label: link.label,
      is_primary_icon: !!link.is_primary_icon,
    }))

    const normalizedType = type.trim().toLowerCase()

    let logoUrl = element?.logo_url ?? ''
    if (explicitLogoClear) {
      logoUrl = ''
    }
    if (!explicitLogoClear && primarySlug) {
      const cached = technologyMeta[primarySlug]
      if (cached?.iconUrl) {
        logoUrl = cached.iconUrl
      } else {
        try {
          const item = await getTechnologyCatalogItemBySlug(primarySlug)
          if (item) {
            setTechnologyMeta((prev) => ({ ...prev, [primarySlug]: item }))
            if (item.iconUrl) logoUrl = item.iconUrl
          }
        } catch {
          // ignore
        }
      }
    }

    const payload = {
      name,
      description,
      kind: normalizedType,
      technology: technologyLinks.map((link) => link.label).join(', '),
      url,
      logo_url: logoUrl,
      technology_connectors: normalizedLinks,
      tags,
      repo: element?.repo,
      branch: element?.branch,
      file_path: element?.file_path,
      language: element?.language,
    }
    return { payload, fingerprint: JSON.stringify(payload) }
  }, [technologyLinks, technologyMeta, explicitLogoClear, type, element, name, description, url, tags])

  const saveIfDirty = useCallback(async () => {
    if (!autoSaveEdit || !element) return
    if (!name.trim()) return

    if (savingRef.current) {
      pendingSaveRef.current = true
      return
    }

    savingRef.current = true
    try {
      const { payload, fingerprint } = await buildPayloadAndFingerprint()
      if (fingerprint === lastSavedFingerprintRef.current) return
      const saved = await api.elements.update(element.id, payload)
      lastSavedFingerprintRef.current = fingerprint
      onSave(saved)
    } catch {
      // ignore
    } finally {
      savingRef.current = false
      if (pendingSaveRef.current) {
        pendingSaveRef.current = false
        void saveIfDirty()
      }
    }
  }, [autoSaveEdit, element, name, buildPayloadAndFingerprint, onSave])

  const saveIfDirtyRef = useRef<(() => Promise<void>) | null>(null)
  useEffect(() => { saveIfDirtyRef.current = saveIfDirty }, [saveIfDirty])

  const scheduleAutoSave = () => {
    if (!autoSaveEdit) return
    requestAnimationFrame(() => {
      void saveIfDirtyRef.current?.()
    })
  }

  const handleClose = useCallback(() => {
    if (autoSaveEdit) {
      void saveIfDirtyRef.current?.()
    }
    onClose()
  }, [autoSaveEdit, onClose])

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
          const item = index.bySlug.get(slug)
          if (item) next[slug] = item
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
      return
    }

    const timer = setTimeout(() => {
      setTechnologySearchLoading(true)
      searchTechnologyCatalog(query)
        .then((results) => {
          setTechnologyResults(results)
          setTechnologyMeta((prev) => {
            const next = { ...prev }
            for (const item of results) {
              next[item.defaultSlug] = item
            }
            return next
          })
        })
        .catch(() => setTechnologyResults([]))
        .finally(() => setTechnologySearchLoading(false))
    }, 140)

    return () => clearTimeout(timer)
  }, [isOpen, technologyQuery])

  useEffect(() => {
    if (!isOpen) return
    const handler = (e: KeyboardEvent) => {
      const target = e.target as HTMLElement
      const isInput = target?.tagName === 'INPUT' || target?.tagName === 'TEXTAREA' || target.isContentEditable

      if (e.key === 'Escape' && !isInput) handleClose()

      if (e.key.toLowerCase() === 't' && !isInput && !e.ctrlKey && !e.metaKey && !e.altKey) {
        e.preventDefault()
        techInputRef.current?.focus()
      }
    }
    window.addEventListener('keydown', handler)
    return () => window.removeEventListener('keydown', handler)
  }, [isOpen, handleClose])

  const addCatalogTechnology = (item: TechnologyCatalogItem) => {
    if (technologyLinks.length >= 3) return
    if (technologyLinks.some((link) => link.type === 'catalog' && link.slug === item.defaultSlug)) return

    const hasPrimaryCatalog = technologyLinks.some((link) => link.type === 'catalog' && !!link.is_primary_icon)

    setTechnologyConnectors((prev) => ([
      ...prev,
      {
        type: 'catalog',
        slug: item.defaultSlug,
        label: item.name,
        is_primary_icon: !hasPrimaryCatalog,
      },
    ]))
    setTechnologyQuery('')
    setTechnologyResults([])
    setTechnologyMeta((prev) => ({ ...prev, [item.defaultSlug]: item }))
    setExplicitLogoClear(false)
    scheduleAutoSave()
  }

  const addCustomTechnology = () => {
    const value = technologyQuery.trim()
    if (!value || technologyLinks.length >= 3) return
    if (technologyLinks.some((link) => link.type === 'custom' && link.label.toLowerCase() === value.toLowerCase())) return

    setTechnologyConnectors((prev) => ([...prev, { type: 'custom', label: value }]))
    setTechnologyQuery('')
    setTechnologyResults([])
    scheduleAutoSave()
  }

  const removeTechnology = (linkToRemove: TechnologyConnector) => {
    if (linkToRemove.type === 'catalog' && linkToRemove.is_primary_icon) {
      setExplicitLogoClear(true)
    }
    setTechnologyConnectors((prev) => prev.filter((link) => (
      !(link.type === linkToRemove.type && link.slug === linkToRemove.slug && link.label === linkToRemove.label)
    )))
    scheduleAutoSave()
  }

  const togglePrimaryIcon = (selectedSlug: string) => {
    const isDeselecting = selectedPrimarySlug === selectedSlug
    setTechnologyConnectors((prev) => prev.map((link) => {
      if (link.type !== 'catalog') {
        return { ...link, is_primary_icon: false }
      }
      return {
        ...link,
        is_primary_icon: !isDeselecting && link.slug === selectedSlug,
      }
    }))
    setExplicitLogoClear(isDeselecting)
    scheduleAutoSave()
  }

  const selectedPrimarySlug = technologyLinks.find((link) => link.type === 'catalog' && !!link.is_primary_icon && !!link.slug)?.slug ?? ''

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

  const handleSave = async () => {
    if (isReadOnly || !name.trim()) return
    setLoading(true)
    try {
      const primaryLink = technologyLinks.find((link) => link.type === 'catalog' && link.is_primary_icon && link.slug)
      const primaryMetadata = primaryLink?.slug
        ? (technologyMeta[primaryLink.slug] ?? await getTechnologyCatalogItemBySlug(primaryLink.slug))
        : null

      const normalizedLinks = technologyLinks.map((link) => ({
        type: link.type,
        slug: link.type === 'catalog' ? link.slug : undefined,
        label: link.label,
        is_primary_icon: !!link.is_primary_icon,
      }))

      const normalizedType = type.trim().toLowerCase()

      const payload = {
        name,
        description,
        kind: normalizedType,
        technology: technologyLinks.map((link) => link.label).join(', '),
        url,
        logo_url: explicitLogoClear ? '' : (primaryMetadata?.iconUrl ?? ''),
        technology_connectors: normalizedLinks,
        tags,
        repo: element?.repo,
        branch: element?.branch,
        file_path: element?.file_path,
        language: element?.language,
      }
      const saved = isEdit
        ? await api.elements.update(element!.id, payload)
        : await api.elements.create(payload)
      onSave(saved)
      onClose()
    } catch { /* intentionally empty */ } finally {
      setLoading(false)
    }
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

  return (
    <>
      <SlidingPanel isOpen={isOpen} onClose={handleClose} panelKey="element" side={isMobile ? 'left' : 'right'} width="300px" hasBackdrop={hasBackdrop}>
        <PanelHeader title={isEdit ? 'Edit Element' : 'New Element'} onClose={handleClose} />

        {/* Body */}
        <ScrollIndicatorWrapper px={4} py={4}>
          <VStack spacing={4} align="stretch">
            <FormControl isRequired isDisabled={isReadOnly}>
              <FormLabel>Name</FormLabel>
              <Input
                size="sm"
                value={name}
                onChange={(e) => setName(e.target.value)}
                onBlur={scheduleAutoSave}
                placeholder="Payment Service"
              />
            </FormControl>
            <FormControl isDisabled={isReadOnly}>
              <FormLabel>Type</FormLabel>
              <VStack align="stretch" spacing={2}>
                <HStack align="flex-start">
                  <InputGroup>
                    <Input
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
                size="sm"
                value={description}
                onChange={(e) => setDescription(e.target.value)}
                onBlur={scheduleAutoSave}
                placeholder="What does this element do?"
                rows={3}
              />
            </FormControl>
            <FormControl isDisabled={isReadOnly}>
              <FormLabel>Technology</FormLabel>
              <VStack align="stretch" spacing={2}>
                <HStack align="flex-start">
                  <Input
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
                        } else if (e.key === 'Enter' && technologyQuery.trim()) {
                          e.preventDefault()
                          addCustomTechnology()
                        }
                      } else if (e.key === 'Escape') {
                        e.preventDefault()
                        e.stopPropagation()
                        setTechnologyQuery('')
                        setTechResultIndex(-1)
                        techInputRef.current?.blur()
                      }
                    }}
                    placeholder="Regex or text (e.g. kafka|rabbitmq)"
                    isDisabled={isReadOnly || technologyLinks.length >= 3}
                  />
                  <Button
                    size="sm"
                    onClick={addCustomTechnology}
                    isDisabled={isReadOnly || technologyLinks.length >= 3 || !technologyQuery.trim()}
                  >
                    Add
                  </Button>
                </HStack>

                {!isReadOnly && technologyQuery.trim() && technologyLinks.length < 3 && (
                  <Box border="1px solid" borderColor="whiteAlpha.200" rounded="md" bg="blackAlpha.300" maxH="190px" overflowY="auto">
                    <VStack spacing={0} align="stretch">
                      {technologyResults.map((item, idx) => (
                        <Box
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
                      {!technologySearchLoading && technologyResults.length === 0 && (
                        <Text px={2} py={2} fontSize="xs" color="gray.400">No match in catalog. Use Add Custom.</Text>
                      )}
                    </VStack>
                  </Box>
                )}

                <Wrap>
                  {technologyLinks.map((link) => {
                    const meta = link.slug ? technologyMeta[link.slug] : undefined
                    const sourceUrl = meta?.websiteUrl || meta?.docsUrl
                    const isSelectable = link.type === 'catalog' && !!link.slug && !isReadOnly
                    const isPrimaryIcon = link.type === 'catalog' && !!link.is_primary_icon && !!link.slug
                    return (
                      <WrapItem key={`${link.type}:${link.slug ?? link.label}`}>
                        <Popover trigger={isMobile ? 'click' : 'hover'} placement="top" closeOnBlur>
                          <PopoverTrigger>
                            <Tag
                              size="sm"
                              variant="subtle"
                              bg={isPrimaryIcon ? 'blue.500' : 'whiteAlpha.100'}
                              border="1px solid"
                              borderColor={isPrimaryIcon ? 'blue.300' : 'whiteAlpha.200'}
                              color={isPrimaryIcon ? 'white' : undefined}
                              cursor={isSelectable ? 'pointer' : 'default'}
                              onClick={() => {
                                if (isSelectable && link.slug) togglePrimaryIcon(link.slug)
                              }}
                            >
                              <TagLabel color="white">
                                {link.type === 'catalog' && meta && (
                                  <Box as="img" src={resolveWithBase(meta.iconUrl)} alt={link.label} boxSize="12px" objectFit="contain" display="inline-block" mr={1.5} verticalAlign="middle" />
                                )}
                                {link.label}
                              </TagLabel>
                              {!isReadOnly && (
                                <TagCloseButton
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
                                <Text fontSize="xs" color="gray.400">{link.type === 'custom' ? 'Custom technology' : (meta?.provider || 'General')}</Text>
                                {sourceUrl && (
                                  <Text as="a" href={sourceUrl} target="_blank" rel="noreferrer" fontSize="xs" color="blue.300" textDecoration="underline" pointerEvents="auto">
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

                <Text fontSize="10px" color="gray.500">Maximum 3 linked technologies.</Text>
              </VStack>
            </FormControl>
            <FormControl isDisabled={isReadOnly}>
              <FormLabel>URL</FormLabel>
              <Input
                size="sm"
                value={url}
                onChange={(e) => setUrl(e.target.value)}
                onBlur={scheduleAutoSave}
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
                    setTags((prev) => [...prev, tag])
                    scheduleAutoSave()
                  }
                }}
                isReadOnly={isReadOnly}
              />
              <Wrap mt={3}>
                {tags.map((tag) => (
                  <WrapItem key={tag}>
                    <Tag size="sm" variant="subtle" bg="whiteAlpha.100" border="1px solid" borderColor="whiteAlpha.200">
                      <TagLabel color="white">{tag}</TagLabel>
                      {!isReadOnly && (
                        <TagCloseButton onClick={() => {
                          setTags((prev) => prev.filter((t) => t !== tag))
                          scheduleAutoSave()
                        }} />
                      )}
                    </Tag>
                  </WrapItem>
                ))}
              </Wrap>
            </FormControl>

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
              <Box borderTop="1px solid" borderColor="whiteAlpha.100" pt={3}>
                <FormLabel fontSize="xs" fontWeight="bold" color="gray.400" mb={2}>DRILL DOWN</FormLabel>
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

            {isEdit && canEdit && (
              <HStack borderTop="1px solid" borderColor="whiteAlpha.100" pt={2} spacing={2}>
                <Button variant="subtle" size="sm" color="white" _hover={{ bg: 'whiteAlpha.100' }} onClick={handleDelete} flex={1}>
                  Remove
                </Button>
                <Button variant="subtle" size="sm" color="red.300" _hover={{ bg: 'red.900', color: 'red.100' }} onClick={confirmPermanentDelete.onOpen} flex={1}>
                  Delete Element
                </Button>
              </HStack>
            )}
          </VStack>
        </ScrollIndicatorWrapper>

        <Divider borderColor="whiteAlpha.100" />

        {/* Footer */}
        <HStack px={4} py={3} justify="space-between" flexShrink={0}>

          {!autoSaveEdit && (
            <HStack ml="auto">
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
        </HStack>
      </SlidingPanel>

      <ConfirmDialog
        isOpen={confirmPermanentDelete.isOpen}
        onClose={confirmPermanentDelete.onClose}
        onConfirm={handlePermanentDelete}
        title="Delete Element"
        body="Permanently delete this element? It will be removed from all views and cannot be recovered."
        confirmLabel="Delete Permanently"
      />
    </>
  )
}

export default memo(ElementPanel)
