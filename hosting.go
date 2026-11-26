package main

// Hosting accounts portal: connect several provider accounts (several per
// provider too), see their servers, use one for Rover, create a server
// (cost shown first, confirm required) and remove accounts.

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"
)

// HostingAccount is one connected provider account. Cred holds the
// credential fields as JSON, protected with protectString (DPAPI on
// Windows); it is never sent to the UI or to any AI.
type HostingAccount struct {
	ID       string         `json:"id"`
	Provider string         `json:"provider"`
	Nickname string         `json:"nickname"`
	Auth     string         `json:"auth"` // oauth | token | keys
	Cred     string         `json:"cred"`
	Hint     string         `json:"hint,omitempty"`
	Who      string         `json:"who,omitempty"`
	Added    string         `json:"added"`
	Checked  string         `json:"checked,omitempty"`
	Status   string         `json:"status,omitempty"` // ok | error
	Error    string         `json:"error,omitempty"`
	Servers  []HostedServer `json:"servers,omitempty"`
}

func findAccount(c *Config, id string) *HostingAccount {
	for _, a := range c.HostingAccounts {
		if a.ID == id {
			return a
		}
	}
	return nil
}

func protectCred(m map[string]string) (string, error) {
	b, _ := json.Marshal(m)
	return protectString(string(b))
}

func accountCred(a *HostingAccount) map[string]string {
	m := map[string]string{}
	if a == nil || a.Cred == "" {
		return m
	}
	_ = json.Unmarshal([]byte(secret(a.Cred)), &m)
	return m
}

func credHint(prov string, m map[string]string) string {
	for _, k := range []string{"token", "access_key", "client_id", "fingerprint"} {
		if v := m[k]; v != "" {
			return keyHint(v)
		}
	}
	return ""
}

// validateCred trims the fields and checks the required ones are there.
func validateCred(p HostingProvider, in map[string]string) (map[string]string, error) {
	out := map[string]string{}
	for _, f := range p.Fields {
		v := in[f.Key]
		if !f.Multiline {
			v = strings.TrimSpace(v)
		}
		if v == "" && !f.Optional {
			return nil, fmt.Errorf("please fill in %s", strings.ToLower(f.Label))
		}
		if len(v) > 8000 {
			return nil, fmt.Errorf("%s is too long", f.Label)
		}
		out[f.Key] = v
	}
	if p.ID == "oracle" {
		if _, err := parseRSAKey(out["key"]); err != nil {
			return nil, err
		}
	}
	return out, nil
}

// refreshDO renews an expired DigitalOcean OAuth token.
func freshCred(ctx context.Context, a *HostingAccount) (map[string]string, error) {
	cr := accountCred(a)
	if a.Provider != "digitalocean" || cr["refresh"] == "" || cr["expires"] == "" {
		return cr, nil
	}
	exp, _ := time.Parse(time.RFC3339, cr["expires"])
	if time.Until(exp) > 10*time.Minute {
		return cr, nil
	}
	c := snapshotConfig()
	tok, err := doTokenRequest(ctx, url.Values{"grant_type": {"refresh_token"}, "refresh_token": {cr["refresh"]}, "client_id": {c.DOClientID}})
	if err != nil {
		return nil, fmt.Errorf("DigitalOcean sign-in expired; click Sign in again (%v)", err)
	}
	cr["token"], cr["refresh"], cr["expires"] = tok.AccessToken, firstNonEmpty(tok.RefreshToken, cr["refresh"]), time.Now().Add(time.Duration(tok.ExpiresIn)*time.Second).UTC().Format(time.RFC3339)
	enc, err := protectCred(cr)
	if err == nil {
		_ = withConfig(func(c *Config) error {
			if x := findAccount(c, a.ID); x != nil {
				x.Cred = enc
			}
			return nil
		})
	}
	return cr, nil
}

func refreshAccount(ctx context.Context, id string) (*HostingAccount, error) {
	c := snapshotConfig()
	a := findAccount(&c, id)
	if a == nil {
		return nil, errors.New("unknown account")
	}
	cr, err := freshCred(ctx, a)
	var servers []HostedServer
	who := ""
	if err == nil {
		servers, who, err = listHostedServers(ctx, a.Provider, cr)
	}
	var out HostingAccount
	_ = withConfig(func(c *Config) error {
		x := findAccount(c, id)
		if x == nil {
			return nil
		}
		x.Checked = time.Now().UTC().Format(time.RFC3339)
		if err != nil {
			x.Status, x.Error = "error", err.Error()
		} else {
			x.Status, x.Error, x.Servers = "ok", "", servers
			if who != "" {
				x.Who = who
			}
		}
		out = *x
		return nil
	})
	return &out, err
}

type accountView struct {
	HostingAccount
	Cred      string            `json:"cred,omitempty"` // always empty
	Name      string            `json:"provider_name"`
	Color     string            `json:"color"`
	Short     string            `json:"short"`
	CanCreate bool              `json:"can_create"`
	OrderURL  string            `json:"order_url,omitempty"`
	Used      map[string]string `json:"used,omitempty"` // provider server id -> Rover server id
}

func accountViews(c *Config) []accountView {
	var out []accountView
	for _, a := range c.HostingAccounts {
		p, _ := hostingProviderByID(a.Provider)
		v := accountView{HostingAccount: *a, Name: p.Name, Color: p.Color, Short: p.Short, CanCreate: p.CanCreate, OrderURL: p.OrderURL, Used: map[string]string{}}
		v.HostingAccount.Cred = ""
		for _, sp := range c.Servers {
			if sp.AccountID == a.ID {
				for _, hs := range a.Servers {
					if hs.ID == sp.ProviderServerID || (hs.IP != "" && hs.IP == sp.ServerIP) {
						v.Used[hs.ID] = sp.ID
					}
				}
			}
		}
		out = append(out, v)
	}
	return out
}

// useHostedServer adds (or re-uses) a Rover server profile for a provider
// server and makes it active. connected=true means Rover's own key
// already works; otherwise the user signs in once with the password.
func useHostedServer(acct, sid string) (profileID string, err error) {
	c := snapshotConfig()
	a := findAccount(&c, acct)
	if a == nil {
		return "", errors.New("unknown account")
	}
	var hs *HostedServer
	for i := range a.Servers {
		if a.Servers[i].ID == sid {
			hs = &a.Servers[i]
		}
	}
	if hs == nil {
		return "", errors.New("that server is not in the list any more; click Refresh")
	}
	if net.ParseIP(hs.IP) == nil {
		return "", errors.New("this server has no public IPv4 address yet")
	}
	p, _ := hostingProviderByID(a.Provider)
	user := firstNonEmpty(hs.User, p.User, "root")
	err = withConfig(func(c *Config) error {
		for _, sp := range c.Servers {
			if sp.ServerIP == hs.IP {
				profileID = sp.ID
				sp.AccountID, sp.ProviderServerID, sp.Provider = acct, hs.ID, a.Provider
				return nil
			}
		}
		np := &ServerProfile{ID: newServerID(), Label: firstNonEmpty(hs.Name, hs.IP) + " (" + p.Name + ")", AccountID: acct, ProviderServerID: hs.ID, Provider: a.Provider,
			ServerIP: hs.IP, SSHPort: 22, SSHUser: user, CoolifyURL: "http://" + hs.IP + ":8000", Added: time.Now().UTC().Format(time.RFC3339)}
		c.Servers = append(c.Servers, np)
		profileID = np.ID
		return nil
	})
	if err != nil {
		return "", err
	}
	return profileID, activateServer(profileID)
}

// createForAccount: create server -> add profile -> connect with Rover's
// key -> install the hosting engine (same zero-touch setup as 0.3).
func createForAccount(j *Job, acct, plan, region string, o SetupOptions) error {
	ctx, cancel := context.WithTimeout(context.Background(), 70*time.Minute)
	defer cancel()
	c := snapshotConfig()
	a := findAccount(&c, acct)
	if a == nil {
		return errors.New("unknown account")
	}
	p, _ := hostingProviderByID(a.Provider)
	cr, err := freshCred(ctx, a)
	if err != nil {
		return err
	}
	_, pub, err := ensureManagedKey()
	if err != nil {
		return err
	}
	j.SetSteps("Creating the server at "+p.Name, "Waiting for it to start", "Connecting with Rover's key", "Installing the hosting engine")
	j.Step(0)
	name := "rover-" + randHex(3)
	id, ip, user, err := createHostedServer(ctx, j, a.Provider, cr, name, plan, region, pub)
	if err != nil {
		return err
	}
	j.Step(1)
	j.Logf("Server %s is running at %s.", name, ip)
	var pid string
	_ = withConfig(func(c *Config) error {
		np := &ServerProfile{ID: newServerID(), Label: name + " (" + p.Name + ")", AccountID: acct, ProviderServerID: id, Provider: a.Provider, ServerIP: ip, SSHPort: 22, SSHUser: user, CoolifyURL: "http://" + ip + ":8000", Added: time.Now().UTC().Format(time.RFC3339)}
		c.Servers = append(c.Servers, np)
		pid = np.ID
		return nil
	})
	if err := activateServer(pid); err != nil {
		return err
	}
	_, _ = refreshAccount(ctx, acct)
	j.Step(2)
	for i := 0; i < 40; i++ {
		if err = tryConnectManaged(); err == nil {
			break
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(hostPollEvery):
		}
	}
	if err != nil {
		return fmt.Errorf("the server was created but SSH is not reachable yet (%v). Open Settings > Server and click Connect in a minute", err)
	}
	j.Step(3)
	return setupServerJob(j, o)
}

// ---------- DigitalOcean OAuth (authorization code + PKCE) ----------

var (
	doOAuthBase = "https://cloud.digitalocean.com/v1/oauth"
	oauthMu     sync.Mutex
	oauthFlows  = map[string]*oauthFlow{}
)

type oauthFlow struct {
	ID, State, Verifier, Redirect, Nickname string
	Done                                    bool
	Err, Account                            string
	ln                                      net.Listener
}

type doToken struct {
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
	ExpiresIn    int64  `json:"expires_in"`
	Info         struct {
		Name, Email, TeamName string `json:"-"`
	} `json:"-"`
	RawInfo map[string]any `json:"info"`
}

func doTokenRequest(ctx context.Context, form url.Values) (*doToken, error) {
	var t doToken
	if err := hostDo(ctx, "DigitalOcean", "POST", doOAuthBase+"/token", map[string]string{"Content-Type": "application/x-www-form-urlencoded"}, []byte(form.Encode()), &t); err != nil {
		return nil, err
	}
	if t.AccessToken == "" {
		return nil, errors.New("DigitalOcean did not return a token")
	}
	return &t, nil
}

// doClientID registers Rover as a public OAuth client once (dynamic
// client registration for native apps; loopback redirects match any port).
func doClientID(ctx context.Context) (string, error) {
	if id := snapshotConfig().DOClientID; id != "" {
		return id, nil
	}
	var r struct {
		ClientID string `json:"client_id"`
	}
	body := map[string]any{"client_name": "Rover", "client_uri": "https://github.com/aamitojj/rover", "redirect_uris": []string{"http://127.0.0.1:53682/callback"},
		"token_endpoint_auth_method": "none", "grant_types": []string{"authorization_code", "refresh_token"}, "response_types": []string{"code"}}
	if err := hostDo(ctx, "DigitalOcean", "POST", doOAuthBase+"/register", nil, jsonBody(body), &r); err != nil {
		return "", err
	}
	if r.ClientID == "" {
		return "", errors.New("DigitalOcean did not register the sign-in")
	}
	_ = withConfig(func(c *Config) error { c.DOClientID = r.ClientID; return nil })
	return r.ClientID, nil
}

func pkce() (verifier, challenge string) {
	verifier = base64.RawURLEncoding.EncodeToString([]byte(randHex(32)))
	h := sha256.Sum256([]byte(verifier))
	return verifier, base64.RawURLEncoding.EncodeToString(h[:])
}

func startDOOAuth(ctx context.Context, nickname string) (*oauthFlow, string, error) {
	cid, err := doClientID(ctx)
	if err != nil {
		return nil, "", err
	}
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return nil, "", err
	}
	f := &oauthFlow{ID: randHex(8), State: randHex(16), Nickname: nickname, ln: ln}
	var ch string
	f.Verifier, ch = pkce()
	f.Redirect = fmt.Sprintf("http://127.0.0.1:%d/callback", ln.Addr().(*net.TCPAddr).Port)
	oauthMu.Lock()
	oauthFlows[f.ID] = f
	oauthMu.Unlock()
	srv := &http.Server{ReadHeaderTimeout: 10 * time.Second}
	srv.Handler = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/callback" {
			http.NotFound(w, r)
			return
		}
		q := r.URL.Query()
		finish := func(msg string, e error) {
			oauthMu.Lock()
			f.Done = true
			if e != nil {
				f.Err = e.Error()
			}
			oauthMu.Unlock()
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			fmt.Fprintf(w, `<!doctype html><meta charset="utf-8"><title>Rover</title><body style="font:16px system-ui;padding:40px"><h2>%s</h2><p>You can close this tab and go back to Rover.</p></body>`, msg)
			go func() { time.Sleep(time.Second); _ = srv.Close() }()
		}
		if q.Get("state") != f.State {
			finish("Sign-in could not be checked.", errors.New("the sign-in reply did not match; try again"))
			return
		}
		if e := q.Get("error"); e != "" {
			finish("Sign-in was cancelled.", errors.New("DigitalOcean: "+firstNonEmpty(q.Get("error_description"), e)))
			return
		}
		tok, err := doTokenRequest(r.Context(), url.Values{"grant_type": {"authorization_code"}, "code": {q.Get("code")}, "client_id": {cid}, "redirect_uri": {f.Redirect}, "code_verifier": {f.Verifier}})
		if err != nil {
			finish("Sign-in failed.", err)
			return
		}
		cr := map[string]string{"token": tok.AccessToken, "refresh": tok.RefreshToken}
		if tok.ExpiresIn > 0 {
			cr["expires"] = time.Now().Add(time.Duration(tok.ExpiresIn) * time.Second).UTC().Format(time.RFC3339)
		}
		who := ""
		if tok.RawInfo != nil {
			who, _ = tok.RawInfo["email"].(string)
		}
		id, err := addAccount("digitalocean", f.Nickname, "oauth", cr, who)
		if err == nil {
			oauthMu.Lock()
			f.Account = id
			oauthMu.Unlock()
			go func() { _, _ = refreshAccount(context.Background(), id) }()
		}
		finish("Rover is connected to DigitalOcean.", err)
	})
	go func() { _ = srv.Serve(ln) }()
	go func() { time.Sleep(10 * time.Minute); _ = srv.Close() }()
	v := url.Values{"response_type": {"code"}, "client_id": {cid}, "redirect_uri": {f.Redirect}, "scope": {"read write"}, "state": {f.State}, "code_challenge": {ch}, "code_challenge_method": {"S256"}}
	return f, doOAuthBase + "/authorize?" + v.Encode(), nil
}

func addAccount(prov, nickname, auth string, cr map[string]string, who string) (string, error) {
	enc, err := protectCred(cr)
	if err != nil {
		return "", err
	}
	p, _ := hostingProviderByID(prov)
	id := "acc-" + randHex(4)
	err = withConfig(func(c *Config) error {
		n := 1
		for _, a := range c.HostingAccounts {
			if a.Provider == prov {
				n++
			}
		}
		if nickname == "" {
			nickname = p.Name
			if n > 1 {
				nickname = fmt.Sprintf("%s %d", p.Name, n)
			}
		}
		c.HostingAccounts = append(c.HostingAccounts, &HostingAccount{ID: id, Provider: prov, Nickname: nickname, Auth: auth, Cred: enc, Hint: credHint(prov, cr), Who: who, Added: time.Now().UTC().Format(time.RFC3339), Status: "ok"})
		return nil
	})
	return id, err
}

// ---------- HTTP ----------

func registerHostingRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/hosting/providers", func(w http.ResponseWriter, r *http.Request) {
		_, pub, _ := ensureManagedKeyIfAny()
		writeJSON(w, 200, map[string]any{"providers": hostingProviders, "public_key": pub})
	})
	mux.HandleFunc("GET /api/hosting/accounts", func(w http.ResponseWriter, r *http.Request) {
		c := snapshotConfig()
		writeJSON(w, 200, map[string]any{"accounts": accountViews(&c), "servers": serverViews(&c), "active": c.ActiveServer})
	})
	// Add an account: verify the details by listing its servers, then save.
	mux.HandleFunc("POST /api/hosting/accounts", func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			Provider string            `json:"provider"`
			Nickname string            `json:"nickname"`
			Fields   map[string]string `json:"fields"`
		}
		if err := readJSON(r, &req); err != nil {
			writeErr(w, err)
			return
		}
		p, ok := hostingProviderByID(req.Provider)
		if !ok || p.Auth == "ssh" {
			writeErr(w, errors.New("choose a provider from the list"))
			return
		}
		cr, err := validateCred(p, req.Fields)
		if err != nil {
			writeErr(w, err)
			return
		}
		servers, who, err := listHostedServers(r.Context(), p.ID, cr)
		if err != nil {
			writeErr(w, fmt.Errorf("could not connect: %v", err))
			return
		}
		auth := "token"
		if p.Auth == "keys" {
			auth = "keys"
		}
		id, err := addAccount(p.ID, strings.TrimSpace(req.Nickname), auth, cr, who)
		if err != nil {
			writeErr(w, err)
			return
		}
		_ = withConfig(func(c *Config) error {
			if a := findAccount(c, id); a != nil {
				a.Servers, a.Checked = servers, time.Now().UTC().Format(time.RFC3339)
			}
			return nil
		})
		writeJSON(w, 200, map[string]any{"ok": true, "id": id, "servers": len(servers), "who": who})
	})
	mux.HandleFunc("POST /api/hosting/accounts/{id}/refresh", func(w http.ResponseWriter, r *http.Request) {
		a, err := refreshAccount(r.Context(), r.PathValue("id"))
		if err != nil {
			writeErr(w, err)
			return
		}
		writeJSON(w, 200, map[string]any{"ok": true, "servers": a.Servers})
	})
	mux.HandleFunc("POST /api/hosting/accounts/{id}/rename", func(w http.ResponseWriter, r *http.Request) {
		var req struct{ Nickname string }
		_ = readJSON(r, &req)
		n := strings.TrimSpace(req.Nickname)
		if n == "" || len(n) > 60 {
			writeErr(w, errors.New("enter a nickname (up to 60 letters)"))
			return
		}
		err := withConfig(func(c *Config) error {
			a := findAccount(c, r.PathValue("id"))
			if a == nil {
				return errors.New("unknown account")
			}
			a.Nickname = n
			return nil
		})
		if err != nil {
			writeErr(w, err)
			return
		}
		writeJSON(w, 200, map[string]any{"ok": true})
	})
	// Remove the account from Rover. Servers keep running; projects keep
	// working. Only the saved token/keys are deleted from this PC.
	mux.HandleFunc("DELETE /api/hosting/accounts/{id}", func(w http.ResponseWriter, r *http.Request) {
		id := r.PathValue("id")
		_ = withConfig(func(c *Config) error {
			keep := c.HostingAccounts[:0]
			for _, a := range c.HostingAccounts {
				if a.ID != id {
					keep = append(keep, a)
				}
			}
			c.HostingAccounts = keep
			for _, sp := range c.Servers {
				if sp.AccountID == id {
					sp.AccountID = ""
				}
			}
			return nil
		})
		writeJSON(w, 200, map[string]any{"ok": true})
	})
	mux.HandleFunc("GET /api/hosting/accounts/{id}/plans", func(w http.ResponseWriter, r *http.Request) {
		c := snapshotConfig()
		a := findAccount(&c, r.PathValue("id"))
		if a == nil {
			writeErr(w, errors.New("unknown account"))
			return
		}
		cr, err := freshCred(r.Context(), a)
		if err != nil {
			writeErr(w, err)
			return
		}
		plans, err := hostingPlans(r.Context(), a.Provider, cr)
		if err != nil {
			writeErr(w, err)
			return
		}
		writeJSON(w, 200, map[string]any{"plans": plans})
	})
	// Create a server. Costs money, so the request must carry confirmed=true
	// and the monthly price the user saw (the UI asks first).
	mux.HandleFunc("POST /api/hosting/accounts/{id}/create", func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			Plan, Region string
			Confirmed    bool    `json:"confirmed"`
			Monthly      float64 `json:"monthly"`
			AdminEmail   string  `json:"admin_email"`
			AutoUpdates  bool    `json:"auto_updates"`
		}
		if err := readJSON(r, &req); err != nil {
			writeErr(w, err)
			return
		}
		if !req.Confirmed {
			writeJSON(w, http.StatusPreconditionRequired, map[string]any{"error": "creating a server costs money; confirm the price first", "need_confirm": true})
			return
		}
		c := snapshotConfig()
		a := findAccount(&c, r.PathValue("id"))
		if a == nil || req.Plan == "" {
			writeErr(w, errors.New("choose a size"))
			return
		}
		if p, _ := hostingProviderByID(a.Provider); !p.CanCreate {
			writeErr(w, errors.New("order this server on the provider's website, then click Refresh"))
			return
		}
		o := SetupOptions{AdminEmail: req.AdminEmail, AutoUpdates: req.AutoUpdates}
		id := a.ID
		j := newJob("hosting-create", func(j *Job) error { return createForAccount(j, id, req.Plan, req.Region, o) })
		writeJSON(w, 200, map[string]any{"job": j.ID})
	})
	mux.HandleFunc("POST /api/hosting/accounts/{id}/servers/{sid}/use", func(w http.ResponseWriter, r *http.Request) {
		pid, err := useHostedServer(r.PathValue("id"), r.PathValue("sid"))
		if err != nil {
			writeErr(w, err)
			return
		}
		connected := tryConnectManaged() == nil
		c := snapshotConfig()
		writeJSON(w, 200, map[string]any{"ok": true, "server": pid, "connected": connected, "engine": c.CoolifyToken != "", "ip": c.ServerIP, "user": c.SSHUser})
	})
	mux.HandleFunc("POST /api/hosting/oauth/digitalocean", func(w http.ResponseWriter, r *http.Request) {
		var req struct{ Nickname string }
		_ = readJSON(r, &req)
		f, u, err := startDOOAuth(r.Context(), strings.TrimSpace(req.Nickname))
		if err != nil {
			writeErr(w, fmt.Errorf("DigitalOcean sign-in is not available right now (%v). Use a personal access token instead", err))
			return
		}
		openBrowser(u) // the user's own browser: DigitalOcean's real sign-in page
		writeJSON(w, 200, map[string]any{"flow": f.ID, "url": u})
	})
	mux.HandleFunc("GET /api/hosting/oauth/{flow}", func(w http.ResponseWriter, r *http.Request) {
		oauthMu.Lock()
		f := oauthFlows[r.PathValue("flow")]
		var out map[string]any
		if f != nil {
			out = map[string]any{"done": f.Done, "error": f.Err, "account": f.Account}
		}
		oauthMu.Unlock()
		if out == nil {
			writeErr(w, errors.New("unknown sign-in"))
			return
		}
		writeJSON(w, 200, out)
	})
}

// ensureManagedKeyIfAny returns Rover's public key, creating it if needed,
// so users can paste it into a provider console (Oracle) when ordering.
func ensureManagedKeyIfAny() (any, string, error) {
	_, pub, err := ensureManagedKey()
	return nil, pub, err
}
