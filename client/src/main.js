import { createApp } from 'vue'
import WaveUI from 'wave-ui'
import App from './App.vue'

import 'wave-ui/dist/wave-ui.css'
import 'font-awesome/css/font-awesome.min.css'


const app = createApp(App)
new WaveUI(app, {
  css: {
    grid: 5
  }
})
app.mount('#app')
