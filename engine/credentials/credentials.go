// Package credentials stores secrets in the operating system's credential
// store. The engine depends on the Store interface rather than on go-keyring
// directly, so the backing library can be replaced without changes elsewhere
// (see ADR 0003).
package credentials

import (
	"errors"
	"fmt"

	"github.com/zalando/go-keyring"
)

// ErrNotFound is returned when no secret is stored under the requested name.
var ErrNotFound = errors.New("credential not found")

// Store saves, loads and deletes named secrets.
type Store interface {
	// Get returns the secret stored under name, or ErrNotFound.
	Get(name string) (string, error)
	// Set stores secret under name, replacing any existing value.
	Set(name string, secret string) error
	// Delete removes the secret stored under name. Deleting a name that has
	// no secret is not an error.
	Delete(name string) error
}

// KeyringStore is a Store backed by the operating system's credential store:
// Windows Credential Manager, the macOS Keychain or the Linux Secret Service.
type KeyringStore struct {
	serviceName string
}

// NewKeyringStore returns a KeyringStore that groups its secrets under
// serviceName, which is how they appear in the operating system's credential
// manager.
func NewKeyringStore(serviceName string) *KeyringStore {
	return &KeyringStore{serviceName: serviceName}
}

// Get returns the secret stored under name, or ErrNotFound.
func (store *KeyringStore) Get(name string) (string, error) {
	secret, err := keyring.Get(store.serviceName, name)
	if errors.Is(err, keyring.ErrNotFound) {
		return "", ErrNotFound
	}
	if err != nil {
		return "", fmt.Errorf("reading credential %q: %w", name, err)
	}
	return secret, nil
}

// Set stores secret under name, replacing any existing value.
func (store *KeyringStore) Set(name string, secret string) error {
	err := keyring.Set(store.serviceName, name, secret)
	if err != nil {
		return fmt.Errorf("saving credential %q: %w", name, err)
	}
	return nil
}

// Delete removes the secret stored under name. Deleting a name that has no
// secret is not an error.
func (store *KeyringStore) Delete(name string) error {
	err := keyring.Delete(store.serviceName, name)
	if errors.Is(err, keyring.ErrNotFound) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("deleting credential %q: %w", name, err)
	}
	return nil
}
