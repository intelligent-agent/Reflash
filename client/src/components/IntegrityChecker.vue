<template>
  <w-icon md class="ma1" :class="this.spin" v-if="visible">fa {{icon}}</w-icon>
</template>

<script>
import axios from 'axios';
export default {
  name: 'IntegrityChecker',
  data: () => ({
    icon: "",
    visible: false
  }),
  computed: {
    spin() {
      return this.icon == "fa-spinner" ? "fa-spin" : "";
    }
  },
  methods: {
    async fileSelected(filename){
      let self = this;
      clearInterval(self.timer);
      if(filename){
        this.icon = "fa-spinner";
        this.visible = true;
        await axios.put(`/api/start_file_integrity_check`, {
          filename: filename
        }).then(() => {
          self.timer = setInterval(self.checkProgress, 100);
        });
      }
      else{
        this.icon = "";
        this.visible = false;
      }
    },
    async checkProgress(){
      let self = this;
      await axios.get(`/api/update_file_integrity_check`).then(response => {
        if(response.data.is_finished){
          clearInterval(self.timer);
          this.icon = response.data.is_file_ok ? "fa-check" : "fa-exclamation";
        }
      });
    }
  }
}
</script>
