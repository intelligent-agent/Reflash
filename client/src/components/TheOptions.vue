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
          @change="$emit('set-option','darkmode', darkmode)"
          v-model="darkmode"
          class="ma2"
          label="Darkmode">
        </w-switch>
        <w-switch
          @change="$emit('set-option', 'rebootWhenDone', rebootWhenDone)"
          v-model="rebootWhenDone"
          class="ma2"
          label="Reboot when flashing is finished">
        </w-switch>
        <w-switch
        @change="$emit('set-option','enableSsh', enableSsh)"
        v-model="enableSsh"
        class="ma2"
        label="Enable SSH access on new image">
        </w-switch>

        <w-divider class="my6 mx-3"></w-divider>
        <h3>Reboot to eMMC</h3>
        <w-switch
          @change="$emit('set-option','bootFromEmmc', bootFromEmmc)"
          v-model="bootFromEmmc"
          class="ma2"
          label="Set boot media to eMMC">
        </w-switch>
        <w-button
          @click="$emit('reboot-board')">
            Reboot now
        </w-button>
       </w-flex>
    </w-drawer>
  </span>
</template>

<script>
export default {
  name: 'TheOptions',
  props: ['settings'],
  data: () => ({
    darkmode: false,
    rebootWhenDone: false,
    enableSsh: false,
    bootFromEmmc: false,
    openDrawer: false
  })
}
</script>
