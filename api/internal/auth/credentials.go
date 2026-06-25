package auth

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"time"

	"mewmail/api/internal/models"
)

// LoadOrCreateAPIKey loads the API key from path or generates a new one.
// Returns the key and whether it was newly generated.
func LoadOrCreateAPIKey(path string) (string, bool, error) {
	data, err := os.ReadFile(path)
	if err == nil {
		var cred models.CredentialsFile
		if err := json.Unmarshal(data, &cred); err != nil {
			return "", false, fmt.Errorf("parse credentials: %w", err)
		}
		if cred.APIKey == "" {
			return "", false, fmt.Errorf("credentials file has empty api_key")
		}
		return cred.APIKey, false, nil
	}
	if !os.IsNotExist(err) {
		return "", false, fmt.Errorf("read credentials: %w", err)
	}

	key, err := generateKey()
	if err != nil {
		return "", false, err
	}
	cred := models.CredentialsFile{
		APIKey:    key,
		CreatedAt: time.Now().UTC(),
	}
	out, err := json.MarshalIndent(cred, "", "  ")
	if err != nil {
		return "", false, err
	}
	// ponytail: 0644 so postfix pipe (nobody) can read shared Docker volume
	if err := os.WriteFile(path, out, 0o644); err != nil {
		return "", false, fmt.Errorf("write credentials: %w", err)
	}
	return key, true, nil
}

func generateKey() (string, error) {
	return GenerateToken()
}

// GenerateToken returns a random hex token (16 bytes).
func GenerateToken() (string, error) {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("generate token: %w", err)
	}
	return hex.EncodeToString(b), nil
}
