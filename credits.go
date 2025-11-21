package main

import (
	"crypto/rand"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"sync"

	"golang.org/x/crypto/nacl/secretbox"
	"maunium.net/go/mautrix/id"
)

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
}

func NewCreditManager(filePath string, globalLimit int, masterKeyStr string) *CreditManager {
	cm := &CreditManager{
		users:       make(map[string]*UserCredit),
		filePath:    filePath,
		globalLimit: globalLimit,
	}

	copy(cm.masterKey[:], masterKeyStr)

	cm.loadFromFile()
	return cm
}

func (cm *CreditManager) loadFromFile() {
	cm.mu.Lock()
	defer cm.mu.Unlock()

	data, err := os.ReadFile(cm.filePath)
	if err != nil {
		if os.IsNotExist(err) {
			return
		}
		log.Printf("Failed to load credits file: %v", err)
		return
	}

	if err := json.Unmarshal(data, &cm.users); err != nil {
		log.Printf("Failed to parse credits file: %v", err)
	}
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

func (cm *CreditManager) SetUserAPIKey(userID id.UserID, apiKey string) error {
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

func (cm *CreditManager) GetUserAPIKey(userID id.UserID) (string, error) {
	cm.mu.RLock()
	defer cm.mu.RUnlock()

	user, exists := cm.users[string(userID)]
	if !exists || user.APIKey == nil {
		return "", fmt.Errorf("no API key found for user")
	}

	return cm.decryptAPIKey(user.APIKey, user.Nonce)
}

func (cm *CreditManager) CanUseAPI(userID id.UserID) bool {
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

func (cm *CreditManager) RecordUsage(userID id.UserID, tokens int) {
	cm.mu.Lock()
	defer cm.mu.Unlock()

	userIDStr := string(userID)
	user, exists := cm.users[userIDStr]
	if !exists {
		user = &UserCredit{UserID: userIDStr}
		cm.users[userIDStr] = user
	}

	if user.APIKey == nil {
		user.TokenCount += tokens
	}

	cm.saveToFile()
}

func (cm *CreditManager) GetUserStats(userID id.UserID) (int, bool) {
	cm.mu.RLock()
	defer cm.mu.RUnlock()

	user, exists := cm.users[string(userID)]
	if !exists {
		return 0, false
	}

	hasOwnKey := user.APIKey != nil
	return user.TokenCount, hasOwnKey
}

func (cm *CreditManager) SetSearchEnabled(userID id.UserID, enabled bool) {
	cm.mu.Lock()
	defer cm.mu.Unlock()

	userIDStr := string(userID)
	if cm.users[userIDStr] == nil {
		cm.users[userIDStr] = &UserCredit{UserID: userIDStr}
	}

	cm.users[userIDStr].SearchEnabled = enabled
	cm.saveToFile()
}

func (cm *CreditManager) IsSearchEnabled(userID id.UserID) bool {
	cm.mu.RLock()
	defer cm.mu.RUnlock()

	user, exists := cm.users[string(userID)]
	if !exists {
		return false
	}
	return user.SearchEnabled
}
