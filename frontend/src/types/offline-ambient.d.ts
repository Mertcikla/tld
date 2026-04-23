/// <reference types="vite/client" />

declare module "*.svg" {
  const src: string
  export default src
}

declare global {
  interface Window {
    __TLD_VSCODE__?: boolean
  }
}

export {}
