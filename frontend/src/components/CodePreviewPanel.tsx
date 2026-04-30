import { useEffect, useState, useRef } from 'react'
import type { SVGProps } from 'react'
import { Box, Button, CloseButton, HStack, Icon, Spinner, Text, Tooltip, VStack } from '@chakra-ui/react'
import { ExternalLinkIcon } from '@chakra-ui/icons'
import CodeMirror, { ReactCodeMirrorRef } from '@uiw/react-codemirror'
import { EditorView } from '@codemirror/view'
import { oneDark } from '@codemirror/theme-one-dark'
import { javascript } from '@codemirror/lang-javascript'
import { python } from '@codemirror/lang-python'
import { cpp } from '@codemirror/lang-cpp'
import { java } from '@codemirror/lang-java'
import { rust } from '@codemirror/lang-rust'

import SlidingPanel from './SlidingPanel'
import { api } from '../api/client'
import { findSymbolByName, getParser, detectLanguage, type SupportedLanguage } from '../utils/treesitter'
import { githubCache } from '../utils/githubCache'
import { getGithubRepoVisibility } from '../utils/githubApi'
import { parseRepoSlug } from '../utils/url'
import { useSourceEditor } from '../utils/sourceEditor'
import { toast } from '../utils/toast'
import type { PlacedElement } from '../types'

const GithubIcon = (props: SVGProps<SVGSVGElement>) => (
  <svg
    viewBox="0 0 24 24"
    fill="currentColor"
    width="16px"
    height="16px"
    {...props}
  >
    <path d="M12 0C5.37 0 0 5.37 0 12c0 5.31 3.435 9.795 8.205 11.385.6.105.825-.255.825-.57 0-.285-.015-1.23-.015-2.235-3.015.555-3.795-.735-4.035-1.41-.135-.345-.72-1.41-1.23-1.695-.42-.225-1.02-.78-.015-.795.945-.015 1.62.87 1.845 1.23 1.08 1.815 2.805 1.305 3.495.99.105-.78.42-1.305.765-1.605-2.67-.3-5.46-1.335-5.46-5.925 0-1.305.465-2.385 1.23-3.225-.12-.3-.54-1.53.12-3.18 0 0 1.005-.315 3.3 1.23.96-.27 1.98-.405 3-.405s2.04.135 3 .405c2.295-1.56 3.3-1.23 3.3-1.23.66 1.65.24 2.88.12 3.18.765.84 1.23 1.905 1.23 3.225 0 4.605-2.805 5.625-5.475 5.925.435.375.765 1.11.765 2.235 0 1.605-.015 2.895-.015 3.3 0 .315.225.69.825.57A12.02 12.02 0 0024 12c0-6.63-5.37-12-12-12z" />
  </svg>
)

const customCodeTheme = EditorView.theme({
  "&": {
    backgroundColor: "transparent !important",
  },
  ".cm-gutters": {
    backgroundColor: "transparent !important",
    borderRight: "1px solid var(--chakra-colors-whiteAlpha-100)",
    color: "var(--chakra-colors-whiteAlpha-300)",
  },
  ".cm-activeLine": {
    backgroundColor: "var(--chakra-colors-whiteAlpha-100) !important",
  },
  ".cm-activeLineGutter": {
    backgroundColor: "var(--chakra-colors-whiteAlpha-200) !important",
  },
  ".cm-content": {
    caretColor: "var(--chakra-colors-blue-400)",
  }
}, { dark: true })

interface Props {
  isOpen: boolean
  onClose: () => void
  element: PlacedElement | null
  hasBackdrop?: boolean
}

function parseAnchor(anchorStr: string):
  | { kind: 'symbol'; name: string; type: string }
  | { kind: 'lines'; startLine: number; endLine: number }
  | { kind: 'none' } {
  if (!anchorStr) return { kind: 'none' }
  try {
    const p = JSON.parse(anchorStr)
    if (p.name && !p.startLine) return { kind: 'symbol', name: p.name, type: p.type || '' }
    if (p.startLine) return { kind: 'lines', startLine: p.startLine, endLine: p.endLine ?? p.startLine }
  } catch {
    // intentionally empty
  }
  return { kind: 'none' }
}

function inferLineFromDescription(description: string | null | undefined, basePath: string): number | null {
  if (!description || !basePath) return null
  const match = description.match(/:(\d+)(?::\d+)?$/)
  if (!match) return null
  const pathPart = description.slice(0, match.index)
  if (pathPart && pathPart !== basePath) return null
  const line = Number(match[1])
  return Number.isFinite(line) && line > 0 ? line : null
}

export default function CodePreviewPanel({ isOpen, onClose, element, hasBackdrop = true }: Props) {
  const [code, setCode] = useState('')
  const [loading, setLoading] = useState(false)
  const [error, setError] = useState<string | null>(null)
  const [resolvedStartLine, setResolvedStartLine] = useState<number | null>(null)
  const [resolvedEndLine, setResolvedEndLine] = useState<number | null>(null)
  const [isPrivateRepo, setIsPrivateRepo] = useState(false)
  const [openingEditor, setOpeningEditor] = useState(false)
  const { editor: sourceEditor } = useSourceEditor()

  const editorRef = useRef<ReactCodeMirrorRef>(null)

  const filePath = element?.file_path || ''
  const hashIdx = filePath.indexOf('#')
  const basePath = hashIdx >= 0 ? filePath.slice(0, hashIdx) : filePath
  const symbolInfoStr = hashIdx >= 0 ? filePath.slice(hashIdx + 1) : ''
  const repoSlug = element?.repo ? parseRepoSlug(element.repo) : ''
  const anchor = parseAnchor(symbolInfoStr)
  const anchorStartLine = anchor.kind === 'lines' ? anchor.startLine : null
  const fallbackStartLine = inferLineFromDescription(element?.description, basePath)
  const editorStartLine = resolvedStartLine ?? anchorStartLine ?? fallbackStartLine

  useEffect(() => {
    if (!isOpen || !element || !repoSlug || !basePath) return

    let cancelled = false
    setLoading(true)
    setError(null)
    setCode('')
    setResolvedStartLine(null)
    setResolvedEndLine(null)
    setIsPrivateRepo(false)

    const branch = element.branch || 'main'
    const rawUrl = `https://raw.githubusercontent.com/${repoSlug}/refs/heads/${branch}/${basePath}`

    // Check repo visibility (cached)
    const checkAndFetch = async () => {
      const cachedVisibility = githubCache.getRepoPublic(repoSlug)
      if (cachedVisibility === false) {
        if (!cancelled) { setIsPrivateRepo(true); setLoading(false) }
        return
      }
      if (cachedVisibility === null) {
        try {
          const visibility = await getGithubRepoVisibility(repoSlug)
          if (visibility === 'public') {
            githubCache.setRepoPublic(repoSlug, true)
          } else if (visibility === 'private') {
            githubCache.setRepoPublic(repoSlug, false)
            if (!cancelled) { setIsPrivateRepo(true); setLoading(false) }
            return
          }
        } catch {
          // Network error: proceed optimistically
        }
      }

      const cached = githubCache.getContent(rawUrl)
      if (cached) {
        if (!cancelled) setCode(cached)
        if (!cancelled) setLoading(false)
        const anchor = parseAnchor(symbolInfoStr)
        const effectiveLanguage = element.language || detectLanguage(basePath)
        if (anchor.kind === 'symbol' && effectiveLanguage) {
          getParser(effectiveLanguage as SupportedLanguage).then(async (parser) => {
            const tree = parser.parse(cached)
            const found = findSymbolByName(tree, effectiveLanguage as SupportedLanguage, anchor.name, anchor.type)
            if (!cancelled && found) {
              setResolvedStartLine(found.startLine)
              setResolvedEndLine(found.endLine)
            }
          }).catch(() => {})
        } else if (anchor.kind === 'lines' && !cancelled) {
          setResolvedStartLine(anchor.startLine)
          setResolvedEndLine(anchor.endLine)
        }
        return
      }

      try {
        const res = await fetch(rawUrl)
        if (!res.ok) throw new Error(`Failed to fetch: ${res.statusText}`)
        const text = await res.text()
        if (cancelled) return
        githubCache.setContent(rawUrl, text)
        setCode(text)

        const anchor = parseAnchor(symbolInfoStr)
        const effectiveLanguage = element.language || detectLanguage(basePath)
        if (anchor.kind === 'symbol' && effectiveLanguage) {
          try {
            const parser = await getParser(effectiveLanguage as SupportedLanguage)
            const tree = parser.parse(text)
            const found = findSymbolByName(tree, effectiveLanguage as SupportedLanguage, anchor.name, anchor.type)
            if (!cancelled && found) {
              setResolvedStartLine(found.startLine)
              setResolvedEndLine(found.endLine)
            }
          } catch {
            // intentionally empty
          }
        } else if (anchor.kind === 'lines') {
          if (!cancelled) {
            setResolvedStartLine(anchor.startLine)
            setResolvedEndLine(anchor.endLine)
          }
        }
      } catch (err: unknown) {
        if (!cancelled) setError(err instanceof Error ? err.message : String(err))
      } finally {
        if (!cancelled) setLoading(false)
      }
    }

    checkAndFetch()
    return () => { cancelled = true }
  }, [isOpen, element, repoSlug, basePath, symbolInfoStr])

  useEffect(() => {
    if (!code || !resolvedStartLine || !editorRef.current?.view) return
    const view = editorRef.current.view
    try {
      const linePos = view.state.doc.line(resolvedStartLine)
      const endPos = view.state.doc.line(resolvedEndLine ?? resolvedStartLine)
      view.dispatch({
        selection: { anchor: linePos.from, head: endPos.to },
        effects: [
          EditorView.scrollIntoView(linePos.from, { y: 'center' })
        ]
      })
    } catch {
    // intentionally empty
  }
  }, [code, resolvedStartLine, resolvedEndLine])

  const githubUrl = element?.repo && basePath
    ? `https://github.com/${repoSlug}/blob/${element.branch || 'main'}/${basePath}`
    + (editorStartLine ? `#L${editorStartLine}-L${resolvedEndLine ?? editorStartLine}` : '')
    : null

  const handleOpenInEditor = async () => {
    if (!basePath) return
    setOpeningEditor(true)
    try {
      await api.editor.open({
        editor: sourceEditor,
        repo: element?.repo ?? '',
        file_path: basePath,
        line: editorStartLine,
      })
    } catch (err) {
      toast({
        title: 'Failed to open editor',
        description: err instanceof Error ? err.message : String(err),
        status: 'error',
        duration: 4000,
      })
    } finally {
      setOpeningEditor(false)
    }
  }

  const getLanguageExtension = () => {
    const extensions = [customCodeTheme]
    const effectiveLanguage = element?.language || detectLanguage(basePath)
    switch (effectiveLanguage) {
      case 'javascript':
      case 'typescript':
        extensions.push(javascript({ typescript: effectiveLanguage === 'typescript' }))
        break
      case 'python': extensions.push(python()); break
      case 'cpp': extensions.push(cpp()); break
      case 'java': extensions.push(java()); break
      case 'rust': extensions.push(rust()); break
    }
    return extensions
  }

  return (
    <SlidingPanel
      isOpen={isOpen}
      onClose={onClose}
      panelKey="code-preview"
      side="left"
      width={{ base: 'calc(100vw - 32px)', md: '45vw' }}
      minWidth="300px"
      height="calc(90vh - 2rem)"
      maxHeight="calc(90vh - 2rem)"
      hasBackdrop={hasBackdrop}
      zIndex={1300}
    >
      {/* Header */}
      <HStack px={4} pt={4} pb={3} justify="space-between" flexShrink={0}
        borderBottom="1px solid" borderColor="whiteAlpha.100">
        <VStack align="start" spacing={0} minW={0} flex={1}>
          <Text fontSize="10px" color="gray.600" letterSpacing="widest" textTransform="uppercase" fontWeight="600">
            Source
          </Text>
          <Text fontSize="sm" fontWeight="700" color="white" isTruncated maxW={{ base: '180px', md: '360px' }} letterSpacing="0.01em">
            {basePath.split('/').pop() || 'Code'}
          </Text>
          {resolvedStartLine && (
            <Text fontSize="10px" color="blue.400" fontFamily="mono">
              L{resolvedStartLine}–L{resolvedEndLine ?? resolvedStartLine}
            </Text>
          )}
          {isPrivateRepo && (
            <Text fontSize="10px" color="orange.400" fontFamily="mono">
              Private repository
            </Text>
          )}
        </VStack>
        <HStack spacing={2} flexShrink={0}>
          {element?.url && (
            <Tooltip label="Open URL" placement="bottom">
              <Button
                as="a"
                href={element.url}
                target="_blank"
                rel="noopener noreferrer"
                aria-label="Open URL"
                leftIcon={<ExternalLinkIcon w="12px" h="12px" />}
                size="xs"
                variant="outline"
                color="whiteAlpha.700"
                borderColor="whiteAlpha.200"
                h="24px"
                px={2.5}
                fontSize="11px"
                fontWeight="600"
                bg="whiteAlpha.50"
                _hover={{
                  color: 'white',
                  bg: 'whiteAlpha.100',
                  borderColor: 'whiteAlpha.400',
                  textDecoration: 'none',
                  transform: 'translateY(-0.5px)',
                  boxShadow: '0 2px 4px rgba(0,0,0,0.2)'
                }}
                _active={{
                  bg: 'whiteAlpha.200',
                  transform: 'translateY(0)',
                }}
                transition="all 0.1s"
              >
                Open URL
              </Button>
            </Tooltip>
          )}
          {githubUrl && (
            <Tooltip label="Open in GitHub" placement="bottom">
              <Button
                as="a"
                href={isPrivateRepo ? undefined : githubUrl}
                target="_blank"
                rel="noopener noreferrer"
                aria-label="Open in GitHub"
                leftIcon={<Icon as={GithubIcon} boxSize="13px" />}
                size="xs"
                variant="outline"
                color={isPrivateRepo ? 'whiteAlpha.300' : 'whiteAlpha.700'}
                borderColor={isPrivateRepo ? 'whiteAlpha.100' : 'whiteAlpha.200'}
                h="24px"
                px={2.5}
                fontSize="11px"
                fontWeight="600"
                bg={isPrivateRepo ? 'transparent' : 'whiteAlpha.50'}
                pointerEvents={isPrivateRepo ? 'none' : undefined}
                opacity={isPrivateRepo ? 0.4 : undefined}
                _hover={isPrivateRepo ? {} : {
                  color: 'white',
                  bg: 'whiteAlpha.100',
                  borderColor: 'whiteAlpha.400',
                  textDecoration: 'none',
                  transform: 'translateY(-0.5px)',
                  boxShadow: '0 2px 4px rgba(0,0,0,0.2)'
                }}
                _active={{
                  bg: 'whiteAlpha.200',
                  transform: 'translateY(0)',
                }}
                transition="all 0.1s"
              >
                Open in GitHub
              </Button>
            </Tooltip>
          )}
          {basePath && (
            <Tooltip label={`Open in ${sourceEditor === 'zed' ? 'Zed' : 'VS Code'}`} placement="bottom">
              <Button
                aria-label={`Open in ${sourceEditor === 'zed' ? 'Zed' : 'VS Code'}`}
                leftIcon={<ExternalLinkIcon w="12px" h="12px" />}
                size="xs"
                variant="outline"
                color="whiteAlpha.700"
                borderColor="whiteAlpha.200"
                h="24px"
                px={2.5}
                fontSize="11px"
                fontWeight="600"
                bg="whiteAlpha.50"
                isLoading={openingEditor}
                onClick={handleOpenInEditor}
                _hover={{
                  color: 'white',
                  bg: 'whiteAlpha.100',
                  borderColor: 'whiteAlpha.400',
                  textDecoration: 'none',
                  transform: 'translateY(-0.5px)',
                  boxShadow: '0 2px 4px rgba(0,0,0,0.2)'
                }}
                _active={{
                  bg: 'whiteAlpha.200',
                  transform: 'translateY(0)',
                }}
                transition="all 0.1s"
              >
                Open in Editor
              </Button>
            </Tooltip>
          )}
          <CloseButton size="sm" color="whiteAlpha.500"
            _hover={{ color: 'white', bg: 'whiteAlpha.100' }}
            onClick={onClose} />
        </HStack>
      </HStack>

      {/* Code area */}
      <Box flex={1} overflow="hidden" position="relative" bg="var(--bg-main)">
        {loading && (
          <VStack position="absolute" inset={0} justify="center" align="center" bg="blackAlpha.600" zIndex={10}>
            <Spinner color="blue.400" />
            <Text color="white" fontSize="sm">Loading source...</Text>
          </VStack>
        )}
        {error && (
          <VStack position="absolute" inset={0} justify="center" align="center" bg="blackAlpha.800" zIndex={10} px={6}>
            <Text color="red.400" fontSize="sm" textAlign="center">{error}</Text>
          </VStack>
        )}
        {isPrivateRepo && !loading && (
          <VStack position="absolute" inset={0} justify="center" align="center" bg="blackAlpha.800" zIndex={10} px={6} spacing={2}>
            <Icon as={GithubIcon} boxSize="32px" color="whiteAlpha.300" />
            <Text color="whiteAlpha.700" fontSize="sm" fontWeight="600">Private repository</Text>
            <Text color="whiteAlpha.400" fontSize="xs" textAlign="center">
              Code preview is unavailable for private repositories.
            </Text>
          </VStack>
        )}
        <Box position="absolute" inset={0}>
          <CodeMirror
            ref={editorRef}
            value={code}
            height="100%"
            extensions={getLanguageExtension()}
            theme={oneDark}
            readOnly={true}
            editable={false}
            basicSetup={{
              lineNumbers: true,
              highlightActiveLineGutter: true,
              highlightSelectionMatches: true,
            }}
            style={{ fontSize: '12px', height: '100%' }}
          />
        </Box>
      </Box>
    </SlidingPanel>
  )
}
