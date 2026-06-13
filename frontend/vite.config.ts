import { defineConfig } from "vite";
import react from "@vitejs/plugin-react";
import viteCompression from "vite-plugin-compression";
import tsconfigPaths from "vite-tsconfig-paths";
import { existsSync, readFileSync } from "node:fs";
import { fileURLToPath, URL } from "node:url";
import { resolve } from "node:path";

const pkg = JSON.parse(readFileSync("./package.json", "utf-8"));
const appBase = process.env.VITE_APP_BASE ?? "/";
const apiTargetHost = process.env.VITE_API_TARGET_HOST ?? "127.0.0.1";
const apiTargetPort = process.env.PORT ?? "8060";
const apiTarget = process.env.VITE_API_TARGET ?? `http://${apiTargetHost}:${apiTargetPort}`;
const localProtoGenDir = process.env.TLD_LOCAL_PROTO_GEN
  ? resolve(__dirname, process.env.TLD_LOCAL_PROTO_GEN)
  : existsSync(resolve(__dirname, "src/gen"))
    ? resolve(__dirname, "src/gen")
    : null;

import type { Plugin } from "vite";

function devPublicIconPath(requestUrl: string | undefined): string | undefined {
  if (!requestUrl?.startsWith("/icons/")) return undefined;

  const parsed = new URL(requestUrl, "http://tld.local");
  const filename = decodeURIComponent(parsed.pathname.slice("/icons/".length));
  if (!filename || filename.includes("/") || filename.includes("\\")) return undefined;

  const localIcon = resolve(__dirname, "public", "icons", filename);
  if (!existsSync(localIcon)) return undefined;

  const normalizedBase = appBase.endsWith("/") ? appBase : `${appBase}/`;
  return `${normalizedBase}icons/${filename}${parsed.search}`;
}

export default defineConfig(async () => {
  const plugins: Plugin[] = [
    react(),
    viteCompression({
      algorithm: "gzip",
      ext: ".gz",
      deleteOriginFile: false,
    }),
    viteCompression({
      algorithm: "brotliCompress",
      ext: ".br",
      deleteOriginFile: false,
    }),
    tsconfigPaths({
      projects: [fileURLToPath(new URL("./tsconfig.json", import.meta.url))],
      ignoreConfigErrors: true,
    }),
  ];

  return {
    plugins,
    base: appBase,
    define: {
      "import.meta.env.VITE_APP_VERSION": JSON.stringify(pkg.version),
    },
    resolve: {
      alias: {
        ...(localProtoGenDir
          ? {
              "@buf/tldiagramcom_diagram.bufbuild_es": localProtoGenDir,
            }
          : {}),
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
        "/icons.json": {
          target: apiTarget,
          changeOrigin: true,
          secure: false,
        },
        "/icons": {
          target: apiTarget,
          changeOrigin: true,
          secure: false,
          bypass: (req) => devPublicIconPath(req.url),
        },
        "/api": {
          target: apiTarget,
          changeOrigin: true,
          secure: false,
        },
      },
    },
  };
});
