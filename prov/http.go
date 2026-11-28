package prov

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// Transport is the injectable HTTP layer used by every provider client.
// Tests set Base[id] to an httptest server and optionally Client.
type Transport struct {
	Client *http.Client
	Base   map[string]string
}

func (t *Transport) client() *http.Client {
	if t != nil && t.Client != nil {
		return t.Client
	}
	return &http.Client{Timeout: 30 * time.Second}
}

// BaseURL returns the official default, or a test override.
func (t *Transport) BaseURL(id, official string) string {
	if t != nil && t.Base != nil {
		if b := strings.TrimSpace(t.Base[id]); b != "" {
			return strings.TrimRight(b, "/")
		}
	}
	return strings.TrimRight(official, "/")
}

// DoJSON sends a JSON request and decodes a JSON object or array response.
func (t *Transport) DoJSON(ctx context.Context, method, rawURL string, hdr map[string]string, body any, out any) error {
	var rdr io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			return err
		}
		rdr = bytes.NewReader(b)
		if hdr == nil {
			hdr = map[string]string{}
		}
		if hdr["Content-Type"] == "" {
			hdr["Content-Type"] = "application/json"
		}
	}
	return t.Do(ctx, method, rawURL, hdr, rdr, out)
}

// DoForm sends application/x-www-form-urlencoded.
func (t *Transport) DoForm(ctx context.Context, rawURL string, hdr map[string]string, form url.Values, out any) error {
	if hdr == nil {
		hdr = map[string]string{}
	}
	hdr["Content-Type"] = "application/x-www-form-urlencoded"
	return t.Do(ctx, "POST", rawURL, hdr, strings.NewReader(form.Encode()), out)
}

// Do performs the request. A non-2xx status is returned as an APIError.
// out may be *[]byte to capture raw body, or any JSON value.
func (t *Transport) Do(ctx context.Context, method, rawURL string, hdr map[string]string, body io.Reader, out any) error {
	req, err := http.NewRequestWithContext(ctx, method, rawURL, body)
	if err != nil {
		return err
	}
	for k, v := range hdr {
		req.Header.Set(k, v)
	}
	if req.Header.Get("User-Agent") == "" {
		req.Header.Set("User-Agent", "Rover/0.9 (https://github.com/aamitojj/rover)")
	}
	resp, err := t.client().Do(req)
	if err != nil {
		return fmt.Errorf("could not reach the provider: %w", err)
	}
	defer resp.Body.Close()
	b, err := io.ReadAll(io.LimitReader(resp.Body, 8<<20))
	if err != nil {
		return err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return APIError{Status: resp.StatusCode, Body: clip(string(b), 400)}
	}
	if out == nil || len(b) == 0 {
		return nil
	}
	if p, ok := out.(*[]byte); ok {
		*p = b
		return nil
	}
	if p, ok := out.(*string); ok {
		*p = string(b)
		return nil
	}
	if err := json.Unmarshal(b, out); err != nil {
		return fmt.Errorf("the provider returned a reply Rover could not read: %w", err)
	}
	return nil
}

// APIError is a non-success HTTP response. The body is clipped and must
// already have been passed through any secret redaction by the caller if it
// might echo credentials (provider APIs generally do not).
type APIError struct {
	Status int
	Body   string
}

func (e APIError) Error() string {
	msg := strings.TrimSpace(e.Body)
	if msg == "" {
		return fmt.Sprintf("the provider returned HTTP %d", e.Status)
	}
	return fmt.Sprintf("the provider returned HTTP %d: %s", e.Status, clip(msg, 240))
}

func clip(s string, n int) string {
	s = strings.Join(strings.Fields(s), " ")
	if len(s) <= n {
		return s
	}
	return s[:n] + "…"
}

func bearer(token string) map[string]string {
	return map[string]string{"Authorization": "Bearer " + strings.TrimSpace(token)}
}

func missing(field string) error {
	return fmt.Errorf("please fill in %s", field)
}
