package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/kciuffolo/nik/internal/codex"
	"github.com/kciuffolo/nik/internal/config"
	"github.com/kciuffolo/nik/internal/db"
	"github.com/kciuffolo/nik/internal/llm"
	"github.com/kciuffolo/nik/internal/secrets"
	"github.com/kciuffolo/nik/internal/workbench"
)

func main() {
	if len(os.Args) < 2 {
		fmt.Fprintln(os.Stderr, "usage: workbench <create-experiment|update-experiment|create-experiment-variant|create-experiment-variant-run|update-experiment-variant-run> [flags]")
		os.Exit(1)
	}

	cmd := os.Args[1]
	fs := flag.NewFlagSet("workbench", flag.ContinueOnError)

	activationRoundID := fs.String("activation_round_id", "", "activation round ID")
	experimentID := fs.String("experiment_id", "", "experiment ID")
	experimentVariantID := fs.String("experiment_variant_id", "", "experiment variant ID")
	experimentVariantRunID := fs.String("experiment_variant_run_id", "", "experiment variant run ID")
	name := fs.String("name", "", "variant name")
	desiredOutcome := fs.String("desired_outcome", "", "desired outcome description")
	analysis := fs.String("analysis", "", "trace analysis")
	status := fs.String("status", "", "experiment status")
	hypothesis := fs.String("hypothesis", "", "hypothesis text")
	patches := fs.String("patches", "", "patches JSON file path")
	reasoningEffort := fs.String("reasoning_effort", "", "reasoning effort override")
	verbosity := fs.String("verbosity", "", "verbosity override")
	isDesired := fs.String("is_desired", "", "true or false")
	rationale := fs.String("rationale", "", "rationale for assessment")
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

	time.Local = cfg.TZ()

	conn, err := db.Open(cfg.DBPath(), cfg.TZ())
	if err != nil {
		fmt.Fprintf(os.Stderr, "open db: %v\n", err)
		os.Exit(1)
	}
	defer conn.Close()

	ctx := context.Background()
	outputDir := filepath.Join(filepath.Dir(cfg.Home), "workbench")

	var output string
	var expID string

	switch cmd {
	case "create-experiment":
		if *activationRoundID == "" || *desiredOutcome == "" || *analysis == "" {
			fatal("usage: workbench create-experiment -activation_round_id <id> -desired_outcome '<text>' -analysis '<text>'")
		}

		expID, err = workbench.CreateExperiment(ctx, conn, *activationRoundID, *desiredOutcome, *analysis)
		if err != nil {
			fatal("create-experiment: %v", err)
		}
		output = fmt.Sprintf("experiment created: %s\n", expID)

	case "update-experiment":
		if *experimentID == "" {
			fatal("usage: workbench update-experiment -experiment_id <id> [-status <s>] [-desired_outcome <d>] [-analysis <a>]")
		}

		var sp, dp, ap *string
		if *status != "" {
			sp = status
		}
		if *desiredOutcome != "" {
			dp = desiredOutcome
		}
		if *analysis != "" {
			ap = analysis
		}

		err = workbench.UpdateExperiment(ctx, conn, *experimentID, sp, dp, ap)
		if err != nil {
			fatal("update-experiment: %v", err)
		}
		expID = *experimentID
		output = fmt.Sprintf("experiment %s updated\n", *experimentID)

	case "create-experiment-variant":
		if *experimentID == "" || *name == "" || *hypothesis == "" {
			fatal("usage: workbench create-experiment-variant -experiment_id <id> -name '<name>' -hypothesis '<text>' [-patches <file>] [-reasoning_effort '<effort>'] [-verbosity '<verbosity>']")
		}

		var patchText string
		if *patches != "" {
			data, err := os.ReadFile(*patches)
			if err != nil {
				fatal("read patches file: %v", err)
			}
			patchText = string(data)
		}

		varID, err := workbench.CreateExperimentVariant(ctx, conn, *experimentID, *name, *hypothesis, patchText, *reasoningEffort, *verbosity)
		if err != nil {
			fatal("create-experiment-variant: %v", err)
		}
		expID = *experimentID
		output = fmt.Sprintf("variant created: %s\n", varID)

	case "create-experiment-variant-run":
		if *experimentVariantID == "" || *n < 1 {
			fatal("usage: workbench create-experiment-variant-run -experiment_variant_id <id> -n <count> [-json]")
		}

		var clientOpts []llm.ClientOption
		auth, err := codex.LoadOrLogin("")
		if err == nil {
			clientOpts = append(clientOpts, llm.WithCodex(auth))
		} else {
			store := secrets.New(cfg.Home)
			openaiKey, _ := store.Get("openai_key")
			if openaiKey != "" {
				clientOpts = append(clientOpts, llm.WithAPIKey(openaiKey))
			} else {
				fatal("codex auth failed and no openai_key in secrets store: %v", err)
			}
		}

		v, err := db.ExperimentVariantGet(ctx, conn, *experimentVariantID)
		if err != nil {
			fatal("get variant: %v", err)
		}
		expID = v.ExperimentID

		afterEach := func() {
			workbench.WriteReport(ctx, conn, expID, outputDir)
		}

		runs, err := workbench.CreateExperimentVariantRun(ctx, conn, *experimentVariantID, *n, clientOpts, afterEach)
		if err != nil {
			fatal("create-experiment-variant-run: %v", err)
		}

		if *jsonOut {
			data, _ := json.MarshalIndent(runs, "", "  ")
			output = string(data)
		} else {
			output = formatRuns(runs)
		}

	case "update-experiment-variant-run":
		if *experimentVariantRunID == "" || *isDesired == "" || *rationale == "" {
			fatal("usage: workbench update-experiment-variant-run -experiment_variant_run_id <id> -is_desired true|false -rationale '<text>'")
		}

		desired := *isDesired == "true" || *isDesired == "yes" || *isDesired == "1"

		expID, err = workbench.UpdateExperimentVariantRun(ctx, conn, *experimentVariantRunID, desired, *rationale)
		if err != nil {
			fatal("update-experiment-variant-run: %v", err)
		}

		tag := "not desired"
		if desired {
			tag = "desired"
		}
		output = fmt.Sprintf("run %s marked as %s\n", *experimentVariantRunID, tag)

	default:
		fatal("unknown command: %s", cmd)
	}

	if expID != "" {
		path, err := workbench.WriteReport(ctx, conn, expID, outputDir)
		if err != nil {
			fmt.Fprintf(os.Stderr, "auto-render report: %v\n", err)
		} else {
			fmt.Fprintf(os.Stderr, "report: %s\n", path)
		}
	}

	fmt.Print(output)
}

func formatRuns(runs []db.ExperimentVariantRun) string {
	var b strings.Builder

	for i, r := range runs {
		key := toolCallKey(r.ToolCalls)
		fmt.Fprintf(&b, "  run %d: %s\n", i+1, key)
	}

	if len(runs) > 1 {
		counts := map[string]int{}
		for _, r := range runs {
			counts[toolCallKey(r.ToolCalls)]++
		}
		total := len(runs)
		fmt.Fprintf(&b, "\nDISTRIBUTION:\n")
		for k, c := range counts {
			fmt.Fprintf(&b, "  %s: %d/%d (%.0f%%)\n", k, c, total, float64(c)/float64(total)*100)
		}
	}

	return b.String()
}

func toolCallKey(raw string) string {
	var calls []struct{ Name string }

	err := json.Unmarshal([]byte(raw), &calls)
	if err != nil || len(calls) == 0 {
		return "no_tools"
	}

	names := make([]string, len(calls))
	for i, c := range calls {
		names[i] = c.Name
	}
	return strings.Join(names, "+")
}

func fatal(format string, args ...any) {
	fmt.Fprintf(os.Stderr, format+"\n", args...)
	os.Exit(1)
}
