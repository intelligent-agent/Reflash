package main

import (
	"sort"
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

	// Reflash#198
	keyRootPassword = "ROOT_PASSWORD"
	keyCountry      = "WIFI_COUNTRY"
	keyTimezone     = "TIMEZONE"
	keyWifiMode     = "WIFI_MODE"
	keyHotspotSSID  = "HOTSPOT_SSID"
	keyHotspotPSK   = "HOTSPOT_PSK"
	// Optional software is SOFTWARE_<name>=on|off, one key per component the
	// installed system offers, so the manifest lists the family as SOFTWARE.
	keySoftware     = "SOFTWARE"
	softwarePrefix  = "SOFTWARE_"
	softwareList    = "SOFTWARE_LIST"
	factoryPassword = "temppwd"
)

// A setting the image's installer takes. SOFTWARE_<name> keys are one family.
func keyAllowed(allowed map[string]bool, k string) bool {
	if allowed[k] {
		return true
	}
	return strings.HasPrefix(k, softwarePrefix) && k != softwareList && !strings.HasSuffix(k, "_INFO") && allowed[keySoftware]
}

// SoftwareItem is one optional component the installed system offers.
type SoftwareItem struct {
	Name string `json:"name"`
	Info string `json:"info"`
	// Installed on the system on the eMMC now, as of the last time it was read.
	Installed bool `json:"installed"`
}

var (
	softwareLock    sync.Mutex
	softwareCatalog []SoftwareItem
)

// softwareOn is the components named in options.Software, which is a comma
// separated list so Options stays something that can be compared.
func softwareOn(list string) map[string]bool {
	on := map[string]bool{}
	for _, n := range strings.Split(list, ",") {
		if n = strings.TrimSpace(n); n != "" {
			on[n] = true
		}
	}
	return on
}

func softwareString(on map[string]bool) string {
	names := make([]string, 0, len(on))
	for n, v := range on {
		if v {
			names = append(names, n)
		}
	}
	sort.Strings(names)
	return strings.Join(names, ",")
}

// parseSettings reads the interface's KEY=VALUE lines, the value literal to the
// end of the line, keeping only the keys the image lists.
func parseSettings(out string, allowed map[string]bool) map[string]string {
	got := map[string]string{}
	for _, line := range strings.Split(out, "\n") {
		if k, v, ok := strings.Cut(line, "="); ok && k != "SETTINGS" && (keyAllowed(allowed, k) || (allowed[keySoftware] && (k == softwareList || strings.HasPrefix(k, softwarePrefix)))) {
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
	// An installer that does not know a key does not print it, and the value
	// Reflash holds stays.
	if v, ok := from[keyCountry]; ok {
		opts.WifiCountry = v
	}
	if v, ok := from[keyTimezone]; ok {
		// What the image ships is the default, and shows as one.
		if v == "Etc/UTC" || v == "UTC" {
			v = ""
		}
		opts.Timezone = v
	}
	if v, ok := from[keyWifiMode]; ok {
		if v == "client" || v == "ap" {
			opts.WifiMode = v
		} else {
			opts.WifiMode = ""
		}
	}
	if name, ok := from[keyHotspotSSID]; ok {
		opts.HotspotSSID = name
		// The password is printed only when it is not the default.
		opts.HotspotPSK = from[keyHotspotPSK]
	}
	if list, ok := from[softwareList]; ok {
		on := map[string]bool{}
		for _, n := range strings.Fields(list) {
			on[n] = from[softwarePrefix+n] == "on"
		}
		opts.Software = softwareString(on)
	}
	return opts
}

// softwareFrom is the catalog the installed system printed.
func softwareFrom(from map[string]string) []SoftwareItem {
	var items []SoftwareItem
	for _, n := range strings.Fields(from[softwareList]) {
		items = append(items, SoftwareItem{Name: n, Info: from[softwarePrefix+n+"_INFO"], Installed: from[softwarePrefix+n] == "on"})
	}
	return items
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
	// A password cannot be read back, so "Default" is the factory password put
	// back explicitly, not a key left out.
	if before.LoginPassword != after.LoginPassword {
		if after.LoginPassword != "" {
			changes[keyPassword] = after.LoginPassword
		} else if before.LoginPassword != "" {
			changes[keyPassword] = factoryPassword
		}
	}
	if before.RootPassword != after.RootPassword {
		if after.RootPassword != "" {
			changes[keyRootPassword] = after.RootPassword
		} else if before.RootPassword != "" {
			changes[keyRootPassword] = factoryPassword
		}
	}
	// The rest: an empty value is the image's own, and is sent, so changing
	// back to Default puts the installed system back too.
	if before.WifiCountry != after.WifiCountry {
		changes[keyCountry] = after.WifiCountry
	}
	if before.Timezone != after.Timezone {
		changes[keyTimezone] = after.Timezone
	}
	if before.WifiMode != after.WifiMode {
		mode := after.WifiMode
		if mode == "" {
			mode = "auto"
		}
		changes[keyWifiMode] = mode
	}
	if before.HotspotSSID != after.HotspotSSID {
		changes[keyHotspotSSID] = after.HotspotSSID
	}
	if before.HotspotPSK != after.HotspotPSK {
		changes[keyHotspotPSK] = after.HotspotPSK
	}
	if before.Software != after.Software {
		was, now := softwareOn(before.Software), softwareOn(after.Software)
		for n := range now {
			if !was[n] {
				changes[softwarePrefix+n] = "on"
			}
		}
		for n := range was {
			if !now[n] {
				changes[softwarePrefix+n] = "off"
			}
		}
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

	// A write to the installed system is under way. Guarded by pushLock.
	pushing bool

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
	// A new change is a new attempt: the last one's failure says nothing about it.
	syncError = ""
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

	pushLock.Lock()
	pushing = true
	pushLock.Unlock()
	err := pushToInstalled(changes)
	if err != nil {
		// A password refused - too short, say - is not kept for the next
		// install to be refused again. The reason is shown, and a new one can
		// be set.
		optionsLock.Lock()
		if options != nil {
			if _, sent := changes[keyPassword]; sent {
				options.LoginPassword = ""
			}
			if _, sent := changes[keyRootPassword]; sent {
				options.RootPassword = ""
			}
			if _, sent := changes[keyHotspotPSK]; sent {
				options.HotspotPSK = ""
			}
			// Software that could not be installed - no connection to GitHub,
			// say - is not left switched on in Reflash for the next image to
			// fail on too (#205). The reason is shown with the error.
			if on := softwareOn(options.Software); len(on) > 0 {
				for k, v := range changes {
					if n, ok := strings.CutPrefix(k, softwarePrefix); ok && v == "on" {
						delete(on, n)
					}
				}
				options.Software = softwareString(on)
			}
		}
		optionsLock.Unlock()
	}
	pushLock.Lock()
	pushing = false
	if err != nil {
		syncError = err.Error()
		logError("The installed system's settings could not be changed: " + err.Error())
	} else {
		syncError = ""
	}
	pushLock.Unlock()
}

// syncState is for the page: whether a change is still on its way to the
// installed system, and why the last one failed, if it did.
func syncState() (busy bool, failure string) {
	pushLock.Lock()
	defer pushLock.Unlock()
	return pushing || len(pushPending) > 0, syncError
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
		if keyAllowed(allowed, k) {
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

	softwareLock.Lock()
	softwareCatalog = softwareFrom(got)
	softwareLock.Unlock()

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
