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
          <pre class="text-left" style="white-space: pre-wrap; overflow: auto;" v-html="replaceWithBr()" />
        </w-flex>
      </w-drawer>
    </div>
  </span>
</template>

<script>

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
    }
  },
  created() {
    const url = "http://"+location.hostname+":8081/api/stream_log"
    const evtSource = new EventSource(url, {
      withCredentials: false
    });
    evtSource.onmessage = (event) => {
      this.log.push(event.data);
    };
  }
}

</script>
