<template>
  <span v-if="options.magicmode">Install</span>
  <w-select v-else v-model="selected" :items="availableMethods" no-unselect return-object />
</template>

<script>
import { mapGetters, mapActions } from 'vuex';

export default {
  name: 'FlashSelector',
  computed: {
    ...mapGetters(['flash', 'options']),
    selected: {
      get() {
        return this.availableMethods[this.options.magicmode ? 0 : this.flash.selectedMethod] || this.availableMethods[0];
      },
      set(method) {
        this.setFlashMethod(this.options.magicmode ? 0 : method.id);
      },
    },
  },
  data: () => ({
    availableMethods: [
      { id: 0, label: 'Install', value: 0 },
      { id: 1, label: 'Backup', value: 1 },
    ],
  }),
  methods: {
    ...mapActions(['setFlashMethod']),
    setSelection(index) {
      this.selected = this.availableMethods[index];
    },
  },
  watch: {
    'options.magicmode': {
      immediate: true,
      handler(magic) {
        if (magic && this.flash.selectedMethod !== 0) this.setFlashMethod(0);
      },
    },
  },
};
</script>
