<template>
  <w-dialog v-model="dialog.show" :width="dialog.width">
    <template #title>
      <span class="dialog_title">Settings of the installed system</span>
    </template>
    <!-- #173: change the system already on the eMMC - a forgotten password,
         the wrong Wi-Fi, a screen the wrong way up - without reinstalling it.
         Read and written by the image's own installer; only what is changed
         here is sent, so everything else stays as it is. -->
    <w-progress v-if="loading" class="ma1" circle></w-progress>
    <div v-else-if="!info.supported" class="pa3">{{ info.reason }}</div>
    <div v-else class="pa3">
      <w-switch
        v-if="has('SSH_ENABLED')"
        v-model="form.ssh"
        class="ma2"
        label="SSH access at boot"
      ></w-switch>
      <div v-if="has('SCREEN_ROTATION')">
        <h4>Screen rotation</h4>
        <w-radios v-model="form.rotation" :items="rotations" inline></w-radios>
      </div>
      <div v-if="has('WIFI_SSID')">
        <h4>Wi-Fi</h4>
        <w-input v-model="form.ssid" class="ma2" label="Network name"></w-input>
        <w-input
          v-if="has('WIFI_PSK')"
          v-model="form.psk"
          type="password"
          class="ma2"
          label="Passphrase (empty: unchanged)"
        ></w-input>
      </div>
      <div v-if="has('LOGIN_PASSWORD')">
        <h4>Login password</h4>
        <w-input
          v-model="form.password"
          type="password"
          class="ma2"
          label="New password (empty: unchanged)"
        ></w-input>
        <w-input
          v-model="form.passwordAgain"
          type="password"
          class="ma2"
          label="Again"
        ></w-input>
        <div v-if="passwordsDiffer" class="error ma2">The two passwords differ.</div>
      </div>
      <div class="mt4">
        <w-button :disabled="busy || passwordsDiffer || !Object.keys(changes).length" @click="apply">
          Apply
        </w-button>
      </div>
      <div v-if="message" class="ma2">{{ message }}</div>
    </div>
  </w-dialog>
</template>

<script>
import axios from "axios";

export default {
  name: "TheInstalledSettings",
  props: { open: Boolean },
  emits: ["close"],
  data: () => ({
    dialog: { show: false, width: 600 },
    loading: false,
    busy: false,
    message: "",
    info: { supported: false, keys: [], current: {} },
    form: blankForm(),
    rotations: [
      { label: "Normal", value: "0" },
      { label: "90", value: "90" },
      { label: "180", value: "180" },
      { label: "270", value: "270" },
    ],
  }),
  computed: {
    passwordsDiffer() {
      return !!(this.form.password || this.form.passwordAgain) && this.form.password !== this.form.passwordAgain;
    },
    // Only what differs from what the system has now. Secrets are never read
    // back, so a typed one is always a change and an empty one never is.
    changes() {
      const c = {}, cur = this.info.current || {};
      if (this.has("SSH_ENABLED") && String(this.form.ssh) !== (cur.SSH_ENABLED || "")) {
        c.SSH_ENABLED = String(this.form.ssh);
      }
      if (this.has("SCREEN_ROTATION") && this.form.rotation !== (cur.SCREEN_ROTATION || "")) {
        c.SCREEN_ROTATION = this.form.rotation;
      }
      if (this.has("WIFI_SSID") && this.form.ssid !== (cur.WIFI_SSID || "")) {
        c.WIFI_SSID = this.form.ssid;
      }
      if (this.has("WIFI_PSK") && this.form.psk) c.WIFI_PSK = this.form.psk;
      if (this.has("LOGIN_PASSWORD") && this.form.password && !this.passwordsDiffer) {
        c.LOGIN_PASSWORD = this.form.password;
      }
      return c;
    },
  },
  methods: {
    has(key) {
      return (this.info.keys || []).includes(key);
    },
    async load() {
      this.loading = true;
      this.message = "";
      try {
        const res = await axios.get(`/api/installed_settings`);
        this.info = res.data;
        const cur = this.info.current || {};
        this.form = {
          ...blankForm(),
          ssh: cur.SSH_ENABLED === "true",
          rotation: cur.SCREEN_ROTATION || "",
          ssid: cur.WIFI_SSID || "",
        };
      } catch (err) {
        this.info = { supported: false, reason: err.response?.data || String(err), keys: [], current: {} };
      }
      this.loading = false;
    },
    async apply() {
      this.busy = true;
      try {
        const res = await axios.post(`/api/installed_settings`, this.changes);
        this.message =
          res.data.status === "OK"
            ? "Changed. The system uses the new settings from its next boot."
            : res.data.error;
        if (res.data.status === "OK") await this.load();
      } catch (err) {
        this.message = err.response?.data || String(err);
      }
      this.form.psk = "";
      this.form.password = "";
      this.form.passwordAgain = "";
      this.busy = false;
    },
  },
  watch: {
    open: {
      immediate: true,
      handler(isOpen) {
        if (isOpen) {
          this.dialog.show = true;
          this.load();
        }
      },
    },
    "dialog.show"(shown) {
      if (!shown) {
        // Typed secrets do not outlive the dialog.
        this.form = blankForm();
        this.$emit("close");
      }
    },
  },
};

function blankForm() {
  return { ssh: false, rotation: "", ssid: "", psk: "", password: "", passwordAgain: "" };
}
</script>
