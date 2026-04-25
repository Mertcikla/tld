import { useState } from 'react'
import {
  Box,
  Button,
  Collapse,
  FormControl,
  FormLabel,
  Grid,
  HStack,
  NumberDecrementStepper,
  NumberIncrementStepper,
  NumberInput,
  NumberInputField,
  NumberInputStepper,
  Select,
  Text,
  VStack,
  Icon,
} from '@chakra-ui/react'
import { ChevronDownIcon, ChevronRightIcon } from './Icons'
import { api } from '../api/client'
import type { ViewTreeNode } from '../types'

type Algorithm = 'dagre' | 'force'

interface DagreConfig {
  direction: 'TB' | 'BT' | 'LR' | 'RL'
  nodeSpacing: number
  layerSpacing: number
}

interface ForceConfig {
  linkDistance: number
  chargeStrength: number
  collideRadius: number
  iterations: number
}

const NODE_W = 200
const NODE_H = 120

const ALGO_META: Record<Algorithm, { label: string }> = {
  dagre: { label: 'Layered' },
  force: { label: 'Organic' },
}

interface Props {
  view: ViewTreeNode | null
  canEdit: boolean
}

export default function LayoutSection({ view, canEdit }: Props) {
  const [open, setOpen] = useState(false)
  const [algo, setAlgo] = useState<Algorithm>('dagre')
  const [running, setRunning] = useState(false)
  const [collisionRunning, setCollisionRunning] = useState(false)

  const [dagreConfig, setDagreConfig] = useState<DagreConfig>({
    direction: 'TB',
    nodeSpacing: 75,
    layerSpacing: 75,
  })

  const [forceConfig, setForceConfig] = useState<ForceConfig>({
    linkDistance: 180,
    chargeStrength: -150,
    collideRadius: 130,
    iterations: 300,
  })

  const handleCollisionRemoval = async () => {
    if (!canEdit || !view) return
    setCollisionRunning(true)
    try {
      const [objs, edgeList] = await Promise.all([
        api.workspace.views.placements.list(view.id),
        api.workspace.connectors.list(view.id),
      ])

      const WIDTH = NODE_W
      const HEIGHT = NODE_H
      const PADDING = 40

      const newPositions = new Map<number, { x: number; y: number }>()
      objs.forEach((o) => newPositions.set(o.element_id, { x: o.position_x, y: o.position_y }))

      for (let pass = 0; pass < 3; pass++) {
        for (let j = 0; j < objs.length; j++) {
          for (let k = j + 1; k < objs.length; k++) {
            const a = objs[j]
            const b = objs[k]
            const posA = newPositions.get(a.element_id)!
            const posB = newPositions.get(b.element_id)!

            const dx = posB.x + WIDTH / 2 - (posA.x + WIDTH / 2)
            const dy = posB.y + HEIGHT / 2 - (posA.y + HEIGHT / 2)
            const adx = Math.abs(dx)
            const ady = Math.abs(dy)

            const minX = WIDTH + PADDING
            const minY = HEIGHT + PADDING

            if (adx < minX && ady < minY) {
              const pushX = (minX - adx) / 2
              const pushY = (minY - ady) / 2
              const factorX = dx >= 0 ? 1 : -1
              const factorY = dy >= 0 ? 1 : -1

              posA.x -= pushX * factorX
              posA.y -= pushY * factorY
              posB.x += pushX * factorX
              posB.y += pushY * factorY
            }
          }
        }
      }

      await Promise.all(
        objs.map((obj) => {
          const pos = newPositions.get(obj.element_id)!
          return api.workspace.views.placements.updatePosition(
            view.id,
            obj.element_id,
            Math.round(pos.x),
            Math.round(pos.y)
          )
        })
      )

      const handleUpdates = []
      for (const edge of edgeList) {
        const sPos = newPositions.get(edge.source_element_id)
        const tPos = newPositions.get(edge.target_element_id)
        if (!sPos || !tPos) continue

        const sourceHandles: Record<string, { x: number; y: number }> = {
          top: { x: sPos.x + WIDTH / 2, y: sPos.y },
          bottom: { x: sPos.x + WIDTH / 2, y: sPos.y + HEIGHT },
          left: { x: sPos.x, y: sPos.y + HEIGHT / 2 },
          right: { x: sPos.x + WIDTH, y: sPos.y + HEIGHT / 2 },
        }

        const targetHandles: Record<string, { x: number; y: number }> = {
          top: { x: tPos.x + WIDTH / 2, y: tPos.y },
          bottom: { x: tPos.x + WIDTH / 2, y: tPos.y + HEIGHT },
          left: { x: tPos.x, y: tPos.y + HEIGHT / 2 },
          right: { x: tPos.x + WIDTH, y: tPos.y + HEIGHT / 2 },
        }

        let minDist = Infinity
        let bestSource = edge.source_handle || 'top'
        let bestTarget = edge.target_handle || 'top'

        for (const [sId, sCoord] of Object.entries(sourceHandles)) {
          for (const [tId, tCoord] of Object.entries(targetHandles)) {
            const dist = Math.sqrt((sCoord.x - tCoord.x) ** 2 + (sCoord.y - tCoord.y) ** 2)
            if (dist < minDist) {
              minDist = dist
              bestSource = sId
              bestTarget = tId
            }
          }
        }

        if (bestSource !== edge.source_handle || bestTarget !== edge.target_handle) {
          handleUpdates.push(api.workspace.connectors.update(view.id, edge.id, {
            source_element_id: edge.source_element_id,
            target_element_id: edge.target_element_id,
            source_handle: bestSource,
            target_handle: bestTarget,
            label: edge.label || undefined,
            description: edge.description || undefined,
            relationship: edge.relationship || undefined,
            direction: edge.direction || undefined,
            style: edge.style === 'default' ? 'bezier' : (edge.style || 'bezier'),
            url: edge.url || undefined,
          }))
        }
      }

      await Promise.all(handleUpdates)
      window.location.reload()
    } catch (err) {
      console.error('Collision removal failed:', err)
    } finally {
      setCollisionRunning(false)
    }
  }

  const applyLayout = async () => {
    if (!view || !canEdit) return
    setRunning(true)
    try {
      const [objs, edgeList] = await Promise.all([
        api.workspace.views.placements.list(view.id),
        api.workspace.connectors.list(view.id),
      ])

      let positions: Map<number, { x: number; y: number }>
      if (algo === 'dagre') {
        positions = await runDagre(objs, edgeList)
      } else {
        positions = await runForce(objs, edgeList)
      }

      await Promise.all(
        objs.map(obj => {
          const pos = positions.get(obj.element_id) ?? { x: obj.position_x, y: obj.position_y }
          return api.workspace.views.placements.updatePosition(
            view.id, obj.element_id, Math.round(pos.x), Math.round(pos.y)
          )
        })
      )

      // Re-optimise connector handle directions to match new positions
      const handleUpdates = []
      for (const edge of edgeList) {
        const sPos = positions.get(edge.source_element_id)
        const tPos = positions.get(edge.target_element_id)
        if (!sPos || !tPos) continue

        const srcHandles: Record<string, { x: number; y: number }> = {
          top: { x: sPos.x + NODE_W / 2, y: sPos.y },
          bottom: { x: sPos.x + NODE_W / 2, y: sPos.y + NODE_H },
          left: { x: sPos.x, y: sPos.y + NODE_H / 2 },
          right: { x: sPos.x + NODE_W, y: sPos.y + NODE_H / 2 },
        }
        const tgtHandles: Record<string, { x: number; y: number }> = {
          top: { x: tPos.x + NODE_W / 2, y: tPos.y },
          bottom: { x: tPos.x + NODE_W / 2, y: tPos.y + NODE_H },
          left: { x: tPos.x, y: tPos.y + NODE_H / 2 },
          right: { x: tPos.x + NODE_W, y: tPos.y + NODE_H / 2 },
        }

        let minDist = Infinity
        let bestSrc = edge.source_handle || 'top'
        let bestTgt = edge.target_handle || 'top'

        for (const [sId, sC] of Object.entries(srcHandles)) {
          for (const [tId, tC] of Object.entries(tgtHandles)) {
            const d = Math.sqrt((sC.x - tC.x) ** 2 + (sC.y - tC.y) ** 2)
            if (d < minDist) { minDist = d; bestSrc = sId; bestTgt = tId }
          }
        }

        if (bestSrc !== edge.source_handle || bestTgt !== edge.target_handle) {
          handleUpdates.push(api.workspace.connectors.update(view.id, edge.id, {
            source_element_id: edge.source_element_id,
            target_element_id: edge.target_element_id,
            source_handle: bestSrc,
            target_handle: bestTgt,
            label: edge.label || undefined,
            description: edge.description || undefined,
            relationship: edge.relationship || undefined,
            direction: edge.direction || undefined,
            style: edge.style === 'default' ? 'bezier' : (edge.style || 'bezier'),
            url: edge.url || undefined,
          }))
        }
      }

      await Promise.all(handleUpdates)
      window.location.reload()
    } catch (err) {
      console.error('Layout failed:', err)
    } finally {
      setRunning(false)
    }
  }

  // eslint-disable-next-line @typescript-eslint/no-explicit-any
  const runDagre = async (objs: any[], edgeList: any[]) => {
    // eslint-disable-next-line @typescript-eslint/no-explicit-any
    const dagreModule = await import('dagre') as any
    const dagre = dagreModule.default ?? dagreModule

    const objSet = new Set<number>(objs.map((o: { element_id?: number }) => Number(o.element_id)))
    const graph = new dagre.graphlib.Graph({ multigraph: true })
    graph.setGraph({
      rankdir: dagreConfig.direction,
      nodesep: dagreConfig.nodeSpacing,
      ranksep: dagreConfig.layerSpacing,
      marginx: 0,
      marginy: 0,
    })
    graph.setDefaultEdgeLabel(() => ({}))

    objs.forEach((obj: { element_id: number }) => {
      graph.setNode(String(obj.element_id), { width: NODE_W, height: NODE_H })
    })

    edgeList
      .filter((e: { source_element_id: number; target_element_id: number }) =>
        objSet.has(e.source_element_id) && objSet.has(e.target_element_id)
      )
      .forEach((e: { id: number; source_element_id: number; target_element_id: number }) => {
        graph.setEdge(String(e.source_element_id), String(e.target_element_id), {}, String(e.id))
      })

    dagre.layout(graph)

    const positions = new Map<number, { x: number; y: number }>()
    graph.nodes().forEach((nodeId: string) => {
      const id = Number(nodeId)
      if (!Number.isFinite(id)) return
      const node = graph.node(nodeId) as { x?: number; y?: number }
      positions.set(id, {
        x: (node.x ?? 0) - NODE_W / 2,
        y: (node.y ?? 0) - NODE_H / 2,
      })
    })
    return positions
  }

  // eslint-disable-next-line @typescript-eslint/no-explicit-any
  const runForce = async (objs: any[], edgeList: any[]) => {
    const d3 = await import('d3-force')

    const nodes = objs.map((obj: { element_id: number; position_x: number; position_y: number }) => ({
      id: obj.element_id,
      x: obj.position_x + NODE_W / 2,
      y: obj.position_y + NODE_H / 2,
    }))
    const nodeIds = new Set(nodes.map(n => n.id))
    const links = edgeList
      .filter((e: { source_element_id: number; target_element_id: number }) =>
        nodeIds.has(e.source_element_id) && nodeIds.has(e.target_element_id)
      )
      .map((e: { source_element_id: number; target_element_id: number }) => ({
        source: e.source_element_id,
        target: e.target_element_id,
      }))

    // eslint-disable-next-line @typescript-eslint/no-explicit-any
    const sim = d3.forceSimulation(nodes as any)
      // eslint-disable-next-line @typescript-eslint/no-explicit-any
      .force('link', d3.forceLink(links as any).id((d: any) => d.id).distance(forceConfig.linkDistance))
      .force('charge', d3.forceManyBody().strength(forceConfig.chargeStrength))
      .force('center', d3.forceCenter(0, 0))
      .force('collide', d3.forceCollide(forceConfig.collideRadius))
      .stop()

    for (let i = 0; i < forceConfig.iterations; i++) sim.tick()

    const positions = new Map<number, { x: number; y: number }>()
    // eslint-disable-next-line @typescript-eslint/no-explicit-any
    nodes.forEach((n: any) => {
      positions.set(n.id, { x: (n.x ?? 0) - NODE_W / 2, y: (n.y ?? 0) - NODE_H / 2 })
    })
    return positions
  }

  const LabelStyle = {
    fontSize: '9px',
    color: 'whiteAlpha.600',
    mb: 1.5,
    textTransform: 'uppercase' as const,
    letterSpacing: '0.1em',
    fontWeight: '700',
  }

  return (
    <Box borderTop="1px solid" borderColor="whiteAlpha.100" pt={4}>
      {/* Section header */}
      <HStack
        mb={open ? 4 : 0}
        cursor="pointer"
        onClick={() => setOpen(v => !v)}
        color={open ? 'blue.400' : 'whiteAlpha.700'}
        _hover={{ color: 'blue.300' }}
        transition="all 0.2s cubic-bezier(0.4, 0, 0.2, 1)"
        userSelect="none"
      >
        <Icon
          as={open ? ChevronDownIcon : ChevronRightIcon}
          boxSize={4}
          strokeWidth={3.5}
          transition="transform 0.25s cubic-bezier(0.25, 1, 0.5, 1)"
        />
        <Text
          fontSize="11px"
          fontWeight="800"
          letterSpacing="0.15em"
          textTransform="uppercase"
        >
          Adjust Layout
        </Text>
      </HStack>

      <Collapse in={open} animateOpacity>
        <VStack pb={5} spacing={5} align="stretch">

          {/* Algorithm segmented control */}
          <Box p={1} bg="whiteAlpha.50" borderRadius="xl">
            <HStack spacing={1}>
              {(Object.keys(ALGO_META) as Algorithm[]).map(a => (
                <Box
                  key={a}
                  flex={1}
                  as="button"
                  py={2}
                  fontSize="10px"
                  fontWeight="800"
                  letterSpacing="0.08em"
                  textTransform="uppercase"
                  cursor="pointer"
                  onClick={() => setAlgo(a)}
                  bg={algo === a ? 'whiteAlpha.200' : 'transparent'}
                  color={algo === a ? 'white' : 'whiteAlpha.500'}
                  borderRadius="lg"
                  _hover={{ bg: algo === a ? 'whiteAlpha.200' : 'whiteAlpha.100', color: 'white' }}
                  transition="all 0.2s"
                >
                  {ALGO_META[a].label}
                </Box>
              ))}
            </HStack>
          </Box>

          {/* Parameters grid */}
          <Box
            p={4}
            bg="whiteAlpha.50"
            borderRadius="xl"
            border="1px solid"
            borderColor="whiteAlpha.100"
          >
            {algo === 'dagre' ? (
              <Grid templateColumns="1fr 1fr" gap={4}>
                <FormControl gridColumn="span 2">
                  <FormLabel {...LabelStyle}>Direction</FormLabel>
                  <Select
                    size="xs"
                    variant="filled"
                    bg="whiteAlpha.100"
                    border="none"
                    _hover={{ bg: 'whiteAlpha.200' }}
                    value={dagreConfig.direction}
                    onChange={e => setDagreConfig(c => ({ ...c, direction: e.target.value as DagreConfig['direction'] }))}
                  >
                    <option value="TB">Top → Bottom</option>
                    <option value="BT">Bottom → Top</option>
                    <option value="LR">Left → Right</option>
                    <option value="RL">Right → Left</option>
                  </Select>
                </FormControl>

                <FormControl>
                  <FormLabel {...LabelStyle}>Element Gap</FormLabel>
                  <NumberInput
                    size="xs"
                    variant="filled"
                    value={dagreConfig.nodeSpacing}
                    min={10} max={400} step={10}
                    onChange={(_, v) => !isNaN(v) && setDagreConfig(c => ({ ...c, nodeSpacing: v }))}
                  >
                    <NumberInputField bg="whiteAlpha.100" border="none" />
                    <NumberInputStepper>
                      <NumberIncrementStepper border="none" />
                      <NumberDecrementStepper border="none" />
                    </NumberInputStepper>
                  </NumberInput>
                </FormControl>

                <FormControl>
                  <FormLabel {...LabelStyle}>Layer Gap</FormLabel>
                  <NumberInput
                    size="xs"
                    variant="filled"
                    value={dagreConfig.layerSpacing}
                    min={10} max={400} step={10}
                    onChange={(_, v) => !isNaN(v) && setDagreConfig(c => ({ ...c, layerSpacing: v }))}
                  >
                    <NumberInputField bg="whiteAlpha.100" border="none" />
                    <NumberInputStepper>
                      <NumberIncrementStepper border="none" />
                      <NumberDecrementStepper border="none" />
                    </NumberInputStepper>
                  </NumberInput>
                </FormControl>
              </Grid>
            ) : (
              <Grid templateColumns="1fr 1fr" gap={4}>
                <FormControl>
                  <FormLabel {...LabelStyle}>Distance</FormLabel>
                  <NumberInput
                    size="xs"
                    variant="filled"
                    value={forceConfig.linkDistance}
                    min={50} max={600} step={10}
                    onChange={(_, v) => !isNaN(v) && setForceConfig(c => ({ ...c, linkDistance: v }))}
                  >
                    <NumberInputField bg="whiteAlpha.100" border="none" />
                    <NumberInputStepper>
                      <NumberIncrementStepper border="none" />
                      <NumberDecrementStepper border="none" />
                    </NumberInputStepper>
                  </NumberInput>
                </FormControl>

                <FormControl>
                  <FormLabel {...LabelStyle}>Strength</FormLabel>
                  <NumberInput
                    size="xs"
                    variant="filled"
                    value={forceConfig.chargeStrength}
                    min={-2000} max={-10} step={10}
                    onChange={(_, v) => !isNaN(v) && setForceConfig(c => ({ ...c, chargeStrength: v }))}
                  >
                    <NumberInputField bg="whiteAlpha.100" border="none" />
                    <NumberInputStepper>
                      <NumberIncrementStepper border="none" />
                      <NumberDecrementStepper border="none" />
                    </NumberInputStepper>
                  </NumberInput>
                </FormControl>

                <FormControl>
                  <FormLabel {...LabelStyle}>Radius</FormLabel>
                  <NumberInput
                    size="xs"
                    variant="filled"
                    value={forceConfig.collideRadius}
                    min={50} max={500} step={10}
                    onChange={(_, v) => !isNaN(v) && setForceConfig(c => ({ ...c, collideRadius: v }))}
                  >
                    <NumberInputField bg="whiteAlpha.100" border="none" />
                    <NumberInputStepper>
                      <NumberIncrementStepper border="none" />
                      <NumberDecrementStepper border="none" />
                    </NumberInputStepper>
                  </NumberInput>
                </FormControl>

                <FormControl>
                  <FormLabel {...LabelStyle}>Quality</FormLabel>
                  <NumberInput
                    size="xs"
                    variant="filled"
                    value={forceConfig.iterations}
                    min={50} max={1000} step={50}
                    onChange={(_, v) => !isNaN(v) && setForceConfig(c => ({ ...c, iterations: v }))}
                  >
                    <NumberInputField bg="whiteAlpha.100" border="none" />
                    <NumberInputStepper>
                      <NumberIncrementStepper border="none" />
                      <NumberDecrementStepper border="none" />
                    </NumberInputStepper>
                  </NumberInput>
                </FormControl>
              </Grid>
            )}
          </Box>
          <Button
            size="sm"
            w="full"
            colorScheme="blue"
            onClick={applyLayout}
            isLoading={running}
            isDisabled={!canEdit || !view}
            loadingText="Applying Layout..."
            fontWeight="bold"
            fontSize="xs"
            letterSpacing="0.05em"
            textTransform="uppercase"
            h="32px"
            transition="all 0.2s"
          >
            Apply Layout
          </Button>
          {/* Apply button */}
          <VStack spacing={2} w="full">
            <Button
              size="sm"
              w="full"
              variant="outline"
              colorScheme="blue"
              onClick={handleCollisionRemoval}
              isLoading={collisionRunning}
              isDisabled={!canEdit || !view}
              loadingText="Removing Connector Collisions..."
              fontWeight="bold"
              fontSize="xs"
              letterSpacing="0.05em"
              textTransform="uppercase"
              h="32px"
              transition="all 0.2s"

            >
              Adjust Connectors
            </Button>

          </VStack>

        </VStack>
      </Collapse>
    </Box>
  )
}
