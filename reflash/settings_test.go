package main

import (
	"encoding/json"
	"io"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// /etc/rebuild-settings is sourced by bash as root on the flashed image. What
// goes in has to come back out as the same values, whatever the user typed.
func TestInstallSettingsSurviveBeingSourced(t *testing.T) {
	dir := setupTest(t)
	written := filepath.Join(dir, "rebuild-settings")
	pwned := filepath.Join(dir, "pwned")
	fakeBin(t, dir, "target-manifest", `exit 0`)
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
	fakeBin(t, dir, "target-manifest", `exit 0`)
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

// An image with a target manifest gets its settings through its own installer
// (#179): literal KEY=VALUE lines in a file, never on the command line, and
// none of Reflash's own Rebuild v1.1 steps.
func TestInstallFinishedHandsSettingsToTheImagesInstaller(t *testing.T) {
	dir := setupTest(t)
	got := filepath.Join(dir, "settings-seen")
	args := filepath.Join(dir, "args-seen")
	fakeBin(t, dir, "target-manifest", `printf 'interface=1\nroot=2\n'`)
	fakeBin(t, dir, "target-install", `echo "$*" > `+args+`; cat "$2" > `+got+`; stat -c %a "$2" >> `+args)
	fakeBin(t, dir, "rotate-screen", `echo called > `+filepath.Join(dir, "legacy"))
	fakeBin(t, dir, "save-settings", `echo called > `+filepath.Join(dir, "legacy"))
	fakeBin(t, dir, "mount-unmount-usb", `exit 0`)

	options = &Options{WifiSSID: `Bob's "home" net`, WifiPSK: `it's $(my) wifi`, EnableSsh: true, ScreenRotation: 270}
	w := httptest.NewRecorder()
	runInstallFinishedCommands(w, httptest.NewRequest("GET", "/api/run_install_finished_commands", nil))

	var resp StatusResult
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil || resp.Status == "ERROR" {
		t.Fatalf("response %+v, %v", resp, err)
	}
	settings, err := os.ReadFile(got)
	if err != nil {
		t.Fatal(err)
	}
	want := "SETTINGS=1\nSSH_ENABLED=true\nSCREEN_ROTATION=270\n" +
		"WIFI_SSID=Bob's \"home\" net\nWIFI_PSK=it's $(my) wifi\n"
	if string(settings) != want {
		t.Errorf("settings = %q, want %q", settings, want)
	}
	a, _ := os.ReadFile(args)
	if !strings.HasPrefix(string(a), "configure ") || strings.Contains(string(a), "wifi") {
		t.Errorf("target-install called as %q", a)
	}
	if !strings.HasSuffix(strings.TrimSpace(string(a)), "600") {
		t.Errorf("settings file mode: %q, want 600", a)
	}
	if _, err := os.Stat(filepath.Join(dir, "legacy")); err == nil {
		t.Error("the Rebuild v1.1 rotate/save-settings path ran for an image with a manifest")
	}
}

// A line break in a value would start a settings line of the user's making.
func TestInstallFinishedRefusesALineBreakInASetting(t *testing.T) {
	dir := setupTest(t)
	ran := filepath.Join(dir, "ran")
	fakeBin(t, dir, "target-manifest", `printf 'interface=1\n'`)
	fakeBin(t, dir, "target-install", `touch `+ran)
	fakeBin(t, dir, "mount-unmount-usb", `exit 0`)
	options = &Options{WifiSSID: "net", WifiPSK: "pass\nSSH_ENABLED=true"}

	w := httptest.NewRecorder()
	runInstallFinishedCommands(w, httptest.NewRequest("GET", "/api/run_install_finished_commands", nil))

	var resp StatusResult
	json.NewDecoder(w.Body).Decode(&resp)
	if resp.Status != "ERROR" {
		t.Errorf("answered %+v, want ERROR", resp)
	}
	if _, err := os.Stat(ran); err == nil {
		t.Error("the installer was run with a settings value containing a line break")
	}
}

// The installer's reason reaches the user, not just "exit status 1".
func TestInstallFinishedReportsTheInstallersReason(t *testing.T) {
	dir := setupTest(t)
	fakeBin(t, dir, "target-manifest", `printf 'interface=1\n'`)
	fakeBin(t, dir, "target-install", `echo "FATAL: the image's installer failed (exit 1): no Weston config"; exit 1`)
	fakeBin(t, dir, "mount-unmount-usb", `exit 0`)
	options = &Options{}

	w := httptest.NewRecorder()
	runInstallFinishedCommands(w, httptest.NewRequest("GET", "/api/run_install_finished_commands", nil))

	if body := w.Body.String(); !strings.Contains(body, "no Weston config") {
		t.Errorf("response %s does not carry the installer's reason", body)
	}
}
