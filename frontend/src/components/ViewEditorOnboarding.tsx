import { useEffect, useState } from 'react'
import { Box, Button, HStack, Text, VStack } from '@chakra-ui/react'

const STORAGE_KEY = `diagrameditor_tutorial_v1_core`

interface Props {
   
  hasElements: boolean
}

// ── SVG helpers ─────────────────────────────────────────────────────────────

function GridDots({ prefix }: { prefix: string }) {
  return (
    <>
      {Array.from({ length: 7 }, (_, r) =>
        Array.from({ length: 14 }, (_, c) => (
          <circle
            key={`${prefix}-${r}-${c}`}
            cx={(c * 20 + 10).toString()}
            cy={(r * 20 + 10).toString()}
            r="0.8"
            fill="#1E2D40"
          />
        )),
      ).flat()}
    </>
  )
}

// Step 0 - click element → panel slides in
function ClickInspectIllustration() {
  return (
    <svg
      viewBox="0 0 280 150"
      xmlns="http://www.w3.org/2000/svg"
      style={{ width: '100%', height: 'auto', borderRadius: 10, display: 'block', overflow: 'hidden' }}
    >
      <defs>
        <style>{`
          .dei-panel {
            animation: deiPanelSlide 4s ease-in-out infinite;
          }
          @keyframes deiPanelSlide {
            0%, 20%  { transform: translateX(85px); opacity: 0; }
            40%, 75% { transform: translateX(0);    opacity: 1; }
            90%, 100%{ transform: translateX(85px); opacity: 0; }
          }
          .dei-node-select {
            animation: deiNodeGlow 4s ease-in-out infinite;
          }
          @keyframes deiNodeGlow {
            0%, 25%  { stroke: #4A5568; filter: none; }
            40%, 75% { stroke: #63B3ED; filter: drop-shadow(0 0 6px rgba(99,179,237,0.5)); }
            90%, 100%{ stroke: #4A5568; filter: none; }
          }
          .dei-cursor {
            animation: deiCursorClick 4s ease-in-out infinite;
          }
          @keyframes deiCursorClick {
            0%, 15%  { transform: translate(82px, 75px) scale(1);   opacity: 0; }
            22%      { transform: translate(82px, 75px) scale(1);   opacity: 1; }
            32%      { transform: translate(82px, 75px) scale(0.85); opacity: 1; }
            42%, 80% { transform: translate(82px, 75px) scale(1);   opacity: 0.4; }
            90%, 100%{ opacity: 0; }
          }
        `}</style>
      </defs>

      <rect width="280" height="150" fill="#0D1117" rx="8" />
      <GridDots prefix="ci" />

      {/* Node */}
      <rect
        x="32" y="47" width="100" height="56"
        rx="8" fill="#1C2535"
        stroke="#4A5568" strokeWidth="1.5"
        className="dei-node-select"
      />
      {/* Handles */}
      <circle cx="32" cy="75" r="3.5" fill="#63B3ED" opacity="0.6" />
      <circle cx="132" cy="75" r="3.5" fill="#63B3ED" opacity="0.6" />
      <circle cx="82" cy="47" r="3.5" fill="#63B3ED" opacity="0.6" />
      <circle cx="82" cy="103" r="3.5" fill="#63B3ED" opacity="0.6" />
      <text x="82" y="72" textAnchor="middle" fill="#E2E8F0" fontSize="11" fontFamily="system-ui,sans-serif" fontWeight="700">API Gateway</text>
      <text x="82" y="87" textAnchor="middle" fill="#718096" fontSize="8.5" fontFamily="system-ui,sans-serif">container</text>

      {/* Cursor */}
      <g className="dei-cursor">
        <path d="M0,0 L0,14 L4,10 L7,17 L9,16 L6,9 L11,9 Z" fill="white" stroke="#0D1117" strokeWidth="0.6" />
      </g>

      {/* Sliding detail panel */}
      <g className="dei-panel">
        <rect x="148" y="10" width="122" height="130" rx="8" fill="#1A202C" stroke="#2D3748" strokeWidth="1" />
        <rect x="148" y="10" width="122" height="28" rx="8" fill="#212A3A" />
        <rect x="148" y="28" width="122" height="10" rx="0" fill="#212A3A" />
        <text x="160" y="28" fill="#90CDF4" fontSize="9.5" fontFamily="system-ui,sans-serif" fontWeight="700">API Gateway</text>
        <text x="160" y="39" fill="#718096" fontSize="8" fontFamily="system-ui,sans-serif">container</text>

        <text x="160" y="57" fill="#718096" fontSize="7.5" fontFamily="system-ui,sans-serif" letterSpacing="0.05em">NAME</text>
        <rect x="160" y="62" width="98" height="12" rx="3" fill="#263040" />
        <text x="165" y="72" fill="#CBD5E0" fontSize="8" fontFamily="system-ui,sans-serif">API Gateway</text>

        <text x="160" y="87" fill="#718096" fontSize="7.5" fontFamily="system-ui,sans-serif" letterSpacing="0.05em">TYPE</text>
        <rect x="160" y="92" width="98" height="12" rx="3" fill="#263040" />
        <text x="165" y="102" fill="#CBD5E0" fontSize="8" fontFamily="system-ui,sans-serif">container</text>

        <text x="160" y="117" fill="#718096" fontSize="7.5" fontFamily="system-ui,sans-serif" letterSpacing="0.05em">DESCRIPTION</text>
        <rect x="160" y="122" width="98" height="14" rx="3" fill="#263040" />
      </g>

      <text x="140" y="144" textAnchor="middle" fill="#2D3748" fontSize="8" fontFamily="system-ui,sans-serif">click any element to inspect and edit it</text>
    </svg>
  )
}

// Step 1 - right-click / long-press canvas → context menu
function CanvasMenuIllustration() {
  return (
    <svg
      viewBox="0 0 280 150"
      xmlns="http://www.w3.org/2000/svg"
      style={{ width: '100%', height: 'auto', borderRadius: 10, display: 'block', overflow: 'hidden' }}
    >
      <defs>
        <style>{`
          .dei-ctx-menu {
            animation: deiCtxPop 5s ease-in-out infinite;
          }
          @keyframes deiCtxPop {
            0%, 35%  { opacity: 0; transform: scale(0.92) translateY(4px); }
            50%, 80% { opacity: 1; transform: scale(1)    translateY(0); }
            93%, 100%{ opacity: 0; transform: scale(0.92) translateY(4px); }
          }
          .dei-ripple {
            animation: deiRipple 5s ease-in-out infinite;
          }
          @keyframes deiRipple {
            0%, 25%  { opacity: 0; r: 4; }
            35%      { opacity: 0.8; r: 4; }
            50%      { opacity: 0; r: 22; }
            100%     { opacity: 0; r: 22; }
          }
          .dei-ctx-cursor {
            animation: deiCtxCursor 5s ease-in-out infinite;
          }
          @keyframes deiCtxCursor {
            0%, 20%  { transform: translate(130px, 90px); opacity: 0; }
            30%      { opacity: 1; }
            50%, 90% { transform: translate(130px, 90px); opacity: 0.4; }
            100%     { opacity: 0; }
          }
        `}</style>
        <marker id="dei-arr" markerWidth="7" markerHeight="7" refX="5" refY="3" orient="auto">
          <path d="M0,0 L0,6 L7,3 z" fill="#4A5568" />
        </marker>
      </defs>

      <rect width="280" height="150" fill="#0D1117" rx="8" />
      <GridDots prefix="cm" />

      {/* Existing nodes faded */}
      <rect x="18" y="30" width="76" height="46" rx="7" fill="#1C2535" stroke="#2D3748" strokeWidth="1" />
      <text x="56" y="57" textAnchor="middle" fill="#718096" fontSize="9" fontFamily="system-ui,sans-serif">Auth Service</text>

      <rect x="186" y="30" width="76" height="46" rx="7" fill="#1C2535" stroke="#2D3748" strokeWidth="1" />
      <text x="224" y="57" textAnchor="middle" fill="#718096" fontSize="9" fontFamily="system-ui,sans-serif">Database</text>

      <line x1="94" y1="53" x2="186" y2="53" stroke="#2D3748" strokeWidth="1.5" markerEnd="url(#dei-arr)" />

      {/* Right-click ripple on empty canvas */}
      <circle cx="130" cy="90" className="dei-ripple" fill="none" stroke="#63B3ED" strokeWidth="1.5" r="4" />

      {/* Cursor */}
      <g className="dei-ctx-cursor">
        <path d="M0,0 L0,14 L4,10 L7,17 L9,16 L6,9 L11,9 Z" fill="white" stroke="#0D1117" strokeWidth="0.6" />
      </g>

      {/* Context menu */}
      <g className="dei-ctx-menu" style={{ transformOrigin: '130px 90px' }}>
        <rect x="90" y="85" width="115" height="32" rx="6" fill="#1A202C" stroke="#2D3748" strokeWidth="1" />

        {/* Add Element row - highlighted */}
        <rect x="90" y="85" width="115" height="32" rx="6" fill="#172030" />
        <text x="103" y="105" fill="#90CDF4" fontSize="9" fontFamily="system-ui,sans-serif" fontWeight="600">Add Element</text>
        <rect x="178" y="96" width="18" height="11" rx="2" fill="#2D3748" />
        <text x="187" y="105" textAnchor="middle" fill="#A0AEC0" fontSize="7.5" fontFamily="system-ui,sans-serif" fontWeight="700">C</text>
      </g>

      <text x="140" y="144" textAnchor="middle" fill="#2D3748" fontSize="8" fontFamily="system-ui,sans-serif">right-click (or hold on mobile) on empty canvas</text>
    </svg>
  )
}

// Step 2 - long-press / right-click element → interactive connect mode
function InteractiveConnectIllustration() {
  return (
    <svg
      viewBox="0 0 280 150"
      xmlns="http://www.w3.org/2000/svg"
      style={{ width: '100%', height: 'auto', borderRadius: 10, display: 'block', overflow: 'hidden' }}
    >
      <defs>
        <style>{`
          .dei-src-glow {
            animation: deiSrcGlow 4.5s ease-in-out infinite;
          }
          @keyframes deiSrcGlow {
            0%, 15%  { stroke: #4A5568; filter: none; }
            30%, 75% { stroke: #63B3ED; filter: drop-shadow(0 0 10px rgba(99,179,237,0.6)); }
            90%, 100%{ stroke: #4A5568; filter: none; }
          }
          .dei-tgt-glow {
            animation: deiTgtGlow 4.5s ease-in-out infinite;
          }
          @keyframes deiTgtGlow {
            0%, 40%  { stroke: #4A5568; filter: none; }
            55%, 75% { stroke: #4FD1C5; filter: drop-shadow(0 0 8px rgba(79,209,197,0.5)); }
            90%, 100%{ stroke: #4A5568; filter: none; }
          }
          .dei-arc {
            animation: deiArcDraw 4.5s ease-in-out infinite;
          }
          @keyframes deiArcDraw {
            0%, 45%  { stroke-dashoffset: 120; opacity: 0; }
            60%, 75% { stroke-dashoffset: 0;   opacity: 1; }
            88%, 100%{ stroke-dashoffset: 0;   opacity: 0; }
          }
          .dei-src-label {
            animation: deiSrcLabel 4.5s ease-in-out infinite;
          }
          @keyframes deiSrcLabel {
            0%, 20%  { opacity: 0; }
            32%, 72% { opacity: 1; }
            85%, 100%{ opacity: 0; }
          }
          .dei-tgt-label {
            animation: deiTgtLabel 4.5s ease-in-out infinite;
          }
          @keyframes deiTgtLabel {
            0%, 44%  { opacity: 0; }
            56%, 74% { opacity: 1; }
            86%, 100%{ opacity: 0; }
          }
          .dei-hold-hint {
            animation: deiHoldHint 4.5s ease-in-out infinite;
          }
          @keyframes deiHoldHint {
            0%, 10%  { opacity: 0; }
            18%, 28% { opacity: 1; }
            36%, 100%{ opacity: 0; }
          }
        `}</style>
        <marker id="dei-arr2" markerWidth="8" markerHeight="8" refX="6" refY="3" orient="auto">
          <path d="M0,0 L0,6 L8,3 z" fill="#63B3ED" />
        </marker>
      </defs>

      <rect width="280" height="150" fill="#0D1117" rx="8" />
      <GridDots prefix="ic" />

      {/* Source node */}
      <rect x="22" y="47" width="96" height="56" rx="8" fill="#1C2535" strokeWidth="2" className="dei-src-glow" stroke="#4A5568" />
      <text x="70" y="72" textAnchor="middle" fill="#E2E8F0" fontSize="10" fontFamily="system-ui,sans-serif" fontWeight="700">Auth Service</text>
      <text x="70" y="86" textAnchor="middle" fill="#718096" fontSize="8" fontFamily="system-ui,sans-serif">container</text>
      {/* Source badge */}
      <rect x="34" y="36" width="42" height="13" rx="6" fill="#1A3A5A" stroke="#63B3ED" strokeWidth="0.8" className="dei-src-label" />
      <text x="55" y="46" textAnchor="middle" fill="#63B3ED" fontSize="7.5" fontFamily="system-ui,sans-serif" fontWeight="700" className="dei-src-label">SOURCE</text>

      {/* Target node */}
      <rect x="162" y="47" width="96" height="56" rx="8" fill="#1C2535" strokeWidth="2" className="dei-tgt-glow" stroke="#4A5568" />
      <text x="210" y="72" textAnchor="middle" fill="#E2E8F0" fontSize="10" fontFamily="system-ui,sans-serif" fontWeight="700">Database</text>
      <text x="210" y="86" textAnchor="middle" fill="#718096" fontSize="8" fontFamily="system-ui,sans-serif">database</text>
      {/* Target badge */}
      <rect x="174" y="36" width="70" height="13" rx="6" fill="#142A2A" stroke="#4FD1C5" strokeWidth="0.8" className="dei-tgt-label" />
      <text x="209" y="46" textAnchor="middle" fill="#4FD1C5" fontSize="7.5" fontFamily="system-ui,sans-serif" fontWeight="700" className="dei-tgt-label">TAP TO CONNECT</text>

      {/* Animated arc */}
      <path
        d="M 118 75 C 138 55, 142 55, 162 75"
        fill="none"
        stroke="#63B3ED"
        strokeWidth="1.8"
        strokeDasharray="120"
        strokeDashoffset="120"
        markerEnd="url(#dei-arr2)"
        className="dei-arc"
      />

      {/* Hold hint */}
      <text x="70" y="120" textAnchor="middle" fill="#4A90D9" fontSize="7.5" fontFamily="system-ui,sans-serif" className="dei-hold-hint">hold to activate</text>

      <text x="140" y="144" textAnchor="middle" fill="#2D3748" fontSize="8" fontFamily="system-ui,sans-serif">right-click (or hold on mobile) an element to enter connect mode</text>
    </svg>
  )
}

// ── Steps data ───────────────────────────────────────────────────────────────

const STEPS = [
  {
    title: 'Click to Inspect',
    body: 'Click any element to open its detail panel. Edit the name, type, technology, description, and more. Click an connector to edit its label and relationship.',
    visual: 'inspect' as const,
  },
  {
    title: 'Canvas Menu',
    body: 'Right-click anywhere on the canvas (or hold on mobile) to open the context menu. Navigate between diagram levels or add a new element at that spot.',
    visual: 'canvasmenu' as const,
  },
  {
    title: 'Connect Mode',
    body: 'Right-click (or hold on mobile) any element to enter connect mode - it glows blue as the source. Then tap any other element to draw an connector between them.',
    visual: 'connect' as const,
  },
]

// ── Main component ───────────────────────────────────────────────────────────

export default function ViewEditorOnboarding({  hasElements }: Props) {
  const [visible, setVisible] = useState(false)
  const [step, setStep] = useState(0)

  useEffect(() => {
    if (!hasElements) return
    if (!localStorage.getItem(STORAGE_KEY)) {
      setVisible(true)
    }
  }, [ hasElements])

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
      {/* Backdrop */}
      <Box
        position="absolute"
        inset={0}
        bg="blackAlpha.700"
        pointerEvents="auto"
        onClick={dismiss}
      />

      {/* Tutorial card */}
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
        {/* Skip */}
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

        {/* Step dots */}
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

        {/* Visual + text */}
        <VStack spacing={4} textAlign="center">
          {current.visual === 'inspect' && <ClickInspectIllustration />}
          {current.visual === 'canvasmenu' && <CanvasMenuIllustration />}
          {current.visual === 'connect' && <InteractiveConnectIllustration />}

          <VStack spacing={2}>
            <Text fontWeight="bold" fontSize="lg" color="gray.100" lineHeight="short">
              {current.title}
            </Text>
            <Text fontSize="sm" color="gray.400" lineHeight="tall" maxW="300px">
              {current.body}
            </Text>
          </VStack>
        </VStack>

        {/* Nav */}
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
