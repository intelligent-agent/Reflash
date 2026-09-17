package main

import (
	"encoding/json"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// A job that runs until its xz is killed, the way backup-emmc and
// flash-from-file do: xz at the end of a pipeline, pipefail on. `yes` rather
// than `sleep`, so the producer dies of SIGPIPE with xz instead of keeping the
// pipeline alive after it.
const xzJob = `set -o pipefail
yes | xz -0 > /dev/null`

// pollUntilSettled drives get_progress the way the client does while a job
// runs, and returns every state it reported. Polling is the point: #152 was
// get_progress turning CANCELLED into IDLE underneath a job that had not
// exited yet.
func pollUntilSettled(t *testing.T) []string {
	t.Helper()
	var seen []string
	deadline := time.Now().Add(15 * time.Second)
	for time.Now().Before(deadline) {
		w := httptest.NewRecorder()
		getProgress(w, httptest.NewRequest("GET", "/api/get_progress", nil))
		var p struct {
			State string `json:"state"`
		}
		if err := json.Unmarshal(w.Body.Bytes(), &p); err != nil {
			t.Fatalf("bad get_progress body: %v", err)
		}
		seen = append(seen, p.State)
		if p.State == CANCELLED || p.State == ERROR || p.State == FINISHED {
			return seen
		}
		time.Sleep(50 * time.Millisecond)
	}
	t.Fatalf("job never settled; states seen: %v", seen)
	return nil
}

// settledAs polls until the job settles, then keeps polling for a while: in
// #152 the state read CANCELLED first and was overwritten with ERROR once the
// killed pipeline finally exited, so the first settled answer is not the last.
func settledAs(t *testing.T, want string) {
	t.Helper()
	seen := pollUntilSettled(t)
	for end := time.Now().Add(2 * time.Second); time.Now().Before(end); {
		w := httptest.NewRecorder()
		getProgress(w, httptest.NewRequest("GET", "/api/get_progress", nil))
		var p struct {
			State string `json:"state"`
		}
		_ = json.Unmarshal(w.Body.Bytes(), &p)
		if p.State != IDLE {
			seen = append(seen, p.State)
		}
		time.Sleep(50 * time.Millisecond)
	}
	for _, s := range seen {
		if s == ERROR && want != ERROR {
			t.Fatalf("reported ERROR on the way (states %v), want %q", seen, want)
		}
	}
	if last := seen[len(seen)-1]; last != want {
		t.Fatalf("ended as %q (states %v), want %q", last, seen, want)
	}
}

func TestCancelledBackupIsCancelledNotAnError(t *testing.T) {
	dir := setupTest(t)
	mounts := filepath.Join(dir, "mounts")
	fakeBin(t, dir, "mount-unmount-usb", `echo "$@" >> `+mounts)
	// Writes its image first, like the real script, so there is a partial file
	// for the cancel to clean up.
	fakeBin(t, dir, "backup-emmc", `echo partial > "$1.img.xz"
`+xzJob)
	state = &State{State: IDLE}

	body := strings.NewReader(`{"filename":"mybackup"}`)
	startBackup(httptest.NewRecorder(), httptest.NewRequest("PUT", "/api/start_backup", body))
	cancelBackup(httptest.NewRecorder(), httptest.NewRequest("PUT", "/api/cancel_backup", nil))

	settledAs(t, CANCELLED)
	// backup-emmc names its output <label>.img.xz; removing <label> left the
	// truncated image in the install list (#153).
	if _, err := os.Stat(filepath.Join(images_folder, "mybackup.img.xz")); !os.IsNotExist(err) {
		t.Errorf("the partial backup image is still on the drive (stat err %v)", err)
	}
	got, _ := os.ReadFile(mounts)
	if !strings.HasSuffix(strings.TrimSpace(string(got)), "mounted "+MODE_RO) {
		t.Errorf("drive not left read-only; mount calls were %q", got)
	}
}

func TestFailedBackupRemovesItsPartialImage(t *testing.T) {
	dir := setupTest(t)
	fakeBin(t, dir, "mount-unmount-usb", `exit 0`)
	fakeBin(t, dir, "backup-emmc", `echo partial > "$1.img.xz"; exit 3`)
	state = &State{State: IDLE}

	startBackup(httptest.NewRecorder(), httptest.NewRequest("PUT", "/api/start_backup",
		strings.NewReader(`{"filename":"broken"}`)))
	seen := pollUntilSettled(t)

	if last := seen[len(seen)-1]; last != ERROR {
		t.Fatalf("a failing backup ended as %q, want %q", last, ERROR)
	}
	if _, err := os.Stat(filepath.Join(images_folder, "broken.img.xz")); !os.IsNotExist(err) {
		t.Error("a failed backup left its truncated image behind")
	}
}

func TestCancelledInstallIsCancelledNotAnError(t *testing.T) {
	dir := setupTest(t)
	fakeBin(t, dir, "flash-from-file", xzJob)
	state = &State{State: IDLE}
	if err := os.WriteFile(filepath.Join(images_folder, "img.img.xz"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}

	installRefactor(httptest.NewRecorder(), httptest.NewRequest("PUT", "/api/start_installation",
		strings.NewReader(`{"filename":"img.img.xz"}`)))
	cancelInstallation(httptest.NewRecorder(), httptest.NewRequest("PUT", "/api/cancel_installation", nil))

	settledAs(t, CANCELLED)
}

// `pkill -f xz -9` killed anything with "xz" in its command line. A cancel has
// to reach the job's xz and nothing else (#156).
func TestCancelKillsOnlyTheJobsXz(t *testing.T) {
	dir := setupTest(t)
	fakeBin(t, dir, "flash-from-file", xzJob)
	state = &State{State: IDLE}

	bystander := exec.Command("bash", "-c", xzJob)
	if err := bystander.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = bystander.Process.Kill() })
	done := make(chan error, 1)
	go func() { done <- bystander.Wait() }()

	installRefactor(httptest.NewRecorder(), httptest.NewRequest("PUT", "/api/start_installation",
		strings.NewReader(`{"filename":"img.img.xz"}`)))
	cancelInstallation(httptest.NewRecorder(), httptest.NewRequest("PUT", "/api/cancel_installation", nil))
	pollUntilSettled(t)

	select {
	case err := <-done:
		t.Fatalf("the cancel killed an unrelated xz pipeline (exit: %v)", err)
	case <-time.After(300 * time.Millisecond):
	}
}

// The UI can send a cancel when nothing is running - the job finished a moment
// earlier, or a button fired the wrong action (#156). That must be a no-op,
// not an error toast and not a changed state.
func TestCancelWithNothingRunningIsANoOp(t *testing.T) {
	setupTest(t)
	state = &State{State: IDLE}

	for name, h := range map[string]func(w *httptest.ResponseRecorder){
		"cancel_installation": func(w *httptest.ResponseRecorder) {
			cancelInstallation(w, httptest.NewRequest("PUT", "/api/cancel_installation", nil))
		},
		"cancel_backup": func(w *httptest.ResponseRecorder) {
			cancelBackup(w, httptest.NewRequest("PUT", "/api/cancel_backup", nil))
		},
		"cancel_magic": func(w *httptest.ResponseRecorder) {
			cancelMagic(w, httptest.NewRequest("PUT", "/api/cancel_magic", nil))
		},
	} {
		w := httptest.NewRecorder()
		h(w)
		var resp StatusResult
		if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
			t.Fatalf("%s: bad body %q", name, w.Body.String())
		}
		if resp.Status != "OK" {
			t.Errorf("%s with nothing running answered %+v", name, resp)
		}
		if state.State != IDLE {
			t.Errorf("%s with nothing running changed the state to %q", name, state.State)
		}
	}
}

// A cancel meant for one job must not stop the next one.
func TestAStaleCancelDoesNotStopTheNextJob(t *testing.T) {
	dir := setupTest(t)
	fakeBin(t, dir, "flash-from-file", `exit 0`)
	state = &State{State: IDLE}

	cancelInstallation(httptest.NewRecorder(), httptest.NewRequest("PUT", "/api/cancel_installation", nil))
	installRefactor(httptest.NewRecorder(), httptest.NewRequest("PUT", "/api/start_installation",
		strings.NewReader(`{"filename":"img.img.xz"}`)))

	seen := pollUntilSettled(t)
	if last := seen[len(seen)-1]; last != FINISHED {
		t.Errorf("install after an earlier cancel ended as %q, want %q", last, FINISHED)
	}
}
