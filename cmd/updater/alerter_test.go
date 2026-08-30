package main

import (
	"errors"
	"strings"
	"testing"

	"go.uber.org/zap"
)

var errTest = errors.New("boom")

func newTestAlerter(sent *[][]byte) *alerter {
	a := newAlerter(zap.NewNop().Sugar())
	a.to = "ops@example.com"
	a.host = "smtp.example.com"
	a.send = func(msg []byte) error {
		*sent = append(*sent, msg)
		return nil
	}
	return a
}

func TestAlerterFiresAtThreshold(t *testing.T) {
	var sent [][]byte
	a := newTestAlerter(&sent)

	for i := 0; i < alertThreshold-1; i++ {
		a.failure("games", errTest)
	}
	if len(sent) != 0 {
		t.Fatalf("alerts sent before threshold: %d", len(sent))
	}

	a.failure("games", errTest) // hits alertThreshold
	if len(sent) != 1 {
		t.Fatalf("sent = %d, want 1 after crossing threshold", len(sent))
	}
}

func TestAlerterRealertsEveryThresholdCrossing(t *testing.T) {
	var sent [][]byte
	a := newTestAlerter(&sent)

	for i := 0; i < 2*alertThreshold; i++ {
		a.failure("games", errTest)
	}
	if len(sent) != 2 {
		t.Fatalf("sent = %d, want 2 (one per threshold crossing)", len(sent))
	}
}

func TestAlerterSuccessResetsCounter(t *testing.T) {
	var sent [][]byte
	a := newTestAlerter(&sent)

	for i := 0; i < alertThreshold-1; i++ {
		a.failure("games", errTest)
	}
	a.success()
	a.failure("games", errTest)
	a.failure("games", errTest)

	if len(sent) != 0 {
		t.Fatalf("sent = %d, want 0: success must reset the counter", len(sent))
	}
}

func TestAlerterDisabledWithoutRecipient(t *testing.T) {
	var sent [][]byte
	a := newTestAlerter(&sent)
	a.to = "" // no ALERT_EMAIL_TO

	for i := 0; i < alertThreshold+1; i++ {
		a.failure("games", errTest)
	}
	if len(sent) != 0 {
		t.Fatalf("sent = %d, want 0 when recipient is unset", len(sent))
	}
}

func TestAlerterSendFailureIsLoggedNotFatal(t *testing.T) {
	t.Helper()
	var sent [][]byte
	a := newTestAlerter(&sent)
	a.send = func([]byte) error { return errors.New("smtp down") }

	// Must not panic; counter still resets so the next crossing re-attempts.
	for i := 0; i < alertThreshold; i++ {
		a.failure("games", errTest)
	}
	for i := 0; i < alertThreshold; i++ {
		a.failure("games", errTest)
	}
	_ = sent
}

func TestBuildMessageHeaders(t *testing.T) {
	msg, err := buildMessage("from@example.com", "to@example.com", "subject line", "body text")
	if err != nil {
		t.Fatalf("buildMessage: %v", err)
	}
	s := string(msg)
	for _, want := range []string{
		"From: from@example.com\r\n",
		"To: to@example.com\r\n",
		"Subject: subject line\r\n",
		"Date: ",
		"MIME-Version: 1.0\r\n",
		"Content-Type: text/plain; charset=UTF-8\r\n",
		"\r\nbody text",
	} {
		if !strings.Contains(s, want) {
			t.Errorf("message missing %q", want)
		}
	}
}

func TestBuildMessageRequiresAddresses(t *testing.T) {
	if _, err := buildMessage("", "to@example.com", "s", "b"); err == nil {
		t.Error("expected error for empty sender")
	}
	if _, err := buildMessage("from@example.com", "", "s", "b"); err == nil {
		t.Error("expected error for empty recipient")
	}
}
