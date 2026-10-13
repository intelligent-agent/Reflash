package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
)

func TestReconnectAddress(t *testing.T) {
	for _, tc := range []struct{ label, body, want string }{
		{"conflict renamed", `if [ "$5" = MulticastDNS ]; then echo 's "yes"'; else echo 's "recore-42"'; fi`, "recore-42.local"},
		{"disabled", `echo 's "no"'`, ""},
		{"missing", `exit 1`, ""},
		{"invalid", `if [ "$5" = MulticastDNS ]; then echo 's "yes"'; else echo 's "bad/name"'; fi`, ""},
	} {
		t.Run(tc.label, func(t *testing.T) {
			dir := setupTest(t)
			fakeBin(t, dir, "busctl", tc.body)
			t.Setenv("PATH", dir+":"+os.Getenv("PATH"))
			rr := httptest.NewRecorder()
			getReconnectAddress(rr, httptest.NewRequest(http.MethodGet, "/api/reconnect_address", nil))
			var result map[string]string
			if err := json.Unmarshal(rr.Body.Bytes(), &result); err != nil {
				t.Fatal(err)
			}
			if result["hostname"] != tc.want {
				t.Fatalf("got %q want %q", result["hostname"], tc.want)
			}
		})
	}
}
func TestReconnectAddressRejectsWrites(t *testing.T) {
	rr := httptest.NewRecorder()
	getReconnectAddress(rr, httptest.NewRequest(http.MethodPost, "/api/reconnect_address", nil))
	if rr.Code != http.StatusMethodNotAllowed {
		t.Fatal(rr.Code)
	}
}
