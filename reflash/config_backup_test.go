package main

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// #184: the backup takes the name the user kept or typed, made safe for a file
// name, with one .tar.gz on the end.
func TestFileBackupTakesTheGivenName(t *testing.T) {
	dir := setupTest(t)
	fakeBin(t, dir, "target-manifest", `printf 'interface=1\nactions=backup,restore\n'`)
	state = &State{State: IDLE}
	fakeBin(t, dir, "get-emmc-version", `echo rebuild-fluidd-v1.2.0`)
	fakeBin(t, dir, "mount-unmount-usb", `exit 0`)
	fakeBin(t, dir, "target-install", `[ "$1" = backup ] && printf ARCHIVE > "$2"`)

	for given, want := range map[string]string{
		"recore-0132 config":        "recore-0132-config.tar.gz",
		"recore-0132-config.tar.gz": "recore-0132-config.tar.gz",
		"../../etc/passwd":          "..-..-etc-passwd.tar.gz",
	} {
		body, _ := json.Marshal(map[string]string{"name": given})
		w := httptest.NewRecorder()
		fileBackups(w, httptest.NewRequest("POST", "/api/file_backups", bytes.NewReader(body)))
		var made map[string]string
		json.NewDecoder(w.Body).Decode(&made)
		if made["name"] != want {
			t.Errorf("%q was saved as %q, want %q", given, made["name"], want)
		}
		if _, err := os.Stat(filepath.Join(backups_folder, want)); err != nil {
			t.Errorf("%s is not in the backups folder", want)
		}
	}
}

// Newest first by when it reached the drive: names are free text, so their
// order says nothing about age.
func TestFileBackupsNewestFirst(t *testing.T) {
	setupTest(t)
	os.MkdirAll(backups_folder, 0o755)
	now := time.Now()
	for i, name := range []string{"zz-oldest.tar.gz", "aa-newest.tar.gz", "mm-middle.tar.gz"} {
		p := filepath.Join(backups_folder, name)
		os.WriteFile(p, []byte("x"), 0o644)
		age := map[int]time.Duration{0: 3 * time.Hour, 1: 0, 2: time.Hour}[i]
		os.Chtimes(p, now.Add(-age), now.Add(-age))
	}
	w := httptest.NewRecorder()
	fileBackups(w, httptest.NewRequest("GET", "/api/file_backups", nil))
	var list []FileBackup
	json.NewDecoder(w.Body).Decode(&list)
	var got []string
	for _, b := range list {
		got = append(got, b.Name)
	}
	if strings.Join(got, " ") != "aa-newest.tar.gz mm-middle.tar.gz zz-oldest.tar.gz" {
		t.Errorf("order %v", got)
	}
}

func configArchive(t *testing.T) []byte {
	t.Helper()
	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gz)
	body := []byte("format=1\n")
	tw.WriteHeader(&tar.Header{Name: "rebuild-backup.manifest", Mode: 0o644, Size: int64(len(body))})
	tw.Write(body)
	tw.Close()
	gz.Close()
	return buf.Bytes()
}

func upload(t *testing.T, name string, payload []byte) {
	t.Helper()
	start, _ := json.Marshal(map[string]any{"filename": name, "size": len(payload), "start_time": 0})
	uploadStart(httptest.NewRecorder(), httptest.NewRequest("PUT", "/api/upload_start", bytes.NewReader(start)))
	uploadChunk(httptest.NewRecorder(), httptest.NewRequest("POST", "/api/upload_chunk", bytes.NewReader(payload)))
	uploadFinish(httptest.NewRecorder(), httptest.NewRequest("PUT", "/api/upload_finish", nil))
}

// An uploaded config archive goes with the backups, by its content: a file
// only named like one stays with the images.
func TestUploadedConfigArchiveGoesWithTheBackups(t *testing.T) {
	dir := setupTest(t)
	fakeBin(t, dir, "mount-unmount-usb", `exit 0`)

	state = &State{State: IDLE}
	upload(t, "mine.tar.gz", configArchive(t))
	if _, err := os.Stat(filepath.Join(backups_folder, "mine.tar.gz")); err != nil {
		t.Errorf("the config archive is not with the backups: %v", err)
	}
	if _, err := os.Stat(filepath.Join(images_folder, "mine.tar.gz")); err == nil {
		t.Error("the config archive was also put with the images")
	}

	state = &State{State: IDLE}
	upload(t, "fake.tar.gz", []byte("not gzip at all"))
	if _, err := os.Stat(filepath.Join(images_folder, "fake.tar.gz")); err != nil {
		t.Errorf("a file that is not a config archive left the images: %v", err)
	}
}

// A config alone goes into the system already installed, through the image's
// installer, and not while the eMMC is busy.
func TestRestoreIntoInstalled(t *testing.T) {
	dir := setupTest(t)
	fakeBin(t, dir, "target-manifest", `printf 'interface=1\nactions=backup,restore\n'`)
	called := filepath.Join(dir, "called")
	os.MkdirAll(backups_folder, 0o755)
	os.WriteFile(filepath.Join(backups_folder, "mine.tar.gz"), configArchive(t), 0o644)
	fakeBin(t, dir, "target-install", `echo "$1 $2" > `+called)

	state = &State{State: IDLE}
	w := httptest.NewRecorder()
	restoreIntoInstalled(w, httptest.NewRequest("POST", "/api/file_backups/restore", strings.NewReader(`{"name":"mine.tar.gz"}`)))
	if b := w.Body.String(); !strings.Contains(b, `"OK"`) {
		t.Fatalf("answered %s", b)
	}
	if got, _ := os.ReadFile(called); string(got) != "restore "+filepath.Join(backups_folder, "mine.tar.gz")+"\n" {
		t.Errorf("installer called with %q", got)
	}

	w = httptest.NewRecorder()
	restoreIntoInstalled(w, httptest.NewRequest("POST", "/api/file_backups/restore", strings.NewReader(`{"name":"gone.tar.gz"}`)))
	if b := w.Body.String(); !strings.Contains(b, "not on the USB drive") {
		t.Errorf("a missing backup answered %s", b)
	}

	state = &State{State: INSTALLING}
	w = httptest.NewRecorder()
	restoreIntoInstalled(w, httptest.NewRequest("POST", "/api/file_backups/restore", strings.NewReader(`{"name":"mine.tar.gz"}`)))
	if w.Code != http.StatusConflict {
		t.Errorf("while installing: %d %s", w.Code, w.Body.String())
	}
}

// An image goes to this computer by a name that cannot leave the folder.
func TestDownloadImage(t *testing.T) {
	setupTest(t)
	os.WriteFile(filepath.Join(images_folder, "rebuild-fluidd.img.xz"), []byte("IMAGE"), 0o644)
	w := httptest.NewRecorder()
	downloadImage(w, httptest.NewRequest("GET", "/api/images/download?name=rebuild-fluidd.img.xz", nil))
	if w.Body.String() != "IMAGE" || !strings.Contains(w.Header().Get("Content-Disposition"), "attachment") {
		t.Errorf("download %q %v", w.Body.String(), w.Header())
	}
	w = httptest.NewRecorder()
	downloadImage(w, httptest.NewRequest("GET", "/api/images/download?name=../options.cfg", nil))
	if w.Code != http.StatusNotFound {
		t.Errorf("a name outside the folder answered %d", w.Code)
	}
}

// An installed system that cannot do it is told so before anything runs,
// naming the system: too old for a manifest, or one that does not list it.
func TestConfigActionsNotSupportedByTheInstalledSystem(t *testing.T) {
	dir := setupTest(t)
	ran := filepath.Join(dir, "ran")
	state = &State{State: IDLE}
	os.MkdirAll(backups_folder, 0o755)
	os.WriteFile(filepath.Join(backups_folder, "mine.tar.gz"), configArchive(t), 0o644)
	fakeBin(t, dir, "get-emmc-version", `echo rebuild-fluidd-v1.0.2`)
	fakeBin(t, dir, "mount-unmount-usb", `exit 0`)
	fakeBin(t, dir, "target-install", `touch `+ran)

	for manifest, want := range map[string]string{
		`exit 0`: "rebuild-fluidd-v1.0.2 is too old",
		`printf 'interface=1\nactions=settings\n'`: "rebuild-fluidd-v1.0.2 does not support",
	} {
		fakeBin(t, dir, "target-manifest", manifest)

		w := httptest.NewRecorder()
		restoreIntoInstalled(w, httptest.NewRequest("POST", "/api/file_backups/restore", strings.NewReader(`{"name":"mine.tar.gz"}`)))
		if b := w.Body.String(); !strings.Contains(b, "not supported: "+want) {
			t.Errorf("install a config, %s: %s", manifest, b)
		}
		w = httptest.NewRecorder()
		fileBackups(w, httptest.NewRequest("POST", "/api/file_backups", nil))
		if b := w.Body.String(); !strings.Contains(b, "not supported: "+want) {
			t.Errorf("back up, %s: %s", manifest, b)
		}
	}
	if _, err := os.Stat(ran); err == nil {
		t.Error("the installer ran although the system does not support it")
	}
}
