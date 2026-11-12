// Fixed reference scale for tuning source layouts. Extending the maximum level
// must not move existing transitions (40 = names, ~63.3 = signatures, ~99.6 = code).
const REFERENCE_MIN_ZOOM = 0.1
const REFERENCE_MAX_ZOOM = 281 / 85
export const sourceDetailZoom = (level: number) => REFERENCE_MIN_ZOOM + (level - 1) / 99 * (REFERENCE_MAX_ZOOM - REFERENCE_MIN_ZOOM)
export const SOURCE_MAX_DETAIL_LEVEL = 300
export const sourceDetailCapScale = sourceDetailZoom(SOURCE_MAX_DETAIL_LEVEL) / REFERENCE_MAX_ZOOM

export const sourceWidthCapScale = sourceDetailZoom(180) / REFERENCE_MAX_ZOOM
