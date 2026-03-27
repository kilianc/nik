package db

import (
	"context"
	"fmt"
	"time"

	"github.com/kciuffolo/nik/internal/id"
	"github.com/kciuffolo/nik/internal/queries"
)

type ActivationRound struct {
	ID                 string
	ActivationID       string
	Round              int
	UserInput          string
	ModelOutput        string
	Messages           string
	ReasoningSummaries []string
	InputTokens        int64
	OutputTokens       int64
	CachedTokens       int64
	ReasoningTokens    int64
	CreatedAt          time.Time
}

func scanActivationRound(s scanner) (ActivationRound, error) {
	var r ActivationRound
	var summaries any

	err := s.Scan(
		&r.ID,
		&r.ActivationID,
		&r.Round,
		&r.UserInput,
		&r.ModelOutput,
		&r.Messages,
		&summaries,
		&r.InputTokens,
		&r.OutputTokens,
		&r.CachedTokens,
		&r.ReasoningTokens,
		&r.CreatedAt,
	)
	if err != nil {
		return r, err
	}

	r.ReasoningSummaries, err = scanStringSlice(summaries)
	if err != nil {
		return r, fmt.Errorf("scan reasoning_summaries: %w", err)
	}

	return r, nil
}

func ActivationRoundGet(ctx context.Context, db DBTX, roundID string) (ActivationRound, error) {
	r, err := scanActivationRound(db.QueryRowContext(ctx, queries.ActivationRoundGet, roundID))
	if err != nil {
		return r, fmt.Errorf("get activation_round %s: %w", roundID, err)
	}

	return r, nil
}

func ActivationRoundList(ctx context.Context, db DBTX, activationID string, beforeRound *int) ([]ActivationRound, error) {
	rows, err := db.QueryContext(ctx, queries.ActivationRoundList, activationID, beforeRound)
	if err != nil {
		return nil, fmt.Errorf("list activation_round for %s: %w", activationID, err)
	}
	defer rows.Close()

	var rounds []ActivationRound

	for rows.Next() {
		r, err := scanActivationRound(rows)
		if err != nil {
			return nil, fmt.Errorf("scan activation_round: %w", err)
		}

		rounds = append(rounds, r)
	}

	return rounds, rows.Err()
}

type ActivationRoundInsertParams struct {
	ActivationID       string
	Round              int
	UserInput          string
	ModelOutput        string
	Messages           string
	ReasoningSummaries []string
	InputTokens        int64
	OutputTokens       int64
	CachedTokens       int64
	ReasoningTokens    int64
}

func ActivationRoundInsert(ctx context.Context, db DBTX, p ActivationRoundInsertParams) (string, error) {
	roundID := id.V7()

	_, err := db.ExecContext(ctx, queries.ActivationRoundInsert,
		roundID,
		p.ActivationID,
		p.Round,
		p.UserInput,
		p.ModelOutput,
		p.Messages,
		MarshalStringSlice(p.ReasoningSummaries),
		p.InputTokens,
		p.OutputTokens,
		p.CachedTokens,
		p.ReasoningTokens,
	)
	if err != nil {
		return "", fmt.Errorf("insert activation_round %s round %d: %w", p.ActivationID, p.Round, err)
	}

	return roundID, nil
}
