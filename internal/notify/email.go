package notify

import (
	"bytes"
	"crypto/tls"
	"fmt"
	"net"
	"net/smtp"
	"strings"
	"time"

	"cu-watcher/internal/parse"
)

type EmailConfig struct {
	Enabled  bool     `yaml:"enabled"`
	From     string   `yaml:"from"`
	To       []string `yaml:"to"`
	SMTPHost string   `yaml:"smtpHost"`
	SMTPPort int      `yaml:"smtpPort"`
	Username string   `yaml:"username"`
	Password string   `yaml:"password"`
	UseTLS   bool     `yaml:"useTLS"`
}

// SendNewReleases emails a summary of newly detected SQL Server fixes/CUs.
// It is a no-op when cfg.Enabled is false or rows is empty.
func SendNewReleases(cfg EmailConfig, rows []parse.BuildRow) error {
	if !cfg.Enabled || len(rows) == 0 || len(cfg.To) == 0 {
		return nil
	}

	subject := fmt.Sprintf("CU Watcher: %d new SQL Server update(s) detected", len(rows))
	msg := buildMessage(cfg.From, cfg.To, subject, buildBody(rows))
	addr := fmt.Sprintf("%s:%d", cfg.SMTPHost, cfg.SMTPPort)

	if cfg.UseTLS {
		return sendTLS(addr, cfg, msg)
	}
	return sendSTARTTLS(addr, cfg, msg)
}

func buildBody(rows []parse.BuildRow) string {
	var sb strings.Builder
	sb.WriteString("The following new SQL Server fixes/CUs have been detected:\n\n")
	for _, r := range rows {
		date := "N/A"
		if r.ReleaseDate != nil {
			date = r.ReleaseDate.Format("2006-01-02")
		}
		fmt.Fprintf(&sb, "  • [SQL %d] %s\n", r.MajorVersion, r.UpdateName)
		fmt.Fprintf(&sb, "    Build:    %s\n", r.SqlBuild)
		fmt.Fprintf(&sb, "    KB:       %s\n", r.KbNumber)
		fmt.Fprintf(&sb, "    Released: %s\n", date)
		if r.KbURL != "" {
			fmt.Fprintf(&sb, "    URL:      %s\n", r.KbURL)
		}
		sb.WriteString("\n")
	}
	return sb.String()
}

func buildMessage(from string, to []string, subject, body string) []byte {
	var buf bytes.Buffer
	fmt.Fprintf(&buf, "From: %s\r\n", from)
	fmt.Fprintf(&buf, "To: %s\r\n", strings.Join(to, ", "))
	fmt.Fprintf(&buf, "Subject: %s\r\n", subject)
	fmt.Fprintf(&buf, "Date: %s\r\n", time.Now().Format("Mon, 02 Jan 2006 15:04:05 -0700"))
	fmt.Fprintf(&buf, "MIME-Version: 1.0\r\n")
	fmt.Fprintf(&buf, "Content-Type: text/plain; charset=UTF-8\r\n")
	fmt.Fprintf(&buf, "\r\n")
	fmt.Fprintf(&buf, "%s", body)
	return buf.Bytes()
}

// sendSTARTTLS connects in plain text then upgrades via STARTTLS (port 587).
func sendSTARTTLS(addr string, cfg EmailConfig, msg []byte) error {
	host, _, _ := net.SplitHostPort(addr)
	var auth smtp.Auth
	if cfg.Username != "" {
		auth = smtp.PlainAuth("", cfg.Username, cfg.Password, host)
	}
	return smtp.SendMail(addr, auth, cfg.From, cfg.To, msg)
}

// sendTLS connects directly over TLS (port 465).
func sendTLS(addr string, cfg EmailConfig, msg []byte) error {
	host, _, _ := net.SplitHostPort(addr)

	conn, err := tls.Dial("tcp", addr, &tls.Config{ServerName: host})
	if err != nil {
		return err
	}

	c, err := smtp.NewClient(conn, host)
	if err != nil {
		return err
	}
	defer func() { _ = c.Quit() }()

	if cfg.Username != "" {
		if err := c.Auth(smtp.PlainAuth("", cfg.Username, cfg.Password, host)); err != nil {
			return err
		}
	}

	if err := c.Mail(cfg.From); err != nil {
		return err
	}
	for _, to := range cfg.To {
		if err := c.Rcpt(to); err != nil {
			return err
		}
	}

	w, err := c.Data()
	if err != nil {
		return err
	}
	if _, err := w.Write(msg); err != nil {
		return err
	}
	return w.Close()
}
