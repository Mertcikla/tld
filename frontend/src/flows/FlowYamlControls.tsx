import { useRef, useState } from 'react'
import { useHref } from 'react-router-dom'
import { Button, Checkbox, HStack, IconButton, Input, Modal, ModalBody, ModalCloseButton, ModalContent, ModalFooter, ModalHeader, ModalOverlay, Text, Textarea, Tooltip } from '@chakra-ui/react'
import { ImportIcon, ExportIcon } from '../components/Icons'
import { faShareFromSquare } from '@fortawesome/free-solid-svg-icons'
import FontAwesomeIcon from '../components/FontAwesomeIcon'
import { copyTextToClipboard } from '../utils/clipboard'
import { toast } from '../utils/toast'
import { exportCompleteFlowYaml, exportFlowYaml, FLOW_YAML_MAX_BYTES, importFlowYaml, parseFlowYaml, type PortableFlowDocument } from './yaml'
import { createFlowSnapshotLink, exportFlowSnapshotYaml, FLOW_SNAPSHOT_TOKEN, loadFlowSnapshotDiagrams, WEB_FLOW_SNAPSHOT_URL } from './sharing'
import { isWailsApp } from '../config/runtime'
import { allSteps, type FlowCatalog, type FlowDiagram, type StoryFlow } from './model'

interface Props {
  catalog?: FlowCatalog
  flow: StoryFlow | null
  diagrams?: Record<string, FlowDiagram>
  loadDiagram?: (viewId: number) => Promise<FlowDiagram>
  diagram: FlowDiagram
  viewId?: number
  canImport: boolean
  disabled: boolean
  onImport: (flow: StoryFlow) => boolean
  onImportComplete?: (source: string) => Promise<boolean>
}

/** Default import only creates a draft. Bundled resources are created after confirmation. */
export function FlowYamlControls({ catalog, flow, diagram, diagrams, loadDiagram, viewId, canImport, disabled, onImport, onImportComplete }: Props) {
  const sharePath = useHref(`/explore/shared/${FLOW_SNAPSHOT_TOKEN}`)
  const [mode, setMode] = useState<'import' | 'export' | 'share' | null>(null)
  const [source, setSource] = useState('')
  const [error, setError] = useState('')
  const [complete, setComplete] = useState(false)
  const [confirmation, setConfirmation] = useState<PortableFlowDocument | null>(null)
  const [importing, setImporting] = useState(false)
  const [reading, setReading] = useState(false)
  const readVersion = useRef(0)
  const close = () => { if (importing) return; readVersion.current++; setMode(null); setReading(false); setConfirmation(null) }
  const open = (next: 'import' | 'export') => {
    readVersion.current++
    setError(''); setSource(''); setComplete(false); setConfirmation(null); setMode(next)
    if (next === 'export' && flow) {
      try { setSource(exportFlowYaml(flow, diagram, catalog)) }
      catch (error) { setError(error instanceof Error ? error.message : 'Could not export flow.') }
    }
  }
  async function share() {
    if (!flow || disabled) return
    const version = ++readVersion.current
    setMode('share'); setSource(''); setError(''); setReading(true); setConfirmation(null)
    try {
      const snapshotDiagrams = await loadFlowSnapshotDiagrams(flow, diagram, catalog, diagrams, loadDiagram)
      const yaml = exportFlowSnapshotYaml(flow, diagram, catalog, snapshotDiagrams)
      const baseUrl = isWailsApp || window.__TLD_VSCODE__ ? WEB_FLOW_SNAPSHOT_URL : new URL(sharePath, window.location.href).href
      const link = await createFlowSnapshotLink(baseUrl, yaml)
      if (version === readVersion.current) setSource(link)
    } catch (error) {
      if (version === readVersion.current) setError(error instanceof Error ? error.message : 'Could not share flow.')
    } finally { if (version === readVersion.current) setReading(false) }
  }
  async function copyLink() {
    if (!source || reading || error) return
    try {
      await copyTextToClipboard(source)
      toast({ status: 'success', title: 'Copied flow link' })
    } catch {
      toast({ status: 'error', title: 'Copy failed', description: 'Select the link and copy it manually.' })
    }
  }
  function toggleComplete(checked: boolean) {
    setComplete(checked); setError(''); setSource('')
    if (!flow) return
    try { setSource(checked ? exportCompleteFlowYaml(flow, diagram, catalog) : exportFlowYaml(flow, diagram, catalog)) }
    catch (error) { setError(error instanceof Error ? error.message : 'Could not export flow.') }
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
      const doc = parseFlowYaml(source)
      if (doc.resources) { setConfirmation(doc); return }
      const imported = importFlowYaml(source, viewId, diagram, {}, catalog)
      if (onImport(imported)) close()
    } catch (error) { setError(error instanceof Error ? error.message : 'Could not import flow.') }
  }
  async function importComplete() {
    if (!onImportComplete || !canImport || disabled || importing) return
    setImporting(true); setError('')
    try {
      if (await onImportComplete(source)) {
        readVersion.current++; setMode(null); setReading(false); setConfirmation(null)
      }
    } catch (error) { setError(error instanceof Error ? error.message : 'Could not import complete flow.') }
    finally { setImporting(false) }
  }
  function importExisting() {
    if (!canImport || disabled || importing || !viewId) return
    try { if (onImport(importFlowYaml(source, viewId, diagram, {}, catalog))) close() }
    catch (error) { setError(error instanceof Error ? error.message : 'Could not import flow.') }
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
  async function copyYaml() {
    if (mode !== 'export' || !source || error) return
    try {
      await copyTextToClipboard(source)
      toast({ status: 'success', title: 'Copied flow YAML' })
    } catch {
      toast({ status: 'error', title: 'Copy failed', description: 'Could not copy flow YAML. Select the text and copy it manually.' })
    }
  }
  return <>
    <HStack spacing={2}>
      {canImport && viewId && <Tooltip label="Import YAML" openDelay={200} closeOnPointerDown closeOnScroll>
        <span><IconButton size="xs" w={7} h={7} variant="ghost" aria-label="Import YAML" icon={<ImportIcon size={14} />} isDisabled={disabled} onClick={() => open('import')} /></span>
      </Tooltip>}
      {flow && <Tooltip label="Export YAML" openDelay={200} closeOnPointerDown closeOnScroll>
        <span><IconButton size="xs" w={7} h={7} variant="ghost" aria-label="Export YAML" icon={<ExportIcon size={14} />} isDisabled={disabled} onClick={() => open('export')} /></span>
      </Tooltip>}
      {flow && <Tooltip label="Share flow" openDelay={200} closeOnPointerDown closeOnScroll>
        <span><IconButton size="xs" w={7} h={7} variant="ghost" aria-label="Share flow" icon={<FontAwesomeIcon icon={faShareFromSquare} size={14} />} isDisabled={disabled} onClick={() => void share()} /></span>
      </Tooltip>}
    </HStack>
    <Modal isOpen={mode !== null} onClose={close} size="xl" closeOnOverlayClick={!importing} closeOnEsc={!importing}>
      <ModalOverlay />
      <ModalContent bg="var(--bg-panel)" mx={3}>
        <ModalHeader>{mode === 'import' ? 'Import flow YAML' : mode === 'share' ? 'Share flow' : 'Export flow YAML'}</ModalHeader>
        <ModalCloseButton isDisabled={importing} />
        <ModalBody>
          {mode === 'share' && <>
            <Text fontSize="sm" mb={3}>Anyone with this link can explore a snapshot of this flow and its included elements and connectors. Later edits won’t change the snapshot.</Text>
            {reading && <Text role="status" fontSize="sm">Creating flow link…</Text>}
            <Input aria-label="Flow share link" value={source} isReadOnly isDisabled={reading} onClick={event => event.currentTarget.select()} />
          </>}
          {mode === 'import' && !confirmation && <>
            <Text fontSize="sm" mb={3}>Choose a file or paste YAML. Elements and connections must match this diagram. Import creates a new draft; use Save flow to keep it.</Text>
            <Input type="file" aria-label="Flow YAML file" accept=".yaml,.yml,application/yaml,text/yaml" p={1} mb={3} isDisabled={reading} onChange={event => { void readFile(event.target.files?.[0]); event.target.value = '' }} />
          </>}
          {mode === 'export' && <Checkbox mb={3} isChecked={complete} onChange={event => toggleComplete(event.target.checked)}>Complete export (include required elements and connectors)</Checkbox>}
          {mode === 'export' && complete && flow && allSteps(flow.steps).some(step => step.goToFlowId) && <Text fontSize="sm" mb={3}>Linked flows must already exist in the destination workspace.</Text>}
          {confirmation && <>
            <Text fontSize="sm" mb={3}>This flow includes {confirmation.resources!.elements.length} elements and {confirmation.resources!.connectors.length} connectors. Create them in a new element’s view under Root named “{confirmation.name}”? The flow will open as a draft; use Save flow to keep it.</Text>
            {!onImportComplete && <Text fontSize="sm" mb={3}>Open this flow in the diagram editor to create a new view.</Text>}
          </>}
          {error && <Text role="alert" color="orange.300" fontSize="sm" mb={2}>{error}</Text>}
          {mode !== 'share' && !confirmation && <Textarea aria-label="Flow YAML" fontFamily="mono" fontSize="xs" minH="300px" spellCheck={false} value={source} isReadOnly={mode === 'export'} isDisabled={reading}
            onClick={event => { if (mode === 'export') { event.currentTarget.select(); void copyYaml() } }}
            onChange={event => { setSource(event.target.value); setError('') }} />}
        </ModalBody>
        <ModalFooter gap={2} flexWrap="wrap">
          <Button size="sm" variant="ghost" isDisabled={importing} onClick={close}>Close</Button>
          {mode === 'share' ? <Button size="sm" variant="primary" isLoading={reading} isDisabled={!source || !!error} onClick={() => void copyLink()}>Copy link</Button> : mode === 'import'
            ? confirmation ? <>
              <Button size="sm" isDisabled={importing || disabled} onClick={() => { setConfirmation(null); setError('') }}>Back</Button>
              <Button size="sm" isDisabled={importing || disabled || !canImport} onClick={importExisting}>Use existing diagram</Button>
              <Button size="sm" variant="primary" isLoading={importing} isDisabled={disabled || !canImport || !onImportComplete} onClick={() => void importComplete()}>Create new view</Button>
            </> : <Button size="sm" variant="primary" isLoading={reading} isDisabled={!source.trim() || disabled || !canImport} onClick={importDraft}>Import as draft</Button>
            : <>
              <Button size="sm" isDisabled={!source || !!error} onClick={() => void copyYaml()}>Copy YAML</Button>
              <Button size="sm" variant="primary" isDisabled={!source || !!error} onClick={download}>Download YAML</Button>
            </>}
        </ModalFooter>
      </ModalContent>
    </Modal>
  </>
}
