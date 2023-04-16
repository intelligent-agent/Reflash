<template>
  <w-select
    v-model="selected"
    :items="availableMethods"
    no-unselect
    return-object
    :input="onChange()">
  </w-select>
</template>

<script>
import { mapGetters, mapActions } from 'vuex';

export default {
  name: 'FlashSelector',
  computed: {
    ...mapGetters(['flash'])
  },
  data: () => ({
    availableMethods: [
      { id: 0, label: 'Install', value: 0 },
      { id: 1, label: 'Backup', value: 1 }
    ],
    selected: { id: 0, label: 'Install', value: 0 }
  }),

  methods: {
    ...mapActions(['setFlashMethod']),
    onChange(){
      this.setFlashMethod(this.selected.id);
    },
    setSelection(sel) {
      this.selected = this.availableMethods[sel];
    }
  }
}
</script>
