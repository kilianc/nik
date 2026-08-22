package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"os"
	"sort"
	"text/tabwriter"
	"time"

	"github.com/kciuffolo/nik/internal/home"
	"github.com/kciuffolo/nik/internal/nikapi"
	"github.com/kciuffolo/nik/internal/version"
)

// runStatus asks nikd how it is. It is the first thing built on the API and
// deliberately the smallest: what it proves is that the socket, the client
// and the daemon's own account of itself all line up.
//
//	nikctl status [--home dir]
func runStatus(args []string) {
	flagSet := flag.NewFlagSet("status", flag.ExitOnError)
	homeFlag := flagSet.String("home", "", "workspace directory")
	parseFlags(flagSet, args)

	h, err := home.Resolve(*homeFlag)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}

	client := nikapi.New(h)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	health, err := client.Health(ctx)
	if errors.Is(err, nikapi.ErrNoDaemon) {
		// The common failure, and the one worth spelling out: a connection
		// error naming a socket is not something anybody can act on.
		fmt.Fprintf(os.Stderr, "no nik running at %s\n\n", h)
		fmt.Fprintln(os.Stderr, "  start it:      nikd --home "+h)
		fmt.Fprintln(os.Stderr, "  or install it: nikctl install --home "+h)
		os.Exit(1)
	}
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}

	ver, err := client.Version(ctx)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}

	switch {
	case health.Ready:
		fmt.Println("nik is running")
	case len(health.Degraded) > 0:
		fmt.Printf("nik is running, degraded: %v\n", health.Degraded)
	default:
		fmt.Println("nik is starting")
	}

	fmt.Println()

	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintf(w, "  version\t%s\n", ver.Version)
	fmt.Fprintf(w, "  uptime\t%s\n", (time.Duration(health.UptimeS) * time.Second).String())
	fmt.Fprintf(w, "  home\t%s\n", h)

	names := make([]string, 0, len(health.Subsystem))
	for name := range health.Subsystem {
		names = append(names, name)
	}
	sort.Strings(names)

	for _, name := range names {
		sub := health.Subsystem[name]
		mark := "ok"
		if !sub.OK {
			mark = "FAILING"
		}
		fmt.Fprintf(w, "  %s\t%s\t%s\n", name, mark, sub.Detail)
	}
	w.Flush()

	// A nikctl older than the nikd it is talking to reads fields that may have
	// changed meaning. They ship together, so this only happens after a
	// partial upgrade — say so rather than printing something subtly wrong.
	if ver.APIVersion != apiVersionExpected {
		fmt.Fprintf(os.Stderr,
			"\nwarning: nikd speaks API v%d, this nikctl expects v%d — reinstall to get a matching pair (nikctl is %s)\n",
			ver.APIVersion, apiVersionExpected, version.String())
	}

	if len(health.Degraded) > 0 {
		os.Exit(1)
	}
}
