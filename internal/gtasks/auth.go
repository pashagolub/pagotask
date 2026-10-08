// Package gtasks signs in to Google and creates tasks.
package gtasks

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"golang.org/x/oauth2"
	"golang.org/x/oauth2/google"
	"google.golang.org/api/tasks/v1"
)

// Built-in desktop OAuth client. For a desktop ("installed") app Google
// documents these as embeddable; set at build time with
// -ldflags "-X github.com/pashagolub/pagotask/internal/gtasks.DefaultClientID=... -X ...DefaultClientSecret=..."
// or override in config.yaml (google.client_id / google.client_secret).
var (
	DefaultClientID     = ""
	DefaultClientSecret = ""
)

// Scope is the only Google permission v1 asks for.
const Scope = tasks.TasksScope

// TokenStore persists the OAuth token. Windows uses Credential Manager,
// Linux the desktop keyring; other platforms a file in the config directory.
type TokenStore interface {
	Load() (*oauth2.Token, error)
	Save(*oauth2.Token) error
	Clear() error
}

// FileTokenStore keeps the token as JSON in a 0600 file.
type FileTokenStore struct{ Path string }

func (f FileTokenStore) Load() (*oauth2.Token, error) {
	data, err := os.ReadFile(f.Path)
	if err != nil {
		return nil, err
	}
	var t oauth2.Token
	if err := json.Unmarshal(data, &t); err != nil {
		return nil, err
	}
	return &t, nil
}

func (f FileTokenStore) Save(t *oauth2.Token) error {
	data, err := json.Marshal(t)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(f.Path), 0o700); err != nil {
		return err
	}
	return os.WriteFile(f.Path, data, 0o600)
}

func (f FileTokenStore) Clear() error {
	err := os.Remove(f.Path)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	return err
}

// Auth holds the OAuth configuration and token store.
type Auth struct {
	cfg   *oauth2.Config
	store TokenStore
	// OpenBrowser opens the consent URL; defaults to the platform opener.
	OpenBrowser func(url string) error
}

// NewAuth builds the OAuth config; empty id/secret use the built-in client.
func NewAuth(clientID, clientSecret string, store TokenStore) (*Auth, error) {
	if clientID == "" {
		clientID, clientSecret = DefaultClientID, DefaultClientSecret
	}
	if clientID == "" {
		return nil, errors.New("no Google OAuth client configured: build with credentials or set google.client_id in config.yaml")
	}
	return &Auth{
		cfg: &oauth2.Config{
			ClientID:     clientID,
			ClientSecret: clientSecret,
			Endpoint:     google.Endpoint,
			Scopes:       []string{Scope},
		},
		store: store,
	}, nil
}

// SignedIn reports whether a stored token exists.
func (a *Auth) SignedIn() bool {
	t, err := a.store.Load()
	return err == nil && t != nil && t.RefreshToken != ""
}

// SignOut forgets the stored token.
func (a *Auth) SignOut() error { return a.store.Clear() }

// Client returns an HTTP client that refreshes and re-stores the token as needed.
func (a *Auth) Client(ctx context.Context) (*http.Client, error) {
	tok, err := a.store.Load()
	if err != nil {
		return nil, fmt.Errorf("not signed in: %w", err)
	}
	src := &savingSource{TokenSource: a.cfg.TokenSource(ctx, tok), store: a.store, last: tok}
	return oauth2.NewClient(ctx, src), nil
}

type savingSource struct {
	oauth2.TokenSource
	store TokenStore
	last  *oauth2.Token
}

func (s *savingSource) Token() (*oauth2.Token, error) {
	t, err := s.TokenSource.Token()
	if err != nil {
		return nil, err
	}
	if t.AccessToken != s.last.AccessToken {
		s.last = t
		_ = s.store.Save(t)
	}
	return t, nil
}

// SignIn runs the desktop OAuth flow: opens the browser, listens on a
// loopback port for the redirect, exchanges the code and stores the token.
func (a *Auth) SignIn(ctx context.Context) error {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return err
	}
	defer ln.Close()
	cfg := *a.cfg
	cfg.RedirectURL = fmt.Sprintf("http://%s/callback", ln.Addr().String())

	state, err := randomString(24)
	if err != nil {
		return err
	}
	verifier := oauth2.GenerateVerifier()
	url := cfg.AuthCodeURL(state, oauth2.AccessTypeOffline, oauth2.ApprovalForce, oauth2.S256ChallengeOption(verifier))

	codeCh := make(chan string, 1)
	errCh := make(chan error, 1)
	srv := &http.Server{Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/callback" {
			http.NotFound(w, r)
			return
		}
		q := r.URL.Query()
		if q.Get("state") != state {
			http.Error(w, "state mismatch", http.StatusBadRequest)
			errCh <- errors.New("oauth state mismatch")
			return
		}
		if e := q.Get("error"); e != "" {
			http.Error(w, e, http.StatusBadRequest)
			errCh <- fmt.Errorf("oauth: %s", e)
			return
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		fmt.Fprint(w, "<h2>pagotask is signed in.</h2><p>You can close this tab.</p>")
		codeCh <- q.Get("code")
	})}
	go func() { _ = srv.Serve(ln) }()
	defer func() {
		c, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		_ = srv.Shutdown(c)
	}()

	open := a.OpenBrowser
	if open == nil {
		open = openBrowser
	}
	if err := open(url); err != nil {
		return fmt.Errorf("open browser: %w", err)
	}

	var code string
	select {
	case code = <-codeCh:
	case err := <-errCh:
		return err
	case <-ctx.Done():
		return ctx.Err()
	}
	tok, err := cfg.Exchange(ctx, code, oauth2.VerifierOption(verifier))
	if err != nil {
		return fmt.Errorf("exchange code: %w", err)
	}
	return a.store.Save(tok)
}

func randomString(n int) (string, error) {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}
