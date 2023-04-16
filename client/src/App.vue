<template>
  <w-app>
    <w-card  class="mxa pa3 card secondary" >
      <w-flex wrap class="text-center">
        <div class="xs5 pa1">
          <img style="width: 40px; height: 40px" :src="computeImage('logo-thing')" />
          <h3>REFLASH</h3>
          <TheLogger :log="theLog"/>
          <w-button @click="openInfo = !openInfo" text>
            <w-icon md>fa fa-info-circle</w-icon>
          </w-button>
          <TheOptions
            @set-option="setOption"
            @reboot-board="rebootBoard"
            @shutdown-board="shutdownBoard"/>
        </div>
        <div class="xs5 pa4">
         <TheInfo :open="openInfo" :version="version" />
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
        <div class="xs1 pa1 align-self-center"><FlashSelector ref="flashSelector"/></div>
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
        <div class="xs1 pa1"><b v-if="flash.selectedMethod == 1">Backup Filename</b></div>

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
          <w-button @click="onTransferButtonClick()" v-if="this.isTransferButtonVisible()">{{this.computeTransferButtonText()}}</w-button>
        </div>
        <w-flex class="xs1 align-self-center flex justify-start">
          <w-select
            v-model="selectedLocalImage"
            :items="localImages"
            item-label-key="name"
            placeholder="Please select one"
            :item-click="onSelectedFileChanged()"
            outline>
          </w-select>
          <IntegrityChecker ref="integritychecker"/>
        </w-flex>
        <div class="xs1 align-self-center">
          <w-button @click="onInstallButtonClick()" v-if="isInstallButtonVisibile()">{{this.installButtonText()}}</w-button>
        </div>
        <div class="xs1">
          <w-input v-model="backupFile" v-if="flash.selectedMethod == 1" outline>Label</w-input>
        </div>

        <div class="xs5">
          <div v-if="installFinished && !showOverlay">
            <w-transition-expand y>
              <w-alert>
                Install finished! Please press the reboot button.
              </w-alert>
            </w-transition-expand>
            <w-button  class="ma1 btn" @click="rebootBoard()">Reboot Now</w-button>
          </div>
          <div v-if="showOverlay">
            <w-transition-expand y>
              <w-alert v-if="showOverlay">
                Please wait while board is rebooting
              </w-alert>
            </w-transition-expand>
            <w-progress class="ma1" circle></w-progress><br>
            <w-button @click="isServerUp()" v-if="showOverlay">Check server</w-button>
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
    backupFile: "",
    version: "",
    theLog: ""
  }),
  computed: mapGetters(['options', 'progress', 'flash']),
  methods: {
    ...mapActions([
      'setProgress',
      'setVisible',
      'setFlashMethod',
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
      return this.flash.selectedMethod == 0 ? 'left' : 'right';
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
        return this.selectedUploadImage.length > 0;
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
    onSelectedFileChanged(){
      if(this.$refs.integritychecker){
        this.$refs.integritychecker.fileSelected(this.selectedLocalImage);
      }
    },
    onFileInput(files){
      this.files = files
      this.file = files[0].file;
    },
    uploadLocalFile(){
      const CHUNK_SIZE = 3*1024*1024;
      let self = this;
      var reader = new FileReader();
      var offset = 0;
      var filesize = this.file.size;
      this.setTimeStarted({name: 'transfer', time: Date.now()});

      reader.onload = function(){
          var result = reader.result;
          var chunk = result;
          axios.post(`/api/upload_chunk`, {
             "chunk": chunk,
             "filename": self.fileName,
             "is_new_file": offset == 0
           }).then(function(response) {
            const status = response.data
            if(status.success && self.isTransferring){
              self.$refs.transferprogressbar.update();
              offset += CHUNK_SIZE;
              if(offset <= filesize){
                var slice = self.file.slice(offset, offset + CHUNK_SIZE);
                reader.readAsDataURL(slice);
                self.transferProgress = Math.round((offset/filesize)*100);
                self.setProgress({name: 'transfer', progress: self.transferProgress });
                self.setVisible({name: 'transfer', visible: true});
              }
              else{
                offset = filesize;
                self.transferProgress = 100;
                self.isTransferring = false;
                self.setVisible({name: 'transfer', visible: false});
                self.getLocalImages();
                self.selectedLocalImage = self.selectedUploadImage[0].name.slice(0, -7);
                self.selectedUploadImage = []
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
      if(this.selectedMethod.id == 0){
        this.downloadSelected();
      }
      else if(this.selectedMethod.id == 1){
        this.uploadLocalFile();
      }
    },
    async downloadSelected(){
      let self = this;
      if(this.isTransferring){
        await axios.put(`/api/download_refactor`, {
          "filename": this.selectedGithubImage, 
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
      const response = await axios.get(`/api/get_download_progress`);
      let data = response.data

      if(data.state == "DOWNLOADING"){
        this.isTransferring = true;
        this.setProgress({name: 'transfer', progress: data.progress});
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
          console.log(data.error);
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
      const response = await axios.get(`/api/get_install_progress`);
      let data = response.data
      this.theLog = data.log
      if(data.state == "INSTALLING"){
        this.isInstalling = true;
        this.setVisible({name: 'install', visible: true});
        this.setProgress({name: 'install', progress: data.progress});
        this.setTimeStarted({name: 'install', time: data.start_time});
        this.selectedLocalImage = data.filename
        this.$refs.installprogressbar.update();
        setTimeout(this.checkInstallProgress, 1000);
      }
      else{
        this.isInstalling = false;
        this.setVisible({name: 'install', visible: false});
        if(data.state == "ERROR"){
          console.log(data.error);
        }
        else if(data.state == "FINISHED"){
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
      const response = await axios.get(`/api/get_backup_progress`);
      let data = response.data
      this.theLog = data.log
      if(data.state == "INSTALLING"){
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
          this.theLog = data.log+"\n\n"+data.error
        }
      }
    },
    rebootBoard(){
      this.showOverlay = true;
      axios.put(`/api/reboot_board`);
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
      const response = await axios.get(`/api/get_local_images`);
      let data = response.data
      this.localImages = data.locals;
      this.version = response.data.reflash_version;
    },
    async getGithubImages(){
      fetch("https://api.github.com/repos/intelligent-agent/Refactor/releases")
      .then(response => response.json())
      .then(data => (this.populateImages(data)));
    }
  },
  created(){
    this.selectedMethod = this.availableMethods[0]
    this.getGithubImages();
    this.getLocalImages();
    this.checkDownloadProgress();
    this.checkInstallProgress();
    this.checkBackupProgress();
  },
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
