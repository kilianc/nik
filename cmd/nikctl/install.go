package main

import (
	"flag"
	"fmt"
	"os"

	"github.com/kciuffolo/nik/internal/daemonctl"
	"github.com/kciuffolo/nik/internal/home"
)

func runInstall(args []string) {
	flagSet := flag.NewFlagSet("install", flag.ExitOnError)
	homeFlag := flagSet.String("home", "", "workspace directory")
	flagSet.Parse(args)

	h, err := home.Resolve(*homeFlag)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}

	// The service runs nikd, not this binary. Resolving it here rather than
	// inside daemonctl keeps the failure where a person can act on it: a
	// half-unpacked install should say so now, not at the first boot.
	nikd, err := daemonctl.SiblingBinary("nikd")
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		fmt.Fprintln(os.Stderr, "nikd and nikctl install together — reinstall from the one-liner")
		os.Exit(1)
	}

	err = daemonctl.Install(nikd, h)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: install service: %v\n", err)
		os.Exit(1)
	}

	fmt.Println("service installed and started")
}
