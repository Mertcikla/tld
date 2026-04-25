import { memo, useEffect, useState } from 'react'
import {
  Button,
  FormControl,
  FormLabel,
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
import { parseMermaid, ParsedImport } from '../pkg/importer/mermaid'
import { api } from '../api/client'
import type { PlanConnector, PlanElement } from '@buf/tldiagramcom_diagram.bufbuild_es/diag/v1/workspace_service_pb'

interface Props {
  isOpen: boolean
  onClose: () => void
  isImporting?: boolean
  onImport: (parsed: ParsedImport) => Promise<void> | void
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

function ImportModal({ isOpen, onClose, isImporting, onImport }: Props) {
  const [code, setCode] = useState('')
  const [format, setFormat] = useState<Format>('mermaid')
  const [step, setStep] = useState<'input' | 'summary'>('input')
  const [parsed, setParsed] = useState<ParsedImport | null>(null)
  const [parseError, setParseError] = useState<string | null>(null)
  const [isParsing, setIsParsing] = useState(false)

  useEffect(() => {
    if (!isOpen) return
    setCode('')
    setStep('input')
    setParsed(null)
    setParseError(null)
  }, [isOpen])

  const handleTabChange = (index: number) => {
    setFormat(index === 0 ? 'mermaid' : 'structurizr')
    setCode('')
    setParseError(null)
  }

  const handleNext = async () => {
    if (!code.trim()) return
    setParseError(null)

    if (format === 'mermaid') {
      const result = parseMermaid(code)
      setParsed(result)
      setStep('summary')
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
      }
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
            {step === 'input' ? (
              <Tabs onChange={handleTabChange} size="sm" variant="enclosed">
                <TabList>
                  <Tab>Mermaid</Tab>
                  <Tab>Structurizr DSL</Tab>
                </TabList>
                <TabPanels>
                  <TabPanel px={0} pb={0}>
                    <FormControl>
                      <FormLabel fontSize="sm">Mermaid code</FormLabel>
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
                        Supported: flowchart / graph, C4Context.
                      </Text>
                    </FormControl>
                  </TabPanel>
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
                {parsed?.warnings && parsed.warnings.length > 0 && (
                  <Box p={3} bg="orange.50" color="orange.800" borderRadius="md" mb={4}>
                    <Text fontWeight="bold" fontSize="xs">Warnings:</Text>
                    {parsed.warnings.map((w, i) => (
                      <Text key={i} fontSize="xs">• {w}</Text>
                    ))}
                  </Box>
                )}
                <Divider mb={4} />
                <Text color="gray.500">
                  This will create the resources listed above in your current workspace.
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
