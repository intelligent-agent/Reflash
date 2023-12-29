module.exports = {
  transpileDependencies: true,
  devServer: {
    proxy: {
      '^/api': {
        target: 'http://127.0.0.1:8080',
        loglevel: 'debug',
        ws: true,
        changeOrigin: true,
      },
      '^/favicon.ico': {
        target: 'http://127.0.0.1:8080'
      }
    },
    compress: false
  },
  assetsDir: "static",
  css: {
    extract: false
  }
}
