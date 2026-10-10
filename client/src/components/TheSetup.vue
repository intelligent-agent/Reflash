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
        <span>{{ countText }}</span>
        <button type="button" class="link" @click="resetAll()">Reset all to default</button>
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
          <div class="field" :class="{ 'is-default': isDef('wifiMode') }">
            <div class="top">
              <span class="lab">Wi-Fi mode</span>
              <label class="keep"><input type="checkbox" :checked="isDef('wifiMode')" @change="keep('wifiMode', $event.target.checked)" /> Default</label>
            </div>
            <div class="radios" role="radiogroup" aria-label="Wi-Fi mode">
              <label class="radio"><input type="radio" name="wifiMode" value="client" :checked="options.wifiMode === 'client'" @change="edit('wifiMode', { wifiMode: 'client' })" />Client</label>
              <label class="radio"><input type="radio" name="wifiMode" value="ap" :checked="options.wifiMode === 'ap'" @change="edit('wifiMode', { wifiMode: 'ap' })" />Access point</label>
            </div>
            <span class="help" v-if="effectiveMode === 'auto'"><b>Default:</b> join the network below if one is set. If it is not found within 2 minutes, start the hotspot until the next boot.</span>
            <span class="help" v-else-if="effectiveMode === 'client'"><b>Client:</b> only join the network below; never start the hotspot.</span>
            <span class="help" v-else><b>Access point:</b> always run the hotspot; the board does not join a network.</span>
          </div>

          <div v-show="effectiveMode !== 'ap'" class="field">
            <div class="top"><span class="lab">Network</span></div>
            <TheWifiSetup :open="expanded" inline />
            <span class="help"><b>Default:</b> no network set. The passphrase stays on the printer and never goes into a config archive.</span>
          </div>

          <div v-show="effectiveMode === 'ap'" class="field" :class="{ 'is-default': isDef('hotspot') }">
            <div class="top">
              <span class="lab">Hotspot</span>
              <label class="keep"><input type="checkbox" :checked="isDef('hotspot')" @change="keep('hotspot', $event.target.checked)" /> Default</label>
            </div>
            <label class="sub" for="hotspot-name">Name</label>
            <input id="hotspot-name" class="input" v-model="hotspotName" placeholder="Recore" @change="setHotspotName()" />
            <label class="sub" for="hotspot-psk">Password</label>
            <div class="row">
              <input id="hotspot-psk" class="input" :type="show.hotspot ? 'text' : 'password'" v-model="hotspotPsk" :placeholder="options.hotspotPSKSet ? '(set)' : '12345678'" autocomplete="off" @keydown.enter="setHotspotPassword()" />
              <button type="button" class="btn small plain" @click="show.hotspot = !show.hotspot">{{ show.hotspot ? "Hide" : "Show" }}</button>
              <button type="button" class="btn small" :disabled="!hotspotPsk" @click="setHotspotPassword()">Set</button>
            </div>
            <span class="error" v-if="problem.hotspot">{{ problem.hotspot }}</span>
            <span class="help"><b>Default:</b> name <code>Recore</code>, password <code>12345678</code>, address 192.168.50.1.</span>
          </div>
        </section>

        <section id="setup-location" data-title="Location">
          <h4>Location</h4>
          <div class="field" :class="{ 'is-default': isDef('country') }">
            <div class="top">
              <label class="lab" for="country">Country</label>
              <label class="keep"><input type="checkbox" :checked="isDef('country')" @change="keep('country', $event.target.checked)" /> Default</label>
            </div>
            <select id="country" class="input" :value="options.wifiCountry || ''" @change="edit('country', { wifiCountry: $event.target.value })">
              <option value="" disabled>Choose a country</option>
              <option v-for="c in countries" :key="c[0]" :value="c[0]">{{ c[1] }} ({{ c[0] }})</option>
            </select>
            <span class="help">Sets the Wi-Fi regulatory domain: which channels and power the radio may use. <b>Default:</b> none set.</span>
          </div>
          <div class="field" :class="{ 'is-default': isDef('timezone') }">
            <div class="top">
              <label class="lab" for="timezone">Timezone</label>
              <label class="keep"><input type="checkbox" :checked="isDef('timezone')" @change="keep('timezone', $event.target.checked)" /> Default</label>
            </div>
            <select id="timezone" class="input" :value="options.timezone || ''" @change="edit('timezone', { timezone: $event.target.value })">
              <option value="" disabled>Choose a timezone</option>
              <option v-for="z in zones" :key="z" :value="z">{{ z }}</option>
            </select>
            <span class="help"><b>Default:</b> UTC.</span>
          </div>
        </section>

        <section id="setup-accounts" data-title="Accounts">
          <h4>Accounts</h4>
          <div class="field" :class="{ 'is-default': isDef('sshPassword') }">
            <div class="top">
              <label class="lab" for="ssh-password">SSH password (user debian)</label>
              <label class="keep"><input type="checkbox" :checked="isDef('sshPassword')" @change="keep('sshPassword', $event.target.checked)" /> Default</label>
            </div>
            <div class="row">
              <input id="ssh-password" class="input" :type="show.ssh ? 'text' : 'password'" v-model="sshPw" :placeholder="options.loginPasswordSet ? '(set)' : 'At least 6 characters'" autocomplete="off" @keydown.enter="setPassword('sshPassword')" />
              <button type="button" class="btn small plain" @click="show.ssh = !show.ssh">{{ show.ssh ? "Hide" : "Show" }}</button>
              <button type="button" class="btn small" :disabled="!sshPw" @click="setPassword('sshPassword')">Set</button>
            </div>
            <span class="error" v-if="problem.sshPassword">{{ problem.sshPassword }}</span>
            <span class="help"><b>Default:</b> <code>temppwd</code>, which must be changed at first login.</span>
          </div>
          <div class="field" :class="{ 'is-default': isDef('rootPassword') }">
            <div class="top">
              <label class="lab" for="root-password">Root password</label>
              <label class="keep"><input type="checkbox" :checked="isDef('rootPassword')" @change="keep('rootPassword', $event.target.checked)" /> Default</label>
            </div>
            <div class="row">
              <input id="root-password" class="input" :type="show.root ? 'text' : 'password'" v-model="rootPw" :placeholder="options.rootPasswordSet ? '(set)' : 'At least 6 characters'" autocomplete="off" @keydown.enter="setPassword('rootPassword')" />
              <button type="button" class="btn small plain" @click="show.root = !show.root">{{ show.root ? "Hide" : "Show" }}</button>
              <button type="button" class="btn small" :disabled="!rootPw" @click="setPassword('rootPassword')">Set</button>
            </div>
            <span class="error" v-if="problem.rootPassword">{{ problem.rootPassword }}</span>
            <span class="help">For the serial console; root cannot log in over SSH with a password. <b>Default:</b> <code>temppwd</code>.</span>
          </div>
          <div class="field" :class="{ 'is-default': isDef('ssh') }">
            <div class="top">
              <span class="lab">SSH access</span>
              <label class="keep"><input type="checkbox" :checked="isDef('ssh')" @change="keep('ssh', $event.target.checked)" /> Default</label>
            </div>
            <label class="switch"><input type="checkbox" :checked="options.enableSsh" @change="edit('ssh', { enableSsh: $event.target.checked })" /><span class="track"></span><span>Enable SSH access</span></label>
            <span class="help">Turns itself off after a while if the password is still the default. <b>Default:</b> off.</span>
          </div>
        </section>

        <section id="setup-display" data-title="Display">
          <h4>Display</h4>
          <div class="field" :class="{ 'is-default': isDef('rotation') }">
            <div class="top">
              <span class="lab">Screen rotation</span>
              <label class="keep"><input type="checkbox" :checked="isDef('rotation')" @change="keep('rotation', $event.target.checked)" /> Default</label>
            </div>
            <div class="radios" role="radiogroup" aria-label="Screen rotation">
              <label v-for="r in rotations" :key="r.value" class="radio"><input type="radio" name="rotation" :value="r.value" :checked="options.screenRotation === r.value" @change="edit('rotation', { screenRotation: r.value })" />{{ r.label }}</label>
            </div>
            <span class="help"><b>Default:</b> normal.</span>
          </div>
        </section>

        <section id="setup-config" data-title="Config archive">
          <h4>Config archive</h4>

          <div class="field">
            <div class="top"><label class="lab" for="save-name">Save this printer's config</label></div>
            <div class="row">
              <input id="save-name" class="input" v-model="saveName" placeholder="Name, for example voron-20261010" @keydown.enter="save()" />
              <button type="button" class="btn small" :disabled="busy.save" @click="save()">{{ busy.save ? "Saving..." : "Save" }}</button>
            </div>
            <button type="button" class="disc" :aria-expanded="String(tree.save.open)" aria-controls="tree-save" @click="toggleTree('save')">
              <span class="chev" aria-hidden="true"></span>Choose files <span class="tally">{{ tally('save') }}</span>
            </button>
            <div id="tree-save" v-if="tree.save.open">
              <p v-if="tree.save.reason" class="help">{{ tree.save.reason }} The whole config is saved.</p>
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

          <div class="field" :class="{ 'is-default': isDef('config') }">
            <div class="top">
              <label class="lab" for="config-install">Config to install</label>
              <label class="keep"><input type="checkbox" :checked="isDef('config')" @change="keep('config', $event.target.checked)" /> Default</label>
            </div>
            <select id="config-install" class="input" :value="options.restoreBackup || ''" @change="chooseConfig($event.target.value)">
              <option value="" disabled>Choose an archive</option>
              <option v-for="a in archives" :key="a.name" :value="a.name">{{ shortName(a.name) }}</option>
            </select>
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
            <span class="help">Goes in after the image, or on its own into the system that is installed now. Only the files ticked are written; the rest of the config stays as it is. <b>Default:</b> the image's own config.</span>
          </div>
        </section>

        <section v-if="software.length" id="setup-software" data-title="Optional software">
          <h4>Optional software</h4>
          <div v-for="s in software" :key="s.name" class="field">
            <div class="top"><span class="lab">{{ title(s.name) }}</span></div>
            <label class="switch"><input type="checkbox" :checked="softwareOn[s.name]" @change="toggleSoftware(s.name, $event.target.checked)" /><span class="track"></span><span>Install <code>{{ s.name }}</code></span></label>
            <span class="help">{{ s.info }}. Like the other settings, it reaches the installed system at once and goes onto the next image after a flash. Off by default.</span>
            <span class="help">Installed on this printer now: <b>{{ s.installed ? "yes" : "no" }}</b></span>
          </div>
          <p class="sub center">More components can be added to this list later.</p>
        </section>
      </div>

      <div class="setup-foot"><button type="button" class="btn big" @click="expanded = false">Done</button></div>
    </div>
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
  data: () => ({
    expanded: false,
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
    effectiveMode() {
      if (this.isDef("wifiMode")) return "auto";
      return this.options.wifiMode || "client";
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
      return { text: "Saved to the installed system", error: false };
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
      this.hotspotName = o.hotspotSSID || "";
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
    async setHotspotName() {
      if (!this.hotspotName) return;
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
    async listArchives() {
      try {
        this.archives = (await axios.get(`/api/file_backups`)).data || [];
      } catch (err) {
        this.archives = [];
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
      this.custom = { ...this.custom, config: true };
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
      this.problem = { ...this.problem, install: "" };
      this.message = { ...this.message, install: "" };
      const include = this.includeOf("install");
      if (include === null) {
        this.problem = { ...this.problem, install: "No files are chosen." };
        return;
      }
      this.busy.install = true;
      try {
        const res = await axios.post(`/api/file_backups/restore`, { name: this.options.restoreBackup, include });
        if (res.data.status === "OK") this.message = { ...this.message, install: `Installed ${shortName(this.options.restoreBackup)}.` };
        else this.problem = { ...this.problem, install: res.data.error || "Installing the config failed" };
      } catch (err) {
        this.problem = { ...this.problem, install: this.errorOf(err) };
      }
      this.busy.install = false;
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
.setup-wrap .switch {
  display: inline-flex;
  align-items: center;
  gap: 10px;
  cursor: pointer;
}
.setup-wrap .switch input {
  position: absolute;
  opacity: 0;
}
.setup-wrap .switch .track {
  width: 38px;
  height: 20px;
  border-radius: 99px;
  background: rgba(var(--w-base-color-rgb), 0.3);
  position: relative;
  flex: none;
  transition: background 0.15s;
}
.setup-wrap .switch .track::after {
  content: "";
  position: absolute;
  top: 2px;
  left: 2px;
  width: 16px;
  height: 16px;
  border-radius: 50%;
  background: var(--w-base-bg-color-rgb);
  transition: left 0.15s;
}
.setup-wrap .switch input:checked + .track {
  background: #04a3e5;
}
.setup-wrap .switch input:checked + .track::after {
  left: 20px;
}
.setup-wrap .switch input:focus-visible + .track {
  outline: 1px solid #04a3e5;
  outline-offset: 2px;
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
