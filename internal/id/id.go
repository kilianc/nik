package id

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"strings"

	"github.com/google/uuid"
)

func V4() string {
	return uuid.New().String()
}

func V7() string {
	id, err := uuid.NewV7()
	if err != nil {
		panic(fmt.Sprintf("generate uuidv7: %v", err))
	}
	return id.String()
}

func Short(n int) string {
	buf := make([]byte, n)
	_, _ = rand.Read(buf)
	return hex.EncodeToString(buf)
}

// Shorten extracts the last 12 hex chars (random portion) of a UUID for
// display in the timeline. Works for v4 and v7. Falls back to the last
// 12 characters for non-UUID strings.
func Shorten(raw string) string {
	clean := strings.ReplaceAll(raw, "-", "")
	if len(clean) < 12 {
		return raw
	}
	return clean[len(clean)-12:]
}
