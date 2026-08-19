import { defineConfig } from 'vitest/config'
import vue from '@vitejs/plugin-vue'
import { fileURLToPath } from 'node:url'

// Component tests run in jsdom; the plain-JS ones (network.js, the Vuex store)
// are happy there too, so there is one environment rather than two.
export default defineConfig({
  plugins: [vue()],
  test: {
    environment: 'jsdom',
    include: ['tests/unit/**/*.spec.js'],
    setupFiles: ['tests/unit/setup.js'],
  },
  resolve: {
    alias: [
      { find: '@', replacement: fileURLToPath(new URL('./src', import.meta.url)) },
      // See tests/unit/svg-stub.js - icon requires would otherwise be parsed
      // as JavaScript.
      {
        find: /\.svg$/,
        replacement: fileURLToPath(new URL('./tests/unit/svg-stub.js', import.meta.url)),
      },
    ],
  },
})
