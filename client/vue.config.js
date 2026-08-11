module.exports = {
  transpileDependencies: true,
  // The board's rootfs is a small, tightly-sized initrd ramdisk (368M
  // total) - sourcemaps add 3MB+ per build for a debugging aid that's
  // not needed on-device, and repeated deploys without them were
  // enough to fill the disk outright during testing.
  productionSourceMap: false,
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
