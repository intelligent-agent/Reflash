<template>
  <div style="display: flex; align-items: center;" class="ml2">
    <w-spinner sm bounce v-if="spinner_visible"  />
    <img v-if="icon_visible" style="width: 20px" :src="computeSVG()" />
  </div>
</template>

<script>
import axios from 'axios';

export default {
  name: 'IntegrityChecker',
  data: () => ({
    icon: "x",
    icon_visible: false,
    spinner_visible: false
  }),  
  methods: {
    async fileSelected(filename){
      let self = this;
      if(filename && filename.length > 0){
        this.icon_visible = false;
        this.spinner_visible = true;
        await axios.put(`/api/check_file_integrity`, {
          filename: filename
        }).then(response => {
          self.icon = response.data.is_file_ok ? "check" : "x";
          this.spinner_visible = false;
          this.icon_visible = true;
        });
      }
      else{
        this.icon_visible = false;
        this.spinner_visble = false;
      }
    },
    computeSVG() {      
      return require("./../assets/" + this.icon + "-" + this.$waveui.theme + ".svg");
    }
  }
}
</script>
