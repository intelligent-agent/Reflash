<template>
  <div class="emmc-actions" role="group" :aria-label="`${label} on eMMC`">
    <span class="emmc-heading">eMMC</span>
    <div class="emmc-buttons">
      <button type="button" class="btn small" :disabled="!!busy || !canGet" @click="$emit('get')">{{ busy === 'get' ? 'Getting...' : 'Get' }}</button>
      <button type="button" class="btn small" :disabled="!!busy || !canSet" @click="$emit('set')">{{ busy === 'set' ? 'Setting...' : 'Set' }}</button>
    </div>
  </div>
</template>
<script>
export default {
  name: 'EmmcActions',
  props: { label: String, canGet: Boolean, canSet: Boolean, busy: String },
  emits: ['get', 'set'],
};
</script>
<style scoped>
.emmc-actions { display: contents; }
.emmc-heading { grid-column: 2; grid-row: 1; text-align: center; font-size: 0.85rem; opacity: 0.7; }
.emmc-buttons { grid-column: 2; grid-row: 2; display: flex; flex-direction: row; gap: 8px; align-items: center; }
</style>
