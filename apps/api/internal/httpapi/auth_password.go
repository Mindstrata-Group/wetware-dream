package httpapi

import (
	"crypto/pbkdf2"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"sync/atomic"
	"time"
)

// passwordPBKDF2 parameters. iter/iterMin are vars so the integration TestMain
// can lower them (otherwise the PBKDF2 cost dominates test time).
const (
	passwordPBKDF2SaltSize = 16
	passwordPBKDF2KeySize  = 32
	passwordResetTTL       = 30 * time.Minute
)

var passwordPBKDF2Iter = 210000
var passwordPBKDF2IterMin = 100000

// S-NEW-3: dummyPasswordVerify imitates the CPU cost of verifyPassword.
// Used in AuthLogin when the email is NOT found so timings do not reveal
// whether the email exists. The salt is fixed, iter matches production.
// Always returns false. An attacker cannot exploit it.
var dummyPasswordSalt = []byte("dummy-salt-equal")

// dummyPasswordVerifyCalls is a counter for tests: we make sure the dummy is really
// called on the "user not found" code path, otherwise the S-NEW-3 protection is lost.
var dummyPasswordVerifyCalls atomic.Int64

func dummyPasswordVerify(password string) bool {
	dummyPasswordVerifyCalls.Add(1)
	_ = pbkdf2SHA256([]byte(password), dummyPasswordSalt, passwordPBKDF2Iter, passwordPBKDF2KeySize)
	return false
}

func pbkdf2SHA256(password, salt []byte, iter, keyLen int) []byte {
	key, err := pbkdf2.Key(sha256.New, string(password), salt, iter, keyLen)
	if err != nil {
		return nil
	}
	return key
}

func hashPassword(password string) (string, error) {
	if len(password) < 10 {
		return "", errors.New("password must be at least 10 characters")
	}
	salt := make([]byte, passwordPBKDF2SaltSize)
	if _, err := rand.Read(salt); err != nil {
		return "", err
	}
	key := pbkdf2SHA256([]byte(password), salt, passwordPBKDF2Iter, passwordPBKDF2KeySize)
	return fmt.Sprintf("pbkdf2_sha256$%d$%s$%s",
		passwordPBKDF2Iter,
		base64.RawStdEncoding.EncodeToString(salt),
		base64.RawStdEncoding.EncodeToString(key),
	), nil
}

func verifyPassword(password, encoded string) bool {
	parts := strings.Split(encoded, "$")
	if len(parts) != 4 || parts[0] != "pbkdf2_sha256" {
		return false
	}
	iter, err := strconv.Atoi(parts[1])
	if err != nil || iter < passwordPBKDF2IterMin {
		return false
	}
	salt, err := base64.RawStdEncoding.DecodeString(parts[2])
	if err != nil {
		return false
	}
	want, err := base64.RawStdEncoding.DecodeString(parts[3])
	if err != nil || len(want) == 0 {
		return false
	}
	got := pbkdf2SHA256([]byte(password), salt, iter, len(want))
	return subtle.ConstantTimeCompare(got, want) == 1
}
