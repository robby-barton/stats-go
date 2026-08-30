package main

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"net"
	"net/smtp"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"go.uber.org/zap"
)

// alertThreshold is the number of consecutive scheduled-job failures that
// trigger an email alert. With the 5-minute games poll, threshold 3 means
// an operator hears about a broken poll within ~15 minutes.
const alertThreshold = 3

// alerter emails the operator after alertThreshold consecutive
// scheduled-job failures. It is disabled unless both ALERT_EMAIL_TO and
// SMTP_HOST are set. The counter resets after alerting, so a persistent
// outage re-alerts on every threshold crossing instead of staying silent,
// and any success resets it cleanly.
type alerter struct {
	log *zap.SugaredLogger

	to, from string
	host     string
	port     int
	user     string
	pass     string

	// send delivers an alert message. A field so tests can stub delivery.
	send func(msg []byte) error

	mu       sync.Mutex
	failures int
}

// newAlerter builds the alerter from environment variables:
//
//	ALERT_EMAIL_TO  recipient address (enables alerting when set)
//	SMTP_HOST       relay hostname (required for delivery)
//	SMTP_PORT       587 (STARTTLS, default) or 465 (implicit TLS)
//	SMTP_USER       relay username
//	SMTP_PASS       relay password
//	SMTP_FROM       sender address (defaults to ALERT_EMAIL_TO)
func newAlerter(log *zap.SugaredLogger) *alerter {
	a := &alerter{
		log:  log,
		to:   os.Getenv("ALERT_EMAIL_TO"),
		host: os.Getenv("SMTP_HOST"),
		user: os.Getenv("SMTP_USER"),
		pass: os.Getenv("SMTP_PASS"),
	}
	if p := os.Getenv("SMTP_PORT"); p != "" {
		if n, err := strconv.Atoi(p); err == nil {
			a.port = n
		}
	}
	if a.port == 0 {
		a.port = 587
	}
	a.from = os.Getenv("SMTP_FROM")
	if a.from == "" {
		a.from = a.to
	}
	a.send = a.smtpSend
	return a
}

// failure records a failed job run and alerts on threshold crossings.
func (a *alerter) failure(job string, jobErr error) {
	a.mu.Lock()
	a.failures++
	n := a.failures
	if n >= alertThreshold {
		a.failures = 0
	}
	a.mu.Unlock()
	if a == nil || a.to == "" || a.host == "" || n < alertThreshold {
		return
	}

	from := a.from
	if from == "" {
		from = a.to
	}
	subject := fmt.Sprintf("stats-go: job %q failed %d consecutive times", job, alertThreshold)
	body := fmt.Sprintf(
		"stats-go updater: job %q has failed %d consecutive times.\n\nLast error:\n%v\n\n"+
			"The counter has reset; you will hear again after %d more consecutive failures,\n"+
			"or immediately after recovery if the job starts failing again.\n",
		job, alertThreshold, jobErr, alertThreshold,
	)
	a.log.Errorf("ALERT: %s: %v", subject, jobErr)

	msg, err := buildMessage(from, a.to, subject, body)
	if err != nil {
		a.log.Errorf("alert: building message: %v", err)
		return
	}
	if err := a.send(msg); err != nil {
		a.log.Errorf("alert: delivering email to %s: %v", a.to, err)
	}
}

// success resets the consecutive-failure counter after a good run.
func (a *alerter) success() {
	if a == nil {
		return
	}
	a.mu.Lock()
	a.failures = 0
	a.mu.Unlock()
}

// buildMessage renders an RFC 5322 plain-text email.
func buildMessage(from, to, subject, body string) ([]byte, error) {
	if from == "" || to == "" {
		return nil, errors.New("empty sender or recipient")
	}
	var b strings.Builder
	fmt.Fprintf(&b, "From: %s\r\n", from)
	fmt.Fprintf(&b, "To: %s\r\n", to)
	fmt.Fprintf(&b, "Subject: %s\r\n", subject)
	fmt.Fprintf(&b, "Date: %s\r\n", time.Now().Format(time.RFC1123Z))
	fmt.Fprintf(&b, "MIME-Version: 1.0\r\n")
	fmt.Fprintf(&b, "Content-Type: text/plain; charset=UTF-8\r\n")
	fmt.Fprintf(&b, "\r\n%s", body)
	return []byte(b.String()), nil
}

// smtpSend delivers msg through the configured relay. Port 465 uses
// implicit TLS; everything else (typically 587) uses STARTTLS.
func (a *alerter) smtpSend(msg []byte) error {
	addr := net.JoinHostPort(a.host, strconv.Itoa(a.port))
	tlsCfg := &tls.Config{ServerName: a.host, MinVersion: tls.VersionTLS12}

	var conn net.Conn
	var err error
	dialer := &net.Dialer{Timeout: 10 * time.Second}
	if a.port == 465 {
		td := &tls.Dialer{NetDialer: dialer, Config: tlsCfg}
		conn, err = td.DialContext(context.Background(), "tcp", addr)
	} else {
		conn, err = dialer.DialContext(context.Background(), "tcp", addr)
	}
	if err != nil {
		return fmt.Errorf("dialing %s: %w", addr, err)
	}
	defer conn.Close()

	c, err := smtp.NewClient(conn, a.host)
	if err != nil {
		return fmt.Errorf("smtp client: %w", err)
	}
	defer c.Close()

	if a.port != 465 {
		if ok, _ := c.Extension("STARTTLS"); !ok {
			return errors.New("relay does not support STARTTLS")
		}
		if err := c.StartTLS(tlsCfg); err != nil {
			return fmt.Errorf("STARTTLS: %w", err)
		}
	}
	if a.user != "" {
		if err := c.Auth(smtp.PlainAuth("", a.user, a.pass, a.host)); err != nil {
			return fmt.Errorf("auth: %w", err)
		}
	}
	if err := c.Mail(a.from); err != nil {
		return fmt.Errorf("MAIL FROM: %w", err)
	}
	if err := c.Rcpt(a.to); err != nil {
		return fmt.Errorf("RCPT TO: %w", err)
	}
	w, err := c.Data()
	if err != nil {
		return fmt.Errorf("DATA: %w", err)
	}
	if _, err := w.Write(msg); err != nil {
		return fmt.Errorf("writing body: %w", err)
	}
	if err := w.Close(); err != nil {
		return fmt.Errorf("closing body: %w", err)
	}
	return c.Quit()
}
