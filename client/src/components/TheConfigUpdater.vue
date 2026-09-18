<template>
  <w-dialog v-model="dialog.show" :width="dialog.width">
    <template #title>
      <span class="dialog_title">Recore Serial Number</span>
    </template>
    <p v-if="serialNumberValid">
      The serial number for this Recore board is {{ serialNumber }}.
    </p>
    <p v-else>
      The serial number is missing from the board. Please provide the serial
      number written on the back of the Recore.
    </p>
    <div class="pa5">
      <img style="width: 20%" :src="computeSVG('Serial')" /><br />
      <w-progress v-if="updatePressed == true" class="ma1" circle></w-progress>
      <w-input style="width: 50%; margin: auto" v-model="inputSerialNumber" type="string"
        >Serial number</w-input
      >
    </div>
    <p v-if="this.isConfigPresent">Config has been updated</p>
    <!-- A way out that is not "click somewhere outside and hope". The dialog
         offered only Set Serial Number, so anyone who opened it to read the
         number had to guess that the backdrop dismisses it - and on the board's
         own touchscreen there is not much backdrop to aim at (#163). -->
    <w-button
      xl
      outline
      class="ma1 btn"
      @click="clickUpdateConfig()"
      ><span>Set Serial Number</span></w-button
    >
    <w-button xl outline class="ma1 btn" @click="dialog.show = false"
      ><span>Cancel</span></w-button
    >
  </w-dialog>
</template>
<script>
import axios from "axios";
import { mapGetters } from "vuex";

export default {
  name: "TheConfigUpdater",
  props: {
    open: Boolean,
    showOverlay: Boolean,
  },
  data: () => ({
    dialog: {
      show: false,
      width: "30%",
    },
    isConfigPresent: false,
    updatePressed: false,
    serialNumber: "",
    serialNumberValid: false,
    inputSerialNumber: "",
  }),
  computed: mapGetters(["options"]),
  methods: {
    computeSVG(name) {
      var color;
      if (this.$waveui.theme == "dark") {
        if (this.serialNumberValid) {
          color = "dark";
        } else {
          color = "light";
        }
      } else {
        if (this.isConfigPresent) {
          color = "light";
        } else {
          color = "dark";
        }
      }
      return require("./../assets/" + name + "-" + color + ".svg");
    },
    async getInfo() {
      var self = this;
      await axios.get(`/api/get_serial_number`).then(function (response) {
        self.serialNumber = response.data.serial_number;
        self.serialNumberValid = self.serialNumber != "";
      });
    },    
    async clickUpdateConfig() {
      var self = this;
      this.updatePressed = true;
      await axios
        .post(`/api/update_config`, { snr: parseInt(self.inputSerialNumber) })
        .then(function (response) {
          self.updatePressed = false;
          if (response.data.status == "OK") {
            self.getInfo();
          } else {
            self.$waveui.notify(response.data.error, "error", 0);
          }
        });
    },
  },
  watch: {
    open: {
      immediate: true,
      handler(is_open) {
        if (is_open) {
          this.dialog.show = true;
          this.getInfo();
        }
      },
    },
    // Tell the parent whenever the dialog goes away, however it went away -
    // Cancel or the backdrop. Without this its `openSerialNumber` stayed true
    // after a dismissal, so the watcher above never saw false -> true again and
    // the button simply stopped opening the dialog.
    "dialog.show"(is_shown) {
      if (!is_shown) {
        this.$emit("close");
      }
    },
  },
};
</script>

<style>
.dialog_title {
  margin: auto;
}
</style>