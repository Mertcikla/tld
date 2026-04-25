/**
 * Vite library build config for @tldiagram/core-ui
 *
 * Produces an ES module bundle and CSS file for consumption by host apps.
 * React, Chakra UI, React Flow, and other large peer deps are externalized.
 */
import { defineConfig } from 'vite'
import react from '@vitejs/plugin-react'
import tsconfigPaths from 'vite-tsconfig-paths'
import { resolve } from 'path'
import type { Plugin } from 'vite'

const EXTERNAL_PACKAGES = new Set([
  // React ecosystem must be provided by host app
  'react',
  'react-dom',
  'react/jsx-runtime',
  'react-router-dom',

  // UI framework host app provides
  '@chakra-ui/react',
  '@chakra-ui/icons',
  '@emotion/react',
  '@emotion/styled',
  'framer-motion',

  // Canvas
  'reactflow',

  // ConnectRPC
  '@connectrpc/connect',
  '@connectrpc/connect-web',
  '@bufbuild/protobuf',

  // Code editor
  '@codemirror/commands',
  '@codemirror/lang-cpp',
  '@codemirror/lang-java',
  '@codemirror/lang-javascript',
  '@codemirror/lang-python',
  '@codemirror/lang-rust',
  '@codemirror/language',
  '@codemirror/state',
  '@codemirror/theme-one-dark',
  '@codemirror/view',
  '@uiw/react-codemirror',

  // Layout / force
  'd3-force',
  'dagre',

  // Tree-sitter
  'web-tree-sitter',

  // Export
  'html-to-image',

  // Mermaid
  'mermaid-ast',
])

export default defineConfig({
  plugins: [react(), tsconfigPaths()] as Plugin[],
  build: {
    lib: {
      entry: resolve(__dirname, 'src/index.ts'),
      formats: ['es'],
      fileName: 'index',
    },
    rollupOptions: {
      external: (id: string) => {
        // Exact match
        if (EXTERNAL_PACKAGES.has(id)) return true
        // Buf proto packages
        if (id.startsWith('@buf/')) return true
        // Sub-path imports of externalized packages (e.g. reactflow/dist/style.css)
        for (const pkg of EXTERNAL_PACKAGES) {
          if (id.startsWith(pkg + '/')) return true
        }
        return false
      },
    },
    // Emit a single CSS file alongside the JS bundle
    cssCodeSplit: false,
    // Keep output clean
    emptyOutDir: true,
  },
})
