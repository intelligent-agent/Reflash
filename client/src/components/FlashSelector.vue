<template>
  <w-select
    v-model="flash.selectedMethod"
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
      { id: 0, label: 'Flash', value: 0},
      { id: 1, label: 'Backup', value: 1}
    ]
  }),
  methods: {
    ...mapActions(['setFlashMethod']),
    onChange(){
      this.setFlashMethod(this.flash.selectedMethod);
    }
  }
}
</script>
