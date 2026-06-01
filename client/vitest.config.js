import { defineConfig } from 'vitest/config'

// Unit tests for plain-JS logic (the Vuex store). Component tests would need
// @vue/test-utils + jsdom; those aren't installed yet, so keep the node env.
export default defineConfig({
  test: {
    environment: 'node',
    include: ['tests/unit/**/*.spec.js'],
  },
})
