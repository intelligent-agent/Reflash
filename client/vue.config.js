module.exports = {
  transpileDependencies: true,
  devServer: {
    proxy: {
      '^/api': {
        target: 'http://127.0.0.1:8081',
        loglevel: 'debug',
        ws: true,
        changeOrigin: true,
        pathRewrite: { '^/api' : '/api' }
      }
    }
  },
  assetsDir: "static"
}
