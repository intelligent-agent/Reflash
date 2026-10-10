package main

import (
	"errors"
	"net/http/httptest"
	"os"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/pelletier/go-toml/v2"
)

func TestParseSettingsKeepsOnlyListedKeys(t *testing.T) {
	out := "SETTINGS=1\nSSH_ENABLED=true\nSCREEN_ROTATION=90\nWIFI_SSID=home net\nWIFI_PSK=p=ss\nOTHER=1\n"
	got := parseSettings(out, manifestSettings(""))
	if got["SSH_ENABLED"] != "true" || got["SCREEN_ROTATION"] != "90" || got["WIFI_SSID"] != "home net" {
		t.Fatalf("got %v", got)
	}
	// The value is literal to the end of the line, "=" included.
	if got["WIFI_PSK"] != "p=ss" {
		t.Errorf("passphrase = %q", got["WIFI_PSK"])
	}
	if _, ok := got["OTHER"]; ok {
		t.Error("a key the image does not list was kept")
	}
	if _, ok := got["SETTINGS"]; ok {
		t.Error("the SETTINGS marker was kept")
	}
}

func TestMergeInstalledReplacesReflashsValues(t *testing.T) {
	in := Options{EnableSsh: true, ScreenRotation: 0, WifiSSID: "old", WifiPSK: "old-psk"}
	got := mergeInstalled(in, map[string]string{
		"SSH_ENABLED": "false", "SCREEN_ROTATION": "270", "WIFI_SSID": "kraake", "WIFI_PSK": "secret"})
	if got.EnableSsh || got.ScreenRotation != 270 || got.WifiSSID != "kraake" || got.WifiPSK != "secret" {
		t.Errorf("got %+v", got)
	}
}

func TestMergeInstalledTakesWifiOnlyWhole(t *testing.T) {
	in := Options{WifiSSID: "mine", WifiPSK: "mine-psk"}
	// A name whose passphrase the installer did not return.
	if got := mergeInstalled(in, map[string]string{"WIFI_SSID": "kraake"}); got != in {
		t.Errorf("half a network was taken: %+v", got)
	}
	// An installed system with no network does not wipe the one saved here.
	if got := mergeInstalled(in, map[string]string{"WIFI_SSID": "", "WIFI_PSK": ""}); got != in {
		t.Errorf("saved credentials were wiped: %+v", got)
	}
	// An open network is a name with an empty passphrase, and is whole.
	got := mergeInstalled(in, map[string]string{"WIFI_SSID": "cafe", "WIFI_PSK": ""})
	if got.WifiSSID != "cafe" || got.WifiPSK != "" {
		t.Errorf("open network: %+v", got)
	}
}

func TestMergeInstalledRefusesAnOddRotation(t *testing.T) {
	for _, bad := range []string{"45", "-90", "360", "abc", ""} {
		got := mergeInstalled(Options{ScreenRotation: 180}, map[string]string{"SCREEN_ROTATION": bad})
		if got.ScreenRotation != 180 {
			t.Errorf("rotation %q was taken: %d", bad, got.ScreenRotation)
		}
	}
}

func TestChangedSettingsIsOnlyWhatDiffers(t *testing.T) {
	a := Options{EnableSsh: true, ScreenRotation: 90, WifiSSID: "a", WifiPSK: "p"}
	if got := changedSettings(a, a); len(got) != 0 {
		t.Fatalf("an unchanged save changed %v", got)
	}
	b := a
	b.ScreenRotation = 180
	if got := changedSettings(a, b); !reflect.DeepEqual(got, map[string]string{"SCREEN_ROTATION": "180"}) {
		t.Errorf("rotation: %v", got)
	}
	b = a
	b.EnableSsh = false
	if got := changedSettings(a, b); !reflect.DeepEqual(got, map[string]string{"SSH_ENABLED": "false"}) {
		t.Errorf("ssh: %v", got)
	}
}

func TestChangedSettingsSendsTheWifiPairAndNeverAnEmptyName(t *testing.T) {
	a := Options{WifiSSID: "a", WifiPSK: "p"}
	// Only the passphrase changed: the pair goes together.
	b := a
	b.WifiPSK = "new"
	if got := changedSettings(a, b); !reflect.DeepEqual(got, map[string]string{"WIFI_SSID": "a", "WIFI_PSK": "new"}) {
		t.Errorf("passphrase only: %v", got)
	}
	// Clearing the fields is not a request to take the system off its network.
	if got := changedSettings(a, Options{}); len(got) != 0 {
		t.Errorf("clearing sent %v", got)
	}
}

func TestChangedSettingsSendsANewPasswordAndTheFactoryOneForDefault(t *testing.T) {
	if got := changedSettings(Options{}, Options{LoginPassword: "pw"}); !reflect.DeepEqual(got, map[string]string{"LOGIN_PASSWORD": "pw"}) {
		t.Errorf("new password: %v", got)
	}
	// Default after a password was set puts the factory one back (#198): a
	// password cannot be read back, so it is sent, not left out.
	if got := changedSettings(Options{LoginPassword: "pw"}, Options{}); !reflect.DeepEqual(got, map[string]string{"LOGIN_PASSWORD": "temppwd"}) {
		t.Errorf("default after a password: %v", got)
	}
	// Never set and still not: nothing to say.
	if got := changedSettings(Options{}, Options{}); len(got) != 0 {
		t.Errorf("nothing changed, sent %v", got)
	}
	if got := changedSettings(Options{RootPassword: "pw"}, Options{}); !reflect.DeepEqual(got, map[string]string{"ROOT_PASSWORD": "temppwd"}) {
		t.Errorf("root default: %v", got)
	}
}

// #198: what Armbian's first login asked, and more. An empty value is Default
// and is sent, so going back to Default puts the installed system back too.
func TestChangedSettingsLocationNetworkModeAndSoftware(t *testing.T) {
	got := changedSettings(Options{}, Options{WifiCountry: "NO", Timezone: "Europe/Oslo", WifiMode: "client", HotspotSSID: "Shop", HotspotPSK: "hemmelig1", RootPassword: "rootpw1"})
	want := map[string]string{"WIFI_COUNTRY": "NO", "TIMEZONE": "Europe/Oslo", "WIFI_MODE": "client", "HOTSPOT_SSID": "Shop", "HOTSPOT_PSK": "hemmelig1", "ROOT_PASSWORD": "rootpw1"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("set: %v", got)
	}
	back := changedSettings(Options{WifiCountry: "NO", Timezone: "Europe/Oslo", WifiMode: "ap", HotspotSSID: "Shop", HotspotPSK: "hemmelig1"}, Options{})
	want = map[string]string{"WIFI_COUNTRY": "", "TIMEZONE": "", "WIFI_MODE": "auto", "HOTSPOT_SSID": "", "HOTSPOT_PSK": ""}
	if !reflect.DeepEqual(back, want) {
		t.Errorf("back to default: %v", back)
	}
	sw := changedSettings(Options{Software: "a,b"}, Options{Software: "b,led_effect"})
	if !reflect.DeepEqual(sw, map[string]string{"SOFTWARE_led_effect": "on", "SOFTWARE_a": "off"}) {
		t.Errorf("software: %v", sw)
	}
}

func TestMergeInstalledTakesTheNewKeysAndShowsDefaultsAsDefault(t *testing.T) {
	from := map[string]string{
		"WIFI_COUNTRY": "SE", "TIMEZONE": "Etc/UTC", "WIFI_MODE": "auto", "HOTSPOT_SSID": "", "HOTSPOT_PSK": "",
		"SOFTWARE_LIST": "led_effect other", "SOFTWARE_led_effect": "on", "SOFTWARE_other": "off",
	}
	got := mergeInstalled(Options{WifiCountry: "NO", Timezone: "Europe/Oslo", WifiMode: "ap", HotspotSSID: "old", HotspotPSK: "oldpsk123"}, from)
	want := Options{WifiCountry: "SE", Software: "led_effect"}
	if got != want {
		t.Errorf("merged %+v, want %+v", got, want)
	}
	got = mergeInstalled(Options{}, map[string]string{"TIMEZONE": "Europe/Oslo", "WIFI_MODE": "client", "HOTSPOT_SSID": "Shop", "HOTSPOT_PSK": "hemmelig1"})
	if got.Timezone != "Europe/Oslo" || got.WifiMode != "client" || got.HotspotSSID != "Shop" || got.HotspotPSK != "hemmelig1" {
		t.Errorf("custom values: %+v", got)
	}
	// An installer that prints none of them leaves Reflash's own.
	keep := Options{WifiCountry: "NO", Timezone: "Europe/Oslo", HotspotPSK: "hemmelig1", Software: "led_effect"}
	if got := mergeInstalled(keep, map[string]string{"SSH_ENABLED": "true"}); got.WifiCountry != "NO" || got.Timezone != "Europe/Oslo" || got.HotspotPSK != "hemmelig1" || got.Software != "led_effect" {
		t.Errorf("an old installer wiped them: %+v", got)
	}
}

func TestSoftwareKeysAreOneFamilyTheManifestListsAsSOFTWARE(t *testing.T) {
	allowed := manifestSettings("interface=1\nsettings=SSH_ENABLED,SOFTWARE\n")
	got := parseSettings("SETTINGS=1\nSSH_ENABLED=true\nSOFTWARE_LIST=led_effect\nSOFTWARE_led_effect=on\nSOFTWARE_led_effect_INFO=LED effects\nROOT_PASSWORD=x\n", allowed)
	if got["SOFTWARE_led_effect"] != "on" || got["SOFTWARE_LIST"] != "led_effect" || got["ROOT_PASSWORD"] != "" {
		t.Errorf("parsed %v", got)
	}
	items := softwareFrom(got)
	if len(items) != 1 || items[0].Name != "led_effect" || items[0].Info != "LED effects" || !items[0].Installed {
		t.Errorf("catalog %+v", items)
	}
	// Without SOFTWARE in the manifest the family is not accepted.
	if got := parseSettings("SOFTWARE_led_effect=on\n", manifestSettings("interface=1\nsettings=SSH_ENABLED\n")); len(got) != 0 {
		t.Errorf("accepted without the manifest: %v", got)
	}
	if keyAllowed(allowed, "SOFTWARE_LIST") || !keyAllowed(allowed, "SOFTWARE_led_effect") || keyAllowed(allowed, "SOFTWARE_led_effect_INFO") {
		t.Error("only SOFTWARE_<name> is a setting to send")
	}
}

// What goes onto a new image: only what is not Default, and the secrets only
// where the image takes them.
func TestTargetSettingsSendsOnlyWhatIsNotDefault(t *testing.T) {
	options = &Options{}
	all := manifestSettings("interface=1\nsettings=SSH_ENABLED,SCREEN_ROTATION,ROOT_PASSWORD,WIFI_COUNTRY,TIMEZONE,WIFI_MODE,HOTSPOT_SSID,HOTSPOT_PSK,SOFTWARE\n")
	got, err := targetSettings(all)
	if err != nil || got != "SETTINGS=1\nSSH_ENABLED=false\nSCREEN_ROTATION=0\n" {
		t.Errorf("all default: %q %v", got, err)
	}
	options = &Options{RootPassword: "rootpw1", WifiCountry: "NO", Timezone: "Europe/Oslo", WifiMode: "ap", HotspotSSID: "Shop", HotspotPSK: "hemmelig1", Software: "led_effect"}
	got, _ = targetSettings(all)
	for _, line := range []string{"ROOT_PASSWORD=rootpw1", "WIFI_COUNTRY=NO", "TIMEZONE=Europe/Oslo", "WIFI_MODE=ap", "HOTSPOT_SSID=Shop", "HOTSPOT_PSK=hemmelig1", "SOFTWARE_led_effect=on"} {
		if !strings.Contains(got, line+"\n") {
			t.Errorf("missing %s in %q", line, got)
		}
	}
	// An image that does not list them is not sent them.
	got, _ = targetSettings(manifestSettings("interface=1\nsettings=SSH_ENABLED\n"))
	if strings.Contains(got, "ROOT") || strings.Contains(got, "SOFTWARE") || strings.Contains(got, "HOTSPOT") {
		t.Errorf("sent to an image that does not take them: %q", got)
	}
	options = &Options{RootPassword: "a\nSETTINGS=2"}
	if _, err := targetSettings(all); err == nil {
		t.Error("a line break in the root password was sent")
	}
}

func TestNewSecretsAreNeverStoredOrReturned(t *testing.T) {
	setupTest(t)
	options = &Options{}
	if err := lockSetOptions([]byte(`{"rootPassword":"rootsecret1","hotspotPSK":"hotsecret12","wifiCountry":"NO"}`)); err != nil {
		t.Fatal(err)
	}
	saved, _ := toml.Marshal(options)
	if strings.Contains(string(saved), "secret") || !strings.Contains(string(saved), "NO") {
		t.Errorf("options.cfg would hold the secrets, or lose the country:\n%s", saved)
	}
	w := httptest.NewRecorder()
	writeOptions(w)
	body := w.Body.String()
	if strings.Contains(body, "secret") || !strings.Contains(body, `"rootPasswordSet":true`) || !strings.Contains(body, `"hotspotPSKSet":true`) {
		t.Errorf("get_options answered %s", body)
	}
}

func TestIncludeArgsRefuseWhatIsNotAPath(t *testing.T) {
	got, err := includeArgs([]string{"home/printer/printer_data/config/printer.cfg", "home/printer/printer_data/config/my files/a.cfg"})
	if err != nil || strings.Join(got, "|") != "--include|home/printer/printer_data/config/printer.cfg|--include|home/printer/printer_data/config/my files/a.cfg" {
		t.Errorf("got %v %v", got, err)
	}
	for _, bad := range []string{"", "../etc/shadow", "a/../b", "a\nb"} {
		if _, err := includeArgs([]string{bad}); err == nil {
			t.Errorf("%q was accepted", bad)
		}
	}
}

// pushRig replaces what writes to the installed system and the timings, and
// gathers what was written.
type pushRig struct {
	mu    sync.Mutex
	calls []map[string]string
	err   error
}

// The push goroutine reads the state under its lock; so does the test.
func setTestState(v string) {
	state.Lock()
	state.State = v
	state.Unlock()
}

func newPushRig(t *testing.T) *pushRig {
	r := &pushRig{}
	savedPush, savedDelay, savedRetry, savedMax, savedState := pushToInstalled, pushDelay, pushRetryDelay, pushMaxTries, state
	pushToInstalled = func(c map[string]string) error {
		r.mu.Lock()
		defer r.mu.Unlock()
		cp := map[string]string{}
		for k, v := range c {
			cp[k] = v
		}
		r.calls = append(r.calls, cp)
		return r.err
	}
	pushDelay, pushRetryDelay, pushMaxTries = 20*time.Millisecond, 20*time.Millisecond, 5
	state = &State{State: IDLE}
	pushLock.Lock()
	pushPending, syncError = map[string]string{}, ""
	pushLock.Unlock()
	t.Cleanup(func() {
		// Nothing of this test may fire once the real functions are back.
		pushLock.Lock()
		if pushTimer != nil {
			pushTimer.Stop()
		}
		pushPending, pushing = map[string]string{}, false
		pushLock.Unlock()
		time.Sleep(2 * pushDelay)
		pushToInstalled, pushDelay, pushRetryDelay, pushMaxTries, state = savedPush, savedDelay, savedRetry, savedMax, savedState
	})
	return r
}

func (r *pushRig) waitFor(t *testing.T, n int) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		r.mu.Lock()
		got := len(r.calls)
		r.mu.Unlock()
		if got >= n {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	t.Fatalf("expected %d writes, got %d: %v", n, len(r.calls), r.calls)
}

func TestSeveralChangesBecomeOneWrite(t *testing.T) {
	r := newPushRig(t)
	queuePush(map[string]string{"SCREEN_ROTATION": "90"})
	queuePush(map[string]string{"SSH_ENABLED": "false", "SCREEN_ROTATION": "180"})
	r.waitFor(t, 1)
	time.Sleep(80 * time.Millisecond)
	r.mu.Lock()
	defer r.mu.Unlock()
	if len(r.calls) != 1 {
		t.Fatalf("writes: %v", r.calls)
	}
	// The later value of a setting wins.
	if !reflect.DeepEqual(r.calls[0], map[string]string{"SCREEN_ROTATION": "180", "SSH_ENABLED": "false"}) {
		t.Errorf("wrote %v", r.calls[0])
	}
}

func TestAChangeWaitsForABusyEMMCAndThenGoes(t *testing.T) {
	r := newPushRig(t)
	setTestState(UPLOADING_MAGIC)
	queuePush(map[string]string{"SSH_ENABLED": "false"})
	time.Sleep(80 * time.Millisecond)
	r.mu.Lock()
	early := len(r.calls)
	r.mu.Unlock()
	if early != 0 {
		t.Fatalf("written while the eMMC was being written: %v", r.calls)
	}
	setTestState(IDLE)
	r.waitFor(t, 1)
}

func TestAChangeIsDroppedWhenTheEMMCStaysBusy(t *testing.T) {
	r := newPushRig(t)
	setTestState(INSTALLING)
	queuePush(map[string]string{"SSH_ENABLED": "false"})
	time.Sleep(300 * time.Millisecond)
	setTestState(IDLE)
	time.Sleep(80 * time.Millisecond)
	r.mu.Lock()
	defer r.mu.Unlock()
	if len(r.calls) != 0 {
		t.Errorf("a change given up on was written later: %v", r.calls)
	}
}

func TestAFailureIsReportedAndClearedByTheNextSuccess(t *testing.T) {
	r := newPushRig(t)
	r.err = errors.New("the installer said no")
	queuePush(map[string]string{"SSH_ENABLED": "false"})
	r.waitFor(t, 1)
	time.Sleep(30 * time.Millisecond)
	pushLock.Lock()
	failed := syncError
	pushLock.Unlock()
	if failed != "the installer said no" {
		t.Fatalf("syncError = %q", failed)
	}
	r.mu.Lock()
	r.err = nil
	r.mu.Unlock()
	queuePush(map[string]string{"SSH_ENABLED": "true"})
	r.waitFor(t, 2)
	time.Sleep(30 * time.Millisecond)
	pushLock.Lock()
	defer pushLock.Unlock()
	if syncError != "" {
		t.Errorf("a later success left %q", syncError)
	}
}

// What the user changes in the options panel reaches the installed system:
// set_options is where it starts.
func TestSetOptionsQueuesWhatChanged(t *testing.T) {
	r := newPushRig(t)
	savedOpts := options
	options = &Options{EnableSsh: true, ScreenRotation: 0}
	defer func() { options = savedOpts }()
	if err := lockSetOptions([]byte(`{"screenRotation":270}`)); err != nil {
		t.Fatal(err)
	}
	r.waitFor(t, 1)
	r.mu.Lock()
	defer r.mu.Unlock()
	if !reflect.DeepEqual(r.calls[0], map[string]string{"SCREEN_ROTATION": "270"}) {
		t.Errorf("wrote %v", r.calls[0])
	}
}

func TestSetOptionsWithNothingChangedWritesNothing(t *testing.T) {
	r := newPushRig(t)
	savedOpts := options
	options = &Options{EnableSsh: true, ScreenRotation: 90}
	defer func() { options = savedOpts }()
	if err := lockSetOptions([]byte(`{"screenRotation":90,"enableSsh":true,"darkmode":true}`)); err != nil {
		t.Fatal(err)
	}
	time.Sleep(100 * time.Millisecond)
	r.mu.Lock()
	defer r.mu.Unlock()
	if len(r.calls) != 0 {
		t.Errorf("an unchanged save wrote %v", r.calls)
	}
}

// The Wi-Fi is stored by the connect handler, not by set_options, so it has
// to queue its own push. A test on the source: the hook was missed once for a
// whole install path, and a streamed install is not easy to drive here.
func TestEveryPlaceThatStoresSettingsBringsTheInstalledSystemAlong(t *testing.T) {
	src, err := os.ReadFile("server.go")
	if err != nil {
		t.Fatal(err)
	}
	text := string(src)
	for _, want := range []string{"queuePush(changedSettings(before, *options))", "queuePush(map[string]string{keyWifiName: ssid, keyWifiPSK: psk})", "syncFromInstalled()"} {
		if !strings.Contains(text, want) {
			t.Errorf("server.go no longer has %q: a setting would stop reaching the installed system (#185)", want)
		}
	}
}

// A change made while an image is being written is kept in Reflash and not
// written to the system being replaced; what Reflash holds when the install
// finishes goes onto the new image (runInstallFinishedCommands reads the live
// options, not a copy taken when the install began). A change made after that
// goes to the new image like any other.
func TestAChangeDuringAFlashIsKeptAndNotWrittenToTheOldSystem(t *testing.T) {
	r := newPushRig(t)
	savedOpts := options
	options = &Options{EnableSsh: true, ScreenRotation: 0}
	defer func() { options = savedOpts }()

	setTestState(INSTALLING)
	if err := lockSetOptions([]byte(`{"screenRotation":90}`)); err != nil {
		t.Fatal(err)
	}
	if options.ScreenRotation != 90 {
		t.Fatalf("not kept in Reflash: %+v", options)
	}
	time.Sleep(60 * time.Millisecond)
	r.mu.Lock()
	during := len(r.calls)
	r.mu.Unlock()
	if during != 0 {
		t.Fatalf("written to the old system while it was being replaced: %v", r.calls)
	}

	// The install finishes: the settings it applies are the live ones.
	if got, err := targetSettings(manifestSettings("")); err != nil || !strings.Contains(got, "SCREEN_ROTATION=90\n") {
		t.Errorf("the new image would not get the change: %q (%v)", got, err)
	}
	setTestState(FINISHED)
	r.waitFor(t, 1)
}

func TestANewChangeClearsTheLastFailure(t *testing.T) {
	newPushRig(t)
	pushLock.Lock()
	syncError = "the password is too short"
	pushLock.Unlock()
	queuePush(map[string]string{"LOGIN_PASSWORD": "temppwd"})
	if busy, failure := syncState(); failure != "" || !busy {
		t.Errorf("busy=%v failure=%q: a stale failure outlived a new attempt", busy, failure)
	}
}

func TestARefusedPasswordIsNotKeptForTheNextInstall(t *testing.T) {
	r := newPushRig(t)
	r.err = errors.New("the password is too short: at least 6 characters")
	savedOpts := options
	options = &Options{LoginPassword: "abc"}
	defer func() { options = savedOpts }()

	queuePush(map[string]string{"LOGIN_PASSWORD": "abc"})
	r.waitFor(t, 1)
	time.Sleep(40 * time.Millisecond)
	optionsLock.Lock()
	kept := options.LoginPassword
	optionsLock.Unlock()
	if kept != "" {
		t.Errorf("a refused password was kept: %q", kept)
	}
	if _, failure := syncState(); failure == "" {
		t.Error("the refusal was not reported")
	}
}

func TestANonPasswordFailureKeepsThePassword(t *testing.T) {
	r := newPushRig(t)
	r.err = errors.New("eMMC error")
	savedOpts := options
	options = &Options{LoginPassword: "longenough"}
	defer func() { options = savedOpts }()
	queuePush(map[string]string{"SSH_ENABLED": "false"})
	r.waitFor(t, 1)
	time.Sleep(40 * time.Millisecond)
	if options.LoginPassword != "longenough" {
		t.Errorf("a failure unrelated to the password dropped it")
	}
}

func TestSyncStateSaysWhenTheWriteIsDone(t *testing.T) {
	r := newPushRig(t)
	if busy, _ := syncState(); busy {
		t.Fatal("busy with nothing queued")
	}
	queuePush(map[string]string{"SSH_ENABLED": "false"})
	if busy, _ := syncState(); !busy {
		t.Error("not busy with a change waiting")
	}
	r.waitFor(t, 1)
	time.Sleep(40 * time.Millisecond)
	if busy, _ := syncState(); busy {
		t.Error("still busy after the write")
	}
}
