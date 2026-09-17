package main

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// gatedBody is a chunk that is still arriving: it blocks until released, so a
// test can hold a chunk in flight inside writeChunk.
type gatedBody struct {
	gate chan struct{}
	data []byte
	sent bool
}

func (g *gatedBody) Read(p []byte) (int, error) {
	if g.sent {
		return 0, io.EOF
	}
	<-g.gate
	g.sent = true
	return copy(p, g.data), nil
}

func startPlainUpload(t *testing.T, name string) {
	t.Helper()
	body, _ := json.Marshal(map[string]any{"filename": name, "size": 1 << 20})
	uploadStart(httptest.NewRecorder(), httptest.NewRequest("PUT", "/api/upload_start", bytes.NewReader(body)))
	if state.File == nil {
		t.Fatal("uploadStart did not open the file")
	}
	w := httptest.NewRecorder()
	uploadChunk(w, httptest.NewRequest("POST", "/api/upload_chunk", bytes.NewReader([]byte("first chunk"))))
	if !strings.Contains(w.Body.String(), "true") {
		t.Fatalf("first chunk failed: %s", w.Body.String())
	}
}

// Pressing Cancel with a chunk on the wire produced two red "The upload failed
// while writing to the USB drive" toasts and left the drive mounted rw: the
// cancel closed the file under the chunk, its write failed with "invalid
// argument", and that set ERROR (#151).
func TestCancelWithAChunkInFlightIsACleanCancel(t *testing.T) {
	dir := setupTest(t)
	mounts := filepath.Join(dir, "mounts")
	fakeBin(t, dir, "mount-unmount-usb", `echo "$@" >> `+mounts)
	state = &State{State: IDLE}
	startPlainUpload(t, "cancelled.img.xz")

	gate := make(chan struct{})
	chunk := httptest.NewRecorder()
	chunkDone := make(chan struct{})
	go func() {
		uploadChunk(chunk, httptest.NewRequest("POST", "/api/upload_chunk",
			&gatedBody{gate: gate, data: []byte("second chunk")}))
		close(chunkDone)
	}()
	waitFor(t, func() bool {
		uploadMutex.Lock()
		defer uploadMutex.Unlock()
		return chunksInFlight == 1
	})

	cancelDone := make(chan struct{})
	go func() {
		uploadCancel(httptest.NewRecorder(), httptest.NewRequest("PUT", "/api/upload_cancel", nil))
		close(cancelDone)
	}()
	time.Sleep(50 * time.Millisecond) // the cancel is now waiting on the chunk
	close(gate)
	<-chunkDone
	<-cancelDone

	if chunk.Code != 200 {
		t.Errorf("the in-flight chunk answered %d %q - a cancel is not a failure", chunk.Code, chunk.Body.String())
	}
	if state.State != CANCELLED {
		t.Errorf("state = %q (error %q), want %q", state.State, state.Error, CANCELLED)
	}
	if uploadFailed {
		t.Error("uploadFailed set by a user cancel")
	}
	if _, err := os.Stat(filepath.Join(images_folder, "cancelled.img.xz")); !os.IsNotExist(err) {
		t.Errorf("the partial upload is still on the drive (#153), stat err %v", err)
	}
	got, _ := os.ReadFile(mounts)
	if !strings.HasSuffix(strings.TrimSpace(string(got)), "mounted "+MODE_RO) {
		t.Errorf("drive not returned to read-only; mount calls %q", got)
	}
	log, _ := os.ReadFile(log_file)
	if strings.Contains(string(log), "Could not write a chunk") {
		t.Errorf("a user cancel was logged as a write failure:\n%s", log)
	}
}

// The chunk that was in flight gets success=false back, and the client answers
// that with a second upload_cancel. By then the poll has moved on to IDLE, and
// acting on it again put CANCELLED back with nobody left polling to clear it.
func TestASecondCancelForTheSameUploadIsIgnored(t *testing.T) {
	dir := setupTest(t)
	fakeBin(t, dir, "mount-unmount-usb", `exit 0`)
	state = &State{State: IDLE}
	startPlainUpload(t, "twice.img.xz")

	uploadCancel(httptest.NewRecorder(), httptest.NewRequest("PUT", "/api/upload_cancel", nil))
	getProgress(httptest.NewRecorder(), httptest.NewRequest("GET", "/api/get_progress", nil))
	if state.State != IDLE {
		t.Fatalf("after the poll state = %q, want %q", state.State, IDLE)
	}
	uploadCancel(httptest.NewRecorder(), httptest.NewRequest("PUT", "/api/upload_cancel", nil))
	if state.State != IDLE {
		t.Errorf("a second cancel moved state back to %q", state.State)
	}
}

// A failed upload still ends in ERROR (#114), and its partial file goes too.
func TestCancelAfterAFailedChunkKeepsTheErrorAndRemovesTheFile(t *testing.T) {
	dir := setupTest(t)
	fakeBin(t, dir, "mount-unmount-usb", `exit 0`)
	state = &State{State: IDLE}
	startPlainUpload(t, "failed.img.xz")
	state.State = ERROR
	uploadFailed = true

	uploadCancel(httptest.NewRecorder(), httptest.NewRequest("PUT", "/api/upload_cancel", nil))

	if state.State != ERROR {
		t.Errorf("state = %q, want %q", state.State, ERROR)
	}
	if _, err := os.Stat(filepath.Join(images_folder, "failed.img.xz")); !os.IsNotExist(err) {
		t.Error("the failed upload's partial file is still on the drive")
	}
}
