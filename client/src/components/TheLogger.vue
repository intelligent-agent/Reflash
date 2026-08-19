<template>
  <span>
    <div v-if="open">
      <w-drawer
        :left="true"
        absolute
        width="30%"
        @close="this.$emit('close')">
        <w-flex class="pa5 secondary" column>
          <h3>Log</h3>
          <w-button xl outline @click="clearLog()">Clear log</w-button>
          <pre class="text-left" style="white-space: pre-wrap; overflow: auto;" v-html="replaceWithBr()" />
        </w-flex>
      </w-drawer>
    </div>
  </span>
</template>

<script>
import axios from 'axios';

export default {
  name: 'TheLogger',
  props: {
    open: Boolean
  },
  data: () => ({
    log: [],
    evtSource: null,
    watchdog: null
  }),
  methods: {
    replaceWithBr() {
      return this.log.join('<br />')
    },
    async clearLog(){
      this.log = [];
      const response = await axios.put(`/api/clear_log`)
      if(response.data.status != 0)
        this.$waveui.notify("Unable to clear log", "error", 0);
    },
    connectLogStream() {
      // The server replays the whole file on every connection - streamLog
      // tails from offset 0, deliberately, so a freshly opened page shows the
      // boot rather than an empty box. That makes each connection a complete
      // snapshot, not an increment, so the previous contents have to go.
      //
      // Without this the log appeared two and three times over: the #95
      // watchdog made reconnects actually happen, and the board reconnects
      // precisely when it changes WiFi mode - so every AP/station switch
      // appended another full copy of the log to the one already on screen.
      this.log = [];
      const evtSource = new EventSource(`/api/stream_log`);
      evtSource.onmessage = (event) => {
        this.log.push(event.data);
        this.resetWatchdog();
      };
      // The server sends one of these every 15s (see streamLog in
      // server.go) purely so the watchdog below has something to reset
      // on even when there's no real log activity.
      evtSource.addEventListener('heartbeat', () => {
        this.resetWatchdog();
      });
      // The board's WiFi interface disappears and reappears when
      // switching between AP/hotspot and station mode (#95). Confirmed
      // live: that kills the connection completely silently on both
      // ends - no error, no data, nothing - so onerror is not enough on
      // its own; the watchdog below is what actually catches this case.
      evtSource.onerror = () => {
        this.reconnect();
      };
      this.evtSource = evtSource;
      this.resetWatchdog();
    },
    resetWatchdog() {
      clearTimeout(this.watchdog);
      // Twice the server's heartbeat interval, so one missed beat isn't
      // treated as a dead connection.
      this.watchdog = setTimeout(() => this.reconnect(), 30000);
    },
    reconnect() {
      clearTimeout(this.watchdog);
      if (this.evtSource) this.evtSource.close();
      setTimeout(() => this.connectLogStream(), 3000);
    }
  },
  created() {
    this.connectLogStream();
  },
  beforeUnmount() {
    clearTimeout(this.watchdog);
    if (this.evtSource) this.evtSource.close();
  }
}

</script>
