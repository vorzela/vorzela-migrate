import { fileURLToPath, URL } from 'node:url'

import { defineConfig, type Plugin } from 'vite'
import { fileRouter } from '@vorzelajs/router/vite'
import { vorzelajs } from '@vorzelajs/vite-plugin'

export default defineConfig({
  resolve: {
    alias: [
      {
        find: '@/runtime',
        replacement: fileURLToPath(
          new URL('./src/.vorzelajs', import.meta.url),
        ),
      },
      {
        find: '~',
        replacement: fileURLToPath(new URL('./src', import.meta.url)),
      },
      {
        find: '@',
        replacement: fileURLToPath(new URL('./src', import.meta.url)),
      },
    ],
  },
  plugins: [
    fileRouter({ generatedRouteTree: 'src/.vorzelajs/routeTree.gen.ts' }),
    vorzelajs({
      jsxImportSource: '@vorzelajs/dom',
      outDir: 'dist/client',
    }),
  ] as Plugin[],
  server: {
    // `vm gui` serves the built app and API on http://127.0.0.1:7845.
    // During `npm run dev`, proxy API calls there; run `vm gui --no-open`.
    proxy: {
      '/api': {
        target: 'http://127.0.0.1:7845',
        changeOrigin: true,
      },
    },
  },
  build: {
    outDir: 'dist/client',
    emptyOutDir: true,
  },
})
