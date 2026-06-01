package main

import (
	"encoding/json"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// setupTest points the helper-script seam (binDir) and the on-disk paths at a
// throwaway directory so handlers run hermetically — no root, no real board.
func setupTest(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	binDir = dir
	log_file = filepath.Join(dir, "reflash.log")
	images_folder = filepath.Join(dir, "images")
	options_file = filepath.Join(dir, "options.cfg")
	if err := os.MkdirAll(images_folder, 0o755); err != nil {
		t.Fatal(err)
	}
	options = &Options{}
	return dir
}

// fakeBin installs an executable stub helper script in dir (which is binDir).
func fakeBin(t *testing.T, dir, name, body string) {
	t.Helper()
	p := filepath.Join(dir, name)
	if err := os.WriteFile(p, []byte("#!/usr/bin/env bash\n"+body+"\n"), 0o755); err != nil {
		t.Fatal(err)
	}
}

func TestResolveCmd(t *testing.T) {
	binDir = "/usr/local/bin"
	cases := map[string]string{
		"wifi-scan":           "/usr/local/bin/wifi-scan", // helper -> joined with binDir
		"get-reflash-version": "/usr/local/bin/get-reflash-version",
		"pkill":               "pkill",     // system util -> unchanged
		"xz":                  "xz",        // system util -> unchanged
		"/bin/echo":           "/bin/echo", // explicit path -> unchanged
	}
	for in, want := range cases {
		if got := resolveCmd(in); got != want {
			t.Errorf("resolveCmd(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestGetInfo(t *testing.T) {
	dir := setupTest(t)
	fakeBin(t, dir, "get-reflash-version", `echo "v9.9.9"`)
	fakeBin(t, dir, "get-recore-revision", `echo "A8"`)
	fakeBin(t, dir, "get-recore-serial-number", `echo "RC-0042"`)
	fakeBin(t, dir, "get-emmc-version", `echo "emmc-1"`)
	fakeBin(t, dir, "is-ssh-enabled", `echo "true"`)
	fakeBin(t, dir, "get-free-space", `echo "12345"`)
	fakeBin(t, dir, "get-hostnames", `echo "recore.local"`)
	if err := os.WriteFile(filepath.Join(images_folder, "test.img.xz"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}

	rr := httptest.NewRecorder()
	getInfo(rr, httptest.NewRequest("GET", "/api/get_info", nil))

	var info GetInfo
	if err := json.Unmarshal(rr.Body.Bytes(), &info); err != nil {
		t.Fatalf("decode: %v (body=%q)", err, rr.Body.String())
	}
	if info.ReflashVersion != "v9.9.9" {
		t.Errorf("ReflashVersion = %q, want v9.9.9", info.ReflashVersion)
	}
	if info.RecoreRevision != "A8" {
		t.Errorf("RecoreRevision = %q, want A8", info.RecoreRevision)
	}
	if info.SerialNumber != "RC-0042" {
		t.Errorf("SerialNumber = %q, want RC-0042", info.SerialNumber)
	}
	if info.EmmcVersion != "emmc-1" {
		t.Errorf("EmmcVersion = %q, want emmc-1", info.EmmcVersion)
	}
	if !info.IsSshEnabled {
		t.Error("IsSshEnabled = false, want true")
	}
	if info.BytesAvailable != 12345 {
		t.Errorf("BytesAvailable = %d, want 12345", info.BytesAvailable)
	}
	if len(info.LocalImages) != 1 || info.LocalImages[0].Name != "test.img.xz" {
		t.Errorf("LocalImages = %+v, want one entry named test.img.xz", info.LocalImages)
	}
}

func TestGetSerialNumber(t *testing.T) {
	dir := setupTest(t)
	fakeBin(t, dir, "get-recore-serial-number", `echo "RC-0042"`)

	rr := httptest.NewRecorder()
	getSerialNumber(rr, httptest.NewRequest("GET", "/api/get_serial_number", nil))

	var got GetSerialNumber
	if err := json.Unmarshal(rr.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if got.SerialNumber != "RC-0042" {
		t.Errorf("SerialNumber = %q, want RC-0042", got.SerialNumber)
	}
}

func TestIsUsbPresent(t *testing.T) {
	for _, want := range []bool{true, false} {
		dir := setupTest(t)
		fakeBin(t, dir, "is-usb-present", "echo "+map[bool]string{true: "true", false: "false"}[want])

		rr := httptest.NewRecorder()
		isUsbPresent(rr, httptest.NewRequest("GET", "/api/is_usb_present", nil))

		var got BinaryCommandResult
		if err := json.Unmarshal(rr.Body.Bytes(), &got); err != nil {
			t.Fatal(err)
		}
		if got.Result != want {
			t.Errorf("is-usb-present -> Result = %v, want %v", got.Result, want)
		}
	}
}

func TestGetWifi(t *testing.T) {
	dir := setupTest(t)
	// getWifi runs `get-setting WIFI_SSID`; echo back a known SSID.
	fakeBin(t, dir, "get-setting", `echo "HomeNet"`)

	rr := httptest.NewRecorder()
	getWifi(rr, httptest.NewRequest("GET", "/api/get_wifi", nil))

	var got GetWifi
	if err := json.Unmarshal(rr.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if got.SSID != "HomeNet" {
		t.Errorf("SSID = %q, want HomeNet", got.SSID)
	}
}

func TestSetThenGetOptions(t *testing.T) {
	setupTest(t)

	body := `{"darkmode":false,"screenRotation":270,"SSID":"HomeNet"}`
	rr := httptest.NewRecorder()
	setOptions(rr, httptest.NewRequest("POST", "/api/set_options", strings.NewReader(body)))

	// getOptions should now reflect what we just set.
	rr2 := httptest.NewRecorder()
	getOptions(rr2, httptest.NewRequest("GET", "/api/get_options", nil))

	var got Options
	if err := json.Unmarshal(rr2.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if got.Darkmode != false || got.ScreenRotation != 270 || got.WifiSSID != "HomeNet" {
		t.Errorf("options round-trip mismatch: %+v", got)
	}
}

func TestParseXzUncompressedSize(t *testing.T) {
	// Real `xz -l` output for a 2 MiB file: rendered in KiB, so the MiB-based
	// parser yields 0. This pins the known limitation (fix: `xz --robot -l`).
	kib := "Strms  Blocks   Compressed Uncompressed  Ratio  Check   Filename\n" +
		"    1       1        448 B  2,048.0 KiB  0.000  CRC64   z.bin.xz\n"
	if got := parseXzUncompressedSize(kib); got != 0 {
		t.Errorf("KiB-rendered size: got %d, want 0 (documents the MiB-only limitation)", got)
	}

	// A real Recore image: both compressed and uncompressed render in MiB, so
	// the uncompressed value lands where the parser looks.
	mib := "Strms  Blocks   Compressed Uncompressed  Ratio  Check   Filename\n" +
		"    1       1      250.0 MiB  1,463.5 MiB  0.171  CRC64   image.img.xz\n"
	want := int(1463.5 * 1024 * 1024)
	if got := parseXzUncompressedSize(mib); got != want {
		t.Errorf("MiB-rendered size: got %d, want %d", got, want)
	}

	// Garbage / empty input must not panic.
	if got := parseXzUncompressedSize(""); got != 0 {
		t.Errorf("empty input: got %d, want 0", got)
	}
}
