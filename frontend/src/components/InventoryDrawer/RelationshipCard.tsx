import { motion } from 'framer-motion'
import { Box, Flex } from '@chakra-ui/react'
import { ElementBody } from '../NodeBody'
import { ElementContainer } from '../NodeContainer'

interface RelationshipCardProps {
  name: string
  type?: string
  technology?: string
  logoUrl?: string
  borderColor?: string
  onClick?: () => void
  compactLevel?: number
  testId?: string
  isSelected?: boolean
  shadow?: string
}

export function RelationshipCard({
  name,
  type = '',
  technology = '',
  logoUrl,
  borderColor = 'whiteAlpha.200',
  onClick,
  compactLevel = 0,
  testId = 'inventory-connector-card',
  isSelected = false,
  shadow,
}: RelationshipCardProps) {
  const cardPadding = compactLevel >= 2 ? 1 : compactLevel >= 1 ? 1.5 : 2
  const showTech = compactLevel < 2 && !!technology
  const showType = compactLevel < 3 && !!type
  const cardW = '180px'
  const cardH = '85px'
  const iconSize = compactLevel >= 3 ? '16px' : compactLevel >= 2 ? '20px' : compactLevel >= 1 ? '24px' : '26px'

  const truncatedName = name.length > 30 ? name.slice(0, 29) + '…' : name
  const nameLen = truncatedName.length
  const nameSize =
    compactLevel >= 3
      ? nameLen > 15
        ? '2xs'
        : 'xs'
      : compactLevel >= 2
        ? nameLen > 20
          ? '2xs'
          : 'xs'
        : compactLevel >= 1
          ? nameLen > 22
            ? 'xs'
            : 'sm'
          : nameLen > 24
            ? 'xs'
            : 'sm'

  return (
    <motion.div
      data-testid={testId}
      data-pan-block="true"
      initial={{ opacity: 0, scale: 0.92 }}
      animate={{ opacity: 1, scale: 1 }}
      whileHover={{ scale: 1.02 }}
      transition={{ duration: 0.18 }}
    >
      <ElementContainer
        onClick={onClick}
        w={cardW}
        h={cardH}
        p={0}
        overflow="hidden"
        isSelected={isSelected}
        cursor={onClick ? 'pointer' : 'default'}
        borderColor={isSelected ? borderColor : 'whiteAlpha.200'}
        borderWidth={isSelected ? '2px' : '1px'}
        boxShadow={shadow}
        _hover={
          onClick
            ? {
                borderColor: isSelected ? borderColor : 'var(--accent)',
                boxShadow: '0 0 0 1px rgba(var(--accent-rgb), 0.25)',
              }
            : undefined
        }
        position="relative"
      >
        <Flex direction="column" align="center" justify="center" gap={0.5} p={cardPadding} w="100%" h="100%" overflow="hidden" boxSizing="border-box">
          {logoUrl && (
            <Flex boxSize={iconSize} align="center" justify="center" flexShrink={0}>
              <Box as="img" src={logoUrl} alt="" maxW="100%" maxH="100%" objectFit="contain" opacity={0.95} />
            </Flex>
          )}
          <ElementBody
            name={truncatedName}
            type={showType ? type : ''}
            technology={showTech ? technology : undefined}
            nameSize={nameSize}
            nameNoOfLines={1}
            typeSize="2xs"
            techSize="2xs"
            align="center"
            width="100%"
            minW={0}
            p={0}
            overflow="hidden"
          />
        </Flex>
      </ElementContainer>
    </motion.div>
  )
}
