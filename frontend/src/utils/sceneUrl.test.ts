import { readFileSync } from 'node:fs'
import { gzipSync } from 'node:zlib'
import { describe, expect, it } from 'vitest'
import { decodeSceneFragment, localSceneURL, LOCAL_SCENE_PREFIX, WEB_SCENE_PREFIX, MAX_SCENE_BYTES } from './sceneUrl'

const fixture = (name: string) => readFileSync(new URL(`../../../internal/mermaid/fixtures/scene-url.${name}`, import.meta.url), 'utf8')
const fragment = (raw: string) => '#v1=' + gzipSync(raw).toString('base64url')

describe('comparison URL transport', () => {
  it('decodes the exact Go-produced URL and preserves schema semantics', async () => {
    const link = fixture('txt').trim()
    const hash = '#' + link.split('#')[1]
    const scene = await decodeSceneFragment(hash)
    expect(scene.comparison?.base).toBe('base')
    expect(scene.views['1'].placements[1].element?.name).toBe('Service Ç')
    expect(scene.files[1].linesAdded).toBeUndefined()
    expect(scene.symbols.s1.startLine).toBe(0)
    expect(localSceneURL(hash)).toBe(link.replace(WEB_SCENE_PREFIX, LOCAL_SCENE_PREFIX))
  })

  it('accepts an empty scene and optional audit data', async () => {
    const scene = await decodeSceneFragment(fragment('{"comparison":{},"provenance":{"references":{"v1":"name"}}}'))
    expect(scene.views).toEqual({})
    expect(scene.provenance?.references.v1).toBe('name')
  })

  it('checks exactly 10000 characters before decoding', async () => {
    const atLimit = '#v1=' + 'A'.repeat(10_000 - WEB_SCENE_PREFIX.length)
    await expect(decodeSceneFragment(atLimit)).rejects.toThrow('decompress')
    await expect(decodeSceneFragment(atLimit + 'A')).rejects.toThrow('10001')
  })

  it('uses the actual web destination length for local and self-hosted URLs', async () => {
    const localPrefix = 'http://127.0.0.1:8060/compare#v1='
    const hash = '#v1=' + 'A'.repeat(10_000 - localPrefix.length)
    await expect(decodeSceneFragment(hash, false, 10_000)).rejects.toThrow('decompress')
    await expect(decodeSceneFragment(hash, false, 10_001)).rejects.toThrow('10001')
    expect(localSceneURL(hash)).toBe(LOCAL_SCENE_PREFIX + hash.slice(4))
  })

  it('rejects invalid and truncated input', async () => {
    for (const hash of ['', '#v2=YQ', '#v1=', '#v1=YQ=', '#v1=YR', '#v1=A', '#v1=YQ']) {
      await expect(decodeSceneFragment(hash)).rejects.toThrow()
    }
    const valid = fragment(fixture('json'))
    await expect(decodeSceneFragment(valid.slice(0, -8))).rejects.toThrow()
    for (const raw of ['not JSON', '{}', '{"comparison":{},"views":{"1":{"placements":[{}]}}}']) {
      await expect(decodeSceneFragment(fragment(raw))).rejects.toThrow('valid comparison scene')
    }
  })

  it('bounds decompressed data even when the URL fits', async () => {
    // Exercise the decoder's expansion guard with a mocked bounded input
    // stream, independent of the compressor's block size and gzip overhead.
    const Original = globalThis.DecompressionStream
    const tinyHash = fragment('{}')
    try {
      globalThis.DecompressionStream = class {
        readable = new ReadableStream({ start(controller) { controller.enqueue(new Uint8Array(MAX_SCENE_BYTES + 1)); controller.close() } })
        writable = new WritableStream()
      } as unknown as typeof DecompressionStream
      await expect(decodeSceneFragment(tinyHash)).rejects.toThrow('decoded bytes')
    } finally { globalThis.DecompressionStream = Original }
  })
})
