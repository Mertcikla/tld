import { defineConfig } from 'vite'
import react from '@vitejs/plugin-react'
import tsconfigPaths from 'vite-tsconfig-paths'
import { readFileSync } from 'node:fs'

const pkg = JSON.parse(readFileSync('./package.json', 'utf-8'))
const appBase = process.env.VITE_APP_BASE ?? '/'

// Middleware that makes /icons/* available as an alias for <base>icons/* in dev.
// This mirrors the nginx alias used in production so that icon URLs without the
// /app/ prefix (created on native builds) resolve correctly in the web dev server.
import type { ViteDevServer, Plugin } from 'vite'

function iconsAliasPlugin() {
  return {
    name: 'icons-alias',
    configureServer(server: ViteDevServer) {
      server.middlewares.use((req: import('http').IncomingMessage & { url?: string }, _res: import('http').ServerResponse, next: (err?: unknown) => void) => {
        if (req.url && req.url.startsWith('/icons/') && !appBase.startsWith('/icons')) {
          req.url = `${appBase}icons/${req.url.slice('/icons/'.length)}`
        }
        next()
      })
    },
  } as Plugin
}

export default defineConfig(async () => {
  const plugins: Plugin[] = [
    react(),
    tsconfigPaths(),
    iconsAliasPlugin(),
  ]

  return {
    plugins,
    base: appBase,
    define: {
      'import.meta.env.VITE_APP_VERSION': JSON.stringify(pkg.version),
    },
    server: {
      host: true,
      port: 5173,
      allowedHosts: ['frontend', 'localhost'],
      watch: {
        usePolling: true,
      },
      proxy: {
        '/api': {
          target: process.env.VITE_API_TARGET ?? 'http://localhost:8081',
          changeOrigin: true,
          secure: false,
          ws: true,
        },
      },
    },
  }
})
