//go:build !windows

package vault

import "errors"

// ErrUnsupported is returned on platforms without a DPAPI equivalent yet.
// Linux/macOS keyring support is tracked as technical debt in CHG-0004 rather
// than silently stored in plaintext.
var ErrUnsupported = errors.New("secret storage is only implemented for windows")

// Protect always fails outside Windows: falling back to plaintext would violate
// ADR-008 and SECURITY.md.
func Protect(plaintext []byte) ([]byte, error) { return nil, ErrUnsupported }

// Unprotect always fails outside Windows.
func Unprotect(ciphertext []byte) ([]byte, error) { return nil, ErrUnsupported }

// Available reports whether a secret store exists.
func Available() bool { return false }
