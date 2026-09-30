package auth

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"strings"
	"unicode/utf8"

	gulv1 "github.com/rootkernel/gul/api/generated/go/gul/v1"
	"golang.org/x/crypto/argon2"
)

const (
	MinPasswordCharacters = gulv1.MinimumPasswordCharacters
	MaxPasswordBytes      = gulv1.MaximumPasswordBytes
	passwordPrefix        = "$argon2id$v=19$m=19456,t=2,p=1$"
	saltBytes             = 16
	keyBytes              = 32
)

// ValidPassword preserves the exact UTF-8 bytes, including spaces. Passwords
// are neither trimmed nor normalized; the bound applies before any hash work.
func ValidPassword(password []byte) bool {
	return len(password) <= MaxPasswordBytes && utf8.Valid(password) && utf8.RuneCount(password) >= MinPasswordCharacters
}

func hashPassword(password []byte) string {
	salt := make([]byte, saltBytes)
	rand.Read(salt)
	key := argon2.IDKey(password, salt, 2, 19*1024, 1, keyBytes)
	defer clear(key)
	return passwordPrefix + base64.RawStdEncoding.EncodeToString(salt) + "$" + base64.RawStdEncoding.EncodeToString(key)
}

// ValidPasswordHash accepts only the pinned cost and canonical fixed-length
// encoding. A damaged record cannot select excessive memory/CPU parameters.
func ValidPasswordHash(encoded string) bool {
	_, _, ok := decodePasswordHash(encoded)
	return ok
}

func decodePasswordHash(encoded string) ([]byte, []byte, bool) {
	if len(encoded) != len(passwordPrefix)+22+1+43 || !strings.HasPrefix(encoded, passwordPrefix) {
		return nil, nil, false
	}
	parts := strings.Split(strings.TrimPrefix(encoded, passwordPrefix), "$")
	if len(parts) != 2 {
		return nil, nil, false
	}
	salt, saltErr := base64.RawStdEncoding.Strict().DecodeString(parts[0])
	key, keyErr := base64.RawStdEncoding.Strict().DecodeString(parts[1])
	if saltErr != nil || keyErr != nil || len(salt) != saltBytes || len(key) != keyBytes ||
		base64.RawStdEncoding.EncodeToString(salt) != parts[0] || base64.RawStdEncoding.EncodeToString(key) != parts[1] {
		return nil, nil, false
	}
	return salt, key, true
}

func verifyPassword(password []byte, encoded string) bool {
	salt, expected, ok := decodePasswordHash(encoded)
	if !ok || !ValidPassword(password) {
		return false
	}
	key := argon2.IDKey(password, salt, 2, 19*1024, 1, keyBytes)
	defer clear(key)
	defer clear(expected)
	return subtle.ConstantTimeCompare(key, expected) == 1
}
