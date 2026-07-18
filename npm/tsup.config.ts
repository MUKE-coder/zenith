import { defineConfig } from 'tsup'

export default defineConfig({
  entry: {
    // Server-side: reads config, holds secrets.
    index: 'src/index.ts',
    // Browser-safe: no config, no secrets.
    client: 'src/client.ts',
    // Next.js adapters.
    next: 'src/next.ts',
    // The <Analytics /> component, for any React app.
    react: 'src/react.tsx',
    cli: 'src/cli.ts',
  },
  format: ['esm', 'cjs'],
  dts: true,
  clean: true,
  sourcemap: true,
  target: 'node18',
  // bcryptjs stays external so the owner's bundler resolves one copy. React is
  // the app's, never ours: bundling a second copy would break hooks and double
  // the payload.
  external: ['bcryptjs', 'react', 'react/jsx-runtime'],
})
