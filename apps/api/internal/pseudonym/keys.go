package pseudonym

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"fmt"
	"strconv"
)

// Keys derives every secret of the package from one master key.
type Keys struct {
	master []byte
	aead   cipher.AEAD
}

// ErrNoKey means PII_MASTER_KEY is missing or malformed: reversible
// pseudonymization is off and callers must not send text to external models.
var ErrNoKey = errors.New("pseudonym: master key is not configured")

// ParseKeys accepts a base64 (std or URL) encoding of exactly 32 bytes.
func ParseKeys(encoded string) (*Keys, error) {
	if encoded == "" {
		return nil, ErrNoKey
	}
	raw, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil {
		raw, err = base64.RawURLEncoding.DecodeString(encoded)
	}
	if err != nil || len(raw) != 32 {
		return nil, fmt.Errorf("%w: need base64 of 32 bytes", ErrNoKey)
	}
	encKey := derive(raw, "vault-encryption-v1")
	block, err := aes.NewCipher(encKey)
	if err != nil {
		return nil, err
	}
	aead, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	return &Keys{master: raw, aead: aead}, nil
}

func derive(key []byte, label string) []byte {
	m := hmac.New(sha256.New, key)
	m.Write([]byte(label))
	return m.Sum(nil)
}

// userKey is unique per user: the same phone number has unrelated
// fingerprints in two users' vaults, so vaults cannot be joined.
func (k *Keys) userKey(userID int64) []byte {
	return derive(k.master, "user:"+strconv.FormatInt(userID, 10))
}

// Fingerprint identifies a value inside one user's vault.
func (k *Keys) Fingerprint(userID int64, kind, key string) []byte {
	m := hmac.New(sha256.New, k.userKey(userID))
	m.Write([]byte(kind))
	m.Write([]byte{0})
	m.Write([]byte(key))
	return m.Sum(nil)
}

func aad(userID int64, kind string) []byte {
	return []byte("user:" + strconv.FormatInt(userID, 10) + "|kind:" + kind)
}

// Seal encrypts plaintext bound to (user, kind): a row copied to another
// user or kind fails to open.
func (k *Keys) Seal(userID int64, kind string, plaintext []byte) (nonce, ciphertext []byte, err error) {
	nonce = make([]byte, k.aead.NonceSize())
	if _, err = rand.Read(nonce); err != nil {
		return nil, nil, err
	}
	return nonce, k.aead.Seal(nil, nonce, plaintext, aad(userID, kind)), nil
}

func (k *Keys) Open(userID int64, kind string, nonce, ciphertext []byte) ([]byte, error) {
	return k.aead.Open(nil, nonce, ciphertext, aad(userID, kind))
}
