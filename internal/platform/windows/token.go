//go:build windows

package windows

import (
	"encoding/json"
	"errors"

	"github.com/danieljoos/wincred"
	"golang.org/x/oauth2"
)

const credTarget = "pagotask/google-oauth"

// CredStore keeps the OAuth token in Windows Credential Manager (DPAPI-protected).
type CredStore struct{}

func (CredStore) Load() (*oauth2.Token, error) {
	c, err := wincred.GetGenericCredential(credTarget)
	if err != nil {
		return nil, err
	}
	var t oauth2.Token
	if err := json.Unmarshal(c.CredentialBlob, &t); err != nil {
		return nil, err
	}
	return &t, nil
}

func (CredStore) Save(t *oauth2.Token) error {
	data, err := json.Marshal(t)
	if err != nil {
		return err
	}
	c := wincred.NewGenericCredential(credTarget)
	c.CredentialBlob = data
	c.UserName = "google"
	c.Persist = wincred.PersistLocalMachine
	return c.Write()
}

func (CredStore) Clear() error {
	c, err := wincred.GetGenericCredential(credTarget)
	if err != nil {
		if errors.Is(err, wincred.ErrElementNotFound) {
			return nil
		}
		return err
	}
	return c.Delete()
}
