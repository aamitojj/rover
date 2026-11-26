//go:build realnginx

package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

// Validates lite configs with a real nginx: go test -tags realnginx -run RealNginx .
func TestRealNginxLite(t *testing.T) {
	dir := t.TempDir()
	os.Chmod(dir, 0o755)
	var inc string
	for _, slug := range []string{"shop", "my-blog"} {
		src := t.TempDir()
		os.WriteFile(filepath.Join(src, "_redirects"), []byte("/old /new 301\n/app/* /index.html 200\n/blog/:slug /posts/:slug 302\n"), 0o644)
		os.WriteFile(filepath.Join(src, "_headers"), []byte("/*\n  X-Frame-Options: DENY\n/assets/*\n  Cache-Control: public, max-age=31536000\n"), 0o644)
		rules := loadSiteRules("", src)
		fs := &siteFiles{}
		conf := generateNginxConf(rules, fs)
		s := &Site{Slug: slug, Domain: slug + ".example", IncludeWWW: true, Primary: "apex"}
		conf = siteConfExtras(conf, s)
		base := filepath.Join(dir, slug)
		os.MkdirAll(filepath.Join(base, "html"), 0o755)
		os.MkdirAll(filepath.Join(base, "nginx"), 0o755)
		lc := liteConf(conf, slug, base, liteHosts(s, &ServerProfile{ServerIP: "192.168.1.9"}))
		p := filepath.Join(base, "nginx", "default.conf")
		os.WriteFile(p, []byte(lc), 0o644)
		inc += "include " + p + ";\n"
	}
	main := "pid " + dir + "/nginx.pid;\nerror_log " + dir + "/error.log;\nevents {}\nhttp {\naccess_log off;\n" + inc + "}\n"
	mp := filepath.Join(dir, "nginx.conf")
	os.WriteFile(mp, []byte(main), 0o644)
	out, err := exec.Command("sudo", "/usr/sbin/nginx", "-t", "-c", mp).CombinedOutput()
	if err != nil {
		t.Fatalf("%v\n%s", err, out)
	}
	t.Logf("%s", out)
}
