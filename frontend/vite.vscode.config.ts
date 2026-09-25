import { defineConfig, type Plugin } from 'vite'
import react from '@vitejs/plugin-react'
import tsconfigPaths from 'vite-tsconfig-paths'
import * as path from 'path'
import * as fs from 'fs'

const overrides: Record<string, string> = {
  [path.resolve(__dirname, 'src/api/transport.ts')]:
    path.resolve(__dirname, 'src/api/transport-vscode.ts'),
  [path.resolve(__dirname, 'src/config/runtime.ts')]:
    path.resolve(__dirname, 'src/config/runtime-vscode.ts'),
  [path.resolve(__dirname, 'src/components/CodePreviewPanel.tsx')]:
    path.resolve(__dirname, 'src/components/CodePreviewPanel-vscode.tsx'),
  // Source links use the workspace file + LSP symbol picker instead of GitHub.
  [path.resolve(__dirname, 'src/components/GitSourceLinker.tsx')]:
    path.resolve(__dirname, 'src/components/LocalSourceLinker.tsx'),
}

const nodeModuleCandidates = [
  path.resolve(__dirname, 'node_modules'),
  path.resolve(__dirname, '../../node_modules'),
]
const localNodeModules = nodeModuleCandidates.find((candidate) => {
  return fs.existsSync(path.resolve(candidate, 'react/jsx-runtime.js'))
}) ?? nodeModuleCandidates[0]

function vscodeOverridesPlugin(): Plugin {
  return {
    name: 'tld-vscode-file-overrides',
    enforce: 'pre',
    resolveId(source, importer) {
      if (!importer) return null
      const resolved = path.resolve(path.dirname(importer), source)
      return (
        overrides[resolved]
        ?? overrides[path.join(resolved, 'index.ts')]
        ?? overrides[path.join(resolved, 'index.tsx')]
        ?? overrides[path.join(resolved, 'index.js')]
        ?? overrides[path.join(resolved, 'index.jsx')]
        ?? overrides[resolved + '.ts']
        ?? overrides[resolved + '.tsx']
        ?? overrides[resolved + '.js']
        ?? overrides[resolved + '.jsx']
        ?? null
      )
    },
  }
}

export default defineConfig({
  plugins: [
    vscodeOverridesPlugin(),
    react(),
    tsconfigPaths({
      projects: [path.resolve(__dirname, 'tsconfig.json')],
      ignoreConfigErrors: true,
    }),
  ],
  base: './',
  resolve: {
    dedupe: [
      'react',
      'react-dom',
      'react-router-dom',
      '@chakra-ui/react',
      '@chakra-ui/icons',
      '@emotion/react',
      '@emotion/styled',
      '@tanstack/react-query',
      'framer-motion',
      'reactflow',
    ],
    alias: [
      {
        find: /^react$/,
        replacement: path.resolve(localNodeModules, 'react/index.js'),
      },
      {
        find: 'react/jsx-runtime',
        replacement: path.resolve(localNodeModules, 'react/jsx-runtime.js'),
      },
      {
        find: 'react/jsx-dev-runtime',
        replacement: path.resolve(localNodeModules, 'react/jsx-dev-runtime.js'),
      },
      {
        find: /^react-dom$/,
        replacement: path.resolve(localNodeModules, 'react-dom/index.js'),
      },
      {
        find: /^react-dom\/client$/,
        replacement: path.resolve(localNodeModules, 'react-dom/client.js'),
      },
      {
        find: /^react-router-dom$/,
        replacement: path.resolve(localNodeModules, 'react-router-dom/dist/index.js'),
      },
    ],
  },
  build: {
    outDir: '../../vscode-extension/out/webview',
    emptyOutDir: true,
    rollupOptions: {
      input: 'src/vscode-entry.tsx',
      output: {
        entryFileNames: 'assets/index.js',
        chunkFileNames: 'assets/[name]-[hash].js',
        assetFileNames: (assetInfo) => {
          if (assetInfo.name?.endsWith('.css')) return 'assets/index.css'
          return 'assets/[name]-[hash][extname]'
        },
      },
    },
  },
  define: {
    'globalThis.__ENABLE_LOGGING__': 'false',
  },
})
