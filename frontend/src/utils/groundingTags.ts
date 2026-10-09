/**
 * Reserved system tag that exempts an element from the ARC205 source-grounding
 * score without marking it external. It is stored alongside ordinary tags but
 * must stay hidden from every user-facing tag surface. The `$` prefix keeps it
 * from colliding with, or being parsed as, a normal tag.
 */
export const GROUNDING_IGNORE_TAG = '$ignored'

const GROUNDING_IGNORE_TAG_PATTERN = /^\$ignored$/i

export function isGroundingIgnoreTag(tag: string): boolean {
  return GROUNDING_IGNORE_TAG_PATTERN.test(tag.trim())
}
