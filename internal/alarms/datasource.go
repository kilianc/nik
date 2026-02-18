package alarms

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/kciuffolo/nik/internal/brain"
	"github.com/kciuffolo/nik/internal/db"
	"github.com/kciuffolo/nik/internal/messaging"
)

type DataSource struct {
	svc     *Service
	msgsSvc *messaging.Service
}

func NewDataSource(svc *Service, msgsSvc *messaging.Service) *DataSource {
	return &DataSource{svc: svc, msgsSvc: msgsSvc}
}

func (d *DataSource) Check(ctx context.Context) ([]brain.DataSourceOutput, error) {
	alarms, err := d.svc.DueAlarms(ctx)
	if err != nil {
		return nil, fmt.Errorf("poll due alarms: %w", err)
	}

	var outputs []brain.DataSourceOutput
	for _, a := range alarms {
		alarm := a
		alreadyFired := alarm.LastFiredAt.Valid && !alarm.LastFiredAt.Time.Before(alarm.NextFireAt.Time)

		msgs := d.conversationContext(ctx, alarm)
		senderLabels := d.msgsSvc.SenderLabels(ctx, msgs)
		requesterName := ""
		if alarm.OriginContactID.Valid && alarm.OriginContactID.String != "" {
			requesterName = d.msgsSvc.ContactLabel(ctx, alarm.OriginContactID.String)
		}

		occurrences := d.recentOccurrences(ctx, alarm.ID)
		lines := formatAlarm(alarm, requesterName, msgs, senderLabels, occurrences, alreadyFired)

		meta := map[string]string{
			"source":    "alarm",
			"source_id": alarm.ID,
			"alarm_id":  alarm.ID,
		}

		hasOriginConversation := alarm.OriginConversationID.Valid && alarm.OriginConversationID.String != ""
		hasOriginContact := alarm.OriginContactID.Valid && alarm.OriginContactID.String != ""
		if hasOriginConversation {
			meta["conversation_id"] = alarm.OriginConversationID.String
		}
		if hasOriginContact {
			meta["contact_id"] = alarm.OriginContactID.String
		}

		out := brain.DataSourceOutput{
			Lines: lines,
			Meta:  meta,
		}

		if !alreadyFired {
			out.Processing = func(ctx context.Context) error {
				slog.Info("alarm fired", "pkg", "alarms", "id", alarm.ID, "goal", alarm.Goal,
					"recurrence", alarm.Recurrence.String)

				occ, err := d.svc.LogOccurrence(ctx, alarm.ID)
				if err != nil {
					return fmt.Errorf("log occurrence: %w", err)
				}

				if m, ok := ctx.Value("meta").(map[string]string); ok {
					m["occurrence_id"] = occ.ID
				}

				return d.svc.ClaimAlarm(ctx, alarm.ID)
			}
		}

		outputs = append(outputs, out)
	}

	return outputs, nil
}

func (d *DataSource) conversationContext(ctx context.Context, alarm Alarm) []db.Message {
	if !alarm.OriginConversationID.Valid || alarm.OriginConversationID.String == "" {
		return nil
	}

	msgs, err := d.msgsSvc.MessagesAround(ctx, alarm.OriginConversationID.String, alarm.CreatedAt, 10)
	if err != nil {
		slog.Warn("alarm context: get messages", "pkg", "alarms", "id", alarm.ID, "error", err)
		return nil
	}

	return msgs
}

func (d *DataSource) recentOccurrences(ctx context.Context, alarmID string) []AlarmOccurrence {
	occurrences, err := d.svc.OccurrenceSummary(ctx, alarmID, 5)
	if err != nil {
		slog.Warn("alarm occurrences", "pkg", "alarms", "id", alarmID, "error", err)
		return nil
	}
	return occurrences
}

func formatAlarm(a Alarm, requesterName string, msgs []db.Message, senderLabels map[string]string, occurrences []AlarmOccurrence, alreadyFired bool) []string {
	recurring := a.Recurrence.Valid && a.Recurrence.String != ""

	header := "[Alarm fired]"
	if alreadyFired {
		header = "[Alarm pending reschedule]"
	} else if recurring {
		header = "[Recurring alarm fired]"
	}

	lines := []string{
		header,
		fmt.Sprintf("Goal: %s", a.Goal),
	}

	if recurring {
		lines = append(lines, fmt.Sprintf("Recurrence: %s", a.Recurrence.String))
	}

	lines = append(lines, fmt.Sprintf("alarm_id: %s", a.ID))

	if requesterName != "" {
		lines = append(lines, fmt.Sprintf("Requested by: %s", requesterName))
	}

	if a.OriginContactID.Valid && a.OriginContactID.String != "" {
		lines = append(lines, fmt.Sprintf("origin_contact_id: %s", a.OriginContactID.String))
	}

	if a.OriginConversationID.Valid && a.OriginConversationID.String != "" {
		lines = append(lines, fmt.Sprintf("origin_conversation_id: %s", a.OriginConversationID.String))
	}

	if len(occurrences) > 0 {
		lines = append(lines, "", "## Recent occurrences")
		for _, o := range occurrences {
			entry := fmt.Sprintf("- %s", o.FiredAt.Format("2006-01-02 15:04"))
			if o.Note.Valid && o.Note.String != "" {
				entry += " — " + o.Note.String
			}
			lines = append(lines, entry)
		}
	}

	if len(msgs) > 0 {
		lines = append(lines, "", "## Conversation context", "")
		for _, m := range msgs {
			lines = append(lines, formatContextMessage(m, senderLabels))
		}
	}

	switch {
	case alreadyFired && recurring:
		lines = append(lines, "", "This recurring alarm already fired but was not rescheduled. Call update_alarm with next_fire_at set to the next occurrence based on the recurrence pattern.")
	case alreadyFired:
		lines = append(lines, "", "This alarm already fired but was not resolved. Call cancel_alarm to dismiss it.")
	case recurring:
		lines = append(lines, "", "Act on this now. After acting, call update_alarm with an occurrence_note describing what you did and next_fire_at set to the next occurrence based on the recurrence pattern.")
	default:
		lines = append(lines, "", "Act on this now. After acting, call cancel_alarm to dismiss it.")
	}

	return lines
}

func formatContextMessage(m db.Message, senderLabels map[string]string) string {
	return messaging.FormatMessageLine(m, senderLabels[m.ID])
}
