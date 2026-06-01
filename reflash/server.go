package main

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
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

type GetInfo struct {
	LocalImages    []Image  `json:"local_images"`
	ReflashVersion string   `json:"reflash_version"`
	RecoreRevision string   `json:"recore_revision"`
	SerialNumber   string   `json:"serial_number"`
	EmmcVersion    string   `json:"emmc_version"`
	IsSshEnabled   bool     `json:"is_ssh_enabled"`
	BytesAvailable int      `json:"bytes_available"`
	IPs            []string `json:"ips"`
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

type Chunk struct {
	Encoded string `json:"chunk"`
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
	SAVING			= "SAVING"
)

const (
	MODE_RO = "ro"
	MODE_RW = "rw"
)

var options *Options
var state *State

var oldState *State
var oldRotation int

var static_dir string
var binDir string
var images_folder string
var options_file string
var log_file string
var http_port string

var last_size_check time.Time
var bytes_last int
var timeStart time.Time
var cancelFunc context.CancelFunc
var isDirty bool
var env string
var optionsLock sync.Mutex
var (
    cachedAccessPoints []AccessPoint
    isScanning         bool
    scanMutex          sync.Mutex
)
var (
    isConnecting   bool
    connectError   error
    connectMutex   sync.Mutex
)
func ServerInit() {
	env = os.Getenv("APP_ENV")
	if env == "dev" {
		static_dir = "../client/dist"
		binDir = "../bin/dev"
		images_folder = "/opt/reflash/images"
		options_file = "../.tmp/opt/options.cfg"
		log_file = "/var/log/reflash.log"
		http_port = ":8080"
	} else {
		static_dir = "/var/www/html/reflash/dist"
		binDir = "/usr/local/bin"
		images_folder = "/mnt/usb/images"
		options_file = "/mnt/usb/options.cfg"
		log_file = "/var/log/reflash.log"
		http_port = ":80"
	}
	// Allow tests (and ad-hoc runs) to point the helper scripts elsewhere.
	if d := os.Getenv("REFLASH_BIN_DIR"); d != "" {
		binDir = d
	}

	state = &State{
		State:      IDLE,
		BytesTotal: 1,
		IPs:        getIPs(),
	}

	oldState = &State{
		State:      IDLE,
		BytesTotal: 1,
	}

	logInfo("-- Server started at " + time.Now().Format("15:04:05") + " --")
	expandUsb()
	mountUsb(MODE_RO)
	loadOptions()
	bringupWifi()
	updateDisplay()
	startWatchdog()

	timer1 := time.NewTimer(3 * time.Second)
	go func() {
		<-timer1.C
		logInfo("Updating IPs")
		state.IPs = getIPs()
		updateDisplay()
	}()

	version := runCommandReturnString("get-reflash-version")

	fs := http.FileServer(http.Dir(static_dir))
	fmt.Println("Starting Reflash go server " + version + " env '" + env + "'")
	http.Handle("/", fs)
	http.HandleFunc("/api/get_info", getInfo)
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
	http.HandleFunc("/api/get_wifi_status", getWifiStatus)
	http.HandleFunc("/api/wifi_start_scan", startScanWifi)
	http.HandleFunc("/api/wifi_poll_scan", getWifiScanResults)
	http.HandleFunc("/api/wifi_start_connect", startConnectWifi)
	http.HandleFunc("/api/wifi_poll_connect", pollConnectWifi)
	
	log.Fatal(http.ListenAndServe(http_port, nil))
}

func getInfo(w http.ResponseWriter, r *http.Request) {
	var get_info *GetInfo = &GetInfo{
		LocalImages:    getLocalImages(),
		ReflashVersion: runCommandReturnString("get-reflash-version"),
		RecoreRevision: runCommandReturnString("get-recore-revision"),
		SerialNumber:   runCommandReturnString("get-recore-serial-number"),
		EmmcVersion:    runCommandReturnString("get-emmc-version"),
		IsSshEnabled:   runCommandReturnBool("is-ssh-enabled"),
		BytesAvailable: getFreeSpace(),
		IPs:            getIPs(),
	}
	json.NewEncoder(w).Encode(get_info)
}

func getSerialNumber(w http.ResponseWriter, r *http.Request) {
	var get_serial_number *GetSerialNumber = &GetSerialNumber{
		SerialNumber: runCommandReturnString("get-recore-serial-number"),
	}
	json.NewEncoder(w).Encode(get_serial_number)
}

func getWifi(w http.ResponseWriter, r *http.Request) {

	ssid, _, _ := runCommand2("get-setting", "WIFI_SSID")

	var get_wifi *GetWifi = &GetWifi{
		SSID: strings.TrimSpace(ssid),
	}
	json.NewEncoder(w).Encode(get_wifi)
}

func bringupWifi(){
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

func getWifiStatus(w http.ResponseWriter, r *http.Request) {
    result := runCommandReturnString("wifi-present")
    w.Header().Set("Content-Type", "application/json")
    w.Write([]byte(result))
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
	state.Lock()
	if oldState.State != state.State || oldState.Progress != state.Progress || oldRotation != options.ScreenRotation || !slices.Equal(oldState.IPs, state.IPs) {
		Draw(float32(state.Progress)/100, state.State, options.ScreenRotation, state.IPs)
		oldState.State = state.State
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
	for line := range t.Lines {
		fmt.Fprint(w, fmt.Sprintf("data: %s\n\n", line.Text))
		flusher.Flush()
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
	mountUsb(MODE_RW)

	timeStart = time.Now()
	logInfo("Starting upload at " + timeStart.Format("15:04:05"))
	logInfo("Filename: " + state.Filename)
	os.Create(images_folder + "/" + state.Filename)

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

	go goUploadMagic()
	time.Sleep(1 * time.Second)

	sendResponse(w, nil)
}

func goUploadMagic() {
	timeStart = time.Now()
	logInfo("Starting magic upload at " + timeStart.Format("15:04:05"))
	logInfo("Filename: " + state.Filename)

	stdout, _, err := runCommand2("flash-mkfifo")
	if err != nil {
		logError("Error encountered when setting up pipe: \n" + stdout)
	}
	logInfo("flash-mkfifo done")
}

func uploadMagicChunk(w http.ResponseWriter, r *http.Request) {
	var chunk *Chunk = &Chunk{}
	var err error
	reqBody, _ := io.ReadAll(r.Body)
	json.Unmarshal(reqBody, &chunk)

	if state.State == CANCELLED {
		response := map[string]bool{"success": false}
		json.NewEncoder(w).Encode(response)
		return
	} else {
		path := "/tmp/mypipe"
		if state.File == nil {
			logInfo("Open file " + path)
			state.File, err = os.OpenFile(path, os.O_APPEND|os.O_WRONLY, 0644)
			if err != nil {
				log.Fatal(err)
			}
		}
	}

	decoded, err := base64.StdEncoding.DecodeString(chunk.Encoded[37:])
	if err != nil {
		http.Error(w, "Failed to decode base64", http.StatusBadRequest)
		return
	}
	_, err = state.File.Write(decoded)
	if err != nil {
		http.Error(w, "Failed to write decompressed data to file", http.StatusInternalServerError)
		return
	}

	state.BytesNow += len(decoded)
	state.Progress = float64(state.BytesNow) * 100 / float64(state.BytesTotal)

	response := map[string]bool{"success": true}
	json.NewEncoder(w).Encode(response)
}

func uploadMagicFinish(w http.ResponseWriter, r *http.Request) {
	if err := state.File.Close(); err != nil {
		log.Fatal(err)
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
		state.State = FINISHED
	}
}

func uploadChunk(w http.ResponseWriter, r *http.Request) {
	var chunk *Chunk = &Chunk{}
	reqBody, _ := io.ReadAll(r.Body)
	json.Unmarshal(reqBody, &chunk)

	decoded, err := base64.StdEncoding.DecodeString(chunk.Encoded[37:len(chunk.Encoded)])

	path := images_folder + "/" + state.Filename

	if state.State == CANCELLED {
		response := map[string]bool{"success": false}
		json.NewEncoder(w).Encode(response)
		return
	}

	f, err := os.OpenFile(path, os.O_APPEND|os.O_WRONLY, 0644)
	if err != nil {
		log.Fatal(err)
	}
	if _, err := f.Write(decoded); err != nil {
		log.Fatal(err)
	}
	if err := f.Close(); err != nil {
		log.Fatal(err)
	}
	state.BytesNow += len(decoded)
	state.Progress = float64(state.BytesNow) * 100 / float64(state.BytesTotal)

	response := map[string]bool{"success": true}
	json.NewEncoder(w).Encode(response)
}

func uploadFinish(w http.ResponseWriter, r *http.Request) {
	mountUsb(MODE_RO)
	duration := time.Since(timeStart)
	logInfo(fmt.Sprintf("Upload finished in %d minutes and %d seconds", int(duration.Minutes()), int(duration.Seconds())%60))
	state.State = FINISHED
}

func uploadCancel(w http.ResponseWriter, r *http.Request) {
	state.State = CANCELLED
	if state.File != nil {
		logInfo("Closing file")
		if err := state.File.Close(); err != nil {
			log.Fatal(err)
		}
		state.File = nil
	}
	duration := time.Since(timeStart)
	logInfo(fmt.Sprintf("Upload cancelled after %d minutes and %d seconds", int(duration.Minutes()), int(duration.Seconds())%60))
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
	timeStart = time.Now()
	logInfo(fmt.Sprintf("Starting magic at %s", timeStart.Format("15:04:05")))
	logInfo(fmt.Sprintf("Url %s", url))

	stdout, _, err := runCommand2("flash-from-url", url)
	if err != nil {
		logError("Error encountered during magic: \n" + stdout)
		state.State = ERROR
		state.Error = "An error was encountered during magic. Check log for details"
		return
	}

	duration := time.Since(timeStart)
	logInfo(fmt.Sprintf("Magic finished in %d minutes and %d seconds", int(duration.Minutes()), int(duration.Seconds())%60))

	state.State = FINISHED
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
	path := images_folder + "/" + state.Filename

	timeStart = time.Now()
	logInfo(fmt.Sprintf("starting backup of %s at time %s", state.Filename, timeStart.Format("15:04:05")))

	stdout, _, err := runCommand2("backup-emmc", path)
	if err != nil {
		logError("Error encountered during backup: \n" + stdout)
		mountUsb(MODE_RO)
		state.State = ERROR
		state.Error = "An error was encountered during backup. Check log for details"
		return
	}

	duration := time.Since(timeStart)
	logInfo(fmt.Sprintf("Backup finished in %d minutes and %d seconds", int(duration.Minutes()), int(duration.Seconds())%60))
	mountUsb(MODE_RO)
	state.State = FINISHED
}

func cancelBackup(w http.ResponseWriter, r *http.Request) {
	duration := time.Since(timeStart)
	logInfo(fmt.Sprintf("Backup cancelled after %d minutes and %d seconds", int(duration.Minutes()), int(duration.Seconds())%60))

	cmd := exec.Command("pkill", "-f", "xz", "-9")
	err := cmd.Run()
	if err != nil {
		if exitError, ok := err.(*exec.ExitError); ok {
			logError(fmt.Sprintf("Command 'pkill -f xz -9' returned exit code %v\n", exitError.ExitCode()))
		}
	}
	os.Remove(images_folder + "/" + state.Filename)
	state.State = CANCELLED
	sendResponse(w, err)
}

func getBlockSize(file string) int {
	if env == "dev" {
		return 100 * 1024 * 1024
	}
	return runCommandReturnInt("lsblk", "-n", "-d", "-o", "SIZE", "--bytes", file)
}

func getProgress(w http.ResponseWriter, r *http.Request) {
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

	state.Progress = (float64(state.BytesNow) / float64(state.BytesTotal)) * 100.0
	elapsed := time.Now().Sub(last_size_check).Seconds()
	last_size_check = time.Now()
	bytes_diff_mb := float32(state.BytesNow-bytes_last) / (1024 * 1024)
	bytes_last = state.BytesNow
	state.Bandwidth = bytes_diff_mb / float32(elapsed)

	updateDisplay()

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

	state.State = FINISHED
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
		log.Fatal(err)
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

	settings := "# Settings from Reflash\n" +
		"SSH_ENABLED_ON_BOOT=" + strconv.FormatBool(options.EnableSsh) + "\n" +
		"SSH_TIMEOUT=60\n" +
		"EXTERNAL_SCREEN_ROTATION=" + strconv.FormatInt(int64(options.ScreenRotation), 10) + "\n" +
		"WIFI_SSID='" + options.WifiSSID + "'\n" +
		"WIFI_PSK='" + options.WifiPSK+"'"

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
	_, _, err := runCommand2("create-recore-config", strconv.Itoa(data.Snr))
	sendResponse(w, err)
}

func cmdRotateScreen(rotation int, place string) error {
	_, _, err := runCommand2("rotate-screen", strconv.Itoa(rotation), place)
	return err
}

func setSshEnabled(is_enabled bool) error {
	var err error
	cmd := exec.Command(resolveCmd("set-ssh-enabled"), strconv.FormatBool(is_enabled))
	if err := cmd.Run(); err != nil {
		if exitError, ok := err.(*exec.ExitError); ok {
			log.Fatalf("Command returned exit code %v\n", exitError.ExitCode())
		}
	}
	return err
}

func isUsbPresent(w http.ResponseWriter, r *http.Request) {
	result := runCommandReturnBool("is-usb-present")
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

func log_msg(msg string) {
	fmt.Println(msg)
	file, err := os.OpenFile(log_file, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		log.Fatal(err)
	}
	if _, err := file.Write([]byte(msg + "\n")); err != nil {
		log.Fatal(err)
	}
	if err := file.Close(); err != nil {
		log.Fatal(err)
	}
}

func clearLog(w http.ResponseWriter, r *http.Request) {
	file, err := os.OpenFile(log_file, os.O_RDWR, 0644)
	if err != nil {
		log.Fatal(err)
	}
	defer file.Close()
	err = file.Truncate(0)
	if err != nil {
		log.Fatal(err)
	}
	log_msg("--- Log start ---")

	response := map[string]int{"status": 0}
	json.NewEncoder(w).Encode(response)
}

func expandUSB() error {
	cmd := exec.Command(resolveCmd("expand-usb"))
	err := cmd.Run()
	if err == nil {
		logInfo("expand-usb returned error")
	}
	return err
}

func runCommand2(cmds ...string) (string, string, error) {
	cmd := exec.Command(resolveCmd(cmds[0]), cmds[1:]...)
	var out bytes.Buffer
	var stderr bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &stderr
	err := cmd.Run()
	if err != nil {
		logError(fmt.Sprintf("%s", cmds) + ": " + fmt.Sprint(err) + ": " + strings.TrimSpace(stderr.String()))
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

func expandUsb() {
	runCommand2("expand-usb")
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

func startWatchdog() {
    // Check every 2 seconds
    ticker := time.NewTicker(2 * time.Second) 
    
    go func() {
        for range ticker.C {
            // This is the function we built in the previous step
            lockSaveOptions() 
        }
    }()
}