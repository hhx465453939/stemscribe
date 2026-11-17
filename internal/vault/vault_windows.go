//go:build windows

// Package vault protects secrets with Windows DPAPI (ADR-008).
//
// DPAPI is chosen over a user-supplied master password because the threat model
// is "another user or a stolen disk", not "an attacker with the user's session".
// The consequence — the ciphertext is tied to the Windows account — is documented
// in SECURITY.md and surfaced in the FAQ.
package vault

import (
	"errors"
	"fmt"
	"syscall"
	"unsafe"
)

var (
	crypt32                = syscall.NewLazyDLL("crypt32.dll")
	procCryptProtectData   = crypt32.NewProc("CryptProtectData")
	procCryptUnprotectData = crypt32.NewProc("CryptUnprotectData")
	kernel32               = syscall.NewLazyDLL("kernel32.dll")
	procLocalFree          = kernel32.NewProc("LocalFree")
)

type dataBlob struct {
	cbData uint32
	pbData *byte
}

func newBlob(d []byte) *dataBlob {
	if len(d) == 0 {
		return &dataBlob{}
	}
	return &dataBlob{cbData: uint32(len(d)), pbData: &d[0]}
}

func (b *dataBlob) bytes() []byte {
	if b.pbData == nil || b.cbData == 0 {
		return nil
	}
	src := unsafe.Slice(b.pbData, b.cbData)
	out := make([]byte, len(src))
	copy(out, src)
	return out
}

func (b *dataBlob) free() {
	if b.pbData != nil {
		_, _, _ = procLocalFree.Call(uintptr(unsafe.Pointer(b.pbData)))
		b.pbData = nil
		b.cbData = 0
	}
}

// Protect encrypts plaintext for the current Windows user.
func Protect(plaintext []byte) ([]byte, error) {
	if len(plaintext) == 0 {
		return nil, errors.New("nothing to protect")
	}
	in := newBlob(plaintext)
	var out dataBlob

	// CRYPTPROTECT_UI_FORBIDDEN = 0x1: never pop a UI from a background worker.
	ret, _, err := procCryptProtectData.Call(
		uintptr(unsafe.Pointer(in)),
		0, // description
		0, // optional entropy
		0, // reserved
		0, // prompt struct
		uintptr(1),
		uintptr(unsafe.Pointer(&out)),
	)
	if ret == 0 {
		return nil, fmt.Errorf("CryptProtectData: %w", err)
	}
	defer out.free()
	return out.bytes(), nil
}

// Unprotect decrypts data produced by Protect for the current Windows user.
func Unprotect(ciphertext []byte) ([]byte, error) {
	if len(ciphertext) == 0 {
		return nil, errors.New("nothing to unprotect")
	}
	in := newBlob(ciphertext)
	var out dataBlob

	ret, _, err := procCryptUnprotectData.Call(
		uintptr(unsafe.Pointer(in)),
		0,
		0,
		0,
		0,
		uintptr(1),
		uintptr(unsafe.Pointer(&out)),
	)
	if ret == 0 {
		return nil, fmt.Errorf("CryptUnprotectData: %w", err)
	}
	defer out.free()
	return out.bytes(), nil
}

// Available reports whether the platform provides a working secret store.
func Available() bool { return true }
