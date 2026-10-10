<template>
  <div v-if="open">
    <!-- Fixed to the window, not absolute: the panel outgrew the page, and an
         absolute drawer as tall as the page cut off what was below its edge
         with no way to reach it (#186). Its contents scroll instead. -->
    <w-drawer width="30%" @close="this.$emit('close')">
      <w-flex class="pa5 secondary options-panel" column>
        <h3>Options</h3>
        <w-switch
          @change="onChange('darkmode', options.darkmode)"
          v-model="options.darkmode"
          class="ma2"
          label="Darkmode"
        >
        </w-switch>
        <w-switch
          @change="onChange('rebootWhenDone', options.rebootWhenDone)"
          v-model="options.rebootWhenDone"
          class="ma2"
          label="Reboot when flashing is finished"
        >
        </w-switch>
        <div class="caption ma2">
          Wi-Fi, location, passwords, SSH and screen rotation are under Set up printer on the main page.
        </div>
        <w-divider class="my6 mx-3"></w-divider>
        <!-- What is not for everyone: the picker's pre-releases, which are
             for developers, and the board's serial number, which is printed
             on it and written once (#198). -->
        <details class="advanced" :open="serialMissing || undefined">
          <summary>Advanced</summary>
          <!-- The picker offers released images only; this puts the release
               candidates back. Filtered in the page from the releases it
               already has, so flicking this re-sorts a list rather than
               asking GitHub again (#172). -->
          <w-switch
            @change="onChange('showPrereleases', options.showPrereleases)"
            v-model="options.showPrereleases"
            class="ma2"
            label="Show pre-releases"
          >
          </w-switch>
          <div class="serial ma2">
            <label for="serial-number">Serial number</label>
            <div class="serial-row">
              <input
                id="serial-number"
                v-model="serialInput"
                :placeholder="serialMissing ? 'From the back of the board' : ''"
                inputmode="numeric"
                @keydown.enter="saveSerial()"
              />
              <w-button outline :disabled="!serialDirty || savingSerial" @click="saveSerial()">
                <span>{{ savingSerial ? "Saving..." : "Save" }}</span>
              </w-button>
            </div>
            <div class="caption">
              Printed on the board and written to the eMMC. Most people never change it.
            </div>
            <div class="caption good" v-if="serialMessage">{{ serialMessage }}</div>
            <div class="caption error" v-if="serialError">{{ serialError }}</div>
          </div>
        </details>
        <w-divider class="my6 mx-3"></w-divider>
        <h4>Actions</h4>
        <!-- Both ask twice. They sit side by side and fired on a single click,
             and the two outcomes are not equally cheap: a stray Shut down on a
             headless board needs someone to walk over and power-cycle it. Same
             two-click pattern as Delete, for the same reason - a native
             confirm() blocks the page (#163). -->
        <div>
          <w-button xl outline class="ma2" @click="confirmAction('reboot')">
            <span>{{ pending === "reboot" ? "Reboot?" : "Reboot now" }}</span>
          </w-button>
          <w-button xl outline class="ma2" @click="confirmAction('shutdown')"
            ><span>{{
              pending === "shutdown" ? "Shut down?" : "Shut down"
            }}</span></w-button
          >
        </div>
      </w-flex>
    </w-drawer>
  </div>
</template>

<script>
import axios from "axios";
import { mapGetters, mapActions } from "vuex";

export default {
  name: "TheOptions",
  methods: {
    ...mapActions(["getOptions", "setOption"]),
    async onChange(name, value) {
      let data = {};
      data[name] = value;
      this.setOption(data);
      this.$emit("set-option", name, value);
    },
    async loadSerial() {
      try {
        const res = await axios.get(`/api/get_serial_number`);
        this.serialSaved = String(res.data.serial_number || "");
        this.serialInput = this.serialSaved;
      } catch (err) {
        // The page still works without it.
      }
    },
    async saveSerial() {
      if (!this.serialDirty) return;
      this.serialError = "";
      this.serialMessage = "";
      this.savingSerial = true;
      try {
        const res = await axios.post(`/api/update_config`, { snr: parseInt(this.serialInput) });
        if (res.data.status === "OK") {
          this.serialSaved = this.serialInput;
          this.serialMessage = "Serial number saved.";
          this.$emit("serial-saved");
        } else {
          this.serialError = res.data.error || "The serial number could not be saved.";
        }
      } catch (err) {
        this.serialError = (err.response && err.response.data) || String(err);
      }
      this.savingSerial = false;
    },
    // First click arms the button, second one within a few seconds does it.
    // Arming one disarms the other, so a click meant for Reboot cannot confirm
    // a pending Shut down.
    confirmAction(which) {
      clearTimeout(this.pendingTimer);
      if (this.pending !== which) {
        this.pending = which;
        this.pendingTimer = setTimeout(() => (this.pending = null), 4000);
        return;
      }
      this.pending = null;
      this.$emit(which === "reboot" ? "reboot-board" : "shutdown-board");
    },
  },
  // A drawer that is closed and reopened must not still be holding an armed
  // button from minutes ago.
  beforeUnmount() {
    clearTimeout(this.pendingTimer);
  },
  computed: {
    ...mapGetters(["options"]),
    serialDirty() {
      return /^\d+$/.test(this.serialInput) && this.serialInput !== this.serialSaved;
    },
  },
  created() {
    this.loadSerial();
    this.getOptions().then(() => {
      this.$emit("set-option", "darkmode", this.options.darkmode);
    });
  },
  props: {
    open: Boolean,
    // The board has no serial number, so Advanced starts open.
    serialMissing: Boolean,
  },
  data: () => ({
    serialInput: "",
    serialSaved: "",
    savingSerial: false,
    serialMessage: "",
    serialError: "",
    // "reboot", "shutdown", or null when neither is armed.
    pending: null,
    pendingTimer: null,
    radioItems: [
      { label: "Normal", value: 0 },
      { label: "90 degrees", value: 90 },
      { label: "180 degrees", value: 180 },
      { label: "270 degrees", value: 270 },
    ],
  }),
};
</script>

<style>
.options-panel {
  height: 100%;
  overflow-y: auto;
}
.options-panel .advanced summary {
  cursor: pointer;
  text-align: center;
  opacity: 0.8;
  padding: 4px;
}
.options-panel .serial {
  text-align: left;
}
.options-panel .serial-row {
  display: flex;
  gap: 12px;
  align-items: flex-end;
}
.options-panel .serial-row input {
  flex: 1;
  min-width: 0;
  background: none;
  border: 0;
  border-bottom: 1px solid rgba(var(--w-base-color-rgb), 0.4);
  color: inherit;
  font: inherit;
  padding: 6px 2px;
}
.options-panel .good {
  color: #5cb85c;
}
</style>
