<template>
  <div>
    <w-progress
      :model-value="progress.progress"
      size="1em"
      outline
      round
      color="light-blue"
      stripes>
    </w-progress>

    <!-- Throughput history. The single MB/s figure below is instantaneous
         (bytes moved between two polls) and therefore jumpy, which makes a
         genuine stall hard to tell apart from normal jitter. The chart keeps
         the last few minutes so a flat stretch is obvious while it happens. -->
    <svg
      v-if="history.length > 1"
      class="sparkline"
      :viewBox="'0 0 ' + MAX_SAMPLES + ' 24'"
      preserveAspectRatio="none">
      <polygon :points="areaPoints" fill="rgba(3, 169, 244, 0.25)" />
      <polyline
        :points="linePoints"
        fill="none"
        stroke="#03a9f4"
        stroke-width="0.7"
        vector-effect="non-scaling-stroke" />
    </svg>

    <w-flex justify-space-between class="wrapper">
      <div class="align-self-start">{{minutes}}m:{{seconds}}s</div>
      <div class="align-self-center">{{bandwidth}} MB/s<span v-if="history.length > 1" class="peak"> (peak {{peak}})</span></div>
      <div class="align-self-end">{{minutesR}}m:{{secondsR}}s</div>
    </w-flex>
  </div>
</template>

<script>
import { mapGetters } from 'vuex';

// At roughly one sample per poll (~1s) this is a few minutes of history,
// which is the timescale the eMMC-backpressure stalls play out over.
const MAX_SAMPLES = 180;

export default {
  name: 'ProgressBar',
  computed: {
    ...mapGetters(['progress']),
    // Scale to the tallest sample so the trace uses the full height, with a
    // floor so that an all-zero history renders flat along the bottom
    // instead of dividing by zero.
    scale: function() {
      return Math.max(...this.history, 0.1);
    },
    peak: function() {
      return Math.max(...this.history, 0).toFixed(1);
    },
    linePoints: function() {
      // Right-aligned, so the newest sample sits at the right edge and the
      // trace grows leftwards instead of stretching as history fills up.
      const offset = MAX_SAMPLES - this.history.length;
      return this.history
        .map((v, i) => (offset + i) + ',' + (24 - (v / this.scale) * 23).toFixed(2))
        .join(' ');
    },
    areaPoints: function() {
      const offset = MAX_SAMPLES - this.history.length;
      return offset + ',24 ' + this.linePoints + ' ' + MAX_SAMPLES + ',24';
    }
  },
  data: () => ({
    seconds: 0,
    minutes: 0,
    secondsR: 0,
    minutesR: 0,
    bandwidth: 0,
    history: [],
    MAX_SAMPLES
  }),
  methods: {
    reset: function() {
      this.history = [];
    },
    update: function() {
      let model = this.progress;
      let timePassedSeconds = (Date.now() - model.timeStarted)/1000;
      this.seconds = Math.floor(timePassedSeconds % 60) ;
      this.minutes = Math.floor(timePassedSeconds / (60));
      let progress = model.progress/100;
      this.bandwidth = model.bandwidth.toFixed(1);

      // Clamp: the server derives bandwidth from a byte-count delta, which
      // goes negative whenever the counter restarts (a new operation, or the
      // magic path switching from upload bytes to flash bytes).
      let sample = model.bandwidth;
      if (!isFinite(sample) || sample < 0) {
        sample = 0;
      }
      this.history.push(sample);
      if (this.history.length > MAX_SAMPLES) {
        this.history.shift();
      }

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

<style scoped>
.sparkline {
  width: 100%;
  height: 2.5em;
  display: block;
  margin-top: 0.3em;
}
.peak {
  opacity: 0.6;
  font-size: 0.85em;
}
</style>
