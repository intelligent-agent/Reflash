package main

import (
	"strconv"
	"strings"
	"sync"
)

// #195, #185: "Keep my settings". A reinstall otherwise comes up on the
// image's factory defaults - no Wi-Fi, rotation 0, SSH off - unless the user
// remembered to set all of it in Reflash first. The system about to be replaced
// is read before the write, while it is still there, and what it held fills in
// whatever the user did not choose in this Reflash session.

// Settings Reflash carries. The Wi-Fi network travels as a pair: a name
// without its passphrase leaves a profile that cannot join anything.
const (
	keySSH      = "SSH_ENABLED"
	keyRotation = "SCREEN_ROTATION"
	keyWifiName = "WIFI_SSID"
	keyWifiPSK  = "WIFI_PSK"
)

var (
	// What the user changed in this session, keyed by the settings key (the
	// Wi-Fi name and passphrase together as WIFI_SSID). Options saved on the
	// drive from an earlier session are not choices made now, so they do not
	// count. Guarded by optionsLock.
	touchedOptions = map[string]bool{}

	// What the system being replaced held, read just before the write. In
	// memory only: it can include the Wi-Fi passphrase, which is never sent to
	// the browser, saved on the drive or logged, and is dropped once an
	// installed system has taken it.
	carriedLock sync.Mutex
	carried     map[string]string
)

// noteTouched records which options a set_options call changed. optionsLock held.
func noteTouched(before, after Options) {
	if before.EnableSsh != after.EnableSsh {
		touchedOptions[keySSH] = true
	}
	if before.ScreenRotation != after.ScreenRotation {
		touchedOptions[keyRotation] = true
	}
	if before.WifiSSID != after.WifiSSID || before.WifiPSK != after.WifiPSK {
		touchedOptions[keyWifiName] = true
	}
}

// parseSettings reads the interface's KEY=VALUE lines, the value literal to the
// end of the line, keeping only the keys the image lists.
func parseSettings(out string, allowed map[string]bool) map[string]string {
	got := map[string]string{}
	for _, line := range strings.Split(out, "\n") {
		if k, v, ok := strings.Cut(line, "="); ok && k != "SETTINGS" && allowed[k] {
			got[k] = v
		}
	}
	return got
}

// applyCarried returns opts with the carried settings filled in wherever the
// user chose nothing this session. It changes nothing when the user switched
// "Keep my settings" off, or nothing was read.
func applyCarried(opts Options, from map[string]string, touched map[string]bool) Options {
	if !opts.KeepSettings || len(from) == 0 {
		return opts
	}
	if v, ok := from[keySSH]; ok && !touched[keySSH] {
		opts.EnableSsh = v == "true"
	}
	if v, ok := from[keyRotation]; ok && !touched[keyRotation] {
		if n, err := strconv.Atoi(v); err == nil && n >= 0 && n < 360 && n%90 == 0 {
			opts.ScreenRotation = n
		}
	}
	// The pair, or neither: the passphrase has to have been returned (an
	// installer that does not know to, or an open network, returns none).
	name, hasName := from[keyWifiName]
	psk, hasPSK := from[keyWifiPSK]
	if hasName && name != "" && hasPSK && !touched[keyWifiName] {
		opts.WifiSSID = name
		opts.WifiPSK = psk
	}
	return opts
}

// captureCarried reads the system on the eMMC before an image is written over
// it. Anything that stops it - no installed system, one without the settings
// action, a failing read - means nothing is carried; only a failing read is
// worth telling the user, so that is the only error returned.
func captureCarried() error {
	carriedLock.Lock()
	carried = nil
	carriedLock.Unlock()
	optionsLock.Lock()
	keep := options != nil && options.KeepSettings
	optionsLock.Unlock()
	if !keep {
		return nil
	}
	installedSettingsLock.Lock()
	defer installedSettingsLock.Unlock()
	manifest, _, err := runCommand2("target-manifest")
	if err != nil || strings.TrimSpace(manifest) == "" {
		return nil
	}
	if installedSupports("settings", "") != nil {
		return nil
	}
	// "secrets": this once, for this reinstall, the installer may also print
	// the Wi-Fi passphrase. An installer that does not know the word ignores
	// it and prints none.
	out, _, err := runCommand2("target-install", "settings", "secrets")
	if err != nil {
		return err
	}
	got := parseSettings(out, manifestSettings(manifest))
	carriedLock.Lock()
	carried = got
	carriedLock.Unlock()
	return nil
}

// forgetCarried drops what captureCarried held.
func forgetCarried() {
	carriedLock.Lock()
	carried = nil
	carriedLock.Unlock()
}

// effectiveOptions is what the next install is configured with: the user's
// options, with the carried settings filling in what they did not choose.
func effectiveOptions() Options {
	optionsLock.Lock()
	opts := *options
	touched := map[string]bool{}
	for k, v := range touchedOptions {
		touched[k] = v
	}
	optionsLock.Unlock()
	carriedLock.Lock()
	from := carried
	carriedLock.Unlock()
	return applyCarried(opts, from, touched)
}
