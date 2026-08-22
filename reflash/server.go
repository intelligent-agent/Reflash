package main

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/grafana/tail"
	"github.com/pelletier/go-toml/v2"
	"golang.org/x/exp/slices"
)

type Image struct {
	Name string `json:"name"`
	Size int64  `json:"size"`
	Id   int    `json:"id"`
}

// GetInfo is what does not change while the page is open. Every field below
// costs a partition mount except the version, so this is fetched once and must
// never be polled.
type GetInfo struct {
	ReflashVersion string `json:"reflash_version"`
	RecoreRevision string `json:"recore_revision"`
	SerialNumber   string `json:"serial_number"`
	EmmcVersion    string `json:"emmc_version"`
}

// GetStatus is what does change: the image list after a download or upload,
// free space as it fills, and how the board is reachable. Nothing here mounts
// anything, so unlike GetInfo it is safe to call as often as the UI needs.
type GetStatus struct {
	LocalImages    []Image       `json:"local_images"`
	BytesAvailable int           `json:"bytes_available"`
	Network        NetworkStatus `json:"network"`
	// Storage is PREPARING until the USB drive is partitioned and mounted.
	// The server now starts before that finishes, so the UI needs to be able
	// to say "still working" rather than showing an empty image list as if the
	// drive were simply empty.
	Storage string `json:"storage"`
}

const (
	STORAGE_PREPARING = "PREPARING"
	STORAGE_READY     = "READY"
	STORAGE_FAILED    = "FAILED"
)

var storageState = STORAGE_PREPARING
var storageLock sync.Mutex

func setStorage(s string) {
	storageLock.Lock()
	storageState = s
	storageLock.Unlock()
	updateDisplay()
}

func getStorage() string {
	storageLock.Lock()
	defer storageLock.Unlock()
	return storageState
}

// NetworkStatus says how the board is reachable (#117). Both transports are
// reported independently because both can be up at once.
type NetworkStatus struct {
	Ethernet EthernetStatus    `json:"ethernet"`
	Wifi     WifiNetworkStatus `json:"wifi"`
}

type EthernetStatus struct {
	Up bool   `json:"up"`
	IP string `json:"ip"`
	// Active means this interface holds the winning default route. Both can be
	// up on one subnet, and then nothing else says which carries traffic (#112).
	Active bool `json:"active"`
}

type WifiNetworkStatus struct {
	Present bool   `json:"present"`
	Mode    string `json:"mode"` // "station" or "ap"
	SSID    string `json:"ssid"`
	IP      string `json:"ip"`
	// Signal strength in dBm, always negative. 0 means unknown.
	RSSI   int  `json:"rssi"`
	Active bool `json:"active"`
}

type GetSerialNumber struct {
	SerialNumber string `json:"serial_number"`
}

type GetWifi struct {
	SSID     string `json:"SSID"`
	Password string `json:"password"`
}

type AccessPoint struct {
	Frequency string `json:"frequency"`
	Signal    string `json:"signal"`
	Flags     string `json:"flags"`
	SSID      string `json:"SSID"`
}

type Options struct {
	Darkmode       bool   `json:"darkmode"`
	RebootWhenDone bool   `json:"rebootWhenDone"`
	EnableSsh      bool   `json:"enableSsh"`
	Magicmode      bool   `json:"magicmode"`
	ScreenRotation int    `json:"screenRotation"`
	WifiSSID       string `json:"SSID"`
	WifiPSK        string `json:"PSK"`
}

type Download struct {
	Filename  string `json:"filename"`
	Size      int    `json:"size"`
	StartTime int64  `json:"start_time"`
	Url       string `json:"url"`
}

type BinaryCommandResult struct {
	ExitCode int  `json:"exit_code"`
	Result   bool `json:"result"`
}

type StatusResult struct {
	Status string `json:"status"`
	Error  string `json:"error"`
}

type RotateCommand struct {
	Rotation int    `json:"rotation"`
	Where    string `json:"where"`
}

type UpdateConfigCommand struct {
	Snr int `json:"snr"`
}

type State struct {
	State      string   `json:"state"`
	Filename   string   `json:"filename"`
	StartTime  int64    `json:"start_time"`
	Progress   float64  `json:"progress"`
	Bandwidth  float32  `json:"bandwidth"`
	BytesNow   int      `json:"bytes_now"`
	BytesTotal int      `json:"bytes_total"`
	Error      string   `json:"error"`
	IPs        []string `json:"ips"`
	File       *os.File
	sync.Mutex
}

type WifiStatus struct {
	Connected bool   `json:"connected"`
	SSID      string `json:"ssid"`
	IP        string `json:"ip"`
	Device    string `json:"device"`
	State     string `json:"state"`
}

const (
	IDLE            = "IDLE"
	DOWNLOADING     = "DOWNLOADING"
	UPLOADING       = "UPLOADING"
	INSTALLING      = "INSTALLING"
	BACKUPING       = "BACKUPING"
	MAGIC           = "MAGIC"
	UPLOADING_MAGIC = "UPLOADING_MAGIC"
	FINISHED        = "FINISHED"
	CANCELLED       = "CANCELLED"
	ERROR           = "ERROR"
	SAVING          = "SAVING"
)

const (
	MODE_RO = "ro"
	MODE_RW = "rw"
)

var options *Options
var state *State

var oldState *State
var oldRotation int
var oldUsb = true

var static_dir string
var binDir string
var images_folder string
var options_file string
var log_file string
var http_port string
var reflashVersion string

// The FIFO the magic upload writes into, with flash-mkfifo's reader on the
// other end. A variable rather than a literal so tests can substitute a real
// FIFO in a temp dir - without that seam the whole magic path was unreachable
// from a test, which is how it stayed at 0% coverage (#118).
var magic_pipe = "/tmp/mypipe"

var last_size_check time.Time
var bytes_last int
var timeStart time.Time
var cancelFunc context.CancelFunc
var isDirty bool
var optionsLock sync.Mutex
var (
	cachedAccessPoints []AccessPoint
	isScanning         bool
	scanMutex          sync.Mutex
)
var (
	isConnecting bool
	connectError error
	connectMutex sync.Mutex
)

// bootPhase times one startup step and logs it. Unconditional: the whole point
// of measuring is that the boot budget was previously guessed at, and #116 was
// diagnosed from the journal after the fact rather than from anything Reflash
// recorded itself.
func bootPhase(name string, f func()) {
	t := time.Now()
	f()
	logInfo(fmt.Sprintf("boot: %-16s %6.2fs", name, time.Since(t).Seconds()))
}

// slowInit is everything that can block: mkfs on a fresh stick (measured at
// 198s on a worn one), mounting, and a WiFi connect that waits up to 30s for
// DHCP. Split out so it can be run either before the listener (current
// behaviour) or alongside it (the experiment).
// WiFi bring-up is off the critical path: it waits up to 30s for DHCP and
// nothing in startup depends on it, yet it was 2.62s of a 3.08s startup even
// when it went well. A package var like startInstall so tests can stub it and
// stay synchronous.
// How long to keep trying the mount before calling the drive unavailable.
// Vars so tests do not have to sit through it.
var (
	mountRetries    = 30
	mountRetryDelay = time.Second
)

var startWifiBringup = func() {
	go bootPhase("wifi-bringup", func() { bringupWifi() })
}

func slowInit() {
	bootPhase("get-ips", func() { state.IPs = getIPs() })
	// Wait for the drive to be ours before taking it, rather than racing for
	// it. Reflash does not create the partition either - ssh-keygen-boot is the
	// only caller of expand-usb now, and running it here as well printed a
	// second "Running expand USB script" that did nothing. Reflash mounts /mnt/usb and holds it for the life of the process, so
	// it has to mount last: ssh-keygen-boot needs the drive read-write first,
	// and a lock cannot share a mount with a process that never lets go.
	//
	// Confirmed live: mounting into that window stacked two mounts, after which
	// a remount read-write failed with "already mounted" and the write
	// underneath hit a read-only filesystem - the first options save was lost.
	bootPhase("wait-for-usb", func() {
		for i := 0; i < mountRetries; i++ {
			if runCommandReturnBool("usb-ready") {
				return
			}
			time.Sleep(mountRetryDelay)
		}
		logError("Timed out waiting for the USB drive to become available")
	})

	var mountErr error
	bootPhase("mount-usb", func() { mountErr = mountUsb(MODE_RO) })
	if mountErr != nil {
		// Do not carry on as if the drive were simply empty. loadOptions would
		// invent defaults and mark them dirty, and saveOptions would then write
		// options.cfg into the bare mountpoint on the tmpfs rootfs - losing the
		// user's settings on the next reboot, silently.
		logError("USB storage unavailable: " + mountErr.Error())
		setStorage(STORAGE_FAILED)
	} else {
		bootPhase("load-options", func() { loadOptions() })
		setStorage(STORAGE_READY)
	}

	startWatchdog()

	startWifiBringup()
}

func ServerInit() {
	static_dir = "/var/www/html/reflash/dist"
	binDir = "/usr/local/bin"
	images_folder = "/mnt/usb/images"
	options_file = "/mnt/usb/options.cfg"
	log_file = "/var/log/reflash.log"
	http_port = ":80"
	// Allow tests (and ad-hoc runs) to point the helper scripts elsewhere.
	if d := os.Getenv("REFLASH_BIN_DIR"); d != "" {
		binDir = d
	}

	// IPs are filled in by slowInit: get-hostnames shells out to `ip` and
	// getent, which is not something to do before the first frame.
	state = &State{
		State:      IDLE,
		BytesTotal: 1,
	}

	oldState = &State{
		State:      IDLE,
		BytesTotal: 1,
	}

	reflashVersion = runCommandReturnString("get-reflash-version")

	logInfo("-- Server started at " + time.Now().Format("15:04:05") + " --")

	// Neither of these touches the USB drive, so they are never worth delaying.
	go serveControlSocket(controlSocketPath())
	go serveSerialControl("/dev/ttyGS1")

	// Draw before doing anything slow, then get out of the way: the listener
	// starts immediately and the drive work happens behind it. Measured, this
	// is worth ~3s; the ~9s that used to sit in front of the server was
	// ssh-keygen-boot, fixed by dropping its ordering (see mkimage.sh).
	//
	// Rotation is a stored option on the drive, so this first frame is
	// unrotated and may flip once load-options completes.
	bootPhase("first-draw", func() {
		msg, progress, _ := storageFrame()
		Draw(float32(progress)/100, msg, 0, nil, reflashVersion)
	})
	go slowInit()

	go watchIPs()

	fs := http.FileServer(http.Dir(static_dir))
	fmt.Println("Starting Reflash go server " + reflashVersion)
	http.Handle("/", fs)
	http.HandleFunc("/api/get_info", getInfo)
	http.HandleFunc("/api/get_status", getStatus)
	http.HandleFunc("/api/stream_log", streamLog)
	http.HandleFunc("/api/get_options", getOptions)
	http.HandleFunc("/api/set_options", setOptions)
	http.HandleFunc("/api/start_download", startDownload)
	http.HandleFunc("/api/cancel_download", cancelDownload)
	http.HandleFunc("/api/upload_start", uploadStart)
	http.HandleFunc("/api/upload_finish", uploadFinish)
	http.HandleFunc("/api/upload_cancel", uploadCancel)
	http.HandleFunc("/api/upload_chunk", uploadChunk)
	http.HandleFunc("/api/start_installation", installRefactor)
	http.HandleFunc("/api/cancel_installation", cancelInstallation)
	http.HandleFunc("/api/reboot_board", rebootBoard)
	http.HandleFunc("/api/shutdown_board", shutdownBoard)
	http.HandleFunc("/api/is_usb_present", isUsbPresent)
	http.HandleFunc("/api/has_internet", hasInternet)
	http.HandleFunc("/api/start_backup", startBackup)
	http.HandleFunc("/api/cancel_backup", cancelBackup)
	http.HandleFunc("/api/start_magic", startMagic)
	http.HandleFunc("/api/cancel_magic", cancelMagic)
	http.HandleFunc("/api/upload_magic_start", uploadMagicStart)
	http.HandleFunc("/api/upload_magic_chunk", uploadMagicChunk)
	http.HandleFunc("/api/upload_magic_finish", uploadMagicFinish)
	http.HandleFunc("/api/get_progress", getProgress)
	http.HandleFunc("/api/check_file_integrity", checkFileIntegrity)
	http.HandleFunc("/api/run_install_finished_commands", runInstallFinishedCommands)
	http.HandleFunc("/api/clear_log", clearLog)
	http.HandleFunc("/api/rotate_screen", rotateScreen)
	http.HandleFunc("/api/update_config", updateConfig)
	http.HandleFunc("/api/is_config_present", isConfigPresent)
	http.HandleFunc("/api/get_serial_number", getSerialNumber)

	http.HandleFunc("/api/get_wifi", getWifi)
	http.HandleFunc("/api/wifi_start_scan", startScanWifi)
	http.HandleFunc("/api/wifi_poll_scan", getWifiScanResults)
	http.HandleFunc("/api/wifi_start_connect", startConnectWifi)
	http.HandleFunc("/api/wifi_start_hotspot", startHotspotWifi)
	http.HandleFunc("/api/wifi_poll_connect", pollConnectWifi)

	log.Fatal(http.ListenAndServe(http_port, nil))
}

func getInfo(w http.ResponseWriter, r *http.Request) {
	var get_info *GetInfo = &GetInfo{
		// Already read once at startup, and the file cannot change under a
		// running server - no reason to shell out again per request.
		ReflashVersion: reflashVersion,
		RecoreRevision: runCommandReturnString("get-recore-revision"),
		SerialNumber:   runCommandReturnString("get-recore-serial-number"),
		EmmcVersion:    runCommandReturnString("get-emmc-version"),
	}
	json.NewEncoder(w).Encode(get_info)
}

func getStatus(w http.ResponseWriter, r *http.Request) {
	var get_status *GetStatus = &GetStatus{
		LocalImages:    getLocalImages(),
		BytesAvailable: getFreeSpace(),
		Network:        getNetworkStatus(),
		Storage:        getStorage(),
	}
	json.NewEncoder(w).Encode(get_status)
}

// getNetworkStatus is the single source for "is a dongle fitted and what is it
// doing". It replaced a separate /api/get_wifi_status that the WiFi dialog
// polled every 2s: that poll raced the mode changes wifi-scan makes and kept
// reporting the dongle as missing mid-scan, and it never stopped (#115).
func getNetworkStatus() NetworkStatus {
	var status NetworkStatus
	out, _, err := runCommand2("network-status")
	if err != nil {
		return status
	}
	if err := json.Unmarshal([]byte(out), &status); err != nil {
		logError("Could not parse network-status output: " + err.Error())
	}
	return status
}

func getSerialNumber(w http.ResponseWriter, r *http.Request) {
	var get_serial_number *GetSerialNumber = &GetSerialNumber{
		SerialNumber: runCommandReturnString("get-recore-serial-number"),
	}
	json.NewEncoder(w).Encode(get_serial_number)
}

// getWifi reports the network Reflash is configured to join, so reopening the
// WiFi dialog comes back to it (#105).
//
// This used to shell out to `get-setting WIFI_SSID`, which could never work: it
// mounted the eMMC and then grepped /etc/rebuild-settings - the initrd's own
// path, not the mounted one - so it always returned nothing, and would have
// returned the whole "WIFI_SSID='x'" line rather than the value even if it had
// found the file. The value is right here in memory, already persisted to
// options.cfg, and needs no mount. (rebuild-settings is about the flashed
// image, which is a different question from what Reflash itself is using.)
//
// The passphrase is deliberately not returned. The dialog keeps it in memory
// for as long as the page is open, which covers reopening the dialog, and the
// server never echoes a secret back to a browser that may be on an open
// hotspot.
func getWifi(w http.ResponseWriter, r *http.Request) {
	optionsLock.Lock()
	ssid := options.WifiSSID
	optionsLock.Unlock()

	var get_wifi *GetWifi = &GetWifi{
		SSID: strings.TrimSpace(ssid),
	}
	json.NewEncoder(w).Encode(get_wifi)
}

// wifiAdapterPresent reports whether a WiFi interface exists at all. WiFi on
// Recore is a USB dongle, so on a board with none fitted there is nothing to
// connect to and nothing to run an AP on. The env seams and the wlan0 default
// mirror the bin/prod wifi-* scripts, which make the same check.
func wifiAdapterPresent() bool {
	iface := os.Getenv("WIFI_INTERFACE")
	if iface == "" {
		iface = "wlan0"
	}
	sysNet := os.Getenv("SYS_NET")
	if sysNet == "" {
		sysNet = "/sys/class/net"
	}
	_, err := os.Stat(filepath.Join(sysNet, iface))
	return err == nil
}

func bringupWifi() {
	// Without this the boot always ends in a failed wifi-connect: the script
	// refuses (correctly) and exits 1, which reads as an error in the log the
	// user is looking at even though nothing is wrong.
	if !wifiAdapterPresent() {
		logInfo("Boot: No WiFi adapter present, skipping WiFi bring-up.")
		return
	}

	// If we have saved credentials, try to connect immediately
	if options.WifiSSID != "" && options.WifiPSK != "" {
		logInfo("Boot: Attempting auto-connect to " + options.WifiSSID)
		runCommand2("wifi-connect", options.WifiSSID, options.WifiPSK)
	} else {
		logInfo("Boot: No SSID found, trying auto bring-up")
		runCommand2("wifi-bringup")
	}
}
func startScanWifi(w http.ResponseWriter, r *http.Request) {
	scanMutex.Lock()
	if isScanning {
		scanMutex.Unlock()
		w.WriteHeader(http.StatusAccepted) // 202: Already working on it
		return
	}
	isScanning = true
	scanMutex.Unlock()

	// Kick off the heavy lifting in a goroutine
	go func() {
		logInfo("WiFi Scan triggered...")

		// This is your existing logic, moved into a background task
		scan_results := runCommandReturnString("wifi-scan")

		var temp_aps []AccessPoint
		lines := strings.Split(scan_results, "\n")
		isDataZone := false

		for _, line := range lines {
			line = strings.TrimSpace(line)
			if line == "---SCAN_RESULTS_START---" {
				isDataZone = true
				continue
			}
			if line == "---SCAN_RESULTS_END---" {
				isDataZone = false
				break
			}
			if isDataZone && line != "" {
				parts := strings.Split(line, "|")
				if len(parts) >= 3 {
					temp_aps = append(temp_aps, AccessPoint{
						SSID:   parts[0],
						Flags:  parts[1],
						Signal: parts[2],
					})
				}
			}
		}

		// Update the cache and flip the flag
		scanMutex.Lock()
		cachedAccessPoints = temp_aps
		isScanning = false
		scanMutex.Unlock()
		logInfo("WiFi Scan complete.")
	}()

	// Return immediately so the UI stays responsive
	w.WriteHeader(http.StatusAccepted)
}

func getWifiScanResults(w http.ResponseWriter, r *http.Request) {
	scanMutex.Lock()
	defer scanMutex.Unlock()

	if isScanning {
		// HTTP 204 No Content tells the frontend "nothing yet, keep waiting"
		w.WriteHeader(http.StatusNoContent)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(cachedAccessPoints)
}

func startConnectWifi(w http.ResponseWriter, r *http.Request) {
	reqBody, _ := io.ReadAll(r.Body)
	var get_wifi GetWifi
	if err := json.Unmarshal(reqBody, &get_wifi); err != nil {
		http.Error(w, "Invalid JSON", 400)
		return
	}

	if len(strings.TrimSpace(get_wifi.Password)) < 8 {
		http.Error(w, "Password must be at least 8 characters", http.StatusBadRequest)
		return
	}

	// 1. Update Global Options and lock memory
	optionsLock.Lock()
	options.WifiSSID = strings.TrimSpace(get_wifi.SSID)
	options.WifiPSK = strings.TrimSpace(get_wifi.Password)
	isDirty = true
	optionsLock.Unlock()

	// 2. Prepare Connection State
	connectMutex.Lock()
	if isConnecting {
		connectMutex.Unlock()
		w.WriteHeader(http.StatusAccepted)
		return
	}
	isConnecting = true
	connectError = nil // Reset previous errors
	connectMutex.Unlock()

	// 3. Run connection in background
	go func() {
		logInfo("Attempting to connect to: " + options.WifiSSID)

		// This command usually takes down the AP and brings up the Station
		_, _, err := runCommand2("wifi-connect", options.WifiSSID, options.WifiPSK)

		connectMutex.Lock()
		connectError = err
		isConnecting = false
		connectMutex.Unlock()

		if err != nil {
			logError("WiFi Connection failed: " + err.Error())
		} else {
			logInfo("WiFi Connection successful")
		}
	}()

	// Respond immediately so Vue knows the process started
	w.WriteHeader(http.StatusAccepted)
}

// startHotspotWifi puts the adapter back into AP mode on request (#105). Async
// like startConnectWifi and for the same reason: if the caller reached this page
// over WiFi, the switch takes their connection down, so the response has to be
// sent before the work starts or it would never arrive.
func startHotspotWifi(w http.ResponseWriter, r *http.Request) {
	if !wifiAdapterPresent() {
		http.Error(w, "No WiFi adapter present", http.StatusBadRequest)
		return
	}
	go func() {
		logInfo("Switching to hotspot mode on request")
		if _, _, err := runCommand2("wifi-hotspot"); err != nil {
			logError("Could not start the hotspot: " + err.Error())
		}
	}()
	w.WriteHeader(http.StatusAccepted)
}

func pollConnectWifi(w http.ResponseWriter, r *http.Request) {
	connectMutex.Lock()
	defer connectMutex.Unlock()

	response := map[string]interface{}{
		"isConnecting": isConnecting,
		"error":        nil,
	}

	if connectError != nil {
		response["error"] = connectError.Error()
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}

func getOptions(w http.ResponseWriter, r *http.Request) {
	json.NewEncoder(w).Encode(options)
}

func setOptions(w http.ResponseWriter, r *http.Request) {
	reqBody, _ := io.ReadAll(r.Body)
	lockSetOptions(reqBody)
	json.NewEncoder(w).Encode(options)
}

func updateDisplay() {
	// Lazy init for paths that touch updateDisplay without going through
	// ServerInit (e.g. tests that drive handleSerialCommand directly).
	if oldState == nil {
		oldState = &State{}
	}
	// state needs the same treatment now that setStorage redraws: storage can
	// fail before ServerInit has finished building it.
	if state == nil {
		state = &State{State: IDLE, BytesTotal: 1}
	}
	// options is nil until loadOptions runs, and it does not run at all when
	// the drive fails to mount - which is exactly when the screen most needs
	// to say so. Reading ScreenRotation off it crashed the server on that
	// path, and Restart=always turned the crash into a restart loop.
	if options == nil {
		options = &Options{}
	}
	// Until the drive is mounted there is nothing meaningful to report about
	// flashing, and IDLE would draw an empty "ready" screen while the board is
	// still busy. Say what is actually happening instead - this is the frame
	// the user sees for most of the boot on a slow stick (#116).
	shown := state.State
	progress := float64(state.Progress)
	if msg, p, ok := storageFrame(); ok {
		shown, progress = msg, p
	}

	state.Lock()
	// oldUsb is in the comparison because the armed screen's text depends on
	// the drive being present, and nothing else in this condition changes when
	// it is pulled - without it the repaint from refreshUsbPresence is dropped
	// here and the panel never acknowledges the removal.
	usbNow := usbStillPresent()
	if oldState.State != shown || oldState.Progress != state.Progress || oldRotation != options.ScreenRotation || !slices.Equal(oldState.IPs, state.IPs) || oldUsb != usbNow {
		oldUsb = usbNow
		Draw(float32(progress)/100, shown, options.ScreenRotation, state.IPs, reflashVersion)
		oldState.State = shown
		oldState.Progress = state.Progress
		oldRotation = options.ScreenRotation
	}
	state.Unlock()
}

func streamLog(w http.ResponseWriter, r *http.Request) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "SSE not supported", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/event-stream")
	t, err := tail.TailFile(log_file, tail.Config{Follow: true})
	if err != nil {
		panic(err)
	}
	// Without watching the request context, this handler (and its tail
	// follower) never returns once the client goes away - e.g. every
	// time the log viewer's EventSource drops during a WiFi mode switch
	// (#95) - leaking one goroutine per dropped connection forever.
	defer t.Stop()

	// Confirmed live (#95): when the board's WiFi interface disappears
	// mid-stream (switching AP/station mode), the connection just goes
	// silent on both ends - no RST, no read error, nothing - so neither
	// side's error handling ever fires and the client waits forever. A
	// periodic heartbeat lets the client notice the silence and
	// reconnect on its own instead.
	heartbeat := time.NewTicker(15 * time.Second)
	defer heartbeat.Stop()

	for {
		select {
		case <-r.Context().Done():
			return
		case <-heartbeat.C:
			if _, err := fmt.Fprint(w, "event: heartbeat\ndata: \n\n"); err != nil {
				return
			}
			flusher.Flush()
		case line, ok := <-t.Lines:
			if !ok {
				return
			}
			fmt.Fprintf(w, "data: %s\n\n", line.Text)
			flusher.Flush()
		}
	}
}

func startDownload(w http.ResponseWriter, r *http.Request) {
	var data *Download = &Download{}
	reqBody, _ := io.ReadAll(r.Body)
	json.Unmarshal(reqBody, &data)

	state.Filename = data.Filename
	state.StartTime = data.StartTime
	url := data.Url
	state.BytesTotal = data.Size
	state.State = DOWNLOADING
	last_size_check = time.Now()
	bytes_last = 0
	mountUsb(MODE_RW)

	ctx, cancel := context.WithCancel(context.Background())
	cancelFunc = cancel

	go goDownload(ctx, state.Filename, url)

	sendResponse(w, nil)
}

func goDownload(ctx context.Context, filename string, url string) {
	disarmReboot()
	out, err := os.Create(images_folder + "/" + filename)
	if err != nil {
		panic(err)
	}

	resp, err := http.Get(url)
	if err != nil {
		panic(err)
	}

	timeStart = time.Now()
	logInfo(fmt.Sprintf("Starting download at %s", timeStart.Format("15:04:05")))

	done := make(chan bool)
	go func() {
		io.Copy(out, resp.Body)
		resp.Body.Close()
		out.Close()
		done <- true
	}()

	select {
	case <-ctx.Done():
		logInfo("Download cancelled.")
		os.Remove(images_folder + "/" + filename)
		state.State = CANCELLED
		mountUsb(MODE_RO)
		return
	case <-done:
		duration := time.Since(timeStart)
		logInfo(fmt.Sprintf("Download finished in %d minutes and %d seconds", int(duration.Minutes()), int(duration.Seconds())%60))
	}

	mountUsb(MODE_RO)

	state.State = FINISHED
}

func cancelDownload(w http.ResponseWriter, r *http.Request) {
	cancelFunc()
	sendResponse(w, nil)
}

func uploadStart(w http.ResponseWriter, r *http.Request) {
	var data *Download = &Download{}
	reqBody, _ := io.ReadAll(r.Body)
	json.Unmarshal(reqBody, &data)

	state.Filename = data.Filename
	state.StartTime = data.StartTime
	state.BytesNow = 0
	state.BytesTotal = data.Size
	state.State = UPLOADING
	uploadFailed = false
	markUploadStarted()
	mountUsb(MODE_RW)

	timeStart = time.Now()
	logInfo("Starting upload at " + timeStart.Format("15:04:05"))
	logInfo("Filename: " + state.Filename)
	f, err := os.Create(images_folder + "/" + state.Filename)
	if err != nil {
		// Report the failure instead of log.Fatal. This runs on the USB
		// drive, so a full disk or a drive that dropped off the bus is an
		// ordinary, recoverable outcome - and log.Fatal here killed the
		// whole Reflash server, taking the web UI and the log stream with
		// it, so the user saw the browser die rather than an error.
		logError("Could not create " + state.Filename + ": " + err.Error())
		state.State = ERROR
		state.Error = "Could not open the image file for writing. Check that the USB drive is present and has free space."
		mountUsb(MODE_RO)
		http.Error(w, state.Error, http.StatusInternalServerError)
		return
	}
	state.File = f

	sendResponse(w, nil)
}

func uploadMagicStart(w http.ResponseWriter, r *http.Request) {
	var data *Download = &Download{}
	reqBody, _ := io.ReadAll(r.Body)
	json.Unmarshal(reqBody, &data)

	state.Filename = data.Filename
	state.StartTime = data.StartTime
	state.BytesNow = 0
	state.BytesTotal = data.Size
	state.State = UPLOADING_MAGIC
	uploadFailed = false
	markUploadStarted()

	go goUploadMagic()
	time.Sleep(1 * time.Second)

	sendResponse(w, nil)
}

func goUploadMagic() {
	disarmReboot()
	timeStart = time.Now()
	logInfo("Starting magic upload at " + timeStart.Format("15:04:05"))
	logInfo("Filename: " + state.Filename)

	stdout, _, err := runCommand2("flash-mkfifo")
	if err != nil {
		logError("Error encountered when setting up pipe: \n" + stdout)
	}
	logInfo("flash-mkfifo done")
}

// A chunk that failed server-side used to return nothing but an HTTP status:
// no log line, and the state left as UPLOADING. The client responds to that by
// calling /upload/cancel, so all the journal showed was "Upload cancelled" -
// identical to the user pressing Cancel, which is exactly the confusion this
// cost us during testing. Record the failure here instead, with the offset it
// happened at, since on the magic path it means a partially written eMMC.
// See issue #114.
// getProgress clears ERROR as soon as it has reported it once, and during an
// upload the client is polling it every second - so by the time the client
// reacts to a failed chunk by calling /upload/cancel, the state has usually
// been reset to IDLE already and says nothing about why. Remember the failure
// separately; it is cleared when a new upload starts.
var uploadFailed bool

// An upload is only ever left by a request from the client that started it:
// upload_finish or upload_cancel. So a browser that goes away mid-upload - page
// refresh, closed tab, network drop - used to leave the server in UPLOADING
// forever: no other flash could start, state.File stayed open on a partial
// image, and the USB drive stayed mounted rw, which is exactly the state you do
// not want when the user reacts to a frozen UI by pulling the drive (#118).
//
// The watchdog is already ticking, so liveness is tracked here and checked
// there. chunksInFlight matters as much as the timestamp: a chunk blocked
// inside Write - on a slow drive, or on a FIFO whose reader is behind - looks
// exactly like an abandoned upload from the outside, and closing state.File
// underneath that write would turn a stall into a crash.
//
// Its own mutex rather than state's embedded one: the chunk handlers have never
// taken state.Lock(), so locking it here would guard the watchdog against
// lockSaveOptions (which cannot collide - both run on this same goroutine) and
// not against the writer this actually has to exclude.
var (
	uploadMutex    sync.Mutex
	lastChunkAt    time.Time
	chunksInFlight int
)

// This has to outlast the client's own patience, not the normal inter-chunk
// gap. The normal gap is well under a second, but uploadLocalFile is built to
// ride out the multi-minute dead spells this board's WiFi actually has (#61):
// 20s per-chunk timeout, 20 retries, exponential backoff capped at 30s. When
// the network is down the server sees nothing at all during that - the requests
// never arrive - so the gap it observes is the client's entire retry budget:
//
//	21 attempts x 20s timeout                       = 420s
//	backoffs 2+4+8+16s then 16 x 30s                = 510s
//	                                                 ~930s = 15.5 min
//
// Fire before that and the server kills an upload the client would have
// recovered, which is strictly worse than the bug being fixed here. After it,
// the client has already given up and sent upload_cancel, so the watchdog only
// ever acts when nobody is driving the upload at all - which is the point.
//
// The cost of erring long is that an abandoned upload leaves the drive mounted
// rw for up to this long. Bounded and recoverable, where before it was forever.
//
// The magic path needs less (300s timeout, no retry) and is covered anyway:
// its slow chunks are slow inside the handler - blocked on the FIFO or on
// io.ReadAll - so chunksInFlight holds them.
var uploadTimeout = 20 * time.Minute

// beginChunk admits a chunk only while the upload is still live, and marks it
// in flight so the watchdog cannot close the destination underneath it.
func beginChunk() bool {
	uploadMutex.Lock()
	defer uploadMutex.Unlock()
	if state.State == CANCELLED || state.State == ERROR {
		return false
	}
	chunksInFlight++
	return true
}

func endChunk() {
	uploadMutex.Lock()
	defer uploadMutex.Unlock()
	chunksInFlight--
	lastChunkAt = time.Now()
}

// markUploadStarted starts the clock at the start handler rather than at the
// first chunk, so an upload abandoned before it ever sends data still times
// out - which is the common case when a page is refreshed just after starting.
func markUploadStarted() {
	uploadMutex.Lock()
	defer uploadMutex.Unlock()
	lastChunkAt = time.Now()
	chunksInFlight = 0
}

// markUploadDone stops the clock, so the watchdog has nothing to act on once an
// upload has finished or been cancelled.
func markUploadDone() {
	uploadMutex.Lock()
	defer uploadMutex.Unlock()
	lastChunkAt = time.Time{}
}

// checkUploadLiveness gives up on an upload whose client has stopped driving
// it. Called from the watchdog tick.
func checkUploadLiveness() {
	uploadMutex.Lock()
	if state == nil || (state.State != UPLOADING && state.State != UPLOADING_MAGIC) ||
		chunksInFlight > 0 || lastChunkAt.IsZero() || time.Since(lastChunkAt) < uploadTimeout {
		uploadMutex.Unlock()
		return
	}
	// Leave UPLOADING while still holding the lock. beginChunk refuses to
	// admit anything once the state is ERROR, so by the time the file is
	// closed below, nothing can be writing to it.
	magic := state.State == UPLOADING_MAGIC
	silent := time.Since(lastChunkAt)
	state.State = ERROR
	// A late upload_cancel from a client that comes back must not relabel
	// this as a clean cancellation - same reasoning as a failed chunk (#114).
	uploadFailed = true
	lastChunkAt = time.Time{}
	uploadMutex.Unlock()

	if magic {
		// Closing the FIFO signals end-of-stream to the decompressor, so the
		// eMMC is left with a truncated image. Say so: the board will not
		// boot, and the user needs to know that rather than retrying into a
		// half-written device and wondering why.
		state.Error = "The magic flash stopped receiving data and was abandoned. The eMMC is partially written - reboot and flash again."
	} else {
		state.Error = "The upload stopped receiving data and was abandoned. Check the network connection and try again."
	}
	logError(fmt.Sprintf("No upload data for %d seconds; abandoning at %d of %d bytes",
		int(silent.Seconds()), state.BytesNow, state.BytesTotal))

	if state.File != nil {
		if err := state.File.Close(); err != nil {
			logError("Could not close the file: " + err.Error())
		}
		state.File = nil
	}
	// Get the drive back to read-only as soon as the file is flushed. Being
	// left writable is the part of this bug that can lose data, since a user
	// facing a stuck UI is likely to pull the drive.
	if !magic {
		mountUsb(MODE_RO)
	}
}

func failChunk(w http.ResponseWriter, code int, what string, userMsg string, err error) {
	progress := ""
	if state.BytesTotal > 0 {
		progress = fmt.Sprintf(" (%.0f%%)", float64(state.BytesNow)*100/float64(state.BytesTotal))
	}
	logError(fmt.Sprintf("%s after %d of %d bytes%s: %s",
		what, state.BytesNow, state.BytesTotal, progress, err.Error()))
	state.State = ERROR
	state.Error = userMsg
	uploadFailed = true
	http.Error(w, userMsg, code)
}

// chunkSink is everything the two upload paths do differently: where the bytes
// go, and what to say when they do not get there. Everything else - the
// CANCELLED/ERROR guard, reading the body, the write, the progress update, the
// response - was duplicated between uploadChunk and uploadMagicChunk, and the
// duplication was not free: the base64 -> raw-binary change had to be made
// twice, and both copies still carry a comment about it.
//
// The messages stay distinct on purpose. A failed plain upload has wasted the
// user's time; a failed magic upload has left a half-written eMMC that will not
// boot. Telling those apart is the whole value of #114, so they are parameters
// here rather than one shared string.
type chunkSink struct {
	// openLate opens the destination on the first chunk. nil when a start
	// handler has already opened it.
	openLate    func() (*os.File, error)
	openFailMsg string
	// destName appears in the write-failure log line, and is only evaluated
	// on failure since it can depend on state.
	destName func() string

	readLog, readMsg   string
	writeLog, writeMsg string
}

var plainSink = chunkSink{
	destName: func() string { return state.Filename },
	readLog:  "Could not read a chunk of the upload from the network",
	readMsg:  "The upload failed while receiving data. Check the network connection and try again.",
	writeLog: "Could not write a chunk of the upload to ",
	writeMsg: "The upload failed while writing to the USB drive. Check that it is present and has free space.",
}

var magicSink = chunkSink{
	// Unlike the plain upload path, the destination here is a FIFO that the
	// flashing process reads from, opened lazily on the first chunk rather
	// than in a start handler.
	openLate: func() (*os.File, error) {
		logInfo("Open file " + magic_pipe)
		return os.OpenFile(magic_pipe, os.O_APPEND|os.O_WRONLY, 0644)
	},
	openFailMsg: "Could not open the flash pipe. Reboot and try again.",
	destName:    func() string { return magic_pipe },
	readLog:     "Could not read a chunk of the magic upload from the network",
	readMsg:     "The magic flash failed while receiving data. The eMMC is partially written - reboot and try again.",
	writeLog:    "Could not write a chunk of the magic upload to ",
	writeMsg:    "The magic flash failed while writing to the flash pipe. The eMMC is partially written - reboot and try again.",
}

func uploadMagicChunk(w http.ResponseWriter, r *http.Request) {
	writeChunk(w, r, magicSink)
}

func uploadChunk(w http.ResponseWriter, r *http.Request) {
	writeChunk(w, r, plainSink)
}

func writeChunk(w http.ResponseWriter, r *http.Request, sink chunkSink) {
	// ERROR as well as CANCELLED: any chunk still in flight when one fails
	// must not overwrite state.Error with a fresh failure of its own. This
	// also admits the chunk for liveness purposes, so the watchdog cannot
	// close state.File while the write below is in progress (#118).
	if !beginChunk() {
		response := map[string]bool{"success": false}
		json.NewEncoder(w).Encode(response)
		return
	}
	defer endChunk()

	if state.File == nil && sink.openLate != nil {
		f, err := sink.openLate()
		if err != nil {
			// Same reasoning as uploadStart: failing to open the
			// destination must not take the whole server down mid-flash.
			logError("Could not open " + sink.destName() + ": " + err.Error())
			state.State = ERROR
			state.Error = sink.openFailMsg
			http.Error(w, state.Error, http.StatusInternalServerError)
			return
		}
		state.File = f
	}

	// The client posts the chunk as a raw binary body. This used to be
	// base64 inside a JSON body, which inflated the wire size by ~33% - on a
	// >1GB upload over this board's WiFi that is a lot of avoidable transfer.
	decoded, err := io.ReadAll(r.Body)
	if err != nil {
		failChunk(w, http.StatusBadRequest, sink.readLog, sink.readMsg, err)
		return
	}

	// state.File is opened once (in uploadStart, or on the first chunk for
	// the magic path) and closed in uploadFinish/uploadCancel - opening,
	// writing and closing it on every chunk was real per-chunk overhead
	// (syscalls plus an implicit flush on close), especially costly against
	// the FAT-formatted USB target.
	//
	// Chunks are sent one at a time (not pipelined/concurrent) - this
	// board's USB hub (WiFi NIC + storage share a single Transaction
	// Translator, confirmed from the hub's datasheet) can't reliably
	// handle simultaneous network-receive and disk-write transactions.
	// Concurrent chunks caused a real disk-write pileup (kernel threads
	// stuck in D-state) and, even after serializing the writes,
	// intermittent connection resets - a hardware constraint, not
	// something fixable by tuning the write path further.
	if _, err := state.File.Write(decoded); err != nil {
		failChunk(w, http.StatusInternalServerError,
			sink.writeLog+sink.destName(), sink.writeMsg, err)
		return
	}

	state.BytesNow += len(decoded)
	state.Progress = float64(state.BytesNow) * 100 / float64(state.BytesTotal)

	response := map[string]bool{"success": true}
	json.NewEncoder(w).Encode(response)
}

func uploadMagicFinish(w http.ResponseWriter, r *http.Request) {
	markUploadDone()
	// Closing the FIFO is what signals end-of-stream to the decompressor, so
	// a failure here means the flash is incomplete - but it is still an
	// error to report, not a reason to kill the server.
	if err := state.File.Close(); err != nil {
		logError("Could not close the flash pipe: " + err.Error())
		state.State = ERROR
		state.Error = "The flash did not complete cleanly. Reboot and try again."
		http.Error(w, state.Error, http.StatusInternalServerError)
		return
	}
	duration := time.Since(timeStart)
	logInfo(fmt.Sprintf("Upload magic finished in %d minutes and %d seconds", int(duration.Minutes()), int(duration.Seconds())%60))
	revision := runCommandReturnString("get-recore-revision")
	stdout, _, err := runCommand2("flash-cleanup", revision)
	if err != nil {
		logError("Error encountered during cleanup: \n" + stdout)
		state.State = ERROR
		state.Error = "An error was encountered during magic. Check log for details"
	} else {
		// Same tail as goInstall and goMagic. Without armReboot() this path
		// finished silently (#123): drawScreen() keys "Flash complete / Remove
		// USB drive" off the arm flag rather than the state, because
		// getProgress flips FINISHED to IDLE on the first poll - so the panel
		// never showed the prompt - and checkAutoReboot() returns at its first
		// line, so "reboot when done" did nothing at all on a magic upload.
		armReboot()
		state.State = FINISHED
		updateDisplay()
	}
}

func uploadFinish(w http.ResponseWriter, r *http.Request) {
	markUploadDone()
	if state.File != nil {
		if err := state.File.Close(); err != nil {
			// The last flush to the USB drive lands here, so this is
			// where a full disk shows up - an incomplete image to
			// report, not a reason to kill the server and the log
			// stream with it.
			logError("Could not close " + state.Filename + ": " + err.Error())
			state.Error = "The image was not written completely. Check that the USB drive has free space and try again."
			state.File = nil
			mountUsb(MODE_RO)
			state.State = ERROR
			return
		}
		state.File = nil
	}
	mountUsb(MODE_RO)
	duration := time.Since(timeStart)
	logInfo(fmt.Sprintf("Upload finished in %d minutes and %d seconds", int(duration.Minutes()), int(duration.Seconds())%60))
	state.State = FINISHED
}

func uploadCancel(w http.ResponseWriter, r *http.Request) {
	// The client calls this both when the user presses Cancel and after a
	// chunk has failed, so an ERROR state has to survive - overwriting it
	// with CANCELLED is what made the two indistinguishable in the first
	// place, in the UI as well as the log. See issue #114.
	markUploadDone()
	failed := uploadFailed || state.State == ERROR
	uploadFailed = false
	if !failed {
		state.State = CANCELLED
	}
	if state.File != nil {
		logInfo("Closing file")
		if err := state.File.Close(); err != nil {
			// Same reasoning as uploadStart: this runs on removable
			// media and feeds a pipe whose reader may already be gone,
			// and killing the server here would take the log stream
			// with it - precisely when there is something to read.
			logError("Could not close the file: " + err.Error())
		}
		state.File = nil
	}
	// timeStart is only set once an upload has actually started, and this
	// endpoint can be reached without one - printing the zero value gives a
	// duration in the hundreds of millions of minutes, which reads like a
	// bug in the very log line that is supposed to explain what happened.
	elapsed := ""
	if !timeStart.IsZero() {
		duration := time.Since(timeStart)
		elapsed = fmt.Sprintf(" after %d minutes and %d seconds", int(duration.Minutes()), int(duration.Seconds())%60)
	}
	if failed {
		logInfo("Upload aborted by the error above" + elapsed)
	} else {
		logInfo("Upload cancelled by the user" + elapsed)
	}
}

func startMagic(w http.ResponseWriter, r *http.Request) {
	var data *Download = &Download{}
	reqBody, _ := io.ReadAll(r.Body)
	json.Unmarshal(reqBody, &data)

	state.StartTime = data.StartTime
	url := data.Url
	state.BytesTotal = data.Size
	state.Filename = data.Filename
	state.State = MAGIC
	last_size_check = time.Now()
	bytes_last = 0
	go goMagic(url)
	time.Sleep(1 * time.Second)

	sendResponse(w, nil)
}

func goMagic(url string) {
	disarmReboot()
	timeStart = time.Now()
	logInfo(fmt.Sprintf("Starting magic at %s", timeStart.Format("15:04:05")))
	logInfo(fmt.Sprintf("Url %s", url))

	stdout, _, err := runCommand2("flash-from-url", url)
	if err != nil {
		logError("Error encountered during magic: \n" + stdout)
		state.State = ERROR
		// flash-from-url prints the specific reason (e.g. the real download
		// error, not just "an unknown error" - #59) as its last line before
		// exiting non-zero. Surface that instead of a generic message.
		state.Error = "An error was encountered during magic"
		lines := strings.Split(strings.TrimSpace(stdout), "\n")
		if lastLine := lines[len(lines)-1]; lastLine != "" {
			state.Error = lastLine
		}
		return
	}

	duration := time.Since(timeStart)
	logInfo(fmt.Sprintf("Magic finished in %d minutes and %d seconds", int(duration.Minutes()), int(duration.Seconds())%60))

	// Arm before publishing FINISHED, not after: getProgress and the panel
	// both read the flag, and a poll landing between the two saw a finished
	// flash with no "Remove USB drive" prompt (#123).
	armReboot()
	state.State = FINISHED
	updateDisplay()
}

func cancelMagic(w http.ResponseWriter, r *http.Request) {
	duration := time.Since(timeStart)

	_, _, err := runCommand2("pkill", "-f", "xz", "-9")
	logInfo(fmt.Sprintf("Magic cancelled after %d minutes and %d seconds", int(duration.Minutes()), int(duration.Seconds())%60))
	state.State = CANCELLED
	sendResponse(w, err)
}

func startBackup(w http.ResponseWriter, r *http.Request) {
	var data *Download = &Download{}
	reqBody, _ := io.ReadAll(r.Body)
	json.Unmarshal(reqBody, &data)

	state.Filename = data.Filename
	state.StartTime = data.StartTime

	state.BytesTotal = getBlockSize("/dev/mmcblk2")
	state.State = BACKUPING
	mountUsb(MODE_RW)

	go goBackup()
	time.Sleep(2 * time.Second)

	sendResponse(w, nil)
}

func goBackup() {
	disarmReboot()
	path := images_folder + "/" + state.Filename

	timeStart = time.Now()
	logInfo(fmt.Sprintf("starting backup of %s at time %s", state.Filename, timeStart.Format("15:04:05")))

	stdout, _, err := runCommand2("backup-emmc", path)
	mountUsb(MODE_RO)

	// Plain reads/writes of state.State here aren't enough to see
	// cancelBackup()'s write reliably - without a shared lock, Go's memory
	// model doesn't guarantee this goroutine observes that write just
	// because it happened first in wall-clock time. state.Lock() gives the
	// same guarantee cancelBackup()'s matching lock below relies on.
	state.Lock()
	defer state.Unlock()
	if err != nil {
		// cancelBackup() kills the backup subprocess to stop it, which makes
		// runCommand2 return an error here too (exit 137 - SIGKILL) - that's
		// the expected result of a deliberate cancel, not a real failure.
		// cancelBackup() sets state to CANCELLED before killing specifically
		// so this check can tell the two apart instead of overwriting the
		// cancellation with a generic error.
		if state.State == CANCELLED {
			return
		}
		logError("Error encountered during backup: \n" + stdout)
		state.State = ERROR
		state.Error = "An error was encountered during backup. Check log for details"
		return
	}

	duration := time.Since(timeStart)
	logInfo(fmt.Sprintf("Backup finished in %d minutes and %d seconds", int(duration.Minutes()), int(duration.Seconds())%60))
	state.State = FINISHED
}

func cancelBackup(w http.ResponseWriter, r *http.Request) {
	duration := time.Since(timeStart)
	logInfo(fmt.Sprintf("Backup cancelled after %d minutes and %d seconds", int(duration.Minutes()), int(duration.Seconds())%60))

	// Set before killing, not after - goBackup() is blocked waiting on the
	// subprocess and wakes up as soon as the kill lands, so this needs to
	// already be visible by then to avoid a race where it sees the kill's
	// resulting error first and reports it as a generic failure instead.
	// Locked so that guarantee actually holds under Go's memory model, not
	// just in wall-clock time - see goBackup().
	state.Lock()
	state.State = CANCELLED
	state.Error = ""
	state.Unlock()

	cmd := exec.Command("pkill", "-f", "xz", "-9")
	err := cmd.Run()
	if err != nil {
		if exitError, ok := err.(*exec.ExitError); ok {
			logError(fmt.Sprintf("Command 'pkill -f xz -9' returned exit code %v\n", exitError.ExitCode()))
		}
	}
	os.Remove(images_folder + "/" + state.Filename)
	sendResponse(w, err)
}

func getBlockSize(file string) int {
	return runCommandReturnInt("lsblk", "-n", "-d", "-o", "SIZE", "--bytes", file)
}

// refreshProgress reads the active progress source (the flash-progress file
// while installing/backing up/magicking, or the downloading file's size),
// recomputes state.Progress + state.Bandwidth, and redraws the embedded
// screen. Called from both the HTTP getProgress handler and the USB STATUS
// dispatcher so both paths keep the on-board display alive.
func refreshProgress() {
	if state.State == INSTALLING || state.State == BACKUPING || state.State == MAGIC {
		bytes := lastLine("/tmp/recore-flash-progress")
		i, err := strconv.Atoi(bytes)
		if err != nil {
			i = 0
		}
		state.BytesNow = i
	} else if state.State == DOWNLOADING {
		fi, err := os.Stat(images_folder + "/" + state.Filename)
		if err == nil {
			state.BytesNow = int(fi.Size())
		}
	}

	if state.BytesTotal > 0 {
		state.Progress = (float64(state.BytesNow) / float64(state.BytesTotal)) * 100.0
	}
	elapsed := time.Now().Sub(last_size_check).Seconds()
	last_size_check = time.Now()
	bytes_diff_mb := float32(state.BytesNow-bytes_last) / (1024 * 1024)
	bytes_last = state.BytesNow
	state.Bandwidth = bytes_diff_mb / float32(elapsed)

	logBandwidth()
	updateDisplay()
}

// Bandwidth was computed for the UI chart and thrown away, so a transfer that
// stalled left nothing behind to look at once the browser was closed - and the
// log is what we have after the fact. Sampled rather than logged per poll: the
// client polls once a second, and 60 lines a minute would bury everything else
// in a log that lives in a tmpfs and is streamed to the browser.
var (
	bandwidthLogEvery = 30 * time.Second
	lastBandwidthLog  time.Time
	bytesAtLastLog    int
)

// Averaged over the whole window rather than reported from state.Bandwidth.
// That field is recomputed on every poll, so it is an instantaneous ~1s rate,
// and sampling it every 30s logs whichever second happened to land on the
// tick. On the run this was written for it printed "0.00 MB/s" twice while the
// transfer was actually moving 2.93 and 4.15 MB/s - so a real stall and an
// unlucky sample were indistinguishable, which defeats the point of logging it.
func logBandwidth() {
	if state.BytesTotal <= 0 {
		return
	}
	now := time.Now()
	if lastBandwidthLog.IsZero() {
		lastBandwidthLog = now
		bytesAtLastLog = state.BytesNow
		return
	}
	window := now.Sub(lastBandwidthLog)
	if window < bandwidthLogEvery {
		return
	}
	mb := float64(state.BytesNow-bytesAtLastLog) / (1024 * 1024)
	lastBandwidthLog = now
	bytesAtLastLog = state.BytesNow
	logInfo(fmt.Sprintf("%s: %.2f MB/s (%d of %d bytes, %.0f%%)",
		state.State, mb/window.Seconds(), state.BytesNow, state.BytesTotal, state.Progress))
}

func getProgress(w http.ResponseWriter, r *http.Request) {
	refreshProgress()
	json.NewEncoder(w).Encode(state)
	if state.State == FINISHED {
		state.State = IDLE
	}
	if state.State == CANCELLED {
		mountUsb(MODE_RO)
		state.State = IDLE
	}
	if state.State == ERROR {
		state.State = IDLE
	}
}

func getLocalImages() []Image {
	entries, err := filepath.Glob(images_folder + "/*.img.xz")
	if err != nil {
		log.Fatal(err)
	}

	images := []Image{}
	for i, name := range entries {
		fi, _ := os.Stat(name)
		var image Image = Image{
			Name: filepath.Base(name),
			Size: fi.Size(),
			Id:   i,
		}
		images = append(images, image)
	}

	return images
}

func checkFileIntegrity(w http.ResponseWriter, r *http.Request) {
	var data *Download = &Download{}
	reqBody, _ := io.ReadAll(r.Body)
	json.Unmarshal(reqBody, &data)

	filename := data.Filename
	path := images_folder + "/" + filename
	_, _, err := runCommand2("xz", "-l", path)
	ret := err == nil
	response := map[string]bool{"is_file_ok": ret}
	json.NewEncoder(w).Encode(response)
}

func installRefactor(w http.ResponseWriter, r *http.Request) {
	var data *Download = &Download{}
	reqBody, _ := io.ReadAll(r.Body)
	json.Unmarshal(reqBody, &data)

	state.Filename = data.Filename
	state.StartTime = data.StartTime
	state.BytesTotal = getUncompressedSize(images_folder + "/" + data.Filename)
	state.State = INSTALLING

	go goInstall(state.Filename)
	time.Sleep(1 * time.Second)

	sendResponse(w, nil)
}

func goInstall(filename string) {
	disarmReboot()
	path := images_folder + "/" + filename

	timeStart = time.Now()
	logInfo(fmt.Sprintf("starting install at %s", timeStart.Format("15:04:05")))
	logInfo(fmt.Sprintf("Filename %s", filename))

	stdout, _, err := runCommand2("flash-from-file", path)
	if err != nil {
		logError("Error encountered during install: \n" + stdout)
		state.State = ERROR
		state.Error = "An error was encountered during install. Check log for details"
		return
	}

	duration := time.Since(timeStart)
	logInfo(fmt.Sprintf("Installation finished in %d minutes and %d seconds", int(duration.Minutes()), int(duration.Seconds())%60))

	// Arm before publishing FINISHED, not after: getProgress and the panel
	// both read the flag, and a poll landing between the two saw a finished
	// flash with no "Remove USB drive" prompt (#123).
	armReboot()
	state.State = FINISHED
	updateDisplay()
}

func getUncompressedSize(path string) int {
	cmd := exec.Command("xz", "--robot", "-l", path)
	stdout, err := cmd.Output()
	if err != nil {
		if exitError, ok := err.(*exec.ExitError); ok {
			logError(fmt.Sprintf("Command 'xz --robot -l %s' returned exit code %v\n", path, exitError.ExitCode()))
			return 1
		}
	}
	return parseXzUncompressedSize(string(stdout[:]))
}

// parseXzUncompressedSize extracts the uncompressed byte count from
// `xz --robot -l` output. The "totals" line is tab-separated with the
// uncompressed size (exact bytes) in field 4, e.g.:
//
//	totals\t1\t1\t448\t2097152\t0.000\tCRC64\t0\t1
//
// This is unit-agnostic, unlike the human-readable `xz -l` table.
func parseXzUncompressedSize(out string) int {
	for _, line := range strings.Split(out, "\n") {
		fields := strings.Split(line, "\t")
		if len(fields) >= 5 && fields[0] == "totals" {
			if n, err := strconv.Atoi(fields[4]); err == nil {
				return n
			}
		}
	}
	return 0
}

func lastLine(file string) string {
	out, err := exec.Command("tail", "-n1", file).Output()
	if err != nil {
		// Expected before the progress file exists yet (e.g. right at
		// the start of a flash, or in tests) - the caller already
		// falls back to 0 on a non-numeric result. log.Fatal here
		// used to kill the whole process on this, not just this one
		// read.
		return ""
	}
	return strings.TrimSpace(string(out[:]))
}

func cancelInstallation(w http.ResponseWriter, r *http.Request) {
	cmd := exec.Command("pkill", "-f", "xz", "-9")
	err := cmd.Run()
	if err != nil {
		if exitError, ok := err.(*exec.ExitError); ok {
			logError(fmt.Sprintf("Command 'pkill -f xz -9' returned exit code %v\n", exitError.ExitCode()))
		}
	}
	sendResponse(w, err)
}

func runInstallFinishedCommands(w http.ResponseWriter, r *http.Request) {
	var err error
	err = cmdRotateScreen(options.ScreenRotation, "CMDLINE")
	if err != nil {
		sendResponse(w, err)
	}
	err = cmdRotateScreen(options.ScreenRotation, "XORG")
	if err != nil {
		sendResponse(w, err)
	}
	err = cmdRotateScreen(options.ScreenRotation, "WESTON")
	if err != nil {
		sendResponse(w, err)
	}
	err = cmdRotateScreen(options.ScreenRotation, "PLYMOUTH")
	if err != nil {
		sendResponse(w, err)
	}

	settings := "# Settings from Reflash\n" +
		"SSH_ENABLED_ON_BOOT=" + strconv.FormatBool(options.EnableSsh) + "\n" +
		"SSH_TIMEOUT=60\n" +
		"EXTERNAL_SCREEN_ROTATION=" + strconv.FormatInt(int64(options.ScreenRotation), 10) + "\n" +
		"WIFI_SSID='" + options.WifiSSID + "'\n" +
		"WIFI_PSK='" + options.WifiPSK + "'"

	runCommand2("save-settings", settings)
	err = unmountUsb()
	sendResponse(w, err)
}

func sendResponse(w http.ResponseWriter, err error) {
	var response *StatusResult = &StatusResult{}

	if err == nil {
		response.Status = "OK"
	} else {
		response.Status = "ERROR"
		response.Error = err.Error()
	}
	json.NewEncoder(w).Encode(response)
}

// resolveCmd maps a Reflash helper-script name to its full path under binDir.
// System utilities already on PATH (and any name given as an explicit path) are
// returned unchanged.
func resolveCmd(name string) string {
	switch name {
	case "pkill", "xz", "tail", "lsblk", "sync":
		return name
	}
	if strings.ContainsRune(name, '/') {
		return name
	}
	return filepath.Join(binDir, name)
}

func runCommandReturnBool(cmd_str string) bool {
	cmd := exec.Command(resolveCmd(cmd_str))
	stdout, err := cmd.Output()
	if err != nil {
		if exitError, ok := err.(*exec.ExitError); ok {
			logError(fmt.Sprintf("Command '%s' returned exit code %v\n", cmd_str, exitError.ExitCode()))
			return false
		}
	}
	ret, err := strconv.ParseBool(strings.TrimSpace(string(stdout[:])))
	return ret
}

func runCommandReturnInt(cmds ...string) int {
	stdout, _, _ := runCommand2(cmds...)
	ret, _ := strconv.Atoi(strings.TrimSpace(stdout))
	return int(ret)
}

func runCommandReturnString(cmd_str string) string {
	stdout, _, _ := runCommand2(cmd_str)
	return strings.TrimSpace(stdout)
}

func rebootBoard(w http.ResponseWriter, r *http.Request) {
	_, _, err := runCommand2("reboot-board")
	sendResponse(w, err)
}

func shutdownBoard(w http.ResponseWriter, r *http.Request) {
	_, _, err := runCommand2("shutdown-board")
	sendResponse(w, err)
}

func isConfigPresent(w http.ResponseWriter, r *http.Request) {
	_, _, err := runCommand2("get-recore-revision")
	sendResponse(w, err)
}

func rotateScreen(w http.ResponseWriter, r *http.Request) {
	var data *RotateCommand = &RotateCommand{}
	reqBody, _ := io.ReadAll(r.Body)
	json.Unmarshal(reqBody, &data)

	err := cmdRotateScreen(data.Rotation, data.Where)
	sendResponse(w, err)
}

func updateConfig(w http.ResponseWriter, r *http.Request) {
	var data *UpdateConfigCommand = &UpdateConfigCommand{}
	reqBody, _ := io.ReadAll(r.Body)
	json.Unmarshal(reqBody, &data)
	out, _, err := runCommand2("create-recore-config", strconv.Itoa(data.Snr))
	// create-recore-config prints a specific, user-facing reason (missing
	// calibration file vs. no internet connection) before exiting non-zero -
	// surface that instead of the generic "exit status N" from err.
	if err != nil {
		lines := strings.Split(strings.TrimSpace(out), "\n")
		if lastLine := lines[len(lines)-1]; lastLine != "" {
			err = fmt.Errorf("%s", lastLine)
		}
	}
	sendResponse(w, err)
}

func cmdRotateScreen(rotation int, place string) error {
	_, _, err := runCommand2("rotate-screen", strconv.Itoa(rotation), place)
	return err
}

func isUsbPresent(w http.ResponseWriter, r *http.Request) {
	result := runCommandReturnBool("is-usb-present")
	var response *BinaryCommandResult = &BinaryCommandResult{
		Result: result,
	}
	json.NewEncoder(w).Encode(response)
}

func hasInternet(w http.ResponseWriter, r *http.Request) {
	result := runCommandReturnBool("has-internet")
	var response *BinaryCommandResult = &BinaryCommandResult{
		Result: result,
	}
	json.NewEncoder(w).Encode(response)
}

func logInfo(msg string) {
	log_msg("[info] " + msg)
}
func logError(msg string) {
	log_msg("[error] " + msg)
}

// Every log line opens, appends to and closes the log file, and all three
// steps used to be log.Fatal - so a server that could not write its log
// killed itself instead of reporting it. That is not hypothetical here: the
// rootfs is the initrd unpacked into a tmpfs capped at half of RAM, and this
// image has historically booted full enough that any write hits ENOSPC (see
// the note in mkimage.sh).
//
// Failures are reported to stderr, never through logError - that would come
// straight back here and recurse. The message is not lost either way: it has
// already gone to stdout, which systemd captures in the journal.
func log_msg(msg string) {
	fmt.Println(msg)
	file, err := os.OpenFile(log_file, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		fmt.Fprintln(os.Stderr, "Could not open "+log_file+": "+err.Error())
		return
	}
	defer file.Close()
	if _, err := file.Write([]byte(msg + "\n")); err != nil {
		fmt.Fprintln(os.Stderr, "Could not write to "+log_file+": "+err.Error())
	}
}

func clearLog(w http.ResponseWriter, r *http.Request) {
	file, err := os.OpenFile(log_file, os.O_RDWR, 0644)
	if err != nil {
		// O_RDWR without O_CREATE, so this fails outright if the log file
		// is not there yet - and it was a log.Fatal, which made the Clear
		// log button in the UI able to kill the server in one click.
		logError("Could not open " + log_file + " to clear it: " + err.Error())
		http.Error(w, "Could not clear the log", http.StatusInternalServerError)
		return
	}
	defer file.Close()
	if err := file.Truncate(0); err != nil {
		logError("Could not truncate " + log_file + ": " + err.Error())
		http.Error(w, "Could not clear the log", http.StatusInternalServerError)
		return
	}
	log_msg("--- Log start ---")

	response := map[string]int{"status": 0}
	json.NewEncoder(w).Encode(response)
}

// Helper commands that are handed a secret on the command line, and which
// argument holds it. runCommand2 logs the argv of anything that fails, and
// /var/log/reflash.log is streamed straight to the browser log viewer - so
// without this the user's WiFi passphrase is printed in clear text every time
// one of these fails, which is on every boot of a board with no adapter.
var secretArgs = map[string][]int{
	"wifi-connect":  {2}, // wifi-connect <SSID> <PASSPHRASE>
	"save-settings": {1}, // the settings blob carries WIFI_PSK='...'
}

func redactArgs(cmds []string) []string {
	idxs, ok := secretArgs[cmds[0]]
	if !ok {
		return cmds
	}
	out := append([]string(nil), cmds...)
	for _, i := range idxs {
		if i < len(out) && out[i] != "" {
			out[i] = "<redacted>"
		}
	}
	return out
}

func runCommand2(cmds ...string) (string, string, error) {
	cmd := exec.Command(resolveCmd(cmds[0]), cmds[1:]...)
	var out bytes.Buffer
	var stderr bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &stderr
	err := cmd.Run()
	if err != nil {
		logError(fmt.Sprintf("%s", redactArgs(cmds)) + ": " + fmt.Sprint(err) + ": " + strings.TrimSpace(stderr.String()))
	}
	return out.String(), stderr.String(), err
}

func mountUsb(mode string) error {
	_, _, err := runCommand2("mount-unmount-usb", "mounted", mode)
	return err
}

func unmountUsb() error {
	_, _, err := runCommand2("mount-unmount-usb", "unmounted", "")
	return err
}

func getFreeSpace() int {
	return runCommandReturnInt("get-free-space")
}

func getIPs() []string {
	ips := runCommandReturnString("get-hostnames")
	return strings.Split(ips, "\n")
}

func getRecoreRevision() string {
	return runCommandReturnString("get-recore-revision")
}

func saveOptions() error {
	var err error
	mountUsb(MODE_RW)
	content, _ := toml.Marshal(options)
	err = os.WriteFile(options_file, content, 0644)
	logInfo("Options saved")
	mountUsb(MODE_RO)
	return err
}

func loadOptions() {
	optionsLock.Lock()
	defer optionsLock.Unlock()

	content, err := os.ReadFile(options_file)
	if err != nil {
		logInfo("No options file found, creating default")
		options = &Options{
			Darkmode:       true,
			RebootWhenDone: true,
			EnableSsh:      true,
			ScreenRotation: 0,
		}
		isDirty = true
	} else {
		toml.Unmarshal(content, &options)
		logInfo("Options loaded from disk successfully")
	}
}

func lockSetOptions(opts []byte) error {
	optionsLock.Lock()
	defer optionsLock.Unlock()

	err := json.Unmarshal(opts, &options)
	if err != nil {
		return err
	}

	isDirty = true
	logInfo("Options updated in memory and marked dirty")

	return nil
}

func lockSaveOptions() {
	// 1. Pre-check: Is there actually work to do?
	optionsLock.Lock()
	if !isDirty {
		optionsLock.Unlock()
		return // Nothing changed, go back to sleep
	}
	optionsLock.Unlock()

	// 2. State-check: Is the system busy with something else?
	state.Lock()
	if state.State != IDLE {
		state.Unlock()
		// We leave isDirty = true so the watchdog tries again next tick
		return
	}

	// 3. Begin the Save Sequence
	state.State = SAVING // Lock the state so others know the disk is busy
	state.Unlock()

	logInfo("Starting thread-safe save operation...")

	// 4. Perform the Hardware I/O
	// This calls your existing logic: mount RW -> write -> mount RO
	err := saveOptions()

	// 5. Cleanup and Reset
	state.Lock()
	if err != nil {
		logError("Save failed: " + err.Error())
		// Note: we don't reset isDirty here so it retries later
	} else {
		isDirty = false
		logInfo("Save successful, dirty flag cleared.")
	}

	state.State = IDLE
	state.Unlock()
	updateDisplay()
}

// storageFrame is what the panel shows while the drive is not usable, and
// ok=false once it is and the flash state should be shown instead.
//
// Shared by the first frame and every later redraw. They used to decide
// separately, and the first one passed a progress of 0 where the other passed
// -1 - so a progress bar appeared for the moment before the first redraw
// replaced it.
//
// Negative progress means no bar: neither of these has a measurable duration.
// mkfs on a worn stick took 198s (#116) and reports nothing along the way, and
// a bar pinned at zero reads as stuck where the message alone reads as working.
func storageFrame() (string, float64, bool) {
	switch getStorage() {
	case STORAGE_PREPARING:
		return "Preparing USB drive", -1, true
	case STORAGE_FAILED:
		return "No USB drive", -1, true
	}
	return "", 0, false
}

// refreshIPs re-reads the addresses and redraws if they changed.
func refreshIPs() {
	ips := getIPs()
	state.Lock()
	changed := !slices.Equal(state.IPs, ips)
	state.IPs = ips
	state.Unlock()
	if changed {
		shown := strings.Join(ips, " ")
		if shown == "" {
			shown = "(none)"
		}
		logInfo("Addresses changed: " + shown)
		updateDisplay()
	}
}

// How long to gather further address events before re-reading. One change
// emits several netlink messages, and each re-read costs a get-hostnames
// (~0.8s measured), so coalescing matters more than reacting instantly.
var ipEventDebounce = 500 * time.Millisecond

// watchIPs refreshes the panel when the addresses actually change.
//
// This replaced a single timer that fired three seconds after startup. That is
// before WiFi has associated - measured at ~15s to a lease - so the panel
// showed whatever was true at three seconds for the whole session, and a board
// on WiFi alone never displayed an address at all.
func watchIPs() {
	refreshIPs()
	for {
		cmd := exec.Command(resolveCmd("watch-ips"))
		stdout, err := cmd.StdoutPipe()
		if err != nil || cmd.Start() != nil {
			time.Sleep(2 * time.Second)
			continue
		}

		events := make(chan struct{}, 1)
		go func() {
			scanner := bufio.NewScanner(stdout)
			for scanner.Scan() {
				// Non-blocking: a burst collapses into the one buffered slot
				// rather than queueing a re-read per netlink message.
				select {
				case events <- struct{}{}:
				default:
				}
			}
			close(events)
		}()

		for range events {
			time.Sleep(ipEventDebounce)
			select { // swallow anything that arrived while settling
			case <-events:
			default:
			}
			refreshIPs()
		}

		cmd.Wait()
		// The monitor should not exit; if it does, do not spin on restarting it.
		time.Sleep(2 * time.Second)
	}
}

func startWatchdog() {
	// 500ms, matching what the web UI used to poll at. The server is now the
	// only thing that reboots on drive removal (the UI no longer races it), so
	// this interval is what the user actually waits between pulling the drive
	// and the board restarting - at 2s that was a noticeable lag, and it made
	// the panel's "USB removed" frame nearly impossible to see.
	//
	// Affordable because every step below returns immediately when there is
	// nothing to do: lockSaveOptions on !isDirty, the armed-window work on
	// !isRebootArmed(), the liveness check on a state that is not UPLOADING.
	// A tick costs four mutex acquisitions when idle.
	ticker := time.NewTicker(500 * time.Millisecond)

	go func() {
		for range ticker.C {
			lockSaveOptions()
			// Repaint the panel when the drive comes or goes, and reboot into
			// the freshly flashed image once it is pulled. Both come off one
			// USB sample - see armedTick.
			refreshUsbPresence()
			// Give up on an upload whose client has gone away, rather than
			// sitting in UPLOADING with the drive mounted rw forever (#118).
			checkUploadLiveness()
		}
	}()
}

// rebootArmed is set when a flash to the eMMC (magic or file install) finishes
// successfully. The watchdog then reboots the board once the user removes the
// USB drive — so the post-flash reboot no longer depends on a browser polling
// is_usb_present from the web UI.
var (
	rebootMutex sync.Mutex
	rebootArmed bool
)

// usbWasPresent is what the panel last drew. Cached rather than probed from
// Draw(): usbPresent() shells out to is-usb-present, and the draw path must not
// block on a subprocess.
//
// Guarded by rebootMutex because it is only meaningful while rebootArmed is
// set, and that keeps it to one lock rather than introducing another.
var usbWasPresent = true

func usbStillPresent() bool {
	rebootMutex.Lock()
	defer rebootMutex.Unlock()
	return usbWasPresent
}

func rebootWhenDone() bool {
	optionsLock.Lock()
	defer optionsLock.Unlock()
	return options != nil && options.RebootWhenDone
}

// refreshUsbPresence repaints the panel when the drive is pulled while a
// finished flash waits for it. Nothing else notices: the web UI polls
// is_usb_present itself and drives its own reboot, and checkAutoReboot only
// consults the drive to decide whether to reboot - so the panel went on saying
// "Remove USB drive" after it had been removed, with no acknowledgement until
// the screen changed for an unrelated reason.
//
// Does nothing unless a flash is armed, so it costs one is-usb-present per 2s
// only in the window between a finished flash and the reboot.
func refreshUsbPresence() {
	if !isRebootArmed() {
		return
	}
	armedTick(usbPresent())
}

// armedTick is everything the armed window needs from one USB sample: repaint
// the panel if the drive came or went, then reboot if that is what was asked
// for. One sample because usbPresent() shells out to is-usb-present, and the
// watchdog now ticks twice a second - sampling separately for the panel and for
// the reboot decision would be four subprocesses a second for the same answer.
func armedTick(present bool) {
	rebootMutex.Lock()
	changed := present != usbWasPresent
	usbWasPresent = present
	rebootMutex.Unlock()
	if changed {
		updateDisplay()
	}
	checkAutoRebootWith(present)
}

func armReboot() {
	rebootMutex.Lock()
	rebootArmed = true
	// The drive is necessarily still in - the flash just read from it - so
	// start from "present" and let refreshUsbPresence notice it leave.
	usbWasPresent = true
	rebootMutex.Unlock()
}
func disarmReboot() { rebootMutex.Lock(); rebootArmed = false; rebootMutex.Unlock() }
func isRebootArmed() bool {
	rebootMutex.Lock()
	defer rebootMutex.Unlock()
	return rebootArmed
}

func usbPresent() bool {
	return runCommandReturnBool("is-usb-present")
}

// checkAutoReboot reboots the board when a flash has finished, the user opted
// into "reboot when done", and the USB drive has been removed. It fires at most
// once per flash (disarmed immediately before rebooting).
func checkAutoReboot() {
	if !isRebootArmed() {
		return
	}
	checkAutoRebootWith(usbPresent())
}

// checkAutoRebootWith takes the drive's presence rather than probing for it, so
// a caller that has already sampled does not pay for a second subprocess.
func checkAutoRebootWith(present bool) {
	if !isRebootArmed() {
		return
	}

	optionsLock.Lock()
	rebootWhenDone := options.RebootWhenDone
	optionsLock.Unlock()
	if !rebootWhenDone {
		return
	}

	// Only reboot from a resting state. A flash finishes as FINISHED, but
	// getProgress flips that to IDLE on the first poll, so accept both; any
	// active operation (MAGIC/INSTALLING/...) must block the reboot. The arm
	// flag (set on flash success, cleared on every operation start) is the real
	// guard against rebooting after a non-flash op.
	state.Lock()
	resting := state.State == FINISHED || state.State == IDLE
	state.Unlock()
	if !resting {
		return
	}

	if present {
		return // wait for the user to remove the USB drive
	}

	disarmReboot()
	logInfo("Flash finished and USB removed; rebooting into the new image")
	runCommand2("reboot-board")
}

// handleSerialCommand parses one line of the control protocol and returns the
// response line(s) to write back. It is transport-agnostic - see the transports
// below - and drives the same flash state machine the HTTP API uses, so there
// is no duplicate flashing logic.
//
//	LIST          -> "IMG <name> <bytes>" per local image, then "OK"
//	STATUS        -> "STATE <state> PROGRESS <pct>"
//	FLASH <file>  -> starts a file install; "OK flashing <file>" or "ERR ..."
//	CANCEL        -> cancels an in-progress flash; "OK"
//
// startInstall launches a file install in the background. It's a package var so
// tests can stub it without spawning the real flashing goroutine.
var startInstall = func(filename string) { go goInstall(filename) }

// The control protocol is served on two transports, both feeding the same
// dispatcher:
//
//	/dev/ttyGS1  - the second ACM gadget function, which flasher-pi drives as
//	               /dev/ttyACM1 without needing the network. The first ACM
//	               function is the login getty: a getty and the protocol cannot
//	               share a tty, so they get one each rather than the protocol
//	               taking the only USB console.
//	a unix socket - for anything running on the board itself, in particular
//	               `reflash-ctl` from that login.
//
// serveSerialControl runs the protocol over the ACM gadget tty. The tty only
// exists once the gadget has bound, and goes away when the host disconnects, so
// we (re)open it in a retry loop.
func serveSerialControl(devPath string) {
	for {
		// O_NOCTTY matters here. This process is a systemd service and so a
		// session leader, and opening a tty without it makes that tty the
		// process's controlling terminal. The gadget serial then hangs up
		// whenever USB is disturbed - replugging any device on the board is
		// enough, it does not take unplugging the OTG cable - and the kernel
		// sends SIGHUP to the session leader, which Go terminates on by
		// default. The whole server died, mid-flash if you were unlucky,
		// leaving a partially written eMMC. The reopen below was already the
		// right recovery; it was just never reached. See issue #113.
		f, err := os.OpenFile(devPath, os.O_RDWR|syscall.O_NOCTTY, 0)
		if err != nil {
			time.Sleep(2 * time.Second) // wait for the gadget tty to appear
			continue
		}
		logInfo("USB control channel open on " + devPath)
		// CRLF: this end is a serial line, and flasher-pi reads it as one.
		serveControlConn(f, "\r\n")
		f.Close() // EOF / host disconnected - reopen.
		time.Sleep(1 * time.Second)
	}
}

// controlSocketPath is where the local transport listens. `reflash-ctl` uses
// it, so a user logged in over USB can drive a flash without taking the tty
// flasher-pi is talking on. REFLASH_CONTROL_SOCKET overrides it for tests.
func controlSocketPath() string {
	if p := os.Getenv("REFLASH_CONTROL_SOCKET"); p != "" {
		return p
	}
	return "/run/reflash/control.sock"
}

// serveControlSocket accepts control-protocol clients on a unix socket. Each
// connection is handled concurrently and drives the same state machine the
// HTTP API uses, so there is still no duplicate flashing logic.
func serveControlSocket(path string) {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		logError("Could not create control socket directory: " + err.Error())
		return
	}
	// A unix socket is a filesystem entry that outlives the process that bound
	// it, so a previous run (or a crash) leaves one behind that Listen would
	// refuse with EADDRINUSE.
	os.Remove(path)

	l, err := net.Listen("unix", path)
	if err != nil {
		logError("Could not listen on control socket " + path + ": " + err.Error())
		return
	}
	// World-accessible on purpose: this exposes exactly what the HTTP API on
	// port 80 already serves unauthenticated to the whole LAN, so restricting
	// the local socket would buy nothing while breaking `reflash-ctl` for the
	// non-root login it exists to serve.
	if err := os.Chmod(path, 0o666); err != nil {
		logError("Could not chmod control socket: " + err.Error())
	}
	logInfo("Control socket listening on " + path)

	for {
		conn, err := l.Accept()
		if err != nil {
			logError("Control socket accept failed: " + err.Error())
			time.Sleep(time.Second)
			continue
		}
		go func() {
			defer conn.Close()
			serveControlConn(conn, "\n")
		}()
	}
}

// serveControlConn runs the line protocol over one connection: a command per
// line in, its response lines out, terminated by eol. On the socket the client
// signals it is done by closing its write side, which ends the scan and closes
// the connection - that EOF is what frames a one-shot response, since the
// protocol itself has no framing.
func serveControlConn(rw io.ReadWriter, eol string) {
	scanner := bufio.NewScanner(rw)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		for _, resp := range handleSerialCommand(line) {
			fmt.Fprint(rw, resp+eol)
		}
	}
}

// runControlClient is the `reflash ctl` side: with arguments it sends one
// command and prints the reply, without them it relays stdin/stdout so the
// protocol can be driven interactively from a ttyGS0 login.
func runControlClient(args []string) int {
	return runControlClientIO(os.Stdin, os.Stdout, args)
}

// The streams are parameters so tests do not have to swap the os.Stdout global
// out from under the server's logging goroutines.
func runControlClientIO(in io.Reader, out io.Writer, args []string) int {
	path := controlSocketPath()
	conn, err := net.Dial("unix", path)
	if err != nil {
		fmt.Fprintln(os.Stderr, "Could not reach the Reflash server on "+path+": "+err.Error())
		return 1
	}
	defer conn.Close()

	// Half-closing tells the server no more commands are coming, so it finishes
	// the response and closes - which is the EOF that terminates the copy below.
	halfClose := func() {
		if c, ok := conn.(*net.UnixConn); ok {
			c.CloseWrite()
		}
	}

	if len(args) > 0 {
		fmt.Fprintf(conn, "%s\n", strings.Join(args, " "))
		halfClose()
	} else {
		go func() {
			io.Copy(conn, in)
			halfClose()
		}()
	}

	if _, err := io.Copy(out, conn); err != nil {
		fmt.Fprintln(os.Stderr, "Control connection failed: "+err.Error())
		return 1
	}
	return 0
}

func handleSerialCommand(line string) []string {
	fields := strings.Fields(line)
	if len(fields) == 0 {
		return nil
	}

	switch strings.ToUpper(fields[0]) {
	case "LIST":
		out := []string{}
		for _, img := range getLocalImages() {
			out = append(out, fmt.Sprintf("IMG %s %d", img.Name, img.Size))
		}
		return append(out, "OK")

	case "STATUS":
		// Refresh the same way the HTTP getProgress handler does so the
		// embedded screen keeps updating when only the USB protocol is in use.
		refreshProgress()
		state.Lock()
		s, p := state.State, state.Progress
		state.Unlock()
		return []string{fmt.Sprintf("STATE %s PROGRESS %d", s, int(p))}

	case "FLASH":
		if len(fields) < 2 {
			return []string{"ERR missing filename"}
		}
		filename := fields[1]
		if _, err := os.Stat(images_folder + "/" + filename); err != nil {
			return []string{"ERR no such image: " + filename}
		}
		state.Lock()
		busy := state.State != IDLE && state.State != FINISHED &&
			state.State != ERROR && state.State != CANCELLED
		if busy {
			s := state.State
			state.Unlock()
			return []string{"ERR busy: " + s}
		}
		state.Filename = filename
		state.BytesTotal = getUncompressedSize(images_folder + "/" + filename)
		state.State = INSTALLING
		state.Unlock()
		startInstall(filename)
		return []string{"OK flashing " + filename}

	case "CANCEL":
		runCommand2("pkill", "-f", "xz", "-9")
		return []string{"OK"}

	default:
		return []string{"ERR unknown command: " + fields[0]}
	}
}
