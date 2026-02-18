package alarms

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/kciuffolo/nik/internal/db"
	"github.com/kciuffolo/nik/internal/llm"
)

var alarmToolDef = llm.ToolDef{
	Name:        "alarm",
	Description: "Set an alarm to wake yourself up at a later time and do something. When it fires you receive the conversation context and can use any of your tools. For recurring alarms, provide a recurrence pattern in natural language.",
	Parameters: map[string]any{
		"type": "object",
		"properties": map[string]any{
			"origin_contact_id": map[string]any{
				"type":        "string",
				"description": "Canonical contact_id for who requested the alarm.",
			},
			"goal": map[string]any{
				"type":        "string",
				"description": "A note to your future self describing what to do and why.",
			},
			"fire_at": map[string]any{
				"type":        "string",
				"description": "RFC3339 timestamp for when the alarm should first fire, e.g. '2026-03-15T14:30:00Z'.",
			},
			"recurrence": map[string]any{
				"type":        "string",
				"description": "Natural language recurrence pattern. Examples: 'every day at 9am', 'every Sunday at 7pm', 'every weekday morning', 'first of every month'. Omit or leave empty for one-shot alarms.",
			},
		},
		"required":             []string{"origin_contact_id", "goal", "fire_at", "recurrence"},
		"additionalProperties": false,
	},
}

var updateAlarmToolDef = llm.ToolDef{
	Name:        "update_alarm",
	Description: "Update an existing alarm. Use to edit the goal, change the recurrence pattern, reschedule the next fire time, or attach a note to the current occurrence after acting on it.",
	Parameters: map[string]any{
		"type": "object",
		"properties": map[string]any{
			"alarm_id": map[string]any{
				"type":        "string",
				"description": "The alarm ID to update.",
			},
			"goal": map[string]any{
				"type":        "string",
				"description": "Updated goal/instructions. Omit to keep current.",
			},
			"recurrence": map[string]any{
				"type":        "string",
				"description": "Updated recurrence pattern in natural language. Omit to keep current.",
			},
			"next_fire_at": map[string]any{
				"type":        "string",
				"description": "RFC3339 timestamp for the next fire time. Use to reschedule or skip an occurrence.",
			},
			"occurrence_note": map[string]any{
				"type":        "string",
				"description": "Short note about what happened during the current occurrence. Only meaningful when called during an alarm activation.",
			},
		},
		"required":             []string{"alarm_id", "goal", "recurrence", "next_fire_at", "occurrence_note"},
		"additionalProperties": false,
	},
}

var cancelAlarmToolDef = llm.ToolDef{
	Name:        "cancel_alarm",
	Description: "Cancel an active alarm. It will not fire again. Works for both one-shot and recurring alarms.",
	Parameters: map[string]any{
		"type": "object",
		"properties": map[string]any{
			"alarm_id": map[string]any{
				"type":        "string",
				"description": "The alarm ID to cancel.",
			},
		},
		"required":             []string{"alarm_id"},
		"additionalProperties": false,
	},
}

func BuildTools(svc *Service) []llm.Tool {
	return []llm.Tool{
		{
			Def:     alarmToolDef,
			Handler: alarmHandler(svc),
		},
		{
			Def:     updateAlarmToolDef,
			Handler: updateAlarmHandler(svc),
		},
		{
			Def:     cancelAlarmToolDef,
			Handler: cancelAlarmHandler(svc),
		},
	}
}

func alarmHandler(svc *Service) llm.ToolExecutor {
	return func(ctx context.Context, call llm.ToolCall) (string, error) {
		var args struct {
			OriginContactID string `json:"origin_contact_id"`
			Goal            string `json:"goal"`
			FireAt          string `json:"fire_at"`
			Recurrence      string `json:"recurrence"`
		}

		err := json.Unmarshal([]byte(call.Arguments), &args)
		if err != nil {
			return fmt.Sprintf(`{"error":%q}`, err.Error()), nil
		}
		if args.Goal == "" {
			return `{"error":"empty goal"}`, nil
		}
		if args.OriginContactID == "" {
			return `{"error":"empty origin_contact_id"}`, nil
		}
		if args.FireAt == "" {
			return `{"error":"empty fire_at"}`, nil
		}

		originConversationID := ""
		if meta, ok := ctx.Value("meta").(map[string]string); ok {
			originConversationID = meta["conversation_id"]
		}

		alarm, err := svc.CreateAlarm(ctx, args.OriginContactID, originConversationID, args.Goal, args.Recurrence, args.FireAt)
		if err != nil {
			return fmt.Sprintf(`{"error":%q}`, err.Error()), nil
		}

		result := map[string]any{
			"created":      true,
			"id":           alarm.ID,
			"goal":         alarm.Goal,
			"next_fire_at": alarm.NextFireAt.Time.Format(time.RFC3339),
		}
		if alarm.Recurrence.Valid {
			result["recurrence"] = alarm.Recurrence.String
		}

		out, _ := json.Marshal(result)
		return string(out), nil
	}
}

func updateAlarmHandler(svc *Service) llm.ToolExecutor {
	return func(ctx context.Context, call llm.ToolCall) (string, error) {
		var args struct {
			AlarmID        string `json:"alarm_id"`
			Goal           string `json:"goal"`
			Recurrence     string `json:"recurrence"`
			NextFireAt     string `json:"next_fire_at"`
			OccurrenceNote string `json:"occurrence_note"`
		}

		err := json.Unmarshal([]byte(call.Arguments), &args)
		if err != nil {
			return fmt.Sprintf(`{"error":%q}`, err.Error()), nil
		}
		if args.AlarmID == "" {
			return `{"error":"empty alarm_id"}`, nil
		}

		hasUpdate := args.Goal != "" || args.Recurrence != "" || args.NextFireAt != ""
		if hasUpdate {
			p := db.AlarmUpdateParams{}
			if args.Goal != "" {
				p.Goal = &args.Goal
			}
			if args.Recurrence != "" {
				p.Recurrence = &args.Recurrence
			}
			if args.NextFireAt != "" {
				t, err := time.Parse(time.RFC3339, args.NextFireAt)
				if err != nil {
					return fmt.Sprintf(`{"error":"parse next_fire_at: %s"}`, err.Error()), nil
				}
				p.NextFireAt = t
			}

			err = svc.UpdateAlarm(ctx, args.AlarmID, p)
			if err != nil {
				return fmt.Sprintf(`{"error":%q}`, err.Error()), nil
			}
		}

		if args.OccurrenceNote != "" {
			occurrenceID := ""
			if meta, ok := ctx.Value("meta").(map[string]string); ok {
				occurrenceID = meta["occurrence_id"]
			}
			if occurrenceID != "" {
				err = svc.UpdateOccurrenceNote(ctx, occurrenceID, args.OccurrenceNote)
				if err != nil {
					return fmt.Sprintf(`{"error":%q}`, err.Error()), nil
				}
			}
		}

		return `{"updated":true}`, nil
	}
}

func cancelAlarmHandler(svc *Service) llm.ToolExecutor {
	return func(ctx context.Context, call llm.ToolCall) (string, error) {
		var args struct {
			AlarmID string `json:"alarm_id"`
		}

		err := json.Unmarshal([]byte(call.Arguments), &args)
		if err != nil {
			return fmt.Sprintf(`{"error":%q}`, err.Error()), nil
		}
		if args.AlarmID == "" {
			return `{"error":"empty alarm_id"}`, nil
		}

		err = svc.Cancel(ctx, args.AlarmID)
		if err != nil {
			return fmt.Sprintf(`{"error":%q}`, err.Error()), nil
		}

		return `{"cancelled":true}`, nil
	}
}
