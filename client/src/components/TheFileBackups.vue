<template>
  <w-dialog v-model="dialog.show" :width="dialog.width">
    <template #title>
      <span class="dialog_title">Backups of files</span>
    </template>
    <!-- #175: the files a reinstall would lose - the printer's configuration,
         Moonraker's database, OctoPrint's settings - as the installed system's
         own installer chooses them. Kept on the USB drive, downloadable, and
         put back into the next system installed. Not the whole-eMMC image
         backup: that is a separate thing. -->
    <p class="pa3">
      Back up the configuration of the system on the board, to keep it across
      a reinstall. Backups are kept on the USB drive, and one can be put back
      into the next system you install.
    </p>
    <div class="pa3">
      <w-button :disabled="busy" @click="makeBackup">Back up the installed system's files</w-button>
      <w-progress v-if="busy" class="ma1" circle></w-progress>
      <div v-if="message" class="ma2">{{ message }}</div>
    </div>
    <div class="pa3">
      <h4>On the USB drive</h4>
      <div v-if="!backups.length">None yet.</div>
      <div v-for="b in backups" :key="b.name" class="ma1">
        <w-radio
          :model-value="options.restoreBackup === b.name"
          @update:model-value="chooseRestore(b.name)"
          :label="b.name + ' (' + kib(b.size) + ')'"
        ></w-radio>
        <a :href="'/api/file_backups/download?name=' + encodeURIComponent(b.name)" class="ml2">Download</a>
      </div>
      <div v-if="options.restoreBackup" class="ma2">
        Goes into the next system you install.
        <w-button text @click="chooseRestore('')">Don't restore</w-button>
      </div>
    </div>
  </w-dialog>
</template>

<script>
import axios from "axios";
import { mapGetters, mapActions } from "vuex";

export default {
  name: "TheFileBackups",
  props: { open: Boolean },
  emits: ["close"],
  data: () => ({
    dialog: { show: false, width: 640 },
    backups: [],
    busy: false,
    message: "",
  }),
  computed: mapGetters(["options"]),
  methods: {
    ...mapActions(["setOption"]),
    kib(n) {
      return Math.max(1, Math.round(n / 1024)) + " KiB";
    },
    async list() {
      try {
        this.backups = (await axios.get(`/api/file_backups`)).data || [];
      } catch (err) {
        this.backups = [];
      }
    },
    async makeBackup() {
      this.busy = true;
      this.message = "";
      try {
        const res = await axios.post(`/api/file_backups`);
        this.message = res.data.status === "OK" ? "Saved as " + res.data.name : res.data.error;
      } catch (err) {
        this.message = err.response?.data || String(err);
      }
      this.busy = false;
      await this.list();
    },
    chooseRestore(name) {
      this.setOption({ restoreBackup: name });
    },
  },
  watch: {
    open: {
      immediate: true,
      handler(isOpen) {
        if (isOpen) {
          this.dialog.show = true;
          this.message = "";
          this.list();
        }
      },
    },
    "dialog.show"(shown) {
      if (!shown) this.$emit("close");
    },
  },
};
</script>
