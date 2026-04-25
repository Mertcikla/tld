import { useEffect, useRef, useState } from 'react'
import { Box, Badge, HStack, Input, Text, VStack } from '@chakra-ui/react'
import type { LibraryElement } from '../types'
import { TYPE_COLORS } from '../types'

interface Props {
  x: number
  y: number
  allElements: LibraryElement[]
  existingElementIds: Set<number>
  onConfirmNew: (name: string) => Promise<void>
  onConfirmExisting: (obj: LibraryElement) => Promise<void>
  onCancel: () => void
  expandResults?: boolean
  allowCreate?: boolean
  placeholder?: string
  title?: string
  getSecondaryLabel?: (obj: LibraryElement) => string | null
}

export default function InlineElementAdder({
  x,
  y,
  allElements,
  existingElementIds,
  onConfirmNew,
  onConfirmExisting,
  onCancel,
  expandResults = false,
  allowCreate = true,
  placeholder = 'Search or create new element...',
  title,
  getSecondaryLabel,
}: Props) {
  const [query, setQuery] = useState('')
  const [activeIndex, setActiveIndex] = useState(0)
  const [busy, setBusy] = useState(false)
  const inputRef = useRef<HTMLInputElement>(null)

  useEffect(() => {
    const t = setTimeout(() => inputRef.current?.focus(), 30)
    return () => clearTimeout(t)
  }, [])

  const filtered = (() => {
    if (!query.trim()) return []
    try {
      const re = new RegExp(query, 'i')
      return allElements.filter((o) => re.test(o.name)).slice(0, 8)
    } catch {
      const q = query.toLowerCase()
      return allElements.filter((o) => o.name.toLowerCase().includes(q)).slice(0, 8)
    }
  })()

  type ResultItem =
    | { kind: 'new'; label: string }
    | { kind: 'existing'; obj: LibraryElement }

  const results: ResultItem[] = [
    ...(allowCreate ? [{ kind: 'new', label: query.trim() || 'Unnamed' } as ResultItem] : []),
    ...filtered.map((obj): ResultItem => ({ kind: 'existing', obj })),
  ]

  useEffect(() => { setActiveIndex(0) }, [query])

  const confirm = async (idx: number) => {
    const item = results[idx]
    if (!item || busy) return
    setBusy(true)
    try {
      if (item.kind === 'new') {
        await onConfirmNew(item.label)
      } else {
        await onConfirmExisting(item.obj)
      }
    } finally {
      setBusy(false)
    }
  }

  const onKeyDown = (e: React.KeyboardEvent<HTMLInputElement>) => {
    if (e.key === 'Escape') { onCancel(); return }
    if (e.key === 'Enter') { e.preventDefault(); confirm(activeIndex); return }
    if (e.key === 'ArrowDown') {
      e.preventDefault()
      setActiveIndex((i) => Math.min(i + 1, results.length - 1))
    }
    if (e.key === 'ArrowUp') {
      e.preventDefault()
      setActiveIndex((i) => Math.max(i - 1, 0))
    }
  }

  return (
    <Box
      position="absolute"
      left={`${x}px`}
      top={`${y}px`}
      zIndex={2000}
      onPointerDown={(e) => e.stopPropagation()}
      onClick={(e) => e.stopPropagation()}
      display="flex"
      flexDirection="column"
      alignItems="center"
      pointerEvents="none"
    >
      <Box position="relative" pointerEvents="auto" transform="translate(-50%, -50%)">
        <Input
          data-testid="inline-element-adder-input"
          ref={inputRef}
          value={query}
          onChange={(e) => setQuery(e.target.value)}
          onKeyDown={onKeyDown}
          placeholder={placeholder}
          size="sm"
          w="220px"
          bg="var(--bg-panel)"
          border="2px solid"
          borderColor="var(--accent)"
          _focus={{ borderColor: 'var(--accent)', boxShadow: 'none' }}
          rounded="md"
          color="white"
          isDisabled={busy}
          autoComplete="off"
        />
        {title && (
          <Box position="absolute" left="8px" top="-18px" zIndex={2002}>
            <Text fontSize="10px" color="gray.400" fontWeight="700" letterSpacing="0.06em" textTransform="uppercase">
              {title}
            </Text>
          </Box>
        )}

        {/* Results Box */}
        {results.length > 0 && (
          <Box
            position="absolute"
            left={expandResults ? 'calc(100% + 8px)' : '0'}
            top={expandResults ? '50%' : 'calc(100% + 4px)'}
            transform={expandResults ? 'translateY(-50%)' : undefined}
            zIndex={2001}
            bg="var(--bg-panel)"
            border="1px solid"
            borderColor="whiteAlpha.100"
            rounded="xl"
            shadow="0 8px 32px rgba(0,0,0,0.5)"
            minW="220px"
            maxW={expandResults ? '340px' : '280px'}
            w="max-content"
            maxH="300px"
            overflowY="auto"
            flexShrink={0}
          >
            <VStack spacing={0} align="stretch">
              {results.map((item, i) => (
                <Box
                  data-testid={item.kind === 'new' ? 'inline-element-adder-create-option' : 'inline-element-adder-existing-option'}
                  key={i}
                  px={3}
                  py={1.5}
                  bg={i === activeIndex ? 'rgba(var(--accent-rgb), 0.12)' : 'transparent'}
                  cursor={busy ? 'not-allowed' : 'pointer'}
                  _hover={{ bg: i === activeIndex ? 'rgba(var(--accent-rgb), 0.12)' : 'whiteAlpha.100' }}
                  onMouseEnter={() => setActiveIndex(i)}
                  onClick={() => !busy && confirm(i)}
                  transition="background 0.1s"
                >
                  {item.kind === 'new' ? (
                    <HStack spacing={1.5}>
                      <Text fontSize="10px" color="var(--accent)" fontWeight="bold" flexShrink={0}>
                        + Create
                      </Text>
                      <Text
                        fontSize="sm"
                        color="white"
                        noOfLines={1}
                        fontStyle={!query.trim() ? 'italic' : 'normal'}
                        pr={1}
                      >
                        {item.label}
                      </Text>
                    </HStack>
                  ) : (
                    <HStack spacing={2}>
                      <Box flex={1} minW={0}>
                        <Text fontSize="sm" color="gray.200" noOfLines={1}>
                          {item.obj.name}
                        </Text>
                        {(getSecondaryLabel?.(item.obj) || item.obj.technology) && (
                          <Text fontSize="10px" color="gray.500" noOfLines={1}>
                            {getSecondaryLabel?.(item.obj) || item.obj.technology}
                          </Text>
                        )}
                      </Box>
                      <Badge
                        colorScheme={TYPE_COLORS[item.obj.kind ?? ''] ?? 'gray'}
                        fontSize="8px"
                        flexShrink={0}
                      >
                        {item.obj.kind}
                      </Badge>
                      {existingElementIds.has(item.obj.id) && (
                        <Text fontSize="9px" color="gray.500" flexShrink={0}>
                          on canvas
                        </Text>
                      )}
                    </HStack>
                  )}
                </Box>
              ))}
            </VStack>
          </Box>
        )}
      </Box>
    </Box>
  )
}
