<template>
  <div class="metrics" v-if="samples.length">
    <div class="panel" :class="{ stat: p.stat }" v-for="p in panels" :key="p.key">
      <div class="head">
        <span class="label">{{ p.label }}</span>
        <span class="value" :class="p.statusClass" v-if="!p.stat">
          <template v-if="p.hoverValue !== null">{{ p.hoverValue }}</template>
          <template v-else-if="p.current !== null">{{ p.current }}</template>
          <template v-else>&mdash;</template>
          <span class="unit" v-if="p.current !== null">{{ p.unit }}</span>
        </span>
      </div>

      <!-- A rail sits at one voltage. A sparkline of a flat line says nothing a
           number does not, so this panel is the reading itself, large, with the
           expectation under it. -->
      <template v-if="p.stat">
        <div class="reading" :class="p.statusClass">
          <template v-if="p.current !== null">{{ p.current }}</template>
          <template v-else>&mdash;</template>
          <span class="unit" v-if="p.current !== null">{{ p.unit }}</span>
        </div>
        <div class="foot stat-foot">
          <span class="note" v-if="p.note">{{ p.note }}</span>
        </div>
      </template>

      <!-- preserveAspectRatio=none stretches x to the panel width; the stroke is
           kept at its literal width by vector-effect, so a wide panel does not
           produce a fat line. -->
      <svg
        v-if="!p.stat"
        class="spark"
        viewBox="0 0 100 30"
        preserveAspectRatio="none"
        @mousemove="onMove($event, p)"
        @mouseleave="hoverIndex = null"
      >
        <line
          v-if="p.referenceY !== null"
          class="reference"
          x1="0" x2="100" :y1="p.referenceY" :y2="p.referenceY"
          vector-effect="non-scaling-stroke"
        />
        <path v-if="p.area" class="area" :class="p.statusClass" :d="p.area" />
        <path v-if="p.line" class="line" :class="p.statusClass" :d="p.line" vector-effect="non-scaling-stroke" />
        <line
          v-if="hoverIndex !== null && p.hoverX !== null"
          class="crosshair"
          :x1="p.hoverX" :x2="p.hoverX" y1="0" y2="30"
          vector-effect="non-scaling-stroke"
        />
        <circle v-if="hoverIndex !== null && p.hoverX !== null && p.hoverY !== null"
                class="dot" :class="p.statusClass" :cx="p.hoverX" :cy="p.hoverY" r="2.5"
                vector-effect="non-scaling-stroke" />
      </svg>

      <div class="foot" v-if="!p.stat">
        <span>{{ p.min }}</span>
        <span class="note" v-if="p.note">{{ p.note }}</span>
        <span>{{ p.max }}</span>
      </div>
    </div>
    <p class="span">
      Last {{ spanLabel }}, sampled once a second.
      <span v-if="hoverIndex !== null">Reading at {{ hoverAgo }}.</span>
    </p>
  </div>
  <p v-else class="metrics-empty">Collecting board metrics&hellip;</p>
</template>

<script>
import axios from "axios";

// What the DRAM rail should be, by board revision. A5/A6 fit DDR3 parts with a
// 1.425V datasheet minimum; A7/A8/B0 fit DDR3L at 1.35V nominal. A board
// running the wrong one is the fault that motivated this panel, and it is
// invisible unless something says what the value ought to be.
const DRAM_EXPECTED = {
  a5: { volts: 1.5, label: "1.5V (DDR3)" },
  a6: { volts: 1.5, label: "1.5V (DDR3)" },
  a7: { volts: 1.36, label: "1.36V (DDR3L)" },
  a8: { volts: 1.36, label: "1.36V (DDR3L)" },
  b0: { volts: 1.36, label: "1.36V (DDR3L)" },
};

export default {
  name: "TheMetrics",
  props: {
    // Polling is driven by the parent's open state: there is no reason to ask a
    // board mid-flash for numbers nobody is looking at.
    active: Boolean,
    revision: String,
  },
  data() {
    return {
      samples: [],
      window: 300,
      hoverIndex: null,
      timer: null,
    };
  },
  computed: {
    latest() {
      return this.samples.length ? this.samples[this.samples.length - 1] : {};
    },
    spanLabel() {
      const n = this.samples.length;
      if (n < 90) return `${n} seconds`;
      return `${Math.round(n / 60)} minutes`;
    },
    hoverAgo() {
      if (this.hoverIndex === null || !this.samples[this.hoverIndex]) return "";
      const s = this.latest.t - this.samples[this.hoverIndex].t;
      return s < 1 ? "now" : `${s}s ago`;
    },
    // One panel per measure, each with its own y-scale. Deliberately not one
    // chart with several series: MB/s, °C, MHz and volts share no axis, and
    // overlaying them on a common scale would be meaningless.
    panels() {
      const dram = DRAM_EXPECTED[(this.revision || "").toLowerCase()];
      return [
        this.panel({
          key: "bandwidth", label: "Throughput", unit: "MB/s", digits: 1,
          note: "idle between flashes",
        }),
        this.panel({ key: "cpu_temp", label: "SoC temperature", unit: "°C", digits: 0,
          // The A64's own trip points, read off cpu0-thermal on an A8 rather
          // than guessed: passive (cpufreq capping starts) at 70, hot at 80,
          // critical at 90. Warning where throttling actually begins means the
          // panel agrees with the Thermal throttle panel beside it instead of
          // colouring first and explaining later.
          warnAbove: 70, criticalAbove: 80 }),
        this.panel({ key: "throttle", label: "Thermal throttle", unit: "/ 7", digits: 0,
          warnAbove: 0, floor: 0, ceiling: 7,
          note: "capping cpufreq for heat" }),
        this.panel({ key: "cpu_freq", label: "CPU frequency", unit: "MHz", digits: 0 }),
        this.panel({
          key: "vcc_dram", label: "DRAM rail", unit: "V", digits: 2,
          note: dram ? `expected ${dram.label}` : null,
          // Shown as a number, not a trace: a rail is a set point, and a
          // sparkline of a flat line is noise. What matters is the value and
          // whether it matches the fitted part, so the comparison is the panel.
          stat: true,
          criticalBelow: dram ? dram.volts - 0.01 : null,
        }),
        this.panel({ key: "dirty", label: "Writeback backlog", unit: "kB", digits: 0 }),
      ];
    },
  },
  watch: {
    active(on) {
      if (on) this.start();
      else this.stop();
    },
  },
  mounted() {
    if (this.active) this.start();
  },
  beforeUnmount() {
    this.stop();
  },
  methods: {
    start() {
      this.fetch();
      if (!this.timer) this.timer = setInterval(this.fetch, 2000);
    },
    stop() {
      clearInterval(this.timer);
      this.timer = null;
    },
    async fetch() {
      try {
        // "since" so a steady poll carries the two new samples rather than the
        // whole window - which would be tens of kB several times a minute for
        // data already held here.
        const since = this.samples.length ? this.latest.t : 0;
        const { data } = await axios.get(`/api/get_metrics?since=${since}`);
        this.window = data.window || this.window;
        if (data.samples && data.samples.length) {
          this.samples = this.samples.concat(data.samples).slice(-this.window);
        }
      } catch (e) {
        // A failed poll is not worth surfacing: the board is busy flashing, and
        // the panel simply stops advancing until the next one lands.
      }
    },
    onMove(evt, panel) {
      const box = evt.currentTarget.getBoundingClientRect();
      const frac = Math.min(Math.max((evt.clientX - box.left) / box.width, 0), 1);
      this.hoverIndex = Math.round(frac * (this.samples.length - 1));
      void panel;
    },
    // Builds one panel: the scaled path, the current reading, and whatever
    // status the value carries. Kept in one place so every panel gets the same
    // treatment of gaps, flat series and missing fields.
    panel(spec) {
      const vals = this.samples.map((s) =>
        s[spec.key] === undefined || s[spec.key] === null ? null : s[spec.key]
      );
      const present = vals.filter((v) => v !== null);
      const fmt = (v) =>
        v === null || v === undefined ? null : Number(v).toFixed(spec.digits);

      const out = {
        ...spec,
        current: present.length ? fmt(vals[vals.length - 1]) : null,
        hoverValue:
          this.hoverIndex !== null && vals[this.hoverIndex] !== undefined
            ? fmt(vals[this.hoverIndex])
            : null,
        line: "", area: "", min: "", max: "",
        referenceY: null, hoverX: null, hoverY: null,
        statusClass: "",
      };
      if (!present.length) return out;

      // A stat panel draws no path, so it needs the status of the current
      // reading and nothing else - no scale, no geometry.
      if (spec.stat) {
        out.statusClass = this.statusOf(spec, present[present.length - 1]);
        return out;
      }

      let lo = spec.floor !== undefined ? spec.floor : Math.min(...present);
      let hi = spec.ceiling !== undefined ? spec.ceiling : Math.max(...present);

      // A reference value belongs in the range even when no sample reaches it -
      // otherwise the line sits mid-panel and the deviation it is meant to show
      // is invisible.
      if (spec.reference != null) {
        lo = Math.min(lo, spec.reference);
        hi = Math.max(hi, spec.reference);
      }

      // A flat series would divide by zero and draw nothing. The band has to be
      // proportional to the value and nothing else: a fixed +/-1 gave a 1.36V
      // rail an axis running 0.36 to 2.36, and an absolute floor of 0.5 gave it
      // 0.71 to 2.01. Both say nothing about a rail. The || 1 covers a series
      // that is flat at exactly zero, where no proportion is available.
      if (hi === lo) {
        const pad = Math.abs(lo) * 0.02 || 1;
        hi = lo + pad;
        lo = lo - pad;
      }
      // Breathing room around a reference line, so it is never flush with the
      // panel edge where it reads as a border.
      if (spec.reference != null) {
        const pad = (hi - lo) * 0.15;
        lo -= pad;
        hi += pad;
      }
      out.min = fmt(lo);
      out.max = fmt(hi);

      const x = (i) => (this.samples.length < 2 ? 100 : (i / (this.samples.length - 1)) * 100);
      const y = (v) => 28 - ((v - lo) / (hi - lo)) * 26;

      // Gaps are breaks in the path rather than interpolated: throughput is
      // absent between flashes, and joining across that would draw a slope
      // through time when nothing was happening.
      let d = "", open = false;
      vals.forEach((v, i) => {
        if (v === null) { open = false; return; }
        d += `${open ? "L" : "M"}${x(i).toFixed(2)},${y(v).toFixed(2)} `;
        open = true;
      });
      out.line = d.trim();

      const first = vals.findIndex((v) => v !== null);
      const last = vals.length - 1 - [...vals].reverse().findIndex((v) => v !== null);
      if (d && first >= 0 && !vals.slice(first, last + 1).includes(null)) {
        out.area = `${d}L${x(last).toFixed(2)},28 L${x(first).toFixed(2)},28 Z`;
      }

      if (spec.reference != null) out.referenceY = y(spec.reference);

      if (this.hoverIndex !== null && vals[this.hoverIndex] != null) {
        out.hoverX = x(this.hoverIndex);
        out.hoverY = y(vals[this.hoverIndex]);
      }

      out.statusClass = this.statusOf(spec, vals[last]);
      return out;
    },
    // Status is on the CURRENT reading, and always alongside the number and its
    // label - never colour alone.
    statusOf(spec, now) {
      if (spec.criticalAbove != null && now > spec.criticalAbove) return "critical";
      if (spec.criticalBelow != null && now < spec.criticalBelow) return "critical";
      if (spec.warnAbove != null && now > spec.warnAbove) return "warn";
      return "";
    },
  },
};
</script>

<style scoped>
/* Ink and surface are taken from the app's own theme variables rather than
   restated here. App.vue flips --w-base-color-rgb from 0,0,0 to 255,255,255 on
   :root[data-theme], so text follows the theme wherever this component is
   mounted - including inside a teleported dialog, and without depending on this
   file's own selector matching. Hardcoding the light values here is what left
   near-black text on a dark surface.

   The trace and status colours stay literal: they are the validated palette's
   series slot 1 and its reserved status steps, and must not drift with the
   theme's greys. */
.metrics {
  --surface: var(--w-base-bg-color-rgb, #fcfcfb);
  --ink: rgb(var(--w-base-color-rgb, 11, 11, 11));
  --ink-dim: rgba(var(--w-base-color-rgb, 11, 11, 11), 0.68);
  --grid: rgba(var(--w-base-color-rgb, 11, 11, 11), 0.14);
  --trace: #2a78d6;
  --warn: #fab219;
  --critical: #d03b3b;
}
/* Only the trace needs a step change: the light blue is chosen against a light
   surface and loses contrast on a dark one. */
:global(:root[data-theme="dark"]) .metrics {
  --trace: #3987e5;
}

.metrics {
  display: grid;
  grid-template-columns: repeat(auto-fit, minmax(190px, 1fr));
  gap: 10px;
  margin-top: 12px;
  text-align: left;
}
.panel {
  border: 1px solid var(--grid);
  border-radius: 6px;
  padding: 7px 9px 5px;
}
.head {
  display: flex;
  align-items: baseline;
  justify-content: space-between;
  gap: 8px;
}
/* Text wears text tokens; the coloured trace beside it carries the status. */
.label {
  font-size: 0.78em;
  color: var(--ink-dim);
  letter-spacing: 0.02em;
}
.value {
  font-variant-numeric: tabular-nums;
  font-weight: 600;
  color: var(--ink);
}
.value.warn::before,
.value.critical::before {
  /* An icon, so a status is never carried by colour alone. */
  content: "\26A0";
  margin-right: 3px;
  font-weight: 400;
}
.value.warn { color: var(--warn); }
.value.critical { color: var(--critical); }
/* The stat panel's reading carries the panel on its own, so it is sized to be
   read at a glance rather than tucked in the header beside the label. */
.reading {
  font-variant-numeric: tabular-nums;
  font-size: 1.8em;
  font-weight: 600;
  line-height: 1.25;
  color: var(--ink);
  margin: 6px 0 2px;
}
.reading.warn::before,
.reading.critical::before {
  content: "\26A0";
  margin-right: 4px;
  font-size: 0.6em;
  font-weight: 400;
  vertical-align: 0.25em;
}
.reading.warn { color: var(--warn); }
.reading.critical { color: var(--critical); }
.reading .unit {
  font-size: 0.45em;
}
/* No axis to sit between, so the note starts at the left like a caption. */
.stat-foot {
  justify-content: flex-start;
}
.stat-foot .note {
  text-align: left;
}
.unit {
  font-size: 0.75em;
  font-weight: 400;
  color: var(--ink-dim);
  margin-left: 2px;
}
.spark {
  display: block;
  width: 100%;
  height: 34px;
  margin: 3px 0 1px;
  overflow: visible;
}
.line {
  fill: none;
  stroke: var(--trace);
  stroke-width: 2;
  stroke-linejoin: round;
  stroke-linecap: round;
}
.line.warn { stroke: var(--warn); }
.line.critical { stroke: var(--critical); }
.area {
  fill: var(--trace);
  opacity: 0.12;
  stroke: none;
}
.area.warn { fill: var(--warn); }
.area.critical { fill: var(--critical); }
/* Recessive and dashed, so it reads as a target rather than as data. */
.reference {
  stroke: var(--ink-dim);
  stroke-width: 1;
  stroke-dasharray: 3 3;
  opacity: 0.55;
}
.crosshair {
  stroke: var(--ink-dim);
  stroke-width: 1;
  opacity: 0.5;
}
.dot {
  fill: var(--trace);
  stroke: var(--surface);
  stroke-width: 2;
}
.dot.warn { fill: var(--warn); }
.dot.critical { fill: var(--critical); }
.foot {
  display: flex;
  justify-content: space-between;
  gap: 6px;
  font-size: 0.68em;
  color: var(--ink-dim);
  font-variant-numeric: tabular-nums;
}
.note {
  font-variant-numeric: normal;
  opacity: 0.85;
  text-align: center;
}
.span {
  grid-column: 1 / -1;
  margin: 2px 0 0;
  font-size: 0.75em;
  color: var(--ink-dim);
}
.metrics-empty {
  font-size: 0.85em;
  opacity: 0.7;
  margin-top: 10px;
}
</style>
