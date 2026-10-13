<template>
  <div class="wifi-root">
  <w-dialog v-if="!inline" v-model="dialog.show" :width="dialog.width" dialog-class="wifi-manager-dialog" content-class="wifi-manager-content" @update:model-value="!$event && $emit('close')">
    <template #title><span class="wifi-dialog-title">Manage Wi-Fi</span></template>
    <div class="wifi-manager setup-wrap">
      <p class="wifi-intro">Connect Reflash to your network or use its hotspot.</p>
      <div v-if="isWifiPresent">
        <div class="wifi-connection" role="status"><span class="wifi-status-dot" aria-hidden="true"></span>{{ wifiSummary }}</div>
        <section class="wifi-manager-section">
          <h4>Client network</h4>
          <label class="wifi-field-label" for="live-wifi-ssid">SSID</label>
          <w-select id="live-wifi-ssid" class="wifi-network-select" v-model="selected" :items="availableAPs" item-label-key="label" return-object placeholder="Select a network" :disabled="busy" />
          <label class="wifi-field-label" for="live-wifi-password">Password</label>
          <div class="row">
            <input id="live-wifi-password" class="input" v-model="inputPassword" :type="showPassword ? 'text' : 'password'" autocomplete="off" placeholder="Enter the network password" :disabled="busy" />
            <button type="button" class="btn small plain" @click="showPassword = !showPassword">{{ showPassword ? 'Hide' : 'Show' }}</button>
          </div>
          <div class="wifi-actions">
            <button type="button" class="btn small" @click="startWifiScan" :disabled="busy">{{ progressVisible ? 'Scanning...' : 'Scan for networks' }}</button>
            <button type="button" class="btn small wifi-connect-button" @click="startWifiConnect" :disabled="busy || !selected">{{ reconnecting ? 'Connecting...' : 'Connect' }}</button>
          </div>
          <div v-if="ipHandoverWarning" class="wifi-handover-warning" role="alert">
            <p>Connecting to a client network will stop the hotspot at this IP address. Open this page by its mDNS name first so it can reconnect after the switch.</p>
            <p v-if="handoverURL"><a :href="handoverURL">Continue at {{ handoverHostname }}</a> — Manage Wi-Fi stays open with this network selected. Re-enter the password there.</p>
            <p v-else>{{ handoverLookup ? 'Looking up the board’s mDNS name...' : 'The mDNS name is unavailable. After connecting, rejoin the destination Wi-Fi and find the board’s new IP in your router.' }}</p>
            <button type="button" class="btn small" :disabled="handoverLookup" @click="startWifiConnect(true)">Connect using IP anyway</button>
          </div>
          <p class="wifi-note">Scanning or changing networks may disconnect this browser.</p>
        </section>
        <section class="wifi-manager-section wifi-split">
          <div><h4>Reflash hotspot</h4><p class="wifi-note">Recore · 192.168.50.1</p></div>
          <button type="button" class="btn small" @click="startHotspot" :disabled="busy || wifi.mode === 'ap'">{{ wifi.mode === 'ap' ? 'Hotspot active' : 'Use hotspot' }}</button>
        </section>
      </div>
      <div v-else-if="wifiStatusKnown" class="wifi-manager-section">
        <p>No Wi-Fi adapter detected.</p>
        <button type="button" class="btn small" @click="getWifiStatus()">Check again</button>
      </div>
      <p v-if="statusMessage" class="wifi-message" role="status">{{ statusMessage }}</p>
      <ReconnectNotice transition-only :active="reconnecting && !boardReachable" :target="reconnectTarget" />
      <section v-if="isWifiPresent" class="wifi-manager-section">
        <h4>Installed image (eMMC)</h4>
        <p class="wifi-note">Apply this Wi-Fi setup to the installed image for its next boot.</p>
        <button type="button" class="btn small" :disabled="busy || applyingSetup" @click="useSetupOnEmmc()">{{ applyingSetup ? 'Applying...' : 'Use this setup on eMMC' }}</button>
        <p v-if="applyMessage" class="wifi-message" role="status">{{ applyMessage }}</p>
      </section>
      <p class="wifi-note wifi-next-image">Choose Wi-Fi for the next flash in Set up printer.</p>
      <div class="wifi-manager-footer"><button type="button" class="btn small" :disabled="applyingSetup" @click="dialog.show = false; $emit('close')">Done</button></div>
    </div>
  </w-dialog>
  <!-- The same form without the dialog around it, for the setup panel (#198). -->
  <div v-else class="wifi-inline">
    <div>
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
        </div>
      </div>

      <!-- Only once the server has actually answered: before the first reply
           "not present" is just "not asked yet", and claiming the dongle is
           missing on the way in is how this warning came to flash spuriously. -->
      <div v-else-if="wifiStatusKnown" class="pa4 text-center mt4">
        <p>No Wi-Fi adapter detected.</p>
        <w-button @click="getWifiStatus()">Check again</w-button>
      </div>

      <div v-if="statusMessage" class="pa2 mt4 text-center">
        <p>{{ statusMessage }}</p>
        <!-- Only while the board is genuinely unreachable. Telling someone to
             reconnect when the page is still talking to the board is noise. -->
        <ReconnectNotice transition-only :active="reconnecting && !boardReachable" :target="reconnectTarget" />
      </div>
    </div>
  </div>
  </div>
</template>
<script>
import axios from "axios";
import { mapGetters } from "vuex";
import ReconnectNotice from "./ReconnectNotice.vue";
import { wifiSummary, isIPAddress, wifiManagerURL } from "../network";

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
  components: { ReconnectNotice },
  props: {
    initialSsid: { type: String, default: "" },
    applyRequest: Function,
    open: Boolean,
    showOverlay: Boolean,
    // Drawn in the page, not in a dialog.
    inline: Boolean,
  },
  data: () => ({
    dialog: {
      show: false,
      width: "min(620px, calc(100vw - 32px))",
    },
    locationURL: window.location.href,
    ipHandoverWarning: false,
    handoverLookup: false,
    handoverHostname: '',
    showPassword: false,
    applyingSetup: false,
    applyMessage: "",
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
    handoverURL() {
      return wifiManagerURL(this.locationURL, this.handoverHostname, this.selected?.SSID || '');
    },
    needsIPHandoverWarning() {
      const host = new URL(this.locationURL).hostname;
      return this.wifi.mode === 'ap' && isIPAddress(host) && (!this.wifi.ip || host === this.wifi.ip);
    },
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
    async useSetupOnEmmc() {
      this.applyingSetup = true; this.applyMessage = '';
      try {
        const body = {source: 'reflash', action: 'apply'};
        if (this.applyRequest) await this.applyRequest('/api/wifi_setup', body);
        else await axios.post('/api/wifi_setup', body, {timeout: 300000});
        this.applyMessage = 'Reflash’s Wi-Fi setup was applied to eMMC for its next boot.';
      } catch (err) {
        this.applyMessage = err.response?.data || 'Could not apply this Wi-Fi setup to eMMC.';
      } finally { this.applyingSetup = false; }
    },
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
        this.inputSSID = this.initialSsid || wifi.data.SSID || "";
        // 204 means a scan is in flight; leave whatever is on screen alone.
        if (aps.status !== 204 && Array.isArray(aps.data)) {
          this.setAvailableAPs(aps.data);
        }
        if (this.initialSsid && !this.availableAPs.some(ap => ap.SSID === this.initialSsid)) {
          this.availableAPs.push({SSID: this.initialSsid, signal: '', label: this.initialSsid});
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
    async startWifiConnect(allowIP = false) {
      if (!this.selected || !this.selected.SSID?.trim()) {
        this.$waveui.notify("Please select an access point", "error", 0);
        return;
      }
      if (this.needsIPHandoverWarning && allowIP !== true) {
        this.ipHandoverWarning = true;
        this.handoverLookup = true;
        try {
          const response = await axios.get('/api/reconnect_address', {timeout: REQUEST_TIMEOUT});
          this.handoverHostname = response.data.hostname || '';
        } catch (_) { this.handoverHostname = ''; }
        finally { this.handoverLookup = false; }
        return;
      }
      if (this.inputPassword.trim().length < 8) {
        this.statusMessage = "Enter the client Wi-Fi password (at least 8 characters) before connecting.";
        this.$waveui.notify(this.statusMessage, "error", 4000);
        return;
      }
      this.ipHandoverWarning = false;
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

      // The server's own verdict on the attempt. The radio state alone cannot
      // give it: iwd names the network while it is still *connecting*, so a
      // wrong passphrase looked exactly like success for the few seconds before
      // it failed, and the watch had stopped by then (#149). null when it
      // cannot be read, which is not a verdict either way.
      let attempt = null;
      if (wifi && wantMode === "station") {
        try {
          const res = await axios.get('/api/wifi_poll_connect', { timeout: REQUEST_TIMEOUT });
          attempt = res.data || null;
        } catch (err) {
          attempt = null;
        }
      }

      if (wifi) {
        if (wifi.mode !== this.reconnectFromMode) {
          this.sawTransition = true;
        }
        if (wantMode === "station" && attempt && !attempt.isConnecting && attempt.error) {
          this.wifi = wifi;
          this.stopReconnectWatch();
          this.statusMessage =
            `Could not join ${target}. The board is back on its own Recore hotspot.`;
          this.$waveui.notify(this.statusMessage, "error", 0);
          return;
        }
        // Only "arrived" once the board reports the state we asked for. Right
        // after the request it is still on the old one, and treating that as
        // success would report a connection that has not happened. For a
        // network that also means an address and an attempt that has finished:
        // named-but-still-associating is not joined.
        const arrived =
          wantMode === "ap"
            ? wifi.mode === "ap"
            : wifi.mode === "station" && wifi.ssid === target && !!wifi.ip &&
              !(attempt && attempt.isConnecting);
        if (arrived) {
          this.wifi = wifi;
          this.isWifiPresent = !!wifi.present;
          this.wifiStatusKnown = true;
          this.stopReconnectWatch();
          this.statusMessage = `Connected to ${target}.`;
          this.$emit("setup-changed");
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
          this.ipHandoverWarning = false;
          this.dialog.show = true;
          this.openDialog();
        } else {
          this.dialog.show = false;
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
.w-dialog__content.wifi-manager-dialog { max-width: calc(100vw - 32px); }
.wifi-manager-content { padding: 0 !important; }
.wifi-dialog-title { font-size: 1.1rem; font-weight: 500; }
.wifi-manager { padding: 18px 24px 20px; text-align: left; }
.wifi-manager p { margin: 0; }
.wifi-manager .wifi-intro { font-size: 0.95rem; margin-bottom: 16px; opacity: 0.8; }
.wifi-connection { display: flex; align-items: center; gap: 10px; font-size: 0.95rem; overflow-wrap: anywhere; }
.wifi-status-dot { width: 8px; height: 8px; flex: 0 0 8px; border-radius: 50%; background: #5cb85c; }
.wifi-manager-section { margin-top: 18px; padding-top: 16px; border-top: 1px solid rgba(var(--w-base-color-rgb), 0.2); }
.wifi-manager-section h4 { margin: 0 0 10px; font-size: 0.95rem; font-weight: 500; }
.wifi-field-label { display: block; font-size: 0.85rem; margin: 12px 0 4px; opacity: 0.8; }
.wifi-manager .wifi-network-select { width: 100%; }
.wifi-manager .wifi-actions { display: flex; justify-content: flex-end; gap: 10px; margin-top: 16px; flex-wrap: wrap; }
.wifi-manager .wifi-connect-button { background: #04a3e5; color: #101010; }
.wifi-manager .wifi-note { font-size: 0.85rem; opacity: 0.7; line-height: 1.5; margin: 10px 0; }
.wifi-manager .wifi-split { display: flex; align-items: center; justify-content: space-between; gap: 16px; }
.wifi-manager .wifi-split h4 { margin-bottom: 0; }
.wifi-manager .wifi-split .wifi-note { margin-bottom: 0; }
.wifi-manager .wifi-message { margin: 14px 0 0; font-size: 0.9rem; overflow-wrap: anywhere; }
.wifi-manager .wifi-next-image { margin-top: 16px; }
.wifi-handover-warning { margin-top: 14px; padding: 12px; border: 1px solid #d9a441; font-size: 0.9rem; line-height: 1.5; }
.wifi-handover-warning p { margin-bottom: 10px; }
.wifi-handover-warning a { color: #04a3e5; text-decoration: underline; }
.wifi-manager-footer { display: flex; justify-content: flex-end; margin-top: 16px; }
@media (max-width: 480px) {
  .wifi-manager { padding: 16px; }
  .wifi-manager .wifi-split { flex-wrap: wrap; }
}
.wifi-inline h3 {
  font-size: 1.05rem;
  margin: 0 0 8px;
}
.wifi-inline .w-select,
.wifi-inline .w-input {
  width: 100% !important;
}
.reconnect-hint {
  opacity: 0.8;
  font-size: 0.9em;
}
</style>