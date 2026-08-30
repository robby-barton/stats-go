package main

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"

	"go.uber.org/zap"
)

var errTest = errors.New("boom")

func newTestAlerter(t *testing.T, hits *int) *alerter {
	t.Helper()
	var mu sync.Mutex
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		mu.Lock()
		*hits++
		mu.Unlock()
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(ts.Close)

	a := newAlerter(zap.NewNop().Sugar())
	a.url = ts.URL
	return a
}

func TestAlerterFiresAtThreshold(t *testing.T) {
	var hits int
	a := newTestAlerter(t, &hits)

	for i := 0; i < alertThreshold-1; i++ {
		a.failure("games", errTest)
	}
	if hits != 0 {
		t.Fatalf("alerts fired before threshold: hits = %d", hits)
	}

	a.failure("games", errTest) // hits alertThreshold
	if hits != 1 {
		t.Fatalf("hits = %d, want 1 after crossing threshold", hits)
	}
}

func TestAlerterRealertsEveryThresholdCrossing(t *testing.T) {
	var hits int
	a := newTestAlerter(t, &hits)

	for i := 0; i < alertThreshold; i++ {
		a.failure("games", errTest)
	}
	for i := 0; i < alertThreshold; i++ {
		a.failure("games", errTest)
	}
	if hits != 2 {
		t.Fatalf("hits = %d, want 2 (one per threshold crossing)", hits)
	}
}

func TestAlerterSuccessResetsCounter(t *testing.T) {
	var hits int
	a := newTestAlerter(t, &hits)

	for i := 0; i < alertThreshold-1; i++ {
		a.failure("games", errTest)
	}
	a.success()
	a.failure("games", errTest)
	a.failure("games", errTest)

	if hits != 0 {
		t.Fatalf("hits = %d, want 0: success must reset the counter", hits)
	}
}

func TestAlerterPayloadIsWebhookCompatible(t *testing.T) {
	var got map[string]string
	done := make(chan struct{})
	ts := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		if err := json.NewDecoder(r.Body).Decode(&got); err != nil {
			t.Errorf("decoding payload: %v", err)
		}
		close(done)
	}))
	defer ts.Close()

	a := newAlerter(zap.NewNop().Sugar())
	a.url = ts.URL
	for i := 0; i < alertThreshold; i++ {
		a.failure("games", errTest)
	}
	<-done

	if got["content"] == "" || got["text"] == "" {
		t.Fatalf("payload missing content/text keys: %v", got)
	}
}

func TestAlerterNoopWithoutURL(t *testing.T) {
	a := newAlerter(zap.NewNop().Sugar())
	if a.url != "" {
		t.Fatalf("expected empty URL without ALERT_WEBHOOK set")
	}
	// Must not panic and must not attempt any HTTP call (no server configured).
	for i := 0; i < alertThreshold+1; i++ {
		a.failure("games", errTest)
	}
}
