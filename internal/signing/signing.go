// Package signing implements a minimal HMAC helper for generating and verifying
// signed URLs. HMAC is easy in Go thanks to the standard library crypto packages.
package signing

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strconv"
)

// Signer generates and validates HMAC based signatures.
type Signer struct {
	secret []byte
}

// NewSigner creates a Signer.
func NewSigner(secret []byte) *Signer {
	return &Signer{secret: secret}
}

// Sign returns the hex signature for inputs.
func (s *Signer) Sign(fileID string, expiresUnix int64) string {
	// hmac.New accepts a hash constructor (sha256.New) plus the secret key.
	mac := hmac.New(sha256.New, s.secret)
	// fmt.Sprintf builds the canonical payload string, ensuring consistent
	// ordering of values. In Go, fmt is the go-to formatting package.
	payload := fmt.Sprintf("%s:%d", fileID, expiresUnix)
	// Write accepts a byte slice; we derive one by converting the string.
	mac.Write([]byte(payload))
	// hex.EncodeToString is handy for turning raw bytes into a printable token.
	return hex.EncodeToString(mac.Sum(nil))
}

// Validate compares the provided signature with the expected.
func (s *Signer) Validate(fileID, expires, signature string) bool {
	// strconv.ParseInt converts the expires query parameter back to an integer.
	exp, err := strconv.ParseInt(expires, 10, 64)
	if err != nil {
		return false
	}
	expected := s.Sign(fileID, exp)
	// hmac.Equal performs constant-time comparison to avoid timing attacks.
	return hmac.Equal([]byte(expected), []byte(signature))
}
