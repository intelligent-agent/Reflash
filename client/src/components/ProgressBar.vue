<template>
  <div v-if="progress[name].visible">
    <w-progress
      :model-value="progress[name].progress"
      size="1em"
      outline
      round
      color="light-blue"
      stripes>
    </w-progress>
    <w-flex justify-space-between class="wrapper">
      <div class="align-self-start">{{minutes}}m:{{seconds}}s</div>
      <div class="align-self-end">{{minutesR}}m:{{secondsR}}s</div>
    </w-flex>
  </div>
</template>

<script>
import { mapGetters } from 'vuex';

export default {
  name: 'ProgressBar',
  props: ['name'],
  computed: {
    ...mapGetters(['progress'])
  },
  data: () => ({
    seconds: 0,
    minutes: 0,
    secondsR: 0,
    minutesR: 0
  }),
  methods: {
    update: function() {
      let model = this.progress[this.name];
      let timePassedSeconds = (Date.now() - model.timeStarted)/1000;
      this.seconds = Math.floor(timePassedSeconds % 60) ;
      this.minutes = Math.floor(timePassedSeconds / (60));
      let progress = model.progress/100;
      let secondsTotal = (timePassedSeconds/progress);
      let timeFinished = new Date(new Date(model.timeStarted).getTime() + secondsTotal*1000);
      let timeRemaining = (timeFinished - Date.now())/1000;
      this.secondsR = Math.floor(timeRemaining % 60);
      this.minutesR = Math.floor(timeRemaining / 60);
      if(isNaN(this.secondsR) || this.seconds == -1){
        this.secondsR = 0
        this.minutesR = 0
      }
    }
  }
}
</script>
