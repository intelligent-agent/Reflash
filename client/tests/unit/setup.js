import { createRequire } from 'node:module'

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

// wave-ui installs $waveui globally in main.js, which the tests do not run.
// Components read $waveui.theme to choose a light or dark icon and call
// $waveui.notify for toasts; both need to exist or mounting throws.
export const waveuiMock = () => ({
  theme: 'dark',
  notify: () => {},
})
