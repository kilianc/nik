package main

import (
	"flag"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/kciuffolo/nik/internal/secrets"
)

func runSecrets(args []string) {
	flagSet := flag.NewFlagSet("secrets", flag.ExitOnError)
	home := flagSet.String("home", "", "workspace directory")
	flagSet.Parse(args)

	remaining := flagSet.Args()
	if len(remaining) == 0 {
		fmt.Fprintln(os.Stderr, "usage: nik secrets {read|list|write|delete} [name]")
		os.Exit(1)
	}

	action := remaining[0]

	h, err := resolveHome(*home)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}

	store := secrets.New(h)

	switch action {
	case "read":
		if len(remaining) < 2 {
			fmt.Fprintln(os.Stderr, "usage: nik secrets read <name>")
			os.Exit(1)
		}
		val, err := store.Get(remaining[1])
		if err != nil {
			fmt.Fprintf(os.Stderr, "error: %v\n", err)
			os.Exit(1)
		}
		fmt.Print(val)

	case "list":
		names, err := store.List()
		if err != nil {
			fmt.Fprintf(os.Stderr, "error: %v\n", err)
			os.Exit(1)
		}
		for _, n := range names {
			fmt.Println(n)
		}

	case "write":
		if len(remaining) < 2 {
			fmt.Fprintln(os.Stderr, "usage: nik secrets write <name>")
			os.Exit(1)
		}
		val, err := io.ReadAll(os.Stdin)
		if err != nil {
			fmt.Fprintf(os.Stderr, "error reading stdin: %v\n", err)
			os.Exit(1)
		}
		err = store.Set(remaining[1], strings.TrimRight(string(val), "\n"))
		if err != nil {
			fmt.Fprintf(os.Stderr, "error: %v\n", err)
			os.Exit(1)
		}

	case "delete":
		if len(remaining) < 2 {
			fmt.Fprintln(os.Stderr, "usage: nik secrets delete <name>")
			os.Exit(1)
		}
		err := store.Delete(remaining[1])
		if err != nil {
			fmt.Fprintf(os.Stderr, "error: %v\n", err)
			os.Exit(1)
		}

	default:
		fmt.Fprintf(os.Stderr, "unknown secrets action %q\n", action)
		fmt.Fprintln(os.Stderr, "usage: nik secrets {read|list|write|delete} [name]")
		os.Exit(1)
	}
}
