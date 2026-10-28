declare module 'libbzip2-wasm' {
  interface Result {
    error: number
    errorMsg: string
    output: Uint8Array
    delete(): void
  }
  export interface Codec {
    compressBZ2(input: Uint8Array, blockSize: number, workFactor: number): Result
    decompressBZ2(input: Uint8Array, small: number): Result
    _free(pointer: number): void
  }
  // Emscripten's legacy thenable resolves to itself; do not await it directly.
  export default function createCodec(options: { onAbort?: (reason: unknown) => void }): {
    then(ready: (codec: Codec) => void): void
  }
}
