package main

import (
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"go.uber.org/zap"
)

// syncRecorder stores alert emails so the deployer goroutine (which invokes
// the alerter's send stub) and the test goroutine can access them without a
// data race.
type syncRecorder struct {
	mu   sync.Mutex
	msgs []string
}

func (r *syncRecorder) add(msg string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.msgs = append(r.msgs, msg)
}

func (r *syncRecorder) len() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return len(r.msgs)
}

func (r *syncRecorder) get(i int) string {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.msgs[i]
}

// newTestDeployer builds a deployer wired to a stubbed alerter (same pattern
// as newTestAlerter in alerter_test.go) and records alert emails into sent.
// The caller supplies the script path and timeout; run() is not started.
func newTestDeployer(t *testing.T, script string, timeout time.Duration, sent *syncRecorder) *deployer {
	t.Helper()
	a := newAlerter(zap.NewNop().Sugar())
	a.to = "ops@example.com"
	a.host = "smtp.example.com"
	a.send = func(msg []byte) error {
		sent.add(string(msg))
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
		if d.failureCount() == want {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for failure count %d", want)
}

// waitForFile polls until path exists, or the test times out.
func waitForFile(t *testing.T, path string) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if _, err := os.Stat(path); err == nil {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for %s to appear", path)
}

// failureCount reads the site deploy failure counter under the alerter's
// lock, so tests never race with the deployer goroutine.
func (d *deployer) failureCount() int {
	d.al.mu.Lock()
	defer d.al.mu.Unlock()
	return d.al.failures["site deploy"]
}

func TestDeployerSuccessResetsFailures(t *testing.T) {
	var sent syncRecorder
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
	var sent syncRecorder
	d := newTestDeployer(t, writeScript(t, "#!/bin/sh\necho boom >&2\nexit 1\n"), time.Minute, &sent)
	startDeployer(t, d)

	d.Trigger()
	waitForConsecutiveFailures(t, d, 1)

	if got := d.failureCount(); got != 1 {
		t.Fatalf("failures = %d, want 1", got)
	}
}

func TestDeployerTriggerCoalescing(t *testing.T) {
	var sent syncRecorder

	// The first deploy blocks in the script until released; every run appends
	// a line to the runs file so the total run count is observable.
	dir := t.TempDir()
	blocked := filepath.Join(dir, "blocked")
	release := filepath.Join(dir, "release")
	runs := filepath.Join(dir, "runs")
	t.Setenv("DEPLOY_TEST_BLOCKED", blocked)
	t.Setenv("DEPLOY_TEST_RELEASE", release)
	t.Setenv("DEPLOY_TEST_RUNS", runs)
	script := writeScript(t, `#!/bin/sh
echo x >> "$DEPLOY_TEST_RUNS"
touch "$DEPLOY_TEST_BLOCKED"
while [ ! -f "$DEPLOY_TEST_RELEASE" ]; do sleep 0.02; done
exit 1
`)

	d := newTestDeployer(t, script, time.Minute, &sent)
	startDeployer(t, d)

	// Fire one trigger and wait until that deploy is in flight (blocked in
	// the script), so the extra triggers below land while it is running.
	d.Trigger()
	waitForFile(t, blocked)

	// While one deploy is in flight, rapid extra triggers must coalesce into
	// exactly one queued run (at most one in-flight + one pending).
	d.Trigger()
	d.Trigger()
	d.Trigger()

	// Release the in-flight deploy and let the queued one run.
	if err := os.WriteFile(release, nil, 0o644); err != nil {
		t.Fatalf("writing release file: %v", err)
	}
	// The first failure comes from the in-flight run and the second from the
	// single coalesced queued run: exactly one additional deploy completes.
	waitForFailureCount(t, d, 2)

	// Give any (incorrect) third run time to start, then assert the totals.
	time.Sleep(100 * time.Millisecond)
	if got := d.failureCount(); got != 2 {
		t.Fatalf("failures = %d, want 2 (one in-flight + one coalesced run)", got)
	}
	runData, err := os.ReadFile(runs)
	if err != nil {
		t.Fatalf("reading runs file: %v", err)
	}
	if n := strings.Count(string(runData), "\n"); n != 2 {
		t.Fatalf("deploy script ran %d times, want 2", n)
	}
}

func TestDeployerTimeoutKillsHangingScript(t *testing.T) {
	var sent syncRecorder
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
	var sent syncRecorder
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
	for time.Now().Before(deadline) && sent.len() == 0 {
		time.Sleep(10 * time.Millisecond)
	}
	if sent.len() != 1 {
		t.Fatalf("sent = %d, want 1 after crossing alertThreshold", sent.len())
	}
	if !strings.Contains(sent.get(0), "site deploy") {
		t.Errorf("alert does not name the job:\n%s", sent.get(0))
	}
}
