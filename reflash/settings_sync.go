package main

import (
	"strconv"
	"strings"
	"sync"
	"time"
)

// #185: Wi-Fi, screen rotation and SSH are one set of settings, held by Reflash
// and by the system on the eMMC, and kept the same:
//
//   - when Reflash starts, the installed system is read and its values replace
//     Reflash's own;
//   - whatever is changed in Reflash is applied to the installed system as
//     soon as it is changed;
//   - when an install finishes, what Reflash then holds goes onto the new
//     image, as it always did.
//
// So there is nothing to carry over: Reflash already holds what the old system
// did, and applies whatever the user has changed since.

// The settings keys Reflash mirrors, in the interface's own words.
const (
	keySSH      = "SSH_ENABLED"
	keyRotation = "SCREEN_ROTATION"
	keyWifiName = "WIFI_SSID"
	keyWifiPSK  = "WIFI_PSK"
	keyPassword = "LOGIN_PASSWORD"
)

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

// mergeInstalled returns opts with the installed system's values over them.
//
// Wi-Fi is a pair - a name without its passphrase is no network - so it is
// taken only whole. An installed system that returned no passphrase (an older
// installer, or no network at all) leaves Reflash's saved network alone, rather
// than wiping credentials the user entered here.
func mergeInstalled(opts Options, from map[string]string) Options {
	if v, ok := from[keySSH]; ok {
		opts.EnableSsh = v == "true"
	}
	if v, ok := from[keyRotation]; ok {
		if n, err := strconv.Atoi(v); err == nil && n >= 0 && n < 360 && n%90 == 0 {
			opts.ScreenRotation = n
		}
	}
	if name, ok := from[keyWifiName]; ok && name != "" {
		if psk, hasPSK := from[keyWifiPSK]; hasPSK {
			opts.WifiSSID = name
			opts.WifiPSK = psk
		}
	}
	return opts
}

// changedSettings is what changing before into after means for the installed
// system: only the settings that differ, in the interface's words. An empty
// Wi-Fi name is not sent: clearing a field in Reflash is not a request to take
// the installed system off its network.
func changedSettings(before, after Options) map[string]string {
	changes := map[string]string{}
	if before.EnableSsh != after.EnableSsh {
		changes[keySSH] = strconv.FormatBool(after.EnableSsh)
	}
	if before.ScreenRotation != after.ScreenRotation {
		changes[keyRotation] = strconv.Itoa(after.ScreenRotation)
	}
	if (before.WifiSSID != after.WifiSSID || before.WifiPSK != after.WifiPSK) && after.WifiSSID != "" {
		changes[keyWifiName] = after.WifiSSID
		changes[keyWifiPSK] = after.WifiPSK
	}
	if before.LoginPassword != after.LoginPassword && after.LoginPassword != "" {
		changes[keyPassword] = after.LoginPassword
	}
	return changes
}

var (
	pushLock    sync.Mutex
	pushPending = map[string]string{}
	pushTimer   *time.Timer
	pushTries   int

	// Why the installed system could not be brought in line, for the options
	// panel. Empty when it could. Guarded by pushLock.
	syncError string

	// A short wait gathers the several changes one dialog makes into one
	// write to the eMMC; a busy eMMC is tried again a few times.
	pushDelay      = 1500 * time.Millisecond
	pushRetryDelay = 3 * time.Second
	pushMaxTries   = 20
)

// queuePush has the installed system brought in line with changes, soon.
// Several calls close together become one write.
func queuePush(changes map[string]string) {
	if len(changes) == 0 {
		return
	}
	pushLock.Lock()
	defer pushLock.Unlock()
	for k, v := range changes {
		pushPending[k] = v
	}
	pushTries = 0
	if pushTimer != nil {
		pushTimer.Stop()
	}
	pushTimer = time.AfterFunc(pushDelay, flushPush)
}

// flushPush writes the queued changes to the installed system. An image being
// written, uploaded or backed up means they wait; one being installed gets
// Reflash's settings when it finishes anyway, so a change that never finds the
// eMMC free is dropped after a while.
func flushPush() {
	pushLock.Lock()
	changes := pushPending
	pushPending = map[string]string{}
	pushLock.Unlock()
	if len(changes) == 0 {
		return
	}

	if busy := eMMCBusy(); busy != "" && busy != string(SAVING) {
		pushLock.Lock()
		for k, v := range changes {
			if _, newer := pushPending[k]; !newer {
				pushPending[k] = v
			}
		}
		pushTries++
		if pushTries < pushMaxTries {
			pushTimer = time.AfterFunc(pushRetryDelay, flushPush)
		} else {
			pushPending = map[string]string{}
			logInfo("Not bringing the installed system in line: the eMMC stayed busy (" + busy + ")")
		}
		pushLock.Unlock()
		return
	}

	err := pushToInstalled(changes)
	pushLock.Lock()
	if err != nil {
		syncError = err.Error()
		logError("The installed system's settings could not be changed: " + err.Error())
	} else {
		syncError = ""
	}
	pushLock.Unlock()
}

// pushToInstalled is a variable so the tests need no board.
var pushToInstalled = pushToInstalledSystem

// pushToInstalledSystem applies changes through the installed system's own
// installer. No installed system, or one that does not take a setting, is not
// an error: there is nothing to bring in line, and the next install gets them
// anyway.
func pushToInstalledSystem(changes map[string]string) error {
	installedSettingsLock.Lock()
	defer installedSettingsLock.Unlock()
	manifest, _, err := runCommand2("target-manifest")
	if err != nil || strings.TrimSpace(manifest) == "" {
		return nil
	}
	allowed := manifestSettings(manifest)
	use := map[string]string{}
	for k, v := range changes {
		if allowed[k] {
			use[k] = v
		}
	}
	if len(use) == 0 {
		return nil
	}
	return applyInstalledSettings(allowed, use)
}

// syncFromInstalled makes the installed system's settings Reflash's own, once,
// as Reflash starts. Asked to include the Wi-Fi passphrase: the pair is then
// whole, and goes onto a new image later. It stays in Reflash's options, which
// is where a passphrase typed here lives too. An installer that does not know
// to return it gives none, and Reflash's own network is left alone.
func syncFromInstalled() error {
	installedSettingsLock.Lock()
	defer installedSettingsLock.Unlock()
	manifest, _, err := runCommand2("target-manifest")
	if err != nil || strings.TrimSpace(manifest) == "" {
		return nil
	}
	if installedSupports("settings", "") != nil {
		return nil
	}
	out, _, err := runCommand2("target-install", "settings", "secrets")
	if err != nil {
		return err
	}
	got := parseSettings(out, manifestSettings(manifest))

	optionsLock.Lock()
	defer optionsLock.Unlock()
	merged := mergeInstalled(*options, got)
	if merged != *options {
		*options = merged
		isDirty = true
		logInfo("Settings taken from the installed system")
	}
	return nil
}
