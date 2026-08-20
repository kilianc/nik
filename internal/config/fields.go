package config

import (
	"errors"
	"fmt"
	"log/slog"
	"strconv"
	"time"
)

// Snapshot is the readable configuration, as data. The brain tool marshals it
// and so does the API — one description of what "the config" means, rather
// than two that drift.
func Snapshot(cfg *Config) map[string]any {
	return map[string]any{
		"models": map[string]any{
			"main": map[string]any{
				"model":            cfg.Models.Main.Model,
				"backend":          cfg.Models.Main.Backend,
				"reasoning_effort": cfg.Models.Main.ReasoningEffort,
				"verbosity":        cfg.Models.Main.Verbosity,
			},
			"task": map[string]any{
				"model":            cfg.Models.Task.Model,
				"backend":          cfg.Models.Task.Backend,
				"reasoning_effort": cfg.Models.Task.ReasoningEffort,
				"verbosity":        cfg.Models.Task.Verbosity,
			},
			"recall": map[string]any{
				"model":            cfg.Models.Recall.Model,
				"backend":          cfg.Models.Recall.Backend,
				"reasoning_effort": cfg.Models.Recall.ReasoningEffort,
				"verbosity":        cfg.Models.Recall.Verbosity,
			},
		},
		"task": map[string]any{
			"max_rounds": cfg.Task.MaxRoundsOrDefault(),
			"timeout":    cfg.Task.TimeoutOrDefault().String(),
		},
		"shell": map[string]any{
			"docker_image": cfg.Shell.DockerImage,
		},
		"max_history":                 cfg.MaxHistory,
		"timezone":                    cfg.Timezone,
		"location":                    cfg.Location,
		"allow_conversation_ids":      cfg.AllowConversationIDs.toMap(),
		"privileged_conversation_ids": cfg.PrivilegedConversationIDs.toMap(),
	}
}

var readOnlyFields = map[string]bool{
	"privileged_conversation_ids": true,
}

// Errors SetField returns for a caller's mistake rather than nik's. They are
// separated so an API can answer 400 instead of 500, which the brain tool
// never needed but a person typing into a console very much does.
var (
	ErrUnknownField  = errors.New("unknown field")
	ErrReadOnlyField = errors.New("field is read-only")
	ErrInvalidValue  = errors.New("invalid value")
)

// SetField changes one field and saves. The config is live-reloaded from
// disk, so a successful return means the running daemon will pick it up
// without anything being restarted.
func SetField(cfg *Config, field, value string) error {
	if field == "" {
		return fmt.Errorf("%w: empty", ErrUnknownField)
	}

	if readOnlyFields[field] {
		return fmt.Errorf("%w: %s", ErrReadOnlyField, field)
	}

	previous := *cfg

	switch field {
	case "timezone":
		cfg.Timezone = value
	case "location":
		cfg.Location = value
	case "max_history":
		n, err := strconv.Atoi(value)
		if err != nil {
			return fmt.Errorf("%w: max_history %q is not a number", ErrInvalidValue, value)
		}
		cfg.MaxHistory = n
	case "task.max_rounds":
		n, err := strconv.Atoi(value)
		if err != nil {
			return fmt.Errorf("%w: task.max_rounds %q is not a number", ErrInvalidValue, value)
		}
		if n < 1 {
			return fmt.Errorf("%w: task.max_rounds must be >= 1", ErrInvalidValue)
		}
		cfg.Task.MaxRounds = n
	case "task.timeout":
		d, err := time.ParseDuration(value)
		if err != nil {
			return fmt.Errorf("%w: task.timeout %q is not a duration", ErrInvalidValue, value)
		}
		if d < time.Minute {
			return fmt.Errorf("%w: task.timeout must be >= 1m", ErrInvalidValue)
		}
		cfg.Task.Timeout = d
	case "models.main.model":
		cfg.Models.Main.Model = value
	case "models.main.reasoning_effort":
		if !isValidReasoningEffort(value) {
			return fmt.Errorf("%w: models.main.reasoning_effort %q (none, minimal, low, medium, high, xhigh, max)", ErrInvalidValue, value)
		}
		cfg.Models.Main.ReasoningEffort = value
	case "models.main.verbosity":
		if !isValidVerbosity(value) {
			return fmt.Errorf("%w: models.main.verbosity %q (low, medium, high, or empty)", ErrInvalidValue, value)
		}
		cfg.Models.Main.Verbosity = value
	case "models.task.model":
		cfg.Models.Task.Model = value
	case "models.task.reasoning_effort":
		if !isValidReasoningEffort(value) {
			return fmt.Errorf("%w: models.task.reasoning_effort %q (none, minimal, low, medium, high, xhigh, max)", ErrInvalidValue, value)
		}
		cfg.Models.Task.ReasoningEffort = value
	case "models.task.verbosity":
		if !isValidVerbosity(value) {
			return fmt.Errorf("%w: models.task.verbosity %q (low, medium, high, or empty)", ErrInvalidValue, value)
		}
		cfg.Models.Task.Verbosity = value
	case "models.recall.model":
		cfg.Models.Recall.Model = value
	case "models.recall.reasoning_effort":
		if !isValidReasoningEffort(value) {
			return fmt.Errorf("%w: models.recall.reasoning_effort %q (none, minimal, low, medium, high, xhigh, max)", ErrInvalidValue, value)
		}
		cfg.Models.Recall.ReasoningEffort = value
	case "models.main.backend":
		cfg.Models.Main.Backend = value
	case "models.task.backend":
		cfg.Models.Task.Backend = value
	case "models.recall.backend":
		cfg.Models.Recall.Backend = value
	case "shell.docker_image":
		cfg.Shell.DockerImage = value
	case "models.recall.verbosity":
		if !isValidVerbosity(value) {
			return fmt.Errorf("%w: models.recall.verbosity %q (low, medium, high, or empty)", ErrInvalidValue, value)
		}
		cfg.Models.Recall.Verbosity = value
	default:
		return fmt.Errorf("%w: %s", ErrUnknownField, field)
	}

	err := validateConfig(*cfg)
	if err != nil {
		*cfg = previous
		return fmt.Errorf("%w: %s", ErrInvalidValue, err)
	}

	err = cfg.Save(cfg.ConfigPath())
	if err != nil {
		*cfg = previous
		return err
	}

	slog.Info("config set", "pkg", "config", "field", field, "value", value)

	return nil
}
