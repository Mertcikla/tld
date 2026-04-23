import { useEffect, useState } from 'react'
import { Box, Button, HStack, Text, VStack } from '@chakra-ui/react'

const STORAGE_KEY = `dependencies_tutorial_v1_core`

interface Props {
   
  hasElements: boolean
}

function GridDots({ prefix }: { prefix: string }) {
  return (
    <>
      {Array.from({ length: 7 }, (_, c) =>
        Array.from({ length: 5 }, (_, r) => (
          <circle
            key={`${prefix}-${r}-${c}`}
            cx={c * 40 + 20}
            cy={r * 30 + 15}
            r={0.8}
            fill="#1E2D40"
          />
        )),
      ).flat()}
    </>
  )
}

// Step 0 - the split layout: list on top, graph below
function SplitLayoutIllustration() {
  return (
    <svg
      viewBox="0 0 280 150"
      xmlns="http://www.w3.org/2000/svg"
      style={{ width: '100%', height: 'auto', borderRadius: 10, display: 'block', overflow: 'hidden' }}
    >
      <defs>
        <style>{`
          .dp-row-0 { animation: dpRowIn 0.4s 0.00s ease both; }
          .dp-row-1 { animation: dpRowIn 0.4s 0.08s ease both; }
          .dp-row-2 { animation: dpRowIn 0.4s 0.16s ease both; }
          .dp-row-3 { animation: dpRowIn 0.4s 0.24s ease both; }
          .dp-row-4 { animation: dpRowIn 0.4s 0.32s ease both; }
          @keyframes dpRowIn {
            from { opacity: 0; transform: translateX(-6px); }
            to   { opacity: 1; transform: translateX(0); }
          }
          .dp-divider-hint {
            animation: dpDivPulse 3s ease-in-out infinite;
          }
          @keyframes dpDivPulse {
            0%, 40%  { stroke: #374151; }
            55%, 70% { stroke: #3B82F6; }
            85%, 100%{ stroke: #374151; }
          }
        `}</style>
      </defs>

      <rect width="280" height="150" fill="#0D1117" rx="8" />

      {/* ── Top half: table ── */}
      {/* Header */}
      <rect x="0" y="0" width="280" height="14" fill="#111827" rx="0" />
      <text x="10" y="10" fill="#4A5568" fontSize="7" fontFamily="system-ui,sans-serif" letterSpacing="0.06em" fontWeight="700">NAME</text>
      <text x="120" y="10" fill="#4A5568" fontSize="7" fontFamily="system-ui,sans-serif" letterSpacing="0.06em" fontWeight="700">TYPE</text>
      <text x="185" y="10" fill="#4A5568" fontSize="7" fontFamily="system-ui,sans-serif" letterSpacing="0.06em" fontWeight="700">TECHNOLOGY</text>
      <text x="258" y="10" textAnchor="end" fill="#4A5568" fontSize="7" fontFamily="system-ui,sans-serif" letterSpacing="0.06em" fontWeight="700">NBR</text>

      {/* Rows */}
      {[
        { name: 'API Gateway', type: 'container', tech: 'nginx', nbr: 8, color: '#F6AD55' },
        { name: 'Auth Service', type: 'service', tech: 'Go', nbr: 5, color: '#63B3ED' },
        { name: 'PostgreSQL', type: 'database', tech: 'postgres', nbr: 3, color: '#63B3ED' },
        { name: 'Kafka', type: 'queue', tech: 'kafka', nbr: 2, color: '#63B3ED' },
        { name: 'Frontend', type: 'webapp', tech: 'React', nbr: 1, color: '#9CA3AF' },
      ].map(({ name, type, tech, nbr, color }, i) => (
        <g key={name} className={`dp-row-${i}`}>
          <rect x="0" y={14 + i * 14} width="280" height="14"
            fill={i === 0 ? '#1A2D40' : 'transparent'}
            stroke="none"
          />
          <text x="10" y={23 + i * 14} fill={i === 0 ? '#90CDF4' : '#CBD5E0'} fontSize="8" fontFamily="system-ui,sans-serif" fontWeight={i === 0 ? '600' : '400'}>{name}</text>
          <rect x="116" y={16 + i * 14} width={type.length * 4.5 + 6} height="9" rx="3" fill="rgba(255,255,255,0.06)" />
          <text x="119" y={23 + i * 14} fill="#718096" fontSize="7" fontFamily="system-ui,sans-serif">{type}</text>
          <text x="185" y={23 + i * 14} fill="#4A5568" fontSize="7.5" fontFamily="system-ui,sans-serif">{tech}</text>
          <text x="260" y={23 + i * 14} textAnchor="end" fill={color} fontSize="8" fontFamily="system-ui,sans-serif" fontWeight="700">{nbr}</text>
        </g>
      ))}

      {/* Divider */}
      <line x1="0" y1="84" x2="280" y2="84" stroke="#374151" strokeWidth="5" className="dp-divider-hint" />
      <rect x="120" y="81.5" width="40" height="5" rx="2.5" fill="#4A5568" />
      <text x="140" y="93" textAnchor="middle" fill="#374151" fontSize="7" fontFamily="system-ui,sans-serif">drag to resize</text>

      {/* ── Bottom half: empty graph prompt ── */}
      <rect x="0" y="84" width="280" height="66" fill="#171923" />
      <GridDots prefix="dp-btm" />
      <text x="140" y="118" textAnchor="middle" fill="#374151" fontSize="10" fontFamily="system-ui,sans-serif">Select an element above</text>
      <text x="140" y="131" textAnchor="middle" fill="#2D3748" fontSize="8" fontFamily="system-ui,sans-serif">The dependency graph will appear here</text>
    </svg>
  )
}

// Step 1 - click a row → graph shows selected + neighbours
function DependencyGraphIllustration() {
  return (
    <svg
      viewBox="0 0 280 150"
      xmlns="http://www.w3.org/2000/svg"
      style={{ width: '100%', height: 'auto', borderRadius: 10, display: 'block', overflow: 'hidden' }}
    >
      <defs>
        <style>{`
          .dp-src-node { animation: dpSrcIn 0.5s 0.1s ease both; }
          .dp-ctr-node { animation: dpCtrIn 0.4s 0.0s ease both; }
          .dp-tgt-node { animation: dpTgtIn 0.5s 0.2s ease both; }
          .dp-top-node { animation: dpTopIn 0.5s 0.15s ease both; }
          @keyframes dpSrcIn {
            from { opacity: 0; transform: translateX(-12px); }
            to   { opacity: 1; transform: translateX(0); }
          }
          @keyframes dpCtrIn {
            from { opacity: 0; transform: scale(0.9); }
            to   { opacity: 1; transform: scale(1); }
          }
          @keyframes dpTgtIn {
            from { opacity: 0; transform: translateX(12px); }
            to   { opacity: 1; transform: translateX(0); }
          }
          @keyframes dpTopIn {
            from { opacity: 0; transform: translateY(-10px); }
            to   { opacity: 1; transform: translateY(0); }
          }
        `}</style>
      </defs>

      <rect width="280" height="150" fill="#171923" rx="8" />
      <GridDots prefix="dg" />

      {/* Top: bidirectional */}
      <g className="dp-top-node">
        <rect x="102" y="8" width="76" height="34" rx="6" fill="#1C2535" stroke="#2D3748" strokeWidth="1" />
        <text x="140" y="23" textAnchor="middle" fill="#CBD5E0" fontSize="8" fontFamily="system-ui,sans-serif" fontWeight="600">Frontend</text>
        <rect x="110" y="28" width="36" height="10" rx="3" fill="rgba(79,209,197,0.15)" />
        <text x="128" y="36" textAnchor="middle" fill="#4FD1C5" fontSize="7" fontFamily="system-ui,sans-serif">↔ calls</text>
        <text x="140" y="6" textAnchor="middle" fill="#718096" fontSize="6.5" fontFamily="system-ui,sans-serif" style={{ textTransform: 'uppercase', letterSpacing: '0.08em' }}>bidirectional</text>
      </g>
      <line x1="140" y1="42" x2="140" y2="56" stroke="#2D3748" strokeWidth="1.5" />

      {/* Left: source */}
      <g className="dp-src-node">
        <text x="46" y="52" textAnchor="middle" fill="#718096" fontSize="6.5" fontFamily="system-ui,sans-serif" letterSpacing="0.08em">SOURCES</text>
        <rect x="8" y="58" width="76" height="40" rx="6" fill="#1C2535" stroke="#2D3748" strokeWidth="1" />
        <text x="46" y="76" textAnchor="middle" fill="#CBD5E0" fontSize="8" fontFamily="system-ui,sans-serif" fontWeight="600">Load Balancer</text>
        <rect x="14" y="82" width="24" height="10" rx="3" fill="rgba(99,179,237,0.15)" />
        <text x="26" y="90" textAnchor="middle" fill="#63B3ED" fontSize="7" fontFamily="system-ui,sans-serif">→</text>
      </g>
      <line x1="84" y1="78" x2="104" y2="90" stroke="#2D3748" strokeWidth="1.5" />

      {/* Centre: selected */}
      <g className="dp-ctr-node">
        <rect x="102" y="56" width="76" height="50" rx="7" fill="#1A2D40" stroke="#63B3ED" strokeWidth="1.8" />
        <rect x="118" y="50" width="44" height="10" rx="5" fill="#3B82F6" />
        <text x="140" y="58" textAnchor="middle" fill="white" fontSize="6.5" fontFamily="system-ui,sans-serif" fontWeight="700" letterSpacing="0.08em">SELECTED</text>
        <text x="140" y="80" textAnchor="middle" fill="#90CDF4" fontSize="9" fontFamily="system-ui,sans-serif" fontWeight="700">API Gateway</text>
        <text x="140" y="92" textAnchor="middle" fill="#718096" fontSize="7.5" fontFamily="system-ui,sans-serif">8 neighbours</text>
      </g>

      {/* Right: targets */}
      <g className="dp-tgt-node">
        <text x="232" y="52" textAnchor="middle" fill="#718096" fontSize="6.5" fontFamily="system-ui,sans-serif" letterSpacing="0.08em">TARGETS</text>
        <rect x="196" y="58" width="76" height="34" rx="6" fill="#1C2535" stroke="#2D3748" strokeWidth="1" />
        <text x="234" y="75" textAnchor="middle" fill="#CBD5E0" fontSize="8" fontFamily="system-ui,sans-serif" fontWeight="600">Auth Service</text>
        <rect x="200" y="80" width="24" height="10" rx="3" fill="rgba(99,179,237,0.15)" />
        <text x="212" y="88" textAnchor="middle" fill="#63B3ED" fontSize="7" fontFamily="system-ui,sans-serif">→</text>
        <rect x="196" y="100" width="76" height="34" rx="6" fill="#1C2535" stroke="#2D3748" strokeWidth="1" />
        <text x="234" y="117" textAnchor="middle" fill="#CBD5E0" fontSize="8" fontFamily="system-ui,sans-serif" fontWeight="600">PostgreSQL</text>
        <rect x="200" y="122" width="24" height="10" rx="3" fill="rgba(99,179,237,0.15)" />
        <text x="212" y="130" textAnchor="middle" fill="#63B3ED" fontSize="7" fontFamily="system-ui,sans-serif">→</text>
      </g>
      <line x1="178" y1="80" x2="196" y2="74" stroke="#2D3748" strokeWidth="1.5" />
      <line x1="178" y1="90" x2="196" y2="116" stroke="#2D3748" strokeWidth="1.5" />

      <text x="140" y="146" textAnchor="middle" fill="#2D3748" fontSize="8" fontFamily="system-ui,sans-serif">click any row to see its dependency graph</text>
    </svg>
  )
}

// Step 2 - pivot + search/filter
function PivotIllustration() {
  return (
    <svg
      viewBox="0 0 280 150"
      xmlns="http://www.w3.org/2000/svg"
      style={{ width: '100%', height: 'auto', borderRadius: 10, display: 'block', overflow: 'hidden' }}
    >
      <defs>
        <style>{`
          .dp-pivot-cursor {
            animation: dpPivotCursor 4s ease-in-out infinite;
          }
          @keyframes dpPivotCursor {
            0%, 20%  { transform: translate(232px, 90px); opacity: 0; }
            30%      { opacity: 1; }
            45%      { transform: translate(232px, 90px) scale(0.85); }
            55%, 90% { transform: translate(232px, 90px) scale(1); opacity: 0.5; }
            100%     { opacity: 0; }
          }
          .dp-pivot-highlight {
            animation: dpPivotHL 4s ease-in-out infinite;
          }
          @keyframes dpPivotHL {
            0%, 35%  { stroke: #2D3748; }
            50%, 80% { stroke: #63B3ED; filter: drop-shadow(0 0 6px rgba(99,179,237,0.4)); }
            93%, 100%{ stroke: #2D3748; filter: none; }
          }
          .dp-search-type {
            animation: dpSearchType 4s ease-in-out infinite;
          }
          @keyframes dpSearchType {
            0%, 10%  { width: 0; }
            40%, 90% { width: 48px; }
            100%     { width: 0; }
          }
        `}</style>
      </defs>

      <rect width="280" height="150" fill="#0D1117" rx="8" />

      {/* Search bar */}
      <rect x="8" y="8" width="180" height="20" rx="4" fill="#1C2535" stroke="#2D3748" strokeWidth="1" />
      <text x="18" y="22" fill="#4A5568" fontSize="8.5" fontFamily="system-ui,sans-serif">🔍</text>
      <text x="32" y="22" fill="#CBD5E0" fontSize="8.5" fontFamily="system-ui,sans-serif">Auth</text>
      <rect x="32" y="13" width="48" height="10" rx="0" fill="#2A3A50" className="dp-search-type" />

      {/* Type filter button */}
      <rect x="196" y="8" width="76" height="20" rx="4" fill="#1C2535" stroke="#2D3748" strokeWidth="1" />
      <text x="234" y="22" textAnchor="middle" fill="#718096" fontSize="8.5" fontFamily="system-ui,sans-serif">service ▾</text>

      {/* Filtered rows */}
      <rect x="0" y="32" width="280" height="12" fill="#111827" />
      <text x="10" y="41" fill="#4A5568" fontSize="6.5" fontFamily="system-ui,sans-serif" letterSpacing="0.06em" fontWeight="700">NAME</text>
      <text x="140" y="41" fill="#4A5568" fontSize="6.5" fontFamily="system-ui,sans-serif" letterSpacing="0.06em" fontWeight="700">TECHNOLOGY</text>
      <text x="258" y="41" textAnchor="end" fill="#4A5568" fontSize="6.5" fontFamily="system-ui,sans-serif" letterSpacing="0.06em" fontWeight="700">NBR</text>

      {[
        { name: 'Auth Service', tech: 'Go', nbr: 5 },
        { name: 'Auth Gateway', tech: 'nginx', nbr: 2 },
      ].map(({ name, tech, nbr }, i) => (
        <g key={name}>
          <rect x="0" y={44 + i * 13} width="280" height="13" fill={i === 0 ? '#1A2D40' : 'transparent'} />
          <text x="10" y={53 + i * 13} fill={i === 0 ? '#90CDF4' : '#CBD5E0'} fontSize="8" fontFamily="system-ui,sans-serif" fontWeight={i === 0 ? '600' : '400'}>{name}</text>
          <text x="140" y={53 + i * 13} fill="#4A5568" fontSize="7.5" fontFamily="system-ui,sans-serif">{tech}</text>
          <text x="260" y={53 + i * 13} textAnchor="end" fill="#63B3ED" fontSize="8" fontFamily="system-ui,sans-serif" fontWeight="700">{nbr}</text>
        </g>
      ))}

      {/* Divider */}
      <line x1="0" y1="72" x2="280" y2="72" stroke="#2D3748" strokeWidth="4" />

      {/* Graph: pivot scenario */}
      <rect x="10" y="80" width="64" height="36" rx="6" fill="#1C2535" stroke="#2D3748" strokeWidth="1" />
      <text x="42" y="97" textAnchor="middle" fill="#CBD5E0" fontSize="7.5" fontFamily="system-ui,sans-serif" fontWeight="600">API GW</text>
      <text x="42" y="108" textAnchor="middle" fill="#718096" fontSize="7" fontFamily="system-ui,sans-serif">source</text>
      <line x1="74" y1="98" x2="94" y2="98" stroke="#2D3748" strokeWidth="1.5" />

      <rect x="94" y="76" width="76" height="44" rx="7" fill="#1A2D40" stroke="#63B3ED" strokeWidth="1.5" />
      <rect x="112" y="71" width="40" height="9" rx="4" fill="#3B82F6" />
      <text x="132" y="78" textAnchor="middle" fill="white" fontSize="6" fontFamily="system-ui,sans-serif" fontWeight="700">SELECTED</text>
      <text x="132" y="97" textAnchor="middle" fill="#90CDF4" fontSize="8.5" fontFamily="system-ui,sans-serif" fontWeight="700">Auth Service</text>
      <text x="132" y="109" textAnchor="middle" fill="#718096" fontSize="7" fontFamily="system-ui,sans-serif">5 neighbours</text>
      <line x1="170" y1="98" x2="192" y2="90" stroke="#2D3748" strokeWidth="1.5" />
      <line x1="170" y1="98" x2="192" y2="112" stroke="#2D3748" strokeWidth="1.5" />

      {/* Pivot target - highlighted on hover */}
      <rect x="192" y="80" width="76" height="34" rx="6" fill="#1C2535" strokeWidth="1.5" className="dp-pivot-highlight" stroke="#2D3748" />
      <text x="230" y="97" textAnchor="middle" fill="#CBD5E0" fontSize="7.5" fontFamily="system-ui,sans-serif" fontWeight="600">PostgreSQL</text>
      <text x="230" y="108" textAnchor="middle" fill="#4A5568" fontSize="7" fontFamily="system-ui,sans-serif">click to pivot</text>
      <rect x="192" y="118" width="76" height="24" rx="6" fill="#1C2535" stroke="#2D3748" strokeWidth="1" />
      <text x="230" y="133" textAnchor="middle" fill="#CBD5E0" fontSize="7.5" fontFamily="system-ui,sans-serif">Redis</text>

      {/* Cursor */}
      <g className="dp-pivot-cursor">
        <path d="M0,0 L0,12 L3.5,9 L6,15 L8,14 L5,8 L9.5,8 Z" fill="white" stroke="#0D1117" strokeWidth="0.5" />
      </g>

      <text x="140" y="146" textAnchor="middle" fill="#2D3748" fontSize="8" fontFamily="system-ui,sans-serif">click any neighbour to pivot to their dependencies</text>
    </svg>
  )
}

const STEPS = [
  {
    title: 'All Elements, One View',
    body: 'Dependencies lists every element across all your diagrams, sorted by how many connections it has. Search by name, type, or technology. Filter to a specific type to focus.',
    visual: 'layout' as const,
  },
  {
    title: 'Dependency Graph',
    body: 'Click any row to see its full dependency graph. Sources appear on the left, targets on the right, bidirectional connections on top. The neighbour count shows how connected it is.',
    visual: 'graph' as const,
  },
  {
    title: 'Pivot & Navigate',
    body: 'Click any neighbour card in the graph to instantly pivot to their dependencies. Drag the divider bar to give more space to the list or the graph.',
    visual: 'pivot' as const,
  },
]

export default function DependenciesOnboarding({  hasElements }: Props) {
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
          {current.visual === 'layout' && <SplitLayoutIllustration />}
          {current.visual === 'graph' && <DependencyGraphIllustration />}
          {current.visual === 'pivot' && <PivotIllustration />}

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
