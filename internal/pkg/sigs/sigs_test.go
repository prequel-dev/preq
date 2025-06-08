package sigs

import (
	"context"
	"os"
	"syscall"
	"testing"
	"time"
)

func TestHandleKill(t *testing.T) {
	t.Run("context is cancelled on real signal", func(t *testing.T) {
		ctx := handleKill(context.Background(), syscall.SIGUSR1)

		process, err := os.FindProcess(os.Getpid())
		if err != nil {
			t.Fatalf("Failed to find current process: %v", err)
		}

		go func() {
			time.Sleep(20 * time.Millisecond)
			process.Signal(syscall.SIGUSR1)
		}()

		select {
		case <-ctx.Done():
			if err := ctx.Err(); err != context.Canceled {
				t.Errorf("Expected context to be canceled, but got error: %v", err)
			}
		case <-time.After(2 * time.Second):
			t.Fatal("Test timed out: context was not cancelled after signal was sent")
		}
	})
}
