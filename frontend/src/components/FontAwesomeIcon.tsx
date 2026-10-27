import type { IconDefinition } from '@fortawesome/free-solid-svg-icons'

export default function FontAwesomeIcon({ icon, size = 13, viewBox }: { icon: IconDefinition; size?: number; viewBox?: string }) {
  const [width, height, , , pathData] = icon.icon
  const paths = Array.isArray(pathData) ? pathData : [pathData]
  return (
    <svg width={size} height={size} viewBox={viewBox ?? `0 0 ${width} ${height}`} fill="currentColor" aria-hidden="true" style={{ flexShrink: 0 }}>
      {paths.map((path, index) => <path key={index} d={path} />)}
    </svg>
  )
}
