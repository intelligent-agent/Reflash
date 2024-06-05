<template>
  <w-app>
    <TheLogger :open="openLog" @close="openLog = false" />
    <TheOptions
      :open="openOptions"
      @set-option="setOption"
      @reboot-board="rebootBoard"
      @shutdown-board="shutdownBoard"
      @close="openOptions = false"
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
          />
        </div>
        <div class="xs1 pa1 align-self-center">
          <w-select
            v-model="selectedMethod"
            :items="availableMethods"
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
          <w-select
            v-if="selectedMethod.id == 1"
            v-model="selectedRefactorImage"
            return-object
            :items="refactorImages"
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
    selectedRefactorImage: undefined,
    selectedRebuildImage: undefined,
    selectedUploadImage: [],
    selectedLocalImage: undefined,
    githubImages: [],
    refactorImages: [],
    rebuildImages: [],
    localImages: [],
    uploadError: false,
    openInfo: false,
    openLog: false,
    openOptions: false,
    showOverlay: false,
    availableMethods: [
      { id: 0, label: "Rebuild", value: 0, image: "Cloud" },
      { id: 1, label: "Refactor", value: 1, image: "Cloud" },
      { id: 2, label: "File upload", value: 2, image: "File" },
    ],
    selectedMethod: 0,
    imageColor: "white",
    files: [],
    backupFile: "",
    reflash_version: "Unknown",
    emmc_version: "Unknown",
    recore_revision: "Unknown",
    bytesAvailable: -1,
    sizeWarning: "",
  }),
  computed: mapGetters(["options", "progress", "flash"]),
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
      if (this.selectedMethod.id == 1)
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
      if (this.selectedMethod.id == 1 && this.selectedRefactorImage)
        return true;
      if (this.selectedMethod.id == 2 && this.selectedUploadImage.file)
        return true;
      return false;
    },
    isTransferButtonVisible() {
      if (this.options.magicmode) return false;
      if (this.selectedMethod.id == 0) return this.selectedRebuildImage;
      if (this.selectedMethod.id == 1) return this.selectedRefactorImage;
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
      } else if (
        this.selectedMethod.id == 1 &&
        this.selectedUploadImage.file &&
        this.bytesAvailable > 0 &&
        this.bytesAvailable < this.selectedUploadImage.size
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
    async startMagicUpload() {
      let self = this;
      if (this.state == "IDLE") {
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
      console.log("magig upload local file");
      const CHUNK_SIZE = 3 * 1024 * 1024;
      let self = this;
      var reader = new FileReader();
      var offset = 0;
      var filesize = this.file.size;
      console.log(filesize);

      reader.onload = function () {
        console.log("onload");
        var result = reader.result;
        var chunk = result;
        axios
          .post(`/api/upload_magic_chunk`, {
            chunk: chunk,
          })
          .then(function (response) {
            const status = response.data;
            if (status.success && self.state == "UPLOADING_MAGIC") {
              offset += CHUNK_SIZE;
              if (offset <= filesize) {
                var slice = self.file.slice(offset, offset + CHUNK_SIZE);
                reader.readAsDataURL(slice);
              } else {
                offset = filesize;
                self.apiCall("upload_finish");
              }
            } else {
              self.apiCall("upload_cancel");
            }
          });
      };

      if (this.file) {
        var slice = this.file.slice(offset, offset + CHUNK_SIZE);
        reader.readAsDataURL(slice);
        self.fileName = this.file.name;
      }
    },
    async uploadSelected() {
      let self = this;
      if (this.state == "IDLE") {
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
      let self = this;
      var reader = new FileReader();
      var offset = 0;
      var filesize = this.file.size;

      reader.onload = function () {
        var result = reader.result;
        var chunk = result;
        axios
          .post(`/api/upload_chunk`, {
            chunk: chunk,
          })
          .then(function (response) {
            const status = response.data;
            if (status.success && self.state == "UPLOADING") {
              offset += CHUNK_SIZE;
              if (offset <= filesize) {
                var slice = self.file.slice(offset, offset + CHUNK_SIZE);
                reader.readAsDataURL(slice);
              } else {
                offset = filesize;
                self.apiCall("upload_finish");
              }
            } else {
              self.apiCall("upload_cancel");
            }
          });
      };

      if (this.file) {
        var slice = this.file.slice(offset, offset + CHUNK_SIZE);
        reader.readAsDataURL(slice);
        self.fileName = this.file.name;
      }
    },
    onMagicButtonClick() {
      if (this.selectedMethod.id == 0) {
        this.selectedGithubImage = this.selectedRebuildImage;
        this.startMagic();
      } else if (this.selectedMethod.id == 1) {
        this.selectedGithubImage = this.selectedRefactorImage;
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
      } else if (data.state == "UPLOADING") {
        this.selectedMethod = this.availableMethods[2];
      }
      this.previousState = this.state;
      this.checkProgress();
    },
    async checkProgress() {
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
          this.getLocalImages();
        } else if (this.previousState == "DOWNLOADING") {
          this.selectedRebuildImage = null;
          this.selectedRefactorImage = null;
          await this.getLocalImages();
          this.selectedLocalImage = data.filename;
        } else if (this.previousState == "UPLOADING") {
          this.selectedUploadImage = [];
          await this.getLocalImages();
          this.selectedLocalImage = data.filename;
        } else if (this.previousState == "MAGIC") {
          this.selectedRebuildImage = null;
          this.selectedRefactorImage = null;
          await axios.get(`/api/run_install_finished_commands`);
          this.installFinished = true;
        }
      } else if (data.state == "CANCELLED") {
        this.selectedGithubImage = null;
        this.getLocalImages();
      } else if (data.state == "ERROR") {
        this.$waveui.notify(data.error, "error", 0);
      }

      this.previousState = this.state;
      if (data.state != "IDLE") setTimeout(this.checkProgress, 1000);
    },
    onTransferButtonClick() {
      if (this.selectedMethod.id == 0) {
        this.selectedGithubImage = this.selectedRebuildImage;
        this.downloadSelected();
      } else if (this.selectedMethod.id == 1) {
        this.selectedGithubImage = this.selectedRefactorImage;
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
    enableSsh() {
      this.apiCall("enable_ssh");
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
    populateRefactorImages(releases) {
      for (let release of releases) {
        for (let asset of release.assets) {
          if (asset.name.includes("Refactor-recore")) {
            this.refactorImages.push({
              name: asset.name,
              id: asset.id,
              url: asset.browser_download_url,
              size: asset.size,
            });
          }
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
    async getLocalImages() {
      const response = await axios.get(`/api/get_info`);
      this.localImages = response.data.local_images;
      this.reflash_version = response.data.reflash_version;
      this.emmc_version = response.data.emmc_version;
      this.recore_revision = response.data.recore_revision;
      this.bytesAvailable = response.data.bytes_available;
    },
    async getGithubImages() {
      fetch("https://api.github.com/repos/intelligent-agent/Refactor/releases")
        .then((response) => response.json())
        .then((data) => this.populateRefactorImages(data));
      fetch("https://api.github.com/repos/intelligent-agent/Rebuild/releases")
        .then((response) => response.json())
        .then((data) => this.populateRebuildImages(data));
    },
  },
  created() {
    this.selectedMethod = this.availableMethods[0];
    this.getGithubImages();
    this.getLocalImages();
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
