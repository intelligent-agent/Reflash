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
		// A release with no manifest at all is backed up by Reflash itself
		// (#187), which is the next test; one that has a manifest and does
		// not list backup is not.
		if manifest == `exit 0` {
			continue
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

// #187: a Rebuild release before the target interface has no manifest and no
// backup action, and is backed up by target-install itself. Reflash lets a
// "rebuild-" system with no manifest through to it; target-install says no (3)
// for a release it does not know, and anything that is not Rebuild is refused
// before it runs.
func TestAnOlderRebuildReleaseIsBackedUpByTargetInstall(t *testing.T) {
	dir := setupTest(t)
	args := filepath.Join(dir, "args")
	state = &State{State: IDLE}
	setStorage(STORAGE_READY)
	fakeBin(t, dir, "mount-unmount-usb", `exit 0`)
	fakeBin(t, dir, "target-manifest", `exit 0`)
	fakeBin(t, dir, "target-install", `echo "$@" > `+args+`; printf archive > "$2"`)

	for _, release := range []string{"rebuild-fluidd-v1.0.2", "rebuild-octoprint-v1.1.0", "rebuild-barebone-v1.0.0"} {
		os.Remove(args)
		fakeBin(t, dir, "get-emmc-version", `echo `+release)
		w := httptest.NewRecorder()
		fileBackups(w, httptest.NewRequest("POST", "/api/file_backups", nil))
		if b := w.Body.String(); !strings.Contains(b, `"status":"OK"`) || !strings.Contains(b, release+"-files-") {
			t.Errorf("%s: %s", release, b)
		}
		if got, _ := os.ReadFile(args); !strings.HasPrefix(string(got), "backup ") {
			t.Errorf("%s: target-install was run with %q", release, got)
		}
	}
}

func TestOnlyARebuildWithoutAManifestIsHandedToTargetInstall(t *testing.T) {
	dir := setupTest(t)
	ran := filepath.Join(dir, "ran")
	state = &State{State: IDLE}
	setStorage(STORAGE_READY)
	fakeBin(t, dir, "mount-unmount-usb", `exit 0`)
	fakeBin(t, dir, "target-manifest", `exit 0`)
	fakeBin(t, dir, "target-install", `touch `+ran)

	for _, system := range []string{"Unknown version", "refactor-v0.9", ""} {
		fakeBin(t, dir, "get-emmc-version", `echo "`+system+`"`)
		w := httptest.NewRecorder()
		fileBackups(w, httptest.NewRequest("POST", "/api/file_backups", nil))
		if b := w.Body.String(); !strings.Contains(b, "not supported") {
			t.Errorf("%q: %s", system, b)
		}
	}
	if _, err := os.Stat(ran); err == nil {
		t.Error("target-install ran for a system that is not a Rebuild")
	}
}

func TestAReleaseTargetInstallDoesNotKnowIsNotSupported(t *testing.T) {
	dir := setupTest(t)
	state = &State{State: IDLE}
	setStorage(STORAGE_READY)
	fakeBin(t, dir, "mount-unmount-usb", `exit 0`)
	fakeBin(t, dir, "target-manifest", `exit 0`)
	fakeBin(t, dir, "get-emmc-version", `echo rebuild-fluidd-v1.9.9`)
	fakeBin(t, dir, "target-install", `exit 3`)
	w := httptest.NewRecorder()
	fileBackups(w, httptest.NewRequest("POST", "/api/file_backups", nil))
	if b := w.Body.String(); !strings.Contains(b, "does not support backing up") {
		t.Errorf("a release target-install turns down: %s", b)
	}
}

// The end of an install unmounts the drive; with the board staying in Reflash,
// reading the backups or restoring one mounts it again first - read-only, and
// not while something is using it.
func TestReadingTheDriveMountsItAgainAfterAnInstall(t *testing.T) {
	dir := setupTest(t)
	mounted := filepath.Join(dir, "mounted")
	fakeBin(t, dir, "mount-unmount-usb", `echo "$1 $2" >> `+mounted)
	setStorage(STORAGE_READY)

	state = &State{State: FINISHED}
	fileBackups(httptest.NewRecorder(), httptest.NewRequest("GET", "/api/file_backups", nil))
	if got, _ := os.ReadFile(mounted); string(got) != "mounted ro\n" {
		t.Errorf("after an install: %q", got)
	}

	os.Remove(mounted)
	state = &State{State: INSTALLING}
	fileBackups(httptest.NewRecorder(), httptest.NewRequest("GET", "/api/file_backups", nil))
	if _, err := os.Stat(mounted); err == nil {
		t.Error("the drive was remounted under an install")
	}
}

// #188: the files window lists images and config archives with their date,
// archives with whether they open, and deletes both through delete_image.
func TestFilesWindowListsAndDeletes(t *testing.T) {
	dir := setupTest(t)
	fakeBin(t, dir, "mount-unmount-usb", `exit 0`)
	os.MkdirAll(backups_folder, 0o755)
	os.WriteFile(filepath.Join(images_folder, "a.img.xz"), []byte("img"), 0o644)
	os.WriteFile(filepath.Join(backups_folder, "good.tar.gz"), configArchive(t), 0o644)
	os.WriteFile(filepath.Join(backups_folder, "bad.tar.gz"), []byte("not gzip"), 0o644)

	if imgs := getLocalImages(); len(imgs) != 1 || imgs[0].Date == 0 {
		t.Errorf("images %+v", imgs)
	}
	w := httptest.NewRecorder()
	fileBackups(w, httptest.NewRequest("GET", "/api/file_backups", nil))
	var list []FileBackup
	json.NewDecoder(w.Body).Decode(&list)
	ok := map[string]bool{}
	for _, b := range list {
		if b.Date == 0 {
			t.Errorf("%s has no date", b.Name)
		}
		ok[b.Name] = b.Ok
	}
	if !ok["good.tar.gz"] || ok["bad.tar.gz"] {
		t.Errorf("integrity %v", ok)
	}

	del := func(name string) int {
		state = &State{State: IDLE}
		w := httptest.NewRecorder()
		deleteImage(w, httptest.NewRequest("PUT", "/api/delete_image",
			strings.NewReader(`{"filename":"`+name+`"}`)))
		return w.Code
	}
	if c := del("good.tar.gz"); c != http.StatusOK {
		t.Errorf("deleting a config archive answered %d", c)
	}
	if _, err := os.Stat(filepath.Join(backups_folder, "good.tar.gz")); err == nil {
		t.Error("the config archive is still there")
	}
	for _, name := range []string{"../options.cfg", "options.cfg", ".hidden.tar.gz"} {
		if c := del(name); c != http.StatusBadRequest {
			t.Errorf("%q answered %d", name, c)
		}
	}
	if c := del("a.img.xz"); c != http.StatusOK {
		t.Errorf("deleting an image answered %d", c)
	}
}

// #198: only some of the files, as the tree picks them. The paths reach the
// installer as --include arguments, and only when it can list files.
func TestFileBackupAndRestoreCarryAnIncludeList(t *testing.T) {
	dir := setupTest(t)
	state = &State{State: IDLE}
	fakeBin(t, dir, "get-emmc-version", `echo rebuild-fluidd-v1.2.0`)
	fakeBin(t, dir, "mount-unmount-usb", `exit 0`)
	argsFile := filepath.Join(dir, "args")
	fakeBin(t, dir, "target-install", `echo "$@" >> `+argsFile+`; [ "$1" = backup ] && printf ARCHIVE > "$2"; exit 0`)

	fakeBin(t, dir, "target-manifest", `printf 'interface=1\nactions=backup,restore\n'`)
	body, _ := json.Marshal(map[string]any{"name": "x", "include": []string{"home/printer/printer_data/config/printer.cfg"}})
	w := httptest.NewRecorder()
	fileBackups(w, httptest.NewRequest("POST", "/api/file_backups", bytes.NewReader(body)))
	if !strings.Contains(w.Body.String(), "does not support") {
		t.Errorf("an installer that cannot list was handed a list: %s", w.Body.String())
	}

	fakeBin(t, dir, "target-manifest", `printf 'interface=1\nactions=backup,restore,list,list-archive\n'`)
	w = httptest.NewRecorder()
	fileBackups(w, httptest.NewRequest("POST", "/api/file_backups", bytes.NewReader(body)))
	got, _ := os.ReadFile(argsFile)
	if !strings.Contains(string(got), "--include home/printer/printer_data/config/printer.cfg") {
		t.Errorf("the installer was run as %q (%s)", got, w.Body.String())
	}

	os.MkdirAll(backups_folder, 0o755)
	os.WriteFile(filepath.Join(backups_folder, "x.tar.gz"), configArchive(t), 0o644)
	os.Remove(argsFile)
	body, _ = json.Marshal(map[string]any{"name": "x.tar.gz", "include": []string{"home/printer/printer_data/config"}})
	w = httptest.NewRecorder()
	restoreIntoInstalled(w, httptest.NewRequest("POST", "/api/file_backups/restore", bytes.NewReader(body)))
	got, _ = os.ReadFile(argsFile)
	if !strings.Contains(string(got), "restore") || !strings.Contains(string(got), "--include home/printer/printer_data/config") {
		t.Errorf("restore was run as %q (%s)", got, w.Body.String())
	}
}

func TestListBackupFilesAsksTheInstalledSystemOrAnArchive(t *testing.T) {
	dir := setupTest(t)
	state = &State{State: IDLE}
	fakeBin(t, dir, "get-emmc-version", `echo rebuild-fluidd-v1.2.0`)
	fakeBin(t, dir, "mount-unmount-usb", `exit 0`)
	fakeBin(t, dir, "target-manifest", `printf 'interface=1\nactions=list,list-archive\n'`)
	fakeBin(t, dir, "target-install", `case $1 in list) printf 'a/one.cfg\na/two.cfg\n';; list-archive) printf 'b/only.cfg\n';; esac`)
	os.MkdirAll(backups_folder, 0o755)
	os.WriteFile(filepath.Join(backups_folder, "x.tar.gz"), configArchive(t), 0o644)

	ask := func(q string) map[string]any {
		w := httptest.NewRecorder()
		listBackupFiles(w, httptest.NewRequest("GET", "/api/file_backups/files"+q, nil))
		var out map[string]any
		json.NewDecoder(w.Body).Decode(&out)
		return out
	}
	if got := ask(""); got["supported"] != true || len(got["files"].([]any)) != 2 {
		t.Errorf("installed: %v", got)
	}
	if got := ask("?name=x.tar.gz"); got["supported"] != true || got["files"].([]any)[0] != "b/only.cfg" {
		t.Errorf("archive: %v", got)
	}
	// Not a name that can leave the folder.
	w := httptest.NewRecorder()
	listBackupFiles(w, httptest.NewRequest("GET", "/api/file_backups/files?name=../../etc/passwd", nil))
	if w.Code != http.StatusNotFound {
		t.Errorf("a path was taken as a name: %d", w.Code)
	}
	// An image that cannot list says so, and the whole archive is the only choice.
	fakeBin(t, dir, "target-manifest", `printf 'interface=1\nactions=backup\n'`)
	if got := ask(""); got["supported"] != false || got["reason"] == "" {
		t.Errorf("unsupported: %v", got)
	}
}

// The demo's bug: an archive of three files put into a system with eight took
// the other five with it, because "all of the archive chosen" was no list and no
// list replaces the config folder. With an installer that can list, a restore
// always asks to merge.
func TestRestoreMergesWhenTheInstallerCanList(t *testing.T) {
	dir := setupTest(t)
	state = &State{State: IDLE}
	fakeBin(t, dir, "get-emmc-version", `echo rebuild-fluidd-v1.2.0`)
	fakeBin(t, dir, "mount-unmount-usb", `exit 0`)
	argsFile := filepath.Join(dir, "args")
	fakeBin(t, dir, "target-install", `echo "$@" >> `+argsFile+`; exit 0`)
	os.MkdirAll(backups_folder, 0o755)
	os.WriteFile(filepath.Join(backups_folder, "x.tar.gz"), configArchive(t), 0o644)

	run := func(manifest string, include []string) string {
		os.Remove(argsFile)
		fakeBin(t, dir, "target-manifest", `printf 'interface=1\nactions=`+manifest+`\n'`)
		if err := restoreFileBackup("x.tar.gz", include); err != nil {
			t.Fatal(err)
		}
		got, _ := os.ReadFile(argsFile)
		return strings.TrimSpace(string(got))
	}
	if got := run("restore,list,list-archive", nil); !strings.HasSuffix(got, "--merge") {
		t.Errorf("an installer that can list was asked to replace: %q", got)
	}
	// A chosen list is merged by the installer already.
	if got := run("restore,list,list-archive", []string{"home/printer/printer_data/config"}); strings.Contains(got, "--merge") || !strings.Contains(got, "--include home/printer/printer_data/config") {
		t.Errorf("a list was sent as %q", got)
	}
	// One that cannot is asked for what it always did.
	if got := run("restore", nil); strings.Contains(got, "--merge") || strings.Contains(got, "--include") {
		t.Errorf("an older installer was sent %q", got)
	}
}
