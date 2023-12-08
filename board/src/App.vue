<template>
  <w-app>
    <TheLogger :log="theLog" :open="openLog" @close="openLog = false" />
    <TheOptions
      :open="openOptions"
      @set-option="setOption"
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
          <TheInfo :open="openInfo" :version="reflash_version" :revision="recore_revision"/>
        </div>
        <div class="xs5">Boot media</div>
        <div class="xs2 pa1 align-self-center">USB drive</div>
        <div class="xs1 pa1 align-self-center"></div>
        <div class="xs2 pa1 align-self-center">eMMC</div>

        <div class="xs2 pa1 align-self-center">
          <img style="width: 30%" :src="computeSVG('USB')" />
        </div>
        <div class="xs1 pa1 align-self-center">
          <div>{{ bootMedia }}</div>
          <img style="width: 50%" :src="computeSVG('Arrow-left-right')" />
        </div>
        <div class="xs2 pa1 align-self-center">
          <img style="width: 30%" :src="computeSVG('eMMC')" />
        </div>

        <div class="xs2 pa1">
          <w-button
            xl
            outline
            @click="changeBootMedia('usb')"
            :disabled="isBootFromUSBEnabled()"
          >
            <span>Boot from USB</span>
          </w-button>
        </div>
        <div class="xs1 pa1"></div>
        <div class="xs2 pa1">
          <w-button
            xl
            outline
            @click="changeBootMedia('emmc')"
            :disabled="isBootFromEMMCEnabled()"
          >
            <span>Boot from eMMC</span>
          </w-button>
        </div>

        <div class="xs5 pa1 my3"></div>

        <div class="xs5 mt10 mb2">SSH access</div>
        <div class="xs5">
          Disabled
          <w-switch
            @change="enableSsh(options.enableSsh)"
            v-model="options.enableSsh"
            class="mx6"
          >
          </w-switch>
          Enabled
        </div>

        <div class="xs5 mt10 mb2">Screen rotation</div>
        <div class="xs5">
          <w-radios
            @change="rotateScreen(options.screenRotation)"
            v-model="options.screenRotation"
            :items="radioItems"
            inline
            label="Screen rotation"
            style="align-self: center"
          >
          </w-radios>
        </div>
        <div class="xs5 mt10"></div>
        <div class="xs5 pa1">
          <w-button xl outline class="ma1 btn" @click="rebootBoard()"
            ><span>Reboot Now</span></w-button
          >
          <w-button xl outline class="ma1 btn" @click="shutdownBoard()"
            ><span>Shut Down</span></w-button
          >
          <w-button xl outline class="ma1 btn" @click="returnToApp()"
            ><span>Return to Mainsail</span></w-button
          >
        </div>
        <div class="xs5 pa1" v-if="showOverlay">
          <w-transition-expand y>
            <w-alert v-if="showOverlay">
              Please wait while board is rebooting
            </w-alert>
          </w-transition-expand>
          <w-progress class="ma1" circle></w-progress><br />
          <w-button xl outline @click="isServerUp()" v-if="showOverlay"
            ><span>Check server</span></w-button
          >
        </div>
      </w-flex>
    </w-card>
  </w-app>
</template>

<script>
import TheOptions from "./components/TheOptions";
import TheLogger from "./components/TheLogger";
import TheInfo from "./components/TheInfo";
import WaveUI from "wave-ui";
import { mapGetters, mapActions } from "vuex";
import axios from "axios";

export default {
  name: "App",
  components: {
    TheOptions,
    TheLogger,
    TheInfo,
  },
  setup() {
    const waveui = new WaveUI(this, {});
    return { waveui };
  },
  data: () => ({
    openInfo: false,
    openLog: false,
    openOptions: false,
    showOverlay: false,
    imageColor: "white",
    reflash_version: "Unknown",
    recore_revision: "Unknown",
    emmc_version: "Unknown",
    theLog: "",
    bootMedia: "unknown",
    radioItems: [
      { label: "Normal", value: 0 },
      { label: "90 degrees", value: 90 },
      { label: "180 degrees", value: 180 },
      { label: "270 degrees", value: 270 },
    ],
  }),
  computed: mapGetters(["options"]),
  methods: {
    ...mapActions([
      'getOptions',
      'setBootMedia',
    ]),
    computeSVG(name) {
      return require("./assets/" + name + "-" + this.imageColor + ".svg");
    },
    setTheme(darkmode) {
      this.imageColor = darkmode ? "white" : "black";
      if (darkmode) {
        if (this.dark.parentElement == null) {
          document.head.appendChild(this.dark);
        }
      } else {
        if (this.dark.parentElement === document.head) {
          document.head.removeChild(this.dark);
        }
      }
    },
    rebootBoard() {
      this.showOverlay = true;
      axios.put(`/api/reboot_board`);
    },
    shutdownBoard() {
      axios.put(`/api/shutdown_board`);
    },
    returnToApp() {
      window.location.href = "http://" + window.location.hostname + ":80";
    },
    enableSsh(ssh_is_enabled) {
      axios.put(`/api/set_ssh_enabled`, { is_enabled: ssh_is_enabled, media: "emmc"});
    },
    rotateScreen(rot) {
      axios.put(`/api/rotate_screen`, { rotation: rot, where: "FBCON" });
    },
    async changeBootMedia(value) {
      var self = this;
      await axios.put(`/api/set_boot_media`, { media: value }).then(function() {
        axios.get(`/api/get_boot_media`, ).then(function(response){
          self.bootMedia = response.data.boot_media;
        })
      });
    },
    isBootFromUSBEnabled() {
      return this.bootMedia == "usb";
    },
    isBootFromEMMCEnabled() {
      return this.bootMedia == "emmc";
    },
    isServerUp() {
      fetch(`/favicon.ico`).then((response) => {
        if (response.status == 200) {
          location.reload();
        }
        return response.status == 200;
      });
    },
    setOption(opt, value) {
      if (opt == "darkmode") {
        if (!this.dark) {
          this.dark = document.createElement("link");
          this.dark.rel = "stylesheet";
          this.dark.href = "/darkmode.css";
        }
        this.setTheme(value);
      }
    },
  },
  created(){
    var self = this;
    axios.get(`/api/get_boot_media`, ).then(function(response){
      self.bootMedia = response.data.boot_media;
    });
    axios.get(`/api/get_info`, ).then(function(response){
      self.recore_revision = response.data.recore_revision;
      self.reflash_version = response.data.reflash_version;
    });
  }
};
</script>

<style>
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
  background-color: #f1f1f1;
  font-family: "Roboto";
  font-style: normal;
  font-weight: 300;
}
#app {
  font-family: Avenir, Helvetica, Arial, sans-serif;
  -webkit-font-smoothing: antialiased;
  -moz-osx-font-smoothing: grayscale;
  text-align: center;
  color: #4d4d4d;
}
.w-select--no-padding .w-select__selection {
  text-align: center;
  font-size: 16px;
  color: #444;
}

.card {
  width: 70%;
  margin-top: 60px;
}

.w-app .primary--bg {
  color: #ddd;
  background-color: #04a3e5;
}

.w-app .secondary {
  color: #444;
}

.w-card {
  border: none;
}

.w-select__selection-wrap {
  border-color: #4d4d4d;
}

.w-select__selection {
  color: #4d4d4d;
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
.w-app .primary {
  color: #292a2c;
}
.w-button.size--xl {
  color: #04a3e5;
}
.w-button.size--xl span {
  color: #4d4d4d;
}
</style>
