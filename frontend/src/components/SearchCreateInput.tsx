import { useEffect, useState } from 'react'
import { Box, HStack, Input, Text, VStack } from '@chakra-ui/react'

interface Props {
  value: string
  onChange: (value: string) => void
  options: string[]
  onSubmit?: (value: string) => void
  submitOnSelect?: boolean
  placeholder?: string
  isDisabled?: boolean
  autoFocus?: boolean
  maxResults?: number
  inputTestId?: string
  createOptionTestId?: string
  existingOptionTestId?: string
}

export default function SearchCreateInput({
  value,
  onChange,
  options,
  onSubmit,
  submitOnSelect = false,
  placeholder = 'Search or create...',
  isDisabled = false,
  autoFocus = false,
  maxResults = 8,
  inputTestId = 'search-create-input',
  createOptionTestId = 'search-create-create-option',
  existingOptionTestId = 'search-create-existing-option',
}: Props) {  const [activeIndex, setActiveIndex] = useState(0)
  const [showResults, setShowResults] = useState(false)

  const query = value.trim()
  const lower = query.toLowerCase()
  const filtered = query
    ? options.filter((option) => option.toLowerCase().includes(lower)).slice(0, maxResults)
    : []
  const hasExact = query.length > 0 && options.some((option) => option.toLowerCase() === lower)
  const results = query.length === 0 ? [] : hasExact ? filtered : [query, ...filtered]

  useEffect(() => { setActiveIndex(0) }, [value])

  const choose = (next: string) => {
    onChange(next)
    setShowResults(false)
    if (submitOnSelect) onSubmit?.(next)
  }

  const onKeyDown = (event: React.KeyboardEvent<HTMLInputElement>) => {
    if (event.key === 'Enter') {
      event.preventDefault()
      if (submitOnSelect && results.length > 0) {
        choose(results[activeIndex] ?? results[0])
      } else if (onSubmit && query) {
        onSubmit(query)
      } else if (results.length > 0) {
        choose(results[activeIndex] ?? results[0])
      }
      return
    }
    if (event.key === 'ArrowDown') {
      event.preventDefault()
      setActiveIndex((index) => Math.min(index + 1, results.length - 1))
    }
    if (event.key === 'ArrowUp') {
      event.preventDefault()
      setActiveIndex((index) => Math.max(index - 1, 0))
    }
  }

  return (
    <Box position="relative">
      <Input
        data-testid={inputTestId}
        value={value}
        onChange={(event) => {
          onChange(event.target.value)
          setShowResults(true)
        }}
        onFocus={() => setShowResults(true)}
        onKeyDown={onKeyDown}
        placeholder={placeholder}
        size="sm"
        bg="blackAlpha.300"
        border="1px solid"
        borderColor="whiteAlpha.200"
        _focus={{ borderColor: 'var(--accent)', boxShadow: 'none' }}
        rounded="md"
        color="white"
        isDisabled={isDisabled}
        autoFocus={autoFocus}
        autoComplete="off"
      />

      {showResults && query.length > 0 && results.length > 0 && (
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
            {results.map((item, index) => {
              const isCreate = !hasExact && index === 0
              return (
                <Box
                  data-testid={isCreate ? createOptionTestId : existingOptionTestId}
                  key={item}
                  px={3}
                  py={2}
                  bg={index === activeIndex ? 'whiteAlpha.200' : 'transparent'}
                  cursor="pointer"
                  _hover={{ bg: 'whiteAlpha.100' }}
                  onMouseEnter={() => setActiveIndex(index)}
                  onMouseDown={(event) => {
                    event.preventDefault()
                    choose(item)
                  }}
                >
                  {isCreate ? (
                    <HStack spacing={1.5}>
                      <Text fontSize="10px" color="var(--accent)" fontWeight="bold">+ Create</Text>
                      <Text fontSize="xs" color="white" noOfLines={1}>{item}</Text>
                    </HStack>
                  ) : (
                    <Text fontSize="xs" color="gray.200" noOfLines={1}>{item}</Text>
                  )}
                </Box>
              )
            })}
          </VStack>
        </Box>
      )}
    </Box>
  )
}
