import { fromJsonString } from '@bufbuild/protobuf'
import { ImpactSceneSchema } from '@buf/tldiagramcom_diagram.bufbuild_es/codeindex/v1/codeindex_pb.js'

export const MAX_SCENE_URL_LENGTH = 10_000
export const MAX_SCENE_BYTES = 16 << 20
export const WEB_SCENE_PREFIX = 'https://tldiagram.com/app/compare#v1='
export const LOCAL_SCENE_PREFIX = 'tld://compare#v1='

function sceneToken(fragment: string, local: boolean, urlLength?: number): string {
  if (!fragment.startsWith('#v1=')) throw new Error('Unsupported comparison URL encoding; expected v1.')
  const token = fragment.slice(4)
  if (!token || !/^[A-Za-z0-9_-]+$/.test(token) || token.length % 4 === 1) throw new Error('Invalid comparison URL payload.')
  const length = urlLength ?? (local ? LOCAL_SCENE_PREFIX : WEB_SCENE_PREFIX).length + token.length
  if (length > MAX_SCENE_URL_LENGTH) throw new Error(`Comparison URL is ${length} characters; limit is ${MAX_SCENE_URL_LENGTH}.`)
  return token
}

export function localSceneURL(fragment: string): string {
  return LOCAL_SCENE_PREFIX + sceneToken(fragment, true)
}

export async function decodeSceneFragment(fragment: string, local = false, urlLength?: number) {
  const token = sceneToken(fragment, local, urlLength)
  if (typeof DecompressionStream === 'undefined') throw new Error('This browser does not support gzip comparison links. Please update your browser.')
  const raw = atob(token.replace(/-/g, '+').replace(/_/g, '/'))
  const bytes = Uint8Array.from(raw, (char) => char.charCodeAt(0))
  // Reject noncanonical trailing bits as the Go base64 decoder does.
  if (btoa(raw).replace(/\+/g, '-').replace(/\//g, '_').replace(/=+$/, '') !== token) throw new Error('Invalid comparison URL payload.')
  const reader = new Blob([bytes]).stream().pipeThrough(new DecompressionStream('gzip')).getReader()
  const decoder = new TextDecoder('utf-8', { fatal: true })
  const chunks: string[] = []
  let size = 0
  try {
    while (true) {
      const { value, done } = await reader.read()
      if (done) break
      size += value.byteLength
      if (size > MAX_SCENE_BYTES) throw new Error(`Comparison scene exceeds ${MAX_SCENE_BYTES} decoded bytes.`)
      chunks.push(decoder.decode(value, { stream: true }))
    }
    chunks.push(decoder.decode())
  } catch (error) {
    await reader.cancel().catch(() => {})
    if (size > MAX_SCENE_BYTES) throw error
    throw new Error('Could not decompress the comparison scene. The link may be damaged.')
  } finally {
    reader.releaseLock()
  }
  try {
    const scene = fromJsonString(ImpactSceneSchema, chunks.join(''))
    if (!scene.comparison || Object.values(scene.views).some((view) => view.placements.some((placement) => !placement.element))) {
      throw new Error('Missing comparison or placement data')
    }
    return scene
  } catch {
    throw new Error('The link does not contain a valid comparison scene.')
  }
}
