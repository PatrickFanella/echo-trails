package api

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"net"
	"net/smtp"
	"net/url"
	"strings"
	"time"
)

type SMTPConfig struct {
	Host            string
	Port            string
	Username        string
	Password        string
	From            string
	AuthLinkBaseURL string
	AllowInsecure   bool
}

type SMTPSender struct{ config SMTPConfig }

func NewSMTPSender(config SMTPConfig) (*SMTPSender, error) {
	if config.Host == "" || config.Port == "" || config.From == "" || config.AuthLinkBaseURL == "" {
		return nil, errors.New("SMTP host, port, from address, and auth link base URL are required")
	}
	if _, err := buildAuthLink(config.AuthLinkBaseURL, "request", "code"); err != nil {
		return nil, err
	}
	return &SMTPSender{config: config}, nil
}

func (s *SMTPSender) SendAuthCode(ctx context.Context, email, requestID, code string, expiresAt time.Time) error {
	address := net.JoinHostPort(s.config.Host, s.config.Port)
	dialer := net.Dialer{Timeout: 10 * time.Second}
	connection, err := dialer.DialContext(ctx, "tcp", address)
	if err != nil {
		return err
	}
	client, err := smtp.NewClient(connection, s.config.Host)
	if err != nil {
		_ = connection.Close()
		return err
	}
	defer client.Close()
	if ok, _ := client.Extension("STARTTLS"); ok {
		if err = client.StartTLS(&tls.Config{ServerName: s.config.Host, MinVersion: tls.VersionTLS12}); err != nil {
			return err
		}
	} else if !s.config.AllowInsecure {
		return errors.New("SMTP server does not advertise STARTTLS")
	}
	if s.config.Username != "" {
		if err = client.Auth(smtp.PlainAuth("", s.config.Username, s.config.Password, s.config.Host)); err != nil {
			return err
		}
	}
	if err = client.Mail(s.config.From); err != nil {
		return err
	}
	if err = client.Rcpt(email); err != nil {
		return err
	}
	writer, err := client.Data()
	if err != nil {
		return err
	}
	link, err := buildAuthLink(s.config.AuthLinkBaseURL, requestID, code)
	if err != nil {
		return err
	}
	message := fmt.Sprintf("From: %s\r\nTo: %s\r\nSubject: Your EchoTrails sign-in code\r\nMIME-Version: 1.0\r\nContent-Type: text/plain; charset=UTF-8\r\n\r\nYour EchoTrails code is %s. It expires at %s.\r\n\r\nOr open this secure sign-in link on your iPhone:\r\n%s\r\n",
		s.config.From, email, strings.TrimSpace(code), expiresAt.UTC().Format(time.RFC3339), link)
	if _, err = writer.Write([]byte(message)); err != nil {
		_ = writer.Close()
		return err
	}
	if err = writer.Close(); err != nil {
		return err
	}
	return client.Quit()
}

func buildAuthLink(baseURL, requestID, code string) (string, error) {
	parsed, err := url.Parse(strings.TrimSpace(baseURL))
	if err != nil || parsed.Scheme == "" || parsed.Host == "" {
		return "", errors.New("auth link base URL must be absolute")
	}
	if parsed.Scheme != "https" && parsed.Hostname() != "127.0.0.1" && parsed.Hostname() != "localhost" {
		return "", errors.New("auth link base URL must use HTTPS")
	}
	parsed.Path = strings.TrimRight(parsed.Path, "/") + "/auth/email/verify"
	query := parsed.Query()
	query.Set("request_id", strings.TrimSpace(requestID))
	query.Set("code", strings.TrimSpace(code))
	parsed.RawQuery = query.Encode()
	return parsed.String(), nil
}
