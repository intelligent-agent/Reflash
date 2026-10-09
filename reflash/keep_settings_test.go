package main

import (
	"os"
	"strings"
	"testing"
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

func carriedAll() map[string]string {
	return map[string]string{"SSH_ENABLED": "false", "SCREEN_ROTATION": "270", "WIFI_SSID": "kraake", "WIFI_PSK": "secret"}
}

func TestApplyCarriedFillsWhatWasNotChosen(t *testing.T) {
	got := applyCarried(Options{KeepSettings: true, EnableSsh: true}, carriedAll(), nil)
	if got.EnableSsh || got.ScreenRotation != 270 || got.WifiSSID != "kraake" || got.WifiPSK != "secret" {
		t.Errorf("got %+v", got)
	}
}

func TestApplyCarriedLeavesWhatTheUserChose(t *testing.T) {
	in := Options{KeepSettings: true, EnableSsh: true, ScreenRotation: 90, WifiSSID: "mine", WifiPSK: "mine-psk"}
	touched := map[string]bool{"SSH_ENABLED": true, "SCREEN_ROTATION": true, "WIFI_SSID": true}
	got := applyCarried(in, carriedAll(), touched)
	if got != in {
		t.Errorf("a choice made this session was overwritten: %+v", got)
	}
	// One choice does not protect the others.
	got = applyCarried(Options{KeepSettings: true}, carriedAll(), map[string]bool{"SCREEN_ROTATION": true})
	if got.ScreenRotation != 0 || got.WifiSSID != "kraake" || got.EnableSsh {
		t.Errorf("got %+v", got)
	}
}

func TestApplyCarriedOffOrEmptyChangesNothing(t *testing.T) {
	in := Options{EnableSsh: true, ScreenRotation: 90}
	if got := applyCarried(in, carriedAll(), nil); got != in {
		t.Errorf("switched off, yet changed: %+v", got)
	}
	in.KeepSettings = true
	if got := applyCarried(in, nil, nil); got != in {
		t.Errorf("nothing read, yet changed: %+v", got)
	}
}

func TestApplyCarriedNeverHalfANetwork(t *testing.T) {
	// A name whose passphrase the old installer did not return: neither.
	c := map[string]string{"WIFI_SSID": "kraake", "SCREEN_ROTATION": "90"}
	got := applyCarried(Options{KeepSettings: true}, c, nil)
	if got.WifiSSID != "" || got.WifiPSK != "" {
		t.Errorf("carried a name without a passphrase: %+v", got)
	}
	if got.ScreenRotation != 90 {
		t.Errorf("rotation should still carry: %+v", got)
	}
	// An old system with no network at all.
	c = map[string]string{"WIFI_SSID": "", "WIFI_PSK": ""}
	if got := applyCarried(Options{KeepSettings: true, WifiSSID: "x"}, c, nil); got.WifiSSID != "x" {
		t.Errorf("an empty name replaced the option: %+v", got)
	}
}

func TestApplyCarriedRefusesAnOddRotation(t *testing.T) {
	for _, bad := range []string{"45", "-90", "360", "abc", ""} {
		got := applyCarried(Options{KeepSettings: true, ScreenRotation: 180}, map[string]string{"SCREEN_ROTATION": bad}, nil)
		if got.ScreenRotation != 180 {
			t.Errorf("rotation %q was taken: %d", bad, got.ScreenRotation)
		}
	}
}

func TestNoteTouchedSeesOnlyChanges(t *testing.T) {
	saved := touchedOptions
	touchedOptions = map[string]bool{}
	defer func() { touchedOptions = saved }()

	before := Options{EnableSsh: true, ScreenRotation: 90, WifiSSID: "a"}
	noteTouched(before, before)
	if len(touchedOptions) != 0 {
		t.Fatalf("an unchanged save counted: %v", touchedOptions)
	}
	after := before
	after.ScreenRotation = 180
	noteTouched(before, after)
	if !touchedOptions["SCREEN_ROTATION"] || touchedOptions["SSH_ENABLED"] || touchedOptions["WIFI_SSID"] {
		t.Errorf("touched = %v", touchedOptions)
	}
	after.WifiPSK = "new"
	noteTouched(before, after)
	if !touchedOptions["WIFI_SSID"] {
		t.Errorf("a new passphrase did not count as choosing a network: %v", touchedOptions)
	}
}

func TestTargetSettingsUsesCarriedAndHidesNothingElse(t *testing.T) {
	saved, savedTouched := options, touchedOptions
	options = &Options{KeepSettings: true, EnableSsh: true, ScreenRotation: 0}
	touchedOptions = map[string]bool{"SSH_ENABLED": true}
	carriedLock.Lock()
	carried = carriedAll()
	carriedLock.Unlock()
	defer func() { options, touchedOptions = saved, savedTouched; forgetCarried() }()

	out, err := targetSettings(manifestSettings(""))
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"SSH_ENABLED=true\n", "SCREEN_ROTATION=270\n", "WIFI_SSID=kraake\n", "WIFI_PSK=secret\n"} {
		if !strings.Contains(out, want) {
			t.Errorf("missing %q in %q", want, out)
		}
	}
	forgetCarried()
	out, _ = targetSettings(manifestSettings(""))
	if !strings.Contains(out, "SCREEN_ROTATION=0\n") || strings.Contains(out, "kraake") {
		t.Errorf("carried settings outlived forgetCarried: %q", out)
	}
}

// Three requests start a write over the installed system: a local image, a
// download streamed to the eMMC, and an upload streamed to it. Reading the old
// system's settings has to happen in each, before the write; the first board
// test found the streamed paths had been missed.
func TestEveryWriteOverTheSystemCapturesItsSettings(t *testing.T) {
	src, err := os.ReadFile("server.go")
	if err != nil {
		t.Fatal(err)
	}
	for _, fn := range []string{"installRefactor", "startMagic", "uploadMagicStart"} {
		start := strings.Index(string(src), "\nfunc "+fn+"(")
		if start < 0 {
			t.Fatalf("%s not found: has it been renamed? update this test", fn)
		}
		body := string(src)[start+1:]
		if end := strings.Index(body, "\nfunc "); end > 0 {
			body = body[:end]
		}
		if !strings.Contains(body, "captureCarried()") {
			t.Errorf("%s writes the eMMC without reading the old system's settings (#195)", fn)
		}
	}
}
