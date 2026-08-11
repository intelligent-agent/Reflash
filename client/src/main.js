import { createApp } from 'vue'
import WaveUI from 'wave-ui'
import axios from 'axios'
import App from './App.vue'
import store from './store'

import 'wave-ui/dist/wave-ui.css'

// Several components poll the board on a timer with no per-call timeout
// (WiFi status, transfer progress, scan results, ...). A request with no
// timeout that never resolves - confirmed live: the board's WiFi
// interface disappearing mid-request during an AP/station switch (#90,
// #95) causes exactly this - sits holding one of the browser's ~6
// same-origin connection slots forever. Enough of those piling up at
// once starves every other request, including ones with their own
// explicit timeout, since they can't get a socket to run on at all. A
// global default bounds every request that doesn't set its own.
axios.defaults.timeout = 10000

const app = createApp(App)
app.use(store)

app.use(WaveUI, {
  css: {
    grid: 5,
  }
})

app.mount('#app')
