import { useEffect, useState } from 'react'
import OnboardingHint from './OnboardingHint'

const STORAGE_KEY = `viewgrid_tutorial_v2_core`

interface Props {
   
  hasViews: boolean
}

// ── SVG helpers ─────────────────────────────────────────────────────────────

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

function GridCard({ x, y, name, label, stats, focused, wasd, cardClass, wasdClass, showConnect }: { x: number, y: number, name: string, label?: string, stats?: string, focused?: boolean, wasd?: string, cardClass?: string, wasdClass?: string, showConnect?: boolean }) {
  const accent = '#63B3ED'
  return (
    <g transform={`translate(${x}, ${y})`} className={cardClass}>
      <rect width="100" height="60" rx="6" fill="#1C2535" stroke={focused ? accent : 'rgba(255,255,255,0.14)'} strokeWidth="1" className="dg-card-rect" />
      <rect width="100" height="24" rx="0" fill="color-mix(in srgb, rgba(28, 37, 53, 0.7), black 20%)" y="36" />
      <text x="6" y="47" fill="#E2E8F0" fontSize="7" fontFamily="system-ui,sans-serif" fontWeight="600">{name}</text>
      {label && <text x="6" y="55" fill={accent} fontSize="5" fontFamily="system-ui,sans-serif" fontWeight="bold" style={{ textTransform: 'uppercase' }}>{label}</text>}
      {stats && <text x="94" y="55" textAnchor="end" fill="#718096" fontSize="5" fontFamily="system-ui,sans-serif">{stats}</text>}

      {showConnect && (
        <g transform="translate(86, 39)">
          <rect width="8" height="8" rx="1.5" fill="rgba(255,255,255,0.1)" stroke={accent} strokeWidth="0.5" />
          <path d="M2,4 L6,4 M4.5,2.5 L6,4 L4.5,5.5" fill="none" stroke={accent} strokeWidth="0.8" strokeLinecap="round" strokeLinejoin="round" />
        </g>
      )}

      {wasd && (
        <g transform="translate(42, -10)" className={wasdClass} style={{ transformOrigin: '8px 6px' }}>
          <rect width="16" height="12" rx="3" fill="#1A202C" stroke={accent} strokeWidth="0.8" />
          <text x="8" y="9" textAnchor="middle" fill="#E2E8F0" fontSize="7" fontFamily="system-ui,sans-serif" fontWeight="800">{wasd}</text>
        </g>
      )}
    </g>
  )
}

// Step 0 - Explore the Grid (WASD navigation)
function GridExploreIllustration() {
  return (
    <svg
      viewBox="0 0 280 150"
      xmlns="http://www.w3.org/2000/svg"
      style={{ width: '100%', height: 'auto', borderRadius: 10, display: 'block', overflow: 'hidden' }}
    >
      <defs>
        <style>{`
          .dg-container {
            animation: dgSlide 6s ease-in-out infinite;
          }
          @keyframes dgSlide {
            0%, 35% { transform: translateY(0); }
            50%, 85% { transform: translateY(70px); }
            95%, 100% { transform: translateY(0); }
          }
          .dg-focus-root .dg-card-rect {
            animation: dgFocusRoot 6s ease-in-out infinite;
          }
          @keyframes dgFocusRoot {
            0%, 45% { stroke: rgba(255,255,255,0.14); stroke-width: 1; filter: none; }
            50%, 85% { stroke: #63B3ED; stroke-width: 2; filter: drop-shadow(0 0 6px rgba(99,179,237,0.5)); }
            95%, 100% { stroke: rgba(255,255,255,0.14); stroke-width: 1; filter: none; }
          }
          .dg-focus-api .dg-card-rect {
            animation: dgFocusApi 6s ease-in-out infinite;
          }
          @keyframes dgFocusApi {
            0%, 35% { stroke: #63B3ED; stroke-width: 2; filter: drop-shadow(0 0 6px rgba(99,179,237,0.5)); }
            40%, 100% { stroke: rgba(255,255,255,0.14); stroke-width: 1; filter: none; }
          }
          .dg-w-flash {
            animation: dgWFlash 6s ease-in-out infinite;
          }
          @keyframes dgWFlash {
            0%, 15% { opacity: 0.4; transform: scale(1); }
            22%, 35% { opacity: 1; transform: scale(1.15); filter: drop-shadow(0 0 8px #63B3ED); }
            42%, 100% { opacity: 0.4; transform: scale(1); }
          }
        `}</style>
      </defs>
      <rect width="280" height="150" fill="#0D1117" rx="8" />
      <GridDots prefix="grid" />

      <g className="dg-container">
        {/* Parent View */}
        <GridCard
          x={90} y={5}
          name="Root System" label="Context" stats="5n 3e"
          wasd="W"
          cardClass="dg-focus-root"
          wasdClass="dg-w-flash"
        />
        {/* Current View (Initially centered at 75) */}
        <GridCard
          x={90} y={75}
          name="API Gateway" label="Container" stats="8n 12e"
          cardClass="dg-focus-api"
        />
        {/* Sibling View */}
        <GridCard
          x={210} y={75}
          name="Analytics" label="Context" stats="2n 1e"
          wasd="D"
          wasdClass="dg-d-key"
        />
      </g>

      <text x="140" y="141" textAnchor="middle" fill="#2D3748" fontSize="8" fontFamily="system-ui,sans-serif">navigate with WASD or arrow keys</text>
    </svg>
  )
}

// Step 1 - Build Hierarchy (+ buttons)
function HierarchyIllustration() {
  return (
    <svg
      viewBox="0 0 280 150"
      xmlns="http://www.w3.org/2000/svg"
      style={{ width: '100%', height: 'auto', borderRadius: 10, display: 'block', overflow: 'hidden' }}
    >
      <defs>
        <style>{`
          .dg-plus-group { animation: dgPlusFade 6s infinite; }
          @keyframes dgPlusFade {
            0%, 15%, 85%, 100% { opacity: 0; }
            20%, 80% { opacity: 1; }
          }
          .dg-hier-cursor {
            animation: dgHierCursorMove 6s ease-in-out infinite;
          }
          @keyframes dgHierCursorMove {
            0%, 25% { transform: translate(220px, 120px); opacity: 0; }
            30% { opacity: 1; }
            45% { transform: translate(145px, 75px); opacity: 1; }
            50% { transform: translate(145px, 75px) scale(0.85); opacity: 1; }
            55%, 85% { transform: translate(145px, 75px) scale(1); opacity: 0; }
            100% { opacity: 0; }
          }
          .dg-new-card-hier {
            animation: dgNewCardSnap 6s infinite;
          }
          @keyframes dgNewCardSnap {
            0%, 49% { opacity: 0; }
            50%, 100% { opacity: 1; }
          }
        `}</style>
      </defs>
      <rect width="280" height="150" fill="#0D1117" rx="8" />
      <GridDots prefix="hier" />

      {/* Primary card shifted left */}
      <GridCard x={40} y={45} name="Auth Service" label="Container" stats="3n 4e" cardClass="dg-focus-api" />

      {/* New card that appears on click */}
      <GridCard x={150} y={45} name="User DB" label="Database" stats="0n" cardClass="dg-new-card-hier" />

      <g className="dg-plus-group">
        {/* Top + */}
        <circle cx="90" cy="40" r="8" fill="#1F2937" stroke="#63B3ED" strokeWidth="1" />
        <text x="90" y="43" textAnchor="middle" fill="#63B3ED" fontSize="10" fontFamily="system-ui,sans-serif">+</text>

        {/* Bottom + */}
        <circle cx="90" cy="110" r="8" fill="#1F2937" stroke="#63B3ED" strokeWidth="1" />
        <text x="90" y="113" textAnchor="middle" fill="#63B3ED" fontSize="10" fontFamily="system-ui,sans-serif">+</text>

        {/* Left + */}
        <circle cx="35" cy="75" r="8" fill="#1F2937" stroke="#63B3ED" strokeWidth="1" />
        <text x="35" y="78" textAnchor="middle" fill="#63B3ED" fontSize="10" fontFamily="system-ui,sans-serif">+</text>

        {/* Right + (Being clicked) */}
        <circle cx="145" cy="75" r="8" fill="#1F2937" stroke="#63B3ED" strokeWidth="1" />
        <text x="145" y="78" textAnchor="middle" fill="#63B3ED" fontSize="10" fontFamily="system-ui,sans-serif">+</text>
      </g>

      {/* Cursor */}
      <g className="dg-hier-cursor">
        <path d="M0,0 L0,14 L4,10 L7,17 L9,16 L6,9 L11,9 Z" fill="white" stroke="#0D1117" strokeWidth="0.6" />
      </g>

      <text x="140" y="141" textAnchor="middle" fill="#2D3748" fontSize="8" fontFamily="system-ui,sans-serif">click the + buttons to build your hierarchy</text>
    </svg>
  )
}

// Step 2 - Connect & Organize
function ConnectIllustration() {
  return (
    <svg
      viewBox="0 0 280 150"
      xmlns="http://www.w3.org/2000/svg"
      style={{ width: '100%', height: 'auto', borderRadius: 10, display: 'block', overflow: 'hidden' }}
    >
      <defs>
        <style>{`
          .dg-conn-arrow { animation: dgConnDraw 4s infinite; }
          @keyframes dgConnDraw {
            0%, 25% { stroke-dashoffset: 100; opacity: 0; }
            45%, 85% { stroke-dashoffset: 0; opacity: 1; }
            95%, 100% { opacity: 0; }
          }
        `}</style>
        <marker id="dg-head" markerWidth="6" markerHeight="6" refX="5" refY="3" orient="auto">
          <path d="M0,0 L0,6 L6,3 z" fill="#63B3ED" />
        </marker>
      </defs>
      <rect width="280" height="150" fill="#0D1117" rx="8" />
      <GridDots prefix="conn" />

      {/* Activated card with connect button */}
      <GridCard
        x={30} y={45}
        name="Database" label="Database" stats="1n"
        showConnect
      />

      <GridCard x={150} y={45} name="Cache" label="Component" stats="0n" />

      {/* Connection arrow starting from the connect button area of the first card (approx x=124, y=88 in svg space) */}
      <path
        d="M130 75 C 135 75, 140 75, 150 75"
        stroke="#63B3ED"
        strokeWidth="1.5"
        strokeDasharray="100"
        strokeDashoffset="100"
        markerEnd="url(#dg-head)"
        className="dg-conn-arrow"
      />

      <text x="140" y="141" textAnchor="middle" fill="#2D3748" fontSize="8" fontFamily="system-ui,sans-serif">link views together with the arrow icon</text>
    </svg>
  )
}

// ── Steps data ───────────────────────────────────────────────────────────────

const STEPS = [
  {
    title: 'Explore the Grid',
    body: 'Your views are laid out in a visual grid showing their hierarchy. Navigate quickly between them using WASD or your arrow keys.',
    visual: 'grid' as const,
  },
  {
    title: 'Build Your Hierarchy',
    body: 'Hover any view to reveal + buttons. Use them to instantly create new parent, child, or sibling views in your C4 map.',
    visual: 'hierarchy' as const,
  },
  {
    title: 'Connect & Organize',
    body: 'Use the arrow icon to link views together. The ··· menu lets you rename, move between levels, or share your work.',
    visual: 'connect' as const,
  },
]

// ── Main component ───────────────────────────────────────────────────────────

export default function ViewsGridOnboarding({  hasViews }: Props) {
  const [visible, setVisible] = useState(false)
  const [step, setStep] = useState(0)

  useEffect(() => {
    if (!hasViews) return
    if (!localStorage.getItem(STORAGE_KEY)) {
      setVisible(true)
    }
  }, [ hasViews])

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
        current.visual === 'grid' ? (
          <GridExploreIllustration />
        ) : current.visual === 'hierarchy' ? (
          <HierarchyIllustration />
        ) : (
          <ConnectIllustration />
        )
      }
      onBack={() => setStep((s) => Math.max(0, s - 1))}
      onNext={() => setStep((s) => Math.min(STEPS.length - 1, s + 1))}
      onSelectStep={setStep}
      onDismiss={dismiss}
    />
  )
}
