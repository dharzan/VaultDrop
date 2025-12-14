// Package config centralizes how VaultDrop reads environment variables and
// exposes them as strongly typed Go values.
package config

import (
	"crypto/rand"
	"encoding/hex"
	"os"
	"strconv"
	"strings"
	"time"
)

// Config represents runtime configuration for the service. Struct fields in Go
// begin with capital letters when they must be exported (visible to other
// packages), while lower-case fields remain private.
type Config struct {
	Address        string
	MaxFileSize    int64
	AllowedTypes   []string
	SigningSecret  []byte
	SignedURLTTL   time.Duration
	ProcessingPool int
}

const (
	// const declares compile-time constants; shifts work on integers so
	// 25 << 20 equals 25 * 2^20 bytes.
	defaultAddress      = ":8080"
	defaultMaxFileSize  = 25 << 20 // 25 MiB
	defaultAllowedTypes = "application/pdf,image/png,image/jpeg,text/plain"
	defaultSignedTTL    = 5 * time.Minute
	defaultWorkerCount  = 2
)

// Load reads configuration from environment variables falling back to defaults.
// It follows Go's convention of returning (value, error) so callers can handle
// failures rather than panicking.
func Load() (*Config, error) {
	cfg := &Config{
		// Struct literal syntax assigns values to each exported field.
		Address:        readEnv("VAULTDROP_ADDRESS", defaultAddress),
		MaxFileSize:    parseInt64("VAULTDROP_MAX_FILE_BYTES", defaultMaxFileSize),
		AllowedTypes:   parseList("VAULTDROP_ALLOWED_TYPES", defaultAllowedTypes),
		SigningSecret:  parseSecret("VAULTDROP_SIGNING_SECRET"),
		SignedURLTTL:   parseDuration("VAULTDROP_SIGNED_TTL", defaultSignedTTL),
		ProcessingPool: parseInt("VAULTDROP_WORKERS", defaultWorkerCount),
	}
	if cfg.SigningSecret == nil {
		// If no secret was supplied we generate one using crypto/rand.
		cfg.SigningSecret = randomSecret()
	}
	if cfg.ProcessingPool <= 0 {
		cfg.ProcessingPool = defaultWorkerCount
	}
	if cfg.MaxFileSize <= 0 {
		cfg.MaxFileSize = defaultMaxFileSize
	}
	if cfg.SignedURLTTL <= 0 {
		cfg.SignedURLTTL = defaultSignedTTL
	}
	return cfg, nil
}

func readEnv(key, def string) string {
	// LookupEnv returns (value, true) when the variable is present, mirroring
	// Go's pattern of providing extra information via multiple return values.
	if v, ok := os.LookupEnv(key); ok && v != "" {
		return v
	}
	return def
}

func parseList(key, def string) []string {
	// Strings.Split returns a slice (dynamic array) of substrings that we trim.
	val := readEnv(key, def)
	out := strings.Split(val, ",")
	for i := range out {
		out[i] = strings.TrimSpace(out[i])
	}
	return out
}

func parseInt64(key string, def int64) int64 {
	// strconv.ParseInt converts strings to integers; Go treats errors as values
	// so we simply ignore invalid input and return the default.
	if v, ok := os.LookupEnv(key); ok && v != "" {
		if parsed, err := strconv.ParseInt(v, 10, 64); err == nil {
			return parsed
		}
	}
	return def
}

func parseInt(key string, def int) int {
	if v, ok := os.LookupEnv(key); ok && v != "" {
		if parsed, err := strconv.Atoi(v); err == nil {
			return parsed
		}
	}
	return def
}

func parseDuration(key string, def time.Duration) time.Duration {
	// time.ParseDuration understands inputs like "5m" or "30s".
	if v, ok := os.LookupEnv(key); ok && v != "" {
		if parsed, err := time.ParseDuration(v); err == nil {
			return parsed
		}
	}
	return def
}

func parseSecret(key string) []byte {
	// In Go, a string can be converted to a []byte slice to work with binary
	// data such as HMAC secrets.
	if v, ok := os.LookupEnv(key); ok && v != "" {
		return []byte(v)
	}
	return nil
}

func randomSecret() []byte {
	// crypto/rand.Read fills a byte slice with secure random data; we return the
	// slice so callers can use it immediately without extra allocations.
	buf := make([]byte, 32)
	if _, err := rand.Read(buf); err != nil {
		return []byte(hex.EncodeToString([]byte("fallbacksecret")))
	}
	return buf
}
