import { defineConfig } from 'tsup'

export default defineConfig({
  entry: {
    // Server-side: reads config, holds secrets.
    index: 'src/index.ts',
    // Browser-safe: no config, no secrets.
    client: 'src/client.ts',
    // Next.js adapters.
    next: 'src/next.ts',
    cli: 'src/cli.ts',
  },
  format: ['esm', 'cjs'],
  dts: true,
  clean: true,
  sourcemap: true,
  target: 'node18',
  // bcryptjs stays external so the owner's bundler resolves one copy.
  external: ['bcryptjs'],
})
