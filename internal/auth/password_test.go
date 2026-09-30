package auth

import (
	"encoding/base64"
	"strings"
	"testing"

	"golang.org/x/crypto/argon2"
)

func TestPasswordHashCanonicalComputation(t *testing.T) {
	password := []byte("canonical password with spaces")
	defer clear(password)
	// Fixed answer computed independently with ADR-0013's v19, 19456 KiB,
	// two iterations, one lane, salt "0123456789abcdef" and 32-byte key.
	const canonical = "$argon2id$v=19$m=19456,t=2,p=1$MDEyMzQ1Njc4OWFiY2RlZg$kzdfHyo+XfF83MlTm7TDgnRjlsWqsU7ooAUY1m0dFGE"
	if !verifyPassword(password, canonical) {
		t.Fatal("canonical independent password record failed verification")
	}
	for _, costs := range []struct {
		iterations uint32
		memory     uint32
		lanes      uint8
	}{{1, 19456, 1}, {2, 16384, 1}, {2, 19456, 2}} {
		key := argon2.IDKey(password, []byte("0123456789abcdef"), costs.iterations, costs.memory, costs.lanes, 32)
		// Keep the canonical declaration so only the actual computation differs.
		variant := "$argon2id$v=19$m=19456,t=2,p=1$MDEyMzQ1Njc4OWFiY2RlZg$" + base64.RawStdEncoding.EncodeToString(key)
		clear(key)
		if verifyPassword(password, variant) {
			t.Fatal("noncanonical computation accepted under the canonical declaration")
		}
	}
	encoded := hashPassword(password)
	salt, got, ok := decodePasswordHash(encoded)
	if !ok {
		t.Fatal("generated hash encoding invalid")
	}
	defer clear(got)
	want := argon2.IDKey(password, salt, 2, 19456, 1, 32)
	defer clear(want)
	if base64.RawStdEncoding.EncodeToString(got) != base64.RawStdEncoding.EncodeToString(want) {
		t.Fatal("hash computation differs from the independently pinned costs")
	}
}

func TestPasswordHashSaltAndExactVerification(t *testing.T) {
	password := []byte(" a long password with spaces ")
	defer clear(password)
	first, second := hashPassword(password), hashPassword(password)
	if first == second || !ValidPasswordHash(first) || !ValidPasswordHash(second) {
		t.Fatal("password hashes must have independent salts and the approved encoding")
	}
	if !verifyPassword(password, first) || verifyPassword([]byte("a long password with spaces"), first) {
		t.Fatal("password bytes were trimmed or failed verification")
	}
	for _, broken := range []string{
		"", first + "$extra", first[:len(first)-1], strings.Repeat("a", 4096),
		strings.Replace(first, "m=19456", "m=99999", 1),
		strings.Replace(first, "t=2", "t=9", 1),
		strings.Replace(first, "p=1", "p=0", 1),
		strings.Replace(first, "v=19", "v=18", 1),
		strings.Replace(first, "argon2id", "argon2ii", 1),
		passwordPrefix + strings.Repeat("!", 22) + "$" + strings.Repeat("!", 43),
	} {
		if ValidPasswordHash(broken) || verifyPassword(password, broken) {
			t.Fatal("damaged or unapproved password hash accepted")
		}
	}
}

func TestPasswordBoundsAndUnicode(t *testing.T) {
	for _, example := range []struct {
		password string
		valid    bool
	}{
		{"short password", false}, {strings.Repeat("한", 15), true},
		{strings.Repeat("😀", 15), true}, {strings.Repeat("a", 1024), true},
		{strings.Repeat("a", 1025), false}, {strings.Repeat("한", 342), false},
		{strings.Repeat("a", 15) + "\xff", false},
	} {
		if ValidPassword([]byte(example.password)) != example.valid {
			t.Fatalf("unexpected validation for %d bytes", len(example.password))
		}
	}
}
