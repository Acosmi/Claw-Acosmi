import { defineConfig } from 'vite';
import react from '@vitejs/plugin-react';
import { resolve } from 'path';

export default defineConfig({
  plugins: [react()],
  build: {
    outDir: resolve(__dirname, '../sidepanel-dist'),
    emptyOutDir: true,
    rollupOptions: {
      input: resolve(__dirname, 'index.html'),
      output: {
        // Single bundle, no code splitting (Chrome extension requirement).
        manualChunks: undefined,
        entryFileNames: 'sidepanel.js',
        assetFileNames: 'sidepanel.[ext]',
      },
    },
    // Inline small assets.
    assetsInlineLimit: 8192,
    minify: 'esbuild',
  },
  // No dev server HMR in extension context.
  server: {
    port: 5174,
  },
});
