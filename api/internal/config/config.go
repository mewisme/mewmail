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
	defaultRetentionDays     = 7
	credentialsPath          = "/data/.credentials"
	defaultMaxBodyBytes      = 10 << 20 // 10 MiB
	defaultRequestTimeoutSec = 30
)

// Config holds application configuration from environment variables.
type Config struct {
	Port               string
	APIHost            string
	DBPath             string
	EmailRetentionDays int
	CredentialsPath    string
	MaxBodyBytes       int64
	RequestTimeoutSec  int
	AllowMultipart     bool
	WebhookURL         string
}

// Load reads configuration from the environment.
func Load() (*Config, error) {
	retention := defaultRetentionDays
	if v := os.Getenv("EMAIL_RETENTION_DAYS"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil || n < 1 {
			return nil, fmt.Errorf("EMAIL_RETENTION_DAYS must be a positive integer")
		}
		retention = n
	}

	allowMultipart := false
	if v := os.Getenv("ALLOW_MULTIPART"); v != "" {
		b, err := strconv.ParseBool(v)
		if err != nil {
			return nil, fmt.Errorf("ALLOW_MULTIPART must be true or false")
		}
		allowMultipart = b
	}

	port := envOr("PORT", defaultPort)
	apiHost := envOr("API_HOST", defaultAPIHost)
	dbPath := envOr("DB_PATH", defaultDBPath)

	return &Config{
		Port:               port,
		APIHost:            apiHost,
		DBPath:             dbPath,
		EmailRetentionDays: retention,
		CredentialsPath:    credentialsPath,
		MaxBodyBytes:       defaultMaxBodyBytes,
		RequestTimeoutSec:  defaultRequestTimeoutSec,
		AllowMultipart:     allowMultipart,
		WebhookURL:         strings.TrimSpace(os.Getenv("WEBHOOK_URL")),
	}, nil
}

func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
