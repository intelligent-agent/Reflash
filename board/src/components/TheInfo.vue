<template>
  <w-transition-expand y>
      <p v-if="open" class="text-left">
        This page is running from the eMMC drive on Recore. You can use this page to change boot media, enable SSH access and 
        rotate the external screen.<br>
        <span>Reflash version: {{version}}</span><br />
        <span>Recore revision: {{revision}}</span><br />
        <span>Emmc version: {{ emmc_version }}</span>

      </p>
  </w-transition-expand>
</template>

<script>
export default {
  name: 'TheInfo',
  props: {
    open: Boolean,
    version: String,
    revision: String, 
    emmc_version: String
  }
}
</script>
