package matrix

import (
	"context"
	"fmt"
	"log"

	"maunium.net/go/mautrix"
	"maunium.net/go/mautrix/crypto/cryptohelper"
)

func InitCrypto(client *mautrix.Client, dbPath, pickleKey string) error {
	if dbPath == "" {
		log.Println("⚠️ Warning: Crypto DB path not set. E2EE disabled.")
		return nil
	}

	pKey := []byte(pickleKey)
	if len(pKey) == 0 {
		pKey = []byte("default-pickle-key")
	}

	helper, err := cryptohelper.NewCryptoHelper(client, pKey, dbPath)
	if err != nil {
		return fmt.Errorf("failed to create crypto helper: %w", err)
	}

	if err := helper.Init(context.Background()); err != nil {
		return fmt.Errorf("failed to init crypto: %w", err)
	}

	client.Crypto = helper
	log.Println("🔒 End-to-End Encryption initialized")

	return nil
}
