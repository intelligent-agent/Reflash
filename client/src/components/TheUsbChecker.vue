<template>
  <w-dialog v-if="open" :width="dialog.width">
    <template #title>
      <span class="dialog_title">Installation finished</span>
    </template>
    <p v-if="rebootPressed == false">{{ computeText() }}</p>
    <p v-if="rebootPressed == true">
      Board rebooting ({{ serverResponding ? "responding" : "not responding" }})
    </p>
    <div class="pa5">
      <img
        v-if="rebootPressed == false"
        style="width: 20%"
        :src="computeSVG('USB')"
      />
      <w-progress v-if="rebootPressed == true" class="ma1" circle></w-progress>
    </div>
    <p v-if="this.options.rebootWhenDone && this.isUsbPresent">
      Board will automatically reboot once USB is removed
    </p>
    <w-button
      v-if="rebootPressed == false"
      xl
      outline
      class="ma1 btn"
      @click="clickReboot()"
      :disabled="this.isUsbPresent"
      ><span>Reboot Now</span></w-button
    >
    <w-button xl outline @click="clickReload()" v-if="serverResponding">
      <span>Reload</span>
    </w-button>
    <div v-if="installFinished && !showOverlay"></div>
    <div v-if="showOverlay">
      <w-transition-expand y>
        <w-alert v-if="showOverlay">
          Please wait while board is rebooting
        </w-alert>
      </w-transition-expand>
      <w-progress class="ma1" circle></w-progress><br />
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
    rebootPressed: false,
    serverResponding: false,
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
        this.checkServerResponse();
      } else if (!this.rebootPressed) {
        setTimeout(this.checkUsbPresent, 500);
      }
    },
    async checkServerResponse() {
      await axios.get('/r=' + Math.random(), {timeout: 500}).then((response) => {
        console.log(response);
        this.serverResponding = true;
        setTimeout(this.checkServerResponse, 500);
      }).catch((error) => {
        console.log(error);
        this.serverResponding = false;
        setTimeout(this.checkServerResponse, 500);
      });
    },
    clickReboot() {
      this.$emit("reboot-board");
      this.rebootPressed = true;
      this.checkServerResponse();
    },
    clickReload() {
      window.location.href = "http://" + window.location.hostname + ":"+location.port;
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