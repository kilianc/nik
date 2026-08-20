package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/kciuffolo/nik/internal/home"
	"github.com/kciuffolo/nik/internal/nikapi"
)

// The commands that used to require being on the box with a terminal. They
// exist here first because a local install should not be worse off than a
// managed one, and because a thing with a CLI in front of it gets exercised.

// nikctl query [--home dir] "SELECT ..."
func runQuery(args []string) {
	flagSet := flag.NewFlagSet("query", flag.ExitOnError)
	homeFlag := flagSet.String("home", "", "workspace directory")
	flagSet.Parse(args)

	query := strings.TrimSpace(strings.Join(flagSet.Args(), " "))
	if query == "" {
		fmt.Fprintln(os.Stderr, `usage: nikctl query [--home dir] "SELECT ..."`)
		os.Exit(1)
	}

	client, ctx, cancel := clientFor(*homeFlag, 60*time.Second)
	defer cancel()

	result, err := client.Query(ctx, query)
	exitOnError(err, *homeFlag)

	// Pretty, because the reason to run this by hand is to read it.
	encoded, err := json.MarshalIndent(result, "", "  ")
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
	fmt.Println(string(encoded))
}

// nikctl shell [--home dir] [--wait seconds] "command"
func runShell(args []string) {
	flagSet := flag.NewFlagSet("shell", flag.ExitOnError)
	homeFlag := flagSet.String("home", "", "workspace directory")
	wait := flagSet.Int("wait", 0, "seconds to wait for output before returning")
	flagSet.Parse(args)

	command := strings.TrimSpace(strings.Join(flagSet.Args(), " "))
	if command == "" {
		fmt.Fprintln(os.Stderr, `usage: nikctl shell [--home dir] [--wait seconds] "command"`)
		os.Exit(1)
	}

	client, ctx, cancel := clientFor(*homeFlag, 3*time.Minute)
	defer cancel()

	result, err := client.Shell(ctx, command, *wait)
	exitOnError(err, *homeFlag)

	if result.Output != "" {
		fmt.Println(result.Output)
	}
	if result.Running {
		fmt.Fprintln(os.Stderr, "\n(still running — it kept going in nik's sandbox)")
		return
	}

	os.Exit(result.ExitCode)
}

// nikctl logs [--home dir] [--errors] [--lines n]
func runLogs(args []string) {
	flagSet := flag.NewFlagSet("logs", flag.ExitOnError)
	homeFlag := flagSet.String("home", "", "workspace directory")
	errorsOnly := flagSet.Bool("errors", false, "read the warnings-and-above log instead")
	lines := flagSet.Int("lines", 0, "how many lines to show")
	flagSet.Parse(args)

	client, ctx, cancel := clientFor(*homeFlag, 30*time.Second)
	defer cancel()

	out, err := client.Logs(ctx, *errorsOnly, *lines)
	exitOnError(err, *homeFlag)

	for _, line := range out {
		fmt.Println(line)
	}
}

// nikctl restart [--home dir]
func runRestart(args []string) {
	flagSet := flag.NewFlagSet("restart", flag.ExitOnError)
	homeFlag := flagSet.String("home", "", "workspace directory")
	flagSet.Parse(args)

	client, ctx, cancel := clientFor(*homeFlag, 30*time.Second)
	defer cancel()

	err := client.Restart(ctx)
	exitOnError(err, *homeFlag)

	// The ack only says nikd read the request. Waiting for the socket to
	// answer again is what proves it came back — and a daemon nobody is
	// managing will not, which is worth saying rather than hanging.
	fmt.Println("restarting...")

	deadline := time.Now().Add(30 * time.Second)
	for time.Now().Before(deadline) {
		time.Sleep(500 * time.Millisecond)

		probe, probeCancel := context.WithTimeout(context.Background(), 2*time.Second)
		_, err := client.Health(probe)
		probeCancel()

		if err == nil {
			fmt.Println("nik is back")
			return
		}
	}

	fmt.Fprintln(os.Stderr, "nik has not come back — if nothing is managing it, start it with `nikd --home <dir>`")
	os.Exit(1)
}

func clientFor(homeFlag string, timeout time.Duration) (*nikapi.Client, context.Context, context.CancelFunc) {
	h, err := home.Resolve(homeFlag)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}

	ctx, cancel := context.WithTimeout(context.Background(), timeout)

	return nikapi.New(h), ctx, cancel
}
