//go:build !windows

package keychain

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
)

const masterKeyBytes = 32
const nonceBytes = 12

var safeFileNameRe = regexp.MustCompile(`[^a-zA-Z0-9._-]`)

func platformGet(service, account string) (string, error) {
	path := filepath.Join(storageDir(service), safeFileName(account))
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return "", ErrNotFound
	}
	if err != nil {
		return "", err
	}
	key, err := getMasterKey(service, false)
	if err != nil {
		return "", err
	}
	return decryptData(data, key)
}

func platformSet(service, account, value string) error {
	key, err := getMasterKey(service, true)
	if err != nil {
		return err
	}
	dir := storageDir(service)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	data, err := encryptData(value, key)
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(dir, safeFileName(account)), data, 0o600)
}

func platformRemove(service, account string) error {
	err := os.Remove(filepath.Join(storageDir(service), safeFileName(account)))
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	return err
}

func storageDir(service string) string {
	base, err := os.UserConfigDir()
	if err != nil || base == "" {
		base = "."
	}
	return filepath.Join(base, "hr-cli", "keychain", safeFileNameRe.ReplaceAllString(service, "_"))
}

func safeFileName(account string) string {
	return safeFileNameRe.ReplaceAllString(account, "_") + ".enc"
}

func getMasterKey(service string, allowCreate bool) ([]byte, error) {
	dir := storageDir(service)
	path := filepath.Join(dir, "master.key")
	key, err := os.ReadFile(path)
	if err == nil && len(key) == masterKeyBytes {
		return key, nil
	}
	if err == nil {
		return nil, errors.New("keychain master key is corrupted")
	}
	if !errors.Is(err, os.ErrNotExist) {
		return nil, err
	}
	if !allowCreate {
		return nil, ErrNotFound
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, err
	}
	key = make([]byte, masterKeyBytes)
	if _, err := rand.Read(key); err != nil {
		return nil, err
	}
	if err := os.WriteFile(path, key, 0o600); err != nil {
		return nil, err
	}
	return key, nil
}

func encryptData(plaintext string, key []byte) ([]byte, error) {
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	nonce := make([]byte, nonceBytes)
	if _, err := rand.Read(nonce); err != nil {
		return nil, err
	}
	return append(nonce, gcm.Seal(nil, nonce, []byte(plaintext), nil)...), nil
}

func decryptData(data []byte, key []byte) (string, error) {
	if len(data) < nonceBytes {
		return "", fmt.Errorf("encrypted data is too short")
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return "", err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", err
	}
	plain, err := gcm.Open(nil, data[:nonceBytes], data[nonceBytes:], nil)
	if err != nil {
		return "", err
	}
	return string(plain), nil
}
