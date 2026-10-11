package auth

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"unicode/utf8"

	"golang.org/x/crypto/argon2"
)

// Verify accepts bounded PHC parameters before allocating memory. Invalid
// encodings and mismatches have the same result and never expose stored hashes.
func Verify(plain, encoded string) bool {
	if !utf8.ValidString(plain) || utf8.RuneCountInString(plain) > 128 || len(encoded) > 512 {
		return false
	}
	parts := strings.Split(encoded, "$")
	if len(parts) != 6 || parts[0] != "" || parts[1] != "argon2id" || parts[2] != "v=19" {
		return false
	}
	params := strings.Split(parts[3], ",")
	if len(params) != 3 {
		return false
	}
	var values [3]uint64
	for i, prefix := range []string{"m=", "t=", "p="} {
		if !strings.HasPrefix(params[i], prefix) {
			return false
		}
		value, err := strconv.ParseUint(strings.TrimPrefix(params[i], prefix), 10, 32)
		if err != nil {
			return false
		}
		values[i] = value
	}
	if values[0] < 8*1024 || values[0] > 64*1024 || values[1] < 1 || values[1] > 4 || values[2] < 1 || values[2] > 4 {
		return false
	}
	salt, err := base64.RawStdEncoding.Strict().DecodeString(parts[4])
	if err != nil || len(salt) < 16 || len(salt) > 32 {
		return false
	}
	key, err := base64.RawStdEncoding.Strict().DecodeString(parts[5])
	if err != nil || len(key) != int(keyLength) {
		return false
	}
	actual := argon2.IDKey([]byte(plain), salt, uint32(values[1]), uint32(values[0]), uint8(values[2]), keyLength)
	return subtle.ConstantTimeCompare(actual, key) == 1
}

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
