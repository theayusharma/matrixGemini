package core

import (
	"crypto/rand"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"sync"
	"time"

	"golang.org/x/crypto/nacl/secretbox"
)

type CreditsConfig struct {
	FilePath    string `toml:"file_path"`
	GlobalLimit int    `toml:"global_limit"`
	MasterKey   string `toml:"master_key"`
}

type UserCredit struct {
	UserID        string   `json:"user_id"`
	TokenCount    int      `json:"token_count"`
	APIKey        []byte   `json:"api_key"`
	Nonce         [24]byte `json:"nonce"`
	SearchEnabled bool     `json:"search_enabled"`
}

type CreditManager struct {
	mu          sync.RWMutex
	users       map[string]*UserCredit
	filePath    string
	masterKey   [32]byte
	globalLimit int
	dirty       bool
}

func NewCreditManager(cfg CreditsConfig) *CreditManager {
	cm := &CreditManager{
		users:       make(map[string]*UserCredit),
		filePath:    cfg.FilePath,
		globalLimit: cfg.GlobalLimit,
	}

	keyBytes := make([]byte, 32)
	copy(keyBytes, []byte(cfg.MasterKey))
	copy(cm.masterKey[:], keyBytes)

	cm.loadFromFile()
	go cm.autoSaveLoop()
	return cm
}

func (cm *CreditManager) autoSaveLoop() {
	ticker := time.NewTicker(1 * time.Minute)
	for range ticker.C {
		cm.mu.Lock()
		if cm.dirty {
			cm.saveToFile()
			cm.dirty = false
		}
		cm.mu.Unlock()
	}
}

func (cm *CreditManager) ForceSave() {
	cm.mu.Lock()
	defer cm.mu.Unlock()

	cm.saveToFileUnsafe()
}

func (cm *CreditManager) loadFromFile() {
	cm.mu.Lock()
	defer cm.mu.Unlock()

	data, err := os.ReadFile(cm.filePath)
	if err != nil {
		return
	}

	json.Unmarshal(data, &cm.users)
}

func (cm *CreditManager) saveToFileUnsafe() {
	data, _ := json.Marshal(cm.users)
	_ = os.WriteFile(cm.filePath, data, 0600)
}

func (cm *CreditManager) saveToFile() {
	data, err := json.Marshal(cm.users)
	if err != nil {
		log.Printf("Failed to marshal credits: %v", err)
		return
	}

	if err := os.WriteFile(cm.filePath, data, 0600); err != nil {
		log.Printf("Failed to save credits file: %v", err)
	}
}

func (cm *CreditManager) encryptAPIKey(apiKey string) ([]byte, [24]byte, error) {
	var nonce [24]byte
	if _, err := rand.Read(nonce[:]); err != nil {
		return nil, nonce, err
	}
	encrypted := secretbox.Seal(nil, []byte(apiKey), &nonce, &cm.masterKey)
	return encrypted, nonce, nil
}

func (cm *CreditManager) decryptAPIKey(encrypted []byte, nonce [24]byte) (string, error) {
	decrypted, ok := secretbox.Open(nil, encrypted, &nonce, &cm.masterKey)
	if !ok {
		return "", fmt.Errorf("failed to decrypt API key")
	}
	return string(decrypted), nil
}

func (cm *CreditManager) SetUserAPIKey(userID string, apiKey string) error {
	cm.mu.Lock()
	defer cm.mu.Unlock()

	encrypted, nonce, err := cm.encryptAPIKey(apiKey)
	if err != nil {
		return err
	}

	userIDStr := string(userID)
	if cm.users[userIDStr] == nil {
		cm.users[userIDStr] = &UserCredit{UserID: userIDStr}
	}

	cm.users[userIDStr].APIKey = encrypted
	cm.users[userIDStr].Nonce = nonce

	cm.saveToFile()
	return nil
}

func (cm *CreditManager) GetUserAPIKey(userID string) (string, error) {
	cm.mu.RLock()
	defer cm.mu.RUnlock()

	user, exists := cm.users[string(userID)]
	if !exists || user.APIKey == nil {
		return "", fmt.Errorf("no API key found for user")
	}

	return cm.decryptAPIKey(user.APIKey, user.Nonce)
}

func (cm *CreditManager) CanUseAPI(userID string) bool {
	cm.mu.RLock()
	defer cm.mu.RUnlock()

	user, exists := cm.users[string(userID)]

	if exists && user.APIKey != nil {
		return true
	}

	if exists && user.TokenCount >= cm.globalLimit {
		return false
	}

	return true
}

func (cm *CreditManager) RecordUsage(userID string, tokens int) {
	cm.mu.Lock()
	defer cm.mu.Unlock()

	if _, exists := cm.users[userID]; !exists {
		cm.users[userID] = &UserCredit{UserID: userID}
	}

	if cm.users[userID].APIKey == nil {
		cm.users[userID].TokenCount += tokens
	}
	cm.dirty = true
}

func (cm *CreditManager) GetUserStats(userID string) (int, bool) {
	cm.mu.RLock()
	defer cm.mu.RUnlock()

	user, exists := cm.users[string(userID)]
	if !exists {
		return 0, false
	}

	hasOwnKey := user.APIKey != nil
	return user.TokenCount, hasOwnKey
}

func (cm *CreditManager) SetSearchEnabled(userID string, enabled bool) {
	cm.mu.Lock()
	defer cm.mu.Unlock()

	userIDStr := string(userID)
	if cm.users[userIDStr] == nil {
		cm.users[userIDStr] = &UserCredit{UserID: userIDStr}
	}

	cm.users[userIDStr].SearchEnabled = enabled
	cm.saveToFile()
}

func (cm *CreditManager) IsSearchEnabled(userID string) bool {
	cm.mu.RLock()
	defer cm.mu.RUnlock()

	user, exists := cm.users[string(userID)]
	if !exists {
		return false
	}
	return user.SearchEnabled
}
