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
          @change="onChange('rotateScreen', options.screenRotation)"
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
        <div>
          <w-button xl outline class="ma2" @click="$emit('reboot-board')">
            <span>Reboot now</span>
          </w-button>
          <w-button xl outline class="ma2" @click="$emit('shutdown-board')"
            ><span>Shut down</span></w-button
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
