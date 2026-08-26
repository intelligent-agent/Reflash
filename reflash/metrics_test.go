package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// setupMetrics points the sysfs/procfs seams at a fixture tree, so the parsing
// runs with no board and no root. Values are the ones measured on an A5 during
// the session that motivated #128 - 71C with cooling_device0 at 3/7, and a DRAM
// rail at 1.36V on a board fitted with 1.5V parts.
func setupMetrics(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	sysRoot, procRoot = dir, dir
	regulatorPaths = nil // clear any cache built against the real /sys
	t.Cleanup(func() {
		sysRoot, procRoot = "", ""
		regulatorPaths = nil
	})

	write := func(p, s string) {
		full := filepath.Join(dir, p)
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(full, []byte(s), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write("/sys/class/thermal/thermal_zone0/temp", "71000\n")
	write("/sys/class/thermal/thermal_zone1/temp", "68000\n")
	write("/sys/class/thermal/cooling_device0/cur_state", "3\n")
	write("/sys/devices/system/cpu/cpufreq/policy0/scaling_cur_freq", "1104000\n")
	write("/sys/class/devfreq/1c62000.dram-controller/cur_freq", "1296000000\n")
	write("/sys/class/regulator/regulator.7/name", "vcc-dram\n")
	write("/sys/class/regulator/regulator.7/microvolts", "1360000\n")
	write("/sys/class/regulator/regulator.3/name", "vdd-cpux\n")
	write("/sys/class/regulator/regulator.3/microvolts", "1040000\n")
	write("/proc/loadavg", "1.51 0.98 0.44 2/143 981\n")
	write("/proc/meminfo", "MemTotal:  959944 kB\nDirty:       3072 kB\nWriteback:   0 kB\n")
	return dir
}

func TestSampleReadsTheUnitsRight(t *testing.T) {
	setupMetrics(t)
	s := sampleNow()

	// Every one of these is a unit conversion that is easy to get wrong and
	// silently plausible when wrong: millidegrees, kHz, Hz and microvolts all
	// look like reasonable numbers if the divisor is off.
	cases := []struct {
		name string
		got  any
		want any
	}{
		{"cpu temp, from millidegrees", s.CPUTemp, 71.0},
		{"gpu temp, from millidegrees", s.GPUTemp, 68.0},
		{"cpu freq, from kHz", s.CPUFreq, 1104},
		{"dram freq, from Hz", s.DRAMFreq, 1296},
		{"vcc-dram, from microvolts", s.VccDRAM, 1.36},
		{"vdd-cpux, from microvolts", s.VddCPU, 1.04},
		{"throttle, as-is", s.Throttle, 3},
		{"load, first field only", s.Load, 1.51},
		{"dirty, kB", s.Dirty, 3072},
	}
	for _, c := range cases {
		switch want := c.want.(type) {
		case float64:
			got, ok := c.got.(*float64)
			if !ok || got == nil {
				t.Errorf("%s: nil", c.name)
			} else if *got != want {
				t.Errorf("%s: got %v want %v", c.name, *got, want)
			}
		case int:
			got, ok := c.got.(*int)
			if !ok || got == nil {
				t.Errorf("%s: nil", c.name)
			} else if *got != want {
				t.Errorf("%s: got %v want %v", c.name, *got, want)
			}
		}
	}
}

// A board that does not report something must yield nil, not zero. 0C and "no
// thermal zone" plot identically otherwise, and the second one is a lie.
func TestMissingFilesAreNilNotZero(t *testing.T) {
	dir := t.TempDir()
	sysRoot, procRoot = dir, dir
	regulatorPaths = nil // clear any cache built against the real /sys
	t.Cleanup(func() {
		sysRoot, procRoot = "", ""
		regulatorPaths = nil
	})

	s := sampleNow()
	if s.CPUTemp != nil || s.VccDRAM != nil || s.DRAMFreq != nil || s.Dirty != nil {
		t.Errorf("expected nils on an empty tree, got %+v", s)
	}
	if s.T == 0 {
		t.Error("the timestamp should be set even when every read fails")
	}

	// And it must serialise without the absent fields, so the client can tell
	// "not reported" from "reported as zero".
	b, err := json.Marshal(s)
	if err != nil {
		t.Fatal(err)
	}
	for _, absent := range []string{"cpu_temp", "vcc_dram", "dram_freq", "dirty"} {
		if strings.Contains(string(b), absent) {
			t.Errorf("%s should be omitted when unread, got %s", absent, b)
		}
	}
}

// The regulator that matters is found by NAME. regulator.7 is not stable across
// kernels, and reading the wrong one would report a plausible voltage for the
// wrong rail - which is the exact fault #128 exists to make visible.
func TestRegulatorsAreFoundByName(t *testing.T) {
	setupMetrics(t)
	if v := regulatorVolts("vcc-dram"); v == nil || *v != 1.36 {
		t.Errorf("vcc-dram: got %v want 1.36", v)
	}
	if v := regulatorVolts("nonexistent-rail"); v != nil {
		t.Errorf("an unknown rail should be nil, got %v", *v)
	}
}

// A transfer sitting at 0 MB/s is a stall, and a stall is the most useful thing
// this panel can show. It must be recorded AS zero: omitting it draws a gap,
// which is how the client renders an idle board, so a stalled flash would look
// identical to no flash at all.
func TestStalledTransferRecordsZeroRatherThanNothing(t *testing.T) {
	setupMetrics(t)
	saved := state
	t.Cleanup(func() { state = saved })

	state = &State{State: UPLOADING_MAGIC, BytesTotal: 1 << 30, Bandwidth: 0}
	if bw := sampleNow().Bandwidth; bw == nil {
		t.Error("a stalled transfer must report 0, not be omitted")
	} else if *bw != 0 {
		t.Errorf("got %v want 0", *bw)
	}

	// Idle is the one case that genuinely has no throughput to report.
	state = &State{State: IDLE}
	if bw := sampleNow().Bandwidth; bw != nil {
		t.Errorf("an idle board should report no throughput, got %v", *bw)
	}
}

func TestGetMetricsSinceReturnsOnlyNewSamples(t *testing.T) {
	metricsMu.Lock()
	metricsRing = []Sample{{T: 100}, {T: 200}, {T: 300}}
	metricsMu.Unlock()
	t.Cleanup(func() {
		metricsMu.Lock()
		metricsRing = nil
		metricsMu.Unlock()
	})

	rec := httptest.NewRecorder()
	getMetrics(rec, httptest.NewRequest(http.MethodGet, "/api/get_metrics?since=200", nil))

	var body struct {
		Samples []Sample `json:"samples"`
		Window  int      `json:"window"`
	}
	if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
		t.Fatal(err)
	}
	if len(body.Samples) != 1 || body.Samples[0].T != 300 {
		t.Errorf("since=200 should return only t=300, got %+v", body.Samples)
	}
	if body.Window != metricsWindow {
		t.Errorf("window: got %d want %d", body.Window, metricsWindow)
	}

	// No "since" means a fresh client, which needs the whole buffer.
	rec = httptest.NewRecorder()
	getMetrics(rec, httptest.NewRequest(http.MethodGet, "/api/get_metrics", nil))
	body.Samples = nil
	if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
		t.Fatal(err)
	}
	if len(body.Samples) != 3 {
		t.Errorf("a fresh client should get everything, got %d samples", len(body.Samples))
	}
}
