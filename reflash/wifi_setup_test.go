package main

import (
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func wifiSetupRequest(body string) *httptest.ResponseRecorder {
	w := httptest.NewRecorder()
	wifiSetup(w, httptest.NewRequest("POST", "/api/wifi_setup", strings.NewReader(body)))
	return w
}
func TestStageManualWifiDoesNotConnectOrApply(t *testing.T) {
	dir := setupTest(t)
	state.State = IDLE
	for _, name := range []string{"wifi-connect", "target-install", "target-manifest"} {
		fakeBin(t, dir, name, `touch "`+dir+`/unexpected"`)
	}
	options.LiveWifiSSID = "WorkingNetwork"
	options.LiveWifiPSK = "workingpassword"
	w := wifiSetupRequest(`{"source":"manual","action":"stage","settings":{"WIFI_MODE":"auto","WIFI_SSID":"UnreachableNetwork","WIFI_PSK":"notvalidated","HOTSPOT_SSID":"Printer","HOTSPOT_PSK":"12345678"}}`)
	if w.Code != 200 {
		t.Fatal(w.Body.String())
	}
	if options.WifiSSID != "UnreachableNetwork" || options.LiveWifiSSID != "WorkingNetwork" {
		t.Fatal("staging changed live Wi-Fi or lost the next-image choice")
	}
	if _, err := os.Stat(filepath.Join(dir, "unexpected")); !os.IsNotExist(err) {
		t.Fatal("staging ran a live connection or installer command")
	}
}
func TestCopyReflashWifiKeepsSecretsOnBoard(t *testing.T) {
	setupTest(t)
	state.State = IDLE
	options.LiveWifiSSID = "CurrentNetwork"
	options.LiveWifiPSK = "currentsecret"
	options.WifiSSID = "Other"
	w := wifiSetupRequest(`{"source":"reflash","action":"stage"}`)
	if w.Code != 200 || options.WifiSSID != "CurrentNetwork" || options.WifiPSK != "currentsecret" {
		t.Fatal("failed to copy live setup")
	}
	if strings.Contains(w.Body.String(), "currentsecret") {
		t.Fatal("secret in response")
	}
	w = httptest.NewRecorder()
	writeOptions(w)
	if strings.Contains(w.Body.String(), "currentsecret") {
		t.Fatal("live secret in public options")
	}
}
func TestCopyInstalledWifiIncludesSecretWithoutApplying(t *testing.T) {
	dir := setupTest(t)
	state.State = IDLE
	fakeBin(t, dir, "target-manifest", `echo 'interface=1'; echo 'settings=WIFI_SSID,WIFI_PSK,WIFI_MODE,HOTSPOT_SSID,HOTSPOT_PSK'`)
	fakeBin(t, dir, "target-install", `[ "$1 $2" = 'settings secrets' ] || exit 1
 echo 'WIFI_SSID=OldNetwork'; echo 'WIFI_PSK=oldsecret123'; echo 'WIFI_MODE=auto'; echo 'HOTSPOT_SSID=Fallback'; echo 'HOTSPOT_PSK=apsecret123'`)
	w := wifiSetupRequest(`{"source":"emmc","action":"stage"}`)
	if w.Code != 200 || options.WifiPSK != "oldsecret123" || options.HotspotPSK != "apsecret123" {
		t.Fatal("complete installed setup was not copied")
	}
	if strings.Contains(w.Body.String(), "secret") {
		t.Fatal("secret returned")
	}
}
func TestExplicitWifiApplyDoesNotChangeStagedSource(t *testing.T) {
	dir := setupTest(t)
	state.State = IDLE
	options.WifiSource = "manual"
	options.WifiSSID = "Next"
	options.LiveWifiSSID = "Live"
	options.LiveWifiPSK = "livepassword"
	fakeBin(t, dir, "target-manifest", `echo 'interface=1'; echo 'settings=WIFI_SSID,WIFI_PSK,WIFI_MODE,HOTSPOT_SSID,HOTSPOT_PSK'`)
	fakeBin(t, dir, "target-install", `[ "$1" = configure ] || exit 1; cp "$2" "`+dir+`/applied"`)
	w := wifiSetupRequest(`{"source":"reflash","action":"apply"}`)
	applied, err := os.ReadFile(filepath.Join(dir, "applied"))
	if w.Code != 200 || err != nil || !strings.Contains(string(applied), "WIFI_SSID=Live\n") || !strings.Contains(string(applied), "WIFI_PSK=livepassword\n") {
		t.Fatal("complete setup not applied")
	}
	if options.WifiSource != "manual" || options.WifiSSID != "Next" {
		t.Fatal("explicit eMMC apply changed next-image source")
	}
}
func TestWifiApplyRefusesPartialUnsupportedImage(t *testing.T) {
	dir := setupTest(t)
	state.State = IDLE
	options.LiveWifiSSID = "Live"
	options.LiveWifiPSK = "livepassword"
	fakeBin(t, dir, "target-manifest", `echo 'interface=1'; echo 'settings=WIFI_SSID,WIFI_PSK'`)
	fakeBin(t, dir, "target-install", `touch "`+dir+`/unexpected"`)
	w := wifiSetupRequest(`{"source":"reflash","action":"apply"}`)
	if w.Code != 400 {
		t.Fatal("unsupported setup accepted")
	}
	if _, err := os.Stat(filepath.Join(dir, "unexpected")); !os.IsNotExist(err) {
		t.Fatal("partially applied setup")
	}
}

func TestMissingInstalledWifiSecretDoesNotReplaceStagedSetup(t *testing.T) {
	dir := setupTest(t)
	state.State = IDLE
	options.WifiSSID = "Keep"
	options.WifiPSK = "keeppassword"
	fakeBin(t, dir, "target-manifest", `echo 'interface=1'; echo 'settings=WIFI_SSID,WIFI_PSK'`)
	fakeBin(t, dir, "target-install", `echo 'WIFI_SSID=NoSecret'`)
	w := wifiSetupRequest(`{"source":"emmc","action":"stage"}`)
	if w.Code != 400 || options.WifiSSID != "Keep" || options.WifiPSK != "keeppassword" {
		t.Fatal("incomplete installed setup replaced the previous choice")
	}
}

func TestBringupKeepsAlreadyConnectedLiveWifi(t *testing.T) {
	dir := setupTest(t)
	netdir := t.TempDir()
	os.Mkdir(filepath.Join(netdir, "wlan0"), 0755)
	t.Setenv("SYS_NET", netdir)
	t.Setenv("WIFI_INTERFACE", "wlan0")
	options.LiveWifiSSID = "Live"
	options.LiveWifiPSK = "livepassword"
	options.WifiSSID = "UnreachableNextImage"
	fakeBin(t, dir, "network-status", `echo '{"wifi":{"present":true,"mode":"station","ssid":"Live","ip":"192.168.1.2"}}'`)
	for _, name := range []string{"wifi-connect", "wifi-hotspot"} {
		fakeBin(t, dir, name, `touch "`+dir+`/unexpected"`)
	}
	bringupWifi()
	if _, err := os.Stat(filepath.Join(dir, "unexpected")); !os.IsNotExist(err) {
		t.Fatal("service restart disturbed the live connection")
	}
}
