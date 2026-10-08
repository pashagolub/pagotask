//go:build linux

package linux

import (
	"encoding/json"
	"errors"
	"log/slog"
	"sync"

	"github.com/zalando/go-keyring"
	"golang.org/x/oauth2"

	"github.com/pashagolub/pagotask/internal/gtasks"
)

const (
	keyringService = "pagotask"
	keyringUser    = "google-oauth"
)

// TokenStore keeps the OAuth token in the desktop keyring (Secret Service,
// GNOME Keyring on Ubuntu). Without a keyring it falls back to File.
type TokenStore struct {
	File gtasks.FileTokenStore
}

// warnOnce keeps a missing keyring to one log line; Load runs often.
var warnOnce sync.Once

func (s TokenStore) Load() (*oauth2.Token, error) {
	data, err := keyring.Get(keyringService, keyringUser)
	if err != nil {
		if !errors.Is(err, keyring.ErrNotFound) {
			warnOnce.Do(func() { slog.Warn("keyring unavailable, using the token file", "err", err) })
		}
		return s.File.Load()
	}
	var t oauth2.Token
	if err := json.Unmarshal([]byte(data), &t); err != nil {
		return nil, err
	}
	return &t, nil
}

func (s TokenStore) Save(t *oauth2.Token) error {
	data, err := json.Marshal(t)
	if err != nil {
		return err
	}
	if err := keyring.Set(keyringService, keyringUser, string(data)); err != nil {
		slog.Warn("keyring unavailable, saving token to file", "err", err)
		return s.File.Save(t)
	}
	return s.File.Clear() // a copy from a keyring-less run is no longer needed
}

func (s TokenStore) Clear() error {
	err := keyring.Delete(keyringService, keyringUser)
	if errors.Is(err, keyring.ErrNotFound) {
		err = nil
	}
	if ferr := s.File.Clear(); ferr != nil {
		return ferr
	}
	if err != nil {
		slog.Warn("keyring unavailable on sign out", "err", err)
	}
	return nil
}
