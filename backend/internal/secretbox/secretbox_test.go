package secretbox

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
)

func TestEncryptDecryptRoundTrip(t *testing.T) {
	root := t.TempDir()
	keyPath := filepath.Join(root, "master.key")
	if err := os.WriteFile(keyPath, []byte("test-master-key\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	plaintext := []byte(`{"secret_id":"id","secret_key":"key"}`)
	encrypted, err := Encrypt(keyPath, plaintext)
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(encrypted, []byte("secret_key")) || bytes.Equal(encrypted, plaintext) {
		t.Fatal("encrypted envelope contains plaintext")
	}
	decrypted, err := Decrypt(keyPath, encrypted)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(decrypted, plaintext) {
		t.Fatalf("decrypted %q", decrypted)
	}

	wrongKeyPath := filepath.Join(root, "wrong.key")
	if err := os.WriteFile(wrongKeyPath, []byte("wrong-master-key"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := Decrypt(wrongKeyPath, encrypted); err == nil {
		t.Fatal("expected decryption with the wrong key to fail")
	}
}
