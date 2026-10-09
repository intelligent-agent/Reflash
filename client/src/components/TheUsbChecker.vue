<template>
  <!-- Once the user has put the dialog away to make last changes, the same
       message stays across the top of the page, so the reboot that follows
       taking the drive out is not a surprise. -->
  <div v-if="open && dismissed && !rebootPressed" class="finished-callout">
    <span>Flashing is finished. {{ computeText() }}</span>
    <w-button
      v-if="options.rebootWhenDone == false"
      class="ml3"
      outline
      sm
      @click="clickReboot()"
      :disabled="isUsbPresent"
      ><span>Reboot Now</span></w-button
    >
  </div>
  <w-dialog v-if="open && !dismissed" :width="dialog.width" persistent>
    <template #title>
      <span class="dialog_title">Installation finished</span>
    </template>
    <p v-if="rebootPressed == false">{{ computeText() }}</p>
    <p v-if="rebootPressed == true">
      {{ serverResponding ? "New image ready!" : "Board rebooting" }}
    </p>
    <div class="pa5">
      <img
        v-if="rebootPressed == false"
        style="width: 20%"
        :src="computeSVG('USB')"
      />
      <w-progress
        v-if="rebootPressed == true && serverResponding == false"
        class="ma1"
        circle
      ></w-progress>
    </div>
    <p v-if="this.options.rebootWhenDone && this.isUsbPresent">
      Board will automatically reboot once USB is removed
    </p>
    <w-button
      v-if="rebootPressed == false && this.options.rebootWhenDone == false"
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
    <!-- Wi-Fi, SSH and the rest still reach the new image from here (#185);
         the board stays put until the drive comes out. -->
    <div v-if="rebootPressed == false" class="mt4">
      <w-button class="ma1" text @click="dismissed = true">
        <span>Make changes first</span>
      </w-button>
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
    // The dialog put away; the callout across the top stands in for it.
    dismissed: false,
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
      } else if (this.options.rebootWhenDone){
        return "USB removed, rebooting";
      } else {
        return "USB removed, ready to reboot";
      }
    },
    async checkUsbPresent() {
      const response = await axios.get(`/api/is_usb_present`);
      this.isUsbPresent = response.data.result;
      if (this.options.rebootWhenDone && this.isUsbPresent == false) {
        // Deliberately does NOT reboot. The server does that itself, in
        // checkAutoReboot, off the same condition. Both used to do it, and at
        // 500ms here against the server's 2s tick the browser always won - so
        // the server path was never exercised by anyone using the UI and stayed
        // broken unnoticed (#123). It is also the only path a non-browser
        // client has: the USB serial protocol offers LIST/STATUS/FLASH/CANCEL
        // and no reboot at all.
        //
        // So just watch for the board to go away and come back.
        this.rebootPressed = true;
        this.dismissed = false;
        this.serverResponding = false;
        setTimeout(this.checkServerResponse, 1000);
      } else if (!this.rebootPressed) {
        setTimeout(this.checkUsbPresent, 500);
      }
    },
    async checkServerResponse() {
      await axios
        .get("/robots.txt?r=" + Math.random(), { timeout: 3000 })
        .then(() => {
          this.serverResponding = true;
          setTimeout(this.checkServerResponse, 1000);
        })
        .catch(() => {
          this.serverResponding = false;
          setTimeout(this.checkServerResponse, 1000);
        });
    },
    clickReboot() {
      this.$emit("reboot-board");
      this.rebootPressed = true;
      this.dismissed = false;
      this.serverResponding = false;
      setTimeout(this.checkServerResponse, 1000);
    },
    clickReload() {
      window.location.href =
        "http://" + window.location.hostname + ":" + location.port+"?r=" + Math.random();
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
.finished-callout {
  position: fixed;
  top: 0;
  left: 0;
  right: 0;
  z-index: 900;
  padding: 8px 16px;
  text-align: center;
  font-size: 0.95em;
  background: #04a3e5;
  color: #fff;
}
</style>