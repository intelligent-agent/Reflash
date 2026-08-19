import { createRequire } from 'node:module'
import { config } from '@vue/test-utils'

// Components choose their icon with require("../assets/<name>-<theme>.svg"),
// which webpack turns into a URL at build time. Under vitest that require is
// Node's own - it bypasses Vite's resolver, so an alias does not reach it - and
// it reads the file as JavaScript, dying on "<svg ...>" before a single
// assertion runs. Teaching Node how to load an .svg is the smallest fix that
// does not mean rewriting the components' asset handling to suit the tests.
const require = createRequire(import.meta.url)
require.extensions['.svg'] = (module) => {
  module.exports = 'svg-stub'
}

// wave-ui registers its w-* components as globals from main.js, which the tests
// do not run - so Vue cannot resolve them and warns on every mount. Registering
// pass-through stubs here silences that in one place instead of a stubs block
// per test.
//
// Pass-through rather than empty: they render their default slot, so a dialog's
// or a card's contents stay assertable. Doing this with the compiler's
// isCustomElement instead looked tidier but made the SFC compiler fall over
// with "Codegen node is missing for element/if/for node".
const passThrough = (tag) => ({
  name: tag,
  template: '<div><slot /></div>',
})

config.global.components = Object.fromEntries(
  [
    'w-app', 'w-button', 'w-card', 'w-dialog', 'w-divider', 'w-drawer',
    'w-flex', 'w-input', 'w-progress', 'w-radios', 'w-select', 'w-spinner',
    'w-switch', 'w-transition-expand',
  ].map((tag) => [tag, passThrough(tag)])
)

// wave-ui also installs $waveui, which components read for the current theme
// (to pick a light or dark icon) and call for toasts. Both need to exist or
// mounting throws.
config.global.mocks = {
  ...config.global.mocks,
  $waveui: { theme: 'dark', notify: () => {} },
}
