import React from 'react'
import { Box, Text } from '@chakra-ui/react'

interface EmptyCanvasStateProps {
  isMobile: boolean
  hasNodes: boolean
}

export const EmptyCanvasState: React.FC<EmptyCanvasStateProps> = React.memo(({ isMobile, hasNodes }) => {
  if (hasNodes) return null

  return (
    <Box position="absolute" top="50%" left="50%" transform="translate(-50%, -50%)"
      textAlign="center" pointerEvents="none" px={4} userSelect="none">
      <svg width="160" height="100" viewBox="0 0 160 100" style={{ display: 'block', margin: '0 auto 20px' }} xmlns="http://www.w3.org/2000/svg">
        <defs>
          <marker id="es-arr" markerWidth="7" markerHeight="7" refX="5" refY="3" orient="auto">
            <path d="M0,0 L0,6 L7,3 z" fill="#374151" />
          </marker>
        </defs>
        <rect x="0" y="0" width="48" height="100" rx="6" fill="#1C2535" stroke="#1F2937" strokeWidth="1" />
        <rect x="6" y="12" width="36" height="14" rx="3" fill="#263044" />
        <rect x="6" y="31" width="36" height="14" rx="3" fill="#2D3A50" stroke="#3B82F6" strokeWidth="0.8" />
        <rect x="6" y="50" width="36" height="14" rx="3" fill="#263044" />
        <line x1="50" y1="38" x2="63" y2="50" stroke="#374151" strokeWidth="1.4" strokeDasharray="3,2" markerEnd="url(#es-arr)" />
        <rect x="56" y="4" width="104" height="92" rx="8" fill="#111827" stroke="#1F2937" strokeWidth="1" />
        {Array.from({ length: 4 }, (_, r) => Array.from({ length: 6 }, (_, c) => (
          <circle key={`${r}-${c}`} cx={(c * 15 + 66).toString()} cy={(r * 22 + 18).toString()} r="0.7" fill="#1F2937" />
        ))).flat()}
        <rect x="66" y="28" width="84" height="44" rx="6" fill="none" stroke="#374151" strokeWidth="1.2" strokeDasharray="4,3" />
        <rect x="76" y="38" width="36" height="22" rx="4" fill="#1C2535" stroke="#3B82F6" strokeWidth="1" opacity="0.6" />
        <text x="94" y="53" textAnchor="middle" fill="#94A3B8" fontSize="7" fontFamily="system-ui,sans-serif">Service</text>
      </svg>
      <Text fontSize="md" fontWeight="semibold" color="gray.500" mb={1}>Start building your view</Text>
      <Text fontSize="xs" color="gray.600" lineHeight="tall">
        {isMobile
          ? 'Tap/Drag from the left panel, or Hold on the canvas to bring up the menu, then tap Add Element.'
          : <> Drag from the left panel, or press <kbd style={{ fontFamily: 'monospace', background: '#1F2937', border: '1px solid #374151', borderRadius: 3, padding: '0 4px' }}>C</kbd> to create a new element directly.</>}
      </Text>
    </Box>
  )
})
EmptyCanvasState.displayName = 'EmptyCanvasState'
