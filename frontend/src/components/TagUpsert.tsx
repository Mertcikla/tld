import { useEffect, useRef, useState } from 'react'
import { Box, HStack, Input, Text, VStack } from '@chakra-ui/react'
import { isElementGroupTag } from '../utils/elementGroups'

interface Props {
  currentTags: string[]
  availableTags: string[]
  groups?: { tag: string; name: string; color?: string | null }[]
  onAddTag: (tag: string) => void
  isReadOnly?: boolean
}

const GROUP_PREFIX = 'group:'

export default function TagUpsert({
  currentTags,
  availableTags,
  groups = [],
  onAddTag,
  isReadOnly = false,
}: Props) {
  const [query, setQuery] = useState('')
  const [activeIndex, setActiveIndex] = useState(0)
  const inputRef = useRef<HTMLInputElement>(null)

  const trimmed = query.trim()
  const lower = trimmed.toLowerCase()
  const isGroupQuery = lower.startsWith(GROUP_PREFIX)
  const groupTerm = isGroupQuery ? lower.slice(GROUP_PREFIX.length).trim() : ''

  const filtered = (() => {
    if (!trimmed || isGroupQuery) return []
    const q = lower
    return availableTags
      .filter((tag) => !isElementGroupTag(tag))
      .filter((t) => t.toLowerCase().includes(q) && !currentTags.includes(t))
      .slice(0, 8)
  })()

  const groupMatches = isGroupQuery
    ? groups
      .filter((group) => !currentTags.includes(group.tag))
      .filter((group) => (
        !groupTerm
        || group.name.toLowerCase().includes(groupTerm)
        || group.tag.toLowerCase().includes(groupTerm)
      ))
      .slice(0, 8)
    : []

  type ResultItem =
    | { kind: 'new'; label: string }
    | { kind: 'existing'; tag: string }
    | { kind: 'group'; tag: string; name: string; color?: string | null }

  const results: ResultItem[] = []

  if (!isGroupQuery && trimmed && !isElementGroupTag(trimmed) && !currentTags.includes(trimmed)) {
    results.push({ kind: 'new', label: trimmed })
  }

  if (!isGroupQuery) {
    filtered.forEach(tag => {
      if (tag.toLowerCase() !== lower) {
        results.push({ kind: 'existing', tag })
      }
    })
  }

  groupMatches.forEach((group) => {
    results.push({ kind: 'group', tag: group.tag, name: group.name, color: group.color })
  })

  useEffect(() => { setActiveIndex(0) }, [query])

  const confirm = (idx: number) => {
    const item = results[idx]
    if (!item) return
    if (item.kind === 'new') {
      onAddTag(item.label)
    } else {
      onAddTag(item.tag)
    }
    setQuery('')
  }

  const onKeyDown = (e: React.KeyboardEvent<HTMLInputElement>) => {
    if (e.key === 'Enter') { 
      e.preventDefault()
      if (results.length > 0) {
        confirm(activeIndex)
      } else if (!isGroupQuery && trimmed && !currentTags.includes(trimmed)) {
        onAddTag(trimmed)
        setQuery('')
      }
      return 
    }
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
    <VStack align="stretch" spacing={2}>
      <Box position="relative">
        <Input
          data-testid="tag-upsert-input"
          ref={inputRef}
          value={query}
          onChange={(e) => setQuery(e.target.value)}
          onKeyDown={onKeyDown}
          placeholder={groups.length > 0 ? 'Search or create tag, or "group:"…' : 'Search or create tag...'}
          size="sm"
          bg="blackAlpha.300"
          border="1px solid"
          borderColor="whiteAlpha.200"
          _focus={{ borderColor: 'var(--accent)', boxShadow: 'none' }}
          rounded="md"
          color="white"
          isDisabled={isReadOnly}
          autoComplete="off"
        />

        {query.trim() && results.length > 0 && (
          <Box
            position="absolute"
            left="0"
            top="calc(100% + 4px)"
            zIndex={100}
            bg="gray.800"
            border="1px solid"
            borderColor="whiteAlpha.300"
            rounded="md"
            shadow="xl"
            w="full"
            maxH="200px"
            overflowY="auto"
          >
            <VStack spacing={0} align="stretch">
              {results.map((item, i) => (
                <Box
                  data-testid={item.kind === 'new' ? 'tag-upsert-create-option' : item.kind === 'group' ? 'tag-upsert-group-option' : 'tag-upsert-existing-option'}
                  key={i}
                  px={3}
                  py={2}
                  bg={i === activeIndex ? 'whiteAlpha.200' : 'transparent'}
                  cursor="pointer"
                  _hover={{ bg: 'whiteAlpha.100' }}
                  onMouseEnter={() => setActiveIndex(i)}
                  onClick={() => confirm(i)}
                >
                  {item.kind === 'new' ? (
                    <HStack spacing={1.5}>
                      <Text fontSize="10px" color="var(--accent)" fontWeight="bold">+ Create</Text>
                      <Text fontSize="xs" color="white" noOfLines={1}>{item.label}</Text>
                    </HStack>
                  ) : item.kind === 'group' ? (
                    <HStack spacing={1.5}>
                      <Box w="7px" h="7px" rounded="full" bg={item.color ?? 'var(--accent)'} flexShrink={0} />
                      <Text fontSize="xs" color="white" noOfLines={1}>{item.name}</Text>
                      <Text fontSize="10px" color="whiteAlpha.500" flexShrink={0}>Group</Text>
                    </HStack>
                  ) : (
                    <Text fontSize="xs" color="gray.200" noOfLines={1}>{item.tag}</Text>
                  )}
                </Box>
              ))}
            </VStack>
          </Box>
        )}
      </Box>
    </VStack>
  )
}
