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
      if(filename){
        this.icon = "fa-spinner";
        this.visible = true;
        await axios.put(`/api/check_file_integrity`, {
          filename: filename
        }).then(response => {
          self.icon = response.data.is_file_ok ? "fa-check" : "fa-exclamation";
        });
      }
      else{
        this.icon = "";
        this.visible = false;
      }
    }
  }
}
</script>
