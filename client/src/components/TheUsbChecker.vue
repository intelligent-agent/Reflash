<template>
  <w-dialog v-if="open" :width="dialog.width">
    <template #title>
      <span class="dialog_title">Installation finished</span>
    </template>
    <p>{{ computeText() }}</p>
    <div class="pa5"><img style="width: 20%" :src="computeSVG('USB')" /></div>
    <p v-if="this.options.rebootWhenDone && this.isUsbPresent">
      Board will automatically reboot once USB is removed
    </p>
    <w-button
      xl
      outline
      class="ma1 btn"
      @click="$emit('reboot-board')"
      :disabled="this.isUsbPresent"
      ><span>Reboot Now</span></w-button
    >
    <div v-if="installFinished && !showOverlay"></div>
    <div v-if="showOverlay">
      <w-transition-expand y>
        <w-alert v-if="showOverlay">
          Please wait while board is rebooting
        </w-alert>
      </w-transition-expand>
      <w-progress class="ma1" circle></w-progress><br />
      <w-button xl outline @click="isServerUp()" v-if="showOverlay"
        ><span>Check server</span></w-button
      >
    </div>
  </w-dialog>
</template>
<script>
import axios from "axios";
import { mapGetters } from "vuex";

export default {
  name: "TheUsbChecker",
  props: {
    open: Boolean,
    installFinished: Boolean,
    showOverlay: Boolean,
  },
  data: () => ({
    dialog: {
      show: true,
      width: "30%",
    },
    isUsbPresent: true,
  }),
  computed: mapGetters(["options"]),
  methods: {
    computeSVG(name) {
      var color;
      if (this.$waveui.theme == "dark") {
        if (this.isUsbPresent) {
          color = "dark";
        } else {
          color = "light";
        }
      } else {
        if (this.isUsbPresent) {
          color = "light";
        } else {
          color = "dark";
        }
      }
      return require("./../assets/" + name + "-" + color + ".svg");
    },
    computeText() {
      if (this.isUsbPresent) {
        return "Please remove USB drive before rebooting";
      } else {
        return "USB removed, ready to reboot";
      }
    },
    async checkUsbPresent() {
      const response = await axios.get(`/api/is_usb_present`);
      this.isUsbPresent = response.data.result;
      if (this.options.rebootWhenDone && this.isUsbPresent == false) {
        this.$emit("reboot-board");
      }
      setTimeout(this.checkUsbPresent, 500);
    },
  },
  watch: {
    open: {
      immediate: true,
      handler(is_open) {
        if (is_open) {
          this.checkUsbPresent();
        }
      },
    },
  },
};
</script>

<style>
.dialog_title {
  margin: auto;
}
</style>