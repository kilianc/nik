package alarms

import (
	"context"
	"fmt"
	"log/slog"
	"strconv"
	"strings"
	"time"

	"github.com/kciuffolo/nik/internal/config"
	"github.com/kciuffolo/nik/internal/db"
)

type coreSchedule struct {
	prefix     string
	goal       string
	recurrence string
	configTime func(*config.Config) string
	offset     time.Duration
}

var coreSchedules = []coreSchedule{
	{prefix: "[NIK_JOURNAL]", goal: "[NIK_JOURNAL] End of day journal -- load journal skill", recurrence: "every day", configTime: func(c *config.Config) string { return c.JournalTime }},
	{prefix: "[NIK_DREAM_1]", goal: "[NIK_DREAM_1] Drift -- load dream skill", recurrence: "every day", configTime: func(c *config.Config) string { return c.DreamStart }},
	{prefix: "[NIK_DREAM_2]", goal: "[NIK_DREAM_2] Weave -- load dream skill", recurrence: "every day", configTime: func(c *config.Config) string { return c.DreamStart }, offset: time.Hour},
	{prefix: "[NIK_DREAM_3]", goal: "[NIK_DREAM_3] Depths -- load dream skill", recurrence: "every day", configTime: func(c *config.Config) string { return c.DreamStart }, offset: 2 * time.Hour},
	{prefix: "[NIK_DREAM_4]", goal: "[NIK_DREAM_4] Crystallize -- load dream skill", recurrence: "every day", configTime: func(c *config.Config) string { return c.DreamStart }, offset: 3 * time.Hour},
	{prefix: "[NIK_DREAM_5]", goal: "[NIK_DREAM_5] Wake -- load dream skill", recurrence: "every day", configTime: func(c *config.Config) string { return c.DreamStart }, offset: 4 * time.Hour},
	{prefix: "[NIK_BRIEFING]", goal: "[NIK_BRIEFING] Morning briefing -- load briefing skill", recurrence: "every day", configTime: func(c *config.Config) string { return c.BriefingTime }},
	{prefix: "[NIK_DIAGNOSTIC]", goal: "[NIK_DIAGNOSTIC] System diagnostic -- load diagnostic skill", recurrence: "every day", configTime: func(c *config.Config) string { return c.DiagnosticTime }},
}

const enforceCooldown = 30 * time.Minute

// CoreAlarmEnforcer returns a reflex that ensures all core alarms exist and
// are healthy. Throttled to run at most once per enforceCooldown.
func (s *Service) CoreAlarmEnforcer(cfg *config.Config) func(ctx context.Context) {
	var lastRun time.Time
	return func(ctx context.Context) {
		if time.Since(lastRun) < enforceCooldown {
			return
		}
		lastRun = time.Now()
		s.ensureCoreAlarms(ctx, cfg)
	}
}

func (s *Service) ensureCoreAlarms(ctx context.Context, cfg *config.Config) {
	if len(cfg.PrivilegedConversationIDs) == 0 {
		slog.Warn("no privileged conversations configured, skipping core alarm enforcement", "pkg", "alarms")
		return
	}

	convID := cfg.PrivilegedConversationIDs[0]
	tz := cfg.TZ()
	now := time.Now()

	for _, sched := range coreSchedules {
		timeOfDay := sched.configTime(cfg)
		if timeOfDay == "" {
			continue
		}

		existing, found, err := db.AlarmGet(ctx, s.db, sched.prefix)
		if err != nil {
			slog.Warn("find core alarm", "pkg", "alarms", "prefix", sched.prefix, "error", err)
			continue
		}

		if found && existing.NextFireAt.Valid && existing.NextFireAt.Time.After(now) {
			continue
		}

		nextFire, err := nextDailyFireAt(timeOfDay, sched.offset, tz, now)
		if err != nil {
			slog.Warn("compute next fire time", "pkg", "alarms", "prefix", sched.prefix, "error", err)
			continue
		}

		if found {
			err = db.AlarmUpdate(ctx, s.db, existing.ID, db.AlarmUpdateParams{
				NextFireAt: nextFire,
			})
			if err != nil {
				slog.Warn("heal core alarm", "pkg", "alarms", "prefix", sched.prefix, "error", err)
				continue
			}
			slog.Info("healed core alarm", "pkg", "alarms", "prefix", sched.prefix, "next_fire_at", nextFire)
			continue
		}

		_, err = db.CreateAlarm(ctx, s.db, db.CreateAlarmParams{
			OriginConversationID: convID,
			Goal:                 sched.goal,
			Recurrence:           sched.recurrence,
			NextFireAt:           nextFire,
		})
		if err != nil {
			slog.Warn("create core alarm", "pkg", "alarms", "prefix", sched.prefix, "error", err)
			continue
		}
		slog.Info("created core alarm", "pkg", "alarms", "prefix", sched.prefix, "next_fire_at", nextFire)
	}
}

func nextDailyFireAt(timeOfDay string, offset time.Duration, tz *time.Location, now time.Time) (time.Time, error) {
	parts := strings.SplitN(timeOfDay, ":", 2)
	if len(parts) != 2 {
		return time.Time{}, fmt.Errorf("invalid time format %q, expected HH:MM", timeOfDay)
	}

	hour, err := strconv.Atoi(parts[0])
	if err != nil {
		return time.Time{}, fmt.Errorf("parse hour %q: %w", parts[0], err)
	}

	minute, err := strconv.Atoi(parts[1])
	if err != nil {
		return time.Time{}, fmt.Errorf("parse minute %q: %w", parts[1], err)
	}

	nowLocal := now.In(tz)
	candidate := time.Date(nowLocal.Year(), nowLocal.Month(), nowLocal.Day(), hour, minute, 0, 0, tz)
	candidate = candidate.Add(offset)

	if !candidate.After(now) {
		candidate = candidate.AddDate(0, 0, 1)
	}

	return candidate, nil
}
