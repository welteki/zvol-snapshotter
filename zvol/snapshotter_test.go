package zvol

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestWaitForFile_AlreadyExists(t *testing.T) {
	dir := t.TempDir()
	filePath := filepath.Join(dir, "existing")
	if err := os.WriteFile(filePath, nil, 0644); err != nil {
		t.Fatal(err)
	}

	start := time.Now()
	err := waitForFile(context.Background(), filePath)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if elapsed := time.Since(start); elapsed > 10*time.Millisecond {
		t.Errorf("fast path took %s, expected <10ms", elapsed)
	}
}

func TestWaitForFile_CreatedAfterWatch(t *testing.T) {
	dir := t.TempDir()
	filePath := filepath.Join(dir, "delayed")

	createDelay := 50 * time.Millisecond
	go func() {
		time.Sleep(createDelay)
		if err := os.WriteFile(filePath, nil, 0644); err != nil {
			t.Errorf("failed to create file: %v", err)
		}
	}()

	start := time.Now()
	err := waitForFile(context.Background(), filePath)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	elapsed := time.Since(start)
	// Should wake up promptly after file creation via inotify,
	// not significantly later than the creation delay.
	if elapsed > createDelay+200*time.Millisecond {
		t.Errorf("took %s, expected roughly %s", elapsed, createDelay)
	}
	t.Logf("waited %s for file (created after %s)", elapsed, createDelay)
}

func TestWaitForFile_ContextCancelled(t *testing.T) {
	dir := t.TempDir()
	filePath := filepath.Join(dir, "never-created")

	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	err := waitForFile(ctx, filePath)
	if err == nil {
		t.Fatal("expected error on context cancellation")
	}
	if err != context.DeadlineExceeded {
		t.Logf("got error: %v (acceptable)", err)
	}
}

func TestWaitForFilePolling_CreatedAfterStart(t *testing.T) {
	dir := t.TempDir()
	filePath := filepath.Join(dir, "polled")

	createDelay := 30 * time.Millisecond
	go func() {
		time.Sleep(createDelay)
		if err := os.WriteFile(filePath, nil, 0644); err != nil {
			t.Errorf("failed to create file: %v", err)
		}
	}()

	start := time.Now()
	err := waitForFilePolling(context.Background(), filePath)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	elapsed := time.Since(start)
	// Polling at 5ms intervals, so should detect within ~createDelay + 5ms.
	if elapsed > createDelay+50*time.Millisecond {
		t.Errorf("polling took %s, expected roughly %s", elapsed, createDelay)
	}
	t.Logf("polling waited %s for file (created after %s)", elapsed, createDelay)
}

func TestWaitForFilePolling_ContextCancelled(t *testing.T) {
	dir := t.TempDir()
	filePath := filepath.Join(dir, "never-created")

	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	err := waitForFilePolling(ctx, filePath)
	if err == nil {
		t.Fatal("expected error on context cancellation")
	}
}

func TestWaitForFile_ParentDirCreatedLate(t *testing.T) {
	// Simulates the real scenario: after commit sets volmode=none,
	// udev removes the /dev/zvol/pool/dataset/ directory. The next
	// clone creates the zvol, and udev must recreate both the
	// directory and the device symlink.
	base := t.TempDir()
	subdir := filepath.Join(base, "late-parent")
	filePath := filepath.Join(subdir, "device")

	createDelay := 80 * time.Millisecond
	go func() {
		time.Sleep(createDelay)
		if err := os.MkdirAll(subdir, 0755); err != nil {
			t.Errorf("failed to create subdir: %v", err)
			return
		}
		if err := os.WriteFile(filePath, nil, 0644); err != nil {
			t.Errorf("failed to create file: %v", err)
		}
	}()

	start := time.Now()
	err := waitForFile(context.Background(), filePath)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	elapsed := time.Since(start)
	if elapsed > createDelay+200*time.Millisecond {
		t.Errorf("took %s, expected roughly %s", elapsed, createDelay)
	}
	t.Logf("waited %s for file with late parent dir (created after %s)", elapsed, createDelay)
}
