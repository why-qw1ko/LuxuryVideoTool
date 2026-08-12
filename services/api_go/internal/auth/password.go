package auth

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"fmt"
	"strconv"
	"strings"

	"golang.org/x/crypto/argon2"
)

const (
	argonMemory      = 64 * 1024
	argonIterations  = 3
	argonParallelism = 4
	saltLength       = 16
	keyLength        = 32
)

func HashPassword(password string) (string, error) {
	if len(password) < 12 || len(password) > 1024 {
		return "", fmt.Errorf("password must contain 12 to 1024 bytes")
	}
	salt := make([]byte, saltLength)
	if _, err := rand.Read(salt); err != nil {
		return "", fmt.Errorf("generate password salt: %w", err)
	}
	digest := argon2.IDKey([]byte(password), salt, argonIterations, argonMemory, argonParallelism, keyLength)
	encoder := base64.RawStdEncoding
	return fmt.Sprintf("$argon2id$v=%d$m=%d,t=%d,p=%d$%s$%s", argon2.Version, argonMemory, argonIterations, argonParallelism, encoder.EncodeToString(salt), encoder.EncodeToString(digest)), nil
}

func VerifyPassword(password, encoded string) (bool, error) {
	parts := strings.Split(encoded, "$")
	if len(parts) != 6 || parts[1] != "argon2id" {
		return false, fmt.Errorf("invalid password hash format")
	}
	var version int
	if _, err := fmt.Sscanf(parts[2], "v=%d", &version); err != nil || version != argon2.Version {
		return false, fmt.Errorf("unsupported argon2 version")
	}
	params := strings.Split(parts[3], ",")
	if len(params) != 3 {
		return false, fmt.Errorf("invalid argon2 parameters")
	}
	memory, err := parseParameter(params[0], "m")
	if err != nil {
		return false, err
	}
	iterations, err := parseParameter(params[1], "t")
	if err != nil {
		return false, err
	}
	parallelism, err := parseParameter(params[2], "p")
	if err != nil || parallelism > 255 {
		return false, fmt.Errorf("invalid argon2 parallelism")
	}
	if memory > 256*1024 || iterations > 10 || memory == 0 || iterations == 0 || parallelism == 0 {
		return false, fmt.Errorf("unsafe argon2 parameters")
	}
	encoder := base64.RawStdEncoding
	salt, err := encoder.DecodeString(parts[4])
	if err != nil || len(salt) < 16 || len(salt) > 64 {
		return false, fmt.Errorf("invalid password salt")
	}
	want, err := encoder.DecodeString(parts[5])
	if err != nil || len(want) < 16 || len(want) > 64 {
		return false, fmt.Errorf("invalid password digest")
	}
	got := argon2.IDKey([]byte(password), salt, iterations, memory, uint8(parallelism), uint32(len(want)))
	return subtle.ConstantTimeCompare(got, want) == 1, nil
}

func parseParameter(raw, name string) (uint32, error) {
	value, found := strings.CutPrefix(raw, name+"=")
	if !found {
		return 0, fmt.Errorf("invalid argon2 parameter %s", name)
	}
	parsed, err := strconv.ParseUint(value, 10, 32)
	if err != nil {
		return 0, fmt.Errorf("invalid argon2 parameter %s", name)
	}
	return uint32(parsed), nil
}

