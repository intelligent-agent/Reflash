<template>
  <w-app>
    <TheLogger :open="openLog"  @close="openLog=false"/>
    <TheOptions 
      :open="openOptions"
      @set-option="setOption"
      @reboot-board="rebootBoard"
      @shutdown-board="shutdownBoard"
      @close="openOptions=false"/>
    <w-card  class="mxa pa3 card secondary" >
      <w-flex wrap class="text-center">
        <div class="xs5 pa1">
          <img style="width: 40px; height: 40px" :src="computeSVG('logo')" />
          <h3>REFLASH</h3>
          <div>
            <w-button @click="openLog = !openLog" text>
              <img style="width: 25px; height: 25px" :src="computeSVG('Log')" />
            </w-button>
            <w-button @click="openInfo = !openInfo" text>
              <img style="width: 25px; height: 25px" :src="computeSVG('Info')" />
            </w-button>
            <w-button @click="openOptions = !openOptions" text>
              <img style="width: 25px; height: 25px" :src="computeSVG('Options')" />
            </w-button>
          </div>
        </div>
        <div class="xs5 pa4">
          <TheInfo :open="openInfo" :version="reflash_version" :revision="recore_revision" />
        </div>
        <div class="xs1 pa1 align-self-center">
          <w-select
            v-model="selectedMethod"
            :items="availableMethods"
            no-unselect
            return-object>
          </w-select>
        </div>
        <div class="xs1 pa1 align-self-center">{{selectedMethod.id == 0 ? "Download" : "Upload"}}</div>
        <div class="xs1 pa1 align-self-center">USB drive</div>
        <div class="xs1 pa1 align-self-center"><FlashSelector ref="flashSelector"/></div>
        <div class="xs1 pa1 align-self-center">eMMC</div>

        <div class="xs1 pa1 align-self-center"><img style="width: 60%;" :src="computeSVG(selectedMethod.image)" /></div>
        <div class="xs1 pa1 align-self-center"><img style="width: 60%;" :src="computeSVG('Arrow-right')" /></div>
        <div class="xs1 pa1 align-self-center"><img style="width: 60%;" :src="computeSVG('USB')" /></div>
        <div class="xs1 pa1 align-self-center"><img style="width: 60%;" :src="computeSVG('Arrow-'+flashDirection())" /></div>
        <div class="xs1 pa1 align-self-center"><img style="width: 60%;" :src="computeSVG('eMMC')" /></div>

        <div class="xs1 pa1 therow">Choose image to {{selectedMethod.id == 0 ? "Download" : "Upload"}}</div>
        <div class="xs1 pa1">
          <ProgressBar ref="transferprogressbar" name="transfer"/>
          <span class="red">{{this.computeSizeCheckText()}}</span>
        </div>
        <div class="xs1 pa1">Choose image to install</div>
        <div class="xs1 pa1">
          <ProgressBar ref="installprogressbar" name="install"/>
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
            placeholder="Please select one">
          </w-select>
          <w-input
            v-if="selectedMethod.id == 1"
            type="file"
            ref="inputFile"
            v-model="selectedUploadImage"
            static-label
            @input="onFileInput">
              Select image file to upload
          </w-input>
        </div>
        <div class="xs1 align-self-center justify-space-between">
          <w-button xl outline @click="onTransferButtonClick()" v-if="this.isTransferButtonVisible()">
            <span>{{this.computeTransferButtonText()}}</span>
          </w-button>
        </div>
        <w-flex class="xs1 align-self-center flex justify-start">
          <w-select
            v-model="selectedLocalImage"
            :items="localImages"
            item-label-key="name"
            placeholder="Please select one"
            :item-click="onSelectedFileChanged()">
          </w-select>
          <IntegrityChecker ref="integritychecker"/>
        </w-flex>
        <div class="xs1 align-self-center">
          <w-button xl outline @click="onInstallButtonClick()" v-if="isInstallButtonVisibile()">
            <span>
              {{this.installButtonText()}}
            </span>
          </w-button>
        </div>
        <div class="xs1">
          <w-input v-model="backupFile" v-if="flash.selectedMethod == 1">Label</w-input>
        </div>

        <div class="xs5">
          <div v-if="installFinished && !showOverlay">
            <w-transition-expand y>
              <w-alert>
                Install finished! Please press the reboot button.
              </w-alert>
            </w-transition-expand>
            <w-button xl outline class="ma1 btn" @click="rebootBoard()"><span>Reboot Now</span></w-button>
            <w-button xl outline class="ma1 btn" @click="checkUsbPresent()"><span>Check USB present</span></w-button>
          </div>
          <div v-if="showOverlay">
            <w-transition-expand y>
              <w-alert v-if="showOverlay">
                Please wait while board is rebooting
              </w-alert>
            </w-transition-expand>
            <w-progress class="ma1" circle></w-progress><br>
            <w-button xl outline @click="isServerUp()" v-if="showOverlay"><span>Check server</span></w-button>
          </div>
        </div>
      </w-flex>
    </w-card>
  </w-app>
</template>

<script>
import TheOptions from './components/TheOptions'
import TheLogger from './components/TheLogger'
import TheInfo from './components/TheInfo'
import ProgressBar from './components/ProgressBar'
import FlashSelector from './components/FlashSelector'
import IntegrityChecker from './components/IntegrityChecker'
import WaveUI from 'wave-ui'
import { mapGetters, mapActions } from 'vuex';
import axios from 'axios';

export default {
  name: 'App',
  components: {
    TheOptions,
    TheLogger,
    TheInfo,
    ProgressBar,
    FlashSelector,
    IntegrityChecker
  },
  setup () {
    const waveui = new WaveUI(this, {})
    return { waveui }
  },
  data: () => ({
    installFinished: false,
    isDownloading: false,
    isTransferring: false,
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
      { id: 0, label: 'GitHub', value: 0, image: 'Cloud'},
      { id: 1, label: 'File upload', value: 1, image: 'File'}
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
    isUsbPresent: false,
  }),
  computed: mapGetters(['options', 'progress', 'flash']),
  methods: {
    ...mapActions([
      'setProgress',
      'setBandwidth',
      'setVisible',
      'setFlashMethod',
      'setTimeStarted',
      'setTimeFinished']),
    computeImage(name){
      return require('./assets/'+name+'-'+this.imageColor+'.png')
    },
    computeSVG(name){
      return require('./assets/'+name+'-'+this.imageColor+'.svg')
    },
    computeTransferButtonText(){
      if(this.selectedMethod.id == 0){
        return this.isTransferring ? "Cancel" : "Download";
      }
      else if(this.selectedMethod.id == 1){
        return this.isTransferring ? "Cancel" : "Upload";
      }
      return "";
    },
    flashDirection(){
      return this.flash.selectedMethod == 0 ? 'right' : 'left';
    },
    installButtonText(){
      if(this.isInstalling){
        return "Cancel"
      }
      if(this.flash.selectedMethod == 0){
        return "Install";
      }
      else{
        return "Backup";
      }
    },
    isInstallButtonVisibile(){
      if(this.flash.selectedMethod == 0){
        return this.selectedLocalImage;
      }
      else{
        return this.backupFile != "";
      }
    },
    isTransferButtonVisible(){
      if(this.selectedMethod.id == 0){
        return this.selectedGithubImage;
      }
      else if(this.selectedMethod.id == 1){
        return this.selectedUploadImage.file;
      }
      return "";
    },    
    setTheme(darkmode){
      this.imageColor = darkmode ? "white" : "black";
      if(darkmode){
        if(this.dark.parentElement == null){
            document.head.appendChild(this.dark)
        }
      }
      else{
        if(this.dark.parentElement === document.head){
          document.head.removeChild(this.dark);
        }
      }
    },
    computeSizeCheckText(){
      if(this.selectedMethod.id == 0 && this.selectedGithubImage && this.bytesAvailable > 0 && this.bytesAvailable < this.selectedGithubImage.size){
          return "Not enough free space on USB";
      }
      else if(this.selectedMethod.id == 1 && this.selectedUploadImage.file && this.bytesAvailable > 0 && this.bytesAvailable < this.selectedUploadImage.size){
        return "Not enough free space on USB"
      }
      else{
        return ""
      }
    },
    onSelectedFileChanged(){
      if(this.$refs.integritychecker && this.selectedLocalImage != []){
        this.$refs.integritychecker.fileSelected(this.selectedLocalImage);
      }
    },
    onFileInput(files){
      this.files = files
      this.file = files.file;
    },
    async uploadFinish(){
      await axios.put(`/api/upload_finish`
      ).then(function(response) {
            self.status = response.data["success"];
      });
    },
    async uploadCancel(){
      await axios.put(`/api/upload_cancel`
      ).then(function(response) {
            self.status = response.data["success"];
      });      
    },
    async uploadSelected(){
      let self = this;
      if(this.isTransferring){
          await axios.put(`/api/upload_start`, {
          "filename": self.file.name,
          "size": self.file.size,
          "start_time": Date.now(),
        }).then(function(response) {
              self.status = response.data["success"];
              self.uploadLocalFile();
              self.checkUploadProgress();
        });
      }
      else{
        self.uploadCancel();
      }
    },
    async uploadLocalFile(){
      const CHUNK_SIZE = 3*1024*1024;
      let self = this;
      var reader = new FileReader();
      var offset = 0;
      var filesize = this.file.size;

      reader.onload = function(){
          var result = reader.result;
          var chunk = result;
          axios.post(`/api/upload_chunk`, {
             "chunk": chunk
           }).then(function(response) {
            const status = response.data
            if(status.success && self.isTransferring){
              offset += CHUNK_SIZE;
              if(offset <= filesize){
                var slice = self.file.slice(offset, offset + CHUNK_SIZE);
                reader.readAsDataURL(slice);
              }
              else{
                offset = filesize;
                self.uploadFinish();
              }
            }
            else{
              self.uploadCancel();
            }
          });
      };

      if(this.file){
          var slice = this.file.slice(offset, offset + CHUNK_SIZE);
          reader.readAsDataURL(slice);
          self.fileName = this.file.name;
      }
    },
    async checkUploadProgress(){
      const response = await axios.get(`/api/get_progress`);
      let data = response.data;
      if(data.state == "UPLOADING"){
        this.isTransferring = true;
        this.setProgress({name: 'transfer', progress: data.progress});
        this.setBandwidth({name: 'transfer', bandwidth: data.bandwidth});
        this.setTimeStarted({name: 'transfer', time: data.start_time});
        this.setVisible({name: 'transfer', visible: true});
        this.$refs.transferprogressbar.update();
        this.selectedMethod = this.availableMethods[1];
        setTimeout(this.checkUploadProgress, 1000);
      }
      else{
        this.isTransferring = false;
        this.setVisible({name: 'transfer', visible: false});
        if(data.state == "ERROR"){
          this.$waveui.notify(data.error, "error", 0);
        }
        else if(data.state == "FINISHED"){
          this.selectedUploadImage = [];
          await this.getLocalImages();
          this.selectedLocalImage = data.filename.slice(0, -7);
        }
        else if(data.state == "CANCELED"){
          this.getLocalImages();
        }
      }
    },
    onTransferButtonClick(){
      this.isTransferring = !this.isTransferring;
      if(this.selectedMethod.id == 0){
        this.downloadSelected();
      }
      else if(this.selectedMethod.id == 1){
        this.uploadSelected();
      }
    },
    async downloadSelected(){
      let self = this;
      if(this.isTransferring){
        await axios.put(`/api/download_refactor`, {
          "filename": this.selectedGithubImage["name"], 
          "size": this.selectedGithubImage["size"],
          "url": this.selectedGithubImage["url"],
          "start_time": Date.now(),
        }).then(() => {
          self.checkDownloadProgress();
        });
      }
      else{
        await axios.put(`/api/cancel_download`).then(() => {
          self.checkDownloadProgress();
        });        
      }
    },
    async checkDownloadProgress() {
      const response = await axios.get(`/api/get_progress`);
      let data = response.data;
      if(data.state == "DOWNLOADING"){
        this.isTransferring = true;
        this.setProgress({name: 'transfer', progress: data.progress});
        this.setBandwidth({name: 'transfer', bandwidth: data.bandwidth});
        this.setTimeStarted({name: 'transfer', time: data.start_time});
        this.setVisible({name: 'transfer', visible: true});
        this.selectedGithubImage = this.getGithubImageFromName(data.filename)
        this.$refs.transferprogressbar.update();
        setTimeout(this.checkDownloadProgress, 1000);
      }
      else{
        this.isTransferring = false;
        this.setVisible({name: 'transfer', visible: false});
        if(data.state == "ERROR"){
          this.$waveui.notify(data.error, "error", 0);
        }
        else if(data.state == "FINISHED"){
          this.selectedGithubImage = null;
          await this.getLocalImages();
          this.selectedLocalImage = data.filename.slice(0, -7);
        }
        else if(data.state == "CANCELED"){
          this.getLocalImages();
        }
      }
    },
    onInstallButtonClick(){
      this.isInstalling = !this.isInstalling;
      if(this.flash.selectedMethod == 0){
        if(this.isInstalling){
          this.installSelected();
        }
        else{
          this.cancelInstall();
        }
      }
      else{
        if(this.isInstalling){
          this.backupSelected();
        }
        else{
          this.cancelBackup();
        }
      }
    },
    async installSelected(){
      let self = this;
      await axios.put(`/api/install_refactor`, {
          "filename": this.selectedLocalImage, 
          "start_time": Date.now(),
      }).then(() => {
        self.checkInstallProgress();
      });
    },
    async cancelInstall(){
      let self = this;
      await axios.put(`/api/cancel_installation`, {
          "filename": this.selectedLocalImage
      }).then(() => {
        self.checkInstallProgress();
      });
    },
    async checkInstallProgress() {
      const response = await axios.get(`/api/get_progress`);
      let data = response.data
      if(data.state == "INSTALLING"){
        this.isInstalling = true;
        this.setVisible({name: 'install', visible: true});
        this.setProgress({name: 'install', progress: data.progress});
        this.setBandwidth({name: 'install', bandwidth: data.bandwidth});
        this.setTimeStarted({name: 'install', time: data.start_time});
        this.selectedLocalImage = data.filename
        this.$refs.installprogressbar.update();
        setTimeout(this.checkInstallProgress, 1000);
      }
      else{
        this.isInstalling = false;
        this.setVisible({name: 'install', visible: false});
        if(data.state == "ERROR"){
          this.$waveui.notify(data.error, "error", 0);
        }
        else if(data.state == "FINISHED"){
          await axios.get(`/api/run_install_finished_commands`);
          if(this.options.rebootWhenDone){
            this.rebootBoard();
          }
          else{
            this.installFinished = true;
          } 
        }
      }
    },
    async cancelBackup(){
      await axios.put(`/api/cancel_backup`, {
          "filename": this.backupFile
      });
    },
    async backupSelected(){
      this.setProgress({name: 'install', progress: 0});
      this.$refs.installprogressbar.update();
      let self = this;
      await axios.put(`/api/backup_refactor`, {
          "filename": this.backupFile,
          "start_time": Date.now()
      }).then(() => {
        self.checkBackupProgress()
      });
    },
    async checkBackupProgress(){
      const response = await axios.get(`/api/get_progress`);
      let data = response.data
      if(data.state == "BACKUPING"){
        this.isInstalling = true;
        this.setVisible({name: 'install', visible: true});
        this.setTimeStarted({name: 'install', time: data.start_time}); 
        this.setProgress({name: 'install', progress: data.progress});

        if(this.flash.selectedMethod != 1){
          this.$refs.flashSelector.setSelection(1);
          this.backupFile = data.filename
        }
        this.$refs.installprogressbar.update();
        setTimeout(this.checkBackupProgress, 1000);
      }
      else{
        this.isInstalling = false;
        this.setVisible({name: 'install', visible: false});
        if(data.state == "FINISHED"){
          this.backupFile = "";
          this.getLocalImages();
        }
        if(data.state == "ERROR"){
          this.$waveui.notify(data.error, "error", 0);
        }
      }
    },
    rebootBoard(){
      this.showOverlay = true;
      axios.put(`/api/reboot_board`);
    },
    async checkUsbPresent(){
      const response = await axios.get(`/api/is_usb_present`);
      this.isUsbPresent = response.data.result
    },
    shutdownBoard(){
      axios.put(`/api/shutdown_board`);
    },
    enableSsh(){
      axios.put(`/api/enable_ssh`);
    },
    isServerUp(){
      fetch(`/favicon.ico`)
        .then(response => {
          if(response.status == 200){
            location.reload();
          }
          return response.status == 200;
        });
    },
    setOption(opt, value){
      if(opt == 'darkmode'){
        if(!this.dark){
          this.dark = document.createElement('link')
          this.dark.rel = 'stylesheet'
          this.dark.href = '/darkmode.css'
        }
        this.setTheme(value);
      }
      if(opt == 'bootFromEmmc'){
        axios.put(`/api/set_boot_media`, {'media': value ? 'emmc' : 'usb'});
      }
    },
    getGithubImageFromName(name){
      for(const img of this.githubImages){
        if(img.name == name){
          return img
        }
      }
    },
    populateImages(releases){
      for(let release of releases){
        for(let asset of release.assets){
          if(asset.name.includes("Refactor-recore")){
            this.githubImages.push({
              name: asset.name,
              id: asset.id,
              url: asset.browser_download_url,
              size: asset.size
            })
          }
        }
      }
    },
    async getLocalImages(){
      const response = await axios.get(`/api/get_info`);
      this.localImages = response.data.local_images;
      this.reflash_version = response.data.reflash_version;
      this.emmc_version = response.data.emmc_version;
      this.recore_revision = response.data.recore_revision
      this.bytesAvailable = response.data.bytes_available
    },
    async getGithubImages(){
      fetch("https://api.github.com/repos/intelligent-agent/Refactor/releases")
      .then(response => response.json())
      .then(data => (this.populateImages(data)));
    }
  },
  created(){
    this.selectedMethod = this.availableMethods[0];
    this.getGithubImages();
    this.getLocalImages();
    this.checkDownloadProgress();
    this.checkInstallProgress();
    this.checkBackupProgress();
    this.checkUploadProgress();
  },
}
</script>

<style>
h3 {
  font-family: 'Roboto';
  font-style: normal;
  font-weight: 300;
  font-size: 2em;
  margin: 0.2em;
}
h4 {
  font-family: 'Roboto';
  font-style: normal;
  font-weight: 300;
}
body {
  background-color: #F1F1F1;
  font-family: 'Roboto';
  font-style: normal;
  font-weight: 300;
}
#app {
  font-family: Avenir, Helvetica, Arial, sans-serif;
  -webkit-font-smoothing: antialiased;
  -moz-osx-font-smoothing: grayscale;
  text-align: center;
  color: #4D4D4D;
}
.w-select--no-padding .w-select__selection {
  text-align: center;
  color: #444;
  display: block;
}

.card {
  width: 70%;
  margin-top: 60px;
}

.w-app .primary--bg {
  color: #DDD;
  background-color: #04A3E5;
}

.w-app .secondary {
  color: #444;
}

.w-card {
  border: none;
}

.w-select__selection-wrap {
  border-color: #4D4D4D;
}

.w-select__selection {
  color: #4D4D4D;
}
.w-input--floating-label .w-input__input-wrap{
  margin: 0;
}
.w-button.size--md {
  padding-left: 16px;
  padding-right: 16px;
}

.w-app .pa3{
  border: none;
}
.therow {
  height: 45px;
}
.w-app .primary{
  color: #292A2C;
}
.w-button.size--xl {
  color: #04A3E5;
}
.w-button.size--xl span{
  color: #4D4D4D;
}

</style>
