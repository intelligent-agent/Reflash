<template>
  <div class="setup-wrap">
    <div class="setup-bar">
      <button
        type="button"
        class="setup-toggle"
        :aria-expanded="String(expanded)"
        aria-controls="setup-panel"
        @click="expanded = !expanded"
      >
        <span class="chev" aria-hidden="true"></span>Set up printer ...
      </button>
      <div class="setup-meta">
        <span :class="status.error ? 'error' : 'good'">{{ status.text }}</span>
      </div>
    </div>

    <div v-if="expanded" id="setup-panel" class="setup-panel" role="region" aria-label="Set up printer">
      <nav class="setup-nav" aria-label="Sections">
        <a
          v-for="s in sections"
          :key="s.id"
          role="button"
          tabindex="0"
          :aria-current="current === s.id ? 'true' : 'false'"
          @click="goTo(s.id)"
          @keydown.enter.prevent="goTo(s.id)"
          @keydown.space.prevent="goTo(s.id)"
          >{{ s.title }}</a
        >
      </nav>

      <div class="setup-scroll" ref="scroll" @scroll="spy">
        <section id="setup-network" data-title="Network">
          <h4>Network</h4>
          <div class="field emmc-field">
            <label class="lab hostname-label" for="hostname">Hostname</label>
            <span id="hostname-emmc-heading" class="emmc-heading">eMMC</span>
            <input id="hostname" class="input" v-model="hostnameDraft" maxlength="63" @change="setHostname(hostnameDraft)" />
            <div class="emmc-buttons" role="group" aria-labelledby="hostname-emmc-heading">
              <button type="button" class="btn small" :disabled="hostnameBusy || !hostnameCanRead" @click="getInstalledHostname()">{{ hostnameBusy === 'get' ? 'Getting...' : 'Get' }}</button>
              <button type="button" class="btn small" :disabled="hostnameBusy || !hostnameCanSet" @click="applyHostname()">{{ hostnameBusy === 'set' ? 'Setting...' : 'Set' }}</button>
            </div>
            <span class="good emmc-feedback" v-if="hostnameMessage">{{ hostnameMessage }}</span>
            <span class="help emmc-feedback" v-if="hostnameReason">{{ hostnameReason }}</span>
            <span class="error emmc-feedback" v-if="problem.hostname">{{ problem.hostname }}</span>
            <span class="help emmc-feedback">Get reads the currently installed image. Set applies to that image for its next boot. Edits are retained for the next flash. Open <code>{{ options.hostname || 'recore' }}.local</code>.</span>
          </div>
          <div v-if="wifiPresent" class="wifi-settings">
            <div class="field">
              <label class="lab" for="wifi-source">Wi-Fi for the new image</label>
              <select id="wifi-source" class="input" v-model="wifiSource" @change="chooseWifiSource()" :disabled="wifiSetupBusy">
                <option value="reflash">Use from Reflash</option>
                <option value="emmc">Get from eMMC</option>
                <option value="manual">Manual</option>
              </select>
              <span class="help">This setup is installed on the new image. It does not change or test Reflash’s connection.</span>
              <span v-if="wifiSource !== 'manual'" class="help">{{ options.wifiMode || 'auto' }} · Client: {{ options.SSID || 'not configured' }} · Hotspot: {{ options.hotspotSSID || 'Recore' }}</span>
              <button v-if="wifiSource === 'emmc'" class="btn small" :disabled="wifiSetupBusy" @click="chooseWifiSource()">Get again from eMMC</button>
            </div>
            <div v-if="wifiSource === 'manual'" class="field">
              <label class="lab" for="manual-wifi-mode">Wi-Fi mode</label>
              <select id="manual-wifi-mode" class="input" v-model="settingDraft.wifiMode" @change="saveManualWifi()">
                <option value="auto">Automatic — client network with fallback hotspot</option>
                <option value="client">Client only</option>
                <option value="ap">Hotspot only</option>
              </select>
              <span class="help" v-if="effectiveMode === 'auto'">Configure both the client network and fallback hotspot. The new image tries the client network first.</span>
              <div v-show="effectiveMode !== 'ap'">
                <label class="sub" for="client-ssid">Client SSID</label>
                <input id="client-ssid" class="input" v-model="settingDraft.client" @change="saveManualWifi()" />
                <label class="sub" for="client-psk">Client password</label>
                <div class="row">
                  <input id="client-psk" class="input" v-model="clientPsk" :type="show.client ? 'text' : 'password'" :placeholder="options.wifiPSKSet ? 'Configured — leave blank to keep for this SSID' : 'Wi-Fi password'" autocomplete="off" @change="saveManualWifi()" />
                  <button class="btn small plain" @click="show.client = !show.client">{{ show.client ? 'Hide' : 'Show' }}</button>
                </div>
              </div>
              <div v-show="effectiveMode !== 'client'">
                <label class="sub" for="hotspot-name">Hotspot name</label>
                <input id="hotspot-name" class="input" v-model="hotspotName" @change="saveManualWifi()" />
                <label class="sub" for="hotspot-psk">Hotspot password</label>
                <div class="row">
                  <input id="hotspot-psk" class="input" v-model="hotspotPsk" :type="show.hotspot ? 'text' : 'password'" placeholder="Configured — leave blank to keep" autocomplete="off" @change="saveManualWifi()" />
                  <button class="btn small plain" @click="show.hotspot = !show.hotspot">{{ show.hotspot ? 'Hide' : 'Show' }}</button>
                </div>
              </div>
              <button class="btn small" :disabled="wifiSetupBusy" @click="saveManualWifi()">Save for next flash</button>
            </div>
            <span class="good" v-if="wifiSetupMessage" role="status">{{ wifiSetupMessage }}</span>
            <span class="error" v-if="wifiSetupError" role="alert">{{ wifiSetupError }}</span>
            <button class="btn small" @click="wifiManagerOpen = true">Manage Wi-Fi</button>
            <TheWifiSetup :open="wifiManagerOpen" :apply-request="emmcRequest" @close="wifiManagerOpen = false" @setup-changed="refreshWifiChoice()" />
          </div>
          <div v-if="wifiPresent !== true" class="field wifi-check">
            <span class="help">{{ wifiPresent === false ? 'No Wi-Fi adapter detected. You can connect an adapter and check again.' : 'Checking for a Wi-Fi adapter...' }}</span>
            <button type="button" class="btn small" :disabled="wifiChecking" @click="checkWifi()">{{ wifiChecking ? 'Checking...' : 'Check again' }}</button>
            <span class="help" v-if="wifiCheckError">{{ wifiCheckError }}</span>
          </div>
        </section>

        <section id="setup-location" data-title="Location">
          <h4>Location</h4>
          <div class="field emmc-field">
            <label class="lab" for="country">Country</label>
            <select id="country" class="input" v-model="settingDraft.country" @change="saveNextSetting('country')">
              <option value="">Not specified</option>
              <option v-for="c in countries" :key="c[0]" :value="c[0]">{{ c[1] }} ({{ c[0] }})</option>
            </select>
            <EmmcActions label="Country" :can-get="canGetSetting('country')" :can-set="canSetSetting('country')" :busy="settingBusy.country" @get="getInstalledSetting('country')" @set="applyInstalledSetting('country')" />
            <span class="good emmc-feedback" v-if="settingMessage.country">{{ settingMessage.country }}</span>
            <span class="error emmc-feedback" v-if="settingError.country">{{ settingError.country }}</span>
            <span class="help emmc-feedback" v-if="settingReason('country')">{{ settingReason('country') }}</span>
            <span class="help emmc-feedback">Sets the Wi-Fi regulatory domain. Edits are retained for the next flash; Set applies to the installed image.</span>
          </div>
          <div class="field emmc-field">
            <label class="lab" for="timezone">Timezone</label>
            <select id="timezone" class="input" v-model="settingDraft.timezone" @change="saveNextSetting('timezone')">
              <option v-if="!zones.includes(settingDraft.timezone)" :value="settingDraft.timezone">{{ settingDraft.timezone }}</option>
              <option v-for="z in zones" :key="z" :value="z">{{ z }}</option>
            </select>
            <EmmcActions label="Timezone" :can-get="canGetSetting('timezone')" :can-set="canSetSetting('timezone')" :busy="settingBusy.timezone" @get="getInstalledSetting('timezone')" @set="applyInstalledSetting('timezone')" />
            <span class="good emmc-feedback" v-if="settingMessage.timezone">{{ settingMessage.timezone }}</span>
            <span class="error emmc-feedback" v-if="settingError.timezone">{{ settingError.timezone }}</span>
            <span class="help emmc-feedback" v-if="settingReason('timezone')">{{ settingReason('timezone') }}</span>
          </div>
        </section>

        <section id="setup-accounts" data-title="Accounts">
          <h4>Accounts</h4>
          <div class="field">
            <div class="top">
              <label class="lab" for="ssh-password">SSH password (user debian)</label>
            </div>
            <div class="row">
              <input id="ssh-password" class="input" :type="show.ssh ? 'text' : 'password'" v-model="sshPw" :placeholder="options.loginPasswordSet ? '(set)' : 'At least 6 characters'" autocomplete="off" @keydown.enter="setPassword('sshPassword')" />
              <button type="button" class="btn small plain" @click="show.ssh = !show.ssh">{{ show.ssh ? "Hide" : "Show" }}</button>
              <button type="button" class="btn small" :disabled="!sshPw" @click="setPassword('sshPassword')">Set</button>
            </div>
            <span class="error" v-if="problem.sshPassword">{{ problem.sshPassword }}</span>
            <span class="help">The documented factory password is temppwd. Set applies the entered password; existing passwords are not read back.</span>
          </div>
          <div class="field">
            <div class="top">
              <label class="lab" for="root-password">Root password</label>
            </div>
            <div class="row">
              <input id="root-password" class="input" :type="show.root ? 'text' : 'password'" v-model="rootPw" :placeholder="options.rootPasswordSet ? '(set)' : 'At least 6 characters'" autocomplete="off" @keydown.enter="setPassword('rootPassword')" />
              <button type="button" class="btn small plain" @click="show.root = !show.root">{{ show.root ? "Hide" : "Show" }}</button>
              <button type="button" class="btn small" :disabled="!rootPw" @click="setPassword('rootPassword')">Set</button>
            </div>
            <span class="error" v-if="problem.rootPassword">{{ problem.rootPassword }}</span>
            <span class="help">For the serial console; root cannot log in over SSH with a password. The documented factory password is temppwd. Set applies the entered password; existing passwords are not read back.</span>
          </div>
          <div class="field emmc-field">
            <span class="lab">SSH access</span>
            <div class="setting-input">
              <w-switch :model-value="settingDraft.ssh" @update:model-value="settingDraft.ssh = $event; saveNextSetting('ssh')" label="Enable SSH access"></w-switch>
            </div>
            <EmmcActions label="SSH access" :can-get="canGetSetting('ssh')" :can-set="canSetSetting('ssh')" :busy="settingBusy.ssh" @get="getInstalledSetting('ssh')" @set="applyInstalledSetting('ssh')" />
            <span class="good emmc-feedback" v-if="settingMessage.ssh">{{ settingMessage.ssh }}</span>
            <span class="error emmc-feedback" v-if="settingError.ssh">{{ settingError.ssh }}</span>
            <span class="help emmc-feedback" v-if="settingReason('ssh')">{{ settingReason('ssh') }}</span>
            <span class="help emmc-feedback">Get reads SSH access on the installed image; Set applies the selected state to that image.</span>
          </div>
        </section>

        <section id="setup-display" data-title="Display">
          <h4>Display</h4>
          <div class="field emmc-field">
            <span class="lab">Screen rotation</span>
            <div class="radios" role="radiogroup" aria-label="Screen rotation">
              <label v-for="r in rotations" :key="r.value" class="radio"><input type="radio" name="rotation" :value="r.value" :checked="settingDraft.rotation === r.value" @change="settingDraft.rotation = r.value; saveNextSetting('rotation')" />{{ r.label }}</label>
            </div>
            <EmmcActions label="Display rotation" :can-get="canGetSetting('rotation')" :can-set="canSetSetting('rotation')" :busy="settingBusy.rotation" @get="getInstalledSetting('rotation')" @set="applyInstalledSetting('rotation')" />
            <span class="good emmc-feedback" v-if="settingMessage.rotation">{{ settingMessage.rotation }}</span>
            <span class="error emmc-feedback" v-if="settingError.rotation">{{ settingError.rotation }}</span>
            <span class="help emmc-feedback" v-if="settingReason('rotation')">{{ settingReason('rotation') }}</span>
            <span class="help emmc-feedback">Get reads the installed image's display rotation; Set applies the selected rotation to that image.</span>
          </div>
        </section>

        <section id="setup-config" data-title="Config archive">
          <h4>Config archive</h4>

          <div class="field">
            <label class="lab" for="config-install">Config to install</label>
            <w-select id="config-install" :model-value="options.restoreBackup || ''" :items="archiveChoices" no-unselect @update:model-value="chooseConfig($event)" />
            <span class="help">Use the image's own configuration, or restore a saved archive after flashing.</span>
            <button type="button" class="btn small" @click="openArchives()">Manage config archives</button>
          </div>
        </section>

        <section v-if="software.length" id="setup-software" data-title="Optional software">
          <h4>Optional software</h4>
          <div v-for="s in software" :key="s.name" class="field">
            <div class="top"><span class="lab">{{ title(s.name) }}</span></div>
            <w-switch :model-value="!!softwareOn[s.name]" :disabled="!internet && !softwareOn[s.name]" @update:model-value="toggleSoftware(s.name, $event)">Install&nbsp;<code>{{ s.name }}</code></w-switch>
            <span class="error" v-if="!internet && !softwareOn[s.name]">It is installed from GitHub, so it needs an internet connection. Connect the board to the internet to install it.</span>
            <span class="help">{{ s.info }}. Cloned from GitHub, so it updates through Moonraker. Like the other settings, it reaches the installed system at once and goes onto the next image after a flash. Off by default.</span>
            <span class="help">Installed on this printer now: <b>{{ s.installed ? "yes" : "no" }}</b></span>
          </div>
          <p class="sub center">More components can be added to this list later.</p>
        </section>
      </div>

      <div class="setup-foot"><button type="button" class="btn big" @click="expanded = false">Done</button></div>
    </div>
    <w-dialog v-if="managerOpen" v-model="managerOpen" width="80%" :persistent="busy.save || busy.upload || busy.install">
      <template #title>Manage config archives</template>
      <div class="archive-manager setup-wrap">
          <div class="field">
            <div class="top"><label class="lab" for="save-name">Create archive from current image</label></div>
            <div class="row">
              <input id="save-name" class="input" v-model="saveName" placeholder="Archive name" @keydown.enter="save()" />
              <button type="button" class="btn small" :disabled="busy.save" @click="save()">{{ busy.save ? "Creating..." : "Create" }}</button>
            </div>
            <button type="button" class="disc" :aria-expanded="String(tree.save.open)" aria-controls="tree-save" @click="toggleTree('save')">
              <span class="chev" aria-hidden="true"></span>Choose files <span class="tally">{{ tally('save') }}</span>
            </button>
            <div id="tree-save" v-if="tree.save.open">
              <p v-if="tree.save.reason" class="help">{{ tree.save.reason }} The whole config is included.</p>
              <FileTree v-else v-model="tree.save.selected" :files="tree.save.files" />
            </div>
            <span class="help">Every folder in <code>config/</code> is listed, including ones you made yourself. The shipped examples are never saved. The Wi-Fi settings, SSH and the passwords are not part of an archive.</span>
            <span class="good" role="status" v-if="message.save">{{ message.save }}</span>
            <span class="error" v-if="problem.save">{{ problem.save }}</span>
          </div>

          <div class="field">
            <div class="top">
              <span class="lab">Archives on the USB drive</span>
              <span>
                <input ref="upload" type="file" class="hidden" accept=".tar.gz,.tgz,.gz" @change="onUpload" />
                <button type="button" class="btn small plain" :disabled="busy.upload" @click="$refs.upload.click()">{{ busy.upload ? "Uploading..." : "Upload ..." }}</button>
              </span>
            </div>
            <div class="archives">
              <div v-if="!archives.length" class="archive empty">No archives on this USB drive yet.</div>
              <div v-for="a in archives" :key="a.name" class="archive">
                <span class="name" :title="a.name">{{ shortName(a.name) }}<span v-if="!a.ok" class="error"> (damaged)</span></span>
                <span class="acts" v-if="deleting !== a.name">
                  <span>{{ size(a.size) }}</span>
                  <time class="archive-date" :datetime="archiveDateTime(a.date)" title="Date saved on this USB drive">{{ archiveDate(a.date) }}</time>
                  <button type="button" class="act" :disabled="!a.ok || busy.install" @click="chooseConfig(a.name)">{{ options.restoreBackup === a.name ? 'Selected' : 'Select' }}</button>
                  <button type="button" class="act" @click="download(a.name)">Download</button>
                  <button type="button" class="act danger" @click="deleting = a.name">Delete</button>
                </span>
                <span class="acts" v-else>
                  <span>Delete {{ shortName(a.name) }}?</span>
                  <button type="button" class="act danger" @click="remove(a.name)">Delete</button>
                  <button type="button" class="act" @click="deleting = ''">Keep</button>
                </span>
              </div>
            </div>
            <span class="help">Archives are only ever kept on this USB drive.</span>
            <span class="error" v-if="problem.archives">{{ problem.archives }}</span>
          </div>

        <div v-if="options.restoreBackup" class="field">
          <h4>Selected archive: {{ shortName(options.restoreBackup) }}</h4>
          <span class="help">Apply the selected files to the currently installed image on eMMC. This does not flash a new image.</span>
            <div v-if="options.restoreBackup">
              <button type="button" class="disc" :aria-expanded="String(tree.install.open)" aria-controls="tree-install" @click="toggleTree('install')">
                <span class="chev" aria-hidden="true"></span>Choose files <span class="tally">{{ tally('install') }}</span>
              </button>
              <div id="tree-install" v-if="tree.install.open">
                <p v-if="tree.install.reason" class="help">{{ tree.install.reason }} All of it is installed.</p>
                <FileTree v-else v-model="tree.install.selected" :files="tree.install.files" @update:modelValue="installSelectionChanged" />
              </div>
            </div>
            <div class="row center">
              <button type="button" class="btn small" :disabled="!options.restoreBackup || busy.install" @click="installNow()">{{ busy.install ? "Installing..." : "Use this config on eMMC" }}</button>
              <span class="good" v-if="message.install">{{ message.install }}</span>
            </div>
            <span class="error" v-if="problem.install">{{ problem.install }}</span>
        </div>
      </div>
      <div class="setup-foot archive-manager-foot setup-wrap"><button type="button" class="btn big" :disabled="busy.save || busy.upload || busy.install" @click="managerOpen = false">Done</button></div>
    </w-dialog>
  </div>
</template>

<script>
import axios from "axios";
import { mapGetters, mapActions } from "vuex";
import EmmcActions from "./EmmcActions.vue";
import TheWifiSetup from "./TheWifiSetup.vue";
import FileTree from "./FileTree.vue";
import { includeFor } from "../fileTree";
import { uploadArchive } from "../archiveUpload";
import { countries, timezones } from "../countries";

// What each option is when the Default box is ticked: what is sent to put the
// installed system back, and what an unchanged option already is.
const DEFAULTS = {
  hostname: { hostname: "" },
  wifiMode: { wifiMode: "" },
  hotspot: { hotspotSSID: "", hotspotPSK: "" },
  country: { wifiCountry: "" },
  timezone: { timezone: "" },
  sshPassword: { loginPassword: "" },
  rootPassword: { rootPassword: "" },
  ssh: { enableSsh: false },
  rotation: { screenRotation: 0 },
  config: { restoreBackup: "", restoreInclude: "" },
};

const EMMC_SETTINGS = {
  wifiMode: { key: 'WIFI_MODE', option: 'wifiMode', label: 'Wi-Fi mode' },
  ssh: { key: 'SSH_ENABLED', option: 'enableSsh', label: 'SSH access' },
  country: { key: 'WIFI_COUNTRY', option: 'wifiCountry', label: 'Country' },
  timezone: { key: 'TIMEZONE', option: 'timezone', label: 'Timezone' },
  rotation: { key: 'SCREEN_ROTATION', option: 'screenRotation', label: 'Display rotation' },
  hotspot: { key: 'HOTSPOT_SSID', option: 'hotspotSSID', label: 'AP name' },
  client: { key: 'WIFI_SSID', option: 'SSID', label: 'Client SSID' },
};

const shortName = (name) => String(name || "").replace(/\.tar\.gz$/, "");

export default {
  name: "TheSetup",
  components: { TheWifiSetup, FileTree, EmmcActions },
  props: {
    // Whether the board can reach the internet: software that is cloned from
    // GitHub cannot be switched on without (#205). Switching it off can.
    internet: { type: Boolean, default: true },
  },
  data: () => ({
    expanded: false,
    managerOpen: false,
    hostnameDraft: "recore",
    hostnameBusy: "",
    hostnameCanRead: false,
    hostnameCanSet: false,
    hostnameReason: "Checking the installed image’s hostname support...",
    hostnameMessage: "",
    installedSettings: { supported: false, keys: [], current: {} },
    settingDraft: { wifiMode: 'auto', ssh: false, country: '', timezone: 'UTC', rotation: 0, hotspot: 'Recore', client: '' },
    settingBusy: {},
    settingMessage: {},
    settingError: {},
    wifiSource: 'manual',
    wifiManagerOpen: false,
    wifiSetupBusy: false,
    wifiSaveQueued: false,
    wifiSetupMessage: '',
    wifiSetupError: '',
    clientPsk: '',
    wifiPresent: null,
    wifiChecking: false,
    wifiCheckError: "",
    current: "setup-network",
    // Options left alone but shown as set: ticking Default again is a choice.
    custom: {},
    seeded: false,
    show: { ssh: false, root: false, hotspot: false, client: false },
    sshPw: "",
    rootPw: "",
    hotspotName: "",
    hotspotPsk: "",
    problem: {},
    message: {},
    busy: { save: false, upload: false, install: false },
    saveName: "",
    archives: [],
    deleting: "",
    tree: {
      save: { open: false, files: [], selected: [], reason: "", loaded: false },
      install: { open: false, files: [], selected: [], reason: "", loaded: false, name: "" },
    },
    // One is always chosen. Automatic is what the image does when nothing is
    // said, and is the Default.
    wifiModes: [
      { label: "Automatic", value: "" },
      { label: "Client only", value: "client" },
      { label: "Access point only", value: "ap" },
    ],
    rotations: [
      { label: "Normal", value: 0 },
      { label: "90 degrees", value: 90 },
      { label: "180 degrees", value: 180 },
      { label: "270 degrees", value: 270 },
    ],
    countries,
    zones: timezones(),
    pollTimer: null,
  }),
  computed: {
    ...mapGetters(["options"]),
    sections() {
      const s = [
        { id: "setup-network", title: "Network" },
        { id: "setup-location", title: "Location" },
        { id: "setup-accounts", title: "Accounts" },
        { id: "setup-display", title: "Display" },
        { id: "setup-config", title: "Config archive" },
      ];
      if (this.software.length) s.push({ id: "setup-software", title: "Optional software" });
      return s;
    },
    archiveChoices() {
      return [{ label: "Default — image configuration", value: "" }, ...this.archives.filter(archive => archive.ok).map((archive) => ({ label: shortName(archive.name), value: archive.name }))];
    },
    effectiveMode() {
      return this.settingDraft.wifiMode;
    },
    software() {
      return this.options.softwareAvailable || [];
    },
    softwareOn() {
      const on = {};
      (this.options.software || "").split(",").forEach((n) => n && (on[n] = true));
      return on;
    },
    changed() {
      return Object.keys(DEFAULTS).filter((k) => !this.isDef(k)).length + Object.keys(this.softwareOn).length;
    },
    countText() {
      return this.changed === 0 ? "All options at default" : `${this.changed} changed from default`;
    },
    status() {
      if (this.options.settingsSyncError) return { text: "Not saved to the installed system: " + this.options.settingsSyncError, error: true };
      if (this.options.settingsSyncBusy) return { text: "Saving to the installed system ...", error: false };
      return { text: "Settings ready", error: false };
    },
  },
  methods: {
    ...mapActions(["getOptions", "setOption"]),
    shortName,
    title(name) {
      return name === "led_effect" ? "LED effects" : name;
    },
    size(bytes) {
      return bytes >= 1024 * 1024 ? (bytes / 1048576).toFixed(1) + " MB" : Math.max(1, Math.round(bytes / 1024)) + " KB";
    },
    // ---- Default boxes ----
    // An option is at default until the user leaves it; what the server holds
    // says where to start.
    seed() {
      const o = this.options;
      if (this.seeded || o.wifiMode === undefined) return;
      this.custom = {
        hostname: !!o.hostname,
        wifiMode: !!o.wifiMode,
        hotspot: !!(o.hotspotSSID || o.hotspotPSKSet),
        country: !!o.wifiCountry,
        timezone: !!o.timezone,
        sshPassword: !!o.loginPasswordSet,
        rootPassword: !!o.rootPasswordSet,
        ssh: !!o.enableSsh,
        rotation: (o.screenRotation || 0) !== 0,
        config: !!o.restoreBackup,
      };
      // A documented factory suggestion, not a password read from the image.
      if (!o.loginPasswordSet) this.sshPw = "temppwd";
      if (!o.rootPasswordSet) this.rootPw = "temppwd";
      if (!o.hotspotPSKSet) this.hotspotPsk = "12345678";
      this.hostnameDraft = o.hostname || "recore";
      this.hotspotName = o.hotspotSSID || "Recore";
      this.wifiSource = o.wifiSource || 'manual';
      this.settingDraft = { wifiMode: o.wifiMode || 'auto', ssh: !!o.enableSsh, country: o.wifiCountry || '', timezone: o.timezone || 'UTC', rotation: o.screenRotation || 0, hotspot: this.hotspotName, client: o.SSID || '' };
      this.seeded = true;
    },
    isDef(name) {
      return !this.custom[name];
    },
    // The box: ticked puts the option back to what the image does.
    keep(name, on) {
      this.custom = { ...this.custom, [name]: !on };
      if (on) {
        this.setOption(DEFAULTS[name]);
        this.problem = { ...this.problem, [name]: "" };
        if (name === "sshPassword") this.sshPw = "";
        if (name === "rootPassword") this.rootPw = "";
        if (name === "hotspot") {
          this.hotspotName = "";
          this.hotspotPsk = "";
        }
        if (name === "config") {
          this.tree.install.loaded = false;
          this.tree.install.open = false;
        }
      }
    },
    // Automatic is Default, so choosing it ticks the box again.
    chooseMode(value) {
      this.custom = { ...this.custom, wifiMode: value !== "" };
      this.setOption({ wifiMode: value });
    },
    edit(name, change) {
      this.custom = { ...this.custom, [name]: true };
      this.setOption(change);
    },
    resetAll() {
      Object.keys(DEFAULTS).forEach((k) => this.keep(k, true));
      if (this.software.length) this.setOption({ software: "" });
    },
    // ---- Passwords and the hotspot's: set on purpose, not on every key ----
    async settle() {
      // The installed system answers within a few seconds; its refusal is what
      // the user needs to read.
      for (let i = 0; i < 40; i++) {
        await new Promise((r) => setTimeout(r, 500));
        await this.getOptions();
        if (!this.options.settingsSyncBusy) break;
      }
      return this.options.settingsSyncError || "";
    },
    async setPassword(kind) {
      const root = kind === "rootPassword";
      const pw = root ? this.rootPw : this.sshPw;
      if (pw.length < 6) {
        this.problem = { ...this.problem, [kind]: "The password is too short: at least 6 characters." };
        return;
      }
      this.problem = { ...this.problem, [kind]: "" };
      this.custom = { ...this.custom, [kind]: true };
      await this.setOption(root ? { rootPassword: pw } : { loginPassword: pw });
      const err = await this.settle();
      if (err) {
        this.problem = { ...this.problem, [kind]: err };
        this.custom = { ...this.custom, [kind]: false };
      }
    },
    async setHostname(value) {
      const hostname = value.trim().toLowerCase() || "recore";
      if (hostname.length > 63 || !/^[a-z0-9]([a-z0-9-]*[a-z0-9])?$/.test(hostname)) {
        this.problem = { ...this.problem, hostname: "Use 1 to 63 letters, digits or hyphens, starting and ending with a letter or digit, without .local." };
        return false;
      }
      this.problem = { ...this.problem, hostname: "" };
      this.hostnameDraft = hostname;
      try {
        await axios.post('/api/set_options?apply=false', { hostname });
        await this.getOptions();
        this.hostnameMessage = `Selected ${hostname} for the next image.`;
        return true;
      } catch (err) {
        this.problem = { ...this.problem, hostname: String(this.errorOf(err)) };
        return false;
      }
    },
    async installedSettingsRequest(changes) {
      return this.emmcRequest('/api/installed_settings', changes);
    },
    async emmcRequest(url, changes, timeout = changes === undefined ? 15000 : 300000) {
      let saveDeadline = 0;
      for (;;) {
        try {
          return changes === undefined
            ? await axios.get(url, { timeout })
            : await axios.post(url, changes, { timeout });
        } catch (err) {
          // A SAVING conflict is returned before the installer runs. Only this
          // brief options-save conflict is safe to retry; never replay a timed
          // out POST or retry across an image flash.
          if (err.response?.status !== 409 || String(err.response.data).trim() !== 'busy: SAVING') throw err;
          if (!saveDeadline) saveDeadline = Date.now() + 20000;
          for (;;) {
            if (Date.now() >= saveDeadline) throw new Error('Options are still being saved. Try again shortly.');
            await new Promise(resolve => setTimeout(resolve, 250));
            const { data } = await axios.get('/api/get_progress', { timeout: Math.min(3000, Math.max(1, saveDeadline - Date.now())) });
            if (data.state !== 'SAVING') {
              if (!['IDLE', 'FINISHED', 'ERROR', 'CANCELLED'].includes(data.state)) throw new Error(`Reflash is busy: ${data.state || 'unknown state'}. Try again when it is idle.`);
              break;
            }
          }
        }
      }
    },
    async readInstalledSettings() {
      const { data } = await this.installedSettingsRequest();
      this.installedSettings = data;
      this.hostnameCanRead = typeof data.current?.HOSTNAME === 'string';
      this.hostnameCanSet = !!data.supported && (data.keys || []).includes('HOSTNAME');
      this.hostnameReason = data.reason || (!this.hostnameCanSet ? 'This installed image cannot apply hostname changes. Your selection is still used for the next flash.' : '');
      return data;
    },
    async readHostnameSupport() {
      return (await this.readInstalledSettings()).current?.HOSTNAME;
    },
    async checkHostnameSupport() {
      try { await this.readHostnameSupport(); }
      catch (err) {
        this.hostnameCanRead = false;
        this.hostnameCanSet = false;
        this.hostnameReason = `Could not read the installed image: ${this.errorOf(err)}`;
      }
    },
    async getInstalledHostname() {
      this.hostnameBusy = 'get';
      this.hostnameMessage = '';
      this.problem = { ...this.problem, hostname: '' };
      try {
        const hostname = await this.readHostnameSupport();
        if (hostname === undefined) throw new Error(this.hostnameReason || 'The installed image cannot return a hostname.');
        if (await this.setHostname(hostname)) this.hostnameMessage = `Read ${this.hostnameDraft} from the installed image; selected for the next flash.`;
      } catch (err) { this.problem = { ...this.problem, hostname: String(this.errorOf(err)) }; }
      finally { this.hostnameBusy = ''; }
    },
    async applyHostname() {
      this.hostnameBusy = 'set';
      this.hostnameMessage = '';
      try {
        if (!await this.setHostname(this.hostnameDraft)) return;
        const { data } = await this.installedSettingsRequest({ HOSTNAME: this.hostnameDraft });
        if (data.status !== 'OK') throw new Error(data.error || data.reason || 'The installed image refused the hostname.');
        this.hostnameMessage = `Set ${this.hostnameDraft} on the installed image; takes effect at its next boot.`;
      } catch (err) { this.problem = { ...this.problem, hostname: String(this.errorOf(err)) }; }
      finally { this.hostnameBusy = ''; }
    },
    canGetSetting(name) {
      return Object.prototype.hasOwnProperty.call(this.installedSettings.current || {}, EMMC_SETTINGS[name].key);
    },
    canSetSetting(name) {
      return !!this.installedSettings.supported && (this.installedSettings.keys || []).includes(EMMC_SETTINGS[name].key);
    },
    settingReason(name) {
      if (this.canGetSetting(name) && this.canSetSetting(name)) return '';
      if (this.installedSettings.reason) return this.installedSettings.reason;
      if (!this.canSetSetting(name)) return `${EMMC_SETTINGS[name].label} cannot be applied by this installed image. Edits can still be used for the next flash.`;
      return `This installed image cannot return ${EMMC_SETTINGS[name].label.toLowerCase()}.`;
    },
    async saveNextSetting(name) {
      const spec = EMMC_SETTINGS[name];
      let value = name === 'hotspot' ? this.hotspotName : this.settingDraft[name];
      this.settingError = { ...this.settingError, [name]: '' };
      if (name === 'country' && value && !/^[A-Z]{2}$/.test(value)) {
        this.settingError = { ...this.settingError, [name]: 'Choose a valid country.' }; return false;
      }
      if (name === 'rotation' && ![0, 90, 180, 270].includes(Number(value))) {
        this.settingError = { ...this.settingError, [name]: 'Choose a supported display rotation.' }; return false;
      }
      if (name === 'rotation') value = Number(value);
      if (name === 'wifiMode' && !['auto', 'client', 'ap'].includes(value)) {
        this.settingError = { ...this.settingError, [name]: 'Choose Automatic, Client only, or Access point only.' }; return false;
      }
      if (name === 'ssh' && typeof value !== 'boolean') {
        this.settingError = { ...this.settingError, [name]: 'Choose whether SSH is enabled.' }; return false;
      }
      if (['client', 'hotspot'].includes(name) && (/[\r\n]/.test(value) || new TextEncoder().encode(value).length > 32 || (name === 'hotspot' && !value.trim()))) {
        this.settingError = { ...this.settingError, [name]: 'Use a Wi-Fi name of at most 32 bytes without line breaks.' }; return false;
      }
      try {
        await axios.post('/api/set_options?apply=false', { [spec.option]: name === 'wifiMode' && value === 'auto' ? '' : value });
        this.settingDraft[name] = value;
        await this.getOptions();
        this.settingMessage = { ...this.settingMessage, [name]: 'Selected for the next flash.' };
        return true;
      } catch (err) {
        this.settingError = { ...this.settingError, [name]: String(this.errorOf(err)) }; return false;
      }
    },
    async getInstalledSetting(name) {
      this.settingBusy = { ...this.settingBusy, [name]: 'get' };
      this.settingMessage = { ...this.settingMessage, [name]: '' };
      try {
        const data = await this.readInstalledSettings();
        if (!Object.prototype.hasOwnProperty.call(data.current || {}, EMMC_SETTINGS[name].key)) throw new Error(this.settingReason(name));
        let value = data.current[EMMC_SETTINGS[name].key];
        if (name === 'rotation') value = Number(value);
        if (name === 'ssh') {
          if (!['true', 'false'].includes(value)) throw new Error('The installed image returned an invalid SSH access state.');
          value = value === 'true';
        }
        if (name === 'timezone') value = value || 'UTC';
        if (name === 'wifiMode') value = value || 'auto';
        if (name === 'hotspot') { value = value || 'Recore'; this.hotspotName = value; }
        this.settingDraft[name] = value;
        if (await this.saveNextSetting(name)) this.settingMessage = { ...this.settingMessage, [name]: 'Read from the installed image; selected for the next flash.' };
      } catch (err) { this.settingError = { ...this.settingError, [name]: String(this.errorOf(err)) }; }
      finally { this.settingBusy = { ...this.settingBusy, [name]: '' }; }
    },
    async applyInstalledSetting(name) {
      this.settingBusy = { ...this.settingBusy, [name]: 'set' };
      this.settingMessage = { ...this.settingMessage, [name]: '' };
      try {
        if (!this.canSetSetting(name)) throw new Error(this.settingReason(name));
        if (!await this.saveNextSetting(name)) return;
        const { data } = await this.installedSettingsRequest({ [EMMC_SETTINGS[name].key]: String(this.settingDraft[name]) });
        if (data.status !== 'OK') throw new Error(data.error || data.reason || 'The installed image refused the setting.');
        this.settingMessage = { ...this.settingMessage, [name]: 'Applied to the installed image on eMMC.' };
      } catch (err) { this.settingError = { ...this.settingError, [name]: String(this.errorOf(err)) }; }
      finally { this.settingBusy = { ...this.settingBusy, [name]: '' }; }
    },
    async setHotspotName() {
      if (!this.hotspotName.trim()) {
        this.problem = { ...this.problem, hotspot: "Enter a hotspot name, for example Recore." };
        return;
      }
      this.problem = { ...this.problem, hotspot: "" };
      this.custom = { ...this.custom, hotspot: true };
      await this.setOption({ hotspotSSID: this.hotspotName });
    },
    async setHotspotPassword() {
      if (this.hotspotPsk.length < 8 || this.hotspotPsk.length > 63) {
        this.problem = { ...this.problem, hotspot: "The hotspot's password must be 8 to 63 characters." };
        return;
      }
      this.problem = { ...this.problem, hotspot: "" };
      this.custom = { ...this.custom, hotspot: true };
      await this.setOption({ hotspotPSK: this.hotspotPsk });
      const err = await this.settle();
      if (err) this.problem = { ...this.problem, hotspot: err };
    },
    // ---- Optional software ----
    toggleSoftware(name, on) {
      const now = { ...this.softwareOn };
      if (on) now[name] = true;
      else delete now[name];
      this.setOption({ software: Object.keys(now).sort().join(",") });
    },
    // ---- Config archives ----
    async openArchives() {
      this.managerOpen = true;
      await this.listArchives();
      if (!this.saveName) {
        try { await this.readInstalledSettings(); } catch (_) { /* Use the saved hostname if the image cannot be read. */ }
        this.saveName = this.suggestArchiveName();
      }
    },
    async refreshWifiChoice() {
      await this.getOptions();
      if (this.wifiSource === 'reflash') this.syncWifiDraft();
    },
    syncWifiDraft() {
      this.settingDraft.wifiMode = this.options.wifiMode || 'auto';
      this.settingDraft.client = this.options.SSID || '';
      this.hotspotName = this.options.hotspotSSID || 'Recore';
      this.clientPsk = '';
      this.hotspotPsk = this.options.hotspotPSKSet ? '' : '12345678';
    },
    async chooseWifiSource() {
      if (this.wifiSource === 'manual') {
        this.syncWifiDraft();
        await this.saveManualWifi();
        return;
      }
      await this.storeWifiSetup(this.wifiSource);
    },
    async saveManualWifi() {
      if (this.wifiSetupBusy) { this.wifiSaveQueued = true; return; }
      await this.storeWifiSetup('manual', {
        WIFI_MODE: this.settingDraft.wifiMode,
        WIFI_SSID: this.settingDraft.client,
        WIFI_PSK: this.clientPsk,
        HOTSPOT_SSID: this.hotspotName,
        HOTSPOT_PSK: this.hotspotPsk,
      });
    },
    async storeWifiSetup(source, settings) {
      if (this.wifiSetupBusy) return;
      this.wifiSetupBusy = true;
      this.wifiSetupMessage = ''; this.wifiSetupError = '';
      try {
        await this.emmcRequest('/api/wifi_setup', { source, action: 'stage', settings });
        await this.getOptions();
        if (source !== 'manual') this.syncWifiDraft();
        this.wifiSetupMessage = 'Selected for the next flash. Reflash’s connection is unchanged.';
      } catch (err) {
        this.wifiSetupError = err.response?.data || 'Could not save the Wi-Fi setup. Complete the fields and try again.';
      } finally {
        this.wifiSetupBusy = false;
        if (this.wifiSaveQueued) { this.wifiSaveQueued = false; await this.saveManualWifi(); }
      }
    },
    async checkWifi() {
      if (this.wifiChecking) return;
      this.wifiChecking = true;
      this.wifiCheckError = "";
      try {
        const result = await axios.get('/api/get_status', { timeout: 3000 });
        this.wifiPresent = !!result.data.network?.wifi?.present;
      } catch (_) {
        this.wifiCheckError = "Could not check for a Wi-Fi adapter. Check again when the board is reachable.";
      } finally { this.wifiChecking = false; }
    },
    suggestArchiveName() {
      const hostname = this.installedSettings.current?.HOSTNAME || this.options.hostname || 'recore';
      const name = hostname.replace(/[^A-Za-z0-9._-]/g, '-');
      const now = new Date();
      const pad = n => String(n).padStart(2, '0');
      const stamp = `${now.getFullYear()}${pad(now.getMonth() + 1)}${pad(now.getDate())}-${pad(now.getHours())}${pad(now.getMinutes())}${pad(now.getSeconds())}`;
      const base = `${name}-config-${stamp}`;
      let result = base;
      let index = 2;
      while (this.archives.some(a => a.name === result + '.tar.gz')) result = `${base}-${index++}`;
      return result;
    },
    archiveDateTime(seconds) {
      const value = Number(seconds);
      return Number.isFinite(value) && value > 0 ? new Date(value * 1000).toISOString() : undefined;
    },
    archiveDate(seconds) {
      const value = Number(seconds);
      return Number.isFinite(value) && value > 0
        ? new Date(value * 1000).toLocaleString(undefined, { year: 'numeric', month: '2-digit', day: '2-digit', hour: '2-digit', minute: '2-digit' })
        : '—';
    },
    async listArchives() {
      try {
        this.archives = (await axios.get(`/api/file_backups`)).data || [];
      } catch (err) {
        this.problem = { ...this.problem, archives: "Could not list config archives: " + this.errorOf(err) };
      }
    },
    notify(message, type = "success", time = 6000) {
      if (this.$waveui) this.$waveui.notify(message, type, time);
    },
    errorOf(err) {
      return (err && err.response && err.response.data) || String(err);
    },
    async loadFiles(which) {
      const t = this.tree[which];
      t.reason = "";
      const url = which === "save" ? `/api/file_backups/files` : `/api/file_backups/files?name=${encodeURIComponent(this.options.restoreBackup)}`;
      try {
        const res = (await this.emmcRequest(url)).data;
        t.sourceFiles = res.supported ? res.files : [];
        t.files = t.sourceFiles.filter(path => !/(^|\/)config\/firmware\/README$/.test(path));
        t.reason = res.supported ? "" : res.reason || "The system cannot list its files.";
        t.selected = [...t.files];
        // The archive chosen before, with only some of its files.
        if (which === "install" && res.supported && this.options.restoreInclude) {
          const inc = this.options.restoreInclude.split("\n").filter(Boolean);
          t.selected = t.files.filter((f) => inc.some((p) => f === p || f.startsWith(p + "/")));
        }
      } catch (err) {
        t.files = [];
        t.reason = typeof this.errorOf(err) === "string" ? this.errorOf(err) : "The files could not be listed.";
      }
      t.loaded = true;
      if (which === "install") t.name = this.options.restoreBackup;
    },
    async toggleTree(which) {
      const t = this.tree[which];
      t.open = !t.open;
      if (t.open && (!t.loaded || (which === "install" && t.name !== this.options.restoreBackup))) await this.loadFiles(which);
    },
    tally(which) {
      const t = this.tree[which];
      if (!t.loaded || t.reason) return "";
      return t.selected.length === t.files.length ? `all ${t.files.length} files` : `${t.selected.length} of ${t.files.length} files`;
    },
    // What to hand the installer: nothing for "all of it".
    includeOf(which) {
      const t = this.tree[which];
      if (!t.loaded || t.reason) return [];
      return includeFor(t.sourceFiles || t.files, t.selected);
    },
    async save() {
      if (this.busy.save) return;
      this.busy.save = true;
      this.problem = { ...this.problem, save: '' };
      this.message = { ...this.message, save: '' };
      try {
        // Keep the complete source list for includeFor so a selected folder
        // cannot accidentally bring its omitted firmware README back in.
        if (!this.tree.save.loaded) await this.loadFiles('save');
        const include = this.includeOf('save');
        if (include === null) throw new Error('No files are chosen.');
        const { data } = await this.emmcRequest('/api/file_backups', { name: this.saveName, include }, 600000);
        if (data.status !== 'OK') throw new Error(data.error || 'The backup failed');
        this.message = { ...this.message, save: `Created ${data.name} on the USB drive.` };
        await this.listArchives();
        this.saveName = this.suggestArchiveName();
      } catch (err) {
        this.problem = { ...this.problem, save: String(this.errorOf(err)) };
      } finally { this.busy.save = false; }
    },
    async onUpload(e) {
      const file = e.target.files && e.target.files[0];
      e.target.value = "";
      if (!file) return;
      this.problem = { ...this.problem, archives: "" };
      this.busy.upload = true;
      try {
        await uploadArchive(file);
        this.notify(`Uploaded ${file.name} to the USB drive`);
      } catch (err) {
        this.problem = { ...this.problem, archives: "The upload failed: " + this.errorOf(err) };
      }
      this.busy.upload = false;
      this.listArchives();
    },
    download(name) {
      window.location = `/api/file_backups/download?name=${encodeURIComponent(name)}`;
    },
    async remove(name) {
      this.deleting = "";
      try {
        await axios.post(`/api/delete_image`, { filename: name });
        if (this.options.restoreBackup === name) this.keep("config", true);
      } catch (err) {
        this.problem = { ...this.problem, archives: this.errorOf(err) };
      }
      this.listArchives();
    },
    async chooseConfig(name) {
      this.custom = { ...this.custom, config: !!name };
      this.tree.install.loaded = false;
      await this.setOption({ restoreBackup: name, restoreInclude: "" });
      if (this.tree.install.open) await this.loadFiles("install");
    },
    // A partial choice goes with the install that follows an image, and a whole
    // one is no list at all.
    installSelectionChanged() {
      const include = this.includeOf("install");
      this.setOption({ restoreInclude: Array.isArray(include) ? include.join("\n") : "" });
    },
    async installNow() {
      if (this.busy.install) return;
      this.problem = { ...this.problem, install: "" };
      this.message = { ...this.message, install: "" };
      this.busy.install = true;
      const name = this.options.restoreBackup;
      try {
        const include = this.includeOf("install");
        if (include === null) throw new Error("No files are chosen.");
        const res = await axios.post(`/api/file_backups/restore`, { name, include });
        if (res.data.status !== "OK") throw new Error(res.data.error || "Installing the config failed");
        const message = `Installed ${shortName(name)}.`;
        this.message = { ...this.message, install: message };
        this.notify(message);
      } catch (err) {
        const message = err instanceof Error && !err.response ? err.message : String(this.errorOf(err));
        this.problem = { ...this.problem, install: message };
        this.notify(message, "error");
      } finally {
        this.busy.install = false;
      }
    },
    // ---- The sidebar ----
    goTo(id) {
      const el = this.$refs.scroll && this.$refs.scroll.querySelector("#" + id);
      if (el) this.$refs.scroll.scrollTo({ top: el.offsetTop - this.$refs.scroll.offsetTop });
    },
    spy() {
      const sc = this.$refs.scroll;
      if (!sc) return;
      let cur = this.sections[0].id;
      this.sections.forEach((s) => {
        const el = sc.querySelector("#" + s.id);
        if (el && el.offsetTop - sc.offsetTop <= sc.scrollTop + 40) cur = s.id;
      });
      if (sc.scrollTop + sc.clientHeight >= sc.scrollHeight - 4) cur = this.sections[this.sections.length - 1].id;
      this.current = cur;
    },
  },
  watch: {
    options: { handler: "seed", deep: false },
    // While the panel is open the page follows what the server holds: a
    // password refused, a push finished.
    expanded(open) {
      clearInterval(this.pollTimer);
      if (open) {
        this.getOptions();
        this.checkWifi();
        this.checkHostnameSupport();
        this.listArchives();
        this.pollTimer = setInterval(() => {
          if (this.options.settingsSyncBusy) this.getOptions();
        }, 1500);
      }
    },
  },
  created() {
    this.seed();
  },
  beforeUnmount() {
    clearInterval(this.pollTimer);
  },
};
</script>

<style>
.setup-wrap {
  margin-top: 22px;
  text-align: left;
}
.setup-bar {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 12px;
  flex-wrap: wrap;
  border-top: 1px solid rgba(var(--w-base-color-rgb), 0.2);
  border-bottom: 1px solid rgba(var(--w-base-color-rgb), 0.2);
  padding: 6px 4px;
}
.setup-toggle {
  display: inline-flex;
  align-items: center;
  gap: 10px;
  flex: 1 1 200px;
  text-align: left;
  background: none;
  border: 0;
  cursor: pointer;
  font: inherit;
  font-size: 1.15rem;
  padding: 6px 8px;
  color: inherit;
}
.setup-toggle:focus-visible,
.setup-wrap .btn:focus-visible,
.setup-wrap .link:focus-visible,
.setup-wrap .disc:focus-visible,
.setup-nav a:focus-visible {
  outline: 1px solid #04a3e5;
  outline-offset: 2px;
}
.setup-wrap .chev {
  width: 8px;
  height: 8px;
  border-right: 1.5px solid currentColor;
  border-bottom: 1.5px solid currentColor;
  transform: rotate(-45deg);
  transition: transform 0.15s;
}
.setup-toggle[aria-expanded="true"] .chev,
.setup-wrap .disc[aria-expanded="true"] .chev {
  transform: rotate(45deg);
}
.setup-meta {
  display: flex;
  align-items: center;
  gap: 14px;
  font-size: 0.8rem;
  opacity: 0.85;
  flex-wrap: wrap;
}
.setup-wrap .good {
  color: #5cb85c;
}
.setup-wrap .error {
  color: #e0604f;
  font-size: 0.85rem;
}
.setup-wrap .link {
  background: none;
  border: 0;
  padding: 0;
  color: #04a3e5;
  cursor: pointer;
  font: inherit;
}

.setup-panel {
  display: grid;
  grid-template-columns: 170px minmax(0, 1fr);
  grid-template-rows: minmax(0, 1fr) auto;
  height: 460px;
  background: rgba(var(--w-base-color-rgb), 0.04);
}
.setup-nav {
  grid-row: 1 / 3;
  border-right: 1px solid rgba(var(--w-base-color-rgb), 0.2);
  padding: 14px 0;
  display: flex;
  flex-direction: column;
  overflow: auto;
}
.setup-nav a {
  display: block;
  padding: 9px 20px;
  cursor: pointer;
  opacity: 0.6;
  border-left: 2px solid transparent;
}
.setup-nav a:hover {
  opacity: 1;
}
.setup-nav a[aria-current="true"] {
  opacity: 1;
  border-left-color: #04a3e5;
}
.setup-scroll {
  min-height: 0;
  overflow-y: auto;
  padding: 0 28px 12px;
  scroll-behavior: smooth;
}
.setup-scroll section {
  padding-block: 16px;
  border-top: 1px solid rgba(var(--w-base-color-rgb), 0.2);
}
.setup-scroll section:first-child {
  border-top: 0;
}
.setup-scroll h4 {
  text-align: center;
  font-size: 1.1rem;
  margin: 0 0 12px;
}
.setup-foot {
  border-top: 1px solid rgba(var(--w-base-color-rgb), 0.2);
  padding: 10px 20px;
  display: flex;
  justify-content: flex-end;
}

.setup-wrap .field {
  display: flex;
  flex-direction: column;
  gap: 8px;
  padding-block: 10px;
}
.setup-wrap .field .top {
  display: flex;
  justify-content: space-between;
  align-items: baseline;
  gap: 12px;
}
.setup-wrap .lab,
.setup-wrap .sub {
  font-size: 0.85rem;
  opacity: 0.7;
}
.setup-wrap .help {
  font-size: 0.78rem;
  opacity: 0.7;
}
.setup-wrap .help b {
  font-weight: 400;
  opacity: 1;
}
.setup-wrap .field.is-default > :not(.top):not(.help):not(.error) {
  opacity: 0.45;
}
.setup-wrap .keep {
  display: inline-flex;
  align-items: center;
  gap: 6px;
  font-size: 0.8rem;
  opacity: 0.8;
  cursor: pointer;
  white-space: nowrap;
}
.setup-wrap .keep input {
  accent-color: #04a3e5;
  margin: 0;
}
.setup-wrap .input {
  width: 100%;
  background: none;
  border: 0;
  border-bottom: 1px solid rgba(var(--w-base-color-rgb), 0.4);
  padding: 6px 2px;
  border-radius: 0;
  min-width: 0;
  color: inherit;
  font: inherit;
}
.setup-wrap select.input option {
  background: var(--w-base-bg-color-rgb);
  color: inherit;
}
.setup-wrap .row {
  display: flex;
  gap: 12px;
  align-items: flex-end;
  min-width: 0;
}
.setup-wrap .row.center {
  align-items: center;
}
.setup-wrap .row .input {
  flex: 1;
}
.setup-wrap .btn {
  background: none;
  border: 1px solid #04a3e5;
  padding: 5px 16px;
  cursor: pointer;
  color: inherit;
  font: inherit;
}
.setup-wrap .btn.big {
  padding: 8px 26px;
  font-size: 1.2rem;
}
.setup-wrap .btn.small {
  padding: 2px 10px;
  font-size: 0.85rem;
}
.setup-wrap .btn.plain {
  border-color: rgba(var(--w-base-color-rgb), 0.4);
}
.setup-wrap .btn[disabled] {
  opacity: 0.4;
  cursor: default;
}
.setup-wrap .hidden {
  display: none;
}
.setup-wrap .center {
  text-align: center;
}
.setup-wrap .radios {
  display: flex;
  flex-wrap: wrap;
  gap: 8px 20px;
  justify-content: center;
}
.setup-wrap .radio {
  display: inline-flex;
  align-items: center;
  gap: 7px;
  cursor: pointer;
}
.setup-wrap .radio input {
  accent-color: #04a3e5;
}
.is-default .radio input:checked {
  accent-color: gray;
  opacity: 0.4;
}
/* Wave hides a switch's checkbox with position: absolute and positions nothing
   around it, so on a page that scrolls the checkbox lands far from the switch
   and focusing it - which a click does - scrolls the page there. The Options
   drawer is a fixed overlay and never showed it. */
.setup-wrap .w-switch {
  position: relative;
}
.setup-wrap .disc {
  align-self: flex-start;
  display: inline-flex;
  align-items: center;
  gap: 8px;
  background: none;
  border: 0;
  padding: 4px 0;
  cursor: pointer;
  color: #04a3e5;
  font: inherit;
  font-size: 0.9rem;
}
.setup-wrap .disc .chev {
  width: 7px;
  height: 7px;
}
.setup-wrap .disc .tally {
  color: inherit;
  opacity: 0.6;
}
.setup-wrap .archives {
  display: flex;
  flex-direction: column;
  border-top: 1px solid rgba(var(--w-base-color-rgb), 0.2);
}
.setup-wrap .archive {
  display: flex;
  justify-content: space-between;
  gap: 12px;
  padding: 7px 2px;
  border-bottom: 1px solid rgba(var(--w-base-color-rgb), 0.2);
  font-size: 0.9rem;
}
.setup-wrap .archive.empty {
  opacity: 0.7;
}
.setup-wrap .archive .name {
  min-width: 0;
  overflow-wrap: anywhere;
}
.setup-wrap .archive .acts {
  display: inline-flex;
  gap: 12px;
  align-items: center;
  white-space: nowrap;
}
.setup-wrap .archive .act {
  background: none;
  border: 0;
  padding: 0;
  color: #04a3e5;
  cursor: pointer;
  font: inherit;
  font-size: 0.85rem;
}
.setup-wrap .archive .act.danger {
  color: #e0604f;
}

@media (max-width: 700px) {
  .setup-panel {
    grid-template-columns: minmax(0, 1fr);
    grid-template-rows: auto minmax(0, 1fr) auto;
    height: 560px;
  }
  .setup-nav {
    grid-row: auto;
    flex-direction: row;
    border-right: 0;
    border-bottom: 1px solid rgba(var(--w-base-color-rgb), 0.2);
    padding: 0;
  }
  .setup-nav a {
    border-left: 0;
    border-bottom: 2px solid transparent;
    white-space: nowrap;
    padding: 10px 14px;
  }
  .setup-nav a[aria-current="true"] {
    border-bottom-color: #04a3e5;
  }
  .setup-scroll {
    padding-inline: 16px;
  }
}
</style>

<style scoped>
.setup-wrap .field.emmc-field {
  display: grid;
  grid-template-columns: minmax(0, 1fr) max-content;
  column-gap: 24px;
  align-items: center;
}
.hostname-label { grid-column: 1; grid-row: 1; }
.emmc-heading { grid-column: 2; grid-row: 1; text-align: center; font-size: 0.85rem; opacity: 0.7; }
.emmc-field > .input, .emmc-field > .radios, .emmc-field > .setting-input { grid-column: 1; grid-row: 2; }
.emmc-buttons { grid-column: 2; grid-row: 2; display: flex; flex-direction: row; gap: 8px; align-items: center; }
.emmc-feedback { grid-column: 1 / -1; }

.archive-manager.setup-wrap, .archive-manager-foot.setup-wrap { margin-top: 0; }
.archive-manager { max-height: 65vh; overflow-y: auto; padding: 8px 16px; text-align: left; }
.archive-manager .field { margin-bottom: 20px; }
.archive-date { font-variant-numeric: tabular-nums; opacity: 0.7; }
</style>
