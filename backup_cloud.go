package main

// Optional cloud destinations for scheduled backups (S3-compatible or
// Supabase Storage). Local ZIP backups stay the default. Secrets never
// go into the archive (backupNow already skips them).

import (
	"bytes"
	"context"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"path"
	"strings"
	"time"
)

func uploadBackupCloud(ctx context.Context, c *Config, localPath string) error {
	b := backupSettings(c)
	dest := strings.ToLower(b.Destination)
	if dest == "" || dest == "local" {
		return nil
	}
	raw, err := os.ReadFile(localPath)
	if err != nil {
		return err
	}
	name := path.Base(localPath)
	switch dest {
	case "s3":
		a := findIntegration(c, b.IntegrationID)
		if a == nil {
			return fmt.Errorf("connect an AWS account in the Integration Centre first")
		}
		cr := integCred(a)
		if b.Bucket == "" {
			return fmt.Errorf("set a bucket name for cloud backups")
		}
		key := strings.Trim(b.Prefix, "/")
		if key != "" {
			key += "/"
		}
		key += name
		region := firstNonEmpty(cr["region"], "us-east-1")
		endpoint := "https://" + b.Bucket + ".s3." + region + ".amazonaws.com/" + url.PathEscape(strings.TrimPrefix(key, "/"))
		// For path-style / custom S3, a future field can override. Official virtual-hosted is the default.
		u, err := url.Parse(endpoint)
		if err != nil {
			return err
		}
		hdr := map[string]string{"Content-Type": "application/zip"}
		// Reuse AWS SigV4 through a tiny internal helper in this file.
		if err := s3Put(ctx, cr["access_key"], cr["secret_key"], region, u, raw, hdr); err != nil {
			return err
		}
		return nil
	case "supabase":
		a := findIntegration(c, b.IntegrationID)
		if a == nil {
			return fmt.Errorf("connect a Supabase account first")
		}
		cr := integCred(a)
		ref, role, bucket := cr["project_ref"], cr["service_role"], firstNonEmpty(b.Bucket, "backups")
		if ref == "" || role == "" {
			return fmt.Errorf("the Supabase account needs a project ref and service role key")
		}
		u := "https://" + ref + ".supabase.co/storage/v1/object/" + url.PathEscape(bucket) + "/" + url.PathEscape(name)
		req, _ := http.NewRequestWithContext(ctx, "POST", u, bytes.NewReader(raw))
		req.Header.Set("apikey", role)
		req.Header.Set("Authorization", "Bearer "+role)
		req.Header.Set("Content-Type", "application/zip")
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			return err
		}
		defer resp.Body.Close()
		if resp.StatusCode >= 300 {
			return fmt.Errorf("Supabase storage returned HTTP %d", resp.StatusCode)
		}
		return nil
	default:
		return fmt.Errorf("unknown backup destination %q", dest)
	}
}

func s3Put(ctx context.Context, ak, sk, region string, u *url.URL, body []byte, hdr map[string]string) error {
	// Use the same SigV4 implementation as the Lightsail client in this module.
	if hdr == nil {
		hdr = map[string]string{}
	}
	sigV4(hdr, "PUT", u, body, ak, sk, region, "s3", time.Now())
	req, err := http.NewRequestWithContext(ctx, "PUT", u.String(), bytes.NewReader(body))
	if err != nil {
		return err
	}
	for k, v := range hdr {
		req.Header.Set(k, v)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		return fmt.Errorf("S3 returned HTTP %d", resp.StatusCode)
	}
	return nil
}
