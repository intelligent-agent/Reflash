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
        <div class="xs1 pa1 align-self-center"><b>Flash</b></div>
        <div class="xs1 pa1 align-self-center"><b>eMMC</b></div>

        <div class="xs1 pa1 align-self-center"><img :src="computeImage(selectedMethod.image)" /></div>
        <div class="xs1 pa1 align-self-center"><img :src="computeImage('arrow-left')" /></div>
        <div class="xs1 pa1 align-self-center"><img :src="computeImage('USB')" /></div>
        <div class="xs1 pa1 align-self-center"><img :src="computeImage('arrow-left')" /></div>
        <div class="xs1 pa1 align-self-center"><img :src="computeImage('eMMC')" /></div>

        <div class="xs1 pa1"><b>Choose image to {{selectedMethod.id == 0 ? "Download" : "Upload"}}</b></div>
        <div class="xs1 pa1">
          <w-progress
            v-if="isTransferring"
            :model-value="transferProgress"
            size="1em"
            outline
            round
            color="light-blue"
            stripes>
          </w-progress>
        </div>
        <div class="xs1 pa1"><b>Choose image to install</b></div>
        <div class="xs1 pa1">
          <w-progress
            v-if="isInstalling"
            :model-value="installProgress"
            size="1em"
            outline
            round
            color="light-blue"
            stripes>
          </w-progress>
        </div>
        <div class="xs1 pa1"></div>

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
          <w-flex justify-space-between>
            <span class="pa3">{{isTransferring ? transferProgress.toFixed(0)+"%" : ""}}</span>
            <w-button @click="onTransferButtonClick()" v-if="this.computeTransferButtonVisible()">{{this.computeTransferButtonText()}}</w-button>
            <span class="pa3">{{ ""}}</span>
          </w-flex>
        </div>
        <div class="xs1">
          <w-select
            v-model="selectedLocalImage"
            :items="localImages"
            item-label-key="name"
            placeholder="Please select one"
            outline>
          </w-select>
        </div>
        <div class="xs1 align-self-center">
          <w-button @click="installSelected()" v-if="selectedLocalImage">{{installButtonText}}</w-button>
        </div>
        <div class="xs1"></div>

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
import WaveUI from 'wave-ui'

export default {
  name: 'App',
  components: {
    TheOptions
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
    options: {},
    availableMethods: [
      { id: 0, label: 'GitHub', value: 0, image: 'GitHub'},
      { id: 1, label: 'File upload', value: 1, image: 'File'}
    ],
    selectedMethod: 0,
    imageColor: "white",
    files: []
  }),
  methods: {
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

      reader.onload = function(){
          var result = reader.result;
          var chunk = result;
          self.runCommand("upload_chunk", {
             "chunk": chunk,
             "filename": self.fileName,
             "is_new_file": offset == 0
           }, function(status) {
            if(status.success){
              offset += CHUNK_SIZE;
              if(offset <= filesize){
                var slice = self.file.slice(offset, offset + CHUNK_SIZE);
                reader.readAsDataURL(slice);
                self.transferProgress = Math.round((offset/filesize)*100);
              }
              else{
                offset = filesize;
                self.transferProgress = 100;
                self.isTransferring = false;
                self.selectedUploadImage = [];
              }
            }
            else{
              self.uploadError = true;
              self.isTransferring = false;
              self.selectedUploadImage = [];
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
    downloadSelected(){
      let self = this;

      if(this.isTransferring){
        this.transferProgress = 0;
        this.runCommand("download_refactor", {
            "refactor_image": this.selectedGithubImage
        }, function() {
            self.transferProgressTimer = setInterval(self.checktransferProgress, 1000);
        });
      }
      else{
        this.runCommand("cancel_download", {}, function() {
            clearInterval(self.transferProgressTimer);
            this.isTransferring = false;
        });
      }
    },
    checktransferProgress() {
      let self = this;
      this.runCommand("get_download_progress", {}, function(data) {
        self.transferProgress = (data.progress*100);
        if(data.is_finished){
          clearInterval(self.transferProgressTimer);
          self.isTransferring = false;
        }
      })
    },
    checkInstallProgress() {
      let self = this;
      this.runCommand("get_install_progress", {}, function(data) {
        self.installProgress = (data.progress*100);
        if(data.is_finished){
          clearInterval(self.installProgressTimer);
          self.isInstalling = false;
          self.installButtonText = "Install";
          if(self.options['enableSsh']){
            self.enableSsh();
          }
          if(self.options['rebootWhenDone']){
            self.rebootBoard();
          }
          else{
            self.installFinished = true;
          }
        }
      })
    },
    installSelected(){
      let self = this;
      this.isInstalling = !this.isInstalling;
      this.installButtonText = this.isInstalling ? "Cancel" : "Install";
      if(this.isInstalling){
        this.installProgress = 0;
        this.runCommand("install_refactor", {
            "filename": this.selectedLocalImage
        }, function() {
          self.installProgressTimer = setInterval(self.checkInstallProgress, 1000);
        });
      }
      else{
        this.runCommand("cancel_installation", {}, function() {
            clearInterval(self.installProgressTimer);
        });
      }
    },
    rebootBoard(){
      this.showOverlay = true;
      this.runCommand("reboot_board", {}, function() {});
    },
    enableSsh(){
      this.runCommand("enable_ssh",{}, function() {});
    },
    isServerUp(){
      fetch(`api/favicon.ico`)
        .then(response => {
          if(response.status == 200){
            location.reload();
          }
          return response.status == 200;
        });
    },
    setOption(opt, value){
      if(opt == "darkmode"){
        this.setTheme(value);
      }
      this.options[opt] = value;
      this.runCommand("save_settings",{settings: this.options}, function() {});
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
    let self = this;
    self.runCommand("get_data", {}, function(data) {
      self.localImages = data.locals;
      for(let setting in data.settings){
        self.setOption(setting, data.settings[setting]);
      }

      if(data.download_progress.state == "DOWNLOADING"){
        self.transferProgressTimer = setInterval(self.checktransferProgress, 1000);
        self.isTransferring = true;
      }
      if(data.install_progress.state == "INSTALLING"){
        self.installProgressTimer = setInterval(self.checkInstallProgress, 1000);
        self.isInstalling = true;
      }
    });

  },
  mounted () {
    this.dark = document.createElement('link')
    this.dark.rel = 'stylesheet'
    this.dark.href = '/darkmode.css'
    this.setTheme(false);
  }
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
</style>