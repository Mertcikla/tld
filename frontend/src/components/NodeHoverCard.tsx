import { Box, Divider, HStack, Tag, Text, VStack } from '@chakra-ui/react'
import { canvasPopoverContentStyles, canvasPopoverKindLabelStyles } from '../styles/surfaces'
import type { PlacedElement, Tag as TagType } from '../types'
import { resolveElementIconUrl } from '../utils/elementIcon'
import { isElementGroupTag } from '../utils/elementGroups'
import { isGroundingIgnoreTag } from '../utils/groundingTags'

interface Props {
    data: PlacedElement & { hasChildLink?: boolean }
    /** Bounding rect of the node element in screen (viewport) coordinates */
    anchorRect: DOMRect
    tagColors?: Record<string, TagType>
}

export default function NodeHoverCard({ data, anchorRect, tagColors }: Props) {
    const kindLabel = data.kind?.trim() || ''
    const visibleTags = data.tags?.filter((tag) => !isElementGroupTag(tag) && !isGroundingIgnoreTag(tag)) ?? []
    const logoUrl = resolveElementIconUrl(data.logo_url, data.technology_connectors)
    const hasBodyContent = Boolean(data.technology || data.description || visibleTags.length > 0 || data.hasChildLink)

    // Position the card centred above the node using fixed coordinates so it
    // escapes React Flow's stacking context and always renders on top.
    const centreX = anchorRect.left + anchorRect.width / 2
    const topY = anchorRect.top - 12   // 12px gap above the node

    return (
        <Box
            position="fixed"
            left={`${centreX}px`}
            top={`${topY}px`}
            transform="translate(-50%, -100%)"
            zIndex={99999}
            pointerEvents="none"
            width="264px"
            animation="nodeCardIn 0.15s ease-out"
            sx={{
                ...canvasPopoverContentStyles,
                '@keyframes nodeCardIn': {
                    from: { opacity: 0, transform: 'translate(-50%, calc(-100% + 6px))' },
                    to: { opacity: 1, transform: 'translate(-50%, -100%)' },
                },
            }}
        >
            <HStack
                px={3.5}
                py={2.5}
                spacing={2}
                align="center"
                justify="space-between"
                borderBottom={hasBodyContent ? '1px solid' : 'none'}
                borderColor="whiteAlpha.100"
            >
                <HStack spacing={2} align="center" flex={1} minW={0}>
                    {logoUrl && (
                        <Box
                            flexShrink={0}
                            w="22px"
                            h="22px"
                            rounded="md"
                            bg="whiteAlpha.50"
                            border="1px solid"
                            borderColor="whiteAlpha.100"
                            display="flex"
                            alignItems="center"
                            justifyContent="center"
                        >
                            <Box as="img" src={logoUrl} w="14px" h="14px" objectFit="contain" />
                        </Box>
                    )}
                    <Text
                        fontSize="13px"
                        fontWeight="700"
                        color="gray.50"
                        lineHeight="1.2"
                        isTruncated
                        flex={1}
                    >
                        {data.name}
                    </Text>
                </HStack>
                {kindLabel && (
                    <Text sx={canvasPopoverKindLabelStyles} isTruncated title={kindLabel}>
                        {kindLabel}
                    </Text>
                )}
            </HStack>

            {hasBodyContent && (
                <VStack align="stretch" spacing={2} px={3.5} py={2.5}>
                    {data.technology && (
                        <Text fontSize="11px" fontWeight="500" color="gray.300" noOfLines={1}>
                            {data.technology}
                        </Text>
                    )}

                    {data.description && (
                        <Text fontSize="11px" color="gray.400" lineHeight="1.4" noOfLines={3}>
                            {data.description}
                        </Text>
                    )}

                    {visibleTags.length > 0 && (
                        <HStack spacing={1} flexWrap="wrap">
                            {visibleTags.map((tag) => {
                                const tagColor = tagColors?.[tag]?.color
                                return (
                                    <Tag
                                        key={tag}
                                        size="sm"
                                        variant="subtle"
                                        bg={tagColor ? `color-mix(in srgb, ${tagColor} 12%, transparent)` : 'whiteAlpha.100'}
                                        color={tagColor || 'gray.400'}
                                        fontSize="9px"
                                        borderColor={tagColor ? `color-mix(in srgb, ${tagColor} 30%, transparent)` : 'rgba(255,255,255,0.1)'}
                                        borderWidth="1px"
                                        borderRadius="full"
                                        px={2}
                                    >
                                        {tag}
                                    </Tag>
                                )
                            })}
                        </HStack>
                    )}

                    {data.hasChildLink && (
                        <>
                            {(data.technology || data.description || visibleTags.length > 0) && (
                                <Divider borderColor="whiteAlpha.100" />
                            )}
                            <HStack spacing={1.5} align="center">
                                <Box w="5px" h="5px" rounded="full" bg="var(--accent)" flexShrink={0} />
                                <Text fontSize="10px" fontWeight="600" color="var(--accent)" letterSpacing="0.02em">
                                    Has child view
                                </Text>
                            </HStack>
                        </>
                    )}
                </VStack>
            )}
        </Box>
    )
}
