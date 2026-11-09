//go:build windows

package store

import (
	"encoding/base64"
	"fmt"
	"syscall"
	"unsafe"
)

var (
	crypt32            = syscall.NewLazyDLL("crypt32.dll")
	procCryptProtect   = crypt32.NewProc("CryptProtectData")
	procCryptUnprotect = crypt32.NewProc("CryptUnprotectData")
	kernel32Free       = syscall.NewLazyDLL("kernel32.dll").NewProc("LocalFree")
)

// blob is CRYPT_DATA_BLOB.
type blob struct {
	size uint32
	data *byte
}

// Protect encrypts plain with the current user's DPAPI: only this user on
// this machine can Unprotect it.
func Protect(plain string) (string, error) {
	if plain == "" {
		return "", nil
	}
	in := toBlob([]byte(plain))
	var out blob
	r, _, err := procCryptProtect.Call(
		uintptr(unsafe.Pointer(&in)), // pDataIn
		0,                            // szDataDescr
		0,                            // pOptionalEntropy
		0,                            // pvReserved
		0,                            // pPromptStruct
		0,                            // dwFlags
		uintptr(unsafe.Pointer(&out)))
	if r == 0 {
		return "", fmt.Errorf("store: CryptProtectData: %w", err)
	}
	defer freeBlob(out)
	return base64.StdEncoding.EncodeToString(unsafe.Slice(out.data, out.size)), nil
}

// Unprotect decrypts what Protect wrote.
func Unprotect(cipher string) (string, error) {
	if cipher == "" {
		return "", nil
	}
	raw, err := base64.StdEncoding.DecodeString(cipher)
	if err != nil {
		return "", fmt.Errorf("store: secret is not base64: %w", err)
	}
	in := toBlob(raw)
	var out blob
	r, _, err := procCryptUnprotect.Call(
		uintptr(unsafe.Pointer(&in)), // pDataIn
		0,                            // ppszDataDescr
		0,                            // pOptionalEntropy
		0,                            // pvReserved
		0,                            // pPromptStruct
		0,                            // dwFlags
		uintptr(unsafe.Pointer(&out)))
	if r == 0 {
		return "", fmt.Errorf("store: CryptUnprotectData: %w", err)
	}
	defer freeBlob(out)
	return string(unsafe.Slice(out.data, out.size)), nil
}

func toBlob(b []byte) blob {
	if len(b) == 0 {
		return blob{size: 0, data: nil}
	}
	return blob{size: uint32(len(b)), data: &b[0]}
}

func freeBlob(b blob) {
	if b.data != nil {
		kernel32Free.Call(uintptr(unsafe.Pointer(b.data)))
	}
}