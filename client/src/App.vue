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
          {{ selectedMethod.id == 0 ? "Download" : "Upload" }}
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
          Choose image to {{ selectedMethod.id == 0 ? "Download" : "Upload" }}
        </div>
        <div class="xs1 pa1">
          <ProgressBar
            ref="transferprogressbar"
            v-show="state == 'DOWNLOADING' || state == 'UPLOADING'"
          />
          <span class="red">{{ this.computeSizeCheckText() }}</span>
        </div>
        <div class="xs1 pa1">
          <ProgressBar ref="magicprogressbar" v-show="state === 'MAGIC'" />
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
            v-model="selectedGithubImage"
            return-object
            :items="githubImages"
            item-label-key="name"
            placeholder="Please select one"
          >
          </w-select>
          <w-input
            v-if="selectedMethod.id == 1"
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
          <IntegrityChecker ref="integritychecker" />
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
    installFinished: false,
    isDownloading: false,
    isTransferring: false,
    isMagicing: false,
    transferProgress: 0,
    isInstalling: false,
    installProgress: 0,
    selectedGithubImage: undefined,
    selectedUploadImage: [],
    selectedLocalImage: undefined,
    githubImages: [],
    localImages: [],
    uploadError: false,
    openInfo: false,
    openLog: false,
    openOptions: false,
    showOverlay: false,
    availableMethods: [
      { id: 0, label: "Refactor", value: 0, image: "Cloud" },
      { id: 1, label: "File upload", value: 1, image: "File" },
      { id: 2, label: "Rebuild", value: 2, image: "Cloud" },
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
      if (this.selectedMethod.id == 0) {
        return this.isTransferring ? "Cancel" : "Download";
      } else if (this.selectedMethod.id == 1) {
        return this.isTransferring ? "Cancel" : "Upload";
      }
      return "";
    },
    computeMagicButtonText() {
      return this.isTransferring ? "Cancel" : "Magic";
    },
    flashDirection() {
      return this.flash.selectedMethod == 0 ? "right" : "left";
    },
    installButtonText() {
      if (this.isInstalling) {
        return "Cancel";
      }
      if (this.flash.selectedMethod == 0) {
        return "Install";
      } else {
        return "Backup";
      }
    },
    isInstallButtonVisibile() {
      if (this.options.magicmode) return false;
      if (this.flash.selectedMethod == 0) {
        return this.selectedLocalImage;
      } else {
        return this.backupFile != "";
      }
    },
    isMagicButtonVisible() {
      if (
        this.options.magicmode &&
        this.selectedMethod.id == 0 &&
        this.selectedGithubImage
      )
        return true;
      return false;
    },
    isTransferButtonVisible() {
      if (this.options.magicmode) return false;
      if (this.selectedMethod.id == 0) {
        return this.selectedGithubImage;
      } else if (this.selectedMethod.id == 1) {
        return this.selectedUploadImage.file;
      }
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
    async uploadFinish() {
      await axios.put(`/api/upload_finish`).then(function (response) {
        self.status = response.data["success"];
      });
    },
    async uploadCancel() {
      await axios.put(`/api/upload_cancel`).then(function (response) {
        self.status = response.data["success"];
      });
    },
    async apiCall(call){
      var self = this
      await axios.put(`/api/`+call).then(function (response) {
        if(response.data.status == "ERROR"){
          self.$waveui.notify(response.data.error, "error", 0);
        }
      });
    },
    async uploadSelected() {
      let self = this;
      if (this.isTransferring) {
        await axios
          .put(`/api/upload_start`, {
            filename: self.file.name,
            size: self.file.size,
            start_time: Date.now(),
          })
          .then(function (response) {
            self.status = response.data["success"];
            self.uploadLocalFile();
            self.checkUploadProgress();
          });
      } else {
        self.uploadCancel();
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
            if (status.success && self.isTransferring) {
              offset += CHUNK_SIZE;
              if (offset <= filesize) {
                var slice = self.file.slice(offset, offset + CHUNK_SIZE);
                reader.readAsDataURL(slice);
              } else {
                offset = filesize;
                self.uploadFinish();
              }
            } else {
              self.uploadCancel();
            }
          });
      };

      if (this.file) {
        var slice = this.file.slice(offset, offset + CHUNK_SIZE);
        reader.readAsDataURL(slice);
        self.fileName = this.file.name;
      }
    },
    async checkUploadProgress() {
      const response = await axios.get(`/api/get_progress`);
      let data = response.data;
      this.state = data.state;
      if (data.state == "UPLOADING") {
        this.isTransferring = true;
        this.setProgress({ progress: data.progress });
        this.setBandwidth({ bandwidth: data.bandwidth });
        this.setTimeStarted({ time: data.start_time });
        this.$refs.transferprogressbar.update();
        this.selectedMethod = this.availableMethods[1];
        setTimeout(this.checkUploadProgress, 1000);
      } else {
        this.isTransferring = false;
        if (data.state == "ERROR") {
          this.$waveui.notify(data.error, "error", 0);
        } else if (data.state == "FINISHED") {
          this.selectedUploadImage = [];
          await this.getLocalImages();
          this.selectedLocalImage = data.filename;
        } else if (data.state == "CANCELED") {
          this.getLocalImages();
        }
      }
    },
    onMagicButtonClick() {
      this.isTransferring = !this.isTransferring;
      this.startMagic();
    },
    async startMagic() {
      let self = this;
      if (this.isTransferring) {
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
    async checkProgress() {
      const response = await axios.get(`/api/get_progress`);
      let data = response.data;
      this.state = data.state;
      if (data.state == "MAGIC") {
        this.isTransferring = true;
        this.setProgress({ progress: data.progress });
        this.setBandwidth({ bandwidth: data.bandwidth });
        this.setTimeStarted({ time: data.start_time });
        this.selectedGithubImage = this.getGithubImageFromName(data.filename);
        this.$refs.magicprogressbar.update();
        setTimeout(this.checkProgress, 1000);
      } else {
        this.isTransferring = false;
        if (data.state == "ERROR") {
          this.$waveui.notify(data.error, "error", 0);
        } else if (data.state == "FINISHED") {
          await axios.get(`/api/run_install_finished_commands`);
          this.installFinished = true;
        } else if (data.state == "CANCELED") {
          this.selectedGithubImage = null;
        }
      }
    },
    onTransferButtonClick() {
      this.isTransferring = !this.isTransferring;
      if (this.selectedMethod.id == 0) {
        this.downloadSelected();
      } else if (this.selectedMethod.id == 1) {
        this.uploadSelected();
      }
    },
    async downloadSelected() {
      let self = this;
      if (this.isTransferring) {
        await axios
          .put(`/api/start_download`, {
            filename: this.selectedGithubImage["name"],
            size: this.selectedGithubImage["size"],
            url: this.selectedGithubImage["url"],
            start_time: Date.now(),
          })
          .then(() => {
            self.checkDownloadProgress();
          });
      } else {
        axios.put(`/api/cancel_download`);
      }
    },
    async checkDownloadProgress() {
      const response = await axios.get(`/api/get_progress`);
      let data = response.data;
      this.state = data.state;
      if (data.state == "DOWNLOADING") {
        this.isTransferring = true;
        this.setProgress({ progress: data.progress });
        this.setBandwidth({ bandwidth: data.bandwidth });
        this.setTimeStarted({ time: data.start_time });
        this.selectedGithubImage = this.getGithubImageFromName(data.filename);
        this.$refs.transferprogressbar.update();
        setTimeout(this.checkDownloadProgress, 1000);
      } else {
        this.isTransferring = false;
        if (data.state == "ERROR") {
          this.$waveui.notify(data.error, "error", 0);
        } else if (data.state == "FINISHED") {
          this.selectedGithubImage = null;
          await this.getLocalImages();
          this.selectedLocalImage = data.filename;
        } else if (data.state == "CANCELED") {
          this.getLocalImages();
        }
      }
    },
    onInstallButtonClick() {
      this.isInstalling = !this.isInstalling;
      if (this.flash.selectedMethod == 0) {
        if (this.isInstalling) {
          this.installSelected();
        } else {
          this.cancelInstall();
        }
      } else {
        if (this.isInstalling) {
          this.backupSelected();
        } else {
          this.cancelBackup();
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
          self.checkInstallProgress();
        });
    },
    async cancelInstall() {
      await axios.put(`/api/cancel_installation`, {
        filename: this.selectedLocalImage,
      });
    },
    async checkInstallProgress() {
      const response = await axios.get(`/api/get_progress`);
      let data = response.data;
      this.state = data.state;
      if (data.state == "INSTALLING") {
        this.isInstalling = true;
        this.setProgress({ progress: data.progress });
        this.setBandwidth({ bandwidth: data.bandwidth });
        this.setTimeStarted({ time: data.start_time });
        this.selectedLocalImage = data.filename;
        this.$refs.installprogressbar.update();
        setTimeout(this.checkInstallProgress, 1000);
      } else {
        this.isInstalling = false;
        if (data.state == "ERROR") {
          this.$waveui.notify(data.error, "error", 0);
        } else if (data.state == "FINISHED") {
          await axios.get(`/api/run_install_finished_commands`);
          this.installFinished = true;
        }
      }
    },
    async cancelBackup() {
      await axios.put(`/api/cancel_backup`, {
        filename: this.backupFile,
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
          self.checkBackupProgress();
        });
    },
    async checkBackupProgress() {
      const response = await axios.get(`/api/get_progress`);
      let data = response.data;
      this.state = data.state;
      if (data.state == "BACKUPING") {
        this.isInstalling = true;
        this.setTimeStarted({ time: data.start_time });
        this.setProgress({ progress: data.progress });
        this.setBandwidth({ bandwidth: data.bandwidth });
        if (this.flash.selectedMethod != 1) {
          this.$refs.flashSelector.setSelection(1);
          this.backupFile = data.filename;
        }
        this.$refs.installprogressbar.update();
        setTimeout(this.checkBackupProgress, 1000);
      } else {
        this.isInstalling = false;
        if (data.state == "FINISHED") {
          this.backupFile = "";
          this.getLocalImages();
        }
        if (data.state == "ERROR") {
          this.$waveui.notify(data.error, "error", 0);
        }
      }
    },
    rebootBoard() {
      this.showOverlay = true;
      axios.put(`/api/reboot_board`);
    },
    shutdownBoard() {
      this.apiCall("shutdown_board")
    },
    enableSsh() {
      axios.put(`/api/enable_ssh`);
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
    populateImages(releases) {
      for (let release of releases) {
        for (let asset of release.assets) {
          if (asset.name.includes("Refactor-recore")) {
            this.githubImages.push({
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
        .then((data) => this.populateImages(data));
    },
  },
  created() {
    this.selectedMethod = this.availableMethods[0];
    this.getGithubImages();
    this.getLocalImages();
    this.checkDownloadProgress();
    this.checkInstallProgress();
    this.checkBackupProgress();
    this.checkUploadProgress();
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
