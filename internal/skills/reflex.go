package skills

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"log/slog"
	"os"
	"strings"

	"github.com/kciuffolo/nik/internal/config"
	"github.com/kciuffolo/nik/internal/db"
)

func SkillChangeReflex(cfg *config.Config, conn *sql.DB) func(ctx context.Context) {
	return func(ctx context.Context) {
		dirs := []string{cfg.SkillsPath(), cfg.WorkspaceSkillsPath()}

		fsSkills := map[string]fsSkill{}
		err := walkSkillDirs(dirs, func(path string, s SkillSummary) {
			raw, readErr := os.ReadFile(path)
			if readErr != nil {
				return
			}
			content := string(raw)
			contentHash := sha256.Sum256(raw)
			installSection := extractInstallSection(content)
			installHash := sha256.Sum256([]byte(installSection))
			fsSkills[s.Name] = fsSkill{
				contentHash: hex.EncodeToString(contentHash[:]),
				installHash: hex.EncodeToString(installHash[:]),
			}
		})
		if err != nil {
			slog.Warn("skill reflex: walk dirs", "error", err)
			return
		}

		latest, err := db.SkillEventLatestPerName(ctx, conn)
		if err != nil {
			slog.Warn("skill reflex: query latest events", "error", err)
			return
		}

		known := map[string]db.SkillEvent{}
		for _, e := range latest {
			known[e.Name] = e
		}

		for name, fs := range fsSkills {
			prev, exists := known[name]

			if !exists || prev.Kind == "removed" {
				_, insertErr := db.SkillEventInsert(ctx, conn, db.SkillEventInsertParams{
					Name:        name,
					Kind:        "added",
					ContentHash: fs.contentHash,
					InstallHash: fs.installHash,
				})
				if insertErr != nil {
					slog.Warn("skill reflex: insert added", "name", name, "error", insertErr)
				} else {
					slog.Info("skill event", "name", name, "type", "added")
				}
				continue
			}

			if prev.ContentHash.Valid && prev.ContentHash.String == fs.contentHash {
				continue
			}

			installChanged := !prev.InstallHash.Valid || prev.InstallHash.String != fs.installHash

			if installChanged {
				_, insertErr := db.SkillEventInsert(ctx, conn, db.SkillEventInsertParams{
					Name:        name,
					Kind:        "changed",
					ContentHash: fs.contentHash,
					InstallHash: fs.installHash,
				})
				if insertErr != nil {
					slog.Warn("skill reflex: insert changed", "name", name, "error", insertErr)
				} else {
					slog.Info("skill event", "name", name, "type", "changed")
				}
			}
		}

		for name, prev := range known {
			if prev.Kind == "removed" {
				continue
			}
			if _, onDisk := fsSkills[name]; onDisk {
				continue
			}

			_, insertErr := db.SkillEventInsert(ctx, conn, db.SkillEventInsertParams{
				Name: name,
				Kind: "removed",
			})
			if insertErr != nil {
				slog.Warn("skill reflex: insert removed", "name", name, "error", insertErr)
			} else {
				slog.Info("skill event", "name", name, "type", "removed")
			}
		}
	}
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
