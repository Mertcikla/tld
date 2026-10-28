import { useEffect, useRef, useState } from 'react'
import { useLocation } from 'react-router-dom'
import { Box, Button, Center, HStack, Spinner, Text } from '@chakra-ui/react'
import { ZUICanvas, type ZUICanvasHandle } from '../components/ZUI'
import { FlowPanel } from '../flows/FlowPanel'
import type { FlowFocus } from '../flows/model'
import { decodeFlowSnapshot, type FlowSnapshot } from '../flows/sharing'

/** A local Explore scene: only URL data, with no workspace queries or editor navigation. */
export default function FlowSnapshotPage() {
  const { hash } = useLocation()
  const [result, setResult] = useState<{ hash: string; snapshot?: FlowSnapshot; error?: string } | null>(null)
  useEffect(() => {
    let active = true
    void decodeFlowSnapshot(hash).then(snapshot => {
      if (active) setResult({ hash, snapshot })
    }).catch((error: unknown) => {
      if (active) setResult({ hash, error: error instanceof Error ? error.message : 'Could not open this flow snapshot.' })
    })
    return () => { active = false }
  }, [hash])
  if (!result || result.hash !== hash) return <Center h="full"><Spinner aria-label="Loading flow snapshot" /></Center>
  if (!result.snapshot) return <Center h="full" px={6}><Box role="alert" textAlign="center"><Text fontWeight="semibold">Could not open flow snapshot</Text><Text mt={2}>{result.error}</Text></Box></Center>
  return <SnapshotScene key={hash} snapshot={result.snapshot} />
}

function SnapshotScene({ snapshot }: { snapshot: FlowSnapshot }) {
  const canvas = useRef<ZUICanvasHandle>(null)
  const [open, setOpen] = useState(true)
  const [focus, setFocus] = useState<FlowFocus | null>(null)
  return <Box position="relative" w="full" h="full" overflow="hidden" bg="var(--bg-canvas)" data-testid="flow-snapshot">
    <ZUICanvas ref={canvas} data={snapshot.data} flowFocus={open ? focus : null} editorLinksEnabled={false}
      crossBranchSettings={{ enabled: false, depth: 1, connectorBudget: 0, connectorPriority: 'external' }} />
    <HStack position="absolute" right={3} bottom={3} spacing={2} data-canvas-overlay="bottom">
      {!open && <Button size="sm" onClick={() => setOpen(true)}>Show flow</Button>}
      <Button size="sm" onClick={() => canvas.current?.fitView()}>Fit view</Button>
    </HStack>
    <FlowPanel open={open} snapshotCatalog={snapshot.catalog} diagrams={snapshot.diagrams} onFocus={setFocus} onClose={() => setOpen(false)} />
  </Box>
}
