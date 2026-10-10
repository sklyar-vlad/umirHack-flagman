package auth

import (
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
	"unicode/utf8"

	"golang.org/x/crypto/argon2"
)

const (
	memoryKiB   uint32 = 19 * 1024
	iterations  uint32 = 2
	parallelism uint8  = 1
	saltLength         = 16
	keyLength   uint32 = 32
)

func Hash(plain string) (string, error) {
	length := utf8.RuneCountInString(plain)
	if !utf8.ValidString(plain) || length < 12 || length > 128 {
		return "", errors.New("password must contain 12–128 valid Unicode characters")
	}

	salt := make([]byte, saltLength)
	if _, err := rand.Read(salt); err != nil {
		return "", fmt.Errorf("generate password salt: %w", err)
	}

	key := argon2.IDKey(
		[]byte(plain),
		salt,
		iterations,
		memoryKiB,
		parallelism,
		keyLength,
	)

	return fmt.Sprintf(
		"$argon2id$v=%d$m=%d,t=%d,p=%d$%s$%s",
		argon2.Version,
		memoryKiB,
		iterations,
		parallelism,
		base64.RawStdEncoding.EncodeToString(salt),
		base64.RawStdEncoding.EncodeToString(key),
	), nil
}
