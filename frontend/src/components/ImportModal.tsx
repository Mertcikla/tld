import { memo, useCallback, useEffect, useState } from 'react'
import {
  Button,
  FormControl,
  FormLabel,
  HStack,
  Modal,
  ModalBody,
  ModalCloseButton,
  ModalContent,
  ModalFooter,
  ModalHeader,
  ModalOverlay,
  Textarea,
  Text,
  VStack,
  Box,
  Divider,
  Tabs,
  TabList,
  Tab,
  TabPanels,
  TabPanel,
} from '@chakra-ui/react'
import {
  extractMermaidCode,
  mermaidImportSourceLimitError,
  parseMermaidAsync,
  parsedMermaidImportError,
  parsedMermaidImportLimitError,
  type ParsedImport,
} from '../pkg/importer/mermaid'
import { api } from '../api/client'
import type { PlanConnector, PlanElement } from '@buf/tldiagramcom_diagram.bufbuild_es/diag/v1/workspace_service_pb'
import { isWailsApp } from '../config/runtime'
import { mermaidImportFilters, onFileDrop, openTextFile, readTextFile } from '../lib/desktop'
import { inferImportFileFormat, unsupportedImportFileMessage } from './importFile'

interface Props {
  isOpen: boolean
  onClose: () => void
  mermaidEnabled?: boolean
  isImporting?: boolean
  onImport: (parsed: ParsedImport) => Promise<void> | void
  getImportWarnings?: (parsed: ParsedImport) => Promise<string[]> | string[]
}

type Format = 'mermaid' | 'structurizr'

const MERMAID_PLACEHOLDER = `flowchart LR
  A[Start] --> B[End]`

const STRUCTURIZR_PLACEHOLDER = `workspace {
  model {
    user = person "User"
    app = softwareSystem "App"
    user -> app "Uses"
  }
}`

function ImportModal({ isOpen, onClose, mermaidEnabled = false, isImporting, onImport, getImportWarnings }: Props) {
  const [code, setCode] = useState('')
  const [format, setFormat] = useState<Format>(() => mermaidEnabled ? 'mermaid' : 'structurizr')
  const [step, setStep] = useState<'input' | 'summary'>('input')
  const [parsed, setParsed] = useState<ParsedImport | null>(null)
  const [importWarnings, setImportWarnings] = useState<string[]>([])
  const [parseError, setParseError] = useState<string | null>(null)
  const [isParsing, setIsParsing] = useState(false)
  const [isOpeningFile, setIsOpeningFile] = useState(false)
  const summaryWarnings = parsed ? [...parsed.warnings, ...importWarnings] : []

  useEffect(() => {
    if (!isOpen) return
    setCode('')
    setFormat(mermaidEnabled ? 'mermaid' : 'structurizr')
    setStep('input')
    setParsed(null)
    setImportWarnings([])
    setParseError(null)
  }, [isOpen, mermaidEnabled])

  const handleTabChange = (index: number) => {
    setFormat(mermaidEnabled && index === 0 ? 'mermaid' : 'structurizr')
    setCode('')
    setImportWarnings([])
    setParseError(null)
  }

  const loadFileContent = useCallback((path: string, content: string) => {
    const message = unsupportedImportFileMessage(path)
    if (message) {
      setParseError(message)
      return
    }
    const nextFormat = inferImportFileFormat(path)
    if (nextFormat === 'mermaid' && !mermaidEnabled) {
      setParseError('Mermaid Markdown import is experimental. Enable it in settings to import Markdown Mermaid blocks.')
      return
    }
    setFormat(nextFormat === 'structurizr' ? 'structurizr' : 'mermaid')
    setCode(content)
    setStep('input')
    setParsed(null)
    setImportWarnings([])
    setParseError(null)
  }, [mermaidEnabled])

  const handleOpenFile = useCallback(async () => {
    if (!isWailsApp) return
    setIsOpeningFile(true)
    try {
      const result = await openTextFile(mermaidImportFilters)
      if (result.canceled) return
      loadFileContent(result.path, result.content)
    } catch (error) {
      setParseError(error instanceof Error ? error.message : 'Failed to open file')
    } finally {
      setIsOpeningFile(false)
    }
  }, [loadFileContent])

  useEffect(() => {
    if (!isOpen || !isWailsApp) return undefined
    return onFileDrop((_x, _y, paths) => {
      const path = paths[0]
      if (!path) return
      void (async () => {
        setIsOpeningFile(true)
        try {
          const result = await readTextFile(path)
          loadFileContent(result.path, result.content)
        } catch (error) {
          setParseError(error instanceof Error ? error.message : 'Failed to open dropped file')
        } finally {
          setIsOpeningFile(false)
        }
      })()
    }) ?? undefined
  }, [isOpen, loadFileContent])

  const handleNext = async () => {
    if (!code.trim()) return
    setParseError(null)
    setImportWarnings([])

    if (format === 'mermaid') {
      const mermaidSource = extractMermaidCode(code) ?? code.trim()
      const sourceLimitError = mermaidImportSourceLimitError(mermaidSource)
      if (sourceLimitError) {
        setParseError(sourceLimitError)
        return
      }
      setIsParsing(true)
      try {
        const result = await parseMermaidAsync(code)
        const error = parsedMermaidImportError(result)
        if (error) {
          setParseError(error)
          return
        }
        const limitError = parsedMermaidImportLimitError(result)
        if (limitError) {
          setParseError(limitError)
          return
        }
        setImportWarnings(getImportWarnings ? await getImportWarnings(result) : [])
        setParsed(result)
        setStep('summary')
      } finally {
        setIsParsing(false)
      }
      return
    }

    // Structurizr: parse server-side
    setIsParsing(true)
    try {
      const res = await api.import.parseStructurizr(code)
      const result: ParsedImport = {
        elements: res.elements as PlanElement[],
        connectors: res.connectors as PlanConnector[],
        warnings: res.warnings,
        direction: 'LR',
        source: code,
      }
      setImportWarnings(getImportWarnings ? await getImportWarnings(result) : [])
      setParsed(result)
      setStep('summary')
    } catch (e: unknown) {
      setParseError(e instanceof Error ? e.message : 'Failed to parse Structurizr DSL')
    } finally {
      setIsParsing(false)
    }
  }

  const handleSubmit = async () => {
    if (!parsed) return
    await onImport(parsed)
  }

  return (
    <Modal isOpen={isOpen} onClose={onClose} size="xl" isCentered>
      <ModalOverlay bg="blackAlpha.700" backdropFilter="blur(4px)" />
      <ModalContent mx={4} data-testid="import-modal">
        <ModalHeader>{step === 'input' ? 'Import Diagram' : 'Confirm Import'}</ModalHeader>
        <ModalCloseButton />
        <ModalBody>
          <VStack spacing={4} align="stretch">
            {step === 'input' && isWailsApp && (
              <HStack justify="flex-end">
                <Button size="sm" variant="outline" onClick={handleOpenFile} isLoading={isOpeningFile}>
                  Open File
                </Button>
              </HStack>
            )}
            {step === 'input' ? (
              <Tabs index={mermaidEnabled && format === 'mermaid' ? 0 : mermaidEnabled ? 1 : 0} onChange={handleTabChange} size="sm" variant="enclosed">
                <TabList>
                  {mermaidEnabled && <Tab>Mermaid Markdown</Tab>}
                  <Tab>Structurizr DSL</Tab>
                </TabList>
                <TabPanels>
                  {mermaidEnabled && (
                    <TabPanel px={0} pb={0}>
                      <FormControl>
                        <FormLabel fontSize="sm">Markdown Mermaid block</FormLabel>
                        <Textarea
                          data-testid="import-mermaid-textarea"
                          value={code}
                          onChange={(e) => setCode(e.target.value)}
                          placeholder={MERMAID_PLACEHOLDER}
                          size="sm"
                          rows={12}
                          fontFamily="mono"
                        />
                        <Text mt={1.5} fontSize="xs" color="gray.400">
                          Supported: flowchart, C4, sequence, class, ER, state, requirement, sankey, pie, git graph, quadrant, mindmap, journey, gantt, timeline, and XY chart.
                        </Text>
                      </FormControl>
                    </TabPanel>
                  )}
                  <TabPanel px={0} pb={0}>
                    <FormControl>
                      <FormLabel fontSize="sm">Structurizr DSL</FormLabel>
                      <Textarea
                        data-testid="import-structurizr-textarea"
                        value={code}
                        onChange={(e) => setCode(e.target.value)}
                        placeholder={STRUCTURIZR_PLACEHOLDER}
                        size="sm"
                        rows={12}
                        fontFamily="mono"
                      />
                      <Text mt={1.5} fontSize="xs" color="gray.400">
                        Paste a Structurizr workspace DSL. Imports people, software systems, containers, and their relationships.
                      </Text>
                    </FormControl>
                  </TabPanel>
                </TabPanels>
              </Tabs>
            ) : (
              <Box fontSize="sm">
                <Text fontWeight="bold" mb={2}>Summary:</Text>
                <VStack align="start" spacing={1} pl={4} mb={4}>
                  <Text>• Elements: {parsed?.elements.length}</Text>
                  <Text>• Connectors: {parsed?.connectors.length}</Text>
                </VStack>
                {summaryWarnings.length > 0 && (
                  <Box p={3} bg="orange.50" color="orange.800" borderRadius="md" mb={4}>
                    <Text fontWeight="bold" fontSize="xs">Warnings:</Text>
                    {summaryWarnings.map((w, i) => (
                      <Text key={i} fontSize="xs">• {w}</Text>
                    ))}
                  </Box>
                )}
                <Divider mb={4} />
                <Text color="gray.500">
                  This will import the resources listed above into your current workspace.
                </Text>
              </Box>
            )}
            {parseError && (
              <Box p={3} bg="red.50" color="red.800" borderRadius="md">
                <Text fontSize="xs">{parseError}</Text>
              </Box>
            )}
          </VStack>
        </ModalBody>

        <ModalFooter gap={2}>
          {step === 'input' ? (
            <>
              <Button data-testid="import-cancel" variant="ghost" size="sm" onClick={onClose}>
                Cancel
              </Button>
              <Button
                size="sm"
                data-testid="import-next"
                colorScheme="blue"
                onClick={handleNext}
                isDisabled={!code.trim()}
                isLoading={isParsing}
              >
                Next
              </Button>
            </>
          ) : (
            <>
              <Button data-testid="import-back" variant="ghost" size="sm" onClick={() => setStep('input')} isDisabled={isImporting}>
                Back
              </Button>
              <Button
                size="sm"
                data-testid="import-confirm"
                colorScheme="green"
                onClick={handleSubmit}
                isLoading={isImporting}
              >
                Confirm & Import
              </Button>
            </>
          )}
        </ModalFooter>
      </ModalContent>
    </Modal>
  )
}

export default memo(ImportModal)
