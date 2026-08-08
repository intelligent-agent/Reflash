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
      <div v-if="isWifiPresent">
        <h3>Connected as: {{ wifiDetails?.name }}</h3>
        <p>Mode: {{ wifiDetails?.mode }}</p>
        
        <w-select
          style="width: 50%; margin: auto"
          v-model="selected"
          :items="availableAPs"
          item-label-key="label"
          return-object
          placeholder="Select an access point">
          SSID
        </w-select>

        <w-input
          class="mt4"
          style="width: 50%; margin: auto"
          v-model="inputPassword"
          type="password">
          Password
        </w-input>

        <div class="mt4">
          <w-button @click="startWifiScan" :disabled="progressVisible" class="mr2">
            Scan for Networks
          </w-button>
          <w-button @click="startWifiConnect" :disabled="progressVisible || !selected">
            Connect
          </w-button>
        </div>
      </div>

      <div v-else class="pa4 text-center mt4 color-error border-error-all">
        <p>⚠️ No WiFi dongle detected. Please plug in a USB WiFi adapter.</p>
      </div>
    </div>
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
    async startWifiScan() {
      this.progressVisible = true;
      try {
        await axios.post('/api/wifi_start_scan');
        setTimeout(this.pollScanResults, 1000);
      } catch (err) {
         this.progressVisible = false;
      }
    },
    async pollScanResults() {
      try {
        const res = await axios.get('/api/wifi_poll_scan');
        if (res.status === 204) {
          setTimeout(this.pollScanResults, 1000);
        } else {
          this.availableAPs = res.data;
          this.progressVisible = false;
          for (const ap in this.availableAPs) {
            this.availableAPs[ap].label = this.availableAPs[ap].SSID + " " + this.availableAPs[ap].signal;
          }
          this.progressVisible = false;
        }
      } catch (err) {
        setTimeout(this.pollScanResults, 1000);
      }
    },
    async startWifiConnect() {
      if (!this.selected) {
        this.$waveui.notify("Please select an access point", "error", 0);
        return;
      }
      this.progressVisible = true;
      try {
        await axios.post('/api/wifi_start_connect',{
          SSID: this.selected.SSID,
          password: this.inputPassword,
        });
        setTimeout(this.pollConnectResults, 1000);
      } catch (err) {
        const msg = err.response?.data || "Could not start connection";
        this.$waveui.notify(msg, "error", 4000);
        this.progressVisible = false;
      }
    },
    async pollConnectResults() {
      try {
        const res = await axios.get('/api/wifi_poll_connect');
        if (res.status === 204) {
          setTimeout(this.pollConnectResults, 1000);
        } else {
          console.log(res.data)
          if(res.data.isConnecting == true){
            setTimeout(this.pollConnectResults, 1000);
          }
          else {
            if (res.data.error) {
              this.$waveui.notify(res.data.error, "error", 0);
            }
            else{
              this.$waveui.notify("Connected to "+this.selected.SSID, "info", 0);              
            }
            this.progressVisible = false;
          }
        }
      } catch (err) {
        setTimeout(this.pollConnectResults, 1000);
      }
    },
    async getWifiStatus() {
      try {
        const response = await axios.get('/api/get_wifi_status');
        
        this.isWifiPresent = response.data.present;
        this.wifiDetails = response.data;

      } catch (err) {
        console.error("WiFi status check failed", err);
        this.isWifiPresent = false;
      } finally {
        setTimeout(this.getWifiStatus, 2000); 
      }
    }
  },
  watch: {
    open: {
      immediate: true,
      handler(is_open) {
        if (is_open) {
          this.getWifiStatus()
          this.dialog.show = true;
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