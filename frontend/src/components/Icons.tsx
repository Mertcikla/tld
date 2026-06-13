import React from 'react'


export function ZoomOutIcon({ size = 14, strokeWidth = 3 }: { size?: number, strokeWidth?: number }) {
  return (
    <svg width={size} height={size} viewBox="0 0 24 24" fill="none" stroke="currentColor"
      strokeWidth={strokeWidth} strokeLinecap="round" strokeLinejoin="round">
      <path d="M19 11a8 8 0 1 1-16 0 8 8 0 0 1 16 0M21 21l-4.343-4.343M8 11h6" />
    </svg>
  )
}

export function ZoomInIcon({ size = 14, strokeWidth = 3 }: { size?: number, strokeWidth?: number }) {
  return (
    <svg width={size} height={size} viewBox="0 0 24 24" fill="none" stroke="currentColor"
      strokeWidth={strokeWidth} strokeLinecap="round" strokeLinejoin="round">
      <path d="M19 11a8 8 0 1 1-16 0 8 8 0 0 1 16 0M21 21l-4.343-4.343M11 8v6M8 11h6" />
    </svg>
  )
}

export function AddElementIcon({ size = 12, strokeWidth = 3 }: { size?: number, strokeWidth?: number }) {
  return (
    <svg width={size} height={size} viewBox="0 0 24 24" fill="none" stroke="currentColor"
      strokeWidth={strokeWidth} strokeLinecap="round" strokeLinejoin="round">
      <line x1="12" y1="5" x2="12" y2="19" />
      <line x1="5" y1="12" x2="19" y2="12" />
    </svg>
  )
}

export function FitViewIcon({ size = 14, strokeWidth = 2.5 }: { size?: number, strokeWidth?: number }) {
  return (
    <svg width={size} height={size} viewBox="0 0 24 24" fill="none" stroke="currentColor"
      strokeWidth={strokeWidth} strokeLinecap="round" strokeLinejoin="round">
      <path d="M3 8V5a2 2 0 0 1 2-2h3" />
      <path d="M16 3h3a2 2 0 0 1 2 2v3" />
      <path d="M21 16v3a2 2 0 0 1-2 2h-3" />
      <path d="M8 21H5a2 2 0 0 1-2-2v-3" />
      <rect x="8" y="8" width="8" height="8" rx="1.5" />
    </svg>
  )
}

export function ShareIcon({ size = 14, strokeWidth = 2.5 }: { size?: number, strokeWidth?: number }) {
  return (
    <svg width={size} height={size} viewBox="0 0 24 24" fill="none" stroke="currentColor"
      strokeWidth={strokeWidth} strokeLinecap="round" strokeLinejoin="round">
      <path d="M4 12v8a2 2 0 0 0 2 2h12a2 2 0 0 0 2-2v-8" />
      <polyline points="16 6 12 2 8 6" />
      <line x1="12" y1="2" x2="12" y2="15" />
    </svg>
  )
}

export function TrashIcon({ size = 13, strokeWidth = 2.5 }: { size?: number, strokeWidth?: number }) {
  return (
    <svg width={size} height={size} viewBox="0 0 24 24" fill="none" stroke="currentColor"
      strokeWidth={strokeWidth} strokeLinecap="round" strokeLinejoin="round">
      <polyline points="3 6 5 6 21 6" />
      <path d="M19 6l-1 14a2 2 0 0 1-2 2H8a2 2 0 0 1-2-2L5 6" />
      <path d="M10 11v6M14 11v6" />
      <path d="M9 6V4a1 1 0 0 1 1-1h4a1 1 0 0 1 1 1v2" />
    </svg>
  )
}

export function DrawIcon({ size = 13, strokeWidth = 2.5 }: { size?: number, strokeWidth?: number }) {
  return (
    <svg width={size} height={size} viewBox="0 0 24 24" fill="none" stroke="currentColor"
      strokeWidth={strokeWidth} strokeLinecap="round" strokeLinejoin="round">
      <path d="M17 3a2.828 2.828 0 1 1 4 4L7.5 20.5 2 22l1.5-5.5L17 3z" />
    </svg>
  )
}

export function GitIcon({ size = 14, strokeWidth = 2 }: { size?: number, strokeWidth?: number }) {
  return (
    <svg width={size} height={size} viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth={strokeWidth} strokeLinecap="round" strokeLinejoin="round">
      <circle cx="12" cy="18" r="3" />
      <circle cx="6" cy="6" r="3" />
      <circle cx="18" cy="6" r="3" />
      <path d="M18 9v2c0 1.1-.9 2-2 2H8a2 2 0 0 1-2-2V9" />
      <path d="M12 13V15" />
    </svg>
  )
}

export function MergeIcon({ size = 14, strokeWidth = 2.4 }: { size?: number, strokeWidth?: number }) {
  return (
    <svg width={size} height={size} viewBox="0 0 24 24" fill="none" stroke="currentColor"
      strokeWidth={strokeWidth} strokeLinecap="round" strokeLinejoin="round">
      <circle cx="7" cy="5" r="2.5" />
      <circle cx="7" cy="19" r="2.5" />
      <circle cx="17" cy="19" r="2.5" />
      <path d="M7 7.5v9" />
      <path d="M7 7.5c0 4 3 6 10 9" />
    </svg>
  )
}

export function MoveSourceIcon({ size = 13, strokeWidth = 2.5 }: { size?: number, strokeWidth?: number }) {
  return (
    <svg width={size} height={size} viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth={strokeWidth} strokeLinecap="round" strokeLinejoin="round">
      <circle cx="5" cy="12" r="2" />
      <line x1="7" y1="12" x2="17" y2="12" />
      <polyline points="14 9 17 12 14 15" />
    </svg>
  )
}

export function MoveTargetIcon({ size = 13, strokeWidth = 2.5 }: { size?: number, strokeWidth?: number }) {
  return (
    <svg width={size} height={size} viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth={strokeWidth} strokeLinecap="round" strokeLinejoin="round">
      <line x1="7" y1="12" x2="17" y2="12" />
      <polyline points="14 9 17 12 14 15" />
      <circle cx="19" cy="12" r="2" />
    </svg>
  )
}

export function MarkdownIcon({ size = 26 }: { size?: number, strokeWidth?: number }) {
  return (
    <svg width={size} height={size} viewBox="0 0 24 24" fill="currentColor">
      <path d="M20.553 18.15H3.447a1.443 1.443 0 0 1-1.442-1.441V7.291c0-.795.647-1.441 1.442-1.441h17.105c.795 0 1.442.646 1.442 1.441v9.418a1.441 1.441 0 0 1-1.441 1.441zM6.811 15.268V11.52l1.922 2.402 1.922-2.402v3.748h1.922V8.732h-1.922l-1.922 2.403-1.922-2.403H4.889v6.535h1.922zM19.688 12h-1.922V8.732h-1.923V12h-1.922l2.884 3.364L19.688 12z" />
    </svg>
  )
}
export function NavigationIcon({ size = 14, strokeWidth = 1.8 }: { size?: number, strokeWidth?: number }) {
  return (
    <svg width={size} height={size} viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth={strokeWidth} strokeLinecap="round" strokeLinejoin="round">
      <rect x="8" y="2" width="8" height="5" rx="1.5" fill="none" />
      <line x1="12" y1="7" x2="12" y2="11.5" />
      <line x1="4.5" y1="11.5" x2="19.5" y2="11.5" />
      <line x1="4.5" y1="11.5" x2="4.5" y2="14" />
      <rect x="1.5" y="14" width="6" height="4.5" rx="1.5" fill="none" />
      <line x1="19.5" y1="11.5" x2="19.5" y2="14" />
      <rect x="16.5" y="14" width="6" height="4.5" rx="1.5" fill="none" />
    </svg>
  )
}

export function LibraryIcon({ size = 14 }: { size?: number }) {
  return (
    <svg width={size} height={size} viewBox="0 0 14 14" fill="none">
      <rect x="1" y="1" width="5" height="5" rx="1" fill="currentColor" opacity="0.85" />
      <rect x="8" y="1" width="5" height="5" rx="1" fill="currentColor" opacity="0.85" />
      <rect x="1" y="8" width="5" height="5" rx="1" fill="currentColor" opacity="0.85" />
      <rect x="8" y="8" width="5" height="5" rx="1" fill="currentColor" opacity="0.85" />
    </svg>
  )
}

export function ExpandExtrasIcon({ size = 14, strokeWidth = 2.5 }: { size?: number, strokeWidth?: number }) {
  return (
    <svg width={size} height={size} viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth={strokeWidth} strokeLinecap="round" strokeLinejoin="round">
      <polyline points="8 6 14 12 8 18" />
      <polyline points="13 6 19 12 13 18" />
    </svg>
  )
}

export function CollapseExtrasIcon({ size = 14, strokeWidth = 2.5 }: { size?: number, strokeWidth?: number }) {
  return (
    <svg width={size} height={size} viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth={strokeWidth} strokeLinecap="round" strokeLinejoin="round">
      <polyline points="16 6 10 12 16 18" />
      <polyline points="11 6 5 12 11 18" />
    </svg>
  )
}

export function EyeIcon({ size = 14, strokeWidth = 2.5 }: { size?: number, strokeWidth?: number }) {
  return (
    <svg width={size} height={size} viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth={strokeWidth} strokeLinecap="round" strokeLinejoin="round">
      <path d="M1 12s4-8 11-8 11 8 11 8-4 8-11 8-11-8-11-8z" />
      <circle cx="12" cy="12" r="3" />
    </svg>
  )
}

export function EyeOffIcon({ size = 14, strokeWidth = 2.5 }: { size?: number, strokeWidth?: number }) {
  return (
    <svg width={size} height={size} viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth={strokeWidth} strokeLinecap="round" strokeLinejoin="round">
      <path d="M17.94 17.94A10.07 10.07 0 0 1 12 20c-7 0-11-8-11-8a18.45 18.45 0 0 1 5.06-5.94M9.9 4.24A9.12 9.12 0 0 1 12 4c7 0 11 8 11 8a18.5 18.5 0 0 1-2.16 3.19m-6.72-1.07a3 3 0 1 1-4.24-4.24" />
      <line x1="1" y1="1" x2="23" y2="23" />
    </svg>
  )
}

export function ImportIcon({ size = 12, strokeWidth = 2.5 }: { size?: number, strokeWidth?: number }) {
  return (
    <svg width={size} height={size} viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth={strokeWidth} strokeLinecap="round" strokeLinejoin="round">
      <path d="M21 15v4a2 2 0 0 1-2 2H5a2 2 0 0 1-2-2v-4" />
      <polyline points="7 10 12 15 17 10" />
      <line x1="12" y1="15" x2="12" y2="3" />
    </svg>
  )
}

export function ExportIcon({ size = 12, strokeWidth = 2.5 }: { size?: number, strokeWidth?: number }) {
  return (
    <svg width={size} height={size} viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth={strokeWidth} strokeLinecap="round" strokeLinejoin="round">
      <path d="M21 15v4a2 2 0 0 1-2 2H5a2 2 0 0 1-2-2v-4" />
      <polyline points="17 8 12 3 7 8" />
      <line x1="12" y1="3" x2="12" y2="15" />
    </svg>
  )
}

export function ImageUploadIcon({ size = 14, strokeWidth = 2.3 }: { size?: number, strokeWidth?: number }) {
  return (
    <svg width={size} height={size} viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth={strokeWidth} strokeLinecap="round" strokeLinejoin="round">
      <rect x="3" y="5" width="18" height="16" rx="2" />
      <circle cx="8.5" cy="10.5" r="1.5" />
      <path d="M21 16l-4.5-4.5L8 20" />
      <path d="M14 8h5" />
      <path d="M16.5 5.5v5" />
    </svg>
  )
}

export function LayerIcon({ size = 14, strokeWidth = 2.5 }: { size?: number, strokeWidth?: number }) {
  return (
    <svg width={size} height={size} viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth={strokeWidth} strokeLinecap="round" strokeLinejoin="round">
      <polygon points="12 2 2 7 12 12 22 7 12 2" />
      <polyline points="2 17 12 22 22 17" />
      <polyline points="2 12 12 17 22 12" />
    </svg>
  )
}

export function ChevronLeftIcon({ size = 14, strokeWidth = 3 }: { size?: number, strokeWidth?: number }) {
  return (
    <svg width={size} height={size} viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth={strokeWidth} strokeLinecap="round" strokeLinejoin="round">
      <path d="M15 18l-6-6 6-6" />
    </svg>
  )
}

export function ChevronRightIcon({ size = 14, strokeWidth = 3 }: { size?: number, strokeWidth?: number }) {
  return (
    <svg width={size} height={size} viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth={strokeWidth} strokeLinecap="round" strokeLinejoin="round">
      <path d="M9 6l6 6-6 6" />
    </svg>
  )
}

export function ChevronDownIcon({ size = 14, strokeWidth = 3 }: { size?: number, strokeWidth?: number }) {
  return (
    <svg width={size} height={size} viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth={strokeWidth} strokeLinecap="round" strokeLinejoin="round">
      <path d="M6 9l6 6 6-6" />
    </svg>
  )
}

export function ChevronUpIcon({ size = 14, strokeWidth = 3 }: { size?: number, strokeWidth?: number }) {
  return (
    <svg width={size} height={size} viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth={strokeWidth} strokeLinecap="round" strokeLinejoin="round">
      <path d="M18 15l-6-6-6 6" />
    </svg>
  )
}

export function TagsIcon({ size = 14, strokeWidth = 2.5 }: { size?: number, strokeWidth?: number }) {
  return (
    <svg width={size} height={size} viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth={strokeWidth} strokeLinecap="round" strokeLinejoin="round">
      <path d="M20.59 13.41l-7.17 7.17a2 2 0 0 1-2.83 0L2 12V2h10l8.59 8.59a2 2 0 0 1 0 2.82z" />
      <line x1="7" y1="7" x2="7.01" y2="7" />
    </svg>
  )
}

export function GridIcon({ size = 12, strokeWidth = 2.5 }: { size?: number, strokeWidth?: number }) {
  return (
    <svg width={size} height={size} viewBox="0 0 24 24" fill="none" stroke="currentColor"
      strokeWidth={strokeWidth} strokeLinecap="round" strokeLinejoin="round">
      <rect x="3" y="3" width="18" height="18" rx="2" />
      <line x1="3" y1="9" x2="21" y2="9" />
      <line x1="3" y1="15" x2="21" y2="15" />
      <line x1="9" y1="3" x2="9" y2="21" />
      <line x1="15" y1="3" x2="15" y2="21" />
    </svg>
  )
}

export function ReloadIcon({ size = 14, strokeWidth = 2.2 }: { size?: number, strokeWidth?: number }) {
  return (
    <svg width={size} height={size} viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth={strokeWidth} strokeLinecap="round" strokeLinejoin="round">
      <path d="M21 12a9 9 0 1 1-2.64-6.36" />
      <polyline points="21 3 21 9 15 9" />
    </svg>
  )
}

export function SaveIcon({ size = 14, strokeWidth = 2.1 }: { size?: number, strokeWidth?: number }) {
  return (
    <svg width={size} height={size} viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth={strokeWidth} strokeLinecap="round" strokeLinejoin="round">
      <path d="M5 3h11l3 3v15H5z" />
      <path d="M8 3v6h8V3" />
      <path d="M9 17h6" />
    </svg>
  )
}

export function CloseIcon({ size = 14, strokeWidth = 2.4 }: { size?: number, strokeWidth?: number }) {
  return (
    <svg width={size} height={size} viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth={strokeWidth} strokeLinecap="round" strokeLinejoin="round">
      <path d="M18 6 6 18" />
      <path d="M6 6l12 12" />
    </svg>
  )
}

export function FocusIcon({ size = 14, strokeWidth = 2.5 }: { size?: number, strokeWidth?: number }) {
  return (
    <svg width={size} height={size} viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth={strokeWidth} strokeLinecap="round" strokeLinejoin="round">
      <circle cx="12" cy="12" r="10" />
      <circle cx="12" cy="12" r="3" />
    </svg>
  )
}

/**
 * Signal-bars icon reflecting the noise gate level.
 * densityLevel: -2 (Quiet, 1 bar) → 2 (Full, 5 bars).
 * Filled bars use currentColor; unfilled bars are dimmed.
 */
export function NoiseGateIcon({ size = 14, level = 0 }: { size?: number, level?: number }) {
  // Map density level (-2..2) to filled bar count (1..5)
  const filledBars = Math.max(1, Math.min(5, level + 3))
  const barCount = 5
  const barWidth = 2.4
  const gap = 1.6
  const totalWidth = barCount * barWidth + (barCount - 1) * gap
  const startX = (24 - totalWidth) / 2

  return (
    <svg width={size} height={size} viewBox="0 0 24 24" fill="none">
      {Array.from({ length: barCount }, (_, i) => {
        const barHeight = 4 + i * 3.2
        const x = startX + i * (barWidth + gap)
        const y = 20 - barHeight
        const filled = i < filledBars
        return (
          <rect
            key={i}
            x={x}
            y={y}
            width={barWidth}
            height={barHeight}
            rx={1}
            fill={filled ? 'currentColor' : 'currentColor'}
            opacity={filled ? 1 : 0.2}
          />
        )
      })}
    </svg>
  )
}
