package secretbox

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"
)

func Encrypt(masterKeyPath string, plaintext []byte) ([]byte, error) {
	master, err := os.ReadFile(masterKeyPath)
	if err != nil {
		return nil, fmt.Errorf("read master key: %w", err)
	}
	key := sha256.Sum256([]byte(strings.TrimSpace(string(master))))
	block, err := aes.NewCipher(key[:])
	if err != nil {
		return nil, fmt.Errorf("create cipher: %w", err)
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("create gcm: %w", err)
	}
	nonce := make([]byte, gcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return nil, fmt.Errorf("read nonce: %w", err)
	}
	envelope, err := json.Marshal(map[string]string{
		"alg":        "AES-256-GCM",
		"nonce":      base64.StdEncoding.EncodeToString(nonce),
		"ciphertext": base64.StdEncoding.EncodeToString(gcm.Seal(nil, nonce, plaintext, nil)),
	})
	if err != nil {
		return nil, fmt.Errorf("encode encrypted secret: %w", err)
	}
	return envelope, nil
}

func Decrypt(masterKeyPath string, encrypted []byte) ([]byte, error) {
	var envelope struct {
		Alg        string `json:"alg"`
		Nonce      string `json:"nonce"`
		Ciphertext string `json:"ciphertext"`
	}
	if err := json.Unmarshal(encrypted, &envelope); err != nil {
		return nil, fmt.Errorf("decode encrypted secret envelope: %w", err)
	}
	if envelope.Alg != "AES-256-GCM" || envelope.Nonce == "" || envelope.Ciphertext == "" {
		return nil, fmt.Errorf("invalid encrypted secret envelope")
	}
	nonce, err := base64.StdEncoding.DecodeString(envelope.Nonce)
	if err != nil {
		return nil, fmt.Errorf("decode encrypted secret nonce: %w", err)
	}
	ciphertext, err := base64.StdEncoding.DecodeString(envelope.Ciphertext)
	if err != nil {
		return nil, fmt.Errorf("decode encrypted secret ciphertext: %w", err)
	}
	master, err := os.ReadFile(masterKeyPath)
	if err != nil {
		return nil, fmt.Errorf("read master key: %w", err)
	}
	key := sha256.Sum256([]byte(strings.TrimSpace(string(master))))
	block, err := aes.NewCipher(key[:])
	if err != nil {
		return nil, fmt.Errorf("create cipher: %w", err)
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("create gcm: %w", err)
	}
	plaintext, err := gcm.Open(nil, nonce, ciphertext, nil)
	if err != nil {
		return nil, fmt.Errorf("decrypt secret: %w", err)
	}
	return plaintext, nil
}
