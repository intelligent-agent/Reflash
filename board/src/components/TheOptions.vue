<template>
  <div v-if="open">
    <w-drawer
      absolute
      width="30%"
      @close="this.$emit('close')">
      <w-flex class="pa5 secondary" column>
        <h3>Options</h3>
        <w-switch
          @change="onChange('darkmode', options.darkmode)"
          v-model="options.darkmode"
          class="ma2"
          label="Darkmode">
        </w-switch>
      </w-flex>
    </w-drawer>
  </div>
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
  computed: mapGetters(['options']),
  created() {
    this.getOptions().then(() => {
      this.$emit('set-option','darkmode', this.options.darkmode);
    });
  },
  props: {
    open: Boolean
  }
}
</script>
