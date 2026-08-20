// Command nikctl is what a person types. It owns no state: every command
// here either talks to nikd or configures the host's service manager, and
// nothing in this binary opens NIK_HOME's database or secret store.
//
// It is installed as both `nikctl` and `nik` — the canonical name is nikctl,
// and `nik` is how it is spelled in the house.
package main

import (
	"fmt"
	"os"
	"strings"

	"github.com/kciuffolo/nik/internal/api"
	"github.com/kciuffolo/nik/internal/version"
)

// apiVersionExpected is the API this nikctl was built against. nikd and nikctl
// ship together, so a mismatch means a partial upgrade — worth a warning, not
// a refusal: a status command that will not run is no help diagnosing why.
const apiVersionExpected = api.APIVersion

func main() {
	subcmd := ""
	if len(os.Args) > 1 && !strings.HasPrefix(os.Args[1], "-") {
		subcmd = os.Args[1]
	}

	known := []string{"connect", "install", "logs", "query", "restart", "secrets", "shell", "status", "tui", "version"}

	switch subcmd {
	case "version":
		fmt.Println(version.String())
	case "connect":
		runConnect(os.Args[2:])
	case "install":
		runInstall(os.Args[2:])
	case "secrets":
		runSecrets(os.Args[2:])
	case "status":
		runStatus(os.Args[2:])
	case "query":
		runQuery(os.Args[2:])
	case "shell":
		runShell(os.Args[2:])
	case "logs":
		runLogs(os.Args[2:])
	case "restart":
		runRestart(os.Args[2:])
	case "tui":
		runTUI(os.Args[2:])
	case "":
		runTUI(os.Args[1:])
	case "daemon":
		// The split's one sharp edge: muscle memory and any hand-written
		// service file still say `nik daemon`. Say where it went rather than
		// "unknown command".
		fmt.Fprintln(os.Stderr, "the daemon is its own binary now: run `nikd --home <dir>`")
		fmt.Fprintln(os.Stderr, "(services installed by `nikctl install` already point at it)")
		os.Exit(1)
	default:
		fmt.Fprintf(os.Stderr, "unknown command %q\n", subcmd)
		fmt.Fprintf(os.Stderr, "available commands: %s\n", strings.Join(known, ", "))
		os.Exit(1)
	}
}
