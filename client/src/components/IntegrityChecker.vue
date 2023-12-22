<template>
  <div style="display: flex; align-items: center;">
    <w-spinner xs v-if="spinner" />
    <img v-if="visible" style="width: 20px" :src="computeSVG()" />
  </div>
</template>

<script>
import axios from 'axios';

export default {
  name: 'IntegrityChecker',
  data: () => ({
    icon: "x",
    visible: false,
    spinner: false
  }),  
  methods: {
    async fileSelected(filename){
      let self = this;
      if(filename && filename.length > 0){
        this.spinner = true;
        this.visible = true;
        await axios.put(`/api/check_file_integrity`, {
          filename: filename
        }).then(response => {
          self.icon = response.data.is_file_ok ? "check" : "x";
          this.spinner = false;  
        });
      }
      else{
        this.visible = false;
        this.spinner = false;
      }
    },
    computeSVG() {      
      return require("./../assets/" + this.icon + "-" + this.$waveui.theme + ".svg");
    }
  }
}
</script>
