package auth

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"net/http"
	"sync"
	"time"
)

type APIKey struct {
	Key       string
	ClientID  string
	IsActive  bool
	CreatedAt int64
}

type Manager struct {
	keys map[string]APIKey
	mu   sync.RWMutex
}

func NewManager() *Manager {
	return &Manager{
		keys: make(map[string]APIKey),
	}
}

// GenerateAPIKey creates a new API key with 32 bytes of entropy
func (m *Manager) GenerateAPIKey(clientID string) (APIKey, error) {
	keyBytes := make([]byte, 32)
	if _, err := rand.Read(keyBytes); err != nil {
		return APIKey{}, err
	}

	apiKey := APIKey{
		Key:       hex.EncodeToString(keyBytes),
		ClientID:  clientID,
		IsActive:  true,
		CreatedAt: time.Now().Unix(),
	}

	m.mu.Lock()
	m.keys[apiKey.Key] = apiKey
	m.mu.Unlock()

	return apiKey, nil
}

// ValidateKey checks if an API key is valid
func (m *Manager) ValidateKey(key string) bool {
	m.mu.RLock()
	apiKey, exists := m.keys[key]
	m.mu.RUnlock()
	return exists && apiKey.IsActive
}

// GetClientID returns the client ID associated with an API key
func (m *Manager) GetClientID(key string) (string, error) {
	m.mu.RLock()
	apiKey, exists := m.keys[key]
	m.mu.RUnlock()

	if !exists || !apiKey.IsActive {
		return "", errors.New("invalid API key")
	}
	return apiKey.ClientID, nil
}

// Middleware creates an authentication middleware
func (m *Manager) Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		apiKey := r.Header.Get("X-API-Key")
		if apiKey == "" {
			http.Error(w, "API key required", http.StatusUnauthorized)
			return
		}

		if !m.ValidateKey(apiKey) {
			http.Error(w, "Invalid API key", http.StatusUnauthorized)
			return
		}

		next.ServeHTTP(w, r)
	})
}
