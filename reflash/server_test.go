package main

import (
	"bytes"
	"encoding/json"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// setupTest points the helper-script seam (binDir) and the on-disk paths at a
// throwaway directory so handlers run hermetically — no root, no real board.
func setupTest(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	binDir = dir
	log_file = filepath.Join(dir, "reflash.log")
	images_folder = filepath.Join(dir, "images")
	options_file = filepath.Join(dir, "options.cfg")
	if err := os.MkdirAll(images_folder, 0o755); err != nil {
		t.Fatal(err)
	}
	options = &Options{}
	return dir
}

// fakeBin installs an executable stub helper script in dir (which is binDir).
func fakeBin(t *testing.T, dir, name, body string) {
	t.Helper()
	p := filepath.Join(dir, name)
	if err := os.WriteFile(p, []byte("#!/usr/bin/env bash\n"+body+"\n"), 0o755); err != nil {
		t.Fatal(err)
	}
}

func TestResolveCmd(t *testing.T) {
	binDir = "/usr/local/bin"
	cases := map[string]string{
		"wifi-scan":           "/usr/local/bin/wifi-scan", // helper -> joined with binDir
		"get-reflash-version": "/usr/local/bin/get-reflash-version",
		"pkill":               "pkill",     // system util -> unchanged
		"xz":                  "xz",        // system util -> unchanged
		"/bin/echo":           "/bin/echo", // explicit path -> unchanged
	}
	for in, want := range cases {
		if got := resolveCmd(in); got != want {
			t.Errorf("resolveCmd(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestGetInfo(t *testing.T) {
	dir := setupTest(t)
	fakeBin(t, dir, "get-reflash-version", `echo "v9.9.9"`)
	fakeBin(t, dir, "get-recore-revision", `echo "A8"`)
	fakeBin(t, dir, "get-recore-serial-number", `echo "RC-0042"`)
	fakeBin(t, dir, "get-emmc-version", `echo "emmc-1"`)
	fakeBin(t, dir, "is-ssh-enabled", `echo "true"`)
	fakeBin(t, dir, "get-free-space", `echo "12345"`)
	fakeBin(t, dir, "get-hostnames", `echo "recore.local"`)
	if err := os.WriteFile(filepath.Join(images_folder, "test.img.xz"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}

	rr := httptest.NewRecorder()
	getInfo(rr, httptest.NewRequest("GET", "/api/get_info", nil))

	var info GetInfo
	if err := json.Unmarshal(rr.Body.Bytes(), &info); err != nil {
		t.Fatalf("decode: %v (body=%q)", err, rr.Body.String())
	}
	if info.ReflashVersion != "v9.9.9" {
		t.Errorf("ReflashVersion = %q, want v9.9.9", info.ReflashVersion)
	}
	if info.RecoreRevision != "A8" {
		t.Errorf("RecoreRevision = %q, want A8", info.RecoreRevision)
	}
	if info.SerialNumber != "RC-0042" {
		t.Errorf("SerialNumber = %q, want RC-0042", info.SerialNumber)
	}
	if info.EmmcVersion != "emmc-1" {
		t.Errorf("EmmcVersion = %q, want emmc-1", info.EmmcVersion)
	}
	if !info.IsSshEnabled {
		t.Error("IsSshEnabled = false, want true")
	}
	if info.BytesAvailable != 12345 {
		t.Errorf("BytesAvailable = %d, want 12345", info.BytesAvailable)
	}
	if len(info.LocalImages) != 1 || info.LocalImages[0].Name != "test.img.xz" {
		t.Errorf("LocalImages = %+v, want one entry named test.img.xz", info.LocalImages)
	}
}

func TestGetSerialNumber(t *testing.T) {
	dir := setupTest(t)
	fakeBin(t, dir, "get-recore-serial-number", `echo "RC-0042"`)

	rr := httptest.NewRecorder()
	getSerialNumber(rr, httptest.NewRequest("GET", "/api/get_serial_number", nil))

	var got GetSerialNumber
	if err := json.Unmarshal(rr.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if got.SerialNumber != "RC-0042" {
		t.Errorf("SerialNumber = %q, want RC-0042", got.SerialNumber)
	}
}

func TestIsUsbPresent(t *testing.T) {
	for _, want := range []bool{true, false} {
		dir := setupTest(t)
		fakeBin(t, dir, "is-usb-present", "echo "+map[bool]string{true: "true", false: "false"}[want])

		rr := httptest.NewRecorder()
		isUsbPresent(rr, httptest.NewRequest("GET", "/api/is_usb_present", nil))

		var got BinaryCommandResult
		if err := json.Unmarshal(rr.Body.Bytes(), &got); err != nil {
			t.Fatal(err)
		}
		if got.Result != want {
			t.Errorf("is-usb-present -> Result = %v, want %v", got.Result, want)
		}
	}
}

func TestGetWifi(t *testing.T) {
	dir := setupTest(t)
	// getWifi runs `get-setting WIFI_SSID`; echo back a known SSID.
	fakeBin(t, dir, "get-setting", `echo "HomeNet"`)

	rr := httptest.NewRecorder()
	getWifi(rr, httptest.NewRequest("GET", "/api/get_wifi", nil))

	var got GetWifi
	if err := json.Unmarshal(rr.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if got.SSID != "HomeNet" {
		t.Errorf("SSID = %q, want HomeNet", got.SSID)
	}
}

func TestSetThenGetOptions(t *testing.T) {
	setupTest(t)

	body := `{"darkmode":false,"screenRotation":270,"SSID":"HomeNet"}`
	rr := httptest.NewRecorder()
	setOptions(rr, httptest.NewRequest("POST", "/api/set_options", strings.NewReader(body)))

	// getOptions should now reflect what we just set.
	rr2 := httptest.NewRecorder()
	getOptions(rr2, httptest.NewRequest("GET", "/api/get_options", nil))

	var got Options
	if err := json.Unmarshal(rr2.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if got.Darkmode != false || got.ScreenRotation != 270 || got.WifiSSID != "HomeNet" {
		t.Errorf("options round-trip mismatch: %+v", got)
	}
}

func TestCheckAutoReboot(t *testing.T) {
	// reboot-board stub records its invocation by creating a marker file.
	arm := func(dir string) string {
		marker := filepath.Join(dir, "rebooted")
		fakeBin(t, dir, "reboot-board", "touch '"+marker+"'") // path may contain ()
		return marker
	}
	rebooted := func(marker string) bool {
		_, err := os.Stat(marker)
		return err == nil
	}

	t.Run("armed + rebootWhenDone + usb removed -> reboots once", func(t *testing.T) {
		dir := setupTest(t)
		marker := arm(dir)
		fakeBin(t, dir, "is-usb-present", `echo false`)
		options.RebootWhenDone = true
		state = &State{State: FINISHED}
		armReboot()

		checkAutoReboot()

		if !rebooted(marker) {
			t.Error("expected reboot-board to be called")
		}
		if isRebootArmed() {
			t.Error("expected disarm after rebooting")
		}
	})

	t.Run("usb still present -> no reboot, stays armed", func(t *testing.T) {
		dir := setupTest(t)
		marker := arm(dir)
		fakeBin(t, dir, "is-usb-present", `echo true`)
		options.RebootWhenDone = true
		state = &State{State: FINISHED}
		armReboot()

		checkAutoReboot()

		if rebooted(marker) {
			t.Error("must not reboot while USB is present")
		}
		if !isRebootArmed() {
			t.Error("should stay armed until USB is removed")
		}
	})

	t.Run("not armed -> no reboot", func(t *testing.T) {
		dir := setupTest(t)
		marker := arm(dir)
		fakeBin(t, dir, "is-usb-present", `echo false`)
		options.RebootWhenDone = true
		state = &State{State: FINISHED}
		disarmReboot()

		checkAutoReboot()

		if rebooted(marker) {
			t.Error("must not reboot when not armed (e.g. after a backup/download)")
		}
	})

	t.Run("rebootWhenDone disabled -> no reboot", func(t *testing.T) {
		dir := setupTest(t)
		marker := arm(dir)
		fakeBin(t, dir, "is-usb-present", `echo false`)
		options.RebootWhenDone = false
		state = &State{State: FINISHED}
		armReboot()

		checkAutoReboot()

		if rebooted(marker) {
			t.Error("must not reboot when user opted out")
		}
	})

	t.Run("active operation blocks reboot", func(t *testing.T) {
		dir := setupTest(t)
		marker := arm(dir)
		fakeBin(t, dir, "is-usb-present", `echo false`)
		options.RebootWhenDone = true
		state = &State{State: BACKUPING}
		armReboot()

		checkAutoReboot()

		if rebooted(marker) {
			t.Error("must not reboot mid-operation")
		}
	})

	t.Run("IDLE still reboots (getProgress reset FINISHED->IDLE)", func(t *testing.T) {
		dir := setupTest(t)
		marker := arm(dir)
		fakeBin(t, dir, "is-usb-present", `echo false`)
		options.RebootWhenDone = true
		state = &State{State: IDLE}
		armReboot()

		checkAutoReboot()

		if !rebooted(marker) {
			t.Error("should still reboot after getProgress flips state to IDLE")
		}
	})
}

func TestHandleSerialCommand(t *testing.T) {
	joined := func(lines []string) string { return strings.Join(lines, "\n") }

	t.Run("LIST returns images then OK", func(t *testing.T) {
		setupTest(t)
		state = &State{State: IDLE}
		os.WriteFile(filepath.Join(images_folder, "a.img.xz"), []byte("x"), 0o644)

		out := handleSerialCommand("LIST")
		if !strings.Contains(joined(out), "IMG a.img.xz 1") {
			t.Errorf("LIST missing image line: %v", out)
		}
		if out[len(out)-1] != "OK" {
			t.Errorf("LIST should end with OK: %v", out)
		}
	})

	t.Run("STATUS reports state and progress", func(t *testing.T) {
		setupTest(t)
		state = &State{State: MAGIC, Progress: 42}
		out := handleSerialCommand("status") // case-insensitive
		if joined(out) != "STATE MAGIC PROGRESS 42" {
			t.Errorf("got %q", joined(out))
		}
	})

	t.Run("FLASH without filename errors", func(t *testing.T) {
		setupTest(t)
		state = &State{State: IDLE}
		if got := handleSerialCommand("FLASH"); got[0] != "ERR missing filename" {
			t.Errorf("got %v", got)
		}
	})

	t.Run("FLASH unknown file errors", func(t *testing.T) {
		setupTest(t)
		state = &State{State: IDLE}
		got := handleSerialCommand("FLASH ghost.img.xz")
		if !strings.HasPrefix(got[0], "ERR no such image") {
			t.Errorf("got %v", got)
		}
	})

	t.Run("FLASH when busy is refused", func(t *testing.T) {
		setupTest(t)
		state = &State{State: BACKUPING}
		os.WriteFile(filepath.Join(images_folder, "a.img.xz"), []byte("x"), 0o644)
		got := handleSerialCommand("FLASH a.img.xz")
		if !strings.HasPrefix(got[0], "ERR busy") {
			t.Errorf("got %v", got)
		}
	})

	t.Run("FLASH valid starts install", func(t *testing.T) {
		setupTest(t)
		state = &State{State: IDLE}
		os.WriteFile(filepath.Join(images_folder, "a.img.xz"), []byte("x"), 0o644)

		// Stub the launcher so the test stays synchronous (no real goroutine).
		var flashed string
		orig := startInstall
		startInstall = func(f string) { flashed = f }
		defer func() { startInstall = orig }()

		got := handleSerialCommand("FLASH a.img.xz")
		if got[0] != "OK flashing a.img.xz" {
			t.Errorf("got %v", got)
		}
		if flashed != "a.img.xz" {
			t.Errorf("install not started for %q (got %q)", "a.img.xz", flashed)
		}
		if state.State != INSTALLING {
			t.Errorf("state = %q, want INSTALLING", state.State)
		}
	})

	t.Run("unknown command errors", func(t *testing.T) {
		setupTest(t)
		state = &State{State: IDLE}
		if got := handleSerialCommand("WIBBLE"); !strings.HasPrefix(got[0], "ERR unknown command") {
			t.Errorf("got %v", got)
		}
	})
}

func TestParseXzUncompressedSize(t *testing.T) {
	// `xz --robot -l` totals line: tab-separated, uncompressed bytes in field 4.
	// Exact regardless of size — a 2 MiB file reports 2097152, not "2.0 MiB".
	robot := "name\tz.bin.xz\n" +
		"file\t1\t1\t448\t2097152\t0.000\tCRC64\t0\n" +
		"totals\t1\t1\t448\t2097152\t0.000\tCRC64\t0\t1\n"
	if got := parseXzUncompressedSize(robot); got != 2097152 {
		t.Errorf("robot totals: got %d, want 2097152", got)
	}

	// Garbage / empty input must not panic and must return 0.
	if got := parseXzUncompressedSize(""); got != 0 {
		t.Errorf("empty input: got %d, want 0", got)
	}
}

// End-to-end against the real xz binary: a small file used to panic the old
// MiB-splitting parser; now it returns the exact uncompressed size.
func TestGetUncompressedSizeRealXz(t *testing.T) {
	dir := t.TempDir()
	raw := filepath.Join(dir, "payload.bin")
	const size = 2 * 1024 * 1024
	if err := os.WriteFile(raw, make([]byte, size), 0o644); err != nil {
		t.Fatal(err)
	}
	xzPath := raw + ".xz"
	cmd := exec.Command("xz", "-k", "-f", raw)
	if err := cmd.Run(); err != nil {
		t.Skipf("xz not available: %v", err)
	}
	if got := getUncompressedSize(xzPath); got != size {
		t.Errorf("getUncompressedSize = %d, want %d", got, size)
	}
}

// End-to-end against the real handlers: drives upload_start -> several
// upload_chunk calls -> upload_finish exactly like the client does, and
// checks the file on disk matches byte-for-byte. Exercises the real
// state.File handle kept open across chunks (rather than the old
// open/write/close-per-chunk code), so this would catch truncation,
// overwriting, or interleaving bugs a build-only check can't.
func TestUploadChunkRoundTrip(t *testing.T) {
	setupTest(t)
	state = &State{State: IDLE}

	filename := "roundtrip-test.img.xz"
	payload := bytes.Repeat([]byte("The quick brown fox jumps over the lazy dog. "), 5000) // ~225KB

	startBody, _ := json.Marshal(map[string]any{
		"filename":   filename,
		"size":       len(payload),
		"start_time": 0,
	})
	w := httptest.NewRecorder()
	uploadStart(w, httptest.NewRequest("PUT", "/api/upload_start", bytes.NewReader(startBody)))
	if state.File == nil {
		t.Fatal("uploadStart did not open state.File")
	}

	const chunkSize = 10 * 1024 // small relative to payload so this genuinely exercises multiple sequential chunk writes, not just one
	for i := 0; i < len(payload); i += chunkSize {
		end := i + chunkSize
		if end > len(payload) {
			end = len(payload)
		}
		w := httptest.NewRecorder()
		uploadChunk(w, httptest.NewRequest("POST", "/api/upload_chunk", bytes.NewReader(payload[i:end])))

		var resp map[string]bool
		if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
			t.Fatalf("chunk at offset %d: bad response body: %v", i, err)
		}
		if !resp["success"] {
			t.Fatalf("chunk at offset %d: server reported failure", i)
		}
	}

	uploadFinish(httptest.NewRecorder(), httptest.NewRequest("PUT", "/api/upload_finish", nil))

	if state.File != nil {
		t.Error("uploadFinish did not clear state.File")
	}

	got, err := os.ReadFile(filepath.Join(images_folder, filename))
	if err != nil {
		t.Fatalf("reading uploaded file: %v", err)
	}
	if !bytes.Equal(got, payload) {
		t.Errorf("uploaded file mismatch: got %d bytes, want %d bytes", len(got), len(payload))
	}
	if state.BytesNow != len(payload) {
		t.Errorf("state.BytesNow = %d, want %d", state.BytesNow, len(payload))
	}
}
