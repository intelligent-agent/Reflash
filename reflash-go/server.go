//go:build ignore

package main

import (
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

	"github.com/grafana/tail"
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
	UsbPresent     bool    `json:"usb_present"`
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

type RotateCommand struct {
	RestartApp bool   `json:"restart_app"`
	Rotation   int    `json:"rotation"`
	Where      string `json:"where"`
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

type State struct {
	State      string  `json:"state"`
	Filename   string  `json:"filename"`
	StartTime  int64   `json:"start_time"`
	Progress   float64 `json:"progress"`
	BytesNow   int     `json:"bytes_now"`
	BytesTotal int     `json:"bytes_total"`
	Error      string  `json:"error"`
}

var options *Options
var state *State

var version_file string
var images_folder string
var db_file string
var static_dir string
var port string
var log_file string

func main() {
	options = &Options{
		Darkmode:       true,
		RebootWhenDone: false,
		EnableSsh:      false,
		ScreenRotation: 0,
	}

	state = &State{
		State: IDLE,
	}

	env := os.Getenv("APP_ENV")
	if env == "dev" {
		static_dir = "../client/dist"
		version_file = "../.tmp/etc/reflash.version"
		images_folder = "../.tmp/opt/reflash/images"
		db_file = "../.tmp/opt/reflash/reflash.db"
		log_file = "/var/log/reflash.log"
		port = ":8080"
	} else {
		static_dir = "/var/www/html/reflash/dist"
		version_file = "/etc/reflash.version"
		images_folder = "/mnt/usb/images"
		db_file = "/opt/reflash/reflash.db"
		log_file = "/var/log/reflash.log"
		port = ":80"
	}
	fs := http.FileServer(http.Dir(static_dir))
	fmt.Println("Starting Reflash go server v0.2.0 env '" + env + "'")
	http.Handle("/", fs)
	http.HandleFunc("/api/get_info", getInfo)
	http.HandleFunc("/api/stream_log", streamLog)
	http.HandleFunc("/api/options", getOptions)
	http.HandleFunc("/api/save_options", setOptions)
	http.HandleFunc("/api/download_refactor", downloadRefactor)
	http.HandleFunc("/api/get_progress", getProgress)
	http.HandleFunc("/api/check_file_integrity", checkFileIntegrity)
	http.HandleFunc("/api/install_refactor", installRefactor)
	http.HandleFunc("/api/run_install_finished_commands", runInstallFinishedCommands)
	http.HandleFunc("/api/reboot_board", rebootBoard)
	http.HandleFunc("/api/is_usb_present", isUsbPresent)
	http.HandleFunc("/api/upload_start", uploadStart)
	http.HandleFunc("/api/upload_finish", uploadFinish)
	http.HandleFunc("/api/upload_cancel", uploadCancel)
	http.HandleFunc("/api/upload_chunk", uploadChunk)
	http.HandleFunc("/api/clear_log", clearLog)
	http.HandleFunc("/api/rotate_screen", rotateScreen)
	log.Fatal(http.ListenAndServe(port, nil))
}

func getInfo(w http.ResponseWriter, r *http.Request) {
	mountUsb()
	var get_info *GetInfo = &GetInfo{
		LocalImages:    getLocalImages(),
		ReflashVersion: "Dummy 0.2.0",
		RecoreRevision: "A7",
		EmmcVersion:    "Fluidd v3.2.0",
		IsSshEnabled:   false,
		BytesAvailable: getFreeSpace(),
	}
	unmountUsb()
	json.NewEncoder(w).Encode(get_info)
}

func getOptions(w http.ResponseWriter, r *http.Request) {
	json.NewEncoder(w).Encode(options)
}

func setOptions(w http.ResponseWriter, r *http.Request) {
	reqBody, _ := io.ReadAll(r.Body)
	json.Unmarshal(reqBody, &options)
	json.NewEncoder(w).Encode(options)
}

func streamLog(w http.ResponseWriter, r *http.Request) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "SSE not supported", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/event-stream")
	t, err := tail.TailFile("/var/log/reflash.log", tail.Config{Follow: true})
	if err != nil {
		panic(err)
	}
	for line := range t.Lines {
		fmt.Fprint(w, fmt.Sprintf("data: %s\n\n", line.Text))
		flusher.Flush()
	}
}

func downloadRefactor(w http.ResponseWriter, r *http.Request) {
	var data *Download = &Download{}
	reqBody, _ := io.ReadAll(r.Body)
	json.Unmarshal(reqBody, &data)

	res := mountUsb()
	if res == 0 {
		state.Filename = data.Filename
		state.StartTime = data.StartTime
		url := data.Url
		state.BytesTotal = data.Size
		state.State = DOWNLOADING

		go _go_downloadRefactor(state.Filename, url)
	}

	response := map[string]int{"status": res}
	json.NewEncoder(w).Encode(response)
}

func _go_downloadRefactor(filename string, url string) {
	out, err := os.Create(images_folder + "/" + filename)
	if err != nil {
		panic(err)
	}
	defer out.Close()

	resp, err := http.Get(url)
	if err != nil {
		panic(err)
	}
	defer resp.Body.Close()

	log_info("Starting download")
	bytes_now, err := io.Copy(out, resp.Body)
	state.BytesNow = int(bytes_now)
	log_info("Download finished")

	unmountUsb()

	state.State = FINISHED
}

func getProgress(w http.ResponseWriter, r *http.Request) {
	if state.State == INSTALLING {
		progress := lastLine("/tmp/recore-flash-progress")
		i, err := strconv.ParseFloat(progress, 64)
		if err != nil {
			i = 0
		}
		state.Progress = i
	}
	if state.State == DOWNLOADING {
		fi, err := os.Stat(images_folder + "/" + state.Filename)
		if err == nil {
			state.BytesNow = int(fi.Size())
			state.Progress = (float64(state.BytesNow) / float64(state.BytesTotal)) * 100.0
		}
	}

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
	state.State = INSTALLING
	mountUsb()
	go _go_installRefactor(state.Filename)

	response := map[string]int{"status": 0}
	json.NewEncoder(w).Encode(response)
}

func _go_installRefactor(filename string) {
	path := images_folder + "/" + filename
	log_info("starting install of " + filename)

	cmd := exec.Command("/usr/local/bin/flash-recore", path)
	if err := cmd.Run(); err != nil {
		log_error("Error encountered during install: " + err.Error())
	}
	log_info("Installation done")

	state.State = FINISHED
}

func lastLine(file string) string {
	out, err := exec.Command("tail", "-n1", file).Output()
	if err != nil {
		log.Fatal(err)
	}
	return strings.TrimSpace(string(out[:]))
}

func runInstallFinishedCommands(w http.ResponseWriter, r *http.Request) {
	var ret int
	ret += cmdRotateScreen(options.ScreenRotation, "CMDLINE", false)
	ret += cmdRotateScreen(options.ScreenRotation, "XORG", false)
	ret += cmdRotateScreen(options.ScreenRotation, "WESTON", false)

	if options.EnableSsh {
		ret += setSshEnabled(true)
	}

	ret += unmountUsb()

	response := map[string]int{"status": ret}
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

func runBinaryCommand(cmd_str string) (bool, int) {
	cmd := exec.Command(cmd_str)
	stdout, err := cmd.Output()
	if err != nil {
		if exitError, ok := err.(*exec.ExitError); ok {
			log_error(fmt.Sprintf("Command '%s' returned exit code %v\n", cmd_str, exitError.ExitCode()))
			return false, exitError.ExitCode()
		}
	}
	fmt.Println(string(stdout[:]))
	ret, err := strconv.ParseBool(strings.TrimSpace(string(stdout[:])))
	return ret, 0
}

func runCommandReturnInt(cmd_str string) int {
	cmd := exec.Command(cmd_str)
	stdout, err := cmd.Output()
	if err != nil {
		if exitError, ok := err.(*exec.ExitError); ok {
			log_error(fmt.Sprintf("Command '%s' returned exit code %v\n", cmd_str, exitError.ExitCode()))
			return 0
		}
	}
	ret, err := strconv.Atoi(strings.TrimSpace(string(stdout[:])))
	return int(ret)
}

func rebootBoard(w http.ResponseWriter, r *http.Request) {
	ret := runCommand("/usr/local/bin/reboot-board")
	response := map[string]int{"status": ret}
	json.NewEncoder(w).Encode(response)
}

func rotateScreen(w http.ResponseWriter, r *http.Request) {
	var data *RotateCommand = &RotateCommand{}
	reqBody, _ := io.ReadAll(r.Body)
	json.Unmarshal(reqBody, &data)

	ret := cmdRotateScreen(data.Rotation, data.Where, data.RestartApp)
	response := map[string]int{"status": ret}
	json.NewEncoder(w).Encode(response)
}

func cmdRotateScreen(rotation int, place string, restart bool) int {
	fmt.Println("/usr/local/bin/rotate-screen", rotation, place, restart)
	cmd := exec.Command("/usr/local/bin/rotate-screen", strconv.Itoa(rotation), place, strings.ToUpper(strconv.FormatBool(restart)))
	if err := cmd.Run(); err != nil {
		if exitError, ok := err.(*exec.ExitError); ok {
			log_error(fmt.Sprintf("Command '%s' returned exit code %v\n", "rotate-screen", exitError.ExitCode()))
			return exitError.ExitCode()
		}
	}
	return 0
}

func setSshEnabled(is_enabled bool) int {
	cmd := exec.Command("/usr/local/bin/set-ssh-enabled", strconv.FormatBool(is_enabled))
	if err := cmd.Run(); err != nil {
		if exitError, ok := err.(*exec.ExitError); ok {
			log.Fatalf("Command returned exit code %v\n", exitError.ExitCode())
			return exitError.ExitCode()
		}
	}
	return 0
}

func isUsbPresent(w http.ResponseWriter, r *http.Request) {
	result, exitCode := runBinaryCommand("/usr/local/bin/is-usb-present")
	var response *BinaryCommandResult = &BinaryCommandResult{
		ExitCode: exitCode,
		Result:   result,
	}
	json.NewEncoder(w).Encode(response)
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
	mountUsb()
	log_info("Starting upload of " + state.Filename)
	os.Create(images_folder + "/" + state.Filename)

	response := map[string]int{"status": 0}
	json.NewEncoder(w).Encode(response)
}

type Chunk struct {
	Encoded string `json:"chunk"`
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
	unmountUsb()
	state.State = FINISHED
}

func uploadCancel(w http.ResponseWriter, r *http.Request) {
	unmountUsb()
	state.State = CANCELLED
}

func log_info(msg string) {
	log_msg("[info] " + msg)
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

func check_err(err error) {
	if err != nil {
		log_error(fmt.Sprintf("An error was encountered %s", err))
		panic(err)
	}
}

func mountUsb() int {
	cmd := exec.Command("grep", "-qs", "/mnt/usb", "/proc/mounts")
	err := cmd.Run()
	if err == nil {
		log_info("/dev/sda2 was already mounted")
		return 0
	}

	log_info("Mounting /dev/sda2")
	cmd = exec.Command("mount", "/dev/sda2", "/mnt/usb")
	err = cmd.Run()
	if err != nil {
		return 1
	}
	return 0
}

func unmountUsb() int {
	log_info("Unmounting /dev/sda2")
	cmd := exec.Command("umount", "/dev/sda2")
	err := cmd.Run()
	if err != nil {
		return 1
	}
	return 0
}

func getFreeSpace() int {
	ret := runCommandReturnInt("get-free-space")
	return ret
}
