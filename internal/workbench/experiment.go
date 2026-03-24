package workbench

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/kciuffolo/nik/internal/db"
	"github.com/kciuffolo/nik/internal/id"
)

type ExperimentStatus struct {
	ExperimentID string
	Status       string
	Variants     []VariantStatus
}

type VariantStatus struct {
	ID           string
	Name         string
	Status       string
	Hypothesis   string
	Patches      []Patch
	RunCount     int
	DesiredCount int
	MissCount    int
	Rate         float64
}

func CreateExperiment(ctx context.Context, conn *sql.DB, activationRoundID, desiredOutcome string) (string, error) {
	expID := id.V7()

	err := db.ExperimentInsert(ctx, conn, db.ExperimentInsertParams{
		ID:                expID,
		ActivationRoundID: activationRoundID,
		Status:            "analysis",
		DesiredOutcome:    desiredOutcome,
	})
	if err != nil {
		return "", err
	}

	baselineID := id.V7()

	err = db.ExperimentVariantInsert(ctx, conn, db.ExperimentVariantInsertParams{
		ID:           baselineID,
		ExperimentID: expID,
		Name:         "baseline",
		Status:       "proposed",
		Patches:      "[]",
	})
	if err != nil {
		return "", fmt.Errorf("create baseline variant: %w", err)
	}

	return expID, nil
}

func CreateVariant(ctx context.Context, conn *sql.DB, experimentID, name, hypothesis string, patches []Patch, effort, verbosity string) (string, error) {
	patchJSON, err := json.Marshal(patches)
	if err != nil {
		return "", fmt.Errorf("marshal patches: %w", err)
	}

	variantID := id.V7()

	err = db.ExperimentVariantInsert(ctx, conn, db.ExperimentVariantInsertParams{
		ID:              variantID,
		ExperimentID:    experimentID,
		Name:            name,
		Status:          "proposed",
		Hypothesis:      hypothesis,
		Patches:         string(patchJSON),
		ReasoningEffort: effort,
		Verbosity:       verbosity,
	})
	if err != nil {
		return "", err
	}

	return variantID, nil
}

func RecordRun(ctx context.Context, conn *sql.DB, variantID string, result ReplayResult, isDesired bool) (string, error) {
	toolCallsJSON, err := json.Marshal(result.ToolCalls)
	if err != nil {
		return "", fmt.Errorf("marshal tool calls: %w", err)
	}

	summariesJSON := db.MarshalStringSlice(result.ReasoningSummaries)

	runID, err := db.ExperimentRunInsert(ctx, conn, db.ExperimentRunInsertParams{
		ExperimentVariantID: variantID,
		ToolCalls:           string(toolCallsJSON),
		ModelOutput:         result.ModelOutput,
		ReasoningSummaries:  summariesJSON,
		IsDesired:           isDesired,
		InputTokens:         result.InputTokens,
		OutputTokens:        result.OutputTokens,
		CachedTokens:        result.CachedTokens,
		ReasoningTokens:     result.ReasoningTokens,
	})
	if err != nil {
		return "", err
	}

	variant, err := db.ExperimentVariantGet(ctx, conn, variantID)
	if err != nil {
		return "", fmt.Errorf("get variant for count update: %w", err)
	}

	newRunCount := variant.RunCount + 1
	newDesiredCount := variant.DesiredCount
	if isDesired {
		newDesiredCount++
	}

	err = db.ExperimentVariantUpdate(ctx, conn, db.ExperimentVariantUpdateParams{
		ID:           variantID,
		RunCount:     &newRunCount,
		DesiredCount: &newDesiredCount,
	})
	if err != nil {
		return "", fmt.Errorf("update variant counts: %w", err)
	}

	return runID, nil
}

func GetStatus(ctx context.Context, conn *sql.DB, experimentID string) (ExperimentStatus, error) {
	exp, err := db.ExperimentGet(ctx, conn, experimentID)
	if err != nil {
		return ExperimentStatus{}, err
	}

	variants, err := db.ExperimentVariantList(ctx, conn, exp.ID)
	if err != nil {
		return ExperimentStatus{}, err
	}

	status := ExperimentStatus{
		ExperimentID: exp.ID,
		Status:       exp.Status,
	}

	for _, v := range variants {
		var patches []Patch

		err = json.Unmarshal([]byte(v.Patches), &patches)
		if err != nil {
			return ExperimentStatus{}, fmt.Errorf("unmarshal patches for variant %s: %w", v.ID, err)
		}

		missCount := v.RunCount - v.DesiredCount
		var rate float64
		if v.RunCount > 0 {
			rate = float64(v.DesiredCount) / float64(v.RunCount) * 100
		}

		status.Variants = append(status.Variants, VariantStatus{
			ID:           v.ID,
			Name:         v.Name,
			Status:       v.Status,
			Hypothesis:   v.Hypothesis,
			Patches:      patches,
			RunCount:     v.RunCount,
			DesiredCount: v.DesiredCount,
			MissCount:    missCount,
			Rate:         rate,
		})
	}

	return status, nil
}

func VariantPatches(ctx context.Context, conn *sql.DB, variantID string) ([]Patch, error) {
	v, err := db.ExperimentVariantGet(ctx, conn, variantID)
	if err != nil {
		return nil, err
	}

	return ParsePatches(v.Patches)
}

func FormatStatus(ctx context.Context, conn *sql.DB, experimentID string) (string, error) {
	status, err := GetStatus(ctx, conn, experimentID)
	if err != nil {
		return "", err
	}

	var b strings.Builder

	fmt.Fprintf(&b, "Experiment %s — %s\n\n", id.Shorten(status.ExperimentID), status.Status)

	for _, v := range status.Variants {
		if v.RunCount > 0 {
			fmt.Fprintf(&b, "  %-20s %d runs, %d hit, %d miss (%.0f%%)\n",
				v.Name, v.RunCount, v.DesiredCount, v.MissCount, v.Rate)
		} else {
			fmt.Fprintf(&b, "  %-20s pending\n", v.Name)
		}
	}

	return b.String(), nil
}

func LoadPatchesFromFile(path string) ([]Patch, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read patches file: %w", err)
	}

	var patches []Patch

	err = json.Unmarshal(data, &patches)
	if err != nil {
		return nil, fmt.Errorf("parse patches file: %w", err)
	}

	return patches, nil
}
