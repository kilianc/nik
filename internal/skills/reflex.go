package skills

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"log/slog"
	"strings"

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

	for _, convID := range privIDs {
		err = db.InsertSystemMessage(ctx, tx, db.SystemMessageParams{
			ConversationID: convID,
			Kind:           "skill_" + kind,
			Body:           skill,
			SentAt:         skill.UpdatedAt,
		})
		if err != nil {
			slog.Warn("skill reflex: insert system message", "name", p.Name, "kind", kind, "error", err)
			return
		}
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
