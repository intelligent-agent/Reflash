<template>
  <w-dialog v-model="dialog.show" :width="dialog.width">
    <template #title>
      <span class="dialog_title">Wi-Fi configuration</span>
    </template>
    <p>
      Here you can choose an external access point and password for your local Wi-Fi router, so
      it can be used with Rebuild. You can also choose to keep the Wi-Fi as an access point. 
    </p>
    <div class="pa5">
      <img style="width: 20%" :src="computeSVG('Wi-Fi')" /><br />
      <w-progress v-if="progressVisible" class="ma1" circle></w-progress>
      <w-select
        style="width: 50%; margin: auto"
        v-model="selected"
        :items="availableAPs"
        item-label-key="label"
        return-object
        placeholder="Select an access point"
        >SSID
      </w-select>
      <w-input
        style="width: 50%; margin: auto"
        v-model="inputPassword"
        type="password"
        >Password</w-input
      >
    </div>
    <w-button xl outline class="ma1 btn" @click="clickConnect()"
      ><span>Connect</span></w-button
    >
    <w-button xl outline class="ma1 btn" @click="performScan()"
      ><span>Rescan</span></w-button
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
    inputPassword: "",
    apList: [],
    availableAPs: [],
    selected: null,
    progressVisible: false,
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
    async performScan() {
      var self = this;
      this.progressVisible = true;
      await axios.get(`/api/wifi_scan`).then(function (response) {
        self.availableAPs = response.data;
        self.progressVisible = false;
        for (const ap in self.availableAPs) {
          self.availableAPs[ap].label = self.availableAPs[ap].SSID + " " + self.availableAPs[ap].signal;
        }
      });
    },
    async clickConnect() {
      var self = this;
      if (!self.selected) {
        self.$waveui.notify("Please select an access point", "error", 0);
        return;
      }
      var ssid = self.selected.SSID || self.selected.ssid;
      var bssid = self.selected.BSSID || self.selected.bssid;
      this.progressVisible = true;
      await axios
        .post(`/api/connect_wifi`, {
          SSID: ssid,
          BSSID: bssid,
          password: self.inputPassword,
        })
        .then(function (response) {
          self.progressVisible = false;
          if (response.data.status != "OK") {
            self.$waveui.notify(response.data.error, "error", 0);
          }
        });
    },
    async getStatus() {
      var self = this;
      await axios.get(`/api/get_wifi_status`).then(function (response) {
        self.isWifiPresent = response.data.connected;
        setTimeout(self.getStatus, 1000);
      });
    },
  },
  watch: {
    open: {
      immediate: true,
      handler(is_open) {
        if (is_open) {
          this.dialog.show = true;
          this.performScan();
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