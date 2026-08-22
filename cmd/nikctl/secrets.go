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

	action, name, err := secretArgs(flagSet.Args())
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}

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
		value, err := io.ReadAll(os.Stdin)
		if err != nil {
			fmt.Fprintf(os.Stderr, "error reading stdin: %v\n", err)
			os.Exit(1)
		}
		err = client.SetSecret(ctx, name, strings.TrimRight(string(value), "\n"))
		exitOnError(err, h)

	case "delete":
		err := client.DeleteSecret(ctx, name)
		exitOnError(err, h)
	}
}

// secretArgs is the action and the name it acts on, or the reason the line is
// not a command.
//
// Exactly the arguments the action takes, no spares. An argument nikctl does
// not want is not punctuation to step over: `secrets write openai_key --hom
// /nik` is a flag misspelled past the point flag.Parse could object to it, and
// counting it and moving on writes the secret into a different home without a
// word about it.
func secretArgs(args []string) (string, string, error) {
	usage := errors.New("usage: nikctl secrets {read|list|write|delete} [name] [--home dir]")
	if len(args) == 0 {
		return "", "", usage
	}

	action, rest := args[0], args[1:]

	if action == "list" {
		if len(rest) != 0 {
			return "", "", errors.New("usage: nikctl secrets list [--home dir]")
		}
		return action, "", nil
	}

	if action != "read" && action != "write" && action != "delete" {
		return "", "", fmt.Errorf("unknown secrets action %q\n%w", action, usage)
	}

	if len(rest) != 1 {
		return "", "", fmt.Errorf("usage: nikctl secrets %s <name> [--home dir]", action)
	}

	return action, rest[0], nil
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
