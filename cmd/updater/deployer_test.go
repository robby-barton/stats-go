package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"go.uber.org/zap"
)

// newTestDeployer builds a deployer wired to a stubbed alerter (same pattern
// as newTestAlerter in alerter_test.go) and records alert emails into sent.
// The caller supplies the script path and timeout; run() is not started.
func newTestDeployer(t *testing.T, script string, timeout time.Duration, sent *[]string) *deployer {
	t.Helper()
	a := newAlerter(zap.NewNop().Sugar())
	a.to = "ops@example.com"
	a.host = "smtp.example.com"
	a.send = func(msg []byte) error {
		*sent = append(*sent, string(msg))
		return nil
	}
	return &deployer{
		script:  script,
		log:     zap.NewNop().Sugar(),
		al:      a,
		timeout: timeout,
		trigger: make(chan struct{}, 1),
	}
}

// writeScript writes an executable shell script to a temp dir.
func writeScript(t *testing.T, body string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "deploy.sh")
	if err := os.WriteFile(path, []byte(body), 0o755); err != nil {
		t.Fatalf("writing script: %v", err)
	}
	if err := os.Chmod(path, 0o755); err != nil {
		t.Fatalf("chmod script: %v", err)
	}
	return path
}

// startRuns the deployer loop in the background and stops it when the test
// ends, so no goroutine stays blocked on the trigger channel.
func startDeployer(t *testing.T, d *deployer) {
	t.Helper()
	go d.run()
	t.Cleanup(d.stop)
}

// waitForConsecutiveFailures polls until the alerter has recorded n
// consecutive failures for the site deploy job, or the test times out.
func waitForConsecutiveFailures(t *testing.T, d *deployer, n int) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		d.al.mu.Lock()
		got := d.al.failures["site deploy"]
		d.al.mu.Unlock()
		if got >= n {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for %d consecutive failures of %q", n, "site deploy")
}

// waitForFailureCount polls until the failure counter equals want (0 means
// the counter was reset by a successful deploy).
func waitForFailureCount(t *testing.T, d *deployer, want int) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		d.al.mu.Lock()
		got := d.al.failures["site deploy"]
		d.al.mu.Unlock()
		if got == want {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for failure count %d", want)
}

func TestDeployerSuccessResetsFailures(t *testing.T) {
	var sent []string
	d := newTestDeployer(t, writeScript(t, "#!/bin/sh\nexit 1\n"), time.Minute, &sent)
	startDeployer(t, d)

	// A failure first, so success has something to reset.
	d.Trigger()
	waitForConsecutiveFailures(t, d, 1)

	d.script = writeScript(t, "#!/bin/sh\necho deployed\n")
	d.Trigger()
	waitForFailureCount(t, d, 0)
}

func TestDeployerFailureRecordsAlert(t *testing.T) {
	var sent []string
	d := newTestDeployer(t, writeScript(t, "#!/bin/sh\necho boom >&2\nexit 1\n"), time.Minute, &sent)
	startDeployer(t, d)

	d.Trigger()
	waitForConsecutiveFailures(t, d, 1)

	if got := d.al.failures["site deploy"]; got != 1 {
		t.Fatalf("failures = %d, want 1", got)
	}
}

func TestDeployerTriggerCoalescing(t *testing.T) {
	var sent []string
	d := newTestDeployer(t, writeScript(t, "#!/bin/sh\nexit 1\n"), time.Minute, &sent)
	startDeployer(t, d)

	// Extra triggers while one is pending must be dropped (channel of size 1),
	// so three Triggers still result in a single failed deploy.
	d.Trigger()
	d.Trigger()
	d.Trigger()
	waitForConsecutiveFailures(t, d, 1)
	time.Sleep(100 * time.Millisecond)

	if got := d.al.failures["site deploy"]; got != 1 {
		t.Fatalf("failures = %d, want 1 (coalesced to a single deploy)", got)
	}
}

func TestDeployerTimeoutKillsHangingScript(t *testing.T) {
	var sent []string
	d := newTestDeployer(t, writeScript(t, "#!/bin/sh\nsleep 10\n"), 100*time.Millisecond, &sent)

	// Run the loop manually so we can observe that it returns after the
	// timeout instead of blocking on the script's 10s sleep / open pipe.
	done := make(chan struct{})
	go func() {
		d.run()
		close(done)
	}()
	d.Trigger()
	waitForConsecutiveFailures(t, d, 1)
	d.stop()

	select {
	case <-done:
		// run() exited promptly: the timeout killed the whole process group.
	case <-time.After(5 * time.Second):
		t.Fatal("run() did not return within 5s of a 100ms deploy timeout")
	}
}

func TestDeployerAlertEmailNamesJob(t *testing.T) {
	var sent []string
	d := newTestDeployer(t, writeScript(t, "#!/bin/sh\nexit 1\n"), time.Minute, &sent)
	startDeployer(t, d)

	// Drive the alerter across the threshold via real deploy runs so the
	// email subject is rendered. (On the crossing failure the alerter resets
	// the counter, so the first two runs are observed via the counter and the
	// third via the delivered email.)
	for i := 0; i < alertThreshold-1; i++ {
		d.Trigger()
		waitForConsecutiveFailures(t, d, i+1)
	}
	d.Trigger()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) && len(sent) == 0 {
		time.Sleep(10 * time.Millisecond)
	}
	if len(sent) != 1 {
		t.Fatalf("sent = %d, want 1 after crossing alertThreshold", len(sent))
	}
	if !strings.Contains(sent[0], "site deploy") {
		t.Errorf("alert does not name the job:\n%s", sent[0])
	}
}
