package main

import (
	"bufio"
	"crypto/tls"
	"errors"
	"fmt"
	"net"
	"net/smtp"
	"strconv"
	"strings"
	"sync"
	"time"
)

func smtpConfigured(a *AlertSettings) bool {
	if a == nil {
		return false
	}
	if strings.TrimSpace(a.SMTPHost) == "" {
		return false
	}
	if strings.TrimSpace(a.Email) == "" && strings.TrimSpace(a.SMTPFrom) == "" {
		return false
	}
	return true
}

func sanitizeSMTPHost(host string) (string, error) {
	host = strings.TrimSpace(host)
	if host == "" {
		return "", errors.New("enter an SMTP host")
	}
	if strings.ContainsAny(host, " \t\r\n:/@") {
		return "", errors.New("the SMTP host must be a hostname or IP, with the port in its own field")
	}
	if net.ParseIP(host) == nil && strings.Contains(host, "..") {
		return "", errors.New("the SMTP host is not valid")
	}
	return host, nil
}

func smtpPort(a *AlertSettings) int {
	if a != nil && a.SMTPPort > 0 {
		return a.SMTPPort
	}
	return 587
}

func smtpTLSMode(a *AlertSettings) string {
	m := ""
	if a != nil {
		m = strings.ToLower(strings.TrimSpace(a.SMTPTLS))
	}
	if m == "" {
		return "starttls"
	}
	return m
}

func localSMTPHost(host string) bool {
	h := strings.ToLower(host)
	return h == "127.0.0.1" || h == "localhost" || h == "::1"
}

func splitAddresses(s string) []string {
	var out []string
	for _, p := range strings.FieldsFunc(s, func(r rune) bool { return r == ',' || r == ';' || r == ' ' }) {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}
		if strings.ContainsAny(p, "\r\n") || !strings.Contains(p, "@") {
			continue
		}
		out = append(out, p)
	}
	return out
}

func headerSafe(s string) string {
	return strings.Map(func(r rune) rune {
		if r == '\r' || r == '\n' {
			return -1
		}
		return r
	}, s)
}

func buildAlertMessage(from string, to []string, level, site, text string) []byte {
	subj := "Rover alert"
	if level != "" {
		subj += " (" + headerSafe(level) + ")"
	}
	if site != "" {
		subj += " — " + headerSafe(site)
	}
	var b strings.Builder
	b.WriteString("From: " + headerSafe(from) + "\r\n")
	b.WriteString("To: " + headerSafe(strings.Join(to, ", ")) + "\r\n")
	b.WriteString("Subject: " + subj + "\r\n")
	b.WriteString("MIME-Version: 1.0\r\n")
	b.WriteString("Content-Type: text/plain; charset=utf-8\r\n")
	b.WriteString("\r\n")
	b.WriteString(text)
	if !strings.HasSuffix(text, "\n") {
		b.WriteString("\r\n")
	}
	return []byte(b.String())
}

// sendSMTPAlert delivers a plain-text alert through the user's SMTP server.
// Credentials are taken from the DPAPI-protected SMTPPass field. TLS mode
// "none" is allowed only for a local host (tests and a mailer on this PC).
func sendSMTPAlert(a *AlertSettings, level, site, text string) error {
	if a == nil {
		return errors.New("SMTP is not configured")
	}
	host, err := sanitizeSMTPHost(a.SMTPHost)
	if err != nil {
		return err
	}
	port := smtpPort(a)
	if port < 1 || port > 65535 {
		return errors.New("the SMTP port is not valid")
	}
	from := strings.TrimSpace(a.SMTPFrom)
	if from == "" {
		from = strings.TrimSpace(a.SMTPUser)
	}
	to := splitAddresses(a.Email)
	if from == "" || strings.ContainsAny(from, "\r\n") || !strings.Contains(from, "@") {
		return errors.New("set a From address for SMTP alerts")
	}
	if len(to) == 0 {
		return errors.New("set at least one To address for SMTP alerts")
	}
	mode := smtpTLSMode(a)
	if mode != "starttls" && mode != "tls" && mode != "none" {
		return errors.New("SMTP TLS must be starttls, tls, or none (local only)")
	}
	if mode == "none" && !localSMTPHost(host) {
		return errors.New("unencrypted SMTP is only allowed to 127.0.0.1 or localhost")
	}
	user := strings.TrimSpace(a.SMTPUser)
	pass := secret(a.SMTPPass)
	msg := buildAlertMessage(from, to, level, site, text)
	addr := net.JoinHostPort(host, strconv.Itoa(port))
	return smtpDeliver(addr, host, mode, user, pass, from, to, msg)
}

// smtpDeliver is the low-level send, replaced in tests when needed.
var smtpDeliver = smtpDeliverLive

func smtpDeliverLive(addr, host, mode, user, pass, from string, to []string, msg []byte) error {
	dialer := &net.Dialer{Timeout: 15 * time.Second}
	var (
		conn net.Conn
		err  error
	)
	tlsConf := &tls.Config{ServerName: host, MinVersion: tls.VersionTLS12}
	switch mode {
	case "tls":
		conn, err = tls.DialWithDialer(dialer, "tcp", addr, tlsConf)
	default:
		conn, err = dialer.Dial("tcp", addr)
	}
	if err != nil {
		return fmt.Errorf("could not reach the SMTP server: %w", err)
	}
	c, err := smtp.NewClient(conn, host)
	if err != nil {
		conn.Close()
		return err
	}
	defer c.Close()
	if mode == "starttls" {
		if ok, _ := c.Extension("STARTTLS"); ok {
			if err := c.StartTLS(tlsConf); err != nil {
				return err
			}
		} else if !localSMTPHost(host) {
			return errors.New("the SMTP server does not advertise STARTTLS")
		}
	}
	if user != "" {
		if err := c.Auth(smtp.PlainAuth("", user, pass, host)); err != nil {
			return fmt.Errorf("SMTP login failed: %w", err)
		}
	}
	if err := c.Mail(from); err != nil {
		return err
	}
	for _, rcpt := range to {
		if err := c.Rcpt(rcpt); err != nil {
			return err
		}
	}
	w, err := c.Data()
	if err != nil {
		return err
	}
	if _, err := w.Write(msg); err != nil {
		_ = w.Close()
		return err
	}
	if err := w.Close(); err != nil {
		return err
	}
	return c.Quit()
}

func redactAlertSettings(a AlertSettings) map[string]any {
	set := a.SMTPPass != ""
	a.SMTPPass = ""
	return map[string]any{
		"in_app": a.InApp, "windows": a.Windows,
		"email": a.Email, "webhook": a.Webhook,
		"smtp_host": a.SMTPHost, "smtp_port": smtpPort(&a), "smtp_user": a.SMTPUser,
		"smtp_from": a.SMTPFrom, "smtp_tls": smtpTLSMode(&a),
		"smtp_pass_set": set,
		"cpu":           a.CPU, "ram": a.RAM, "disk": a.Disk, "ssl_days": a.SSLDays, "cooldown_min": a.Cooldown,
	}
}

func mergeAlertSecrets(prev *AlertSettings, req *AlertSettings) error {
	if req.SMTPHost != "" {
		h, err := sanitizeSMTPHost(req.SMTPHost)
		if err != nil {
			return err
		}
		req.SMTPHost = h
	}
	if req.SMTPPort < 0 || req.SMTPPort > 65535 {
		return errors.New("the SMTP port is not valid")
	}
	mode := smtpTLSMode(req)
	if mode != "starttls" && mode != "tls" && mode != "none" {
		return errors.New("SMTP TLS must be starttls, tls, or none (local only)")
	}
	if mode == "none" && req.SMTPHost != "" && !localSMTPHost(req.SMTPHost) {
		return errors.New("unencrypted SMTP is only allowed to 127.0.0.1 or localhost")
	}
	if strings.ContainsAny(req.SMTPFrom, "\r\n") || strings.ContainsAny(req.Email, "\r\n") {
		return errors.New("email addresses cannot contain line breaks")
	}
	pass := strings.TrimSpace(req.SMTPPass)
	if pass == "" && prev != nil {
		req.SMTPPass = prev.SMTPPass
		return nil
	}
	if pass == "" {
		req.SMTPPass = ""
		return nil
	}
	enc, err := protectString(pass)
	if err != nil {
		return err
	}
	req.SMTPPass = enc
	return nil
}

// startFakeSMTP is a tiny unencrypted SMTP listener for tests. It records
// DATA payloads and accepts optional PLAIN auth.
func startFakeSMTP() (addr string, got *smtpCapture, closeFn func()) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		panic(err)
	}
	cap := &smtpCapture{}
	done := make(chan struct{})
	go func() {
		defer close(done)
		for {
			c, err := ln.Accept()
			if err != nil {
				return
			}
			go handleFakeSMTP(c, cap)
		}
	}()
	return ln.Addr().String(), cap, func() { ln.Close(); <-done }
}

type smtpCapture struct {
	mu               sync.Mutex
	From, Data, User string
	To               []string
	Auth             bool
}

func (s *smtpCapture) snapshot() (from, data string, to []string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.From, s.Data, append([]string(nil), s.To...)
}

func handleFakeSMTP(c net.Conn, cap *smtpCapture) {
	defer c.Close()
	_ = c.SetDeadline(time.Now().Add(5 * time.Second))
	r := bufio.NewReader(c)
	fmt.Fprintf(c, "220 rover.test ESMTP\r\n")
	for {
		line, err := r.ReadString('\n')
		if err != nil {
			return
		}
		cmd := strings.ToUpper(strings.TrimSpace(line))
		switch {
		case strings.HasPrefix(cmd, "EHLO") || strings.HasPrefix(cmd, "HELO"):
			fmt.Fprintf(c, "250-rover.test\r\n250-AUTH PLAIN LOGIN\r\n250 OK\r\n")
		case strings.HasPrefix(cmd, "AUTH PLAIN"):
			parts := strings.Fields(strings.TrimSpace(line))
			cap.mu.Lock()
			cap.Auth = true
			if len(parts) > 2 {
				cap.User = parts[2]
			}
			cap.mu.Unlock()
			fmt.Fprintf(c, "235 2.7.0 OK\r\n")
		case strings.HasPrefix(cmd, "MAIL FROM:"):
			rest := strings.TrimSpace(line)
			if len(rest) >= 10 {
				rest = rest[10:]
			}
			cap.mu.Lock()
			cap.From = strings.Trim(strings.TrimSpace(rest), "<>")
			cap.mu.Unlock()
			fmt.Fprintf(c, "250 OK\r\n")
		case strings.HasPrefix(cmd, "RCPT TO:"):
			rest := strings.TrimSpace(line)
			if len(rest) >= 8 {
				rest = rest[8:]
			}
			cap.mu.Lock()
			cap.To = append(cap.To, strings.Trim(strings.TrimSpace(rest), "<>"))
			cap.mu.Unlock()
			fmt.Fprintf(c, "250 OK\r\n")
		case strings.HasPrefix(cmd, "DATA"):
			fmt.Fprintf(c, "354 End data with <CR><LF>.<CR><LF>\r\n")
			var body strings.Builder
			for {
				l, err := r.ReadString('\n')
				if err != nil {
					return
				}
				if l == ".\r\n" || l == ".\n" {
					break
				}
				body.WriteString(l)
			}
			cap.mu.Lock()
			cap.Data = body.String()
			cap.mu.Unlock()
			fmt.Fprintf(c, "250 OK\r\n")
		case strings.HasPrefix(cmd, "QUIT"):
			fmt.Fprintf(c, "221 bye\r\n")
			return
		default:
			fmt.Fprintf(c, "250 OK\r\n")
		}
	}
}
