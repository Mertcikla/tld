import { useEffect, useState } from 'react'
import { Box, Button, HStack, Text, VStack } from '@chakra-ui/react'

const STORAGE_KEY = `explore_page_tutorial_v1_core`

interface Props {
   
  hasDiagrams: boolean
}

function GridDots({ prefix }: { prefix: string }) {
  return (
    <>
      {Array.from({ length: 7 }, (_, r) =>
        Array.from({ length: 14 }, (_, c) => (
          <circle
            key={`${prefix}-${r}-${c}`}
            cx={c * 20 + 10}
            cy={r * 20 + 10}
            r={0.8}
            fill="#1E2D40"
          />
        )),
      ).flat()}
    </>
  )
}

// Step 0 - overview: all diagrams on one canvas
function OverviewIllustration() {
  return (
    <svg
      viewBox="0 0 280 150"
      xmlns="http://www.w3.org/2000/svg"
      style={{ width: '100%', height: 'auto', borderRadius: 10, display: 'block', overflow: 'hidden' }}
    >
      <defs>
        <style>{`
          .ep-group-a { animation: epFadeIn 0.5s 0.0s ease both; }
          .ep-group-b { animation: epFadeIn 0.5s 0.2s ease both; }
          .ep-group-c { animation: epFadeIn 0.5s 0.4s ease both; }
          .ep-edges   { animation: epFadeIn 0.5s 0.6s ease both; }
          @keyframes epFadeIn {
            from { opacity: 0; }
            to   { opacity: 1; }
          }
        `}</style>
        <marker id="ep-arr" markerWidth="7" markerHeight="7" refX="5" refY="3" orient="auto">
          <path d="M0,0 L0,6 L7,3 z" fill="#4A5568" />
        </marker>
      </defs>

      <rect width="280" height="150" fill="#0D1117" rx="8" />
      <GridDots prefix="ov" />

      {/* Diagram group A */}
      <g className="ep-group-a">
        <rect x="10" y="18" width="110" height="60" rx="6" fill="none" stroke="#2D3748" strokeWidth="1.2" strokeDasharray="5,3" />
        <text x="14" y="29" fill="#4A5568" fontSize="7" fontFamily="system-ui,sans-serif">System Context</text>
        <rect x="18" y="34" width="40" height="30" rx="5" fill="#1C2535" stroke="#374151" strokeWidth="1" />
        <text x="38" y="53" textAnchor="middle" fill="#94A3B8" fontSize="7.5" fontFamily="system-ui,sans-serif">User</text>
        <rect x="70" y="34" width="40" height="30" rx="5" fill="#1C2535" stroke="#374151" strokeWidth="1" />
        <text x="90" y="53" textAnchor="middle" fill="#94A3B8" fontSize="7.5" fontFamily="system-ui,sans-serif">API</text>
        {/* blue dot on API - has child */}
        <circle cx="107" cy="37" r="3.5" fill="#3B82F6" />
        <circle cx="107" cy="37" r="3.5" fill="none" stroke="#3B82F6" strokeWidth="1">
          <animate attributeName="r" values="3.5;9" dur="2s" repeatCount="indefinite" />
          <animate attributeName="opacity" values="0.7;0" dur="2s" repeatCount="indefinite" />
        </circle>
      </g>

      {/* Diagram group B */}
      <g className="ep-group-b">
        <rect x="130" y="18" width="140" height="60" rx="6" fill="none" stroke="#2D3748" strokeWidth="1.2" strokeDasharray="5,3" />
        <text x="134" y="29" fill="#4A5568" fontSize="7" fontFamily="system-ui,sans-serif">Container View</text>
        <rect x="138" y="34" width="36" height="30" rx="5" fill="#1C2535" stroke="#374151" strokeWidth="1" />
        <text x="156" y="53" textAnchor="middle" fill="#94A3B8" fontSize="7.5" fontFamily="system-ui,sans-serif">Router</text>
        <rect x="186" y="34" width="36" height="30" rx="5" fill="#1C2535" stroke="#374151" strokeWidth="1" />
        <text x="204" y="53" textAnchor="middle" fill="#94A3B8" fontSize="7.5" fontFamily="system-ui,sans-serif">Auth</text>
        <rect x="234" y="34" width="30" height="30" rx="5" fill="#1C2535" stroke="#374151" strokeWidth="1" />
        <text x="249" y="53" textAnchor="middle" fill="#94A3B8" fontSize="7.5" fontFamily="system-ui,sans-serif">DB</text>
      </g>

      {/* Diagram group C */}
      <g className="ep-group-c">
        <rect x="10" y="88" width="110" height="50" rx="6" fill="none" stroke="#2D3748" strokeWidth="1.2" strokeDasharray="5,3" />
        <text x="14" y="99" fill="#4A5568" fontSize="7" fontFamily="system-ui,sans-serif">Data Platform</text>
        <rect x="18" y="104" width="36" height="26" rx="5" fill="#1C2535" stroke="#374151" strokeWidth="1" />
        <text x="36" y="121" textAnchor="middle" fill="#94A3B8" fontSize="7.5" fontFamily="system-ui,sans-serif">Kafka</text>
        <rect x="70" y="104" width="40" height="26" rx="5" fill="#1C2535" stroke="#374151" strokeWidth="1" />
        <text x="90" y="121" textAnchor="middle" fill="#94A3B8" fontSize="7.5" fontFamily="system-ui,sans-serif">Spark</text>
      </g>

      {/* Cross-diagram edges */}
      <g className="ep-edges">
        <line x1="110" y1="49" x2="138" y2="49" stroke="#374151" strokeWidth="1.2" markerEnd="url(#ep-arr)" />
        <line x1="90" y1="64" x2="90" y2="104" stroke="#374151" strokeWidth="1.2" markerEnd="url(#ep-arr)" />
      </g>

      <text x="140" y="145" textAnchor="middle" fill="#2D3748" fontSize="8" fontFamily="system-ui,sans-serif">all diagrams on one infinite canvas</text>
    </svg>
  )
}

// Step 1 - zoom in on a blue-dot node → transitions inside
function ZoomInIllustration() {
  return (
    <svg
      viewBox="0 0 280 150"
      xmlns="http://www.w3.org/2000/svg"
      style={{ width: '100%', height: 'auto', borderRadius: 10, display: 'block', overflow: 'hidden' }}
    >
      <defs>
        <style>{`
          .ep-outer {
            animation: epOuter 5s ease-in-out infinite;
          }
          @keyframes epOuter {
            0%, 15%  { transform: scale(1);   opacity: 1; }
            40%, 60% { transform: scale(2.6); opacity: 0; }
            82%, 100%{ transform: scale(1);   opacity: 1; }
          }
          .ep-inner {
            animation: epInner 5s ease-in-out infinite;
          }
          @keyframes epInner {
            0%, 30%  { opacity: 0; }
            50%, 62% { opacity: 1; }
            78%, 100%{ opacity: 0; }
          }
        `}</style>
        <marker id="ep-arr2" markerWidth="7" markerHeight="7" refX="5" refY="3" orient="auto">
          <path d="M0,0 L0,6 L7,3 z" fill="#374151" />
        </marker>
      </defs>

      <rect width="280" height="150" fill="#0D1117" rx="8" />
      <GridDots prefix="zi" />

      {/* Outer scene */}
      <g className="ep-outer" style={{ transformOrigin: '207px 49px' }}>
        <rect x="22" y="42" width="86" height="52" rx="7" fill="#1C2535" stroke="#2D3748" strokeWidth="1" />
        <text x="65" y="65" textAnchor="middle" fill="#94A3B8" fontSize="9" fontFamily="system-ui,sans-serif" fontWeight="600">Auth Service</text>
        <text x="65" y="78" textAnchor="middle" fill="#475569" fontSize="7.5" fontFamily="system-ui,sans-serif">container</text>
        <line x1="108" y1="68" x2="166" y2="68" stroke="#2D3748" strokeWidth="1.5" markerEnd="url(#ep-arr2)" />
        <rect x="170" y="42" width="86" height="52" rx="7" fill="#1C2535" stroke="#2D3748" strokeWidth="1" />
        <text x="213" y="65" textAnchor="middle" fill="#94A3B8" fontSize="9" fontFamily="system-ui,sans-serif" fontWeight="600">API Gateway</text>
        <text x="213" y="78" textAnchor="middle" fill="#475569" fontSize="7.5" fontFamily="system-ui,sans-serif">container</text>
        {/* Blue dot on API Gateway */}
        <circle cx="249" cy="48" r="4" fill="#3B82F6" />
        <circle cx="249" cy="48" r="4" fill="none" stroke="#3B82F6" strokeWidth="1.5">
          <animate attributeName="r" values="4;14" dur="2s" repeatCount="indefinite" />
          <animate attributeName="opacity" values="0.8;0" dur="2s" repeatCount="indefinite" />
        </circle>
        <text x="213" y="31" textAnchor="middle" fill="#3B82F6" fontSize="7" fontFamily="system-ui,sans-serif">zoom in ↑</text>
      </g>

      {/* Inner scene */}
      <g className="ep-inner" opacity="0">
        <rect width="280" height="150" fill="#0D1117" />
        <GridDots prefix="zi2" />
        <text x="140" y="16" textAnchor="middle" fill="#3B82F6" fontSize="8" fontFamily="system-ui,sans-serif" fontWeight="700" letterSpacing="0.05em">API GATEWAY - INTERNALS</text>
        <rect x="14" y="28" width="66" height="44" rx="6" fill="#1C2535" stroke="#2D3748" strokeWidth="1" />
        <text x="47" y="54" textAnchor="middle" fill="#94A3B8" fontSize="8.5" fontFamily="system-ui,sans-serif">Router</text>
        <rect x="107" y="52" width="66" height="44" rx="6" fill="#1C2535" stroke="#2D3748" strokeWidth="1" />
        <text x="140" y="78" textAnchor="middle" fill="#94A3B8" fontSize="8.5" fontFamily="system-ui,sans-serif">Auth MW</text>
        <rect x="200" y="28" width="66" height="44" rx="6" fill="#1C2535" stroke="#2D3748" strokeWidth="1" />
        <text x="233" y="54" textAnchor="middle" fill="#94A3B8" fontSize="8.5" fontFamily="system-ui,sans-serif">Cache</text>
        <line x1="80" y1="56" x2="107" y2="70" stroke="#374151" strokeWidth="1.5" strokeDasharray="4,2" markerEnd="url(#ep-arr2)" />
        <line x1="173" y1="70" x2="200" y2="56" stroke="#374151" strokeWidth="1.5" strokeDasharray="4,2" markerEnd="url(#ep-arr2)" />
      </g>

      <text x="140" y="144" textAnchor="middle" fill="#2D3748" fontSize="8" fontFamily="system-ui,sans-serif">scroll to zoom · nodes with a blue dot have sub-diagrams</text>
    </svg>
  )
}

// Step 2 - bottom bar + breadcrumbs
function NavigateIllustration() {
  return (
    <svg
      viewBox="0 0 280 150"
      xmlns="http://www.w3.org/2000/svg"
      style={{ width: '100%', height: 'auto', borderRadius: 10, display: 'block', overflow: 'hidden' }}
    >
      <defs>
        <style>{`
          .ep-crumb-glow {
            animation: epCrumbGlow 3.5s ease-in-out infinite;
          }
          @keyframes epCrumbGlow {
            0%, 20%  { fill: #374151; }
            45%, 65% { fill: #3B82F6; }
            85%, 100%{ fill: #374151; }
          }
          .ep-bar-btn {
            animation: epBarPulse 3.5s ease-in-out infinite;
          }
          @keyframes epBarPulse {
            0%, 30%  { opacity: 0.5; }
            50%, 70% { opacity: 1;   }
            90%, 100%{ opacity: 0.5; }
          }
        `}</style>
      </defs>

      <rect width="280" height="150" fill="#0D1117" rx="8" />
      <GridDots prefix="nav" />

      {/* Canvas content */}
      <rect x="20" y="20" width="110" height="72" rx="6" fill="none" stroke="#1E2D3E" strokeWidth="1.2" strokeDasharray="5,3" />
      <text x="24" y="31" fill="#2D3748" fontSize="7" fontFamily="system-ui,sans-serif">System</text>
      <rect x="28" y="37" width="40" height="26" rx="4" fill="#1C2535" stroke="#2D3748" strokeWidth="1" />
      <text x="48" y="54" textAnchor="middle" fill="#94A3B8" fontSize="7.5" fontFamily="system-ui,sans-serif">Auth</text>
      <rect x="82" y="37" width="40" height="26" rx="4" fill="#1C2535" stroke="#2D3748" strokeWidth="1" />
      <text x="102" y="54" textAnchor="middle" fill="#94A3B8" fontSize="7.5" fontFamily="system-ui,sans-serif">API</text>

      {/* Focused inner group */}
      <rect x="148" y="20" width="120" height="72" rx="6" fill="none" stroke="#3B82F6" strokeWidth="1.2" />
      <text x="153" y="31" fill="#3B82F6" fontSize="7" fontFamily="system-ui,sans-serif">API Gateway (current)</text>
      <rect x="153" y="37" width="32" height="26" rx="4" fill="#1C2535" stroke="#2D3748" strokeWidth="1" />
      <text x="169" y="54" textAnchor="middle" fill="#94A3B8" fontSize="7" fontFamily="system-ui,sans-serif">Router</text>
      <rect x="197" y="37" width="32" height="26" rx="4" fill="#1C2535" stroke="#2D3748" strokeWidth="1" />
      <text x="213" y="54" textAnchor="middle" fill="#94A3B8" fontSize="7" fontFamily="system-ui,sans-serif">Auth MW</text>
      <rect x="232" y="37" width="30" height="26" rx="4" fill="#1C2535" stroke="#2D3748" strokeWidth="1" />
      <text x="247" y="54" textAnchor="middle" fill="#94A3B8" fontSize="7" fontFamily="system-ui,sans-serif">Cache</text>

      {/* Breadcrumb bar */}
      <rect x="0" y="97" width="280" height="20" fill="rgba(26,32,44,0.9)" />
      <text x="10" y="111" fill="#4A5568" fontSize="8" fontFamily="system-ui,sans-serif">All Diagrams</text>
      <text x="85" y="111" fill="#4A5568" fontSize="8" fontFamily="system-ui,sans-serif">/</text>
      <text x="95" y="111" fill="#4A5568" fontSize="8" fontFamily="system-ui,sans-serif">System Context</text>
      <text x="185" y="111" fill="#4A5568" fontSize="8" fontFamily="system-ui,sans-serif">/</text>
      <rect x="192" y="101" width="73" height="14" rx="3" className="ep-crumb-glow" fill="#374151" />
      <text x="228" y="111" textAnchor="middle" fill="white" fontSize="8" fontFamily="system-ui,sans-serif" fontWeight="600">API Gateway</text>

      {/* Bottom bar */}
      <rect x="60" y="124" width="160" height="20" rx="8" fill="rgba(22,30,43,0.95)" stroke="rgba(255,255,255,0.07)" strokeWidth="1" />
      <text x="85" y="137" textAnchor="middle" fill="#9CA3AF" fontSize="7.5" fontFamily="system-ui,sans-serif" className="ep-bar-btn">Zoom Out</text>
      <line x1="115" y1="127" x2="115" y2="141" stroke="rgba(255,255,255,0.07)" strokeWidth="1" />
      <text x="140" y="137" textAnchor="middle" fill="#9CA3AF" fontSize="7.5" fontFamily="system-ui,sans-serif" className="ep-bar-btn">Fit View</text>
      <line x1="165" y1="127" x2="165" y2="141" stroke="rgba(255,255,255,0.07)" strokeWidth="1" />
      <text x="195" y="137" textAnchor="middle" fill="#9CA3AF" fontSize="7.5" fontFamily="system-ui,sans-serif" className="ep-bar-btn">Share</text>
    </svg>
  )
}

const STEPS = [
  {
    title: 'Your Full Architecture',
    body: 'Explore renders all your diagrams together on one infinite canvas. Every node and connection is visible - see how your systems relate at a glance.',
    visual: 'overview' as const,
  },
  {
    title: 'Zoom In to Drill Down',
    body: 'Nodes with a pulsing blue dot are linked to a sub-diagram. Scroll to zoom in on one and the view transitions inside. Zoom out past the boundary to go back up.',
    visual: 'zoomin' as const,
  },
  {
    title: 'Navigate Levels',
    body: 'The breadcrumb trail at the top shows your current depth - click any crumb to jump there instantly. Use the bottom bar to fit all diagrams back into view or share the canvas.',
    visual: 'navigate' as const,
  },
]

export default function ExplorePageOnboarding({  hasDiagrams }: Props) {
  const [visible, setVisible] = useState(false)
  const [step, setStep] = useState(0)

  useEffect(() => {
    if (!hasDiagrams) return
    if (!localStorage.getItem(STORAGE_KEY)) {
      setVisible(true)
    }
  }, [ hasDiagrams])

  const dismiss = () => {
    localStorage.setItem(STORAGE_KEY, '1')
    setVisible(false)
  }

  if (!visible) return null

  const current = STEPS[step]
  const isLast = step === STEPS.length - 1

  return (
    <Box
      position="fixed"
      inset={0}
      zIndex={2000}
      display="flex"
      alignItems="center"
      justifyContent="center"
      pointerEvents="none"
    >
      <Box
        position="absolute"
        inset={0}
        bg="blackAlpha.700"
        pointerEvents="auto"
        onClick={dismiss}
      />
      <Box
        position="relative"
        w="380px"
        bg="var(--bg-panel)"
        border="1px solid"
        borderColor="var(--border-main)"
        borderRadius="16px"
        p={6}
        boxShadow="0 24px 60px rgba(0,0,0,0.6), 0 4px 16px rgba(0,0,0,0.4), 0 0 0 1px rgba(255,255,255,0.04)"
        pointerEvents="auto"
      >
        <Button
          position="absolute"
          top={3}
          right={3}
          size="xs"
          variant="ghost"
          color="gray.500"
          _hover={{ color: 'gray.200', bg: 'whiteAlpha.100' }}
          onClick={dismiss}
          fontWeight="normal"
        >
          Skip Tutorial
        </Button>

        <HStack justify="center" spacing={2} mb={5}>
          {STEPS.map((_, i) => (
            <Box
              key={i}
              w={i === step ? '18px' : '6px'}
              h="6px"
              rounded="full"
              bg={i === step ? 'blue.400' : 'gray.600'}
              transition="all 0.25s ease"
              cursor="pointer"
              onClick={() => setStep(i)}
              _hover={{ bg: i === step ? 'blue.400' : 'gray.500' }}
            />
          ))}
        </HStack>

        <VStack spacing={4} textAlign="center">
          {current.visual === 'overview' && <OverviewIllustration />}
          {current.visual === 'zoomin' && <ZoomInIllustration />}
          {current.visual === 'navigate' && <NavigateIllustration />}

          <VStack spacing={2}>
            <Text fontWeight="bold" fontSize="lg" color="gray.100" lineHeight="short">
              {current.title}
            </Text>
            <Text fontSize="sm" color="gray.400" lineHeight="tall" maxW="300px">
              {current.body}
            </Text>
          </VStack>
        </VStack>

        <HStack mt={6} justify="space-between" align="center">
          <Button
            size="sm"
            variant="ghost"
            color="gray.500"
            _hover={{ color: 'gray.300' }}
            onClick={() => setStep(step - 1)}
            visibility={step > 0 ? 'visible' : 'hidden'}
          >
            ← Back
          </Button>
          <Button
            size="sm"
            colorScheme="blue"
            px={5}
            onClick={isLast ? dismiss : () => setStep(step + 1)}
          >
            {isLast ? 'Got it' : 'Next →'}
          </Button>
        </HStack>
      </Box>
    </Box>
  )
}
