package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
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
	reflashVersion = ""
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
	// Read once at startup, not per request - so seed the cache, and install no
	// get-reflash-version stub. If the handler shells out it will fail, which is
	// the point.
	reflashVersion = "v9.9.9"
	fakeBin(t, dir, "get-recore-revision", `echo "A8"`)
	fakeBin(t, dir, "get-recore-serial-number", `echo "RC-0042"`)
	fakeBin(t, dir, "get-emmc-version", `echo "emmc-1"`)

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
}

func TestGetStatus(t *testing.T) {
	dir := setupTest(t)
	fakeBin(t, dir, "get-free-space", `echo "12345"`)
	fakeBin(t, dir, "network-status", `echo '{"ethernet":{"up":true,"ip":"192.168.1.42"},`+
		`"wifi":{"present":false}}'`)
	if err := os.WriteFile(filepath.Join(images_folder, "test.img.xz"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}

	rr := httptest.NewRecorder()
	getStatus(rr, httptest.NewRequest("GET", "/api/get_status", nil))

	var status GetStatus
	if err := json.Unmarshal(rr.Body.Bytes(), &status); err != nil {
		t.Fatalf("decode: %v (body=%q)", err, rr.Body.String())
	}
	if status.BytesAvailable != 12345 {
		t.Errorf("BytesAvailable = %d, want 12345", status.BytesAvailable)
	}
	if len(status.LocalImages) != 1 || status.LocalImages[0].Name != "test.img.xz" {
		t.Errorf("LocalImages = %+v, want one entry named test.img.xz", status.LocalImages)
	}
	if !status.Network.Ethernet.Up {
		t.Errorf("Network = %+v, want ethernet up", status.Network)
	}
}

// The split only pays off if the expensive half stays out of the polled half:
// get-recore-revision, get-recore-serial-number and get-emmc-version each mount
// a partition.
func TestGetStatusMountsNothing(t *testing.T) {
	dir := setupTest(t)
	fakeBin(t, dir, "get-free-space", `echo "1"`)
	fakeBin(t, dir, "network-status", `echo '{}'`)

	ran := filepath.Join(dir, "ran")
	for _, name := range []string{"get-recore-revision", "get-recore-serial-number",
		"get-emmc-version", "get-reflash-version", "get-hostnames", "is-ssh-enabled"} {
		fakeBin(t, dir, name, `echo `+name+` >> `+ran)
	}

	rr := httptest.NewRecorder()
	getStatus(rr, httptest.NewRequest("GET", "/api/get_status", nil))

	if b, err := os.ReadFile(ran); err == nil {
		t.Errorf("get_status ran mounting/dead helpers: %s", b)
	}
}

func TestGetNetworkStatus(t *testing.T) {
	t.Run("passes the helper's JSON through", func(t *testing.T) {
		dir := setupTest(t)
		fakeBin(t, dir, "network-status", `echo '{"ethernet":{"up":true,"ip":"192.168.1.42","active":false},`+
			`"wifi":{"present":true,"mode":"station","ssid":"HomeNet","ip":"192.168.1.87",`+
			`"rssi":-55,"active":true}}'`)

		got := getNetworkStatus()
		if !got.Ethernet.Up || got.Ethernet.IP != "192.168.1.42" {
			t.Errorf("Ethernet = %+v", got.Ethernet)
		}
		if got.Wifi.Mode != "station" || got.Wifi.SSID != "HomeNet" || got.Wifi.IP != "192.168.1.87" {
			t.Errorf("Wifi = %+v", got.Wifi)
		}
		// Both up, and only these say which one carries traffic (#112).
		if got.Ethernet.Active || !got.Wifi.Active {
			t.Errorf("active flags: eth=%v wifi=%v, want false/true",
				got.Ethernet.Active, got.Wifi.Active)
		}
		if got.Wifi.RSSI != -55 {
			t.Errorf("RSSI = %d, want -55", got.Wifi.RSSI)
		}
	})

	// The info panel carries the serial number and versions too, so a broken
	// helper must not cost the user all of it.
	t.Run("a failing helper yields a zero status, not an error", func(t *testing.T) {
		dir := setupTest(t)
		fakeBin(t, dir, "network-status", `exit 1`)
		if got := getNetworkStatus(); got.Ethernet.Up || got.Wifi.Present {
			t.Errorf("got %+v, want zero value", got)
		}
	})

	t.Run("unparseable output yields a zero status", func(t *testing.T) {
		dir := setupTest(t)
		fakeBin(t, dir, "network-status", `echo 'not json'`)
		if got := getNetworkStatus(); got.Ethernet.Up || got.Wifi.Present {
			t.Errorf("got %+v, want zero value", got)
		}
	})
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
	setupTest(t)
	// The SSID comes from the options the server already holds - no helper, no
	// eMMC mount.
	options.WifiSSID = "HomeNet"
	options.WifiPSK = "hunter2secret"

	rr := httptest.NewRecorder()
	getWifi(rr, httptest.NewRequest("GET", "/api/get_wifi", nil))

	var got GetWifi
	if err := json.Unmarshal(rr.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if got.SSID != "HomeNet" {
		t.Errorf("SSID = %q, want HomeNet", got.SSID)
	}
	// The dialog repopulates the passphrase from its own memory; the server
	// must never send it to the browser.
	if strings.Contains(rr.Body.String(), "hunter2secret") {
		t.Errorf("passphrase sent to the client: %s", rr.Body.String())
	}
}

func TestRunCommand2KeepsSecretsOutOfTheLog(t *testing.T) {
	dir := setupTest(t)
	// A helper that fails is what makes runCommand2 log the argv it ran.
	fakeBin(t, dir, "wifi-connect", `exit 1`)
	fakeBin(t, dir, "save-settings", `exit 1`)

	runCommand2("wifi-connect", "HomeNet", "hunter2secret")
	runCommand2("save-settings", "WIFI_PSK='hunter2secret'")

	logged, err := os.ReadFile(log_file)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(logged), "hunter2secret") {
		t.Errorf("passphrase leaked into %s:\n%s", log_file, logged)
	}
	// The SSID is not a secret and stays, so the log is still useful.
	if !strings.Contains(string(logged), "HomeNet") {
		t.Errorf("SSID missing from log, redaction too broad:\n%s", logged)
	}
}

func TestWifiAdapterPresent(t *testing.T) {
	sysNet := t.TempDir()
	t.Setenv("SYS_NET", sysNet)
	t.Setenv("WIFI_INTERFACE", "wlan0")

	if wifiAdapterPresent() {
		t.Error("no adapter directory, want false")
	}
	if err := os.Mkdir(filepath.Join(sysNet, "wlan0"), 0o755); err != nil {
		t.Fatal(err)
	}
	if !wifiAdapterPresent() {
		t.Error("adapter directory exists, want true")
	}
}

func TestBringupWifiSkippedWithoutAdapter(t *testing.T) {
	dir := setupTest(t)
	t.Setenv("SYS_NET", t.TempDir())
	t.Setenv("WIFI_INTERFACE", "wlan0")
	options.WifiSSID = "HomeNet"
	options.WifiPSK = "hunter2secret"

	// Both helpers record that they ran; neither should.
	ran := filepath.Join(dir, "ran")
	fakeBin(t, dir, "wifi-connect", `echo connect >> `+ran)
	fakeBin(t, dir, "wifi-bringup", `echo bringup >> `+ran)

	bringupWifi()

	if _, err := os.Stat(ran); err == nil {
		t.Error("bringupWifi ran a wifi helper with no adapter present")
	}
}

func TestStartHotspotWifi(t *testing.T) {
	t.Run("refuses with no adapter instead of shelling out", func(t *testing.T) {
		dir := setupTest(t)
		t.Setenv("SYS_NET", t.TempDir())
		t.Setenv("WIFI_INTERFACE", "wlan0")
		ran := filepath.Join(dir, "ran")
		fakeBin(t, dir, "wifi-hotspot", `echo ran >> `+ran)

		rr := httptest.NewRecorder()
		startHotspotWifi(rr, httptest.NewRequest("POST", "/api/wifi_start_hotspot", nil))

		if rr.Code != http.StatusBadRequest {
			t.Errorf("status = %d, want 400", rr.Code)
		}
		if _, err := os.Stat(ran); err == nil {
			t.Error("ran wifi-hotspot with no adapter present")
		}
	})

	// 202 before the work starts, not after: the switch takes down the origin
	// this request arrived on if the caller is on WiFi.
	t.Run("accepts and runs the helper when an adapter is fitted", func(t *testing.T) {
		dir := setupTest(t)
		sysNet := t.TempDir()
		if err := os.Mkdir(filepath.Join(sysNet, "wlan0"), 0o755); err != nil {
			t.Fatal(err)
		}
		t.Setenv("SYS_NET", sysNet)
		t.Setenv("WIFI_INTERFACE", "wlan0")
		ran := filepath.Join(dir, "ran")
		fakeBin(t, dir, "wifi-hotspot", `echo ran >> `+ran)

		rr := httptest.NewRecorder()
		startHotspotWifi(rr, httptest.NewRequest("POST", "/api/wifi_start_hotspot", nil))

		if rr.Code != http.StatusAccepted {
			t.Errorf("status = %d, want 202", rr.Code)
		}
		for i := 0; i < 100; i++ {
			if _, err := os.Stat(ran); err == nil {
				return
			}
			time.Sleep(10 * time.Millisecond)
		}
		t.Error("wifi-hotspot never ran")
	})
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

// startControlSocket brings up the control socket on a throwaway path and
// waits for it to accept, so tests do not race the listener goroutine.
func startControlSocket(t *testing.T) string {
	t.Helper()
	// The socket path goes in the sandbox, not $TMPDIR directly: unix socket
	// paths are capped at ~108 bytes.
	sock := filepath.Join(t.TempDir(), "c.sock")
	t.Setenv("REFLASH_CONTROL_SOCKET", sock)
	go serveControlSocket(sock)

	for i := 0; i < 100; i++ {
		if c, err := net.Dial("unix", sock); err == nil {
			c.Close()
			return sock
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("control socket never came up")
	return ""
}

// The two transports must stay byte-compatible apart from their line ending:
// the ACM tty is a serial line that flasher-pi reads as CRLF, the socket is not.
func TestServeControlConnLineEndings(t *testing.T) {
	for _, tc := range []struct{ name, eol string }{
		{"tty uses CRLF", "\r\n"},
		{"socket uses LF", "\n"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			setupTest(t)
			state = &State{State: IDLE, Progress: 5}

			var out bytes.Buffer
			serveControlConn(struct {
				io.Reader
				io.Writer
			}{strings.NewReader("STATUS\n"), &out}, tc.eol)

			if got, want := out.String(), "STATE IDLE PROGRESS 5"+tc.eol; got != want {
				t.Errorf("got %q, want %q", got, want)
			}
		})
	}
}

func TestControlSocket(t *testing.T) {
	t.Run("serves the line protocol", func(t *testing.T) {
		setupTest(t)
		state = &State{State: MAGIC, Progress: 42}
		os.WriteFile(filepath.Join(images_folder, "a.img.xz"), []byte("x"), 0o644)
		sock := startControlSocket(t)

		conn, err := net.Dial("unix", sock)
		if err != nil {
			t.Fatal(err)
		}
		defer conn.Close()

		fmt.Fprint(conn, "LIST\nSTATUS\n")
		conn.(*net.UnixConn).CloseWrite() // half-close frames the response

		out, err := io.ReadAll(conn)
		if err != nil {
			t.Fatal(err)
		}
		got := string(out)
		for _, want := range []string{"IMG a.img.xz 1", "OK", "STATE MAGIC PROGRESS 42"} {
			if !strings.Contains(got, want) {
				t.Errorf("response missing %q:\n%s", want, got)
			}
		}
		// Serial framing has no business on a socket.
		if strings.Contains(got, "\r") {
			t.Errorf("response contains CR: %q", got)
		}
	})

	t.Run("client sends one command and prints the reply", func(t *testing.T) {
		setupTest(t)
		state = &State{State: IDLE, Progress: 7}
		startControlSocket(t)

		var out bytes.Buffer
		code := runControlClientIO(strings.NewReader(""), &out, []string{"STATUS"})

		if code != 0 {
			t.Errorf("exit code = %d, want 0", code)
		}
		if strings.TrimSpace(out.String()) != "STATE IDLE PROGRESS 7" {
			t.Errorf("got %q", out.String())
		}
	})

	t.Run("client with no arguments relays stdin", func(t *testing.T) {
		setupTest(t)
		state = &State{State: IDLE, Progress: 3}
		startControlSocket(t)

		var out bytes.Buffer
		code := runControlClientIO(strings.NewReader("STATUS\nWIBBLE\n"), &out, nil)

		if code != 0 {
			t.Errorf("exit code = %d, want 0", code)
		}
		if !strings.Contains(out.String(), "STATE IDLE PROGRESS 3") ||
			!strings.Contains(out.String(), "ERR unknown command") {
			t.Errorf("got %q", out.String())
		}
	})

	t.Run("client reports a missing server instead of hanging", func(t *testing.T) {
		setupTest(t)
		t.Setenv("REFLASH_CONTROL_SOCKET", filepath.Join(t.TempDir(), "absent.sock"))
		if code := runControlClient([]string{"STATUS"}); code != 1 {
			t.Errorf("exit code = %d, want 1", code)
		}
	})

	t.Run("a stale socket file does not block startup", func(t *testing.T) {
		setupTest(t)
		state = &State{State: IDLE}
		sock := filepath.Join(t.TempDir(), "c.sock")
		// What a previous run or a crash leaves behind.
		if err := os.WriteFile(sock, nil, 0o644); err != nil {
			t.Fatal(err)
		}
		t.Setenv("REFLASH_CONTROL_SOCKET", sock)
		go serveControlSocket(sock)

		var conn net.Conn
		for i := 0; i < 100; i++ {
			if c, err := net.Dial("unix", sock); err == nil {
				conn = c
				break
			}
			time.Sleep(10 * time.Millisecond)
		}
		if conn == nil {
			t.Fatal("listener never bound over the stale socket")
		}
		conn.Close()
	})
}

func TestControlSocketPath(t *testing.T) {
	t.Run("prod default", func(t *testing.T) {
		t.Setenv("REFLASH_CONTROL_SOCKET", "")
		t.Setenv("APP_ENV", "")
		if got := controlSocketPath(); got != "/run/reflash/control.sock" {
			t.Errorf("got %q", got)
		}
	})
	t.Run("env override wins", func(t *testing.T) {
		t.Setenv("REFLASH_CONTROL_SOCKET", "/tmp/x.sock")
		t.Setenv("APP_ENV", "dev")
		if got := controlSocketPath(); got != "/tmp/x.sock" {
			t.Errorf("got %q", got)
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
