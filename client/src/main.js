import { createApp } from 'vue'
import WaveUI from 'wave-ui'
import App from './App.vue'
import store from './store'

import 'wave-ui/dist/wave-ui.css'
import 'font-awesome/css/font-awesome.min.css'


const app = createApp(App)
app.use(store)

app.use(WaveUI, {
  css: {
    grid: 5,
  }
})

app.mount('#app')
