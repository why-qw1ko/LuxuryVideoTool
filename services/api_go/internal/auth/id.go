package auth

import (
	"crypto/rand"
	"fmt"
	"time"

	"github.com/oklog/ulid/v2"
)

func NewID(now time.Time) (string, error) {
	id, err := ulid.New(ulid.Timestamp(now.UTC()), rand.Reader)
	if err != nil {
		return "", fmt.Errorf("generate ULID: %w", err)
	}
	return id.String(), nil
}
