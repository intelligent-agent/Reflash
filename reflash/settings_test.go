package main

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/pelletier/go-toml/v2"
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

// A failed preparation shows the user why, not "Check log for details" (#179).
func TestPreparationFailureSurfacesTheReason(t *testing.T) {
	out := "Target manifest: ...\n  installer: working\nFATAL: the image cannot be installed by this Reflash: interface '2' is not supported by this Reflash (it knows 1)\n"
	if got := preparationFailure(out, "generic"); got != "the image cannot be installed by this Reflash: interface '2' is not supported by this Reflash (it knows 1)" {
		t.Errorf("got %q", got)
	}
	if got := preparationFailure("some output\nexit 1\n", "generic"); got != "generic" {
		t.Errorf("without a FATAL line got %q, want the fallback", got)
	}
}

// An image's manifest says which settings it applies; others are not sent,
// and an image without settings= gets the original four.
func TestTargetSettingsFollowTheManifest(t *testing.T) {
	options = &Options{WifiSSID: "net", WifiPSK: "pass", EnableSsh: true, ScreenRotation: 90}

	got, err := targetSettings(manifestSettings("interface=1\nsettings=SCREEN_ROTATION,LOGIN_PASSWORD\n"))
	if err != nil || got != "SETTINGS=1\nSCREEN_ROTATION=90\n" {
		t.Errorf("with settings= got %q, %v", got, err)
	}
	got, _ = targetSettings(manifestSettings("interface=1\nroot=2\n"))
	if want := "SETTINGS=1\nSSH_ENABLED=true\nSCREEN_ROTATION=90\nWIFI_SSID=net\nWIFI_PSK=pass\n"; got != want {
		t.Errorf("without settings= got %q, want %q", got, want)
	}
}

// The login password lives in memory only: not in options.cfg on the USB
// drive, and never sent back by get_options - only whether one is set (#182).
func TestLoginPasswordIsNeverStoredOrReturned(t *testing.T) {
	setupTest(t)
	options = &Options{WifiSSID: "net"}
	if err := lockSetOptions([]byte(`{"loginPassword":"correct horse"}`)); err != nil {
		t.Fatal(err)
	}
	if options.LoginPassword != "correct horse" {
		t.Fatalf("not set: %+v", options)
	}
	saved, _ := toml.Marshal(options)
	if strings.Contains(string(saved), "horse") {
		t.Errorf("the password would be written to options.cfg:\n%s", saved)
	}
	w := httptest.NewRecorder()
	writeOptions(w)
	body := w.Body.String()
	if strings.Contains(body, "horse") || !strings.Contains(body, `"loginPasswordSet":true`) {
		t.Errorf("get_options answered %s", body)
	}
}

// Sent to an image that takes it, and then forgotten: the next board starts
// afresh.
func TestLoginPasswordGoesToTheImageAndIsForgotten(t *testing.T) {
	dir := setupTest(t)
	got := filepath.Join(dir, "settings-seen")
	fakeBin(t, dir, "target-manifest", `printf 'interface=1\nsettings=SSH_ENABLED,LOGIN_PASSWORD\n'`)
	fakeBin(t, dir, "target-install", `cat "$2" > `+got)
	fakeBin(t, dir, "mount-unmount-usb", `exit 0`)
	options = &Options{EnableSsh: true, LoginPassword: "correct horse"}

	w := httptest.NewRecorder()
	runInstallFinishedCommands(w, httptest.NewRequest("GET", "/api/run_install_finished_commands", nil))

	if b := w.Body.String(); strings.Contains(b, "ERROR") {
		t.Fatalf("answered %s", b)
	}
	s, _ := os.ReadFile(got)
	if string(s) != "SETTINGS=1\nSSH_ENABLED=true\nLOGIN_PASSWORD=correct horse\n" {
		t.Errorf("settings = %q", s)
	}
	if options.LoginPassword != "" {
		t.Error("the password was kept after the image took it")
	}
}

// An image that cannot take a password - one whose manifest does not list
// it, or one with no manifest at all - is said to, not silently skipped.
func TestLoginPasswordForAnImageThatCannotTakeIt(t *testing.T) {
	for name, manifest := range map[string]string{
		"manifest without LOGIN_PASSWORD": `printf 'interface=1\n'`,
		"no manifest":                     `exit 0`,
	} {
		t.Run(name, func(t *testing.T) {
			dir := setupTest(t)
			got := filepath.Join(dir, "settings-seen")
			fakeBin(t, dir, "target-manifest", manifest)
			fakeBin(t, dir, "target-install", `cat "$2" > `+got)
			fakeBin(t, dir, "rotate-screen", `exit 0`)
			fakeBin(t, dir, "save-settings", `exit 0`)
			fakeBin(t, dir, "mount-unmount-usb", `exit 0`)
			options = &Options{LoginPassword: "correct horse"}

			w := httptest.NewRecorder()
			runInstallFinishedCommands(w, httptest.NewRequest("GET", "/api/run_install_finished_commands", nil))

			if b := w.Body.String(); !strings.Contains(b, "keeps its factory password") {
				t.Errorf("answered %s", b)
			}
			if s, _ := os.ReadFile(got); strings.Contains(string(s), "horse") {
				t.Errorf("sent the password to an image that does not take it: %q", s)
			}
		})
	}
}

// #173: read the installed system's settings through its installer, never a
// secret among them, and change only the ones the user changed.
func TestInstalledSettingsRead(t *testing.T) {
	dir := setupTest(t)
	state = &State{State: IDLE}
	fakeBin(t, dir, "target-manifest", `printf 'interface=1\nsettings=SSH_ENABLED,WIFI_SSID,WIFI_PSK,LOGIN_PASSWORD\nactions=settings\n'`)
	fakeBin(t, dir, "target-install", `[ "$1" = settings ] && printf 'SETTINGS=1\nSSH_ENABLED=true\nWIFI_SSID=home\nSCREEN_ROTATION=90\n'`)

	w := httptest.NewRecorder()
	installedSettings(w, httptest.NewRequest("GET", "/api/installed_settings", nil))

	var got InstalledSettings
	if err := json.NewDecoder(w.Body).Decode(&got); err != nil {
		t.Fatal(err)
	}
	if !got.Supported || strings.Join(got.Keys, ",") != "LOGIN_PASSWORD,SSH_ENABLED,WIFI_PSK,WIFI_SSID" {
		t.Errorf("got %+v", got)
	}
	// SCREEN_ROTATION is not in this image's settings=, so it is not offered.
	if len(got.Current) != 2 || got.Current["WIFI_SSID"] != "home" || got.Current["SSH_ENABLED"] != "true" {
		t.Errorf("current = %v", got.Current)
	}
}

func TestInstalledSettingsChangeOnlyWhatIsGiven(t *testing.T) {
	dir := setupTest(t)
	state = &State{State: IDLE}
	seen := filepath.Join(dir, "seen")
	fakeBin(t, dir, "target-manifest", `printf 'interface=1\nsettings=SSH_ENABLED,LOGIN_PASSWORD\n'`)
	fakeBin(t, dir, "target-install", `cat "$2" > `+seen)

	w := httptest.NewRecorder()
	installedSettings(w, httptest.NewRequest("POST", "/api/installed_settings",
		strings.NewReader(`{"LOGIN_PASSWORD":"correct horse"}`)))
	if b := w.Body.String(); !strings.Contains(b, `"OK"`) {
		t.Fatalf("answered %s", b)
	}
	if s, _ := os.ReadFile(seen); string(s) != "SETTINGS=1\nLOGIN_PASSWORD=correct horse\n" {
		t.Errorf("sent %q", s)
	}

	w = httptest.NewRecorder()
	installedSettings(w, httptest.NewRequest("POST", "/api/installed_settings",
		strings.NewReader(`{"WIFI_SSID":"home"}`)))
	if b := w.Body.String(); !strings.Contains(b, "cannot change WIFI_SSID") {
		t.Errorf("a key the image does not list answered %s", b)
	}
}

func TestInstalledSettingsTooOldOrBusy(t *testing.T) {
	dir := setupTest(t)
	fakeBin(t, dir, "target-manifest", `exit 0`)
	state = &State{State: IDLE}
	w := httptest.NewRecorder()
	installedSettings(w, httptest.NewRequest("GET", "/api/installed_settings", nil))
	var got InstalledSettings
	json.NewDecoder(w.Body).Decode(&got)
	if got.Supported || !strings.Contains(got.Reason, "too old") {
		t.Errorf("an image without a manifest answered %+v", got)
	}

	state = &State{State: INSTALLING}
	w = httptest.NewRecorder()
	installedSettings(w, httptest.NewRequest("GET", "/api/installed_settings", nil))
	if w.Code != http.StatusConflict {
		t.Errorf("while installing: %d %s", w.Code, w.Body.String())
	}
}

// #175: a backup of the installed system's files is made by its installer,
// listed, and downloadable only by a name that cannot leave the folder.
func TestFileBackupsMakeListDownload(t *testing.T) {
	dir := setupTest(t)
	fakeBin(t, dir, "target-manifest", `printf 'interface=1\nactions=backup,restore\n'`)
	state = &State{State: IDLE}
	fakeBin(t, dir, "get-emmc-version", `echo "rebuild-fluidd-v1.2.0"`)
	fakeBin(t, dir, "mount-unmount-usb", `exit 0`)
	fakeBin(t, dir, "target-install", `[ "$1" = backup ] && printf ARCHIVE > "$2"`)

	w := httptest.NewRecorder()
	fileBackups(w, httptest.NewRequest("POST", "/api/file_backups", nil))
	var made map[string]string
	json.NewDecoder(w.Body).Decode(&made)
	if made["status"] != "OK" || !strings.HasPrefix(made["name"], "rebuild-fluidd-v1.2.0-files-") {
		t.Fatalf("made %v", made)
	}

	w = httptest.NewRecorder()
	fileBackups(w, httptest.NewRequest("GET", "/api/file_backups", nil))
	var list []FileBackup
	json.NewDecoder(w.Body).Decode(&list)
	if len(list) != 1 || list[0].Name != made["name"] || list[0].Size != 7 {
		t.Errorf("list %v", list)
	}

	w = httptest.NewRecorder()
	downloadFileBackup(w, httptest.NewRequest("GET", "/api/file_backups/download?name="+made["name"], nil))
	if w.Body.String() != "ARCHIVE" {
		t.Errorf("download %q", w.Body.String())
	}
	w = httptest.NewRecorder()
	downloadFileBackup(w, httptest.NewRequest("GET", "/api/file_backups/download?name=../options.cfg", nil))
	if w.Code != http.StatusNotFound {
		t.Errorf("a name outside the folder answered %d", w.Code)
	}
}

func TestFileBackupNotSupported(t *testing.T) {
	dir := setupTest(t)
	fakeBin(t, dir, "target-manifest", `printf 'interface=1\nactions=backup,restore\n'`)
	state = &State{State: IDLE}
	fakeBin(t, dir, "get-emmc-version", `echo rebuild`)
	fakeBin(t, dir, "mount-unmount-usb", `exit 0`)
	fakeBin(t, dir, "target-install", `exit 3`)
	w := httptest.NewRecorder()
	fileBackups(w, httptest.NewRequest("POST", "/api/file_backups", nil))
	if b := w.Body.String(); !strings.Contains(b, "does not support backing up") {
		t.Errorf("answered %s", b)
	}
	if left, _ := filepath.Glob(backups_folder + "/*"); len(left) != 0 {
		t.Errorf("left %v behind", left)
	}
}

// The chosen backup goes into the next installed system after prepare and
// before configure, and only once.
func TestRestoreBackupAtInstall(t *testing.T) {
	dir := setupTest(t)
	order := filepath.Join(dir, "order")
	os.MkdirAll(backups_folder, 0o755)
	os.WriteFile(backups_folder+"/old-files-1.tar.gz", []byte("x"), 0o644)
	fakeBin(t, dir, "target-manifest", `printf 'interface=1\nactions=restore\n'`)
	fakeBin(t, dir, "target-install", `echo "$1" >> `+order)
	fakeBin(t, dir, "mount-unmount-usb", `exit 0`)
	options = &Options{RestoreBackup: "old-files-1.tar.gz"}

	w := httptest.NewRecorder()
	runInstallFinishedCommands(w, httptest.NewRequest("GET", "/api/run_install_finished_commands", nil))

	if b := w.Body.String(); strings.Contains(b, "ERROR") {
		t.Fatalf("answered %s", b)
	}
	if got, _ := os.ReadFile(order); string(got) != "restore\nconfigure\n" {
		t.Errorf("order %q", got)
	}
	if options.RestoreBackup != "" {
		t.Error("the restore choice was kept for the next install")
	}
}
