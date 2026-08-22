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
    // The app imports components without the extension ("./components/TheInfo").
    // vue-cli's webpack resolves that because it adds .vue to its extension
    // list; Vite's default list does not, so importing App.vue from a test
    // failed on its first child import until .vue was added here too.
    extensions: ['.mjs', '.js', '.mts', '.ts', '.jsx', '.tsx', '.json', '.vue'],
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
