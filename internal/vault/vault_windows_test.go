//go:build windows

package vault

import (
	"bytes"
	"testing"
)

func TestProtectUnprotectRoundTrip(t *testing.T) {
	secret := []byte("sk-test-placeholder-not-a-real-key")
	ct, err := Protect(secret)
	if err != nil {
		t.Fatalf("protect: %v", err)
	}
	if len(ct) == 0 {
		t.Fatal("ciphertext is empty")
	}
	if bytes.Equal(ct, secret) {
		t.Fatal("ciphertext equals plaintext")
	}

	pt, err := Unprotect(ct)
	if err != nil {
		t.Fatalf("unprotect: %v", err)
	}
	if !bytes.Equal(pt, secret) {
		t.Fatalf("round trip mismatch: got %q", pt)
	}
}

// AC-13: nothing written to disk may contain the plaintext key.
func TestCiphertextHidesPlaintext(t *testing.T) {
	secret := []byte("placeholder-credential-value-0123456789")
	ct, err := Protect(secret)
	if err != nil {
		t.Fatalf("protect: %v", err)
	}
	if bytes.Contains(ct, secret) {
		t.Fatal("ciphertext embeds the plaintext secret")
	}
	if bytes.Contains(ct, []byte("placeholder")) {
		t.Fatal("ciphertext leaks a plaintext fragment")
	}
}

func TestProtectRejectsEmpty(t *testing.T) {
	if _, err := Protect(nil); err == nil {
		t.Fatal("empty plaintext must be rejected")
	}
	if _, err := Unprotect(nil); err == nil {
		t.Fatal("empty ciphertext must be rejected")
	}
}

func TestUnprotectRejectsTampered(t *testing.T) {
	ct, err := Protect([]byte("value"))
	if err != nil {
		t.Fatalf("protect: %v", err)
	}
	// Flip a byte in the middle of the blob.
	ct[len(ct)/2] ^= 0xff
	if _, err = Unprotect(ct); err == nil {
		t.Fatal("tampered ciphertext must fail to decrypt")
	}
}

func TestAvailable(t *testing.T) {
	if !Available() {
		t.Fatal("DPAPI must report available on windows")
	}
}
