<template>
  <div>
    <!-- The bar owns the hover interaction, so the popup is anchored to the
         thing it annotates rather than to the page. -->
    <div
      class="bar-hover"
      @mouseenter="onEnter"
      @mousemove="onMove"
      @mouseleave="onLeave"
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
      <div class="align-self-center">{{bandwidth}} MB/s<span v-if="history.length > 1" class="peak"> (peak {{peak}})</span></div>
      <div class="align-self-end">{{minutesR}}m:{{secondsR}}s</div>
    </w-flex>

    <!-- Throughput history. The MB/s figure above is instantaneous (bytes moved
         between two polls) and therefore jumpy, which makes a genuine stall hard
         to tell apart from normal jitter. This keeps the last few minutes so a
         flat stretch is obvious while it is happening.

         Out of the layout and into a popup (#122): inline it overhung the
         progress bar, collided with the timing line under it, and was too narrow
         to read - and it was on screen permanently even though the numbers
         beside it already answer the usual question. Opt-in suits it better:
         whoever wants the shape of the transfer goes looking for it.

         position: fixed so it cannot be clipped by an ancestor's overflow. -->
    <div
      v-if="plotVisible"
      class="speed-popup"
      :style="{ left: popupLeft + 'px', top: popupTop + 'px' }">
      <div class="speed-popup-title">
        throughput <span class="peak">peak {{peak}} MB/s</span>
      </div>
      <svg
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
      <div v-if="pinned" class="speed-popup-hint">tap the bar again to hide</div>
    </div>
  </div>
</template>

<script>
import { mapGetters } from 'vuex';

// At roughly one sample per poll (~1s) this is a few minutes of history,
// which is the timescale the eMMC-backpressure stalls play out over.
const MAX_SAMPLES = 180;

// Popup geometry, in px. Wide enough that the trace has a shape rather than a
// few spikes, which was the main complaint about the inline version.
const POPUP_W = 280;
const POPUP_H = 96;
const CURSOR_GAP = 14;

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
    // Nothing to show until there are two points to draw a line between.
    plotVisible: function() {
      return (this.hovering || this.pinned) && this.history.length > 1;
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
    hovering: false,
    pinned: false,
    popupLeft: 0,
    popupTop: 0,
    MAX_SAMPLES
  }),
  methods: {
    reset: function() {
      this.history = [];
      this.hovering = false;
      this.pinned = false;
    },
    // Follow the cursor on X only, and sit at a fixed height above the bar.
    // Following both axes reads as a tooltip but makes the trace bob while you
    // are trying to read it; pinning Y to the bar keeps it steady and still
    // tracks the pointer in the direction that matters.
    place: function(evt) {
      const bar = evt.currentTarget.getBoundingClientRect();
      const maxLeft = window.innerWidth - POPUP_W - 8;
      // Flip rather than overflow at the right edge.
      this.popupLeft = Math.max(8, Math.min(evt.clientX - POPUP_W / 2, maxLeft));
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
    // desktop browser, so tapping the bar pins the popup open - otherwise the
    // plot would simply not exist on the panel (#122).
    togglePin: function(evt) {
      this.pinned = !this.pinned;
      if (this.pinned) {
        this.place(evt);
      }
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
/* Give the pointer something with height to be inside: the bar itself is 1em
   and easy to slip off while moving along it. */
.bar-hover {
  padding: 0.35em 0;
  cursor: crosshair;
}
.speed-popup {
  position: fixed;
  width: 280px;
  height: 96px;
  z-index: 1000;
  padding: 0.4em 0.6em;
  border-radius: 6px;
  background: rgba(20, 20, 20, 0.92);
  color: #eee;
  box-shadow: 0 2px 10px rgba(0, 0, 0, 0.45);
  pointer-events: none;
}
.speed-popup-title {
  font-size: 0.75em;
  opacity: 0.8;
  margin-bottom: 0.2em;
}
.speed-popup-hint {
  font-size: 0.65em;
  opacity: 0.5;
  text-align: center;
}
.sparkline {
  width: 100%;
  height: 3.2em;
  display: block;
}
.peak {
  opacity: 0.6;
  font-size: 0.85em;
}
</style>
