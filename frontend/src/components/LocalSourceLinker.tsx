/**
 * VS Code variant of GitSourceLinker.
 * Uses the postMessage bridge to query workspace files and LSP symbols
 * instead of fetching from GitHub.
 */
import { useState, useEffect, useCallback, useRef } from 'react'
import {
  Box,
  Button,
  HStack,
  Input,
  InputGroup,
  InputRightElement,
  Spinner,
  Text,
  VStack,
} from '@chakra-ui/react'
import { CheckIcon, SearchIcon } from '@chakra-ui/icons'
import { vscodeBridge } from '../lib/vscodeBridge'
import type { WorkspaceSymbol, ExtensionToWebviewMessage } from '../types/vscode-messages'
import type { LibraryElement } from '../types'

interface Props {
  element: LibraryElement
  isReadOnly: boolean
  onUpdate: (updates: Partial<LibraryElement>) => void
}

function randomId() {
  return Math.random().toString(36).slice(2, 10)
}

function buildSymbolAnchor(sym: Pick<WorkspaceSymbol, 'filePath' | 'name' | 'kind'>): string {
  return `${sym.filePath}#${JSON.stringify({ name: sym.name, type: sym.kind.toLowerCase() })}`
}

function parseSourceAnchor(link: string): { name?: string; type?: string; startLine?: number } {
  const hashIdx = link.indexOf('#')
  if (hashIdx < 0) {
    return {}
  }

  try {
    const parsed = JSON.parse(link.slice(hashIdx + 1))
    return {
      name: typeof parsed.name === 'string' ? parsed.name : undefined,
      type: typeof parsed.type === 'string' ? parsed.type : undefined,
      startLine: typeof parsed.startLine === 'number' ? parsed.startLine : undefined,
    }
  } catch {
    return {}
  }
}

export default function LocalSourceLinker({ element, isReadOnly, onUpdate }: Props) {
  const [fileQuery, setFileQuery] = useState('')
  const [files, setFiles] = useState<string[]>([])
  const [filesLoading, setFilesLoading] = useState(false)
  const [selectedFile, setSelectedFile] = useState<string | null>(null)
  const [symbols, setSymbols] = useState<WorkspaceSymbol[]>([])
  const [symbolsLoading, setSymbolsLoading] = useState(false)

  const pendingRef = useRef<Map<string, (data: unknown) => void>>(new Map())

  // Register message listener once
  useEffect(() => {
    const unsubscribe = vscodeBridge.onMessage((msg: ExtensionToWebviewMessage) => {
      if (msg.type === 'workspace-files') {
        const resolve = pendingRef.current.get(msg.requestId)
        if (resolve) {
          pendingRef.current.delete(msg.requestId)
          resolve(msg.files)
        }
      } else if (msg.type === 'workspace-symbols') {
        const resolve = pendingRef.current.get(msg.requestId)
        if (resolve) {
          pendingRef.current.delete(msg.requestId)
          resolve(msg.symbols)
        }
      }
    })
    return unsubscribe
  }, [])

  // Pre-populate from existing file_path
  useEffect(() => {
    if (!element.file_path) return
    const hashIdx = element.file_path.indexOf('#')
    const fp = hashIdx >= 0 ? element.file_path.slice(0, hashIdx) : element.file_path
    if (fp) {
      setSelectedFile(fp)
      setFileQuery(fp)
    }
  }, [element.file_path])

  const requestFiles = useCallback(
    (query: string) => {
      if (!query.trim()) {
        setFiles([])
        return
      }
      const requestId = randomId()
      setFilesLoading(true)
      const pattern = query.includes('*') ? query : `**/*${query}*`
      const promise = new Promise<string[]>((resolve) => {
        pendingRef.current.set(requestId, (data) => resolve(data as string[]))
      })
      vscodeBridge.postMessage({ type: 'request-workspace-files', requestId, pattern })
      void promise.then((result) => {
        setFiles(result.slice(0, 50))
        setFilesLoading(false)
      })
    },
    [],
  )

  const requestSymbols = useCallback((filePath: string) => {
    const requestId = randomId()
    setSymbolsLoading(true)
    const promise = new Promise<WorkspaceSymbol[]>((resolve) => {
      pendingRef.current.set(requestId, (data) => resolve(data as WorkspaceSymbol[]))
    })
    vscodeBridge.postMessage({ type: 'request-symbol-list-for-file', requestId, filePath })
    void promise.then((result) => {
      setSymbols(result)
      setSymbolsLoading(false)
    })
  }, [])

  // Debounce file search
  useEffect(() => {
    if (isReadOnly) return
    const timer = setTimeout(() => requestFiles(fileQuery), 300)
    return () => clearTimeout(timer)
  }, [fileQuery, isReadOnly, requestFiles])

  const handleSelectFile = (fp: string) => {
    setSelectedFile(fp)
    setFileQuery(fp)
    setFiles([])
    setSymbols([])
    requestSymbols(fp)
  }

  const handleSelectSymbol = (sym: WorkspaceSymbol) => {
    onUpdate({ file_path: buildSymbolAnchor(sym) })
  }

  const handleClear = () => {
    setSelectedFile(null)
    setFileQuery('')
    setFiles([])
    setSymbols([])
    onUpdate({ file_path: '' })
  }

  const currentLink = element.file_path || ''
  const hasLink = !!currentLink

  if (isReadOnly && hasLink) {
    const hashIdx = currentLink.indexOf('#')
    const filePart = hashIdx >= 0 ? currentLink.slice(0, hashIdx) : currentLink
    const anchor = parseSourceAnchor(currentLink)
    const symbolName = anchor.name ?? ''
    return (
      <Box>
        <HStack spacing={1} mb={1}>
          <CheckIcon color="green.400" boxSize={3} />
          <Text fontSize="11px" color="gray.300" fontWeight="medium" isTruncated>
            {symbolName ? `${symbolName} ${filePart}` : filePart}
          </Text>
        </HStack>
        <Button
          size="xs"
          variant="ghost"
          color="blue.400"
          px={0}
          h="auto"
          onClick={() => {
            if (!element.file_path) return
            const hashIdx2 = element.file_path.indexOf('#')
            const fp = hashIdx2 >= 0 ? element.file_path.slice(0, hashIdx2) : element.file_path
            const parsedAnchor = parseSourceAnchor(element.file_path)
            vscodeBridge.postMessage({
              type: 'open-file',
              filePath: fp,
              startLine: parsedAnchor.startLine,
              symbolName: parsedAnchor.name,
              symbolKind: parsedAnchor.type,
            })
          }}
        >
          Open in Editor
        </Button>
      </Box>
    )
  }

  return (
    <VStack align="stretch" spacing={2}>
      <Box>
        <Text fontSize="10px" color="gray.500" mb={1} textTransform="uppercase" letterSpacing="wider">
          Workspace File
        </Text>
        <InputGroup size="sm">
          <Input
            value={fileQuery}
            onChange={(e) => setFileQuery(e.target.value)}
            placeholder="Search files…"
            bg="whiteAlpha.50"
            border="1px solid"
            borderColor="whiteAlpha.100"
            fontSize="12px"
            isDisabled={isReadOnly}
          />
          <InputRightElement>
            {filesLoading ? <Spinner size="xs" /> : <SearchIcon boxSize={3} color="gray.500" />}
          </InputRightElement>
        </InputGroup>

        {files.length > 0 && (
          <Box
            mt={1}
            maxH="140px"
            overflowY="auto"
            bg="#0d1117"
            border="1px solid"
            borderColor="whiteAlpha.100"
            rounded="md"
            className="custom-scrollbar"
          >
            {files.map((f) => (
              <Box
                key={f}
                px={2}
                py={1}
                fontSize="11px"
                color="gray.300"
                cursor="pointer"
                _hover={{ bg: 'whiteAlpha.100', color: 'white' }}
                onClick={() => handleSelectFile(f)}
                isTruncated
              >
                {f}
              </Box>
            ))}
          </Box>
        )}
      </Box>

      {selectedFile && (
        <Box>
          <Text fontSize="10px" color="gray.500" mb={1} textTransform="uppercase" letterSpacing="wider">
            Symbol
          </Text>
          {symbolsLoading ? (
            <HStack spacing={2} py={1}>
              <Spinner size="xs" />
              <Text fontSize="11px" color="gray.500">Loading symbols…</Text>
            </HStack>
          ) : symbols.length === 0 ? (
            <Text fontSize="11px" color="gray.600">No symbols found</Text>
          ) : (
            <Box
              maxH="140px"
              overflowY="auto"
              bg="#0d1117"
              border="1px solid"
              borderColor="whiteAlpha.100"
              rounded="md"
              className="custom-scrollbar"
            >
              {symbols.map((sym) => {
                const anchor = buildSymbolAnchor(sym)
                const isLinked = currentLink === anchor
                return (
                  <HStack
                    key={`${sym.name}-${sym.startLine}`}
                    px={2}
                    py={1}
                    spacing={2}
                    cursor="pointer"
                    bg={isLinked ? 'rgba(49,130,206,0.2)' : 'transparent'}
                    _hover={{ bg: isLinked ? 'rgba(49,130,206,0.3)' : 'whiteAlpha.100' }}
                    onClick={() => handleSelectSymbol(sym)}
                  >
                    {isLinked && <CheckIcon color="blue.400" boxSize={2.5} flexShrink={0} />}
                    <Text fontSize="11px" color="gray.300" isTruncated>{sym.name}</Text>
                    <Text fontSize="10px" color="gray.600" flexShrink={0}>{sym.kind}</Text>
                  </HStack>
                )
              })}
            </Box>
          )}
        </Box>
      )}

      {hasLink && (
        <HStack spacing={2}>
          <Button
            size="xs"
            variant="ghost"
            color="blue.400"
            px={0}
            h="auto"
            onClick={() => {
              const hashIdx2 = currentLink.indexOf('#')
              const fp = hashIdx2 >= 0 ? currentLink.slice(0, hashIdx2) : currentLink
              const parsedAnchor = parseSourceAnchor(currentLink)
              vscodeBridge.postMessage({
                type: 'open-file',
                filePath: fp,
                startLine: parsedAnchor.startLine,
                symbolName: parsedAnchor.name,
                symbolKind: parsedAnchor.type,
              })
            }}
          >
            Open in Editor
          </Button>
          {!isReadOnly && (
            <Button size="xs" variant="ghost" color="red.400" px={0} h="auto" onClick={handleClear}>
              Clear
            </Button>
          )}
        </HStack>
      )}
    </VStack>
  )
}
