package skills

import (
	"bytes"
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"log/slog"
	"os/exec"
	"strings"
	"time"

	"github.com/kciuffolo/nik/internal/config"
	"github.com/kciuffolo/nik/internal/db"
)

func SkillChangeReflex(cfg *config.Config, conn *sql.DB) func(ctx context.Context) {
	return func(ctx context.Context) {
		dirs := []string{cfg.SkillsPath(), cfg.WorkspaceSkillsPath()}

		fsSkills := map[string]fsSkill{}
		err := walkSkillDirs(dirs, func(s SkillSummary, data []byte) {
			content := string(data)
			contentHash := sha256.Sum256(data)
			installSection := extractInstallSection(content)

			var installHashStr string
			if installSection != "" {
				h := sha256.Sum256([]byte(installSection))
				installHashStr = hex.EncodeToString(h[:])
			}

			fsSkills[s.Name] = fsSkill{
				contentHash: hex.EncodeToString(contentHash[:]),
				installHash: installHashStr,
			}
		})
		if err != nil {
			slog.Warn("skill reflex: walk dirs", "error", err)
			return
		}

		existing, err := db.SkillList(ctx, conn)
		if err != nil {
			slog.Warn("skill reflex: list skills", "error", err)
			return
		}

		known := map[string]db.Skill{}
		for _, s := range existing {
			known[s.Name] = s
		}

		privIDs := cfg.PrivilegedIDs()

		for name, fs := range fsSkills {
			prev, exists := known[name]

			if !exists || prev.Status == "removed" {
				applySkillChange(ctx, conn, privIDs, "added", db.SkillUpsertParams{
					Name:        name,
					Status:      "active",
					ContentHash: fs.contentHash,
					InstallHash: fs.installHash,
				})
				continue
			}

			contentChanged := prev.ContentHash.String != fs.contentHash
			installChanged := prev.InstallHash.String != fs.installHash

			if !contentChanged && !installChanged {
				continue
			}

			if installChanged {
				applySkillChange(ctx, conn, privIDs, "changed", db.SkillUpsertParams{
					Name:        name,
					Status:      "active",
					ContentHash: fs.contentHash,
					InstallHash: fs.installHash,
				})
				continue
			}

			err = upsertSkillOnly(ctx, conn, db.SkillUpsertParams{
				Name:        name,
				Status:      "active",
				ContentHash: fs.contentHash,
				InstallHash: fs.installHash,
			})
			if err != nil {
				slog.Warn("skill reflex: upsert prompt-only change", "name", name, "error", err)
			}
		}

		for name, prev := range known {
			if prev.Status == "removed" {
				continue
			}
			if _, onDisk := fsSkills[name]; onDisk {
				continue
			}

			applySkillChange(ctx, conn, privIDs, "removed", db.SkillUpsertParams{
				Name:   name,
				Status: "removed",
			})
		}
	}
}

func applySkillChange(ctx context.Context, conn *sql.DB, privIDs []string, kind string, p db.SkillUpsertParams) {
	tx, err := conn.BeginTx(ctx, nil)
	if err != nil {
		slog.Warn("skill reflex: begin tx", "name", p.Name, "error", err)
		return
	}
	defer tx.Rollback()

	skill, err := db.SkillUpsert(ctx, tx, p)
	if err != nil {
		slog.Warn("skill reflex: upsert "+kind, "name", p.Name, "error", err)
		return
	}

	_, err = db.SkillEventInsert(ctx, tx, db.SkillEventInsertParams{
		Name:        p.Name,
		Kind:        kind,
		ContentHash: p.ContentHash,
		InstallHash: p.InstallHash,
	})
	if err != nil {
		slog.Warn("skill reflex: insert event", "name", p.Name, "kind", kind, "error", err)
		return
	}

	err = db.InsertSystemMessage(ctx, tx, db.SystemMessageParams{
		ConversationID: privIDs[0],
		Kind:           "skill_" + kind,
		Body:           skill,
		SentAt:         skill.UpdatedAt,
	})
	if err != nil {
		slog.Warn("skill reflex: insert system message", "name", p.Name, "kind", kind, "error", err)
		return
	}

	err = tx.Commit()
	if err != nil {
		slog.Warn("skill reflex: commit", "name", p.Name, "error", err)
		return
	}

	slog.Info("skill event", "name", p.Name, "type", kind)
}

func upsertSkillOnly(ctx context.Context, conn *sql.DB, p db.SkillUpsertParams) error {
	tx, err := conn.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	_, err = db.SkillUpsert(ctx, tx, p)
	if err != nil {
		return err
	}

	return tx.Commit()
}

type fsSkill struct {
	contentHash string
	installHash string
}

func extractInstallSection(content string) string {
	idx := strings.Index(content, "\n## Install")
	if idx < 0 {
		return ""
	}

	section := content[idx+1:]
	end := strings.Index(section[len("## Install"):], "\n## ")
	if end >= 0 {
		section = section[:len("## Install")+end]
	}

	return strings.TrimSpace(section)
}

// ExtractInstallSection is exported for testing.
func ExtractInstallSection(content string) string {
	return extractInstallSection(content)
}

const maxCheckTimeout = 30 * time.Second

func SkillCheckReflex(cfg *config.Config, conn *sql.DB) func(ctx context.Context) {
	lastRun := map[string]time.Time{}

	return func(ctx context.Context) {
		dirs := []string{cfg.SkillsPath(), cfg.WorkspaceSkillsPath()}

		reflexes, err := ListReflexes(dirs...)
		if err != nil {
			slog.Warn("skill check reflex: list reflexes", "error", err)
			return
		}

		now := time.Now().In(cfg.TZ())

		for key, def := range reflexes {
			if t, ok := lastRun[key]; ok && !def.IsDue(t, now) {
				continue
			}
			lastRun[key] = now

			runSkillCheck(ctx, cfg, conn, key, def)
		}
	}
}

func runSkillCheck(ctx context.Context, cfg *config.Config, conn *sql.DB, key string, def SkillReflexDef) {
	lastMeta, err := db.SkillReflexLatest(ctx, conn, key)
	if err != nil {
		slog.Warn("skill check reflex: get latest", "key", key, "error", err)
		return
	}

	var newMeta string

	if def.Command == "" {
		newMeta = time.Now().UTC().Format(time.RFC3339)
	} else {
		cmdCtx, cancel := context.WithTimeout(ctx, maxCheckTimeout)
		defer cancel()

		cmd := exec.CommandContext(cmdCtx, "sh", "-c", def.Command)
		cmd.Dir = cfg.Home
		cmd.Stdin = strings.NewReader(lastMeta)

		var stdout, stderr bytes.Buffer
		cmd.Stdout = &stdout
		cmd.Stderr = &stderr

		err = cmd.Run()
		if err != nil {
			slog.Warn("skill check reflex: command failed",
				"key", key,
				"command", def.Command,
				"error", err,
				"stderr", stderr.String(),
			)
			return
		}

		newMeta = strings.TrimSpace(stdout.String())
	}

	if newMeta == "" || newMeta == lastMeta {
		return
	}

	err = db.SkillReflexInsert(ctx, conn, key, newMeta)
	if err != nil {
		slog.Warn("skill check reflex: insert meta", "key", key, "error", err)
		return
	}

	privIDs := cfg.PrivilegedIDs()
	if len(privIDs) == 0 {
		slog.Warn("skill check reflex: no privileged conversations", "key", key)
		return
	}

	skillName, _, _ := strings.Cut(key, "/")

	body := map[string]string{
		"skill": skillName,
		"name":  def.Name,
	}
	if def.Command != "" {
		body["meta"] = newMeta
	}

	err = db.InsertSystemMessage(ctx, conn, db.SystemMessageParams{
		ConversationID: privIDs[0],
		Kind:           "skill_reflex_fired",
		Body:           body,
		SentAt:         time.Now().UTC(),
	})
	if err != nil {
		slog.Warn("skill check reflex: insert system message", "key", key, "error", err)
		return
	}

	slog.Info("skill reflex fired", "key", key)
}
