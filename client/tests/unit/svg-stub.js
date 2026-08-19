// Components choose icons with require("../assets/<name>-<theme>.svg"), which
// webpack turns into a URL at build time. Under vitest the require resolves for
// real and the file's "<svg ...>" is parsed as JavaScript, so mounting anything
// with an icon dies on a SyntaxError before a single assertion runs. Every .svg
// import is aliased here instead.
export default 'svg-stub'
