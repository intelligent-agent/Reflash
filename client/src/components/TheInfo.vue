<template>
  <w-transition-expand y>
      <p v-if="open" class="text-left">
        This page is running from a USB drive on Recore. You can use this page to download and
        install distros to the eMMC of Recore.
        <ol>
          <li>Start by downloading an image, probably the latest version. </li>
          <li>Once downloaded, you can flash the image to the internal storage (eMMC)</li>
          <li>Finally reboot the board. The board will boot from the internal storage.</li>
        </ol>
        <span>Reflash version: {{version}}</span><br />
        <span>Recore revision: {{revision}}</span><br />
        <span>Recore serial number: {{serialNumber}}</span>
        <template v-for="line in networkLines" :key="line.text">
          <br /><span>Network: {{line.text}}</span
          ><span v-if="line.bars" class="signal" :title="line.rssi + ' dBm'">
            <i v-for="n in 4" :key="n" :class="{ on: n <= line.bars }" :style="barStyle(n)"></i>
          </span
          ><span
            v-if="line.hotspot"
            class="badge"
            title="Clients connect to this board - there is no internet, so downloads will fail"
            >hotspot</span
          ><span v-if="line.active" class="badge" title="Carries traffic (default route)">active</span>
        </template>
        <!-- Lives here rather than in the icon row: the metrics are a detail
             about this board, like the revision and serial number above, not a
             fourth top-level action beside Log, Info and Options. -->
        <br />
        <w-button class="metrics-link" @click="$emit('open-metrics')" text sm>
          Board metrics&hellip;
        </w-button>
      </p>
  </w-transition-expand>
</template>

<script>
import { networkLines } from "../network";

export default {
  name: "TheInfo",
  emits: ["open-metrics"],
  props: {
    open: Boolean,
    version: String,
    revision: String,
    serialNumber: String,
    network: Object,
  },
  computed: {
    networkLines() {
      return networkLines(this.network);
    },
  },
  methods: {
    // Rising heights, so the shape reads as strength even before colour.
    barStyle(n) {
      return { height: 3 + n * 2 + "px" };
    },
  },
};
</script>

<style scoped>
.signal {
  display: inline-flex;
  align-items: flex-end;
  gap: 1px;
  margin-left: 6px;
  height: 11px;
  vertical-align: -1px;
}
.signal i {
  width: 3px;
  /* Unfilled bars stay visible so the reading is "2 of 4", not "2". */
  background: currentColor;
  opacity: 0.25;
}
.signal i.on {
  opacity: 1;
}
/* Left-aligned with the lines above it, so it reads as part of the list rather
   than as a floating control. */
.metrics-link {
  margin: 6px 0 0 -8px;
  text-transform: none;
}
.badge {
  margin-left: 6px;
  padding: 0 5px;
  border: 1px solid currentColor;
  border-radius: 8px;
  font-size: 0.75em;
  opacity: 0.8;
}
</style>
