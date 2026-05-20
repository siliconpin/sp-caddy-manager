package functions

import (
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

var apiKeyLabelPattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]{0,63}$`)

type APIKeyStore struct {
	Dir string
}

func NewAPIKeyStore(dir string) *APIKeyStore {
	return &APIKeyStore{Dir: dir}
}

func (s *APIKeyStore) Add(label string) (string, string, error) {
	label, err := normalizeAPIKeyLabel(label)
	if err != nil {
		return "", "", err
	}
	if err := os.MkdirAll(s.Dir, 0700); err != nil {
		return "", "", err
	}

	path := s.keyPath(label)
	if _, err := os.Stat(path); err == nil {
		return "", "", fmt.Errorf("key label already exists")
	} else if !os.IsNotExist(err) {
		return "", "", err
	}

	key, err := generateAPIKey()
	if err != nil {
		return "", "", err
	}
	if err := os.WriteFile(path, []byte(hashAPIKey(key)+"\n"), 0600); err != nil {
		return "", "", err
	}

	return key, path, nil
}

func (s *APIKeyStore) List() ([]string, error) {
	entries, err := os.ReadDir(s.Dir)
	if os.IsNotExist(err) {
		return []string{}, nil
	}
	if err != nil {
		return nil, err
	}

	labels := make([]string, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".key") {
			continue
		}
		label := strings.TrimSuffix(entry.Name(), ".key")
		if _, err := normalizeAPIKeyLabel(label); err == nil {
			labels = append(labels, label)
		}
	}
	sort.Strings(labels)
	return labels, nil
}

func (s *APIKeyStore) Delete(label string) error {
	label, err := normalizeAPIKeyLabel(label)
	if err != nil {
		return err
	}
	path := s.keyPath(label)
	if err := os.Remove(path); err != nil {
		if os.IsNotExist(err) {
			return fmt.Errorf("key label not found")
		}
		return err
	}
	return nil
}

func (s *APIKeyStore) Valid(key string) (bool, error) {
	key = strings.TrimSpace(key)
	if key == "" {
		return false, nil
	}

	entries, err := os.ReadDir(s.Dir)
	if err != nil {
		return false, err
	}

	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".key") {
			continue
		}
		storedBytes, err := os.ReadFile(filepath.Join(s.Dir, entry.Name()))
		if err != nil {
			return false, err
		}
		stored := strings.TrimSpace(string(storedBytes))
		if validStoredAPIKey(stored, key) {
			return true, nil
		}
	}

	return false, nil
}

func (s *APIKeyStore) ValidForLabel(label, key string) (bool, error) {
	label, err := normalizeAPIKeyLabel(label)
	if err != nil {
		return false, err
	}
	key = strings.TrimSpace(key)
	if key == "" {
		return false, nil
	}

	storedBytes, err := os.ReadFile(s.keyPath(label))
	if os.IsNotExist(err) {
		return false, nil
	}
	if err != nil {
		return false, err
	}

	return validStoredAPIKey(strings.TrimSpace(string(storedBytes)), key), nil
}

func (s *APIKeyStore) HasKeys() (bool, error) {
	labels, err := s.List()
	if err != nil {
		return false, err
	}
	return len(labels) > 0, nil
}

func (s *APIKeyStore) keyPath(label string) string {
	return filepath.Join(s.Dir, label+".key")
}

func normalizeAPIKeyLabel(label string) (string, error) {
	label = strings.TrimSpace(label)
	if !apiKeyLabelPattern.MatchString(label) {
		return "", fmt.Errorf("label must be 1-64 characters and use only letters, numbers, dot, underscore, or hyphen")
	}
	if strings.Contains(label, "..") {
		return "", fmt.Errorf("label cannot contain '..'")
	}
	return label, nil
}

func generateAPIKey() (string, error) {
	raw := make([]byte, 32)
	if _, err := rand.Read(raw); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(raw), nil
}

func hashAPIKey(key string) string {
	sum := sha256.Sum256([]byte(key))
	return "sha256:" + hex.EncodeToString(sum[:])
}

func validStoredAPIKey(stored, key string) bool {
	hashed := hashAPIKey(key)
	if subtle.ConstantTimeCompare([]byte(stored), []byte(hashed)) == 1 {
		return true
	}

	// Accept old plaintext key files so existing installs do not lock users out.
	return !strings.HasPrefix(stored, "sha256:") &&
		subtle.ConstantTimeCompare([]byte(stored), []byte(key)) == 1
}
