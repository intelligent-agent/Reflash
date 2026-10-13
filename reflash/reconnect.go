package main

import (
	"context"
	"encoding/json"
	"net/http"
	"os/exec"
	"strconv"
	"strings"
	"time"
)

// resolved uses the same conflict-renamed label for LLMNR and mDNS, appending
// .local for mDNS. Read its published identity, never the next image's option.
func publishedMDNSName() string {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	property := func(name string) string {
		out, err := exec.CommandContext(ctx, resolveCmd("busctl"), "get-property",
			"org.freedesktop.resolve1", "/org/freedesktop/resolve1",
			"org.freedesktop.resolve1.Manager", name).Output()
		if err != nil {
			return ""
		}
		text := strings.TrimSpace(string(out))
		if !strings.HasPrefix(text, "s ") {
			return ""
		}
		value, err := strconv.Unquote(strings.TrimPrefix(text, "s "))
		if err != nil {
			return ""
		}
		return value
	}
	if property("MulticastDNS") != "yes" {
		return ""
	}
	name := strings.ToLower(property("LLMNRHostname"))
	if name == "" || validateHostname(name) != nil {
		return ""
	}
	return name + ".local"
}

func getReconnectAddress(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.Header().Set("Allow", "GET")
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"hostname": publishedMDNSName()})
}
