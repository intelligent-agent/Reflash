<template>
  <w-app>
    <w-card  class="mxa pa3 card secondary" >
      <w-flex wrap class="text-center">
        <div class="xs5 pa1">
          <img style="width: 40px; height: 40px" :src="computeImage('logo-thing')" />
          <h3>REFLASH</h3>
          <w-button @click="openInfo = !openInfo" text>
            <w-icon md>fa fa-info-circle</w-icon>
          </w-button>
          <TheOptions
            @set-option="setOption"
            @reboot-board="rebootBoard"/>
        </div>
        <div class="xs5 pa4">
          <w-transition-expand y>
              <p v-if="openInfo" class="text-left">
                This page is running from a USB drive on Recore. You can use this page to download and
                install distros to the eMMC of Recore.
                <ol>
                  <li>Start by downloading an image, probably the latest version. </li>
                  <li>Once downloaded, you can flash the image to the internal storage (eMMC)</li>
                  <li>Finally reboot the board. The board will boot from the internal storage.</li>
                </ol>
              </p>
          </w-transition-expand>
        </div>

        <div class="xs1 pa1 align-self-center">
          <w-select
            v-model="selectedMethod"
            :items="availableMethods"
            no-unselect
            return-object>
          </w-select>
        </div>
        <div class="xs1 pa1 align-self-center"><b>{{selectedMethod.id == 0 ? "Download" : "Upload"}}</b></div>
        <div class="xs1 pa1 align-self-center"><b >USB drive</b></div>
        <div class="xs1 pa1 align-self-center"><FlashSelector /></div>
        <div class="xs1 pa1 align-self-center"><b>eMMC</b></div>

        <div class="xs1 pa1 align-self-center"><img :src="computeImage(selectedMethod.image)" /></div>
        <div class="xs1 pa1 align-self-center"><img :src="computeImage('arrow-left')" /></div>
        <div class="xs1 pa1 align-self-center"><img :src="computeImage('USB')" /></div>
        <div class="xs1 pa1 align-self-center"><img :src="computeImage('arrow-'+flashDirection())" /></div>
        <div class="xs1 pa1 align-self-center"><img :src="computeImage('eMMC')" /></div>

        <div class="xs1 pa1"><b>Choose image to {{selectedMethod.id == 0 ? "Download" : "Upload"}}</b></div>
        <div class="xs1 pa1">
          <ProgressBar ref="transferprogressbar" name="transfer"/>
        </div>
        <div class="xs1 pa1"><b>Choose image to install</b></div>
        <div class="xs1 pa1">
          <ProgressBar ref="installprogressbar" name="install"/>
        </div>
        <div class="xs1 pa1"><b v-if="flash.selectedMethod.id == 1">Backup Filename</b></div>

        <div class="xs1 pa1">
          <w-select
            v-if="selectedMethod.id == 0"
            v-model="selectedGithubImage"
            return-object
            :items="githubImages"
            item-label-key="name"
            placeholder="Please select one"
            outline>
          </w-select>
          <w-input
            type="file"
            ref="inputFile"
            v-model="selectedUploadImage"
            static-label
            @input="onFileInput"
            v-if="selectedMethod.id == 1">
              Select image file to upload
          </w-input>
        </div>
        <div class="xs1 align-self-center justify-space-between">
          <w-button @click="onTransferButtonClick()" v-if="this.computeTransferButtonVisible()">{{this.computeTransferButtonText()}}</w-button>
        </div>
        <div class="xs1 align-self-center">
          <w-select
            v-model="selectedLocalImage"
            :items="localImages"
            item-label-key="name"
            placeholder="Please select one"
            outline>
          </w-select>
        </div>
        <div class="xs1 align-self-center">
          <w-button @click="onInstallButtonClick()" v-if="installButtonVisibility()">{{this.computeInstallButtonText()}}</w-button>
        </div>
        <div class="xs1">
          <w-input v-model="backupFile" v-if="flash.selectedMethod.id == 1" outline>Label</w-input>
        </div>

        <div class="xs5">
          <w-transition-expand y>
            <w-alert v-if="installFinished">
              Install finished! Now reboot the board by pressing this button
            </w-alert>
          </w-transition-expand>
          <w-button v-if="installFinished" class="ma1 btn" @click="rebootBoard()">Reboot Now</w-button>
        </div>
      </w-flex>
    </w-card>
    <w-overlay v-model="showOverlay">
      <div class="text-center">
        <w-progress class="ma1" circle></w-progress>
        <p>Please wait while board is rebooting</p>
        <w-button @click="isServerUp()">Check server</w-button>
      </div>
    </w-overlay>
  </w-app>
</template>

<script>
import TheOptions from './components/TheOptions'
import ProgressBar from './components/ProgressBar'
import FlashSelector from './components/FlashSelector'
import WaveUI from 'wave-ui'
import { mapGetters, mapActions } from 'vuex';
import axios from 'axios';

export default {
  name: 'App',
  components: {
    TheOptions,
    ProgressBar,
    FlashSelector
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
    installButtonText: "Install",
    selectedGithubImage: null,
    selectedUploadImage: [],
    selectedLocalImage: null,
    githubImages: [],
    localImages: [],
    uploadError: false,
    openInfo: false,
    showOverlay: false,
    availableMethods: [
      { id: 0, label: 'GitHub', value: 0, image: 'GitHub'},
      { id: 1, label: 'File upload', value: 1, image: 'File'}
    ],
    selectedMethod: 0,
    imageColor: "white",
    files: [],
    backupFile: ""
  }),
  methods: {
    ...mapActions([
      'setProgress',
      'setVisible',
      'setTimeStarted',
      'setTimeFinished']),
    computeImage(name){
      return require('./assets/'+name+'-'+this.imageColor+'.png')
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
      return this.flash.selectedMethod.id == 0 ? 'left' : 'right';
    },
    computeInstallButtonText(){
      if(this.isInstalling){
        return "Cancel"
      }
      if(this.flash.selectedMethod.id == 0){
        return "Install";
      }
      else{
        return "Backup";
      }
    },
    installButtonVisibility(){
      if(this.flash.selectedMethod.id == 0){
        return this.selectedLocalImage;
      }
      else{
        return this.backupFile != "";
      }
    },
    computeTransferButtonVisible(){
      if(this.selectedMethod.id == 0){
        return this.selectedGithubImage;
      }
      else if(this.selectedMethod.id == 1){
        return this.selectedUploadImage.length > 0;
      }
      return "";
    },
    isTransferButtonVisible(){
      if(this.selectedMethod.id == 0){
        return this.selectedGithubImage !== null;
      }
      else if(this.selectedMethod.id == 1){
        return this.selectedUploadImage != [];
      }
      return false;
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
    onFileInput(files){
      this.files = files
      this.file = files[0].file;
    },
    uploadLocalFile(){
      const CHUNK_SIZE = 1024*1024;
      let self = this;
      var reader = new FileReader();
      var offset = 0;
      var filesize = this.file.size;
      this.setTimeStarted({name: 'transfer', time: Date.now()});

      reader.onload = function(){
          var result = reader.result;
          var chunk = result;
          self.runCommand("upload_chunk", {
             "chunk": chunk,
             "filename": self.fileName,
             "is_new_file": offset == 0
           }, function(status) {
            if(status.success && self.isTransferring){
              self.$refs.transferprogressbar.update();
              offset += CHUNK_SIZE;
              if(offset <= filesize){
                var slice = self.file.slice(offset, offset + CHUNK_SIZE);
                reader.readAsDataURL(slice);
                self.transferProgress = Math.round((offset/filesize)*100);
                self.setProgress({name: 'transfer', progress: self.transferProgress });
              }
              else{
                offset = filesize;
                self.transferProgress = 100;
                self.isTransferring = false;
                self.setVisible({name: 'transfer', visible: false});
                self.getData();
              }
            }
            else{
              self.uploadError = true;
              self.isTransferring = false;
              self.setVisible({name: 'transfer', visible: false});
            }
          });
      };

      if(this.file)
      {
          var slice = this.file.slice(offset, offset + CHUNK_SIZE);
          reader.readAsDataURL(slice);
          self.fileName = this.file.name;
      }
    },
    onTransferButtonClick(){
      this.isTransferring = !this.isTransferring;
      this.setVisible({name: 'transfer', visible: this.isTransferring});
      if(this.selectedMethod.id == 0){
        this.downloadSelected();
      }
      else if(this.selectedMethod.id == 1){
        this.uploadLocalFile();
      }
    },
    downloadSelected(){
      let self = this;
      if(this.isTransferring){
        self.setTimeStarted({name: 'transfer', time: Date.now()});
        this.runCommand("download_refactor", {
            "refactor_image": this.selectedGithubImage
        }, function() {
            self.downloadProgressTimer = setInterval(self.getDownloadProgress, 1000);
        });
      }
      else{
        this.runCommand("cancel_download", {}, function() {
            clearInterval(self.downloadProgressTimer);
            self.isTransferring = false;
            self.setVisible({name: 'transfer', visible: false});
        });
      }
    },
    async getDownloadProgress() {
      const response = await axios.get(`/api/get_download_progress`);
      let data = response.data
      this.setProgress({name: 'transfer', progress: data.progress*100});
      this.$refs.transferprogressbar.update();
      if(data.is_finished){
        clearInterval(this.downloadProgressTimer);
        this.isTransferring = false;
        this.setVisible({name: 'transfer', visible: this.isTransferring});
        this.getData();
      }
    },
    onInstallButtonClick(){
      this.isInstalling = !this.isInstalling;
      this.setVisible({name: 'install', visible: this.isInstalling});
      if(this.flash.selectedMethod.id == 0){
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
    async cancelBackup(){
      await axios.put(`/api/cancel_backup`, {
          "filename": this.backupFile
      });
    },
    async backupSelected(){
      this.setTimeStarted({name: 'install', time: Date.now()});
      this.setProgress({name: 'install', progress: 0});
      this.$refs.installprogressbar.update();
      let self = this;
      await axios.put(`/api/backup_refactor`, {
          "filename": this.backupFile
      }).then(() => {
        self.backupProgressTimer = setInterval(self.checkBackupProgress, 1000);
      });
    },
    async installSelected(){
      this.setTimeStarted({name: 'install', time: Date.now()});
      this.setProgress({name: 'install', progress: 0});
      this.$refs.installprogressbar.update();
      let self = this;
      await axios.put(`/api/install_refactor`, {
          "filename": this.selectedLocalImage
      }).then(() => {
        self.installProgressTimer = setInterval(self.checkInstallProgress, 1000);
      });
    },
    async cancelInstall(){
      let self = this;
      await axios.put(`/api/cancel_installation`, {
          "filename": this.selectedLocalImage
      }).then(() => {
        clearInterval(self.installProgressTimer);
      });
    },
    async checkInstallProgress() {
      const response = await axios.get(`/api/get_install_progress`);
      let data = response.data
      this.setProgress({name: 'install', progress: data.progress*100});
      this.$refs.installprogressbar.update();
      if(data.error){
        console.log(data.error);
        clearInterval(this.installProgressTimer);
        this.isInstalling = false;
        this.setVisible({name: 'install', visible: false});
      }
      else if(data.is_finished){
        clearInterval(this.installProgressTimer);
        this.isInstalling = false;
        this.setVisible({name: 'install', visible: false});
        if(this.options.enableSsh){
          this.enableSsh();
        }
        if(this.options.rebootWhenDone){
          this.rebootBoard();
        }
        else{
          this.installFinished = true;
        }
      }
    },
    async checkBackupProgress(){
      const response = await axios.get(`/api/get_backup_progress`);
      let data = response.data
      this.setProgress({name: 'install', progress: data.progress*100});
      this.$refs.installprogressbar.update();
      if(data.error){
        clearInterval(this.backupProgressTimer);
        this.isInstalling = false;
        this.setVisible({name: 'install', visible: false});
      }
      else if(data.is_finished){
        clearInterval(this.backupProgressTimer);
        this.isInstalling = false;
        this.setVisible({name: 'install', visible: false});
      }
      else if(data.state == "CANCELLED"){
        clearInterval(this.backupProgressTimer);
        this.isInstalling = false;
        this.setVisible({name: 'install', visible: false});
      }
    },
    rebootBoard(){
      this.showOverlay = true;
      axios.put(`/api/reboot_board`);
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
    async getData(){
      const response = await axios.get(`/api/get_data`);
      let data = response.data
      this.localImages = data.locals;
      if(data.download_progress.state === "DOWNLOADING"){
        this.downloadProgressTimer = setInterval(this.getDownloadProgress, 1000);
        this.setVisible({name: 'transfer', visible: true});
        this.isTransferring = true;
      }
      if(data.install_progress.state == "INSTALLING"){
        this.installProgressTimer = setInterval(this.checkInstallProgress, 1000);
        this.isInstalling = true;
      }
    },
    async runCommand(command, params, on_success){
      await fetch(`api/run_command`, {
        method: 'POST',
        headers: {
          'Content-type': 'application/json',
        },
        body: JSON.stringify({
          "command": command,
          ...params
        })
      }).then(response => response.json())
      .then(data => (on_success(data)));
    }
  },
  created(){
      this.selectedMethod = this.availableMethods[0]
    fetch("https://api.github.com/repos/intelligent-agent/Refactor/releases")
      .then(response => response.json())
      .then(data => (this.populateImages(data)));
    this.getData();
  },
  computed: mapGetters(['options', 'progress', 'flash']),
}
</script>

<style>
#app {
  font-family: Avenir, Helvetica, Arial, sans-serif;
  -webkit-font-smoothing: antialiased;
  -moz-osx-font-smoothing: grayscale;
  text-align: center;
  color: #2c3e50;
  margin-top: 60px;
}
.w-select--no-padding .w-select__selection {
  font-weight: bold;
  text-align: center;
  font-size:  16px;
  color: #444;
}

.card {
  width: 70%;
}

.w-app .primary {
    color: #234781;
}

.w-app .primary--bg {
  color: #DDD;
  background-color: #234781;
}

.w-app .secondary {
  color: #444;
}

.w-card {
  border: 1px solid #444;
}

.w-select__selection-wrap {
  border-color: #444;
}

.w-select__selection {
  color: #444;
}
.w-input--floating-label .w-input__input-wrap{
  margin: 0;
}
</style>
