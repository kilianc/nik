package contacts

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"

	"github.com/kciuffolo/nik/internal/db"
)

const NikContactID = "00000000-0000-7000-8000-000000000001"

type Service struct {
	db *sql.DB
}

func NewService(conn *sql.DB) *Service {
	return &Service{db: conn}
}

func (s *Service) EnsureContactForMessage(ctx context.Context, platform string, externalIDs []string, isFromMe bool, at time.Time) (string, error) {
	if len(externalIDs) == 0 || externalIDs[0] == "" {
		return "", fmt.Errorf("empty external sender id")
	}

	primaryID := externalIDs[0]

	switch platform {
	case "whatsapp":
		if isFromMe {
			existing, err := db.ContactGet(ctx, s.db, primaryID)
			if err == nil && existing.ID != NikContactID {
				return "", fmt.Errorf("sender %s belongs to non-nik contact %s", primaryID, existing.ID)
			}

			self, err := db.ContactUpsert(ctx, s.db, db.ContactUpsertParams{
				Platform:      platform,
				ExternalID:    primaryID,
				IsSelf:        true,
				SelfID:        NikContactID,
				LastMessageAt: at,
			})
			if err != nil {
				return "", err
			}

			for _, id := range externalIDs[1:] {
				_ = db.ContactAddWhatsAppID(ctx, s.db, db.ContactAddWhatsAppIDParams{
					ContactID: self.ID,
					JID:       id,
					Phone:     phoneFromWhatsAppID(id),
				})
			}

			return self.ID, nil
		}

		matched := s.resolveWhatsAppContact(ctx, externalIDs)

		if matched == nil {
			phone := phoneFromWhatsAppID(primaryID)
			created, err := db.ContactUpsert(ctx, s.db, db.ContactUpsertParams{
				Platform:      platform,
				ExternalID:    primaryID,
				Name:          "",
				Phone:         phone,
				LastMessageAt: at,
			})
			if err != nil {
				return "", err
			}

			for _, id := range externalIDs[1:] {
				_ = db.ContactAddWhatsAppID(ctx, s.db, db.ContactAddWhatsAppIDParams{
					ContactID: created.ID,
					JID:       id,
					Phone:     phoneFromWhatsAppID(id),
				})
			}

			return created.ID, nil
		}

		for _, id := range externalIDs {
			_ = db.ContactAddWhatsAppID(ctx, s.db, db.ContactAddWhatsAppIDParams{
				ContactID: matched.ID,
				JID:       id,
				Phone:     phoneFromWhatsAppID(id),
			})
		}

		return matched.ID, nil

	case "local":
		if isFromMe {
			return NikContactID, nil
		}
		return db.OwnerContactID, nil

	default:
		if isFromMe {
			return "", fmt.Errorf("self-contact upsert not implemented for platform %s", platform)
		}

		contact, err := db.ContactGet(ctx, s.db, primaryID)
		if err != nil {
			return "", fmt.Errorf("resolve contact %s/%s: %w", platform, primaryID, err)
		}

		return contact.ID, nil
	}
}

func (s *Service) resolveWhatsAppContact(ctx context.Context, externalIDs []string) *db.Contact {
	for _, id := range externalIDs {
		c, err := db.ContactGet(ctx, s.db, id)
		if err == nil {
			return &c
		}
	}

	return nil
}

func phoneFromWhatsAppID(id string) string {
	parts := strings.SplitN(id, "@", 2)
	if len(parts) == 0 {
		return ""
	}

	return parts[0]
}
