package v1

import (
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
)

const (
	HeaderKeyID     = "X-Scheduler-Key-ID"
	HeaderTimestamp = "X-Scheduler-Timestamp"
	HeaderNonce     = "X-Scheduler-Nonce"
	HeaderSignature = "X-Scheduler-Signature"
	HeaderPayload   = "X-Scheduler-Content-SHA256"
)

func GenerateKeyPair() (publicKey, privateKey string, err error) {
	public, private, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		return "", "", fmt.Errorf("generate connector key: %w", err)
	}
	return base64.RawURLEncoding.EncodeToString(public),
		base64.RawURLEncoding.EncodeToString(private), nil
}

func DecodePublicKey(encoded string) (ed25519.PublicKey, error) {
	raw, err := base64.RawURLEncoding.DecodeString(strings.TrimSpace(encoded))
	if err != nil || len(raw) != ed25519.PublicKeySize {
		return nil, errors.New("invalid Ed25519 public key")
	}
	return ed25519.PublicKey(raw), nil
}

func DecodePrivateKey(encoded string) (ed25519.PrivateKey, error) {
	raw, err := base64.RawURLEncoding.DecodeString(strings.TrimSpace(encoded))
	if err != nil || len(raw) != ed25519.PrivateKeySize {
		return nil, errors.New("invalid Ed25519 private key")
	}
	return ed25519.PrivateKey(raw), nil
}

func PayloadDigest(body []byte) string {
	digest := sha256.Sum256(body)
	return hex.EncodeToString(digest[:])
}

func CanonicalRequest(method, path, timestamp, nonce, payloadDigest string) []byte {
	return []byte(strings.Join([]string{
		strings.ToUpper(strings.TrimSpace(method)),
		path,
		timestamp,
		nonce,
		strings.ToLower(payloadDigest),
	}, "\n"))
}

func SignRequest(privateKey ed25519.PrivateKey, method, path, timestamp, nonce string, body []byte) string {
	signature := ed25519.Sign(privateKey, CanonicalRequest(
		method, path, timestamp, nonce, PayloadDigest(body),
	))
	return base64.RawURLEncoding.EncodeToString(signature)
}

func VerifyRequest(publicKey ed25519.PublicKey, method, path, timestamp, nonce, digest, signature string) bool {
	raw, err := base64.RawURLEncoding.DecodeString(signature)
	if err != nil || len(raw) != ed25519.SignatureSize {
		return false
	}
	return ed25519.Verify(publicKey, CanonicalRequest(method, path, timestamp, nonce, digest), raw)
}
