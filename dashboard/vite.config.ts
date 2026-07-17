import { defineConfig } from 'vite'
import react from '@vitejs/plugin-react'

const CORE_URL = process.env.ZENITH_CORE_URL ?? 'http://localhost:8080'

export default defineConfig({
  plugins: [react()],

  // Relative asset URLs, because the bundle is mounted somewhere other than
  // the origin root: Zenith serves it under /dashboard/. With the default
  // base the stylesheet asks for /assets/font.woff2 at the root, which is not
  // where it is, and every font silently fails to load.
  base: './',

  build: {
    // The console serves these and the npm proxy ships them to owner domains.
    outDir: 'dist',
    sourcemap: true,

    rollupOptions: {
      output: {
        // The entry and the stylesheet keep stable names, because the npm
        // proxy renders a page on the owner's server that names them and
        // cannot know a hash that changes every build. Freshness for those two
        // comes from revalidation (ETag), not the filename.
        entryFileNames: 'assets/zenith.js',
        assetFileNames: (info) => {
          if (info.names?.some((n) => n.endsWith('.css'))) return 'assets/zenith.css'
          // Fonts keep their hash: a dozen of them, they never change, cache
          // forever.
          return 'assets/[name]-[hash][extname]'
        },
        // Lazy chunks DO carry a hash. The entry imports them by the exact
        // name Vite wrote into it, so nothing external needs to predict it —
        // and two builds' chunks must not collide under one cached URL.
        chunkFileNames: 'assets/zenith-[name]-[hash].js',
      },
    },
  },

  server: {
    port: 5173,
    // Mirrors production, where the npm package proxies core onto the owner's
    // own domain. Keeping dev same-origin too means no CORS anywhere.
    // Paths pass through untouched — core owns these routes as-is.
    proxy: {
      '/api': { target: CORE_URL, changeOrigin: true },
      '/health': { target: CORE_URL, changeOrigin: true },
    },
  },
})
