//go:build ignore

package main

import (
	"fmt"
	"os"
	"os/signal"
	"syscall"
)

func main() {
	// `reflash ctl [COMMAND...]` is the client half, for a user logged in on
	// ttyGS0. Handled before anything else starts: it must not touch the
	// screen or bind the server's ports.
	if len(os.Args) > 1 && os.Args[1] == "ctl" {
		os.Exit(runControlClient(os.Args[2:]))
	}

	// Belt and braces alongside the O_NOCTTY in serveSerialControl: a flashing
	// appliance should not be killable by a USB cable event. Go's default
	// disposition for SIGHUP is to terminate, and there is nothing this process
	// wants to do on a hangup - it has no controlling terminal to lose and no
	// config to reload. See issue #113.
	signal.Ignore(syscall.SIGHUP)

	env := os.Getenv("APP_ENV")
	if env != "dev" {
		fmt.Println("Starting Screen")
		ScreenInit()
	}
	fmt.Println("Starting Server")
	ServerInit()
}
