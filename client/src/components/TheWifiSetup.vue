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
        <h3>{{ wifiSummary }}</h3>
        
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
          <w-button @click="startWifiScan" :disabled="busy" class="mr2">
            Scan for Networks
          </w-button>
          <w-button @click="startWifiConnect" :disabled="busy || !selected">
            Connect
          </w-button>
          <w-button
            @click="startHotspot"
            :disabled="busy || wifi.mode === 'ap'"
            class="ml2"
            title="Serve the Recore access point instead of joining a network"
          >
            Use hotspot
          </w-button>
        </div>
      </div>

      <!-- Only once the server has actually answered: before the first reply
           "not present" is just "not asked yet", and claiming the dongle is
           missing on the way in is how this warning came to flash spuriously. -->
      <div v-else-if="wifiStatusKnown" class="pa4 text-center mt4 color-error border-error-all">
        <p>⚠️ No WiFi dongle detected. Please plug in a USB WiFi adapter.</p>
      </div>

      <div v-if="statusMessage" class="pa2 mt4 text-center">
        <p>{{ statusMessage }}</p>
        <!-- Only while the board is genuinely unreachable. Telling someone to
             reconnect when the page is still talking to the board is noise. -->
        <p v-if="reconnecting && !boardReachable" class="reconnect-hint">
          Reconnect this computer to the same network.
        </p>
      </div>
    </div>
  </w-dialog>
</template>
<script>
import axios from "axios";
import { mapGetters } from "vuex";
import { wifiSummary } from "../network";

// Every request here can be in flight while the radio changes mode, and a
// request to an origin that has just gone away does not necessarily fail -
// confirmed live (#95) that it can hang silently, forever, with no error. Each
// poll loop retries on rejection, so a timeout is what keeps them moving;
// without one they stop dead the first time the AP drops.
//
// Scanning is the common case: one radio cannot host an AP and scan at the same
// time, so wifi-scan takes the hotspot down for ~5s - and if this page arrived
// over that hotspot, every request in that window hangs.
const REQUEST_TIMEOUT = 3000;

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
    wifiStatusKnown: false,
    wifi: {},
    updatePressed: false,
    serialNumber: "",
    serialNumberValid: false,
    inputSSID: "",
    inputPassword: "",
    availableAPs: [],
    selected: null,
    progressVisible: false,
    // A mode switch is in flight and we are waiting to reach the board again.
    reconnecting: false,
    // Whether the board answered our last probe. Starts true so the hint does
    // not flash before the first probe has had a chance to run.
    boardReachable: true,
    reconnectTarget: "",
    reconnectFromMode: "",
    sawTransition: false,
    reconnectTimer: null,
    statusMessage: "",
  }),
  computed: {
    ...mapGetters(["options"]),
    wifiSummary() {
      return wifiSummary(this.wifi);
    },
    // One flag for "don't touch the radio right now", so the buttons come back
    // by themselves the moment the board is reachable and settled again.
    busy() {
      return this.progressVisible || this.reconnecting;
    },
  },
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
    // Reopening the dialog used to show an empty list, no SSID and no selection
    // (#105). The server already keeps the last scan in cachedAccessPoints and
    // the SSID in its options, so both survive even a page reload - they just
    // were never asked for. wifi_poll_scan returns that cache without starting
    // a scan, so this costs nothing and the user can still hit "Scan for
    // Networks" for a fresh list.
    async restoreState() {
      try {
        const [wifi, aps] = await Promise.all([
          axios.get('/api/get_wifi', { timeout: REQUEST_TIMEOUT }),
          axios.get('/api/wifi_poll_scan', { timeout: REQUEST_TIMEOUT }),
        ]);
        this.inputSSID = wifi.data.SSID || "";
        // 204 means a scan is in flight; leave whatever is on screen alone.
        if (aps.status !== 204 && Array.isArray(aps.data)) {
          this.setAvailableAPs(aps.data);
        }
        this.reselectSavedSSID();
      } catch (err) {
        console.error("Could not restore WiFi state", err);
      }
    },
    setAvailableAPs(aps) {
      this.availableAPs = aps;
      for (const ap in this.availableAPs) {
        this.availableAPs[ap].label =
          this.availableAPs[ap].SSID + " " + this.availableAPs[ap].signal;
      }
    },
    // Preselect the network the board is set to use, so reopening the dialog
    // comes back to where the user left it rather than to "Please select one".
    reselectSavedSSID() {
      if (!this.selected && this.inputSSID) {
        this.selected =
          this.availableAPs.find((ap) => ap.SSID === this.inputSSID) || null;
      }
    },
    async startHotspot() {
      this.statusMessage = "Switching to the Recore hotspot...";
      try {
        await axios.post('/api/wifi_start_hotspot', null, { timeout: REQUEST_TIMEOUT });
        this.beginReconnect("Recore", "ap");
      } catch (err) {
        this.statusMessage = err.response?.data || "Could not start the hotspot.";
      }
    },
    async startWifiScan() {
      this.progressVisible = true;
      this.statusMessage = "Scanning for networks...";
      try {
        await axios.post('/api/wifi_start_scan', null, { timeout: REQUEST_TIMEOUT });
        setTimeout(this.pollScanResults, 1000);
      } catch (err) {
         this.progressVisible = false;
         this.statusMessage = "Could not start scan.";
      }
    },
    async pollScanResults() {
      // Bounded by the dialog being open, like the reconnect watch (#115).
      // This reschedules itself on a 204 and again on every error, so a scan
      // still in flight when the dialog closes would poll the board forever.
      // Previously that needed the user to press Scan and then close mid-scan;
      // now that opening the dialog can start one by itself, it would happen to
      // anyone who opened the WiFi page and closed it again.
      if (!this.open) {
        this.progressVisible = false;
        return;
      }
      try {
        const res = await axios.get('/api/wifi_poll_scan', { timeout: REQUEST_TIMEOUT });
        if (res.status === 204) {
          setTimeout(this.pollScanResults, 1000);
        } else {
          this.setAvailableAPs(res.data);
          this.progressVisible = false;
          this.reselectSavedSSID();
          this.statusMessage = `Found ${this.availableAPs.length} network(s).`;
          this.getWifiStatus();
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
      this.statusMessage = `Connecting to ${this.selected.SSID}...`;
      try {
        await axios.post('/api/wifi_start_connect', {
          SSID: this.selected.SSID,
          password: this.inputPassword,
        }, { timeout: REQUEST_TIMEOUT });
        this.beginReconnect(this.selected.SSID, "station");
      } catch (err) {
        const msg = err.response?.data || "Could not start connection";
        this.$waveui.notify(msg, "error", 4000);
        this.statusMessage = msg;
      }
    },
    // Switching the radio takes the board off whatever network this page came
    // in on, so the origin may die mid-request. Rather than giving up after a
    // deadline and offering a "continue at recore.local" button that only
    // guesses, watch for the board to come back and say what is true at each
    // step: switching -> (unreachable) reconnect this computer -> connected.
    beginReconnect(target, wantMode) {
      this.stopReconnectWatch();
      this.reconnecting = true;
      this.reconnectTarget = target;
      // The state the board is in right now. Nothing it reports counts as an
      // outcome until it has left this - straight after the request it is
      // still here, and reading that as a result is wrong in both directions.
      this.reconnectFromMode = this.wifi.mode || "";
      this.sawTransition = false;
      this.progressVisible = false; // the reconnect state drives `busy` now
      this.boardReachable = true;
      this.statusMessage = `Recore is switching to ${target}.`;
      this.watchForReconnect(wantMode);
    },
    stopReconnectWatch() {
      if (this.reconnectTimer) {
        clearTimeout(this.reconnectTimer);
        this.reconnectTimer = null;
      }
      this.reconnecting = false;
    },
    async watchForReconnect(wantMode) {
      // Bounded by the dialog, not by a timer: closing it stops the watch, so
      // this cannot become another loop that runs forever (#115).
      if (!this.open) {
        this.stopReconnectWatch();
        return;
      }
      const target = this.reconnectTarget;
      let wifi = null;
      try {
        // An explicit timeout, because a request to an origin that has gone
        // away does not necessarily fail - confirmed live (#95) that it can
        // hang silently forever. This turns that into a normal rejection.
        const res = await axios.get('/api/get_status', { timeout: REQUEST_TIMEOUT });
        wifi = res.data.network?.wifi || {};
        this.boardReachable = true;
      } catch (err) {
        this.boardReachable = false;
      }

      if (wifi) {
        if (wifi.mode !== this.reconnectFromMode) {
          this.sawTransition = true;
        }
        // Only "arrived" once the board reports the state we asked for. Right
        // after the request it is still on the old one, and treating that as
        // success would report a connection that has not happened.
        const arrived =
          wantMode === "ap"
            ? wifi.mode === "ap"
            : wifi.mode === "station" && wifi.ssid === target;
        if (arrived) {
          this.wifi = wifi;
          this.isWifiPresent = !!wifi.present;
          this.wifiStatusKnown = true;
          this.stopReconnectWatch();
          this.statusMessage = `Connected to ${target}.`;
          return;
        }
        // Reachable, and it fell back to its own hotspot instead of joining:
        // wifi-connect does that on any failure (#90), and nothing used to say
        // so - the user was left believing they had joined their network.
        //
        // Only once it has actually left the mode it started in. Connecting
        // *from* the hotspot is the common case, and without this the first
        // probe sees the AP it has not torn down yet and calls the attempt
        // failed before the board has done anything.
        if (wantMode === "station" && wifi.mode === "ap" && this.sawTransition) {
          this.wifi = wifi;
          this.stopReconnectWatch();
          this.statusMessage =
            `Could not join ${target}. The board is back on its own Recore hotspot.`;
          this.$waveui.notify(this.statusMessage, "error", 0);
          return;
        }
      }

      this.statusMessage = `Recore is switching to ${target}.`;
      this.reconnectTimer = setTimeout(() => this.watchForReconnect(wantMode), 2000);
    },
    // Fetched on demand, never on a timer. This used to poll /api/get_wifi_status
    // every 2s and never stop (#115); that poll raced the mode changes wifi-scan
    // makes and kept reporting the dongle as missing mid-scan, collapsing the
    // dialog. A scan cannot add or remove hardware, so there is nothing here a
    // timer would have caught.
    // Both still go out together - they are independent, and the dialog should
    // not take two round trips to populate. The join is only so that
    // autoScanIfEmpty can see the radio mode and the cached list, which come
    // from different requests, before deciding whether to scan.
    async openDialog() {
      await Promise.all([this.getWifiStatus(), this.restoreState()]);
      this.autoScanIfEmpty();
    },
    // The cached scan lives in the server's memory, so it is empty after a
    // boot or a service restart - and the dialog then opened with an empty
    // list and no indication that pressing "Scan for Networks" was what you
    // were supposed to do. Start one automatically when there is nothing to
    // show.
    //
    // Deliberately not in AP mode. One radio cannot host an AP and scan at the
    // same time, so wifi-scan stops the hotspot for ~5s - and if this page was
    // loaded over that hotspot, an automatic scan would drop the connection it
    // is being viewed through, for a list nobody asked for. The button stays,
    // so it is still a choice there. In station mode wifi-scan leaves the mode
    // alone and the existing connection is unaffected.
    autoScanIfEmpty() {
      if (this.availableAPs.length > 0) return;
      if (this.busy) return;
      if (!this.isWifiPresent) return;
      if (this.wifi.mode !== "station") return;
      this.startWifiScan();
    },
    async getWifiStatus() {
      try {
        const response = await axios.get('/api/get_status', { timeout: REQUEST_TIMEOUT });
        this.wifi = response.data.network?.wifi || {};
        this.isWifiPresent = !!this.wifi.present;
        this.wifiStatusKnown = true;
      } catch (err) {
        // Says nothing about the hardware: scanning takes the AP down, so a
        // browser reaching the board over the hotspot loses the origin for the
        // duration. Keep the last known answer.
        console.error("WiFi status check failed", err);
      }
    }
  },
  watch: {
    open: {
      immediate: true,
      handler(is_open) {
        if (is_open) {
          this.statusMessage = "";
          this.dialog.show = true;
          this.openDialog();
        } else {
          // The watch is bounded by the dialog being open; without this it
          // would keep polling an invisible dialog forever (#115).
          this.stopReconnectWatch();
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
.reconnect-hint {
  opacity: 0.8;
  font-size: 0.9em;
}
</style>