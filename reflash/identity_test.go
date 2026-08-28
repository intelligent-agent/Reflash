package main

import (
	"os"
	"path/filepath"
	"sync"
	"testing"
)

// The config partition is reached through a transient systemd mount, and two
// readers of it collide. Reading it once per boot is what stops a flash and a
// UI poll fighting over it (#138), so "how many times was it read" is the
// property under test, not just the value returned.
func countingBins(t *testing.T, dir, revision, serial string) string {
	t.Helper()
	counter := filepath.Join(dir, "reads")
	fakeBin(t, dir, "get-recore-revision",
		"echo x >> "+counter+"\necho "+revision)
	fakeBin(t, dir, "get-recore-serial-number", "echo "+serial)
	return counter
}

func reads(t *testing.T, counter string) int {
	t.Helper()
	b, err := os.ReadFile(counter)
	if err != nil {
		return 0
	}
	n := 0
	for _, c := range b {
		if c == '\n' {
			n++
		}
	}
	return n
}

func resetIdentity() {
	identityMu.Lock()
	identityIsCached = false
	cachedRevision, cachedSerial = "", ""
	identityMu.Unlock()
}

func TestBoardIdentityIsReadOncePerBoot(t *testing.T) {
	dir := setupTest(t)
	counter := countingBins(t, dir, "a7", "0390")
	resetIdentity()
	t.Cleanup(resetIdentity)

	for i := 0; i < 10; i++ {
		rev, ser := boardIdentity()
		if rev != "a7" || ser != "0390" {
			t.Fatalf("got %q/%q, want a7/0390", rev, ser)
		}
	}
	if n := reads(t, counter); n != 1 {
		t.Errorf("read the config partition %d times, want 1", n)
	}
}

// Concurrent callers are the actual failure mode: get_info is per-request and
// the UI polls it while a flash is running.
func TestConcurrentCallersReadItOnce(t *testing.T) {
	dir := setupTest(t)
	counter := countingBins(t, dir, "a7", "0390")
	resetIdentity()
	t.Cleanup(resetIdentity)

	var wg sync.WaitGroup
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func() { defer wg.Done(); boardIdentity() }()
	}
	wg.Wait()

	if n := reads(t, counter); n != 1 {
		t.Errorf("read the config partition %d times under 20 concurrent callers, want 1", n)
	}
}

// A failed read must not be cached: that would make one transient mount failure
// permanent for the rest of the boot, which is the fault #138 describes.
func TestAFailedReadIsNotCached(t *testing.T) {
	dir := setupTest(t)
	counter := countingBins(t, dir, "", "")
	resetIdentity()
	t.Cleanup(resetIdentity)

	for i := 0; i < 3; i++ {
		if rev, _ := boardIdentity(); rev != "" {
			t.Fatalf("expected an empty revision, got %q", rev)
		}
	}
	if n := reads(t, counter); n != 3 {
		t.Errorf("retried %d times, want 3 - a failure must not stick", n)
	}
}

// Provisioning is the one thing that changes the answer.
func TestProvisioningInvalidatesTheCache(t *testing.T) {
	dir := setupTest(t)
	counter := countingBins(t, dir, "a7", "0390")
	resetIdentity()
	t.Cleanup(resetIdentity)

	boardIdentity()
	boardIdentity()
	if n := reads(t, counter); n != 1 {
		t.Fatalf("setup: read %d times, want 1", n)
	}

	invalidateBoardIdentity()
	boardIdentity()

	if n := reads(t, counter); n != 2 {
		t.Errorf("read %d times after invalidation, want 2", n)
	}
}
