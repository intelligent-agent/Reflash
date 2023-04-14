<template>
  <w-transition-expand y>
      <p v-if="open" class="text-left">
        This page is running from a USB drive on Recore. You can use this page to download and
        install distros to the eMMC of Recore.
        <ol>
          <li>Start by downloading an image, probably the latest version. </li>
          <li>Once downloaded, you can flash the image to the internal storage (eMMC)</li>
          <li>Finally reboot the board. The board will boot from the internal storage.</li>
        </ol>
        <span>Reflash version {{version}}</span>
      </p>
  </w-transition-expand>
</template>

<script>
export default {
  name: 'TheInfo',
  props: {
    open: Boolean,
    version: String
  }
}
</script>
