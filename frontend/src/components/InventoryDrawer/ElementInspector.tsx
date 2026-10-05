import { Box, Flex, Grid, HStack, Text, VStack } from '@chakra-ui/react'
import type { LibraryElement } from '../../types'
import { TYPE_COLORS } from '../../types'
import type { NeighbourNode } from './types'
import { chunkNodes, toCompactLevel } from './utils'
import { RelationshipCard } from './RelationshipCard'
import { ConnectionIndicator } from './ConnectionIndicator'
import { resolveElementIconUrl } from '../../utils/elementIcon'

interface ElementInspectorProps {
  selectedElement: LibraryElement | null | undefined
  neighborGraph: NeighbourNode[]
  graphHeight: number
  cardShadow: string
  accent: string
  onSelectRow: (key: string) => void
}

export function ElementInspector({
  selectedElement,
  neighborGraph,
  graphHeight,
  cardShadow,
  accent,
  onSelectRow,
}: ElementInspectorProps) {
  const leftNodes = neighborGraph.filter((n) => n.position === 'left')
  const rightNodes = neighborGraph.filter((n) => n.position === 'right')
  const topNodes = neighborGraph.filter((n) => n.position === 'top')
  const bottomNodes = neighborGraph.filter((n) => n.position === 'bottom')

  const leftColumns = chunkNodes(leftNodes)
  const rightColumns = chunkNodes(rightNodes)
  const topRows = chunkNodes(topNodes)
  const bottomRows = chunkNodes(bottomNodes)

  const leftColumnSize = Math.max(...leftColumns.map((column) => column.length), 0)
  const rightColumnSize = Math.max(...rightColumns.map((column) => column.length), 0)

  const leftCompactLevel = toCompactLevel(graphHeight > 0 && leftColumnSize > 0 ? graphHeight / leftColumnSize : 999)
  const rightCompactLevel = toCompactLevel(graphHeight > 0 && rightColumnSize > 0 ? graphHeight / rightColumnSize : 999)
  const maxCompactLevel = Math.max(leftCompactLevel, rightCompactLevel, 0)

  const colSpacing = maxCompactLevel >= 3 ? 2 : maxCompactLevel >= 2 ? 3 : maxCompactLevel >= 1 ? 5 : 8
  const nodeSpacing = maxCompactLevel >= 2 ? 1 : maxCompactLevel >= 1 ? 2 : 3

  const logoFor = (element: LibraryElement | null | undefined) =>
    resolveElementIconUrl(element?.logo_url, element?.technology_connectors) ?? undefined

  return (
    <Flex direction="column" align="center">
      {topNodes.length > 0 && (
        <Flex direction="column" align="center">
          <VStack spacing={nodeSpacing} align="center">
            {topRows.map((row, rowIndex) => (
              <HStack key={`top-row-${rowIndex}`} spacing={nodeSpacing} align="flex-end">
                {row.map((node) => (
                  <RelationshipCard
                    key={node.element.id}
                    name={node.element.name}
                    type={node.element.kind || ''}
                    technology={node.element.technology || ''}
                    logoUrl={logoFor(node.element)}
                    borderColor={TYPE_COLORS[node.element.kind || ''] || 'gray'}
                    compactLevel={maxCompactLevel}
                    onClick={() => onSelectRow(`element:${node.element.id}`)}
                  />
                ))}
              </HStack>
            ))}
          </VStack>
          <ConnectionIndicator position="top" compactLevel={maxCompactLevel} />
        </Flex>
      )}

      <Grid templateColumns="1fr auto 1fr" gap={colSpacing} alignItems="center" w="full">
        <Flex justify="flex-end">
          {leftNodes.length > 0 && (
            <Flex gap={nodeSpacing} align="center">
              {leftColumns.map((column, columnIndex) => (
                <VStack key={`left-column-${columnIndex}`} spacing={nodeSpacing} align="flex-end">
                  {column.map((node) => (
                    <RelationshipCard
                      key={node.element.id}
                      name={node.element.name}
                      type={node.element.kind || ''}
                      technology={node.element.technology || ''}
                      logoUrl={logoFor(node.element)}
                      borderColor={TYPE_COLORS[node.element.kind || ''] || 'gray'}
                      compactLevel={leftCompactLevel}
                      onClick={() => onSelectRow(`element:${node.element.id}`)}
                    />
                  ))}
                </VStack>
              ))}
              <ConnectionIndicator position="left" compactLevel={leftCompactLevel} />
            </Flex>
          )}
        </Flex>

        <Box position="relative" zIndex={10} isolation="isolate" data-pan-block="true">
          <RelationshipCard
            testId="inventory-selected-card"
            isSelected
            name={selectedElement?.name || ''}
            type={selectedElement?.kind || ''}
            technology={selectedElement?.technology || ''}
            logoUrl={logoFor(selectedElement)}
            borderColor={accent}
            shadow={cardShadow}
            compactLevel={0}
          />
        </Box>

        <Flex justify="flex-start">
          {rightNodes.length > 0 && (
            <Flex gap={nodeSpacing} align="center">
              <ConnectionIndicator position="right" compactLevel={rightCompactLevel} />
              {rightColumns.map((column, columnIndex) => (
                <VStack key={`right-column-${columnIndex}`} spacing={nodeSpacing} align="flex-start">
                  {column.map((node) => (
                    <RelationshipCard
                      key={node.element.id}
                      name={node.element.name}
                      type={node.element.kind || ''}
                      technology={node.element.technology || ''}
                      logoUrl={logoFor(node.element)}
                      borderColor={TYPE_COLORS[node.element.kind || ''] || 'gray'}
                      compactLevel={rightCompactLevel}
                      onClick={() => onSelectRow(`element:${node.element.id}`)}
                    />
                  ))}
                </VStack>
              ))}
            </Flex>
          )}
        </Flex>
      </Grid>

      {bottomNodes.length > 0 && (
        <Flex direction="column" align="center">
          <ConnectionIndicator position="bottom" compactLevel={maxCompactLevel} />
          <VStack spacing={nodeSpacing} align="center">
            {bottomRows.map((row, rowIndex) => (
              <HStack key={`bottom-row-${rowIndex}`} spacing={nodeSpacing} align="flex-start">
                {row.map((node) => (
                  <RelationshipCard
                    key={node.element.id}
                    name={node.element.name}
                    type={node.element.kind || ''}
                    technology={node.element.technology || ''}
                    logoUrl={logoFor(node.element)}
                    borderColor={TYPE_COLORS[node.element.kind || ''] || 'gray'}
                    compactLevel={maxCompactLevel}
                    onClick={() => onSelectRow(`element:${node.element.id}`)}
                  />
                ))}
              </HStack>
            ))}
          </VStack>
        </Flex>
      )}

      {neighborGraph.length === 0 && (
        <Text color="gray.600" fontSize="sm" fontStyle="italic" mt={2} data-pan-block="true">
          No direct connections found.
        </Text>
      )}
    </Flex>
  )
}
