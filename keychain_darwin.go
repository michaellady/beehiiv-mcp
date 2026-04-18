//go:build darwin

package main

import (
	"errors"

	"github.com/keybase/go-keychain"
)

// macOSKeychain is a credStore backed by the macOS Keychain Services.
// Access is bound to the binary's signing identity, so the user sees a single
// "Allow access" prompt on first use (one per account).
type macOSKeychain struct{}

func newMacOSKeychain() credStore { return macOSKeychain{} }

func (macOSKeychain) Get(service, account string) ([]byte, error) {
	q := keychain.NewItem()
	q.SetSecClass(keychain.SecClassGenericPassword)
	q.SetService(service)
	q.SetAccount(account)
	q.SetMatchLimit(keychain.MatchLimitOne)
	q.SetReturnData(true)

	results, err := keychain.QueryItem(q)
	if err != nil {
		if errors.Is(err, keychain.ErrorItemNotFound) {
			return nil, errNotFound
		}
		return nil, err
	}
	if len(results) == 0 {
		return nil, errNotFound
	}
	return results[0].Data, nil
}

func (macOSKeychain) Set(service, account string, data []byte) error {
	item := keychain.NewItem()
	item.SetSecClass(keychain.SecClassGenericPassword)
	item.SetService(service)
	item.SetAccount(account)
	item.SetLabel("beehiiv-mcp " + account)
	item.SetData(data)
	item.SetSynchronizable(keychain.SynchronizableNo)
	item.SetAccessible(keychain.AccessibleWhenUnlocked)

	err := keychain.AddItem(item)
	if err == nil {
		return nil
	}
	if !errors.Is(err, keychain.ErrorDuplicateItem) {
		return err
	}
	// Item exists — update in place.
	q := keychain.NewItem()
	q.SetSecClass(keychain.SecClassGenericPassword)
	q.SetService(service)
	q.SetAccount(account)

	upd := keychain.NewItem()
	upd.SetData(data)
	return keychain.UpdateItem(q, upd)
}

func (macOSKeychain) Delete(service, account string) error {
	q := keychain.NewItem()
	q.SetSecClass(keychain.SecClassGenericPassword)
	q.SetService(service)
	q.SetAccount(account)
	err := keychain.DeleteItem(q)
	if err == nil || errors.Is(err, keychain.ErrorItemNotFound) {
		return nil
	}
	return err
}
