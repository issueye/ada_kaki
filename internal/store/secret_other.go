//go:build !windows

package store

import "errors"

// Protect has no implementation off Windows: secrets stay empty there.
func Protect(plain string) (string, error) {
	if plain == "" {
		return "", nil
	}
	return "", errors.New("store: secret encryption needs Windows")
}

// Unprotect has no implementation off Windows.
func Unprotect(cipher string) (string, error) {
	return "", errors.New("store: secret decryption needs Windows")
}
