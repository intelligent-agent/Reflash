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
          <w-button @click="getLog">Get log</w-button>
          <pre class="text-left" style="white-space: pre-wrap; overflow: auto;" v-html="replaceWithBr()" />
        </w-flex>
      </w-drawer>
    </div>
  </span>
</template>

<script>
import axios from "axios";

export default {
  name: 'TheLogger',
  props: {
    log: String,
    open: Boolean
  },
  data: () => ({
    log2: ""
  }),
  methods: {
    replaceWithBr() {
      return this.log2.replace(/\\n/g, "<br />")
    },
    getLog() {
      var self = this;
      axios.get(`/api/get_log`).then(function(response){
        self.log2 = response.data.success;
      });
    }
  }
}

</script>
