import { defineConfig } from "vite";
import react from "@vitejs/plugin-react";
import tsconfigPaths from "vite-tsconfig-paths";
import { readFileSync } from "node:fs";
import { fileURLToPath, URL } from "node:url";

const pkg = JSON.parse(readFileSync("./package.json", "utf-8"));
const appBase = process.env.VITE_APP_BASE ?? "/";
const apiTargetHost = process.env.VITE_API_TARGET_HOST ?? "127.0.0.1";
const apiTargetPort = process.env.PORT ?? "8060";

// Middleware that makes /icons/* available as an alias for <base>icons/* in dev.
// This mirrors the nginx alias used in production so that icon URLs without the
// /app/ prefix (created on native builds) resolve correctly in the web dev server.
import type { ViteDevServer, Plugin } from "vite";

function iconsAliasPlugin() {
  return {
    name: "icons-alias",
    configureServer(server: ViteDevServer) {
      server.middlewares.use(
        (
          req: import("http").IncomingMessage & { url?: string },
          _res: import("http").ServerResponse,
          next: (err?: unknown) => void,
        ) => {
          if (
            req.url &&
            req.url.startsWith("/icons/") &&
            !appBase.startsWith("/icons")
          ) {
            req.url = `${appBase}icons/${req.url.slice("/icons/".length)}`;
          }
          next();
        },
      );
    },
  } as Plugin;
}

export default defineConfig(async () => {
  const plugins: Plugin[] = [react(), tsconfigPaths(), iconsAliasPlugin()];

  return {
    plugins,
    base: appBase,
    define: {
      "import.meta.env.VITE_APP_VERSION": JSON.stringify(pkg.version),
    },
    resolve: {
      alias: {
        fs: fileURLToPath(
          new URL("./src/shims/empty-node-module.ts", import.meta.url),
        ),
        path: fileURLToPath(
          new URL("./src/shims/empty-node-module.ts", import.meta.url),
        ),
      },
    },
    build: {
      chunkSizeWarningLimit: 1500,
      rollupOptions: {
        onwarn(warning, warn) {
          if (
            warning.code === "EVAL" &&
            typeof warning.id === "string" &&
            warning.id.includes("web-tree-sitter/tree-sitter.js")
          ) {
            return;
          }
          warn(warning);
        },
        output: {
          manualChunks(id) {
            if (!id.includes("node_modules")) return;
            if (id.includes("web-tree-sitter")) return "tree-sitter";
            if (id.includes("dagre") || id.includes("graphlib")) return "dagre";
            if (
              id.includes("@codemirror") ||
              id.includes("@uiw/react-codemirror")
            )
              return "codemirror";
            if (
              id.includes("@chakra-ui") ||
              id.includes("@emotion") ||
              id.includes("framer-motion")
            )
              return "ui";
            if (id.includes("reactflow")) return "reactflow";
          },
        },
      },
    },
    server: {
      host: true,
      port: 5173,
      allowedHosts: ["frontend", "localhost"],
      watch: {
        usePolling: true,
      },
      proxy: {
        "/api": {
          target:
            process.env.VITE_API_TARGET ?? `http://${apiTargetHost}:${apiTargetPort}`,
          changeOrigin: true,
          secure: false,
        },
      },
    },
  };
});
