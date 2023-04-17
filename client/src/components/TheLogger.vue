<template>
  <span>
    <div v-if="open">
      <w-drawer
        :left="true"
        absolute
        width="30%"
        @close="this.$emit('close')">
        <w-flex class="pa5 secondary text-left" column>
          <h3>Log</h3>
          <pre style="white-space: pre-wrap; overflow: auto;" v-html="replaceWithBr()" />
        </w-flex>
      </w-drawer>
    </div>
  </span>
</template>

<script>

export default {
  name: 'TheLogger',
  props: {
    log: String,
    open: Boolean
  },
  methods: {
    replaceWithBr() {
      return this.log.replace(/\\n/g, "<br />")
    }
  }
}

</script>
