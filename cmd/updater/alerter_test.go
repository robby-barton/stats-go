package main

import (
	"errors"
	"strings"
	"testing"
	"time"

	"go.uber.org/zap"
)

var errTest = errors.New("boom")

// fakeClock is a controllable clock for rate-limit tests.
type fakeClock struct {
	now time.Time
}

func (c *fakeClock) Now() time.Time { return c.now }

func (c *fakeClock) Advance(d time.Duration) { c.now = c.now.Add(d) }

func newTestAlerter(t *testing.T, sent *[]string) (*alerter, *fakeClock) {
	t.Helper()
	a := newAlerter(zap.NewNop().Sugar())
	a.to = "ops@example.com"
	a.host = "smtp.example.com"
	a.send = func(msg []byte) error {
		*sent = append(*sent, string(msg))
		return nil
	}
	clock := &fakeClock{now: time.Date(2026, 8, 30, 12, 0, 0, 0, time.UTC)}
	a.now = clock.Now
	return a, clock
}

func TestAlerterFiresAtThreshold(t *testing.T) {
	var sent []string
	a, _ := newTestAlerter(t, &sent)

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

func TestAlerterRateLimitsToOncePerDay(t *testing.T) {
	var sent []string
	a, clock := newTestAlerter(t, &sent)

	// Cross the threshold: one alert.
	for i := 0; i < alertThreshold; i++ {
		a.failure("games", errTest)
	}
	// Keep failing: no further alerts within the interval.
	for i := 0; i < 10; i++ {
		a.failure("games", errTest)
	}
	if len(sent) != 1 {
		t.Fatalf("sent = %d, want 1 (rate-limited within 24h)", len(sent))
	}

	// After 24 hours of continued failure: exactly one more alert.
	clock.Advance(alertInterval + time.Minute)
	a.failure("games", errTest)
	if len(sent) != 2 {
		t.Fatalf("sent = %d, want 2 after the rate-limit window elapsed", len(sent))
	}
}

func TestAlerterPerJobIndependence(t *testing.T) {
	var sent []string
	a, _ := newTestAlerter(t, &sent)

	for i := 0; i < alertThreshold; i++ {
		a.failure("ncaaf games", errTest)
	}
	for i := 0; i < alertThreshold; i++ {
		a.failure("ncaam games", errTest)
	}
	if len(sent) != 2 {
		t.Fatalf("sent = %d, want 2 (one per failing job)", len(sent))
	}
	if !strings.Contains(sent[0], "ncaaf games") || !strings.Contains(sent[1], "ncaam games") {
		t.Errorf("alerts do not identify their jobs:\n%s\n%s", sent[0], sent[1])
	}
}

func TestAlerterSuccessResetsCounter(t *testing.T) {
	var sent []string
	a, _ := newTestAlerter(t, &sent)

	for i := 0; i < alertThreshold-1; i++ {
		a.failure("games", errTest)
	}
	a.success("games")
	for i := 0; i < alertThreshold-1; i++ {
		a.failure("games", errTest)
	}

	if len(sent) != 0 {
		t.Fatalf("sent = %d, want 0: success must reset the counter", len(sent))
	}
}

func TestAlerterRecoveryThenFreshEpisodeAlertsImmediately(t *testing.T) {
	var sent []string
	a, clock := newTestAlerter(t, &sent)

	// Episode 1: alert, then recover.
	for i := 0; i < alertThreshold; i++ {
		a.failure("games", errTest)
	}
	a.success("games")
	if len(sent) != 1 {
		t.Fatalf("sent = %d, want 1 after first episode", len(sent))
	}

	// Episode 2 starts more than 24h after the first alert: alerts immediately.
	clock.Advance(48 * time.Hour)
	for i := 0; i < alertThreshold; i++ {
		a.failure("games", errTest)
	}
	if len(sent) != 2 {
		t.Fatalf("sent = %d, want 2 (fresh episode after recovery)", len(sent))
	}
}

func TestAlerterRecoveryThenSameDayEpisodeIsRateLimited(t *testing.T) {
	var sent []string
	a, _ := newTestAlerter(t, &sent)

	// Episode 1: alert, then recover.
	for i := 0; i < alertThreshold; i++ {
		a.failure("games", errTest)
	}
	a.success("games")

	// Episode 2 the same day: the rate limit still applies.
	for i := 0; i < alertThreshold; i++ {
		a.failure("games", errTest)
	}
	if len(sent) != 1 {
		t.Fatalf("sent = %d, want 1 (same-day episode is rate-limited)", len(sent))
	}
}

func TestAlerterFailedDeliveryRetriesOnNextFailure(t *testing.T) {
	var sent []string
	a, _ := newTestAlerter(t, &sent)
	attempts := 0
	a.send = func([]byte) error {
		attempts++
		return errors.New("smtp down")
	}

	// A failed delivery must not start the rate-limit window: the next
	// failure attempts delivery again rather than waiting 24 hours.
	for i := 0; i < 2*alertThreshold; i++ {
		a.failure("games", errTest)
	}
	if attempts != 2 {
		t.Fatalf("delivery attempts = %d, want 2 (one per threshold crossing)", attempts)
	}
}

func TestAlerterDisabledWithoutRecipient(t *testing.T) {
	var sent []string
	a, _ := newTestAlerter(t, &sent)
	a.to = "" // no ALERT_EMAIL_TO

	for i := 0; i < alertThreshold+1; i++ {
		a.failure("games", errTest)
	}
	if len(sent) != 0 {
		t.Fatalf("sent = %d, want 0 when recipient is unset", len(sent))
	}
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
