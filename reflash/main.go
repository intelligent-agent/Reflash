//go:build ignore

package main

import (
	"fmt"
	"os"
)

func main() {
	env := os.Getenv("APP_ENV")
	if env != "dev" {
		fmt.Println("Starting Screen")
		ScreenInit()
	}
	fmt.Println("Starting Server")
	ServerInit()
}
