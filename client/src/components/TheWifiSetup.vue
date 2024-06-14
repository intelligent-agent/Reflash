<template>
  <w-dialog v-model="dialog.show" :width="dialog.width">
    <template #title>
      <span class="dialog_title">Set up Wi-Fi</span>
    </template>
    <p>
      Here you can supply the SSID and password for your local Wi-Fi 
      router, so it can be used with Rebuild.
    </p>
    <div class="pa5">
        <img style="width: 20%" :src="computeSVG('Wi-Fi')" /><br />
      <w-progress v-if="updatePressed == true" class="ma1" circle></w-progress>
      <w-input style="width: 50%; margin: auto" v-model="inputSSID" type="string"
        >SSID</w-input
      >
      <w-input style="width: 50%; margin: auto" v-model="inputPassword" type="password"
        >Password</w-input
      >
    </div>
    <w-button
      xl
      outline
      class="ma1 btn"
      @click="clickUpdateConfig()"
      ><span>Save Credentials</span></w-button
    >
  </w-dialog>
</template>
<script>
import axios from "axios";
import { mapGetters } from "vuex";

export default {
  name: "TheWifiSetup",
  props: {
    open: Boolean,
    showOverlay: Boolean,
  },
  data: () => ({
    dialog: {
      show: false,
      width: "30%",
    },
    isWifiPresent: false,
    updatePressed: false,
    serialNumber: "",
    serialNumberValid: false,
    inputSSID: "",
    inputPassword: "",
  }),
  computed: mapGetters(["options"]),
  methods: {
    computeSVG(name) {
      var color;
      if (this.$waveui.theme == "dark") {
        if (this.isWifiPresent) {
          color = "dark";
        } else {
          color = "light";
        }
      } else {
        if (this.isWifiPresent) {
          color = "light";
        } else {
          color = "dark";
        }
      }
      return require("./../assets/" + name + "-" + color + ".svg");
    },
    async getInfo() {
      var self = this;
      await axios.get(`/api/get_wifi`).then(function (response) {
        self.inputSSID = response.data.SSID;
      });
    },    
    async clickUpdateConfig() {
      var self = this;
      this.updatePressed = true;
      await axios
        .post(`/api/save_wifi`, { ssid: self.inputSSID, password: self.inputPassword })
        .then(function (response) {
          self.updatePressed = false;
          if (response.data.status != "OK") {
            self.$waveui.notify(response.data.error, "error", 0);
          }
        });
    },
  },
  watch: {
    open: {
      immediate: true,
      handler(is_open) {
        if (is_open) {
          this.dialog.show = true;
          this.getInfo();
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