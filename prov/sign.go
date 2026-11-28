package prov

import (
	"context"
	"crypto"
	"crypto/hmac"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"net/http"
	"net/url"
	"sort"
	"strings"
	"time"
)

func hmacSHA256(key []byte, s string) []byte {
	m := hmac.New(sha256.New, key)
	m.Write([]byte(s))
	return m.Sum(nil)
}

func awsEscape(s string) string {
	return strings.ReplaceAll(url.QueryEscape(s), "+", "%20")
}

// sigV4 adds X-Amz-Date and Authorization (AWS Signature Version 4).
func sigV4(hdr map[string]string, method string, u *url.URL, body []byte, akid, secretKey, region, service string, now time.Time) {
	if hdr["X-Amz-Content-Sha256"] == "" {
		sum := sha256.Sum256(body)
		hdr["X-Amz-Content-Sha256"] = hex.EncodeToString(sum[:])
	}
	amzDate := now.UTC().Format("20060102T150405Z")
	day := now.UTC().Format("20060102")
	hdr["X-Amz-Date"] = amzDate
	all := map[string]string{"host": u.Host}
	for k, v := range hdr {
		all[strings.ToLower(k)] = strings.TrimSpace(v)
	}
	var names []string
	for k := range all {
		names = append(names, k)
	}
	sort.Strings(names)
	var ch strings.Builder
	for _, k := range names {
		ch.WriteString(k + ":" + all[k] + "\n")
	}
	signed := strings.Join(names, ";")
	path := u.EscapedPath()
	if path == "" {
		path = "/"
	}
	var qs []string
	for k, vs := range u.Query() {
		for _, v := range vs {
			qs = append(qs, awsEscape(k)+"="+awsEscape(v))
		}
	}
	sort.Strings(qs)
	payloadHash := hdr["X-Amz-Content-Sha256"]
	canonical := strings.Join([]string{method, path, strings.Join(qs, "&"), ch.String(), signed, payloadHash}, "\n")
	ch2 := sha256.Sum256([]byte(canonical))
	scope := day + "/" + region + "/" + service + "/aws4_request"
	sts := "AWS4-HMAC-SHA256\n" + amzDate + "\n" + scope + "\n" + hex.EncodeToString(ch2[:])
	k := hmacSHA256([]byte("AWS4"+secretKey), day)
	k = hmacSHA256(k, region)
	k = hmacSHA256(k, service)
	k = hmacSHA256(k, "aws4_request")
	sig := hex.EncodeToString(hmacSHA256(k, sts))
	hdr["Authorization"] = "AWS4-HMAC-SHA256 Credential=" + akid + "/" + scope + ", SignedHeaders=" + signed + ", Signature=" + sig
}

func parseRSAKey(pemText string) (*rsa.PrivateKey, error) {
	blk, _ := pem.Decode([]byte(strings.TrimSpace(pemText)))
	if blk == nil {
		return nil, fmt.Errorf("the private key is not a .pem key; paste the whole file including the BEGIN and END lines")
	}
	if k, err := x509.ParsePKCS1PrivateKey(blk.Bytes); err == nil {
		return k, nil
	}
	k, err := x509.ParsePKCS8PrivateKey(blk.Bytes)
	if err != nil {
		return nil, fmt.Errorf("could not read the private key (keys with a passphrase are not supported)")
	}
	rk, ok := k.(*rsa.PrivateKey)
	if !ok {
		return nil, fmt.Errorf("this key is not an RSA private key")
	}
	return rk, nil
}

// ociSign adds Oracle Cloud Infrastructure Signature version 1 headers.
func ociSign(hdr map[string]string, method string, u *url.URL, tenancy, user, fingerprint, keyPEM string, now time.Time) error {
	key, err := parseRSAKey(keyPEM)
	if err != nil {
		return err
	}
	date := now.UTC().Format(http.TimeFormat)
	hdr["Date"] = date
	target := strings.ToLower(method) + " " + u.RequestURI()
	signing := "(request-target): " + target + "\ndate: " + date + "\nhost: " + u.Host
	h := sha256.Sum256([]byte(signing))
	sig, err := rsa.SignPKCS1v15(rand.Reader, key, crypto.SHA256, h[:])
	if err != nil {
		return err
	}
	keyID := strings.TrimSpace(tenancy) + "/" + strings.TrimSpace(user) + "/" + strings.TrimSpace(fingerprint)
	hdr["Authorization"] = fmt.Sprintf(`Signature version="1",keyId="%s",algorithm="rsa-sha256",headers="(request-target) date host",signature="%s"`, keyID, base64.StdEncoding.EncodeToString(sig))
	return nil
}

// googleJWT builds a signed JWT and exchanges it at the token endpoint
// (https://oauth2.googleapis.com/token by default) for an access token.
func googleJWT(ctx context.Context, tr *Transport, creds map[string]string, scope string) (string, error) {
	email := strings.TrimSpace(creds["client_email"])
	key := creds["private_key"]
	if email == "" || key == "" {
		// Allow a full service-account JSON blob in "json".
		if raw := strings.TrimSpace(creds["json"]); raw != "" {
			var sa struct {
				ClientEmail string `json:"client_email"`
				PrivateKey  string `json:"private_key"`
				TokenURI    string `json:"token_uri"`
			}
			if err := json.Unmarshal([]byte(raw), &sa); err != nil {
				return "", fmt.Errorf("the service account file is not valid JSON")
			}
			email, key = sa.ClientEmail, sa.PrivateKey
			if sa.TokenURI != "" && creds["token_uri"] == "" {
				creds["token_uri"] = sa.TokenURI
			}
		}
	}
	if email == "" || key == "" {
		return "", missing("service account JSON (or client_email and private_key)")
	}
	rk, err := parseRSAKey(key)
	if err != nil {
		return "", err
	}
	now := time.Now().UTC().Unix()
	claim, _ := json.Marshal(map[string]any{
		"iss":   email,
		"scope": scope,
		"aud":   "https://oauth2.googleapis.com/token",
		"iat":   now,
		"exp":   now + 3600,
	})
	head := base64.RawURLEncoding.EncodeToString([]byte(`{"alg":"RS256","typ":"JWT"}`))
	body := base64.RawURLEncoding.EncodeToString(claim)
	msg := head + "." + body
	h := sha256.Sum256([]byte(msg))
	sig, err := rsa.SignPKCS1v15(rand.Reader, rk, crypto.SHA256, h[:])
	if err != nil {
		return "", err
	}
	jwt := msg + "." + base64.RawURLEncoding.EncodeToString(sig)
	tokenURL := tr.BaseURL("google-token", "https://oauth2.googleapis.com/token")
	if u := strings.TrimSpace(creds["token_uri"]); u != "" && (tr == nil || tr.Base["google-token"] == "") {
		tokenURL = u
	}
	var tok struct {
		AccessToken string `json:"access_token"`
		Error       string `json:"error"`
		ErrorDesc   string `json:"error_description"`
	}
	form := url.Values{"grant_type": {"urn:ietf:params:oauth:grant-type:jwt-bearer"}, "assertion": {jwt}}
	if err := tr.DoForm(ctx, tokenURL, nil, form, &tok); err != nil {
		return "", err
	}
	if tok.AccessToken == "" {
		return "", fmt.Errorf("Google did not return an access token: %s %s", tok.Error, tok.ErrorDesc)
	}
	return tok.AccessToken, nil
}

func googleBearer(ctx context.Context, tr *Transport, creds map[string]string, scope string) (map[string]string, error) {
	if t := strings.TrimSpace(creds["access_token"]); t != "" {
		return bearer(t), nil
	}
	t, err := googleJWT(ctx, tr, creds, scope)
	if err != nil {
		return nil, err
	}
	return bearer(t), nil
}
