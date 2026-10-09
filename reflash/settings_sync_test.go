package main

import (
	"errors"
	"os"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"
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

func TestChangedSettingsSendsANewPasswordNotAClearedOne(t *testing.T) {
	if got := changedSettings(Options{}, Options{LoginPassword: "pw"}); !reflect.DeepEqual(got, map[string]string{"LOGIN_PASSWORD": "pw"}) {
		t.Errorf("new password: %v", got)
	}
	if got := changedSettings(Options{LoginPassword: "pw"}, Options{}); len(got) != 0 {
		t.Errorf("a password dropped after an install was sent: %v", got)
	}
}

// pushRig replaces what writes to the installed system and the timings, and
// gathers what was written.
type pushRig struct {
	mu    sync.Mutex
	calls []map[string]string
	err   error
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
	state.State = UPLOADING_MAGIC
	queuePush(map[string]string{"SSH_ENABLED": "false"})
	time.Sleep(80 * time.Millisecond)
	r.mu.Lock()
	early := len(r.calls)
	r.mu.Unlock()
	if early != 0 {
		t.Fatalf("written while the eMMC was being written: %v", r.calls)
	}
	state.State = IDLE
	r.waitFor(t, 1)
}

func TestAChangeIsDroppedWhenTheEMMCStaysBusy(t *testing.T) {
	r := newPushRig(t)
	state.State = INSTALLING
	queuePush(map[string]string{"SSH_ENABLED": "false"})
	time.Sleep(300 * time.Millisecond)
	state.State = IDLE
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

	state.State = INSTALLING
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
	state.State = FINISHED
	r.waitFor(t, 1)
}
