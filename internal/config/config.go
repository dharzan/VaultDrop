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
	DatabaseURL    string
	RedisAddr      string
	RedisPassword  string
	RedisDB        int
	S3Endpoint     string
	S3AccessKey    string
	S3SecretKey    string
	S3UseSSL       bool
	S3Region       string
	RawBucket      string
	ProcessedBucket string
}

const (
	// const declares compile-time constants; shifts work on integers so
	// 25 << 20 equals 25 * 2^20 bytes.
	defaultAddress      = ":8080"
	defaultMaxFileSize  = 25 << 20 // 25 MiB
	defaultAllowedTypes = "application/pdf,image/png,image/jpeg,text/plain"
	defaultSignedTTL    = 5 * time.Minute
	defaultWorkerCount  = 2
	defaultDatabaseURL  = "postgres://vaultdrop:vaultdrop@localhost:5432/vaultdrop?sslmode=disable"
	defaultRedisAddr    = "localhost:6379"
	defaultRedisDB      = 0
	defaultS3Endpoint   = "localhost:9000"
	defaultS3Region     = ""
	defaultRawBucket    = "vaultdrop-raw"
	defaultProcessedBucket = "vaultdrop-processed"
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
		DatabaseURL:    readEnv("VAULTDROP_DATABASE_URL", defaultDatabaseURL),
		RedisAddr:      readEnv("VAULTDROP_REDIS_ADDR", defaultRedisAddr),
		RedisPassword:  readEnv("VAULTDROP_REDIS_PASSWORD", ""),
		RedisDB:        parseInt("VAULTDROP_REDIS_DB", defaultRedisDB),
		S3Endpoint:     readEnv("VAULTDROP_S3_ENDPOINT", defaultS3Endpoint),
		S3AccessKey:    readEnv("VAULTDROP_S3_ACCESS_KEY", "minioadmin"),
		S3SecretKey:    readEnv("VAULTDROP_S3_SECRET_KEY", "minioadmin"),
		S3UseSSL:       parseBool("VAULTDROP_S3_USE_SSL", false),
		S3Region:       readEnv("VAULTDROP_S3_REGION", defaultS3Region),
		RawBucket:      readEnv("VAULTDROP_S3_RAW_BUCKET", defaultRawBucket),
		ProcessedBucket: readEnv("VAULTDROP_S3_PROCESSED_BUCKET", defaultProcessedBucket),
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

func parseBool(key string, def bool) bool {
	if v, ok := os.LookupEnv(key); ok && v != "" {
		if parsed, err := strconv.ParseBool(v); err == nil {
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
