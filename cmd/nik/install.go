package main

import (
	"flag"
	"fmt"
	"os"

	"github.com/kciuffolo/nik/internal/daemonctl"
)

func runInstall(args []string) {
	flagSet := flag.NewFlagSet("install", flag.ExitOnError)
	home := flagSet.String("home", "", "workspace directory")
	flagSet.Parse(args)

	h, err := resolveHome(*home)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}

	exe, err := os.Executable()
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: resolve executable path: %v\n", err)
		os.Exit(1)
	}

	err = daemonctl.Install(exe, h)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: install service: %v\n", err)
		os.Exit(1)
	}

	fmt.Println("service installed and started")
}
