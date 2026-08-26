package main

// Board telemetry: what the hardware was doing, sampled once a second and kept
// in a ring buffer so the UI can plot the recent past (#128).
//
// This exists because chasing memory corruption on an A5 cost a long session
// largely for want of it, and the variables that mattered were not the ones
// being watched: the DRAM rail was 1.36V on a board fitted with 1.5V parts, the
// SoC reached 71C during a 1.25GB flash and engaged thermal throttling, and CPU
// DVFS ran ~57 transitions a second while DRAM devfreq managed about six across
// the whole flash. All of it had to be gathered by hand with an ad-hoc script
// streamed over SSH.
//
// Everything here is a pure read of sysfs/procfs, so it is written in Go rather
// than as a bash helper - the standing convention for this codebase. A missing
// file is normal (paths differ across kernels and revisions) and yields a nil
// field rather than an error: a board that cannot report its DRAM frequency
// should still plot its temperature.

import (
	"bufio"
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"
)

// Five minutes at one sample a second. The interesting part is always the last
// stretch before something went wrong, and a whole-session buffer on a board
// with 1GB of RAM - most of it already spoken for by the initramfs - is not
// worth the memory.
const metricsWindow = 300

// A nil field means "this board does not report it", which is different from
// zero: 0C and "no thermal zone" must not plot the same.
type Sample struct {
	T         int64    `json:"t"`                   // unix seconds
	CPUTemp   *float64 `json:"cpu_temp,omitempty"`  // C
	GPUTemp   *float64 `json:"gpu_temp,omitempty"`  // C
	CPUFreq   *int     `json:"cpu_freq,omitempty"`  // MHz
	DRAMFreq  *int     `json:"dram_freq,omitempty"` // MHz
	VddCPU    *float64 `json:"vdd_cpu,omitempty"`   // V
	VccDRAM   *float64 `json:"vcc_dram,omitempty"`  // V
	Throttle  *int     `json:"throttle,omitempty"`  // cooling state, 0-7
	Load      *float64 `json:"load,omitempty"`      // 1-minute load average
	Dirty     *int     `json:"dirty,omitempty"`     // kB awaiting writeback
	Bandwidth *float64 `json:"bandwidth,omitempty"` // MB/s, while a transfer runs
}

// Path seams, in the style of binDir elsewhere in this package: tests point
// them at a temp directory holding fixture files, so the parsing can be
// exercised without a board and without root. Empty means the real thing.
var (
	sysRoot  = ""
	procRoot = ""
)

func sysPath(p string) string  { return sysRoot + p }
func procPath(p string) string { return procRoot + p }

var (
	metricsMu   sync.RWMutex
	metricsRing []Sample

	// Resolved once and cached: the regulator directory names are stable for
	// the life of the boot, and scanning /sys/class/regulator on every sample
	// would be a directory walk a second for an answer that cannot change.
	// A nil check rather than sync.Once, so a test can clear it after pointing
	// the seam at a fixture tree.
	regulatorMu    sync.Mutex
	regulatorPaths map[string]string
)

func readTrimmed(path string) (string, bool) {
	b, err := os.ReadFile(path)
	if err != nil {
		return "", false
	}
	return strings.TrimSpace(string(b)), true
}

// readScaled reads an integer from sysfs and divides it, which is most of what
// this file does: sysfs reports millidegrees, kHz, Hz and microvolts, and the
// UI wants degrees, MHz and volts.
func readScaled(path string, divisor float64) *float64 {
	s, ok := readTrimmed(path)
	if !ok {
		return nil
	}
	n, err := strconv.ParseFloat(s, 64)
	if err != nil {
		return nil
	}
	v := n / divisor
	return &v
}

func readScaledInt(path string, divisor int) *int {
	s, ok := readTrimmed(path)
	if !ok {
		return nil
	}
	n, err := strconv.Atoi(s)
	if err != nil {
		return nil
	}
	v := n / divisor
	return &v
}

// Regulators are addressed by the name they report, not by their directory
// index: regulator.7 is not stable across kernels, and vcc-dram is the whole
// point of this - a board running 1.36V when it should be at 1.5V is the fault
// that started this work.
func findRegulators() map[string]string {
	found := map[string]string{}
	dirs, err := filepath.Glob(sysPath("/sys/class/regulator/regulator.*"))
	if err != nil {
		return found
	}
	for _, d := range dirs {
		if name, ok := readTrimmed(filepath.Join(d, "name")); ok {
			found[name] = d
		}
	}
	return found
}

func regulatorVolts(name string) *float64 {
	regulatorMu.Lock()
	if regulatorPaths == nil {
		regulatorPaths = findRegulators()
	}
	dir, ok := regulatorPaths[name]
	regulatorMu.Unlock()
	if !ok {
		return nil
	}
	return readScaled(filepath.Join(dir, "microvolts"), 1e6)
}

// The DRAM controller's devfreq node is named after its address, which differs
// between device trees - so match the suffix rather than hard-coding the A64's
// 1c62000. Falls back to whatever single devfreq device exists.
func dramFreqMHz() *int {
	dirs, _ := filepath.Glob(sysPath("/sys/class/devfreq/*dram*"))
	if len(dirs) == 0 {
		dirs, _ = filepath.Glob(sysPath("/sys/class/devfreq/*"))
	}
	if len(dirs) == 0 {
		return nil
	}
	return readScaledInt(filepath.Join(dirs[0], "cur_freq"), 1e6)
}

func loadAvg() *float64 {
	s, ok := readTrimmed(procPath("/proc/loadavg"))
	if !ok {
		return nil
	}
	fields := strings.Fields(s)
	if len(fields) == 0 {
		return nil
	}
	v, err := strconv.ParseFloat(fields[0], 64)
	if err != nil {
		return nil
	}
	return &v
}

// Writeback backlog. During a flash this is the difference between "the upload
// is slow" and "the upload is fine and the card cannot keep up".
func dirtyKB() *int {
	f, err := os.Open(procPath("/proc/meminfo"))
	if err != nil {
		return nil
	}
	defer f.Close()
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		if !strings.HasPrefix(sc.Text(), "Dirty:") {
			continue
		}
		fields := strings.Fields(sc.Text())
		if len(fields) < 2 {
			return nil
		}
		if v, err := strconv.Atoi(fields[1]); err == nil {
			return &v
		}
		return nil
	}
	return nil
}

func sampleNow() Sample {
	s := Sample{
		T:        time.Now().Unix(),
		CPUTemp:  readScaled(sysPath("/sys/class/thermal/thermal_zone0/temp"), 1000),
		GPUTemp:  readScaled(sysPath("/sys/class/thermal/thermal_zone1/temp"), 1000),
		CPUFreq:  readScaledInt(sysPath("/sys/devices/system/cpu/cpufreq/policy0/scaling_cur_freq"), 1000),
		DRAMFreq: dramFreqMHz(),
		VddCPU:   regulatorVolts("vdd-cpux"),
		VccDRAM:  regulatorVolts("vcc-dram"),
		Throttle: readScaledInt(sysPath("/sys/class/thermal/cooling_device0/cur_state"), 1),
		Load:     loadAvg(),
		Dirty:    dirtyKB(),
	}

	// Throughput is only meaningful while something is transferring; a flat
	// zero between flashes would read as a stall rather than as idleness.
	if state != nil {
		state.Lock()
		if state.State != IDLE && state.Bandwidth > 0 {
			bw := float64(state.Bandwidth)
			s.Bandwidth = &bw
		}
		state.Unlock()
	}
	return s
}

// Sampling runs for the life of the server rather than only during a flash. The
// board being idle is itself a reading - it is what a temperature ramp is
// measured against - and the cost is a dozen small sysfs reads a second.
func startMetrics() {
	metricsRing = make([]Sample, 0, metricsWindow)
	go func() {
		for {
			s := sampleNow()
			metricsMu.Lock()
			if len(metricsRing) == metricsWindow {
				metricsRing = append(metricsRing[1:], s)
			} else {
				metricsRing = append(metricsRing, s)
			}
			metricsMu.Unlock()
			time.Sleep(time.Second)
		}
	}()
}

// GET /api/get_metrics?since=<unix>
//
// "since" keeps the steady-state response to the one or two samples the client
// does not have. Without it every poll would resend the whole five-minute
// window, which is ~30kB of JSON several times a minute for data the client
// already holds.
func getMetrics(w http.ResponseWriter, r *http.Request) {
	since, _ := strconv.ParseInt(r.URL.Query().Get("since"), 10, 64)

	metricsMu.RLock()
	out := make([]Sample, 0, len(metricsRing))
	for _, s := range metricsRing {
		if s.T > since {
			out = append(out, s)
		}
	}
	metricsMu.RUnlock()

	json.NewEncoder(w).Encode(map[string]any{
		"samples": out,
		"window":  metricsWindow,
	})
}
