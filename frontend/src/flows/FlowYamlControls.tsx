import { useRef, useState } from 'react'
import { Button, HStack, IconButton, Input, Modal, ModalBody, ModalCloseButton, ModalContent, ModalFooter, ModalHeader, ModalOverlay, Text, Textarea, Tooltip } from '@chakra-ui/react'
import { ImportIcon, ExportIcon } from '../components/Icons'
import { exportFlowYaml, FLOW_YAML_MAX_BYTES, importFlowYaml } from './yaml'
import type { FlowDiagram, StoryFlow } from './model'

interface Props {
  flow: StoryFlow | null
  diagram: FlowDiagram
  viewId?: number
  canImport: boolean
  disabled: boolean
  onImport: (flow: StoryFlow) => boolean
}

/** Import is local and creates a draft; the normal Save flow action persists it. */
export function FlowYamlControls({ flow, diagram, viewId, canImport, disabled, onImport }: Props) {
  const [mode, setMode] = useState<'import' | 'export' | null>(null)
  const [source, setSource] = useState('')
  const [error, setError] = useState('')
  const [reading, setReading] = useState(false)
  const readVersion = useRef(0)
  const close = () => { readVersion.current++; setMode(null); setReading(false) }
  const open = (next: 'import' | 'export') => {
    setError(''); setSource(''); setMode(next)
    if (next === 'export' && flow) {
      try { setSource(exportFlowYaml(flow, diagram)) }
      catch (error) { setError(error instanceof Error ? error.message : 'Could not export flow.') }
    }
  }
  async function readFile(file?: File) {
    if (!file) return
    const version = ++readVersion.current
    setError(''); setReading(true)
    try {
      if (file.size > FLOW_YAML_MAX_BYTES) throw new Error('YAML must be at most 1 MiB.')
      const value = await file.text()
      if (version === readVersion.current) setSource(value)
    } catch (error) {
      if (version === readVersion.current) setError(error instanceof Error ? error.message : 'Could not read file.')
    } finally { if (version === readVersion.current) setReading(false) }
  }
  function importDraft() {
    if (!canImport || disabled || !viewId || reading) return
    try {
      const imported = importFlowYaml(source, viewId, diagram)
      if (onImport(imported)) close()
    } catch (error) { setError(error instanceof Error ? error.message : 'Could not import flow.') }
  }
  function download() {
    const url = URL.createObjectURL(new Blob([source], { type: 'application/yaml;charset=utf-8' }))
    const anchor = document.createElement('a')
    anchor.href = url
    anchor.download = `${flow?.name.replace(/[^a-zA-Z0-9_-]+/g, '-').replace(/^-|-$/g, '').slice(0, 80) || 'flow'}.flow.yaml`
    document.body.appendChild(anchor)
    anchor.click()
    anchor.remove()
    setTimeout(() => URL.revokeObjectURL(url), 1000)
  }
  return <>
    <HStack spacing={2}>
      {canImport && viewId && <Tooltip label="Import YAML" openDelay={200} closeOnPointerDown closeOnScroll>
        <span><IconButton size="xs" w={7} h={7} variant="ghost" aria-label="Import YAML" icon={<ImportIcon size={14} />} isDisabled={disabled} onClick={() => open('import')} /></span>
      </Tooltip>}
      {flow && <Tooltip label="Export YAML" openDelay={200} closeOnPointerDown closeOnScroll>
        <span><IconButton size="xs" w={7} h={7} variant="ghost" aria-label="Export YAML" icon={<ExportIcon size={14} />} isDisabled={disabled} onClick={() => open('export')} /></span>
      </Tooltip>}
    </HStack>
    <Modal isOpen={mode !== null} onClose={close} size="xl">
      <ModalOverlay />
      <ModalContent bg="var(--bg-panel)" mx={3}>
        <ModalHeader>{mode === 'import' ? 'Import flow YAML' : 'Export flow YAML'}</ModalHeader>
        <ModalCloseButton />
        <ModalBody>
          {mode === 'import' && <>
            <Text fontSize="sm" mb={3}>Choose a file or paste YAML. Elements and connections must match this diagram. Import creates a new draft; use Save flow to keep it.</Text>
            <Input type="file" aria-label="Flow YAML file" accept=".yaml,.yml,application/yaml,text/yaml" p={1} mb={3} isDisabled={reading} onChange={event => { void readFile(event.target.files?.[0]); event.target.value = '' }} />
          </>}
          {error && <Text role="alert" color="orange.300" fontSize="sm" mb={2}>{error}</Text>}
          <Textarea aria-label="Flow YAML" fontFamily="mono" fontSize="xs" minH="300px" spellCheck={false} value={source} isReadOnly={mode === 'export'} isDisabled={reading} onChange={event => { setSource(event.target.value); setError('') }} />
        </ModalBody>
        <ModalFooter gap={2}>
          <Button size="sm" variant="ghost" onClick={close}>Close</Button>
          {mode === 'import'
            ? <Button size="sm" variant="primary" isLoading={reading} isDisabled={!source.trim() || disabled || !canImport} onClick={importDraft}>Import as draft</Button>
            : <Button size="sm" variant="primary" isDisabled={!source || !!error} onClick={download}>Download YAML</Button>}
        </ModalFooter>
      </ModalContent>
    </Modal>
  </>
}
