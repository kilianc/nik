package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"path/filepath"

	"github.com/kciuffolo/nik/internal/codex"
	"github.com/kciuffolo/nik/internal/config"
	"github.com/kciuffolo/nik/internal/db"
	"github.com/kciuffolo/nik/internal/workbench"
)

func main() {
	if len(os.Args) < 2 {
		fmt.Fprintln(os.Stderr, "usage: workbench <replay|create|variant|status|report> [flags]")
		os.Exit(1)
	}

	cmd := os.Args[1]
	fs := flag.NewFlagSet("workbench", flag.ContinueOnError)
	round := fs.String("round", "", "activation round ID")
	experiment := fs.String("experiment", "", "experiment ID")
	variant := fs.String("variant", "", "variant ID")
	name := fs.String("name", "", "variant name")
	desired := fs.String("desired", "", "desired outcome or tool pattern")
	hypothesis := fs.String("hypothesis", "", "hypothesis text")
	patches := fs.String("patches", "", "patches JSON file path")
	effort := fs.String("effort", "", "reasoning effort override")
	verbosity := fs.String("verbosity", "", "verbosity override")
	n := fs.Int("n", 1, "number of replay attempts")
	jsonOut := fs.Bool("json", false, "structured JSON output")

	err := fs.Parse(os.Args[2:])
	if err != nil {
		os.Exit(1)
	}

	cfg, err := config.Load("")
	if err != nil {
		fmt.Fprintf(os.Stderr, "load config: %v\n", err)
		os.Exit(1)
	}

	conn, err := db.Open(cfg.DBPath(), cfg.TZ())
	if err != nil {
		fmt.Fprintf(os.Stderr, "open db: %v\n", err)
		os.Exit(1)
	}
	defer conn.Close()

	oaiClient, err := codex.BuildOpenAIClient(cfg.OpenAIKey)
	if err != nil {
		fmt.Fprintf(os.Stderr, "openai client: %v\n", err)
		os.Exit(1)
	}

	ctx := context.Background()
	outputDir := filepath.Join(filepath.Dir(cfg.Home), "workbench")

	var output string

	switch cmd {
	case "replay":
		if *round == "" {
			fatal("usage: workbench replay -round <id> [-n 10] [-desired <key>] [-variant <id>] [-json]")
		}

		result, err := workbench.RunReplay(ctx, conn, oaiClient, workbench.RunReplayParams{
			ActivationRoundID: *round,
			VariantID:         *variant,
			Desired:           *desired,
			N:                 *n,
			EffortOverride:    *effort,
		})
		if err != nil {
			fatal("replay: %v", err)
		}

		if *jsonOut {
			output = result.JSON()
		} else {
			output = result.Text()
		}

	case "create":
		if *round == "" || *desired == "" {
			fatal("usage: workbench create -round <id> -desired <behavior>")
		}

		expID, err := workbench.CreateExperiment(ctx, conn, *round, *desired)
		if err != nil {
			fatal("create: %v", err)
		}
		output = fmt.Sprintf("experiment created: %s\n", expID)

	case "variant":
		if *experiment == "" || *name == "" {
			fatal("usage: workbench variant -experiment <id> -name <name> [-hypothesis <text>] [-patches <file>]")
		}

		var p []workbench.Patch
		if *patches != "" {
			p, err = workbench.LoadPatchesFromFile(*patches)
			if err != nil {
				fatal("load patches: %v", err)
			}
		}

		varID, err := workbench.CreateVariant(ctx, conn, *experiment, *name, *hypothesis, p, *effort, *verbosity)
		if err != nil {
			fatal("variant: %v", err)
		}
		output = fmt.Sprintf("variant created: %s\n", varID)

	case "status":
		output, err = workbench.FormatStatus(ctx, conn, *experiment)
		if err != nil {
			fatal("status: %v", err)
		}

	case "report":
		if *experiment == "" {
			fatal("usage: workbench report -experiment <id>")
		}

		path, err := workbench.WriteReport(ctx, conn, *experiment, outputDir)
		if err != nil {
			fatal("report: %v", err)
		}
		output = fmt.Sprintf("report written to %s\n", path)

	default:
		fatal("unknown command: %s", cmd)
	}

	fmt.Print(output)
}

func fatal(format string, args ...any) {
	fmt.Fprintf(os.Stderr, format+"\n", args...)
	os.Exit(1)
}
