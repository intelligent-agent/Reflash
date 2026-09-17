package main

import (
	"encoding/json"
	"io"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

// /etc/rebuild-settings is sourced by bash as root on the flashed image. What
// goes in has to come back out as the same values, whatever the user typed.
func TestInstallSettingsSurviveBeingSourced(t *testing.T) {
	dir := setupTest(t)
	written := filepath.Join(dir, "rebuild-settings")
	pwned := filepath.Join(dir, "pwned")
	fakeBin(t, dir, "save-settings", `printf '%s\n' "$1" > `+written)
	fakeBin(t, dir, "rotate-screen", `exit 0`)
	fakeBin(t, dir, "mount-unmount-usb", `exit 0`)

	ssid := `Bob's "home" net`
	psk := `it's-my-wifi\n'$(touch ` + pwned + `)'`
	options = &Options{WifiSSID: ssid, WifiPSK: psk, EnableSsh: true, ScreenRotation: 90}

	runInstallFinishedCommands(httptest.NewRecorder(), httptest.NewRequest("GET", "/api/run_install_finished_commands", nil))

	out, err := exec.Command("bash", "-c", `. "$1" && printf '%s\n%s' "$WIFI_SSID" "$WIFI_PSK"`, "_", written).CombinedOutput()
	if err != nil {
		t.Fatalf("sourcing the settings failed: %v\n%s", err, out)
	}
	if want := ssid + "\n" + psk; string(out) != want {
		t.Errorf("sourced values = %q, want %q", out, want)
	}
	if _, err := os.Stat(pwned); err == nil {
		t.Error("sourcing the settings ran a command from the passphrase")
	}
}

// A failed rotate step used to write a response and carry on, so the reply held
// several JSON bodies and the client could parse none of them.
func TestInstallFinishedCommandsRespondOnce(t *testing.T) {
	dir := setupTest(t)
	fakeBin(t, dir, "rotate-screen", `exit 1`)
	fakeBin(t, dir, "save-settings", `exit 0`)
	fakeBin(t, dir, "mount-unmount-usb", `exit 0`)
	options = &Options{}

	w := httptest.NewRecorder()
	runInstallFinishedCommands(w, httptest.NewRequest("GET", "/api/run_install_finished_commands", nil))

	body := w.Body.String()
	dec := json.NewDecoder(w.Body)
	var resp StatusResult
	if err := dec.Decode(&resp); err != nil {
		t.Fatalf("no JSON response: %v", err)
	}
	if resp.Status != "ERROR" {
		t.Errorf("a failed rotate answered %+v, want ERROR", resp)
	}
	if err := dec.Decode(&resp); err != io.EOF {
		t.Errorf("more than one response body in the reply: %s", body)
	}
}
