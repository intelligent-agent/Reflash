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
	EmmcVersion    string   `json:"emmc_version"`
	IsSshEnabled   bool     `json:"is_ssh_enabled"`
	BytesAvailable int      `json:"bytes_available"`
	IPs            []string `json:"ips"`
}

type Options struct {
	Darkmode       bool `json:"darkmode"`
	RebootWhenDone bool `json:"rebootWhenDone"`
	EnableSsh      bool `json:"enableSsh"`
	Magicmode      bool `json:"magicmode"`
	ScreenRotation int  `json:"screenRotation"`
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
}

type Chunk struct {
	Encoded string `json:"chunk"`
}

type Settings struct {
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
var images_folder string
var options_file string
var log_file string
var http_port string

var last_size_check time.Time
var bytes_last int
var timeStart time.Time
var cancelFunc context.CancelFunc
var stateMutex sync.Mutex
var saveOptionsWhenIdle bool
var env string

func ServerInit() {
	env = os.Getenv("APP_ENV")
	if env == "dev" {
		static_dir = "../client/dist"
		images_folder = "/opt/reflash/images"
		options_file = "../.tmp/opt/options.cfg"
		log_file = "/var/log/reflash.log"
		http_port = ":8080"
	} else {
		static_dir = "/var/www/html/reflash/dist"
		images_folder = "/mnt/usb/images"
		options_file = "/mnt/usb/options.cfg"
		log_file = "/var/log/reflash.log"
		http_port = ":80"
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
	updateDisplay()

	timer1 := time.NewTimer(10 * time.Second)
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
	log.Fatal(http.ListenAndServe(http_port, nil))
}

func getInfo(w http.ResponseWriter, r *http.Request) {
	var get_info *GetInfo = &GetInfo{
		LocalImages:    getLocalImages(),
		ReflashVersion: runCommandReturnString("get-reflash-version"),
		RecoreRevision: runCommandReturnString("get-recore-revision"),
		EmmcVersion:    runCommandReturnString("get-emmc-version"),
		IsSshEnabled:   runCommandReturnBool("is-ssh-enabled"),
		BytesAvailable: getFreeSpace(),
		IPs:            getIPs(),
	}
	json.NewEncoder(w).Encode(get_info)
}

func getOptions(w http.ResponseWriter, r *http.Request) {
	json.NewEncoder(w).Encode(options)
}

func setOptions(w http.ResponseWriter, r *http.Request) {
	reqBody, _ := io.ReadAll(r.Body)
	json.Unmarshal(reqBody, &options)
	if state.State == IDLE {
		saveOptions()
	} else {
		logInfo("Options not saved because the disk is in use")
		saveOptionsWhenIdle = true
	}
	updateDisplay()
	json.NewEncoder(w).Encode(options)
}

func updateDisplay() {
	stateMutex.Lock()
	if oldState.State != state.State || oldState.Progress != state.Progress || oldRotation != options.ScreenRotation || !slices.Equal(oldState.IPs, state.IPs) {
		Draw(float32(state.Progress)/100, state.State, options.ScreenRotation, state.IPs)
		oldState.State = state.State
		oldState.Progress = state.Progress
		oldRotation = options.ScreenRotation
	}
	stateMutex.Unlock()
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

	stdout, _, err := runCommand2("/usr/local/bin/flash-mkfifo")
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
	stdout, _, err := runCommand2("/usr/local/bin/flash-cleanup", revision)
	if err != nil {
		logError("Error encountered during cleanup: \n" + stdout)
		state.State = ERROR
		state.Error = "An error was encountered during magic. Check log for details"
	} else {
		state.State = FINISHED
	}
	if saveOptionsWhenIdle {
		saveOptions()
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
	if saveOptionsWhenIdle {
		saveOptions()
	}
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

	stdout, _, err := runCommand2("/usr/local/bin/flash-direct", url)
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

	stdout, _, err := runCommand2("/usr/local/bin/backup-emmc", path)
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

	stdout, _, err := runCommand2("/usr/local/bin/flash-recore", path)
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
	cmd := exec.Command("xz", "-l", path)
	stdout, err := cmd.Output()
	if err != nil {
		if exitError, ok := err.(*exec.ExitError); ok {
			logError(fmt.Sprintf("Command 'xz -l %s' returned exit code %v\n", path, exitError.ExitCode()))
			return 1
		}
	}
	trimmed := string(stdout[:])
	trimmed = strings.ReplaceAll(trimmed, " ", "")
	trimmed = strings.ReplaceAll(trimmed, ",", "")
	strs := strings.Split(trimmed, "MiB")
	ret, err := strconv.ParseFloat(strs[1], 32)
	return int(ret * 1024 * 1024)
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
		"EXTERNAL_SCREEN_ROTATION=" + strconv.FormatInt(int64(options.ScreenRotation), 10)

	runCommand2("/usr/local/bin/save-settings", settings)
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

func runCommandReturnBool(cmd_str string) bool {
	cmd := exec.Command(cmd_str)
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
	_, _, err := runCommand2("/usr/local/bin/reboot-board")
	sendResponse(w, err)
}

func shutdownBoard(w http.ResponseWriter, r *http.Request) {
	_, _, err := runCommand2("/usr/local/bin/shutdown-board")
	sendResponse(w, err)
}

func rotateScreen(w http.ResponseWriter, r *http.Request) {
	var data *RotateCommand = &RotateCommand{}
	reqBody, _ := io.ReadAll(r.Body)
	json.Unmarshal(reqBody, &data)

	err := cmdRotateScreen(data.Rotation, data.Where)
	sendResponse(w, err)
}

func cmdRotateScreen(rotation int, place string) error {
	_, _, err := runCommand2("/usr/local/bin/rotate-screen", strconv.Itoa(rotation), place)
	return err
}

func setSshEnabled(is_enabled bool) error {
	var err error
	cmd := exec.Command("/usr/local/bin/set-ssh-enabled", strconv.FormatBool(is_enabled))
	if err := cmd.Run(); err != nil {
		if exitError, ok := err.(*exec.ExitError); ok {
			log.Fatalf("Command returned exit code %v\n", exitError.ExitCode())
		}
	}
	return err
}

func isUsbPresent(w http.ResponseWriter, r *http.Request) {
	result := runCommandReturnBool("/usr/local/bin/is-usb-present")
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
	cmd := exec.Command("expand-usb")
	err := cmd.Run()
	if err == nil {
		logInfo("expand-usb returned error")
	}
	return err
}

func runCommand2(cmds ...string) (string, string, error) {
	cmd := exec.Command(cmds[0], cmds[1:]...)
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
	content, err := toml.Marshal(options)
	err = os.WriteFile(options_file, content, 0644)
	logInfo("Options saved")
	mountUsb(MODE_RO)
	return err
}

func loadOptions() {
	content, err := os.ReadFile(options_file)
	if err != nil {
		logInfo("No options file found, creating default")
		options = &Options{
			Darkmode:       true,
			RebootWhenDone: false,
			EnableSsh:      false,
			ScreenRotation: 0,
		}
		saveOptions()
	} else {
		toml.Unmarshal(content, &options)
	}
}

func expandUsb() {
	runCommand2("/usr/local/bin/expand-usb")
}
