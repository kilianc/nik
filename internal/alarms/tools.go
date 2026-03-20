package alarms

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

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
				"description": "Date and time for when the alarm should fire, format YYYY-MM-DD HH:MM.",
			},
			"timezone": map[string]any{
				"type":        "string",
				"description": "IANA timezone for interpreting fire_at (e.g. Europe/Rome). Defaults to system timezone when omitted.",
			},
			"recurrence": map[string]any{
				"type":        "string",
				"description": "Natural language recurrence pattern. Examples: 'every day at 9am', 'every Sunday at 7pm', 'every weekday morning', 'first of every month'. Omit or leave empty for one-shot alarms.",
			},
		},
		"required":             []string{"origin_contact_id", "goal", "fire_at", "timezone", "recurrence"},
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
				"description": "Date and time for the next fire, format YYYY-MM-DD HH:MM. Use to reschedule or skip an occurrence.",
			},
			"timezone": map[string]any{
				"type":        "string",
				"description": "IANA timezone for interpreting next_fire_at (e.g. Europe/Rome). Defaults to system timezone when omitted.",
			},
			"occurrence_note": map[string]any{
				"type":        "string",
				"description": "Short note about what happened during the current occurrence. Only meaningful when called during an alarm activation.",
			},
		},
		"required":             []string{"alarm_id", "goal", "recurrence", "next_fire_at", "timezone", "occurrence_note"},
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

func (s *Service) parseLocalTime(raw, tz string) (time.Time, error) {
	loc := time.UTC
	if s.cfg != nil {
		loc = s.cfg.TZ()
	}

	if tz != "" {
		parsed, err := time.LoadLocation(tz)
		if err != nil {
			return time.Time{}, fmt.Errorf("invalid timezone %q: %w", tz, err)
		}
		loc = parsed
	}

	return time.ParseInLocation("2006-01-02 15:04", raw, loc)
}

func alarmHandler(svc *Service) llm.ToolExecutor {
	return func(ctx context.Context, call llm.ToolCall) (string, error) {
		var args struct {
			OriginContactID string `json:"origin_contact_id"`
			Goal            string `json:"goal"`
			FireAt          string `json:"fire_at"`
			Timezone        string `json:"timezone"`
			Recurrence      string `json:"recurrence"`
		}

		err := json.Unmarshal([]byte(call.Arguments), &args)
		if err != nil {
			return llm.ToolError(err), nil
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

		fireAt, err := svc.parseLocalTime(args.FireAt, args.Timezone)
		if err != nil {
			return llm.ToolErrorf("parse fire_at: %s", err.Error()), nil
		}

		var originConversationID string
		if meta, ok := ctx.Value("meta").(map[string]string); ok {
			originConversationID = meta["conversation_id"]
		}

		alarm, err := svc.CreateAlarm(ctx, args.OriginContactID, originConversationID, args.Goal, args.Recurrence, fireAt)
		if err != nil {
			return llm.ToolError(err), nil
		}

		result := map[string]any{
			"created":      true,
			"id":           alarm.ID,
			"goal":         alarm.Goal,
			"next_fire_at": alarm.NextFireAt.Time.Format("Jan 2, 2006 3:04 PM"),
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
			Timezone       string `json:"timezone"`
			OccurrenceNote string `json:"occurrence_note"`
		}

		err := json.Unmarshal([]byte(call.Arguments), &args)
		if err != nil {
			return llm.ToolError(err), nil
		}
		if args.AlarmID == "" {
			return `{"error":"empty alarm_id"}`, nil
		}

		alarmID, err := svc.ResolveAlarmID(ctx, args.AlarmID)
		if err != nil {
			return llm.ToolError(err), nil
		}

		p := UpdateParams{}
		if args.Goal != "" {
			p.Goal = &args.Goal
		}
		if args.Recurrence != "" {
			p.Recurrence = &args.Recurrence
		}
		if args.NextFireAt != "" {
			t, parseErr := svc.parseLocalTime(args.NextFireAt, args.Timezone)
			if parseErr != nil {
				return llm.ToolErrorf("parse next_fire_at: %s", parseErr.Error()), nil
			}
			p.NextFireAt = &t
		}
		p.Note = args.OccurrenceNote

		err = svc.Update(ctx, alarmID, p)
		if err != nil {
			return llm.ToolError(err), nil
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
			return llm.ToolError(err), nil
		}
		if args.AlarmID == "" {
			return `{"error":"empty alarm_id"}`, nil
		}

		alarmID, err := svc.ResolveAlarmID(ctx, args.AlarmID)
		if err != nil {
			return llm.ToolError(err), nil
		}

		err = svc.Cancel(ctx, alarmID)
		if err != nil {
			return llm.ToolError(err), nil
		}

		return `{"cancelled":true}`, nil
	}
}
