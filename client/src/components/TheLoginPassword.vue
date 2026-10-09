<template>
  <w-dialog v-model="dialog.show" :width="dialog.width">
    <template #title>
      <span class="dialog_title">SSH password</span>
    </template>
    <!-- For the installed system, not for Reflash (#182, #186). Sent to the
         server and nowhere else: it is not kept in the page's store, and the
         server holds it in memory only and never sends it back - this dialog
         learns just whether one is set. -->
    <p>
      The password for the user <b>debian</b>, for logging in and over SSH. It
      goes onto the installed system now and onto the one you install next.
      The default is <b>temppwd</b>.
    </p>
    <div class="pa5">
      <img style="width: 20%" :src="computeSVG('Password')" /><br />
      <w-input
        style="width: 60%; margin: auto"
        v-model="password"
        type="password"
        label="New password"
      ></w-input>
      <w-input
        class="mt4"
        style="width: 60%; margin: auto"
        v-model="passwordAgain"
        type="password"
        label="Again"
      ></w-input>
      <div v-if="passwordsDiffer" class="error mt2">The two passwords differ.</div>
      <div class="mt4">
        <w-button
          class="mr2"
          :disabled="!password || password !== passwordAgain"
          @click="setPassword(password)"
        >
          Set password
        </w-button>
        <w-button @click="setPassword(defaultPassword)">
          Set default
        </w-button>
      </div>
      <div v-if="message" class="mt4" :class="messageOk ? 'success' : 'error'">
        {{ message }}
      </div>
      <div v-else class="caption mt4">
        {{ options.loginPasswordSet ? "Set for the user debian." : "The user debian has the default password, temppwd." }}
      </div>
    </div>
  </w-dialog>
</template>

<script>
import axios from "axios";
import { mapGetters, mapActions } from "vuex";

export default {
  name: "TheLoginPassword",
  props: { open: Boolean },
  emits: ["close"],
  data: () => ({
    dialog: { show: false, width: 560 },
    password: "",
    passwordAgain: "",
    message: "",
    messageOk: true,
    // What "Set default" sets: the factory password, now not expired.
    defaultPassword: "temppwd",
  }),
  computed: {
    ...mapGetters(["options"]),
    passwordsDiffer() {
      return !!(this.password && this.passwordAgain) && this.password !== this.passwordAgain;
    },
  },
  methods: {
    ...mapActions(["getOptions"]),
    computeSVG(name) {
      return require("../assets/" + name + "-" + this.$waveui.theme + ".svg");
    },
    // Straight to the server, not through setOption: that would keep the
    // password in the page's store. Rereading the options is how the page
    // learns the one thing it may know - whether a password is set.
    // The window stays open and says how it went.
    async setPassword(value) {
      this.password = "";
      this.passwordAgain = "";
      this.message = "";
      try {
        const res = await axios.post(`/api/set_options`, { loginPassword: value });
        if (res.data && res.data.status === "ERROR") throw new Error(res.data.error);
        // The installed system is told in the background; say how it went,
        // not that it was sent - it may refuse a password its rules reject.
        await this.waitForInstalledSystem();
        if (this.options.settingsSyncError) {
          this.messageOk = false;
          this.message = "The system did not take it: " + this.options.settingsSyncError;
        } else {
          this.messageOk = true;
          this.message =
            value === this.defaultPassword
              ? "The password for debian is the default again: " + this.defaultPassword + "."
              : "Password set for debian.";
        }
      } catch (err) {
        this.messageOk = false;
        this.message = "Could not set the password: " + (err.response?.data || err.message || err);
      }
    },
    // Until the installed system has answered, a minute at most.
    async waitForInstalledSystem() {
      for (let i = 0; i < 60; i++) {
        await this.getOptions();
        if (!this.options.settingsSyncBusy) return;
        await new Promise((r) => setTimeout(r, 1000));
      }
    },
  },
  watch: {
    open: {
      immediate: true,
      handler(isOpen) {
        if (isOpen) {
          this.message = "";
          this.dialog.show = true;
        }
      },
    },
    "dialog.show"(shown) {
      if (!shown) {
        // Typed passwords do not outlive the dialog.
        this.password = "";
        this.passwordAgain = "";
        this.$emit("close");
      }
    },
  },
};
</script>
