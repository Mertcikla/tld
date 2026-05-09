import { useState, useMemo, useCallback, useEffect, useRef } from 'react'
import {
  Box,
  Button,
  CloseButton,
  HStack,
  Input,
  InputGroup,
  InputLeftElement,
  Radio,
  RadioGroup,
  Spinner,
  Text,
  VStack,
  Badge,
} from '@chakra-ui/react'
import type { LibraryElement } from '../types'
import { useElementSearch } from '../hooks/useElementSearch'

interface MergeDialogProps {
  isOpen: boolean
  onClose: () => void
  source: LibraryElement | null
  onMerge: (survivorId: number, resolved: {
    kind: string | null
    description: string | null
    repo: string | null
    branch: string | null
    file_path: string | null
    language: string | null
  }) => Promise<void>
}

type Step = 'select' | 'resolve'

function fieldsConflict(a: LibraryElement, b: LibraryElement): boolean {
  if ((a.kind || null) !== (b.kind || null)) return true
  if ((a.description || null) !== (b.description || null)) return true
  if ((a.repo || null) !== (b.repo || null)) return true
  if ((a.branch || null) !== (b.branch || null)) return true
  if ((a.file_path || null) !== (b.file_path || null)) return true
  if ((a.language || null) !== (a.language || null)) return true
  return false
}

export default function MergeDialog({ isOpen, onClose, source, onMerge }: MergeDialogProps) {
  const [step, setStep] = useState<Step>('select')
  const [survivor, setSurvivor] = useState<LibraryElement | null>(null)
  const [resolved, setResolved] = useState<Record<string, string>>({})
  const [loading, setLoading] = useState(false)
  const [error, setError] = useState<string | null>(null)
  const { query: search, setQuery: setSearch, remoteElements, fetching } = useElementSearch()
  const searchInputRef = useRef<HTMLInputElement>(null)

  useEffect(() => {
    if (!isOpen) return
    const t = setTimeout(() => searchInputRef.current?.focus(), 30)
    return () => clearTimeout(t)
  }, [isOpen])

  const candidates = useMemo(() => {
    if (!source) return []
    const results = remoteElements.filter((el) => el.id !== source.id)
    return results
  }, [source, remoteElements])

  const conflicts = useMemo(() => {
    if (!source || !survivor || !fieldsConflict(source, survivor)) return null
    const items: { key: string; label: string; source: string | null; survivor: string | null }[] = []
    if ((source.kind || null) !== (survivor.kind || null)) {
      items.push({ key: 'kind', label: 'Type/Kind', source: source.kind || null, survivor: survivor.kind || null })
    }
    if ((source.description || null) !== (survivor.description || null)) {
      items.push({ key: 'description', label: 'Description', source: source.description || null, survivor: survivor.description || null })
    }
    const srcGit = [source.repo, source.branch, source.file_path, source.language].filter(Boolean).join(' / ')
    const surGit = [survivor.repo, survivor.branch, survivor.file_path, survivor.language].filter(Boolean).join(' / ')
    if (srcGit !== surGit) {
      items.push({ key: 'gitsource', label: 'Git Source', source: srcGit || null, survivor: surGit || null })
    }
    return items.length > 0 ? items : null
  }, [source, survivor])

  const handleSelect = useCallback((el: LibraryElement) => {
    setSurvivor(el)
    if (source && fieldsConflict(source, el)) {
      setStep('resolve')
    } else {
      handleMerge(el.id)
    }
  // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [source])

  const handleResolveChange = useCallback((key: string, value: string) => {
    setResolved((prev) => ({ ...prev, [key]: value }))
  }, [])

  const handleMerge = useCallback(async (survivorId: number) => {
    setLoading(true)
    setError(null)
    try {
      const finalResolved: {
        kind: string | null
        description: string | null
        repo: string | null
        branch: string | null
        file_path: string | null
        language: string | null
      } = { kind: null, description: null, repo: null, branch: null, file_path: null, language: null }

      if (resolved.kind) finalResolved.kind = resolved.kind
      if (resolved.description) finalResolved.description = resolved.description
      if (resolved.gitsource) {
        const parts = resolved.gitsource.split(' / ')
        if (parts.length === 4) {
          finalResolved.repo = parts[0] || null
          finalResolved.branch = parts[1] || null
          finalResolved.file_path = parts[2] || null
          finalResolved.language = parts[3] || null
        }
      }
      await onMerge(survivorId, finalResolved)
    } catch (e) {
      setError(e instanceof Error ? e.message : 'Merge failed')
    } finally {
      setLoading(false)
    }
  }, [resolved, onMerge])

  const handleResolve = useCallback(async () => {
    if (!survivor) return
    await handleMerge(survivor.id)
  }, [survivor, handleMerge])

  const handleClose = useCallback(() => {
    setStep('select')
    setSearch('')
    setSurvivor(null)
    setResolved({})
    setError(null)
    onClose()
  }, [onClose, setSearch])

  if (!isOpen || !source) return null

  return (
    <Box position="fixed" inset={0} zIndex={1500} display="flex" alignItems="center" justifyContent="center">
      <Box position="absolute" inset={0} bg="blackAlpha.800" onClick={handleClose} />
      <Box position="relative" bg="var(--bg-panel)" border="1px solid" borderColor="whiteAlpha.100" rounded="xl"
        boxShadow="0 8px 32px rgba(0,0,0,0.5)" backdropFilter="blur(20px)" mx={4} w="100%" maxW="440px" maxH="80vh" overflow="hidden">
        <Box px={4} py={3} borderBottom="1px solid" borderColor="whiteAlpha.100">
          <HStack justify="space-between">
            <Text fontWeight="semibold" color="white">
              {step === 'select' ? 'Merge Element Into...' : 'Resolve Conflicts'}
            </Text>
            <CloseButton size="sm" onClick={handleClose} />
          </HStack>
        </Box>

        {step === 'select' && (
          <>
            <Box px={4} pt={3} pb={2}>
              <Text fontSize="xs" color="gray.400">
                Merging <Badge variant="subtle" colorScheme="blue" fontSize="xs" mx={0.5}>{source.name}</Badge> into another element. All connectors will be reassigned.
              </Text>
            </Box>
            <Box px={4} pb={2}>
              <InputGroup size="sm">
                <InputLeftElement pointerEvents="none" color="gray.400" fontSize="xs">
                <Text>&#x1F50D;</Text>
              </InputLeftElement>
                <Input ref={searchInputRef} placeholder="Search elements..." value={search} onChange={(e) => setSearch(e.target.value)}
                  autoFocus bg="whiteAlpha.50" borderColor="whiteAlpha.100" _hover={{ borderColor: 'whiteAlpha.200' }}
                  _focus={{ borderColor: 'var(--accent)' }} />
              </InputGroup>
            </Box>
            <Box maxH="320px" overflowY="auto" px={1}>
              {fetching ? (
                <Box px={4} py={6} textAlign="center">
                  <Spinner size="sm" color="var(--accent)" />
                </Box>
              ) : !search.trim() ? (
                <Box px={4} py={6} textAlign="center">
                  <Text fontSize="sm" color="gray.500">Type to search elements...</Text>
                </Box>
              ) : candidates.length === 0 ? (
                <Box px={4} py={6} textAlign="center">
                  <Text fontSize="sm" color="gray.500">No matching elements</Text>
                </Box>
              ) : (
                <VStack spacing={0} align="stretch" px={1} pb={2}>
                  {candidates.map((el) => (
                    <Button key={el.id} variant="ghost" size="sm" h="auto" py={2.5} px={3} justifyContent="flex-start"
                      color="clay.text" _hover={{ bg: 'whiteAlpha.100' }} onClick={() => handleSelect(el)}>
                      <VStack spacing={0.5} align="start" w="full">
                        <Text fontWeight="medium" fontSize="sm">{el.name}</Text>
                        {el.kind && <Badge variant="subtle" fontSize="2xs" colorScheme="gray">{el.kind}</Badge>}
                      </VStack>
                    </Button>
                  ))}
                </VStack>
              )}
            </Box>
          </>
        )}

        {step === 'resolve' && survivor && conflicts && (
          <>
            <Box px={4} py={2}>
              <Text fontSize="xs" color="gray.400" mb={1}>
                Some fields differ. Choose which value to keep for each conflict.
              </Text>
              <HStack spacing={2} mb={4}>
                <Badge variant="subtle" colorScheme="red" fontSize="2xs">{source.name} (will be deleted)</Badge>
                <Badge variant="subtle" colorScheme="green" fontSize="2xs">{survivor.name} (survivor)</Badge>
              </HStack>
            </Box>
            <Box px={4} pb={2} maxH="280px" overflowY="auto">
              <VStack spacing={4} align="stretch">
                {conflicts.map((conflict) => (
                  <Box key={conflict.key} p={3} bg="whiteAlpha.50" rounded="md" border="1px solid" borderColor="whiteAlpha.100">
                    <Text fontSize="xs" fontWeight="medium" color="gray.300" mb={2}>{conflict.label}</Text>
                    <RadioGroup value={resolved[conflict.key] || ''} onChange={(val) => handleResolveChange(conflict.key, val)}>
                      <VStack spacing={1.5} align="stretch">
                        {conflict.source && conflict.source !== 'null' && conflict.source !== '' && (
                          <Radio value="source" size="sm">
                            <HStack spacing={1.5}>
                              <Text fontSize="xs" color="gray.300">{conflict.source}</Text>
                              <Badge variant="subtle" colorScheme="red" fontSize="2xs">source</Badge>
                            </HStack>
                          </Radio>
                        )}
                        {conflict.survivor && conflict.survivor !== 'null' && conflict.survivor !== '' && (
                          <Radio value="survivor" size="sm">
                            <HStack spacing={1.5}>
                              <Text fontSize="xs" color="gray.300">{conflict.survivor}</Text>
                              <Badge variant="subtle" colorScheme="green" fontSize="2xs">survivor</Badge>
                            </HStack>
                          </Radio>
                        )}
                        <Radio value="none" size="sm">
                          <Text fontSize="xs" color="gray.400">Leave empty</Text>
                        </Radio>
                      </VStack>
                    </RadioGroup>
                  </Box>
                ))}
              </VStack>
            </Box>
            <Box px={4} pt={2} pb={4} borderTop="1px solid" borderColor="whiteAlpha.100">
              <HStack justify="space-between">
                <Button variant="ghost" size="sm" onClick={() => setStep('select')}>Back</Button>
                <Button size="sm" colorScheme="blue" onClick={handleResolve} isLoading={loading}>
                  Merge
                </Button>
              </HStack>
            </Box>
          </>
        )}

        {error && (
          <Box px={4} pb={3}>
            <Text fontSize="xs" color="red.300">{error}</Text>
          </Box>
        )}
      </Box>
    </Box>
  )
}
