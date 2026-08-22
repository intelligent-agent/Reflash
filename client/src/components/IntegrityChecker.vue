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
    // The result is emitted as well as drawn, because the icon is not the only
    // thing that depends on it: the Install button has to refuse an image that
    // failed, and previously the answer lived only in this component's `icon`.
    // null means "no answer" - nothing selected, or a check still running.
    async fileSelected(filename){
      let self = this;
      if(filename && filename.length > 0){
        this.icon_visible = false;
        this.spinner_visible = true;
        this.$emit("integrity", null);
        await axios.put(`/api/check_file_integrity`, {
          filename: filename
        }).then(response => {
          const ok = response.data.is_file_ok;
          self.icon = ok ? "check" : "x";
          this.spinner_visible = false;
          this.icon_visible = true;
          this.$emit("integrity", ok);
        }).catch(() => {
          // An unanswered check is not a pass. Leaving it null keeps Install
          // disabled rather than letting a network blip enable it.
          this.spinner_visible = false;
          this.$emit("integrity", null);
        });
      }
      else{
        this.icon_visible = false;
        this.spinner_visible = false;
        this.$emit("integrity", null);
      }
    },
    computeSVG() {      
      return require("./../assets/" + this.icon + "-" + this.$waveui.theme + ".svg");
    }
  }
}
</script>
