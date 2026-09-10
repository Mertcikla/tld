import { useEffect, useState } from 'react'
import OnboardingHint from './OnboardingHint'

const STORAGE_KEY = `explore_tutorial_v1_core`

interface Props {
   
  hasLinkedNodes: boolean
}

// ── SVG helpers ────────────────────────────────────────────────────────────────

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

// Step 1 - animated zoom-in / zoom-out illustration
function ZoomAnimation() {
  return (
    <svg
      viewBox="0 0 280 150"
      xmlns="http://www.w3.org/2000/svg"
      style={{ width: '100%', height: 'auto', borderRadius: 10, display: 'block', overflow: 'hidden' }}
    >
      <defs>
        <style>{`
          .eo-outer {
            transform-box: view-box;
            transform-origin: 79% 52%;
            animation: eoOuterZoom 5s ease-in-out infinite;
          }
          @keyframes eoOuterZoom {
            0%,  18%  { transform: scale(1);   opacity: 1; }
            40%, 62%  { transform: scale(2.4); opacity: 0; }
            82%, 100% { transform: scale(1);   opacity: 1; }
          }
          .eo-inner {
            animation: eoInnerFade 5s ease-in-out infinite;
          }
          @keyframes eoInnerFade {
            0%,  32%  { opacity: 0; }
            50%, 60%  { opacity: 1; }
            77%, 100% { opacity: 0; }
          }
        `}</style>
        <marker id="eo-arr" markerWidth="8" markerHeight="8" refX="6" refY="3" orient="auto">
          <path d="M0,0 L0,6 L8,3 z" fill="#4A5568" />
        </marker>
        <marker id="eo-arr-dash" markerWidth="8" markerHeight="8" refX="6" refY="3" orient="auto">
          <path d="M0,0 L0,6 L8,3 z" fill="#374151" />
        </marker>
      </defs>

      {/* Background */}
      <rect width="280" height="150" fill="#0D1117" rx="8" />
      <GridDots prefix="z-o" />

      {/* ── Outer scene: two nodes connected ── */}
      <g className="eo-outer">
        {/* Node A */}
        <rect x="22" y="47" width="90" height="56" rx="7" fill="#1C2535" stroke="#2D3748" strokeWidth="1" />
        <text x="67" y="70" textAnchor="middle" fill="#94A3B8" fontSize="11" fontFamily="system-ui,sans-serif" fontWeight="600">Auth</text>
        <text x="67" y="83" textAnchor="middle" fill="#475569" fontSize="8" fontFamily="system-ui,sans-serif">service</text>

        {/* Arrow */}
        <line x1="112" y1="75" x2="163" y2="75" stroke="#2D3748" strokeWidth="1.5" markerEnd="url(#eo-arr)" />

        {/* Node B - has child link (blue dot) */}
        <rect x="167" y="47" width="90" height="56" rx="7" fill="#1C2535" stroke="#2D3748" strokeWidth="1" />
        <text x="212" y="70" textAnchor="middle" fill="#94A3B8" fontSize="11" fontFamily="system-ui,sans-serif" fontWeight="600">API</text>
        <text x="212" y="83" textAnchor="middle" fill="#475569" fontSize="8" fontFamily="system-ui,sans-serif">gateway</text>

        {/* Blue dot with ripple */}
        <circle cx="249" cy="54" r="4" fill="#3B82F6" />
        <circle cx="249" cy="54" r="4" fill="none" stroke="#3B82F6" strokeWidth="1.5">
          <animate attributeName="r" values="4;16" dur="2s" repeatCount="indefinite" />
          <animate attributeName="opacity" values="0.7;0" dur="2s" repeatCount="indefinite" />
        </circle>

        {/* Hint label */}
        <text x="212" y="35" textAnchor="middle" fill="#3B82F6" fontSize="7.5" fontFamily="system-ui,sans-serif">zoom in ↑</text>
      </g>

      {/* ── Inner scene: child diagram ── */}
      <g className="eo-inner" opacity="0">
        <rect width="280" height="150" fill="#0D1117" />
        <GridDots prefix="z-i" />
        <text x="140" y="18" textAnchor="middle" fill="#3B82F6" fontSize="8.5" fontFamily="system-ui,sans-serif" fontWeight="700" letterSpacing="0.05em">API GATEWAY - INTERNALS</text>

        <rect x="15" y="35" width="72" height="46" rx="6" fill="#1C2535" stroke="#2D3748" strokeWidth="1" />
        <text x="51" y="55" textAnchor="middle" fill="#94A3B8" fontSize="10" fontFamily="system-ui,sans-serif" fontWeight="600">Router</text>
        <text x="51" y="68" textAnchor="middle" fill="#475569" fontSize="7.5" fontFamily="system-ui,sans-serif">container</text>

        <rect x="104" y="57" width="72" height="46" rx="6" fill="#1C2535" stroke="#2D3748" strokeWidth="1" />
        <text x="140" y="77" textAnchor="middle" fill="#94A3B8" fontSize="10" fontFamily="system-ui,sans-serif" fontWeight="600">Auth MW</text>
        <text x="140" y="90" textAnchor="middle" fill="#475569" fontSize="7.5" fontFamily="system-ui,sans-serif">component</text>

        <rect x="193" y="35" width="72" height="46" rx="6" fill="#1C2535" stroke="#2D3748" strokeWidth="1" />
        <text x="229" y="55" textAnchor="middle" fill="#94A3B8" fontSize="10" fontFamily="system-ui,sans-serif" fontWeight="600">Cache</text>
        <text x="229" y="68" textAnchor="middle" fill="#475569" fontSize="7.5" fontFamily="system-ui,sans-serif">database</text>

        <line x1="87" y1="62" x2="107" y2="75" stroke="#374151" strokeWidth="1.5" strokeDasharray="4,2" markerEnd="url(#eo-arr-dash)" />
        <line x1="176" y1="75" x2="196" y2="62" stroke="#374151" strokeWidth="1.5" strokeDasharray="4,2" markerEnd="url(#eo-arr-dash)" />
      </g>
    </svg>
  )
}

// Step 0 - pan/navigate illustration
function PanIllustration() {
  return (
    <svg
      viewBox="0 0 280 140"
      xmlns="http://www.w3.org/2000/svg"
      style={{ width: '100%', height: 'auto', borderRadius: 10, display: 'block' }}
    >
      <rect width="280" height="140" fill="#0D1117" rx="8" />
      <GridDots prefix="p" />

      <rect x="20" y="30" width="70" height="44" rx="6" fill="#1C2535" stroke="#2D3748" strokeWidth="1" />
      <text x="55" y="57" textAnchor="middle" fill="#94A3B8" fontSize="9" fontFamily="system-ui,sans-serif">System A</text>

      <rect x="105" y="48" width="70" height="44" rx="6" fill="#1C2535" stroke="#2D3748" strokeWidth="1" />
      <text x="140" y="75" textAnchor="middle" fill="#94A3B8" fontSize="9" fontFamily="system-ui,sans-serif">System B</text>

      <rect x="190" y="30" width="70" height="44" rx="6" fill="#1C2535" stroke="#2D3748" strokeWidth="1" />
      <text x="225" y="57" textAnchor="middle" fill="#94A3B8" fontSize="9" fontFamily="system-ui,sans-serif">System C</text>

      {/* Directional arrows */}
      <text x="140" y="19" textAnchor="middle" fill="#4A5568" fontSize="15" fontFamily="system-ui,sans-serif">↑</text>
      <text x="140" y="134" textAnchor="middle" fill="#4A5568" fontSize="15" fontFamily="system-ui,sans-serif">↓</text>
      <text x="8" y="77" textAnchor="middle" fill="#4A5568" fontSize="15" fontFamily="system-ui,sans-serif">←</text>
      <text x="273" y="77" textAnchor="middle" fill="#4A5568" fontSize="15" fontFamily="system-ui,sans-serif">→</text>

      <text x="140" y="118" textAnchor="middle" fill="#2D3748" fontSize="8.5" fontFamily="system-ui,sans-serif">scroll to zoom · drag to pan</text>
    </svg>
  )
}

// Step 2 - zoom-out / breadcrumb illustration
function ZoomOutIllustration() {
  return (
    <svg
      viewBox="0 0 280 140"
      xmlns="http://www.w3.org/2000/svg"
      style={{ width: '100%', height: 'auto', borderRadius: 10, display: 'block' }}
    >
      <rect width="280" height="140" fill="#0D1117" rx="8" />
      <GridDots prefix="b" />

      {/* Outer parent box (dashed) */}
      <rect x="10" y="10" width="260" height="120" rx="10" fill="none" stroke="#2D3748" strokeWidth="1.5" strokeDasharray="6,3" />
      <text x="26" y="25" fill="#374151" fontSize="8.5" fontFamily="system-ui,sans-serif">Parent Diagram</text>

      {/* Inner focused box */}
      <rect x="50" y="30" width="180" height="88" rx="8" fill="#111827" stroke="#3B82F6" strokeWidth="1.5" />
      <text x="140" y="46" textAnchor="middle" fill="#3B82F6" fontSize="8" fontFamily="system-ui,sans-serif" fontWeight="600">Current View</text>

      {/* Inner nodes */}
      <rect x="62" y="52" width="55" height="50" rx="5" fill="#1C2535" stroke="#2D3748" strokeWidth="1" />
      <text x="89" y="81" textAnchor="middle" fill="#94A3B8" fontSize="8" fontFamily="system-ui,sans-serif">Service A</text>

      <rect x="163" y="52" width="55" height="50" rx="5" fill="#1C2535" stroke="#2D3748" strokeWidth="1" />
      <text x="190" y="81" textAnchor="middle" fill="#94A3B8" fontSize="8" fontFamily="system-ui,sans-serif">Service B</text>

      {/* Breadcrumb trail */}
      <text x="10" y="136" fill="#374151" fontSize="7.5" fontFamily="system-ui,sans-serif">All Diagrams  /  Parent  /  Current View</text>

      {/* Zoom-out indicator */}
      <circle cx="256" cy="24" r="10" fill="none" stroke="#3B82F6" strokeWidth="1.5" />
      <line x1="252" y1="24" x2="260" y2="24" stroke="#3B82F6" strokeWidth="1.5" />
      <line x1="262" y1="30" x2="267" y2="35" stroke="#3B82F6" strokeWidth="1.5" />
    </svg>
  )
}

// ── Steps data ─────────────────────────────────────────────────────────────────

const STEPS = [
  {
    title: 'Navigate the Canvas',
    body: 'Scroll to zoom in and out. Drag to pan. Your entire architecture is laid out on one infinite canvas.',
    visual: 'pan' as const,
  },
  {
    title: 'Zoom In to Dive Deeper',
    body: 'Nodes with a blue dot \u2022 have a linked sub-diagram. Zoom in on one - the view automatically transitions inside.',
    visual: 'zoom' as const,
  },
  {
    title: 'Zoom Out to Go Back',
    body: 'Zoom out past the diagram boundary to return to the parent. Follow the breadcrumb at the top to jump anywhere.',
    visual: 'zoomout' as const,
  },
]

// ── Main component ─────────────────────────────────────────────────────────────

export default function ExploreOnboarding({  hasLinkedNodes }: Props) {
  const [visible, setVisible] = useState(false)
  const [step, setStep] = useState(0)

  useEffect(() => {
    if (!hasLinkedNodes) return
    if (!localStorage.getItem(STORAGE_KEY)) {
      setVisible(true)
    }
  }, [ hasLinkedNodes])

  const dismiss = () => {
    localStorage.setItem(STORAGE_KEY, '1')
    setVisible(false)
  }

  if (!visible) return null

  const current = STEPS[step]

  return (
    <OnboardingHint
      step={step}
      totalSteps={STEPS.length}
      title={current.title}
      body={current.body}
      visual={
        current.visual === 'pan' ? (
          <PanIllustration />
        ) : current.visual === 'zoom' ? (
          <ZoomAnimation />
        ) : (
          <ZoomOutIllustration />
        )
      }
      onBack={() => setStep((s) => Math.max(0, s - 1))}
      onNext={() => setStep((s) => Math.min(STEPS.length - 1, s + 1))}
      onSelectStep={setStep}
      onDismiss={dismiss}
    />
  )
}
