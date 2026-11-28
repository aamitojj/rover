package main

// Google Sign-In for a personal Rover account: OAuth 2 loopback + PKCE.
// The user must create an OAuth client (Desktop or "iOS"/public client)
// in Google Cloud Console and paste the client ID in Settings. There is
// no client secret. Extra scopes are requested only when connecting
// Analytics, Search Console or AdSense.

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"
)

type GoogleAccount struct {
	Email        string `json:"email,omitempty"`
	Name         string `json:"name,omitempty"`
	Sub          string `json:"sub,omitempty"`
	AccessToken  string `json:"access_token,omitempty"`  // protected
	RefreshToken string `json:"refresh_token,omitempty"` // protected
	Expiry       string `json:"expiry,omitempty"`
	Scopes       string `json:"scopes,omitempty"`
}

type googleFlow struct {
	ID, State, Verifier, Redirect, Scope string
	ln                                   net.Listener
	done                                 chan error
	account                              *GoogleAccount
}

var (
	googleOAuthBase = "https://accounts.google.com/o/oauth2/v2/auth"
	googleTokenBase = "https://oauth2.googleapis.com/token"
	googleUserInfo  = "https://openidconnect.googleapis.com/v1/userinfo"
	gflowMu         sync.Mutex
	gflows          = map[string]*googleFlow{}
)

func registerGoogleAuthRoutes(mux *http.ServeMux) {
	mux.HandleFunc("POST /api/account/google/start", apiGoogleStart)
	mux.HandleFunc("GET /api/account/google", apiGoogleStatus)
	mux.HandleFunc("DELETE /api/account/google", apiGoogleSignOut)
	mux.HandleFunc("POST /api/account/google/client", apiGoogleClientID)
}

func apiGoogleClientID(w http.ResponseWriter, r *http.Request) {
	var req struct {
		ClientID string `json:"client_id"`
	}
	if err := readJSON(r, &req); err != nil {
		writeErr(w, err)
		return
	}
	id := strings.TrimSpace(req.ClientID)
	if id == "" || !strings.Contains(id, ".apps.googleusercontent.com") {
		writeErr(w, errors.New("paste the OAuth client ID from Google Cloud Console (it ends in .apps.googleusercontent.com)"))
		return
	}
	if err := withConfig(func(c *Config) error { c.GoogleClientID = id; return nil }); err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, 200, map[string]any{"ok": true})
}

func apiGoogleStatus(w http.ResponseWriter, r *http.Request) {
	c := snapshotConfig()
	out := map[string]any{"client_id_set": c.GoogleClientID != "", "signed_in": false}
	if c.GoogleAccount != nil {
		out["signed_in"] = true
		out["email"] = c.GoogleAccount.Email
		out["name"] = c.GoogleAccount.Name
		out["scopes"] = c.GoogleAccount.Scopes
	}
	writeJSON(w, 200, out)
}

func apiGoogleSignOut(w http.ResponseWriter, r *http.Request) {
	_ = withConfig(func(c *Config) error { c.GoogleAccount = nil; return nil })
	writeJSON(w, 200, map[string]any{"ok": true})
}

func apiGoogleStart(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Scopes string `json:"scopes"`
	}
	_ = readJSON(r, &req)
	c := snapshotConfig()
	if c.GoogleClientID == "" {
		writeErr(w, errors.New("add your Google OAuth client ID first (Settings → Sign in). Create a Desktop app client in Google Cloud Console and add the loopback redirect http://127.0.0.1:<port>/callback"))
		return
	}
	scope := strings.TrimSpace(req.Scopes)
	if scope == "" {
		scope = "openid email profile"
	}
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		writeErr(w, err)
		return
	}
	ver := pkceVerifier()
	sum := sha256.Sum256([]byte(ver))
	ch := base64.RawURLEncoding.EncodeToString(sum[:])
	f := &googleFlow{ID: randHex(8), State: randHex(16), Verifier: ver, Redirect: "http://" + ln.Addr().String() + "/callback", Scope: scope, ln: ln, done: make(chan error, 1)}
	gflowMu.Lock()
	gflows[f.ID] = f
	gflowMu.Unlock()
	mux := http.NewServeMux()
	mux.HandleFunc("/callback", func(rw http.ResponseWriter, req *http.Request) {
		q := req.URL.Query()
		if q.Get("state") != f.State {
			http.Error(rw, "state mismatch", 400)
			f.done <- errors.New("sign-in state mismatch")
			return
		}
		if q.Get("error") != "" {
			msg := q.Get("error")
			io.WriteString(rw, "Sign-in was cancelled. You can close this tab.")
			f.done <- fmt.Errorf("Google sign-in: %s", msg)
			return
		}
		tok, err := googleTokenRequest(req.Context(), url.Values{
			"grant_type":    {"authorization_code"},
			"code":          {q.Get("code")},
			"client_id":     {c.GoogleClientID},
			"redirect_uri":  {f.Redirect},
			"code_verifier": {f.Verifier},
		})
		if err != nil {
			http.Error(rw, "token exchange failed", 500)
			f.done <- err
			return
		}
		acct, err := googleUser(req.Context(), tok.AccessToken)
		if err != nil {
			http.Error(rw, "could not read the Google profile", 500)
			f.done <- err
			return
		}
		acct.AccessToken, _ = protectString(tok.AccessToken)
		if tok.RefreshToken != "" {
			acct.RefreshToken, _ = protectString(tok.RefreshToken)
		}
		acct.Expiry = time.Now().Add(time.Duration(tok.ExpiresIn) * time.Second).UTC().Format(time.RFC3339)
		acct.Scopes = f.Scope
		_ = withConfig(func(cfg *Config) error { cfg.GoogleAccount = acct; return nil })
		io.WriteString(rw, "<!doctype html><title>Rover</title><p>Signed in as "+htmlEscape(acct.Email)+". You can close this tab and return to Rover.</p>")
		f.account = acct
		f.done <- nil
	})
	srv := &http.Server{Handler: mux}
	go srv.Serve(ln)
	go func() {
		select {
		case <-f.done:
		case <-time.After(10 * time.Minute):
		}
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		_ = srv.Shutdown(ctx)
		cancel()
		_ = ln.Close()
		gflowMu.Lock()
		delete(gflows, f.ID)
		gflowMu.Unlock()
	}()
	v := url.Values{
		"response_type":         {"code"},
		"client_id":             {c.GoogleClientID},
		"redirect_uri":          {f.Redirect},
		"scope":                 {scope},
		"state":                 {f.State},
		"code_challenge":        {ch},
		"code_challenge_method": {"S256"},
		"access_type":           {"offline"},
		"prompt":                {"consent"},
	}
	writeJSON(w, 200, map[string]any{"flow": f.ID, "url": googleOAuthBase + "?" + v.Encode(), "redirect": f.Redirect})
}

type googleTok struct {
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
	ExpiresIn    int    `json:"expires_in"`
	Error        string `json:"error"`
	ErrorDesc    string `json:"error_description"`
}

func googleTokenRequest(ctx context.Context, form url.Values) (*googleTok, error) {
	req, _ := http.NewRequestWithContext(ctx, "POST", googleTokenBase, strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	var tok googleTok
	if err := json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(&tok); err != nil {
		return nil, err
	}
	if tok.AccessToken == "" {
		return nil, fmt.Errorf("Google token error: %s %s", tok.Error, tok.ErrorDesc)
	}
	return &tok, nil
}

func googleUser(ctx context.Context, access string) (*GoogleAccount, error) {
	req, _ := http.NewRequestWithContext(ctx, "GET", googleUserInfo, nil)
	req.Header.Set("Authorization", "Bearer "+access)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	var u struct {
		Sub, Email, Name string
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(&u); err != nil {
		return nil, err
	}
	if u.Email == "" && u.Sub == "" {
		return nil, errors.New("Google did not return a profile")
	}
	return &GoogleAccount{Email: u.Email, Name: u.Name, Sub: u.Sub}, nil
}

func pkceVerifier() string {
	b := make([]byte, 32)
	_, _ = rand.Read(b)
	return base64.RawURLEncoding.EncodeToString(b)
}
