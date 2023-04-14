<template>
  <span>
    <w-button @click="openDrawer = 'left'" text>
      <w-icon md>fa fa-file</w-icon>
    </w-button>
    <w-drawer
      v-model="openDrawer"
      :left="true"
      absolute
      width="30%">
      <w-flex class="pa5 secondary text-left" column>
        <h3>Log</h3>
        <pre style="white-space: pre-wrap;"
          v-html="replaceWithBr()" />
       </w-flex>
    </w-drawer>
  </span>
</template>

<script>

export default {
  name: 'TheLogger',
  data: () => ({
    openDrawer: false
  }),
  props: {
    log: String
  },
  methods: {
    replaceWithBr() {
      return this.log.replace(/\\n/g, "<br />")
    }
  }
}

</script>