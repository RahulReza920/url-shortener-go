// Package config loads runtime configuration from environment variables.
package config

import "os"

type Config struct {
	Port               string
	RedisAddr          string
	PASETOSecretKeyHex string // ed25519 private key, hex-encoded; generated on first run if empty
	SafeBrowsingAPIKey string // optional; malicious-URL check is skipped if empty
}

func Load() Config {
	return Config{
		Port:               getenv("PORT", "8080"),
		RedisAddr:          getenv("REDIS_ADDR", "localhost:6379"),
		PASETOSecretKeyHex: os.Getenv("PASETO_SECRET_KEY"),
		SafeBrowsingAPIKey: os.Getenv("SAFE_BROWSING_API_KEY"),
	}
}

func getenv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
