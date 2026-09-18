<template>
  <div class="metrics-hover" @mouseleave="onLeave">
    <!-- The bar owns the hover interaction, so the popup is anchored to the
         thing it annotates rather than to the page. -->
    <div
      ref="bar"
      class="bar-hover"
      @mouseenter="onEnter"
      @mousemove="onMove"
      @click="togglePin">
      <w-progress
        :model-value="progress.progress"
        size="1em"
        outline
        round
        color="light-blue"
        stripes>
      </w-progress>
    </div>

    <w-flex justify-space-between class="wrapper">
      <div class="align-self-start">{{minutes}}m:{{seconds}}s</div>
      <!-- Instantaneous only. The hover panel holds the historical board
           readings, leaving this timing line to answer the usual at-a-glance
           transfer question. -->
      <div class="align-self-center">{{bandwidth}} MB/s</div>
      <div class="align-self-end">{{minutesR}}m:{{secondsR}}s</div>
    </w-flex>

    <!-- The same hover that previously exposed a throughput trace now exposes
         the complete board state. It is a descendant of .metrics-hover, so the
         popup stays open while the pointer moves from the bar onto a graph. -->
    <div
      v-if="metricsVisible"
      class="metrics-popup"
      :style="{ left: popupLeft + 'px', top: popupTop + 'px' }">
      <div class="metrics-popup-title">Board metrics</div>
      <TheMetrics :active="metricsVisible" :revision="revision" />
      <div v-if="pinned" class="metrics-popup-hint">tap the bar again to hide</div>
    </div>
  </div>
</template>

<script>
import { mapGetters } from 'vuex';
import TheMetrics from './TheMetrics.vue';

// Popup geometry, in px. This matches the former metrics dialog width while
// keeping the panel inside a small display and above or below the transfer bar.
const POPUP_W = 820;
const POPUP_H = 420;
const CURSOR_GAP = 14;

export default {
  name: 'ProgressBar',
  components: { TheMetrics },
  props: { revision: String },
  computed: {
    ...mapGetters(['progress']),
    metricsVisible: function() {
      return this.hovering || this.pinned;
    }
  },
  data: () => ({
    seconds: 0,
    minutes: 0,
    secondsR: 0,
    minutesR: 0,
    bandwidth: 0,
    hovering: false,
    pinned: false,
    popupLeft: 0,
    popupTop: 0,
  }),
  methods: {
    reset: function() {
      this.hovering = false;
      this.pinned = false;
    },
    // Follow the cursor on X only, and sit at a fixed height above the bar.
    // Pinning Y to the bar keeps the larger panel steady while it is read.
    place: function(evt) {
      const bar = this.$refs.bar.getBoundingClientRect();
      const width = Math.min(POPUP_W, window.innerWidth - 16);
      const maxLeft = window.innerWidth - width - 8;
      // Flip rather than overflow at the right edge.
      this.popupLeft = Math.max(8, Math.min(evt.clientX - width / 2, maxLeft));
      // Above the bar by default; below it if there is no room above.
      const above = bar.top - POPUP_H - CURSOR_GAP;
      this.popupTop = above >= 8 ? above : bar.bottom + CURSOR_GAP;
    },
    onEnter: function(evt) {
      this.hovering = true;
      this.place(evt);
    },
    onMove: function(evt) {
      if (this.hovering) {
        this.place(evt);
      }
    },
    onLeave: function() {
      this.hovering = false;
    },
    // Touch has no hover. The board is used from a touchscreen as well as a
    // desktop browser, so tapping the bar pins the metrics panel open.
    togglePin: function(evt) {
      this.pinned = !this.pinned;
      if (this.pinned) {
        this.place(evt);
      }
    },
    update: function() {
      let model = this.progress;
      // Clamped: a start time in the future (the board and the browser do not
      // share a clock) otherwise runs the elapsed count backwards too.
      let timePassedSeconds = Math.max(0, (Date.now() - model.timeStarted)/1000);
      this.seconds = Math.floor(timePassedSeconds % 60) ;
      this.minutes = Math.floor(timePassedSeconds / (60));
      let progress = model.progress/100;
      this.bandwidth = model.bandwidth.toFixed(1);

      let secondsTotal = (timePassedSeconds/progress);
      let timeFinished = new Date(new Date(model.timeStarted).getTime() + secondsTotal*1000);
      // Clamped before the split, so neither part can carry the sign:
      // Math.floor(-4 % 60) is -4, which is how "0m:-4s" reached the screen.
      let timeRemaining = Math.max(0, (timeFinished - Date.now())/1000);
      this.secondsR = Math.floor(timeRemaining % 60);
      this.minutesR = Math.floor(timeRemaining / 60);
      if(isNaN(this.secondsR)){
        this.secondsR = 0
        this.minutesR = 0
      }
    }
  }
}
</script>

<style scoped>
/* Give the pointer something with height to be inside: the bar itself is 1em
   and easy to slip off while moving along it. */
.bar-hover {
  padding: 0.35em 0;
  cursor: crosshair;
}
/* The three figures under the bar are spread by justify-space-between, which
   leaves no space at all once the row is narrow: elapsed, rate and remaining
   ran together as "3.7 MB/s3m:13s". A gap keeps them apart, and nowrap stops a
   single figure being broken across lines (#163). */
.wrapper {
  gap: 0.6em;
  white-space: nowrap;
}
.metrics-popup {
  position: fixed;
  width: min(820px, calc(100vw - 16px));
  max-height: min(420px, calc(100vh - 16px));
  overflow-y: auto;
  z-index: 1000;
  padding: 0.4em 0.6em;
  border-radius: 6px;
  /* Opaque. At 0.92 the page behind showed through the panel - the REFLASH
     wordmark and the version line ran underneath the plots, which is exactly
     where the numbers are read from (#163). */
  background: #141414;
  color: #eee;
  box-shadow: 0 2px 10px rgba(0, 0, 0, 0.45);
  pointer-events: auto;
}
.metrics-popup-title {
  font-size: 0.75em;
  opacity: 0.8;
  margin-bottom: 0.2em;
}
.metrics-popup-hint {
  font-size: 0.65em;
  opacity: 0.5;
  text-align: center;
}
</style>
