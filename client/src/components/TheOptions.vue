<template>
  <span>
    <w-button @click="openDrawer = true" text>
      <w-icon md>fa fa-cog</w-icon>
    </w-button>
    <w-drawer
      v-model="openDrawer"
      absolute
      width="30%">
      <w-flex class="pa5 secondary" column>
        <h3>Options</h3>
        <w-switch
          @change="onChange('darkmode', options.darkmode)"
          v-model="options.darkmode"
          class="ma2"
          label="Darkmode">
        </w-switch>
        <w-switch
          @change="onChange('rebootWhenDone', options.rebootWhenDone)"
          v-model="options.rebootWhenDone"
          class="ma2"
          label="Reboot when flashing is finished">
        </w-switch>
        <w-switch
        @change="onChange('enableSsh', options.enableSsh)"
        v-model="options.enableSsh"
        class="ma2"
        label="Enable SSH access on new image">
        </w-switch>
        <w-divider class="my6 mx-3"></w-divider>
        <h3>Reboot to eMMC</h3>
        <w-switch
          @change="onChange('bootFromEmmc', options.bootFromEmmc)"
          v-model="options.bootFromEmmc"
          class="ma2"
          label="Set boot media to eMMC">
        </w-switch>
        <w-divider class="my6 mx-3"></w-divider>
        <w-button
          class="ma2"
          @click="$emit('reboot-board')">
            Reboot
        </w-button>
        <w-button
          class="ma2"
          @click="$emit('shutdown-board')">
            Shut down
        </w-button>
       </w-flex>
    </w-drawer>
  </span>
</template>

<script>
import { mapGetters, mapActions } from 'vuex';

export default {
  name: 'TheOptions',
  methods: {
    ...mapActions(['getOptions', 'setOption']),
    onChange(name, value){
      let data = {};
      data[name] = value;
      this.setOption(data);
      this.$emit('set-option',name, value);
    }
  },
  data: () => ({
    openDrawer: false
  }),
  computed: mapGetters(['options']),
  created() {
    this.getOptions().then(() => {
      this.$emit('set-option','darkmode', this.options.darkmode);
    });
  }
}
</script>
