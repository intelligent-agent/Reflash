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
        <w-divider class="my6 mx-3"></w-divider>
        <h4>Screen rotation</h4>
        <w-radios
          @change="onChange('rotateScreen', options.screenRotation)"
          v-model="options.screenRotation"
          :items="radioItems"
          inline
          label="Screen rotation"
          style="align-self: center"
        >
        </w-radios>
        <w-divider class="my6 mx-3"></w-divider>
        <h4>Reboot to eMMC</h4>
        <w-switch
          @change="changeBootMedia(options.bootFromEmmc)"
          v-model="options.bootFromEmmc"
          class="ma2"
          label="Set boot media to eMMC"
        >
        </w-switch>
        <w-divider class="my6 mx-3"></w-divider>
        <w-button xl outline class="ma2" @click="$emit('reboot-board')">
          <span>Reboot</span>
        </w-button>
        <w-button xl outline class="ma2" @click="$emit('shutdown-board')"
          ><span>Shut down</span></w-button
        >
      </w-flex>
    </w-drawer>
  </div>
</template>

<script>
import { mapGetters, mapActions } from "vuex";
import axios from "axios";

export default {
  name: "TheOptions",
  methods: {
    ...mapActions(["getOptions", "setOption"]),
    async onChange(name, value) {
      let data = {};
      data[name] = value;
      this.setOption(data);
      this.$emit("set-option", name, value);
      if (name == "rotateScreen") {
        const result = await axios.put(`/api/rotate_screen`, { rotation: value, where: "FBCON", restart_app: true });
        if (result.data.status != 0) {
          this.$waveui.notify(result.data.result, "error", 0);
        }
      }
    },
    async changeBootMedia(value) {
      const result = await axios.put(`/api/set_boot_media`, {
        media: value ? "emmc" : "usb",
      });
      if (result.data.status != 0) {
        this.$waveui.notify(result.data.result, "error", 0);
      }
    },
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
    radioItems: [
      { label: "Normal", value: 0 },
      { label: "90 degrees", value: 90 },
      { label: "180 degrees", value: 180 },
      { label: "270 degrees", value: 270 },
    ],
  }),
};
</script>
