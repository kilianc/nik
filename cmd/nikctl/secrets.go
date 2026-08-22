package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"github.com/kciuffolo/nik/internal/home"
	"github.com/kciuffolo/nik/internal/nikapi"
)

// runSecrets reads and writes the encrypted store through nikd rather than
// through the files.
//
// The contract is unchanged — read / list / write / delete, value on stdout,
// value from stdin — because workspace/secrets/cli and every skill that shells
// out to it depend on exactly that. What changed is who holds the key: inside
// the shell container this now reaches a socket that answers for some names
// and refuses for others, instead of decrypting a file the sandbox had mounted
// all along.
func runSecrets(args []string) {
	flagSet := flag.NewFlagSet("secrets", flag.ExitOnError)
	homeFlag := flagSet.String("home", "", "workspace directory")
	parseFlags(flagSet, args)

	remaining := flagSet.Args()
	if len(remaining) == 0 {
		usageSecrets()
	}

	action := remaining[0]

	h, err := home.Resolve(*homeFlag)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}

	client := nikapi.New(h)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	switch action {
	case "read":
		name := secretName(remaining, "read")
		value, err := client.Secret(ctx, name)
		exitOnError(err, h)
		fmt.Print(value)

	case "list":
		names, err := client.Secrets(ctx)
		exitOnError(err, h)
		for _, n := range names {
			fmt.Println(n)
		}

	case "write":
		name := secretName(remaining, "write")
		value, err := io.ReadAll(os.Stdin)
		if err != nil {
			fmt.Fprintf(os.Stderr, "error reading stdin: %v\n", err)
			os.Exit(1)
		}
		err = client.SetSecret(ctx, name, strings.TrimRight(string(value), "\n"))
		exitOnError(err, h)

	case "delete":
		name := secretName(remaining, "delete")
		err := client.DeleteSecret(ctx, name)
		exitOnError(err, h)

	default:
		fmt.Fprintf(os.Stderr, "unknown secrets action %q\n", action)
		usageSecrets()
	}
}

func secretName(args []string, action string) string {
	if len(args) < 2 {
		fmt.Fprintf(os.Stderr, "usage: nikctl secrets %s <name>\n", action)
		os.Exit(1)
	}

	return args[1]
}

func usageSecrets() {
	fmt.Fprintln(os.Stderr, "usage: nikctl secrets {read|list|write|delete} [name]")
	os.Exit(1)
}

func exitOnError(err error, h string) {
	if err == nil {
		return
	}

	if errors.Is(err, nikapi.ErrNoDaemon) {
		fmt.Fprintf(os.Stderr, "error: no nik running at %s — secrets live in the daemon now\n", h)
		os.Exit(1)
	}

	fmt.Fprintf(os.Stderr, "error: %v\n", err)
	os.Exit(1)
}
