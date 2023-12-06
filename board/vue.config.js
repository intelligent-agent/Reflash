module.exports = {
  transpileDependencies: true,
  devServer: {
    proxy: {
      '^/api': {
        target: 'http://127.0.0.1:8081',
        loglevel: 'debug',
        ws: true,
        changeOrigin: true,
      },
      '^/favicon.ico':{
        target: 'http://127.0.0.1:8081'
      },
      '^/darkmode.css': {
        target: 'http://127.0.0.1:8081'
      }
    }
  },
  assetsDir: "static"
}
