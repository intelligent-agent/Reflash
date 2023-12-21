//go:build ignore

package main

import (
	"bytes"
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
	"time"

	"github.com/grafana/tail"
	"github.com/pelletier/go-toml/v2"
)

type Image struct {
	Name string `json:"name"`
	Size int64  `json:"size"`
	Id   int    `json:"id"`
}

type GetInfo struct {
	LocalImages    []Image `json:"local_images"`
	ReflashVersion string  `json:"reflash_version"`
	RecoreRevision string  `json:"recore_revision"`
	EmmcVersion    string  `json:"emmc_version"`
	IsSshEnabled   bool    `json:"is_ssh_enabled"`
	BytesAvailable int     `json:"bytes_available"`
}

type Options struct {
	Darkmode       bool `json:"darkmode"`
	RebootWhenDone bool `json:"rebootWhenDone"`
	EnableSsh      bool `json:"enableSsh"`
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
	RestartApp bool   `json:"restart_app"`
	Rotation   int    `json:"rotation"`
	Where      string `json:"where"`
}

type State struct {
	State      string  `json:"state"`
	Filename   string  `json:"filename"`
	StartTime  int64   `json:"start_time"`
	Progress   float64 `json:"progress"`
	Bandwidth  float32 `json:"bandwidth"`
	BytesNow   int     `json:"bytes_now"`
	BytesTotal int     `json:"bytes_total"`
	Error      string  `json:"error"`
}

type Chunk struct {
	Encoded string `json:"chunk"`
}

const (
	IDLE        = "IDLE"
	DOWNLOADING = "DOWNLOADING"
	UPLOADING   = "UPLOADING"
	INSTALLING  = "INSTALLING"
	BACKUPING   = "BACKUPING"
	FINISHED    = "FINISHED"
	CANCELLED   = "CANCELLED"
	ERROR       = "ERROR"
)

const (
	MODE_RO = "ro"
	MODE_RW = "rw"
)

var options *Options
var state *State

var static_dir string
var version_file string
var images_folder string
var options_file string
var log_file string
var http_port string

var last_size_check time.Time
var bytes_last int
var time_start time.Time

func main() {
	env := os.Getenv("APP_ENV")
	if env == "dev" {
		static_dir = "../client/dist"
		version_file = "../.tmp/etc/reflash.version"
		images_folder = "../.tmp/opt/reflash/images"
		options_file = "../.tmp/opt/options.cfg"
		log_file = "/var/log/reflash.log"
		http_port = ":8080"
	} else {
		static_dir = "/var/www/html/reflash/dist"
		version_file = "/etc/reflash.version"
		images_folder = "/mnt/usb/images"
		options_file = "/mnt/usb/options.cfg"
		log_file = "/var/log/reflash.log"
		http_port = ":80"
	}

	state = &State{
		State: IDLE,
	}

	log_info("-- Server started at " + time.Now().Format("15:04:05") + " --")
	expandUsb()
	mountUsb(MODE_RO)
	loadOptions()

	fs := http.FileServer(http.Dir(static_dir))
	fmt.Println("Starting Reflash go server v0.2.0 env '" + env + "'")
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
	http.HandleFunc("/api/is_usb_present", isUsbPresent)
	http.HandleFunc("/api/start_backup", startBackup)
	http.HandleFunc("/api/cancel_backup", cancelBackup)
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
	}
	json.NewEncoder(w).Encode(get_info)
}

func getOptions(w http.ResponseWriter, r *http.Request) {
	json.NewEncoder(w).Encode(options)
}

func setOptions(w http.ResponseWriter, r *http.Request) {
	reqBody, _ := io.ReadAll(r.Body)
	json.Unmarshal(reqBody, &options)
	saveOptions()
	json.NewEncoder(w).Encode(options)
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
	go goDownload(state.Filename, url)

	sendResponse(w, nil)
}

func goDownload(filename string, url string) {
	out, err := os.Create(images_folder + "/" + filename)
	if err != nil {
		panic(err)
	}

	resp, err := http.Get(url)
	if err != nil {
		panic(err)
	}

	time_start = time.Now()
	log_info(fmt.Sprintf("Starting download at %s", time_start.Format("15:04:05")))

	bytes_now, err := io.Copy(out, resp.Body)
	state.BytesNow = int(bytes_now)
	resp.Body.Close()
	out.Close()

	duration := time.Since(time_start)
	log_info(fmt.Sprintf("Download finished in %d minutes and %d seconds", int(duration.Minutes()), int(duration.Seconds())%60))
	mountUsb(MODE_RO)

	state.State = FINISHED
}

func cancelDownload(w http.ResponseWriter, r *http.Request) {
	state.State = CANCELLED
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

	time_start := time.Now()
	log_info("Starting upload at " + time_start.Format("15:04:05"))
	log_info("Filename: " + state.Filename)
	os.Create(images_folder + "/" + state.Filename)

	sendResponse(w, nil)
}

func uploadChunk(w http.ResponseWriter, r *http.Request) {
	var chunk *Chunk = &Chunk{}
	reqBody, _ := io.ReadAll(r.Body)
	json.Unmarshal(reqBody, &chunk)

	decoded, err := base64.StdEncoding.DecodeString(chunk.Encoded[37:len(chunk.Encoded)])

	path := images_folder + "/" + state.Filename

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
	duration := time.Since(time_start)
	log_info(fmt.Sprintf("Upload finished in %d minutes and %d seconds", int(duration.Minutes()), int(duration.Seconds())%60))
	state.State = FINISHED
}

func uploadCancel(w http.ResponseWriter, r *http.Request) {
	mountUsb(MODE_RO)
	duration := time.Since(time_start)
	log_info(fmt.Sprintf("Upload cancelled after %d minutes and %d seconds", int(duration.Minutes()), int(duration.Seconds())%60))
	state.State = CANCELLED
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

	time_start = time.Now()
	log_info(fmt.Sprintf("starting backup of %s at time %s", state.Filename, time_start.Format("15:04:05")))

	stdout, _, err := runCommand2("/usr/local/bin/backup-emmc", path)
	if err != nil {
		log_error("Error encountered during backup: \n" + stdout)
		mountUsb(MODE_RO)
		state.State = ERROR
		return
	}

	duration := time.Since(time_start)
	log_info(fmt.Sprintf("Backup finished in %d minutes and %d seconds", int(duration.Minutes()), int(duration.Seconds())%60))
	mountUsb(MODE_RO)
	state.State = FINISHED
}

func cancelBackup(w http.ResponseWriter, r *http.Request) {
	duration := time.Since(time_start)
	log_info(fmt.Sprintf("Backup cancelled after %d minutes and %d seconds", int(duration.Minutes()), int(duration.Seconds())%60))

	cmd := exec.Command("pkill", "-f", "xz", "-9")
	err := cmd.Run()
	if err != nil {
		if exitError, ok := err.(*exec.ExitError); ok {
			log_error(fmt.Sprintf("Command 'pkill -f xz -9' returned exit code %v\n", exitError.ExitCode()))
		}
	}

	mountUsb(MODE_RO)
	state.State = CANCELLED
	sendResponse(w, err)
}

func getBlockSize(file string) int {
	return runCommandReturnInt("lsblk", "-n", "-d", "-o", "SIZE", "--bytes", file)
}

func getProgress(w http.ResponseWriter, r *http.Request) {
	if state.State == INSTALLING || state.State == BACKUPING {
		bytes := lastLine("/tmp/recore-flash-progress")
		i, err := strconv.Atoi(bytes)
		if err != nil {
			i = 0
		}
		state.BytesNow = i
		state.Progress = (float64(state.BytesNow) / float64(state.BytesTotal)) * 100.0
	}
	if state.State == DOWNLOADING {
		fi, err := os.Stat(images_folder + "/" + state.Filename)
		if err == nil {
			state.BytesNow = int(fi.Size())
			state.Progress = (float64(state.BytesNow) / float64(state.BytesTotal)) * 100.0
		}
	}

	elapsed := time.Now().Sub(last_size_check).Seconds()
	last_size_check = time.Now()
	bytes_diff_mb := float32(state.BytesNow-bytes_last) / (1024 * 1024)
	bytes_last = state.BytesNow
	state.Bandwidth = bytes_diff_mb / float32(elapsed)

	json.NewEncoder(w).Encode(state)
	if state.State == FINISHED {
		state.State = IDLE
	}
	if state.State == CANCELLED {
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
	response := map[string]bool{"is_file_ok": true}
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
	time.Sleep(2 * time.Second)

	sendResponse(w, nil)
}

func goInstall(filename string) {
	path := images_folder + "/" + filename

	time_start = time.Now()
	log_info(fmt.Sprintf("starting install of %s at time %s", filename, time_start.Format("15:04:05")))

	stdout, _, err := runCommand2("/usr/local/bin/flash-recore", path)
	if err != nil {
		log_error("Error encountered during install: \n" + stdout)
		state.State = ERROR
		return
	}

	duration := time.Since(time_start)
	log_info(fmt.Sprintf("Installation finished in %d minutes and %d seconds", int(duration.Minutes()), int(duration.Seconds())%60))

	state.State = FINISHED
}

func getUncompressedSize(path string) int {
	cmd := exec.Command("xz", "-l", path)
	stdout, err := cmd.Output()
	if err != nil {
		if exitError, ok := err.(*exec.ExitError); ok {
			log_error(fmt.Sprintf("Command 'xz -l %s' returned exit code %v\n", path, exitError.ExitCode()))
			return 1
		}
	}
	strs := strings.Split(strings.ReplaceAll(string(stdout[:]), " ", ""), "MiB")
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
			log_error(fmt.Sprintf("Command 'pkill -f xz -9' returned exit code %v\n", exitError.ExitCode()))
		}
	}
	sendResponse(w, err)
}

func runInstallFinishedCommands(w http.ResponseWriter, r *http.Request) {
	var err error
	err = cmdRotateScreen(options.ScreenRotation, "CMDLINE", false)
	if err != nil {
		sendResponse(w, err)
	}
	err = cmdRotateScreen(options.ScreenRotation, "XORG", false)
	if err != nil {
		sendResponse(w, err)
	}
	err = cmdRotateScreen(options.ScreenRotation, "WESTON", false)
	if err != nil {
		sendResponse(w, err)
	}
	if options.EnableSsh {
		err = setSshEnabled(true)
		if err != nil {
			sendResponse(w, err)
		}
	}
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

func runCommand(cmd_str string) int {
	cmd := exec.Command(cmd_str)
	if err := cmd.Run(); err != nil {
		if exitError, ok := err.(*exec.ExitError); ok {
			log_error(fmt.Sprintf("Command '%s' returned exit code %v\n", cmd_str, exitError.ExitCode()))
			return exitError.ExitCode()
		}
	}
	return 0
}

func runCommandReturnBool(cmd_str string) bool {
	cmd := exec.Command(cmd_str)
	stdout, err := cmd.Output()
	if err != nil {
		if exitError, ok := err.(*exec.ExitError); ok {
			log_error(fmt.Sprintf("Command '%s' returned exit code %v\n", cmd_str, exitError.ExitCode()))
			return false
		}
	}
	fmt.Println(string(stdout[:]))
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
	return stdout
}

func rebootBoard(w http.ResponseWriter, r *http.Request) {
	_, _, err := runCommand2("/usr/local/bin/reboot-board")
	sendResponse(w, err)
}

func rotateScreen(w http.ResponseWriter, r *http.Request) {
	var data *RotateCommand = &RotateCommand{}
	reqBody, _ := io.ReadAll(r.Body)
	json.Unmarshal(reqBody, &data)

	err := cmdRotateScreen(data.Rotation, data.Where, data.RestartApp)
	sendResponse(w, err)
}

func cmdRotateScreen(rotation int, place string, restart bool) error {
	cmd := exec.Command("/usr/local/bin/rotate-screen", strconv.Itoa(rotation), place, strings.ToUpper(strconv.FormatBool(restart)))
	var err error
	if err = cmd.Run(); err != nil {
		if exitError, ok := err.(*exec.ExitError); ok {
			log_error(fmt.Sprintf("Command '%s' returned exit code %v\n", "rotate-screen", exitError.ExitCode()))
		}
	}
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

func log_info(msg string) {
	log_msg("[info]  " + msg)
}
func log_error(msg string) {
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
		log_info("expand-usb returned error")
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
		log_error(fmt.Sprintf("%s", cmds) + ": " + fmt.Sprint(err) + ": " + stderr.String())
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

func getRecoreRevision() string {
	return runCommandReturnString("get-recore-revision")
}

func saveOptions() error {
	var err error
	mountUsb(MODE_RW)
	content, err := toml.Marshal(options)
	err = os.WriteFile(options_file, content, 0644)
	mountUsb(MODE_RO)
	return err
}

func loadOptions() {
	content, err := os.ReadFile(options_file)
	if err != nil {
		log_info("No options file found, creating default")
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
