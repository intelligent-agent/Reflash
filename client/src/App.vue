<template>
  <w-app>
    <TheLogger :open="openLog" @close="openLog = false" />
    <TheOptions
      :open="openOptions"
      @set-option="setOption"
      @reboot-board="rebootBoard"
      @shutdown-board="shutdownBoard"
      @close="openOptions = false"
      @open-serial-number="openSerialNumber=true"
      @open-wifi="openWifi=true"
    />
    <w-card class="mxa pa3 card secondary">
      <w-flex wrap class="text-center">
        <div class="xs5 pa1">
          <img style="width: 40px; height: 40px" :src="computeSVG('logo')" />
          <h3>REFLASH</h3>
          <div>
            <w-button @click="openLog = !openLog" text>
              <img style="width: 25px; height: 25px" :src="computeSVG('Log')" />
            </w-button>
            <w-button @click="openInfo = !openInfo" text>
              <img
                style="width: 25px; height: 25px"
                :src="computeSVG('Info')"
              />
            </w-button>
            <w-button @click="openOptions = !openOptions" text>
              <img
                style="width: 25px; height: 25px"
                :src="computeSVG('Options')"
              />
            </w-button>
          </div>
        </div>
        <div class="xs5 pa4">
          <TheInfo
            :open="openInfo"
            :version="reflash_version"
            :revision="recore_revision"
            :serialNumber="serial_number"
          />
        </div>
        <div class="xs1 pa1 align-self-center">
          <w-select
            v-model="selectedMethod"
            :items="filteredMethods"
            no-unselect
            return-object
          >
          </w-select>
        </div>
        <div class="xs1 pa1 align-self-center">
          {{ selectedMethod.id == 2 ? "Upload" : "Download" }}
        </div>
        <div class="xs1 pa1 align-self-center">
          {{ this.options.magicmode ? "Magic" : "USB drive" }}
        </div>
        <div class="xs1 pa1 align-self-center">
          <FlashSelector ref="flashSelector" />
        </div>
        <div class="xs1 pa1 align-self-center">eMMC</div>

        <div class="xs1 pa1 align-self-center">
          <img style="width: 60%" :src="computeSVG(selectedMethod.image)" />
        </div>
        <div class="xs1 pa1 align-self-center">
          <img style="width: 60%" :src="computeSVG('Arrow-right')" />
        </div>
        <div class="xs1 pa1 align-self-center">
          <img
            style="width: 60%"
            :src="computeSVG(this.options.magicmode ? 'magic' : 'USB')"
          />
        </div>
        <div class="xs1 pa1 align-self-center">
          <img
            style="width: 60%"
            :src="computeSVG('Arrow-' + flashDirection())"
          />
        </div>
        <div class="xs1 pa1 align-self-center">
          <img style="width: 60%" :src="computeSVG('eMMC')" />
        </div>

        <div class="xs1 pa1 therow">
          Choose image to {{ selectedMethod.id == 2 ? "Upload" : "Download" }}
        </div>
        <div class="xs1 pa1">
          <ProgressBar
            ref="transferprogressbar"
            v-show="state == 'DOWNLOADING' || state == 'UPLOADING'"
          />
          <span class="red">{{ this.computeSizeCheckText() }}</span>
        </div>
        <div class="xs1 pa1">
          <ProgressBar
            ref="magicprogressbar"
            v-show="state === 'MAGIC' || state === 'UPLOADING_MAGIC'"
          />
          {{ this.options.magicmode ? "" : "Choose image to install" }}
        </div>
        <div class="xs1 pa1">
          <ProgressBar
            ref="installprogressbar"
            v-show="state == 'INSTALLING' || state == 'BACKUPING'"
          />
        </div>
        <div class="xs1 pa1">
          <div v-if="flash.selectedMethod == 1">Backup Filename</div>
          <div v-if="flash.selectedMethod == 0">{{ emmc_version }}</div>
        </div>
        <div class="xs1 pa1">
          <w-select
            v-if="selectedMethod.id == 0"
            v-model="selectedRebuildImage"
            return-object
            :items="rebuildImages"
            item-label-key="name"
            placeholder="Please select one"
          >
          </w-select>
          <w-input
            v-if="selectedMethod.id == 2"
            type="file"
            ref="inputFile"
            v-model="selectedUploadImage"
            static-label
            @input="onFileInput"
          >
            Select image file to upload
          </w-input>
        </div>
        <div class="xs1 align-self-center justify-space-between">
          <w-button
            xl
            outline
            @click="onTransferButtonClick()"
            v-if="this.isTransferButtonVisible()"
          >
            <span>{{ this.computeTransferButtonText() }}</span>
          </w-button>
        </div>
        <w-flex class="xs1 align-self-center flex justify-start">
          <w-select
            v-if="this.options.magicmode == false"
            v-model="selectedLocalImage"
            :items="localImages"
            item-label-key="name"
            placeholder="Please select one"
            :item-click="onSelectedFileChanged()"
          >
          </w-select>
          <w-button
            style="margin: auto"
            xl
            outline
            @click="onMagicButtonClick()"
            v-if="isMagicButtonVisible()"
          >
            <span> {{ this.computeMagicButtonText() }} </span>
          </w-button>
          <IntegrityChecker ref="integritychecker" v-if="!options.magicmode" />
        </w-flex>
        <div class="xs1 align-self-center">
          <w-button
            xl
            outline
            @click="onInstallButtonClick()"
            v-if="isInstallButtonVisibile()"
          >
            <span>
              {{ this.installButtonText() }}
            </span>
          </w-button>
        </div>
        <div class="xs1">
          <w-input v-model="backupFile" v-if="flash.selectedMethod == 1"
            >Label</w-input
          >
        </div>
        <TheUsbChecker
          :open="installFinished"
          ref="TheUsbChecker"
          @reboot-board="rebootBoard"
        />
        <TheConfigUpdater
          :open="openSerialNumber"
          ref="TheConfigUpdater"
          @close="openSerialNumber = false; this.getInfo()"
        />
        <TheWifiSetup
          :open="openWifi"
          ref="TheWifiSetup"
          @close="openWifi = false; this.checkInternet()"
        />
      </w-flex>
    </w-card>
  </w-app>
</template>

<script>
import TheOptions from "./components/TheOptions";
import TheLogger from "./components/TheLogger";
import TheInfo from "./components/TheInfo";
import ProgressBar from "./components/ProgressBar";
import FlashSelector from "./components/FlashSelector";
import IntegrityChecker from "./components/IntegrityChecker";
import TheUsbChecker from "./components/TheUsbChecker";
import TheConfigUpdater from "./components/TheConfigUpdater";
import TheWifiSetup from "./components/TheWifiSetup";
import WaveUI from "wave-ui";
import { mapGetters, mapActions } from "vuex";
import axios from "axios";

export default {
  name: "App",
  components: {
    TheOptions,
    TheLogger,
    TheInfo,
    ProgressBar,
    FlashSelector,
    IntegrityChecker,
    TheUsbChecker,
    TheConfigUpdater,
    TheWifiSetup,
  },
  setup() {
    const waveui = new WaveUI(this, {});
    return { waveui };
  },
  data: () => ({
    state: "IDLE",
    previousState: "IDLE",
    installFinished: false,
    isDownloading: false,
    isMagicing: false,
    transferProgress: 0,
    isInstalling: false,
    installProgress: 0,
    selectedGithubImage: undefined,
    selectedRebuildImage: undefined,
    selectedUploadImage: [],
    selectedLocalImage: undefined,
    githubImages: [],
    rebuildImages: [],
    localImages: [],
    uploadError: false,
    openInfo: false,
    openLog: false,
    openOptions: false,
    showOverlay: false,
    openSerialNumber: false,
    openWifi: false,
    availableMethods: [
      { id: 0, label: "Rebuild", value: 0, image: "Cloud" },
      { id: 2, label: "File upload", value: 2, image: "File" },
    ],
    // Optimistic default so the download options aren't shown then
    // immediately hidden while the check is in flight - see checkInternet().
    hasInternet: true,
    selectedMethod: 0,
    imageColor: "white",
    files: [],
    backupFile: "",
    reflash_version: "Unknown",
    emmc_version: "Unknown",
    recore_revision: "Unknown",
    serial_number: "Unknown",
    bytesAvailable: -1,
    sizeWarning: ""
  }),
  computed: {
    ...mapGetters(["options", "progress", "flash"]),
    // Rebuild downloads from GitHub via the board itself (flash-from-url) -
    // without internet that can't work, so hide it and leave only the
    // local-file paths (upload, magic, install) - #74.
    filteredMethods() {
      if (this.hasInternet) {
        return this.availableMethods;
      }
      return this.availableMethods.filter((m) => m.id == 2);
    },
  },
  methods: {
    ...mapActions([
      "setProgress",
      "setBandwidth",
      "setFlashMethod",
      "setTimeStarted",
      "setTimeFinished",
    ]),
    computeImage(name) {
      return require("./assets/" + name + "-" + this.imageColor + ".png");
    },
    computeSVG(name) {
      return require("./assets/" + name + "-" + this.$waveui.theme + ".svg");
    },
    computeTransferButtonText() {
      if (this.selectedMethod.id == 0)
        return this.state == "DOWNLOADING" ? "Cancel" : "Download";
      if (this.selectedMethod.id == 2)
        return this.state == "UPLOADING" ? "Cancel" : "Upload";
      return "";
    },
    computeMagicButtonText() {
      return this.state == "MAGIC" || this.state == "UPLOADING_MAGIC"
        ? "Cancel"
        : "Magic";
    },
    flashDirection() {
      return this.flash.selectedMethod == 1 ? "left" : "right";
    },
    installButtonText() {
      if (this.state == "INSTALLING" || this.state == "BACKUPING") {
        return "Cancel";
      }
      if (this.flash.selectedMethod == 1) {
        return "Backup";
      } else {
        return "Install";
      }
    },
    isInstallButtonVisibile() {
      if (this.options.magicmode) return false;
      if (this.flash.selectedMethod == 1) {
        return this.backupFile != "";
      } else {
        return this.selectedLocalImage;
      }
    },
    isMagicButtonVisible() {
      if (!this.options.magicmode) return false;
      if (this.selectedMethod.id == 0 && this.selectedRebuildImage) return true;
      if (this.selectedMethod.id == 2 && this.selectedUploadImage.file)
        return true;
      return false;
    },
    isTransferButtonVisible() {
      if (this.options.magicmode) return false;
      if (this.selectedMethod.id == 0) return this.selectedRebuildImage;
      if (this.selectedMethod.id == 2) return this.selectedUploadImage.file;
      return "";
    },
    setTheme(darkmode) {
      this.imageColor = darkmode ? "white" : "black";
      if (darkmode) {
        this.$waveui.switchTheme("dark");
      } else {
        this.$waveui.switchTheme("light");
      }
    },
    computeSizeCheckText() {
      if (
        this.selectedMethod.id == 0 &&
        this.selectedGithubImage &&
        this.bytesAvailable > 0 &&
        this.bytesAvailable < this.selectedGithubImage.size
      ) {
        return "Not enough free space on USB";
      } else {
        return "";
      }
    },
    onSelectedFileChanged() {
      if (this.$refs.integritychecker && this.selectedLocalImage != []) {
        this.$refs.integritychecker.fileSelected(this.selectedLocalImage);
      }
    },
    onFileInput(files) {
      this.files = files;
      this.file = files.file;
    },
    async apiCall(call) {
      var self = this;
      await axios.put(`/api/` + call).then(function (response) {
        if (response.data.status == "ERROR") {
          self.$waveui.notify(response.data.error, "error", 0);
        }
      });
    },
    // Clear the throughput history so a new operation charts from scratch
    // rather than inheriting the tail of the previous one.
    resetProgressBars() {
      ["installprogressbar", "magicprogressbar", "transferprogressbar"].forEach(
        (ref) => {
          if (this.$refs[ref]) {
            this.$refs[ref].reset();
          }
        }
      );
    },
    async startMagicUpload() {
      let self = this;
      if (this.state == "IDLE") {
        this.resetProgressBars();
        this.state = "UPLOADING_MAGIC";
        await axios
          .put(`/api/upload_magic_start`, {
            filename: self.file.name,
            size: self.file.size,
            start_time: Date.now(),
          })
          .then(function (response) {
            self.status = response.data["success"];
            self.magicUploadLocalFile();
            self.checkProgress();
          });
      } else {
        this.apiCall("upload_cancel");
      }
    },
    async magicUploadLocalFile() {
      const CHUNK_SIZE = 3 * 1024 * 1024;
      // Five minutes, because a slow chunk here is normal rather than a
      // fault: the server writes into a FIFO that the flashing process
      // drains at eMMC speed, so once the pipe buffer fills the upload
      // is deliberately throttled and a single 3MB chunk can take a
      // long time. A 20s timeout treated that backpressure as an error
      // and killed the upload at ~1GB in, reproducibly; 60s got a flash
      // through but still stalled visibly for a long stretch, close
      // enough to the limit to be worth more headroom.
      //
      // This is not a latency budget, it is a deadlock guard - the only
      // thing it needs to catch is a server that will never drain the
      // pipe at all. Erring long is cheap; erring short corrupts the
      // flash, since there is no safe retry (see below).
      const CHUNK_TIMEOUT_MS = 300000;
      let self = this;
      var offset = 0;
      var filesize = this.file.size;

      // Chunks are posted as raw binary Blob slices rather than base64
      // inside JSON, matching uploadLocalFile - base64 inflated the wire
      // size by ~33%, which is a lot of avoidable transfer on a >1GB
      // image over this board's WiFi.
      //
      // Deliberately NO retry here, unlike uploadLocalFile. The
      // destination is a stream that cannot be rewound: if a request
      // times out client-side the server may already have written those
      // bytes into the pipe, so re-posting the chunk duplicates data and
      // xz dies with "Compressed data is corrupt" - which is exactly
      // what retrying caused. Retries would only be safe if the server
      // tracked chunk indices and ignored ones it had already consumed.
      // Until then, fail loudly and let the user start over.
      function postChunk() {
        var slice = self.file.slice(offset, offset + CHUNK_SIZE);
        axios
          .post(`/api/upload_magic_chunk`, slice, {
            headers: { "Content-Type": "application/octet-stream" },
            timeout: CHUNK_TIMEOUT_MS,
          })
          .then(function (response) {
            const status = response.data;
            if (status.success && self.state == "UPLOADING_MAGIC") {
              offset += CHUNK_SIZE;
              if (offset < filesize) {
                postChunk();
              } else {
                offset = filesize;
                self.apiCall("upload_magic_finish");
              }
            } else {
              self.apiCall("upload_cancel");
            }
          })
          .catch(function (error) {
            if (self.state != "UPLOADING_MAGIC") {
              return;
            }
            console.log("upload_magic_chunk failed: " + error);
            // Concatenate rather than passing the Error object -
            // notify() renders an object as an empty callout, which is
            // what made an earlier failure look like nothing happened.
            self.$waveui.notify(
              "Magic flash failed at " +
                Math.round((offset / filesize) * 100) +
                "%: " +
                error +
                ". The eMMC is now partially written - reboot and try again.",
              "error",
              0
            );
            self.apiCall("upload_cancel");
          });
      }

      if (this.file) {
        self.fileName = this.file.name;
        postChunk();
      }
    },
    async uploadSelected() {
      let self = this;
      if (this.state == "IDLE") {
        this.resetProgressBars();
        this.state = "UPLOADING";
        await axios
          .put(`/api/upload_start`, {
            filename: self.file.name,
            size: self.file.size,
            start_time: Date.now(),
          })
          .then(function (response) {
            self.status = response.data["success"];
            self.uploadLocalFile();
            self.checkProgress();
          });
      } else {
        this.apiCall("upload_cancel");
      }
    },
    async uploadLocalFile() {
      const CHUNK_SIZE = 3 * 1024 * 1024;
      const MAX_CHUNK_RETRIES = 20;
      const CHUNK_TIMEOUT_MS = 20000;
      const RETRY_BACKOFF_MS = 2000;
      const RETRY_BACKOFF_MAX_MS = 30000;
      let self = this;
      var offset = 0;
      var filesize = this.file.size;

      // Chunks are posted as raw binary Blob slices, not base64-encoded
      // inside a JSON body - base64 inflates the wire size by ~33%,
      // which mattered a lot on the WiFi-constrained upload path. This
      // also drops the FileReader/readAsDataURL round trip entirely.
      //
      // Chunks are sent one at a time, not pipelined. Concurrent chunks
      // were tried and reverted - this board's USB hub has WiFi and
      // storage sharing a single Transaction Translator (confirmed from
      // the hub's datasheet), so simultaneous network-receive and
      // disk-write transactions destabilize the connection rather than
      // just being slower.
      //
      // This link also has occasional multi-minute dead spells (no
      // error, no response, ever - axios has no default timeout for
      // that case) that look like genuine WiFi/AP hiccups rather than
      // one-off blips. Without an explicit timeout and a generous retry
      // budget, one bad stretch used to freeze the whole upload forever
      // with no feedback - see #61. Retries use exponential backoff
      // (capped) so a real multi-minute dropout can be ridden out
      // instead of giving up after a few seconds.
      function sendChunk(retriesLeft = MAX_CHUNK_RETRIES) {
        var slice = self.file.slice(offset, offset + CHUNK_SIZE);
        axios
          .post(`/api/upload_chunk`, slice, {
            headers: { "Content-Type": "application/octet-stream" },
            timeout: CHUNK_TIMEOUT_MS,
          })
          .then(function (response) {
            const status = response.data;
            if (status.success && self.state == "UPLOADING") {
              offset += CHUNK_SIZE;
              if (offset < filesize) {
                sendChunk();
              } else {
                self.apiCall("upload_finish");
              }
            } else {
              self.apiCall("upload_cancel");
            }
          })
          .catch(function (error) {
            if (self.state != "UPLOADING") {
              return;
            }
            if (retriesLeft > 0) {
              const attempt = MAX_CHUNK_RETRIES - retriesLeft;
              const backoff = Math.min(
                RETRY_BACKOFF_MS * Math.pow(2, attempt),
                RETRY_BACKOFF_MAX_MS
              );
              console.log(
                "upload_chunk failed, retrying in " +
                  backoff +
                  "ms (" +
                  retriesLeft +
                  " left): " +
                  error
              );
              setTimeout(function () {
                sendChunk(retriesLeft - 1);
              }, backoff);
            } else {
              self.$waveui.notify(
                "Upload failed after repeated retries: " + error,
                "error",
                0
              );
              self.apiCall("upload_cancel");
            }
          });
      }

      if (this.file) {
        self.fileName = this.file.name;
        sendChunk();
      }
    },
    onMagicButtonClick() {
      if (this.selectedMethod.id == 0) {
        this.selectedGithubImage = this.selectedRebuildImage;
        this.startMagic();
      } else if (this.selectedMethod.id == 2) {
        this.selectedGithubImage = this.selectedLocalImage;
        this.startMagicUpload();
      }
    },
    async startMagic() {
      let self = this;
      if (this.state == "IDLE") {
        this.state = "MAGIC";
        await axios
          .put(`/api/start_magic`, {
            filename: this.selectedGithubImage["name"],
            size: this.selectedGithubImage["size"],
            url: this.selectedGithubImage["url"],
            start_time: Date.now(),
          })
          .then(() => {
            self.checkProgress();
          });
      } else {
        axios.put(`/api/cancel_magic`);
      }
    },
    async checkOnLoadProgress() {
      // If this one-shot call fails (see checkProgress() below for why
      // that's a real possibility), checkProgress() never even gets
      // called, and the polling loop that's supposed to watch an
      // in-progress operation never starts in the first place.
      try {
        const response = await axios.get(`/api/get_progress`);
        let data = response.data;
        this.state = data.state;
        if (data.state == "MAGIC") {
          this.selectedGithubImage = this.getGithubImageFromName(data.filename);
        } else if (data.state == "DOWNLOADING") {
          this.selectedGithubImage = this.getGithubImageFromName(data.filename);
        } else if (data.state == "INSTALLING") {
          this.selectedLocalImage = data.filename;
        } else if (data.state == "BACKUPING") {
          if (this.flash.selectedMethod != 1) {
            this.$refs.flashSelector.setSelection(1);
            this.backupFile = data.filename;
          }
          // This method is called on page load. If a refresh happens during upload, we can not continue.
        } else if (data.state == "UPLOADING" || data.state == "UPLOADING_MAGIC") {
          this.selectedMethod = this.availableMethods.find((m) => m.id == 2);
        }
        this.previousState = this.state;
      } catch (err) {
        console.log("checkOnLoadProgress failed, starting polling anyway: " + err);
      }
      this.checkProgress();
    },
    async checkProgress() {
      // A timeout here (the server can legitimately take a while to
      // respond while it's busy with heavy eMMC I/O mid-flash) used to
      // throw out of this function entirely, skipping the
      // setTimeout(...) below and silently killing the polling loop for
      // good - the flash kept running server-side, but the UI stopped
      // watching it and just looked frozen. Retry instead of dying.
      try {
        const response = await axios.get(`/api/get_progress`);
        let data = response.data;
        this.state = data.state;
        if (
          [
            "DOWNLOADING",
            "UPLOADING",
            "INSTALLING",
            "BACKUPING",
            "MAGIC",
            "UPLOADING_MAGIC",
          ].includes(this.state)
        ) {
          this.setProgress({ progress: data.progress });
          this.setBandwidth({ bandwidth: data.bandwidth });
          this.setTimeStarted({ time: data.start_time });
          this.$refs.installprogressbar.update();
          this.$refs.magicprogressbar.update();
          this.$refs.transferprogressbar.update();
        } else if (data.state == "FINISHED") {
          if (this.previousState == "INSTALLING") {
            this.selectedLocalImage = null;
            await axios.get(`/api/run_install_finished_commands`);
            this.installFinished = true;
          } else if (this.previousState == "BACKUPING") {
            this.backupFile = "";
            this.getInfo();
          } else if (this.previousState == "DOWNLOADING") {
            this.selectedRebuildImage = null;
            await this.getInfo();
            this.selectedLocalImage = data.filename;
          } else if (this.previousState == "UPLOADING") {
            this.selectedUploadImage = [];
            await this.getInfo();
            this.selectedLocalImage = data.filename;
          } else if (this.previousState == "MAGIC") {
            this.selectedRebuildImage = null;
            await axios.get(`/api/run_install_finished_commands`);
            this.installFinished = true;
          } else if (this.previousState == "UPLOADING_MAGIC") {
            this.selectedUploadImage = [];
            await axios.get(`/api/run_install_finished_commands`);
            this.installFinished = true;
          }
        } else if (data.state == "CANCELLED") {
          this.selectedGithubImage = null;
          this.getInfo();
        } else if (data.state == "ERROR") {
          this.$waveui.notify(data.error, "error", 0);
        }

        this.previousState = this.state;
        if (data.state != "IDLE") setTimeout(this.checkProgress, 1000);
      } catch (err) {
        console.log("get_progress failed, retrying: " + err);
        setTimeout(this.checkProgress, 1000);
      }
    },
    onTransferButtonClick() {
      if (this.selectedMethod.id == 0) {
        this.selectedGithubImage = this.selectedRebuildImage;
        this.downloadSelected();
      } else if (this.selectedMethod.id == 2) {
        this.uploadSelected();
      }
    },
    async downloadSelected() {
      let self = this;
      if (this.state == "IDLE") {
        this.state = "DOWNLOADING";
        await axios
          .put(`/api/start_download`, {
            filename: this.selectedGithubImage["name"],
            size: this.selectedGithubImage["size"],
            url: this.selectedGithubImage["url"],
            start_time: Date.now(),
          })
          .then(() => {
            self.checkProgress();
          });
      } else {
        axios.put(`/api/cancel_download`);
      }
    },
    onInstallButtonClick() {
      if (this.flash.selectedMethod == 0) {
        if (this.state == "IDLE") {
          this.installSelected();
        } else {
          this.apiCall("cancel_installation");
        }
      } else {
        if (this.state == "IDLE") {
          this.backupSelected();
        } else {
          this.apiCall("cancel_backup");
        }
      }
    },
    async installSelected() {
      let self = this;
      await axios
        .put(`/api/start_installation`, {
          filename: this.selectedLocalImage,
          start_time: Date.now(),
        })
        .then(() => {
          self.checkProgress();
        });
    },
    async backupSelected() {
      this.setProgress({ progress: 0 });
      this.resetProgressBars();
      this.$refs.installprogressbar.update();
      let self = this;
      await axios
        .put(`/api/start_backup`, {
          filename: this.backupFile,
          start_time: Date.now(),
        })
        .then(() => {
          self.checkProgress();
        });
    },
    rebootBoard() {
      this.showOverlay = true;
      this.apiCall("reboot_board");
    },
    shutdownBoard() {
      this.apiCall("shutdown_board");
    },
    setOption(opt, value) {
      if (opt == "darkmode") {
        this.setTheme(value);
      }
    },
    getGithubImageFromName(name) {
      for (const img of this.githubImages) {
        if (img.name == name) {
          return img;
        }
      }
    },
    populateRebuildImages(releases) {
      for (let release of releases) {
        for (let asset of release.assets) {
          if (asset.name.includes("rebuild")) {
            this.rebuildImages.push({
              name: asset.name,
              id: asset.id,
              url: asset.browser_download_url,
              size: asset.size,
            });
          }
        }
      }
    },
    async getInfo() {
      const response = await axios.get(`/api/get_info`);
      this.localImages = response.data.local_images;
      this.reflash_version = response.data.reflash_version;
      this.emmc_version = response.data.emmc_version;
      this.recore_revision = response.data.recore_revision;
      this.serial_number = response.data.serial_number;
      this.bytesAvailable = response.data.bytes_available;
      this.openSerialNumber = (this.serial_number == "");
    },
    async getGithubImages() {
      // Reset first - this can now be called again (see checkInternet())
      // after already having been called once, and populateRebuildImages
      // appends rather than replaces.
      this.rebuildImages = [];
      fetch("https://api.github.com/repos/intelligent-agent/Rebuild/releases")
        .then((response) => response.json())
        .then((data) => this.populateRebuildImages(data));
    },
    async checkInternet() {
      const response = await axios.get(`/api/has_internet`);
      this.hasInternet = response.data.result;
      if (this.hasInternet) {
        // Refetch in case the initial page-load fetch happened before
        // internet was available (e.g. connected via the WiFi setup
        // window), which would otherwise leave rebuildImages empty.
        this.getGithubImages();
      } else if (this.selectedMethod.id != 2) {
        this.selectedMethod = this.availableMethods.find((m) => m.id == 2);
      }
    },
  },
  created() {
    this.selectedMethod = this.availableMethods.find((m) => m.id == 0);
    this.getGithubImages();
    this.getInfo();
    this.checkInternet();
    this.checkOnLoadProgress();
  },
};
</script>

<style>
:root[data-theme="light"] {
  --w-base-bg-color-rgb: #f1f1f1;
  --w-base-color-rgb: 0, 0, 0; /* black */
  --w-contrast-bg-color-rgb: 0, 0, 0; /* black */
  --w-contrast-color-rgb: 255, 255, 255; /* white */
  --w-disabled-color-rgb: 204, 204, 204; /* #ccc */
  --w-secondary-color: #292a2c;
  --w-primary-color: #292a2c;
}

:root[data-theme="dark"] {
  --w-base-bg-color-rgb: #292a2c; /* #222 */
  --w-base-color-rgb: 255, 255, 255; /* white */
  --w-contrast-bg-color-rgb: 255, 255, 255; /* white */
  --w-contrast-color-rgb: 0, 0, 0; /* black */
  --w-disabled-color-rgb: 74, 74, 74; /* #4a4a4a */
  --w-secondary-color: #c9c9c9;
  --w-primary-color: #c9c9c9;
}

h3 {
  font-family: "Roboto";
  font-style: normal;
  font-weight: 300;
  font-size: 2em;
  margin: 0.2em;
}
h4 {
  font-family: "Roboto";
  font-style: normal;
  font-weight: 300;
}
body {
  background-color: var(--w-base-bg-color-rgb);
  font-family: "Roboto";
  font-style: normal;
  font-weight: 300;
}
#app {
  font-family: Avenir, Helvetica, Arial, sans-serif;
  -webkit-font-smoothing: antialiased;
  -moz-osx-font-smoothing: grayscale;
  text-align: center;
}
.w-select--no-padding .w-select__selection {
  text-align: center;
  display: block;
}

.card {
  width: 70%;
  margin-top: 60px;
}

.w-app .primary--bg[data-theme="light"] {
  color: #ddd;
  background-color: #04a3e5;
}

.w-app .primary--bg[data-theme="dark"] {
  color: #292a2c;
  background-color: #04a3e5;
}

.w-card {
  border: none;
}

.w-input--floating-label .w-input__input-wrap {
  margin: 0;
}
.w-button.size--md {
  padding-left: 16px;
  padding-right: 16px;
}

.w-app .pa3 {
  border: none;
}
.therow {
  height: 45px;
}
.w-app .primary[data-theme="light"] {
  color: #292a2c;
}
.w-app .primary[data-theme="dark"] {
  color: #9ea1a8;
}

.w-button.size--xl {
  color: #04a3e5;
}
.w-button.size--xl span {
  color: var(--w-primary-color);
}
</style>
