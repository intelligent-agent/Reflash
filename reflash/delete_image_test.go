package main

import (
	"encoding/json"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func deleteReq(name string) *httptest.ResponseRecorder {
	body, _ := json.Marshal(map[string]string{"filename": name})
	w := httptest.NewRecorder()
	deleteImage(w, httptest.NewRequest("PUT", "/api/delete_image", strings.NewReader(string(body))))
	return w
}

func TestDeleteImageRemovesItAndLeavesTheDriveReadOnly(t *testing.T) {
	dir := setupTest(t)
	mounts := filepath.Join(dir, "mounts")
	fakeBin(t, dir, "mount-unmount-usb", `echo "$@" >> `+mounts)
	state = &State{State: IDLE}
	path := filepath.Join(images_folder, "truncated.img.xz")
	os.WriteFile(path, []byte("x"), 0o644)

	w := deleteReq("truncated.img.xz")

	if w.Code != 200 || !strings.Contains(w.Body.String(), `"OK"`) {
		t.Fatalf("delete answered %d %s", w.Code, w.Body.String())
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Error("the image is still there")
	}
	got, _ := os.ReadFile(mounts)
	if strings.TrimSpace(string(got)) != "mounted rw\nmounted ro" {
		t.Errorf("mount calls %q, want rw then ro", got)
	}
	if state.State != IDLE {
		t.Errorf("state left at %q", state.State)
	}
}

// It deletes as root, so only a bare image name in the images folder.
func TestDeleteImageRefusesAnythingButAnImageName(t *testing.T) {
	dir := setupTest(t)
	fakeBin(t, dir, "mount-unmount-usb", `exit 0`)
	state = &State{State: IDLE}
	victim := filepath.Join(dir, "options.cfg")
	os.WriteFile(victim, []byte("keep"), 0o644)

	for _, name := range []string{"", "../options.cfg", "options.cfg", "../x.img.xz", ".img.xz", "sub/x.img.xz"} {
		if w := deleteReq(name); w.Code != 400 {
			t.Errorf("%q answered %d, want 400", name, w.Code)
		}
	}
	if _, err := os.Stat(victim); err != nil {
		t.Error("a file outside the images folder was deleted")
	}
}

func TestDeleteImageRefusesWhileBusy(t *testing.T) {
	setupTest(t)
	path := filepath.Join(images_folder, "in-use.img.xz")
	os.WriteFile(path, []byte("x"), 0o644)

	for _, s := range []string{INSTALLING, UPLOADING, DOWNLOADING, BACKUPING, MAGIC, UPLOADING_MAGIC, SAVING} {
		state = &State{State: s}
		if w := deleteReq("in-use.img.xz"); w.Code != 409 {
			t.Errorf("during %s answered %d, want 409", s, w.Code)
		}
	}
	if _, err := os.Stat(path); err != nil {
		t.Error("deleted an image while the drive was in use")
	}
}

func TestDeleteImageThatIsNotThere(t *testing.T) {
	setupTest(t)
	state = &State{State: IDLE}
	if w := deleteReq("gone.img.xz"); w.Code != 404 {
		t.Errorf("answered %d, want 404", w.Code)
	}
}
