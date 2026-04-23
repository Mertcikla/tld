import { useEffect, useRef, useState } from 'react'
import { Box, HStack, Input, Text, VStack } from '@chakra-ui/react'

interface Props {
  currentTags: string[]
  availableTags: string[]
  onAddTag: (tag: string) => void
  isReadOnly?: boolean
}

export default function TagUpsert({
  currentTags,
  availableTags,
  onAddTag,
  isReadOnly = false,
}: Props) {
  const [query, setQuery] = useState('')
  const [activeIndex, setActiveIndex] = useState(0)
  const inputRef = useRef<HTMLInputElement>(null)

  const filtered = (() => {
    if (!query.trim()) return []
    const q = query.toLowerCase()
    return availableTags
      .filter((t) => t.toLowerCase().includes(q) && !currentTags.includes(t))
      .slice(0, 8)
  })()

  type ResultItem =
    | { kind: 'new'; label: string }
    | { kind: 'existing'; tag: string }

  const results: ResultItem[] = []
  
  if (query.trim() && !currentTags.includes(query.trim())) {
    results.push({ kind: 'new', label: query.trim() })
  }
  
  filtered.forEach(tag => {
    if (tag.toLowerCase() !== query.trim().toLowerCase()) {
      results.push({ kind: 'existing', tag })
    }
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
      } else if (query.trim() && !currentTags.includes(query.trim())) {
        onAddTag(query.trim())
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
          ref={inputRef}
          value={query}
          onChange={(e) => setQuery(e.target.value)}
          onKeyDown={onKeyDown}
          placeholder="Search or create tag..."
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
