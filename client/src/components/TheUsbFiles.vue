<template>
  <w-dialog v-model="dialog.show" :width="dialog.width">
    <template #title>
      <span class="dialog_title">Files on the USB drive</span>
    </template>
    <!-- #188: everything on the drive in one place - images and config
         archives, with their size, date and whether they are intact - and
         where they are deleted, instead of a Delete squeezed in next to the
         image on the main screen. -->
    <div class="pa3" spellcheck="false">
      <w-progress v-if="loading" class="ma1" circle></w-progress>
      <template v-else>
        <div v-for="section in sections" :key="section.kind" class="mb4">
          <h4 class="text-left mb1">{{ section.title }}</h4>
          <div v-if="!section.files.length" class="text-left caption">None on the drive.</div>
          <table v-else class="usb-files">
            <thead>
              <tr>
                <th class="text-left">Name</th>
                <th class="text-right">Size</th>
                <th>Date</th>
                <th>Intact</th>
                <th></th>
              </tr>
            </thead>
            <tbody>
              <tr v-for="f in section.files" :key="f.kind + f.name">
                <td class="text-left name">{{ f.name }}</td>
                <td class="text-right">{{ size(f.size) }}</td>
                <td>{{ date(f.date) }}</td>
                <td>
                  <w-spinner v-if="f.ok === null" xs bounce />
                  <img v-else style="width: 18px" :src="computeSVG(f.ok ? 'check' : 'x')"
                       :title="f.ok ? 'Intact' : 'Damaged or incomplete'" />
                </td>
                <td>
                  <w-button text :disabled="busy" @click="remove(f)">
                    {{ confirm === f.kind + f.name ? "Delete?" : "Delete" }}
                  </w-button>
                </td>
              </tr>
            </tbody>
          </table>
        </div>
      </template>
      <div v-if="message" class="error mt2">{{ message }}</div>
    </div>
  </w-dialog>
</template>

<script>
import axios from "axios";

export default {
  name: "TheUsbFiles",
  props: { open: Boolean },
  emits: ["close", "changed"],
  data: () => ({
    dialog: { show: false, width: 760 },
    files: [],
    loading: false,
    busy: false,
    confirm: "",
    confirmTimer: null,
    message: "",
  }),
  computed: {
    // Images first, then config archives, each newest first.
    sections() {
      return [
        { kind: "Image", title: "Images", files: this.files.filter((f) => f.kind == "Image") },
        { kind: "Config", title: "Config archives", files: this.files.filter((f) => f.kind == "Config") },
      ];
    },
  },
  methods: {
    computeSVG(name) {
      return require("../assets/" + name + "-" + this.$waveui.theme + ".svg");
    },
    size(bytes) {
      if (bytes >= 1 << 30) return (bytes / (1 << 30)).toFixed(1) + " GiB";
      if (bytes >= 1 << 20) return (bytes / (1 << 20)).toFixed(0) + " MiB";
      return Math.max(1, Math.round(bytes / 1024)) + " KiB";
    },
    date(seconds) {
      if (!seconds) return "";
      const d = new Date(seconds * 1000);
      const pad = (n) => String(n).padStart(2, "0");
      return `${d.getFullYear()}-${pad(d.getMonth() + 1)}-${pad(d.getDate())} ${pad(d.getHours())}:${pad(d.getMinutes())}`;
    },
    async load() {
      this.loading = true;
      this.message = "";
      let images = [], configs = [];
      try {
        images = (await axios.get(`/api/get_status`)).data.local_images || [];
      } catch (err) {
        this.message = "Could not list the images: " + err;
      }
      try {
        configs = (await axios.get(`/api/file_backups`)).data || [];
      } catch (err) {
        this.message = "Could not list the config archives: " + err;
      }
      // Newest first; sections() splits them by kind.
      this.files = images
        .map((i) => ({ kind: "Image", name: i.name, size: i.size, date: i.date, ok: null }))
        .concat(configs.map((c) => ({ kind: "Config", name: c.name, size: c.size, date: c.date, ok: c.ok })))
        .sort((a, b) => (b.date || 0) - (a.date || 0));
      this.loading = false;
      // xz -l reads only an image's index, so checking them all is quick;
      // one at a time, so the drive is not read by several at once.
      for (const f of this.files) {
        if (f.kind != "Image") continue;
        try {
          const res = await axios.put(`/api/check_file_integrity`, { filename: f.name });
          f.ok = !!res.data.is_file_ok;
        } catch (err) {
          f.ok = false;
        }
      }
    },
    // Twice to delete, as on the main screen before (#153): a native confirm
    // would block the page, progress polling included.
    async remove(f) {
      const key = f.kind + f.name;
      clearTimeout(this.confirmTimer);
      if (this.confirm !== key) {
        this.confirm = key;
        this.confirmTimer = setTimeout(() => (this.confirm = ""), 4000);
        return;
      }
      this.confirm = "";
      this.busy = true;
      try {
        const res = await axios.put(`/api/delete_image`, { filename: f.name });
        // A refusal can come back as a 200 with status ERROR - "busy: UPLOADING".
        if (res.data && res.data.status == "ERROR") throw new Error(res.data.error);
        this.files = this.files.filter((x) => x !== f);
        this.$emit("changed");
      } catch (err) {
        this.message = "Could not delete " + f.name + ": " + (err.response?.data || err.message || err);
      }
      this.busy = false;
    },
  },
  watch: {
    open: {
      immediate: true,
      handler(isOpen) {
        if (isOpen) {
          this.dialog.show = true;
          this.load();
        }
      },
    },
    "dialog.show"(shown) {
      if (!shown) {
        this.confirm = "";
        this.$emit("close");
      }
    },
  },
  beforeUnmount() {
    clearTimeout(this.confirmTimer);
  },
};
</script>

<style>
.usb-files {
  width: 100%;
  border-collapse: collapse;
}
.usb-files th,
.usb-files td {
  padding: 4px 8px;
  text-align: center;
  vertical-align: middle;
}
.usb-files .text-left {
  text-align: left;
}
.usb-files .text-right {
  text-align: right;
  white-space: nowrap;
}
.usb-files td.name {
  word-break: break-all;
}
.usb-files tbody tr:nth-child(odd) {
  background: rgba(128, 128, 128, 0.08);
}
</style>
