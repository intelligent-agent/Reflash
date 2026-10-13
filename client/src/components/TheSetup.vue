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
          <div class="field">
            <div class="top">
              <label class="lab" for="hostname">Hostname</label>
            </div>
            <input id="hostname" class="input" v-model="hostnameDraft" maxlength="63" @change="setHostname(hostnameDraft)" />
            <div class="row">
              <button type="button" class="btn small" :disabled="hostnameBusy || !hostnameCanRead" @click="getInstalledHostname()">{{ hostnameBusy === 'get' ? 'Getting...' : 'Get' }}</button>
              <button type="button" class="btn small" :disabled="hostnameBusy || !hostnameCanSet" @click="applyHostname()">{{ hostnameBusy === 'set' ? 'Setting...' : 'Set' }}</button>
              <span class="good" v-if="hostnameMessage">{{ hostnameMessage }}</span>
            </div>
            <span class="help" v-if="hostnameReason">{{ hostnameReason }}</span>
            <span class="error" v-if="problem.hostname">{{ problem.hostname }}</span>
            <span class="help">Get reads the currently installed image. Set applies to that image for its next boot. Edits are retained for the next flash. Open <code>{{ options.hostname || 'recore' }}.local</code>.</span>
          </div>
          <div v-if="wifiPresent" class="wifi-settings">
          <div class="field">
            <div class="top">
              <span class="lab">Wi-Fi mode</span>
            </div>
            <div class="radios" role="radiogroup" aria-label="Wi-Fi mode">
              <label v-for="m in wifiModes" :key="m.value" class="radio"><input type="radio" name="wifiMode" :value="m.value" :checked="(options.wifiMode || '') === m.value" @change="chooseMode(m.value)" />{{ m.label }}</label>
            </div>
            <span class="help" v-if="effectiveMode === 'auto'"><b>Automatic:</b> connect to the client network. If it is not found within 2 minutes, start the fallback hotspot until the next boot. Configure both sets of credentials below; the hotspot can use the displayed defaults.</span>
            <span class="help" v-else-if="effectiveMode === 'client'"><b>Client only:</b> only join the network below; never start the hotspot.</span>
            <span class="help" v-else><b>Access point only:</b> always run the hotspot; the board does not join a network.</span>
          </div>

          <div v-show="effectiveMode !== 'ap'" class="field">
            <div class="top"><span class="lab">Client network — router SSID and password</span></div>
            <TheWifiSetup :open="expanded" inline />
            <span class="help"><b>When unchanged:</b> no network set. The passphrase stays on the printer and never goes into a config archive.</span>
          </div>

          <div v-show="effectiveMode !== 'client'" class="field">
            <div class="top">
              <span class="lab">{{ effectiveMode === 'auto' ? "Fallback hotspot — printer AP name and password" : "Hotspot — printer AP name and password" }}</span>
            </div>
            <label class="sub" for="hotspot-name">Name</label>
            <input id="hotspot-name" class="input" v-model="hotspotName" placeholder="Recore" @change="setHotspotName()" />
            <label class="sub" for="hotspot-psk">Password</label>
            <div class="row">
              <input id="hotspot-psk" class="input" :type="show.hotspot ? 'text' : 'password'" v-model="hotspotPsk" :placeholder="options.hotspotPSKSet ? 'Password configured — leave blank to keep' : '12345678'" autocomplete="off" @keydown.enter="setHotspotPassword()" />
              <button type="button" class="btn small plain" @click="show.hotspot = !show.hotspot">{{ show.hotspot ? "Hide" : "Show" }}</button>
              <button type="button" class="btn small" :disabled="!hotspotPsk" @click="setHotspotPassword()">Set</button>
            </div>
            <span class="error" v-if="problem.hotspot">{{ problem.hotspot }}</span>
            <span class="help"><b>When unchanged:</b> name <code>Recore</code>, password <code>12345678</code>, address 192.168.50.1.</span>
          </div>
          </div>
          <div v-if="wifiPresent !== true" class="field wifi-check">
            <span class="help">{{ wifiPresent === false ? 'No Wi-Fi adapter detected. You can connect an adapter and check again.' : 'Checking for a Wi-Fi adapter...' }}</span>
            <button type="button" class="btn small" :disabled="wifiChecking" @click="checkWifi()">{{ wifiChecking ? 'Checking...' : 'Check again' }}</button>
            <span class="help" v-if="wifiCheckError">{{ wifiCheckError }}</span>
          </div>
        </section>

        <section id="setup-location" data-title="Location">
          <h4>Location</h4>
          <div class="field">
            <div class="top">
              <label class="lab" for="country">Country</label>
            </div>
            <select id="country" class="input" :value="options.wifiCountry || ''" @change="edit('country', { wifiCountry: $event.target.value })">
              <option value="">Not specified</option>
              <option v-for="c in countries" :key="c[0]" :value="c[0]">{{ c[1] }} ({{ c[0] }})</option>
            </select>
            <span class="help">Sets the Wi-Fi regulatory domain: which channels and power the radio may use. <b>When unchanged:</b> none set.</span>
          </div>
          <div class="field">
            <div class="top">
              <label class="lab" for="timezone">Timezone</label>
            </div>
            <select id="timezone" class="input" :value="options.timezone || 'UTC'" @change="edit('timezone', { timezone: $event.target.value })">

              <option v-for="z in zones" :key="z" :value="z">{{ z }}</option>
            </select>
            <span class="help">Uses the selected timezone.</span>
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
            <span class="help">Leave blank to keep the installed image’s existing password.</span>
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
            <span class="help">For the serial console; root cannot log in over SSH with a password. Leave blank to keep the existing password.</span>
          </div>
          <div class="field">
            <div class="top">
              <span class="lab">SSH access</span>
            </div>
            <w-switch :model-value="!!options.enableSsh" @update:model-value="edit('ssh', { enableSsh: $event })" label="Enable SSH access"></w-switch>
            <span class="help">Turns itself off after a while if the password is still the default. <b>When unchanged:</b> off.</span>
          </div>
        </section>

        <section id="setup-display" data-title="Display">
          <h4>Display</h4>
          <div class="field">
            <div class="top">
              <span class="lab">Screen rotation</span>
            </div>
            <div class="radios" role="radiogroup" aria-label="Screen rotation">
              <label v-for="r in rotations" :key="r.value" class="radio"><input type="radio" name="rotation" :value="r.value" :checked="options.screenRotation === r.value" @change="edit('rotation', { screenRotation: r.value })" />{{ r.label }}</label>
            </div>
            <span class="help"><b>When unchanged:</b> normal.</span>
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
              <input id="save-name" class="input" v-model="saveName" placeholder="Name, for example voron-20261010" @keydown.enter="save()" />
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
              <button type="button" class="btn small" :disabled="!options.restoreBackup || busy.install" @click="installNow()">{{ busy.install ? "Installing..." : "Install config now" }}</button>
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

const shortName = (name) => String(name || "").replace(/\.tar\.gz$/, "");

export default {
  name: "TheSetup",
  components: { TheWifiSetup, FileTree },
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
    wifiPresent: null,
    wifiChecking: false,
    wifiCheckError: "",
    current: "setup-network",
    // Options left alone but shown as set: ticking Default again is a choice.
    custom: {},
    seeded: false,
    show: { ssh: false, root: false, hotspot: false },
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
      return this.options.wifiMode || "auto";
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
      this.hostnameDraft = o.hostname || "recore";
      this.hotspotName = o.hotspotSSID || "Recore";
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
      if (root) this.rootPw = "";
      else this.sshPw = "";
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
    async readHostnameSupport() {
      const { data } = await axios.get('/api/installed_settings', { timeout: 15000 });
      this.hostnameCanRead = typeof data.current?.HOSTNAME === 'string';
      this.hostnameCanSet = !!data.supported && (data.keys || []).includes('HOSTNAME');
      this.hostnameReason = data.reason || (!this.hostnameCanRead && !this.hostnameCanSet ? 'This installed image does not expose hostname settings.' : '');
      return data.current?.HOSTNAME;
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
        const { data } = await axios.post('/api/installed_settings', { HOSTNAME: this.hostnameDraft });
        if (data.status !== 'OK') throw new Error(data.error || data.reason || 'The installed image refused the hostname.');
        this.hostnameMessage = `Set ${this.hostnameDraft} on the installed image; takes effect at its next boot.`;
      } catch (err) { this.problem = { ...this.problem, hostname: String(this.errorOf(err)) }; }
      finally { this.hostnameBusy = ''; }
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
      this.hotspotPsk = "";
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
        const res = (await axios.get(url)).data;
        t.files = res.supported ? res.files : [];
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
      return includeFor(t.files, t.selected);
    },
    async save() {
      this.problem = { ...this.problem, save: "" };
      const include = this.includeOf("save");
      if (include === null) {
        this.problem = { ...this.problem, save: "No files are chosen." };
        return;
      }
      this.busy.save = true;
      try {
        const res = await axios.post(`/api/file_backups`, { name: this.saveName, include });
        if (res.data.status === "OK") {
          this.notify(`Saved ${res.data.name} on the USB drive`);
          this.saveName = "";
        } else {
          this.problem = { ...this.problem, save: res.data.error || "The backup failed" };
        }
      } catch (err) {
        this.problem = { ...this.problem, save: this.errorOf(err) };
      }
      this.busy.save = false;
      this.listArchives();
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
.archive-manager.setup-wrap, .archive-manager-foot.setup-wrap { margin-top: 0; }
.archive-manager { max-height: 65vh; overflow-y: auto; padding: 8px 16px; text-align: left; }
.archive-manager .field { margin-bottom: 20px; }
</style>
