package main

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func startTestDownload(t *testing.T, url, name string, size int) {
	t.Helper()
	body, _ := json.Marshal(map[string]any{"filename": name, "url": url, "size": size})
	startDownload(httptest.NewRecorder(), httptest.NewRequest("PUT", "/api/start_download", bytes.NewReader(body)))
}

func waitForState(t *testing.T, want string) {
	t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		if state.State == want {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatalf("state = %q (error %q), want %q", state.State, state.Error, want)
}

// Cancel used to stop only the wait: the copy ran on in its own goroutine,
// downloading into the removed file until the image was complete, so the
// remount failed with "target is busy" and progress kept being logged.
func TestDownloadCancelStopsTheTransfer(t *testing.T) {
	dir := setupTest(t)
	mounts := filepath.Join(dir, "mounts")
	fakeBin(t, dir, "mount-unmount-usb", `echo "$@" >> `+mounts)
	state = &State{State: IDLE}

	var disconnected atomic.Bool
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		chunk := bytes.Repeat([]byte("x"), 64<<10)
		for i := 0; i < 1000; i++ {
			select {
			case <-r.Context().Done():
				disconnected.Store(true)
				return
			default:
			}
			if _, err := w.Write(chunk); err != nil {
				disconnected.Store(true)
				return
			}
			w.(http.Flusher).Flush()
			time.Sleep(10 * time.Millisecond)
		}
	}))
	defer srv.Close()

	startTestDownload(t, srv.URL, "slow.img.xz", 64<<20)
	path := filepath.Join(images_folder, "slow.img.xz")
	waitFor(t, func() bool {
		fi, err := os.Stat(path)
		return err == nil && fi.Size() > 0
	})
	cancelDownload(httptest.NewRecorder(), httptest.NewRequest("PUT", "/api/cancel_download", nil))

	waitForState(t, CANCELLED)
	waitFor(t, disconnected.Load)
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Errorf("the cancelled download is still on the drive, stat err %v", err)
	}
	got, _ := os.ReadFile(mounts)
	if !strings.HasSuffix(strings.TrimSpace(string(got)), "mounted "+MODE_RO) {
		t.Errorf("drive not left read-only; mount calls %q", got)
	}
}

// The progress get_progress and STATUS report is sampled on each poll, so the
// end of a download was whatever the last poll caught - 96% (#155).
func TestFinishedDownloadReportsAllOfIt(t *testing.T) {
	dir := setupTest(t)
	fakeBin(t, dir, "mount-unmount-usb", `exit 0`)
	state = &State{State: IDLE}
	payload := bytes.Repeat([]byte("y"), 300<<10)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write(payload)
	}))
	defer srv.Close()

	startTestDownload(t, srv.URL, "whole.img.xz", len(payload))
	// A poll mid-download, as the client makes, so the sampled counters are
	// behind when the copy finishes.
	refreshProgress()
	waitForState(t, FINISHED)

	if state.Progress != 100 || state.BytesNow != len(payload) {
		t.Errorf("finished download reports %.1f%% and %d bytes, want 100%% and %d",
			state.Progress, state.BytesNow, len(payload))
	}
}

// A download that could not start used to panic, taking the web UI and the
// log stream down with the server.
func TestDownloadFailureIsAnErrorNotAPanic(t *testing.T) {
	dir := setupTest(t)
	fakeBin(t, dir, "mount-unmount-usb", `exit 0`)
	state = &State{State: IDLE}
	srv := httptest.NewServer(http.NotFoundHandler())
	defer srv.Close()

	startTestDownload(t, srv.URL, "missing.img.xz", 100)
	waitForState(t, ERROR)
	if _, err := os.Stat(filepath.Join(images_folder, "missing.img.xz")); !os.IsNotExist(err) {
		t.Error("a failed download left a file behind - a 404 page saved as an image")
	}
}

func TestCancelDownloadWithNothingRunningDoesNotPanic(t *testing.T) {
	setupTest(t)
	state = &State{State: IDLE}
	cancelFunc = nil
	cancelDownload(httptest.NewRecorder(), httptest.NewRequest("PUT", "/api/cancel_download", nil))
}

// The first bandwidth line of a download after a cancelled one read
// "-4.66 MB/s": the log's baseline was where the previous transfer stopped
// (#155).
func TestANewTransferDoesNotInheritTheLastOnesCounters(t *testing.T) {
	dir := setupTest(t)
	fakeBin(t, dir, "mount-unmount-usb", `exit 0`)
	saved := bandwidthLogEvery
	bandwidthLogEvery = 50 * time.Millisecond
	defer func() { bandwidthLogEvery = saved; lastBandwidthLog = time.Time{}; bytesAtLastLog = 0 }()

	// Where a cancelled transfer left things.
	state = &State{State: IDLE, BytesNow: 252 << 20, BytesTotal: 332 << 20, Progress: 76}
	bytesAtLastLog = 252 << 20
	lastBandwidthLog = time.Now().Add(-time.Minute)
	bytes_last = 252 << 20

	body, _ := json.Marshal(map[string]any{"filename": "next.img.xz", "size": 300 << 20})
	uploadStart(httptest.NewRecorder(), httptest.NewRequest("PUT", "/api/upload_start", bytes.NewReader(body)))
	defer uploadCancel(httptest.NewRecorder(), httptest.NewRequest("PUT", "/api/upload_cancel", nil))

	if state.BytesNow != 0 || state.Progress != 0 || bytes_last != 0 ||
		bytesAtLastLog != 0 || !lastBandwidthLog.IsZero() {
		t.Fatalf("counters carried over into the new transfer: BytesNow=%d Progress=%.0f bytes_last=%d bytesAtLastLog=%d lastBandwidthLog=%v",
			state.BytesNow, state.Progress, bytes_last, bytesAtLastLog, lastBandwidthLog)
	}

	logBandwidth()
	time.Sleep(60 * time.Millisecond)
	state.BytesNow = 10 << 20
	logBandwidth()
	logged, _ := os.ReadFile(log_file)
	if regexp.MustCompile(`: -[0-9.]+ MB/s`).Match(logged) {
		t.Errorf("negative throughput logged for a fresh transfer:\n%s", logged)
	}
}

// get_progress kept polling after a transfer and logged a rate for IDLE.
func TestBandwidthIsNotLoggedOnceTheTransferIsOver(t *testing.T) {
	setupTest(t)
	saved := bandwidthLogEvery
	bandwidthLogEvery = 50 * time.Millisecond
	defer func() { bandwidthLogEvery = saved; lastBandwidthLog = time.Time{}; bytesAtLastLog = 0 }()

	state = &State{State: IDLE, BytesNow: 235 << 20, BytesTotal: 314 << 20}
	lastBandwidthLog = time.Now().Add(-time.Minute)
	logBandwidth()

	logged, _ := os.ReadFile(log_file)
	if strings.Contains(string(logged), "MB/s") {
		t.Errorf("logged a rate while IDLE:\n%s", logged)
	}
}

func TestProgressJSONHasNoFileHandle(t *testing.T) {
	setupTest(t)
	state = &State{State: IDLE}
	w := httptest.NewRecorder()
	getProgress(w, httptest.NewRequest("GET", "/api/get_progress", nil))
	if strings.Contains(w.Body.String(), `"File"`) {
		t.Errorf("get_progress carries the Go file handle: %s", w.Body.String())
	}
}
