package keychain

import (
	"errors"
	"fmt"
)

const ServiceName = "hr-cli"

var ErrNotFound = errors.New("keychain: item not found")

func Get(service, account string) (string, error) {
	value, err := platformGet(service, account)
	if errors.Is(err, ErrNotFound) {
		return "", nil
	}
	if err != nil {
		return "", fmt.Errorf("keychain get failed: %w", err)
	}
	return value, nil
}

func Set(service, account, value string) error {
	if err := platformSet(service, account, value); err != nil {
		return fmt.Errorf("keychain set failed: %w", err)
	}
	return nil
}

func Remove(service, account string) error {
	if err := platformRemove(service, account); err != nil && !errors.Is(err, ErrNotFound) {
		return fmt.Errorf("keychain remove failed: %w", err)
	}
	return nil
}
