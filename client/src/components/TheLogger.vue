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
    log: []
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
    }
  },
  created() {
    const evtSource = new EventSource(`/api/stream_log`);
    evtSource.onmessage = (event) => {
      this.log.push(event.data);
    };
  }
}

</script>
