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

// Credentials holds role-scoped API keys loaded from disk.
type Credentials struct {
	ExternalAPIKey string
	InternalAPIKey string
}

// CredentialChanges reports which keys were newly generated on load.
type CredentialChanges struct {
	CreatedExternal bool
	CreatedInternal bool
}

// LoadOrCreateCredentials loads keys from path or generates missing ones.
func LoadOrCreateCredentials(path string) (Credentials, CredentialChanges, error) {
	data, err := os.ReadFile(path)
	if err != nil && !os.IsNotExist(err) {
		return Credentials{}, CredentialChanges{}, fmt.Errorf("read credentials: %w", err)
	}

	var cred models.CredentialsFile
	var changes CredentialChanges

	if err == nil {
		if err := json.Unmarshal(data, &cred); err != nil {
			return Credentials{}, CredentialChanges{}, fmt.Errorf("parse credentials: %w", err)
		}
		if cred.CreatedAt.IsZero() {
			cred.CreatedAt = time.Now().UTC()
		}
	} else {
		cred.CreatedAt = time.Now().UTC()
	}

	if cred.APIKey == "" {
		key, err := generateKey()
		if err != nil {
			return Credentials{}, CredentialChanges{}, err
		}
		cred.APIKey = key
		changes.CreatedExternal = true
	}
	if cred.InternalKey == "" {
		key, err := generateKey()
		if err != nil {
			return Credentials{}, CredentialChanges{}, err
		}
		cred.InternalKey = key
		changes.CreatedInternal = true
	}

	if changes.CreatedExternal || changes.CreatedInternal {
		if err := writeCredentials(path, cred); err != nil {
			return Credentials{}, CredentialChanges{}, err
		}
	}

	return Credentials{
		ExternalAPIKey: cred.APIKey,
		InternalAPIKey: cred.InternalKey,
	}, changes, nil
}

func writeCredentials(path string, cred models.CredentialsFile) error {
	out, err := json.MarshalIndent(cred, "", "  ")
	if err != nil {
		return err
	}
	// ponytail: 0644 so postfix pipe (nobody) can read shared Docker volume
	if err := os.WriteFile(path, out, 0o644); err != nil {
		return fmt.Errorf("write credentials: %w", err)
	}
	return nil
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
