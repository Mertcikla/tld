import { Box, Divider, Flex, HStack, Tag, Text, VStack } from '@chakra-ui/react'
import { TYPE_COLORS, type PlacedElement } from '../types'

interface Props {
    data: PlacedElement & { hasChildLink?: boolean }
    /** Bounding rect of the node element in screen (viewport) coordinates */
    anchorRect: DOMRect
}

export default function NodeHoverCard({ data, anchorRect }: Props) {
    const color = TYPE_COLORS[data.kind ?? ''] ?? 'gray'

    // Position the card centred above the node using fixed coordinates so it
    // escapes React Flow's stacking context and always renders on top.
    const centreX = anchorRect.left + anchorRect.width / 2
    const topY = anchorRect.top - 12   // 12px gap above the node

    return (
        <Box
            position="fixed"
            // Temporarily off-screen width to measure; real centering via transform
            left={`${centreX}px`}
            top={`${topY}px`}
            transform="translate(-50%, -100%)"
            zIndex={99999}
            pointerEvents="none"
            minW="220px"
            maxW="300px"
            // Soft Clay styles
            bg="clay.bg"
            border="1px solid rgba(255,255,255,0.08)"
            rounded="xl"
            boxShadow="clay-out"
            px={4}
            py={3}
            // Fade-in animation
            animation="nodeCardIn 0.15s ease-out"
            sx={{
                '@keyframes nodeCardIn': {
                    from: { opacity: 0, transform: 'translate(-50%, calc(-100% + 6px))' },
                    to: { opacity: 1, transform: 'translate(-50%, -100%)' },
                },
            }}
        >
            {/* Caret pointing down toward the node */}
            <Box
                position="absolute"
                bottom="-6px"
                left="50%"
                transform="translateX(-50%)"
                w={0}
                h={0}
                borderLeft="6px solid transparent"
                borderRight="6px solid transparent"
                borderTop="6px solid"
                borderTopColor="clay.bg"
            />

            <VStack align="stretch" spacing={2}>
                {/* Header: name + type badge */}
                <Flex align="flex-start" justify="space-between" gap={2}>
                    <Text
                        fontSize="sm"
                        fontWeight="semibold"
                        color="gray.100"
                        lineHeight="tight"
                    >
                        {data.name}
                    </Text>
                    <Tag
                        size="sm"
                        colorScheme={color}
                        variant="subtle"
                        flexShrink={0}
                        fontSize="9px"
                        textTransform="uppercase"
                        letterSpacing="0.06em"
                        mt="1px"
                    >
                        {data.kind}
                    </Tag>
                </Flex>

                {/* Technology */}
                {data.technology && (
                    <Text fontSize="xs" color="gray.500">
                        {data.technology}
                    </Text>
                )}

                {/* Description */}
                {data.description && (
                    <>
                        <Divider borderColor="rgba(255,255,255,0.06)" />
                        <Text fontSize="xs" color="gray.400" noOfLines={4}>
                            {data.description}
                        </Text>
                    </>
                )}

                {/* Tags */}
                {data.tags && data.tags.length > 0 && (
                    <HStack spacing={1} flexWrap="wrap">
                        {data.tags.map((tag) => (
                            <Tag
                                key={tag}
                                size="sm"
                                variant="outline"
                                colorScheme="gray"
                                fontSize="9px"
                                borderColor="rgba(255,255,255,0.1)"
                                color="gray.500"
                            >
                                {tag}
                            </Tag>
                        ))}
                    </HStack>
                )}

                {/* Child view hint */}
                {data.hasChildLink && (
                    <>
                        <Divider borderColor="rgba(255,255,255,0.06)" />
                        <Flex align="center" gap={1.5}>
                            <Box w="5px" h="5px" rounded="full" bg="blue.400" opacity={0.8} />
                            <Text fontSize="9px" color="blue.400" letterSpacing="0.04em">
                                Has child view
                            </Text>
                        </Flex>
                    </>
                )}
            </VStack>
        </Box>
    )
}
