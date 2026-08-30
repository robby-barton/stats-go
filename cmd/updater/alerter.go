package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"sync"
	"time"

	"go.uber.org/zap"
)

// alertThreshold is the number of consecutive scheduled-job failures that
// trigger a webhook alert. With the 5-minute games poll, threshold 3 means
// an operator hears about a broken poll within ~15 minutes.
const alertThreshold = 3

// alerter fires a webhook after alertThreshold consecutive scheduled-job
// failures. It is a no-op when ALERT_WEBHOOK is unset. The counter resets
// after alerting, so a persistent outage re-alerts on every threshold
// crossing instead of staying silent, and any success resets it cleanly.
type alerter struct {
	log *zap.SugaredLogger

	mu       sync.Mutex
	url      string
	failures int
	client   *http.Client
}

func newAlerter(log *zap.SugaredLogger) *alerter {
	return &alerter{
		log:    log,
		url:    os.Getenv("ALERT_WEBHOOK"),
		client: &http.Client{Timeout: 10 * time.Second},
	}
}

// failure records a failed job run and alerts on threshold crossings.
func (a *alerter) failure(job string, err error) {
	if a == nil || a.url == "" {
		return
	}
	a.mu.Lock()
	a.failures++
	n := a.failures
	if n >= alertThreshold {
		a.failures = 0
	}
	a.mu.Unlock()
	if n < alertThreshold {
		return
	}

	msg := fmt.Sprintf(
		"stats-go updater: job %q has failed %d consecutive times; last error: %v",
		job, n, err,
	)
	a.log.Errorf("ALERT: %s", msg)
	a.send(msg)
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

// send posts the alert. Both "content" (Discord) and "text" (Slack-style)
// keys are included so the payload works with common webhook receivers.
func (a *alerter) send(msg string) {
	payload, err := json.Marshal(map[string]string{"content": msg, "text": msg})
	if err != nil {
		a.log.Errorf("alert: marshaling payload: %v", err)
		return
	}
	req, err := http.NewRequestWithContext(context.Background(), http.MethodPost, a.url, bytes.NewReader(payload))
	if err != nil {
		a.log.Errorf("alert: building request: %v", err)
		return
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := a.client.Do(req)
	if err != nil {
		a.log.Errorf("alert: posting webhook: %v", err)
		return
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		a.log.Errorf("alert: webhook returned HTTP %d", resp.StatusCode)
	}
}
