package crypto

import "testing"

func TestSecretBoxEncryptDecrypt(t *testing.T) {
	box, err := NewSecretBox("12345678901234567890123456789012")
	if err != nil {
		t.Fatalf("NewSecretBox() error = %v", err)
	}

	encrypted, err := box.Encrypt("secret-value")
	if err != nil {
		t.Fatalf("Encrypt() error = %v", err)
	}
	if encrypted == "" || encrypted == "secret-value" {
		t.Fatalf("Encrypt() returned unsafe value %q", encrypted)
	}

	decrypted, err := box.Decrypt(encrypted)
	if err != nil {
		t.Fatalf("Decrypt() error = %v", err)
	}
	if decrypted != "secret-value" {
		t.Fatalf("Decrypt() = %q, want %q", decrypted, "secret-value")
	}
}
