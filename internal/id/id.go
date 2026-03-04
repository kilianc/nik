package id

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"

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
