package auth

import (
	"encoding/base64"
	"strings"
	"testing"

	"github.com/google/uuid"
)

func TestHashUsesRandomSaltAndPHCFormat(t *testing.T) {
	plain := uuid.NewString()

	first, err := Hash(plain)
	if err != nil {
		t.Fatal(err)
	}

	second, err := Hash(plain)
	if err != nil {
		t.Fatal(err)
	}

	if first == second {
		t.Fatal("same password must receive different random salts")
	}

	if strings.Contains(first, plain) {
		t.Fatal("encoded hash contains plaintext")
	}

	parts := strings.Split(first, "$")
	if len(parts) != 6 ||
		parts[1] != "argon2id" ||
		parts[2] != "v=19" ||
		parts[3] != "m=19456,t=2,p=1" {
		t.Fatal("unexpected PHC format")
	}

	salt, err := base64.RawStdEncoding.DecodeString(parts[4])
	if err != nil || len(salt) != 16 {
		t.Fatal("invalid salt")
	}

	key, err := base64.RawStdEncoding.DecodeString(parts[5])
	if err != nil || len(key) != 32 {
		t.Fatal("invalid derived key")
	}
}

func TestHashRejectsInvalidPassword(t *testing.T) {
	for _, plain := range []string{
		"",
		strings.Repeat("x", 11),
		strings.Repeat("x", 129),
		string([]byte{0xff}),
	} {
		if _, err := Hash(plain); err == nil {
			t.Fatal("invalid password accepted")
		}
	}
}
