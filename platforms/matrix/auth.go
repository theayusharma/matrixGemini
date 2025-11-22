package matrix

import (
	"context"
	"crypto/rand"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"syscall"

	"golang.org/x/crypto/argon2"
	"golang.org/x/crypto/nacl/secretbox"
	"golang.org/x/term"
	"maunium.net/go/mautrix"
	"maunium.net/go/mautrix/id"
)

type Config struct {
	Homeserver        string `toml:"homeserver"`
	UserID            string `toml:"user_id"`
	CredentialsDBPath string `toml:"credentials_db_path"`
	CryptoDBPath      string `toml:"crypto_db_path"`
	PickleKey         string `toml:"pickle_key"`
	AutoJoinInvites   bool   `toml:"auto_join_invites"`
}

type CredentialStore struct {
	Homeserver    string   `json:"homeserver"`
	UserID        string   `json:"user_id"`
	DeviceID      string   `json:"device_id"`
	EncryptedData []byte   `json:"encrypted_data"`
	Nonce         [24]byte `json:"nonce"`
	Salt          []byte   `json:"salt"`
}

func deriveKey(password string, salt []byte) [32]byte {
	derived := argon2.IDKey([]byte(password), salt, 1, 64*1024, 4, 32)

	var key [32]byte
	copy(key[:], derived)
	return key
}

func getEncryptionKey(password string) [32]byte {
	key := [32]byte{}
	copy(key[:], password)
	// todo: use argon2
	for i := len(password); i < 32; i++ {
		key[i] = 0xFF // Padding
	}
	return key
}

func getPassword() (string, error) {
	if password := os.Getenv("MATRIX_PASSWORD"); password != "" {
		return password, nil
	}

	fmt.Print("🔑 Enter Matrix password (or set MATRIX_PASSWORD env var): ")
	bytePassword, err := term.ReadPassword(int(syscall.Stdin))
	fmt.Println()
	if err != nil {
		return "", err
	}
	return string(bytePassword), nil
}

func loadCredentials(dbPath, password string) (*mautrix.Client, error) {
	data, err := os.ReadFile(dbPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read credentials file: %w", err)
	}

	var store CredentialStore
	if err := json.Unmarshal(data, &store); err != nil {
		return nil, fmt.Errorf("failed to parse credentials file: %w", err)
	}

	// future: remove this
	if len(store.Salt) == 0 {
		return nil, errors.New("legacy credentials file detected (no salt). Please delete the credentials.json file and log in again to upgrade security")
	}

	key := deriveKey(password, store.Salt)
	decrypted, ok := secretbox.Open(nil, store.EncryptedData, &store.Nonce, &key)
	if !ok {
		return nil, errors.New("failed to decrypt credentials - wrong password?")
	}

	client, err := mautrix.NewClient(store.Homeserver, id.UserID(store.UserID), string(decrypted))
	if err != nil {
		return nil, fmt.Errorf("failed to create client: %w", err)
	}

	client.DeviceID = id.DeviceID(store.DeviceID)
	return client, nil
}

func loginAndSaveCredentials(homeserver, userID, password, dbPath string) (*mautrix.Client, error) {
	fmt.Printf("Logging into %s as %s...\n", homeserver, userID)

	client, err := mautrix.NewClient(homeserver, id.UserID(userID), "")
	if err != nil {
		return nil, fmt.Errorf("failed to create client: %w", err)
	}

	resp, err := client.Login(context.Background(), &mautrix.ReqLogin{
		Type: "m.login.password",
		Identifier: mautrix.UserIdentifier{
			Type: "m.id.user",
			User: userID,
		},
		Password: password,
	})
	if err != nil {
		return nil, fmt.Errorf("login failed: %w", err)
	}

	client.AccessToken = resp.AccessToken
	client.DeviceID = resp.DeviceID

	salt := make([]byte, 16)
	if _, err := io.ReadFull(rand.Reader, salt); err != nil {
		return nil, fmt.Errorf("failed to generate salt: %w", err)
	}

	key := deriveKey(password, salt)
	var nonce [24]byte
	if _, err := io.ReadFull(rand.Reader, nonce[:]); err != nil {
		return nil, fmt.Errorf("failed to generate nonce: %w", err)
	}

	encrypted := secretbox.Seal(nil, []byte(resp.AccessToken), &nonce, &key)

	store := CredentialStore{
		Homeserver:    homeserver,
		UserID:        userID,
		DeviceID:      string(resp.DeviceID),
		EncryptedData: encrypted,
		Nonce:         nonce,
		Salt:          salt,
	}

	data, err := json.Marshal(store)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal credentials: %w", err)
	}

	if err := os.WriteFile(dbPath, data, 0600); err != nil {
		return nil, fmt.Errorf("failed to write credentials file: %w", err)
	}

	fmt.Println("Credentials saved to:", dbPath)
	return client, nil
}

func GetMatrixClient(config *Config) (*mautrix.Client, error) {
	password, err := getPassword()
	if err != nil {
		return nil, fmt.Errorf("failed to get password: %w", err)
	}

	if _, err := os.Stat(config.CredentialsDBPath); os.IsNotExist(err) {
		fmt.Println("First-time login detected...")
		return loginAndSaveCredentials(
			config.Homeserver,
			config.UserID,
			password,
			config.CredentialsDBPath,
		)
	}

	fmt.Println("Loading existing session...")
	return loadCredentials(config.CredentialsDBPath, password)
}
