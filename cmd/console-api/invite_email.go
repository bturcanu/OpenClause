package main

import (
	"bytes"
	"context"
	"crypto/tls"
	"fmt"
	"log/slog"
	"net"
	"net/mail"
	"net/smtp"
	"os"
	"strconv"
	"strings"
	"time"
)

const (
	InviteEmailStatusLogged = "logged"
	InviteEmailStatusSent   = "sent"
	InviteEmailStatusFailed = "failed"
)

type InviteEmailMessage struct {
	To         string
	Name       string
	TenantName string
	Role       string
	AcceptURL  string
	ExpiresAt  time.Time
}

type InviteEmailSendResult struct {
	Status       string
	ErrorMessage string
}

type InviteEmailSender interface {
	SendInvite(ctx context.Context, msg InviteEmailMessage) (InviteEmailSendResult, error)
}

type loggedInviteEmailSender struct {
	log *slog.Logger
}

func (s *loggedInviteEmailSender) SendInvite(_ context.Context, msg InviteEmailMessage) (InviteEmailSendResult, error) {
	s.log.Info(
		"invite email logged",
		"email", msg.To,
		"tenant_name", msg.TenantName,
		"role", msg.Role,
		"accept_url", msg.AcceptURL,
		"expires_at", msg.ExpiresAt,
	)
	return InviteEmailSendResult{Status: InviteEmailStatusLogged}, nil
}

type misconfiguredInviteEmailSender struct {
	log           *slog.Logger
	publicMessage string
}

func (s *misconfiguredInviteEmailSender) SendInvite(_ context.Context, msg InviteEmailMessage) (InviteEmailSendResult, error) {
	err := fmt.Errorf("invite email sender misconfigured")
	s.log.Error("invite email sender misconfigured", "email", msg.To, "error", s.publicMessage)
	return InviteEmailSendResult{
		Status:       InviteEmailStatusFailed,
		ErrorMessage: s.publicMessage,
	}, err
}

type smtpInviteEmailSender struct {
	host string
	port int
	user string
	pass string
	from string
}

func (s *smtpInviteEmailSender) SendInvite(ctx context.Context, msg InviteEmailMessage) (InviteEmailSendResult, error) {
	addr := net.JoinHostPort(s.host, strconv.Itoa(s.port))
	dialer := &net.Dialer{Timeout: 5 * time.Second}
	conn, err := dialer.DialContext(ctx, "tcp", addr)
	if err != nil {
		return InviteEmailSendResult{
			Status:       InviteEmailStatusFailed,
			ErrorMessage: "SMTP connection failed",
		}, fmt.Errorf("dial smtp: %w", err)
	}
	defer conn.Close() //nolint:errcheck

	if deadline, ok := ctx.Deadline(); ok {
		_ = conn.SetDeadline(deadline)
	}

	client, err := smtp.NewClient(conn, s.host)
	if err != nil {
		return InviteEmailSendResult{
			Status:       InviteEmailStatusFailed,
			ErrorMessage: "SMTP handshake failed",
		}, fmt.Errorf("new smtp client: %w", err)
	}
	defer client.Close() //nolint:errcheck

	if ok, _ := client.Extension("STARTTLS"); ok {
		if err := client.StartTLS(&tls.Config{MinVersion: tls.VersionTLS12, ServerName: s.host}); err != nil {
			return InviteEmailSendResult{
				Status:       InviteEmailStatusFailed,
				ErrorMessage: "SMTP TLS negotiation failed",
			}, fmt.Errorf("starttls smtp: %w", err)
		}
	}

	if s.user != "" {
		if ok, _ := client.Extension("AUTH"); !ok {
			return InviteEmailSendResult{
				Status:       InviteEmailStatusFailed,
				ErrorMessage: "SMTP authentication is unavailable",
			}, fmt.Errorf("smtp auth unavailable")
		}
		if err := client.Auth(smtp.PlainAuth("", s.user, s.pass, s.host)); err != nil {
			return InviteEmailSendResult{
				Status:       InviteEmailStatusFailed,
				ErrorMessage: "SMTP authentication failed",
			}, fmt.Errorf("auth smtp: %w", err)
		}
	}

	if err := client.Mail(s.from); err != nil {
		return InviteEmailSendResult{
			Status:       InviteEmailStatusFailed,
			ErrorMessage: "SMTP sender was rejected",
		}, fmt.Errorf("mail from smtp: %w", err)
	}
	if err := client.Rcpt(msg.To); err != nil {
		return InviteEmailSendResult{
			Status:       InviteEmailStatusFailed,
			ErrorMessage: "SMTP recipient was rejected",
		}, fmt.Errorf("rcpt to smtp: %w", err)
	}

	wc, err := client.Data()
	if err != nil {
		return InviteEmailSendResult{
			Status:       InviteEmailStatusFailed,
			ErrorMessage: "SMTP message upload failed",
		}, fmt.Errorf("smtp data: %w", err)
	}

	body := buildInviteEmailMessage(s.from, msg)
	if _, err := wc.Write(body); err != nil {
		_ = wc.Close()
		return InviteEmailSendResult{
			Status:       InviteEmailStatusFailed,
			ErrorMessage: "SMTP message upload failed",
		}, fmt.Errorf("write smtp message: %w", err)
	}
	if err := wc.Close(); err != nil {
		return InviteEmailSendResult{
			Status:       InviteEmailStatusFailed,
			ErrorMessage: "SMTP message upload failed",
		}, fmt.Errorf("close smtp data: %w", err)
	}
	if err := client.Quit(); err != nil {
		return InviteEmailSendResult{
			Status:       InviteEmailStatusFailed,
			ErrorMessage: "SMTP delivery failed",
		}, fmt.Errorf("quit smtp: %w", err)
	}

	return InviteEmailSendResult{Status: InviteEmailStatusSent}, nil
}

func buildInviteEmailMessage(from string, msg InviteEmailMessage) []byte {
	displayName := strings.TrimSpace(msg.Name)
	if displayName == "" {
		displayName = "there"
	}
	tenantName := strings.TrimSpace(msg.TenantName)
	if tenantName == "" {
		tenantName = "your OpenClause workspace"
	}

	subject := fmt.Sprintf("You're invited to %s on OpenClause", tenantName)
	body := fmt.Sprintf(
		"Hi %s,\r\n\r\nYou've been invited to join %s as %s.\r\n\r\nAccept your invite:\r\n%s\r\n\r\nThis invite expires at %s.\r\n",
		displayName,
		tenantName,
		msg.Role,
		msg.AcceptURL,
		msg.ExpiresAt.UTC().Format(time.RFC1123Z),
	)

	var raw bytes.Buffer
	raw.WriteString(fmt.Sprintf("From: %s\r\n", from))
	raw.WriteString(fmt.Sprintf("To: %s\r\n", msg.To))
	raw.WriteString(fmt.Sprintf("Subject: %s\r\n", subject))
	raw.WriteString("MIME-Version: 1.0\r\n")
	raw.WriteString("Content-Type: text/plain; charset=UTF-8\r\n")
	raw.WriteString("\r\n")
	raw.WriteString(body)
	return raw.Bytes()
}

func newInviteEmailSenderFromEnv(log *slog.Logger) InviteEmailSender {
	host := strings.TrimSpace(os.Getenv("SMTP_HOST"))
	portText := strings.TrimSpace(os.Getenv("SMTP_PORT"))
	user := strings.TrimSpace(os.Getenv("SMTP_USER"))
	pass := os.Getenv("SMTP_PASS")
	from := strings.TrimSpace(os.Getenv("SMTP_FROM"))

	if host == "" && portText == "" && user == "" && pass == "" && from == "" {
		return &loggedInviteEmailSender{log: log}
	}
	if host == "" || from == "" {
		return &misconfiguredInviteEmailSender{
			log:           log,
			publicMessage: "SMTP invite delivery is misconfigured",
		}
	}
	if _, err := mail.ParseAddress(from); err != nil {
		return &misconfiguredInviteEmailSender{
			log:           log,
			publicMessage: "SMTP invite sender address is invalid",
		}
	}

	port := 587
	if portText != "" {
		parsed, err := strconv.Atoi(portText)
		if err != nil || parsed <= 0 {
			return &misconfiguredInviteEmailSender{
				log:           log,
				publicMessage: "SMTP invite delivery port is invalid",
			}
		}
		port = parsed
	}
	if (user == "") != (pass == "") {
		return &misconfiguredInviteEmailSender{
			log:           log,
			publicMessage: "SMTP invite delivery credentials are incomplete",
		}
	}

	return &smtpInviteEmailSender{
		host: host,
		port: port,
		user: user,
		pass: pass,
		from: from,
	}
}
