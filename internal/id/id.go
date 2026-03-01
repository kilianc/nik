package id

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"

	"github.com/google/uuid"
)

// V4 returns a random UUIDv4 string.
func V4() string {
	return uuid.New().String()
}

// V7 returns a time-ordered UUIDv7 string.
// panics on rand failure (same contract as uuid.New).
func V7() string {
	id, err := uuid.NewV7()
	if err != nil {
		panic(fmt.Sprintf("generate uuidv7: %v", err))
	}
	return id.String()
}

// Short returns a random hex string of 2*n characters (n random bytes).
func Short(n int) string {
	buf := make([]byte, n)
	_, _ = rand.Read(buf)
	return hex.EncodeToString(buf)
}
