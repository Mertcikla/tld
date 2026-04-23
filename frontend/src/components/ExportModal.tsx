import { memo, useEffect, useMemo, useState } from 'react'
import {
  Button,
  FormControl,
  FormLabel,
  HStack,
  Input,
  Modal,
  ModalBody,
  ModalCloseButton,
  ModalContent,
  ModalFooter,
  ModalHeader,
  ModalOverlay,
  Radio,
  RadioGroup,
  Text,
  VStack,
} from '@chakra-ui/react'

export type ExportFormat = 'svg' | 'png' | 'mermaid'

export interface ExportOptions {
  format: ExportFormat
  scale: 1 | 2 | 3
  filename: string
}

interface Props {
  isOpen: boolean
  onClose: () => void
  defaultFilename: string
  isExporting?: boolean
  onExport: (options: ExportOptions) => Promise<void> | void
}

function sanitizeFilename(value: string) {
  const trimmed = value.trim()
  if (!trimmed) return 'diagram-export'
  return trimmed.replace(/[\\/:*?"<>|]+/g, '-').replace(/\s+/g, ' ')
}

function ExportModal({
  isOpen,
  onClose,
  defaultFilename,
  isExporting,
  onExport,
}: Props) {
  const [format, setFormat] = useState<ExportFormat>('svg')
  const [scale, setScale] = useState<1 | 2 | 3>(2)
  const [filename, setFilename] = useState(defaultFilename)

  useEffect(() => {
    if (!isOpen) return
    setFilename(defaultFilename)
    setFormat('svg')
    setScale(2)
  }, [isOpen, defaultFilename])

  const extension = useMemo(() => (format === 'svg' ? '.svg' : format === 'png' ? '.png' : '.mermaid'), [format])

  const handleSubmit = async () => {
    await onExport({
      format,
      scale,
      filename: sanitizeFilename(filename),
    })
  }

  return (
    <Modal isOpen={isOpen} onClose={onClose} isCentered>
      <ModalOverlay bg="blackAlpha.700" backdropFilter="blur(4px)" />
      <ModalContent mx={4}>
        <ModalHeader>Export Diagram</ModalHeader>
        <ModalCloseButton />
        <ModalBody>
          <VStack spacing={4} align="stretch">
            <FormControl id="export-format">
              <FormLabel fontSize="sm">Format</FormLabel>
              <RadioGroup name="format" value={format} onChange={(value) => setFormat(value as ExportFormat)}>
                <HStack spacing={4}>
                  <Radio value="svg">SVG</Radio>
                  <Radio value="png">PNG</Radio>
                  <Radio value="mermaid">Mermaid</Radio>
                </HStack>
              </RadioGroup>
            </FormControl>

            {format === 'png' && (
              <FormControl id="export-resolution">
                <FormLabel fontSize="sm">Resolution</FormLabel>
                <RadioGroup name="scale" value={String(scale)} onChange={(value) => setScale(Number(value) as 1 | 2 | 3)}>
                  <HStack spacing={4}>
                    <Radio value="1">1x</Radio>
                    <Radio value="2">2x</Radio>
                    <Radio value="3">3x</Radio>
                  </HStack>
                </RadioGroup>
              </FormControl>
            )}

            <FormControl id="export-filename">
              <FormLabel fontSize="sm">Filename</FormLabel>
              <Input
                name="filename"
                value={filename}
                onChange={(e) => setFilename(e.target.value)}
                placeholder="diagram-export"
                size="sm"
              />
              <Text mt={1.5} fontSize="xs" color="gray.400">
                File extension will be added automatically ({extension})
              </Text>
            </FormControl>
          </VStack>
        </ModalBody>

        <ModalFooter gap={2}>
          <Button variant="ghost" size="sm" onClick={onClose} isDisabled={isExporting}>
            Cancel
          </Button>
          <Button size="sm" colorScheme="blue" onClick={handleSubmit} isLoading={isExporting}>
            Export
          </Button>
        </ModalFooter>
      </ModalContent>
    </Modal>
  )
}

export default memo(ExportModal)
