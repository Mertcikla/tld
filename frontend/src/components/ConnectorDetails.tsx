import { Box, Button, HStack, Text, VStack } from '@chakra-ui/react'
import type { ComponentProps, ReactNode } from 'react'
import { Link as RouterLink } from 'react-router-dom'
import type { ProxyConnectorDetails, ProxyConnectorLeaf } from '../crossBranch/types'
import { canvasPopoverInsetCardStyles, canvasPopoverNameButtonStyles } from '../styles/surfaces'
import { EditorIcon } from './Icons'

function directionGlyph(direction?: string): string {
  switch (direction) {
    case 'backward':
      return '←'
    case 'both':
    case 'bidirectional':
      return '↔'
    case 'merged':
    case 'none':
      return '—'
    case 'forward':
    default:
      return '→'
  }
}

function isDistinctProxySummaryLabel(label: string | undefined, count: number): boolean {
  const trimmed = label?.trim()
  if (!trimmed) return false
  const lower = trimmed.toLowerCase()
  return (
    lower !== 'cross-view' &&
    lower !== 'off-view connector' &&
    lower !== 'connectors' &&
    lower !== `${count} connector` &&
    lower !== `${count} connectors`
  )
}

export function ConnectorNameButton({
  label,
  to,
  isDisabled = false,
  size = 'sm',
  onNavigate,
}: {
  label: string
  to?: string
  isDisabled?: boolean
  size?: 'sm' | 'md'
  onNavigate?: () => void
}) {
  const canNavigate = Boolean(to) && !isDisabled
  return (
    <Button
      as={canNavigate ? RouterLink : undefined}
      isDisabled={!canNavigate}
      to={canNavigate ? to : undefined}
      size="xs"
      variant="outline"
      sx={{
        ...canvasPopoverNameButtonStyles,
        fontSize: size === 'md' ? '12px' : '11px',
        fontWeight: size === 'md' ? '700' : '600',
        minH: size === 'md' ? '24px' : '22px',
        px: size === 'md' ? 2.5 : 2,
        '& .chakra-button__icon': { opacity: size === 'md' ? 1 : 0 },
      }}
      rightIcon={<EditorIcon size={10} />}
      onClick={(e) => {
        e.stopPropagation()
        if (canNavigate) onNavigate?.()
      }}
      title={label}
    >
      <Text as="span" isTruncated>
        {label}
      </Text>
    </Button>
  )
}

export function ConnectorRouteCard({
  source,
  sourceTo,
  sourceDisabled,
  target,
  targetTo,
  targetDisabled,
  direction,
  subtitle,
  description,
  actions,
  onNavigate,
  compact = false,
}: {
  source: string
  sourceTo?: string
  sourceDisabled?: boolean
  target: string
  targetTo?: string
  targetDisabled?: boolean
  direction?: string
  subtitle?: string | null
  description?: string | null
  actions?: ReactNode
  onNavigate?: () => void
  compact?: boolean
}) {
  return (
    <Box
      px={compact ? 2 : 2.5}
      py={compact ? 1.5 : 2}
      sx={canvasPopoverInsetCardStyles}
      borderRadius={compact ? 'md' : 'lg'}
    >
      <HStack spacing={1.5} align="center" justify="space-between" minW={0}>
        <Box flex={1} minW={0} display="flex" justifyContent="flex-start">
          {sourceTo ? (
            <ConnectorNameButton
              label={source}
              to={sourceTo}
              isDisabled={sourceDisabled}
              size="sm"
              onNavigate={onNavigate}
            />
          ) : (
            <Text fontSize="11px" fontWeight="600" color="gray.100" isTruncated title={source}>{source}</Text>
          )}
        </Box>
        <Text
          as="span"
          fontSize="11px"
          fontWeight="700"
          color="var(--accent)"
          flexShrink={0}
          lineHeight={1}
        >
          {directionGlyph(direction)}
        </Text>
        <Box flex={1} minW={0} display="flex" justifyContent="flex-end">
          {targetTo ? (
            <ConnectorNameButton
              label={target}
              to={targetTo}
              isDisabled={targetDisabled}
              size="sm"
              onNavigate={onNavigate}
            />
          ) : (
            <Text fontSize="11px" fontWeight="600" color="gray.100" isTruncated title={target}>{target}</Text>
          )}
        </Box>
      </HStack>
      {subtitle && (
        <Text
          fontSize="10px"
          color="gray.400"
          mt={1}
          noOfLines={1}
          lineHeight="1.3"
        >
          {subtitle}
        </Text>
      )}
      {description && (
        <Text fontSize="11px" color="gray.400" mt={1} noOfLines={3} lineHeight="1.4">
          {description}
        </Text>
      )}
      {actions && <Box mt={1.5}>{actions}</Box>}
    </Box>
  )
}

export function ConnectorSummaryCard({ details, direction = 'merged', ...props }: {
  details: ProxyConnectorDetails
} & Omit<ComponentProps<typeof ConnectorRouteCard>, 'source' | 'target' | 'subtitle'>) {
  return (
    <ConnectorRouteCard
      {...props}
      source={details.sourceAnchorName}
      target={details.targetAnchorName}
      direction={direction}
      subtitle={isDistinctProxySummaryLabel(details.label, details.count) ? details.label : null}
    />
  )
}

export function UnderlyingConnectors({ details, maxHeight = '240px', onNavigate, renderActions }: {
  details: ProxyConnectorDetails
  maxHeight?: string
  onNavigate?: () => void
  renderActions?: (leaf: ProxyConnectorLeaf) => ReactNode
}) {
  return (
    <VStack align="stretch" spacing={1} width="full">
      <Text color="gray.500" fontSize="9px" fontWeight="700" letterSpacing="0.08em" textTransform="uppercase">
        Underlying Connectors · {details.connectors.length}
      </Text>
      <VStack align="stretch" spacing={1} maxH={maxHeight} overflowY="auto" data-zui-native-wheel="true">
        {details.connectors.map((leaf, index) => {
          // Anchor and owner views may not contain the actual endpoint element.
          const sourceViewId = leaf.source.placementViewId
          const targetViewId = leaf.target.placementViewId
          return (
            <ConnectorRouteCard
              key={`${leaf.ownerViewId}-${leaf.connector.id}-${index}`}
              source={leaf.source.actualElementName}
              sourceTo={sourceViewId != null && sourceViewId > 0 ? `/views/${sourceViewId}?element=${leaf.source.actualElementId}` : undefined}
              target={leaf.target.actualElementName}
              targetTo={targetViewId != null && targetViewId > 0 ? `/views/${targetViewId}?element=${leaf.target.actualElementId}` : undefined}
              direction={leaf.connector.direction}
              subtitle={Array.from(new Set([leaf.connector.label?.trim(), leaf.connector.relationship?.trim()].filter(Boolean))).join(' · ') || null}
              description={leaf.connector.description}
              actions={renderActions?.(leaf)}
              onNavigate={onNavigate}
              compact
            />
          )
        })}
      </VStack>
    </VStack>
  )
}

