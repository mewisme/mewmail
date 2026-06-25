package config

import (
	"fmt"
	"os"
	"strconv"
	"strings"
)

const (
	defaultPort              = "8080"
	defaultAPIHost           = "0.0.0.0"
	defaultDBPath            = "/data/mail.db"
	defaultRetentionHours    = 168 // 7 days
	credentialsPath          = "/data/.credentials"
	defaultMaxBodyBytes      = 10 << 20 // 10 MiB
	defaultRequestTimeoutSec = 30
)

// Config holds application configuration from environment variables.
type Config struct {
	Port                string
	APIHost             string
	DBPath              string
	EmailRetentionHours int
	CredentialsPath     string
	MaxBodyBytes        int64
	RequestTimeoutSec   int
	WebhookURL          string
	PublicURL           string
}

// Load reads configuration from the environment.
func Load() (*Config, error) {
	retention := defaultRetentionHours
	if v := os.Getenv("EMAIL_RETENTION_HOURS"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil || n < 1 {
			return nil, fmt.Errorf("EMAIL_RETENTION_HOURS must be a positive integer")
		}
		retention = n
	}

	port := envOr("PORT", defaultPort)
	apiHost := envOr("API_HOST", defaultAPIHost)
	dbPath := envOr("DB_PATH", defaultDBPath)

	return &Config{
		Port:                port,
		APIHost:             apiHost,
		DBPath:              dbPath,
		EmailRetentionHours: retention,
		CredentialsPath:     credentialsPath,
		MaxBodyBytes:        defaultMaxBodyBytes,
		RequestTimeoutSec:   defaultRequestTimeoutSec,
		WebhookURL:          strings.TrimSpace(os.Getenv("WEBHOOK_URL")),
		PublicURL:           strings.TrimRight(strings.TrimSpace(os.Getenv("PUBLIC_URL")), "/"),
	}, nil
}

func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
