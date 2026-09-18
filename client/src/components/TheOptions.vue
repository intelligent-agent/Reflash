<template>
  <div v-if="open">
    <w-drawer absolute width="30%" @close="this.$emit('close')">
      <w-flex class="pa5 secondary" column>
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
        <w-switch
          @change="onChange('enableSsh', options.enableSsh)"
          v-model="options.enableSsh"
          class="ma2"
          label="Enable SSH access on new image"
        >
        </w-switch>
        <w-switch
          @change="onChange('magicmode', options.magicmode)"
          v-model="options.magicmode"
          class="ma2"
          label="Magicmode"
        >
        </w-switch>
        <w-divider class="my6 mx-3"></w-divider>
        <h4>Screen rotation</h4>
        <w-radios
          @change="onChange('screenRotation', options.screenRotation)"
          v-model="options.screenRotation"
          :items="radioItems"
          inline
          label="Screen rotation"
          style="align-self: center"
        >
        </w-radios>
        <w-divider class="my6 mx-3"></w-divider>
        <h4>Serial number</h4>
        <div>
          <w-button xl outline class="ma2" @click="$emit('open-serial-number')">
            <span>Set Serial Number</span>
          </w-button>
        </div>
        <w-divider class="my6 mx-3"></w-divider>
        <h4>Wi-Fi credentials</h4>
        <div>
          <w-button xl outline class="ma2" @click="$emit('open-wifi')">
            <span>Set Wi-Fi credentials</span>
          </w-button>
        </div>
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
  computed: mapGetters(["options"]),
  created() {
    this.getOptions().then(() => {
      this.$emit("set-option", "darkmode", this.options.darkmode);
    });
  },
  props: {
    open: Boolean,
  },
  data: () => ({
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
