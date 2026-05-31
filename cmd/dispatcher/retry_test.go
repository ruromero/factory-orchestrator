package main

import (
	"context"
	"errors"
	"fmt"
	"os/exec"
	"runtime"
	"testing"
)

func TestIsRetryable(t *testing.T) {
	t.Run("deadline exceeded is retryable", func(t *testing.T) {
		if !isRetryable(context.DeadlineExceeded) {
			t.Error("expected DeadlineExceeded to be retryable")
		}
	})

	t.Run("wrapped deadline exceeded is retryable", func(t *testing.T) {
		err := fmt.Errorf("phase failed: %w", context.DeadlineExceeded)
		if !isRetryable(err) {
			t.Error("expected wrapped DeadlineExceeded to be retryable")
		}
	})

	t.Run("generic error is not retryable", func(t *testing.T) {
		if isRetryable(errors.New("something broke")) {
			t.Error("expected generic error to not be retryable")
		}
	})

	t.Run("context canceled is not retryable", func(t *testing.T) {
		if isRetryable(context.Canceled) {
			t.Error("expected context.Canceled to not be retryable")
		}
	})

	t.Run("exit code 1 is not retryable", func(t *testing.T) {
		err := runAndGetError("exit 1")
		if err == nil {
			t.Fatal("expected error from exit 1")
		}
		if isRetryable(err) {
			t.Error("expected exit code 1 to not be retryable")
		}
	})

	t.Run("exit code 2 is not retryable", func(t *testing.T) {
		err := runAndGetError("exit 2")
		if err == nil {
			t.Fatal("expected error from exit 2")
		}
		if isRetryable(err) {
			t.Error("expected exit code 2 to not be retryable")
		}
	})

	if runtime.GOOS != "windows" {
		t.Run("signal kill exit 137 is retryable", func(t *testing.T) {
			// exit 137 simulates SIGKILL (128 + 9)
			err := runAndGetError("exit 137")
			if err == nil {
				t.Fatal("expected error from exit 137")
			}
			if !isRetryable(err) {
				t.Error("expected exit code 137 (SIGKILL) to be retryable")
			}
		})

		t.Run("exit 129 SIGHUP is retryable", func(t *testing.T) {
			err := runAndGetError("exit 129")
			if err == nil {
				t.Fatal("expected error from exit 129")
			}
			if !isRetryable(err) {
				t.Error("expected exit code 129 (SIGHUP) to be retryable")
			}
		})
	}
}

// runAndGetError runs a shell command and returns the resulting error.
func runAndGetError(shellCmd string) error {
	cmd := exec.Command("sh", "-c", shellCmd)
	return cmd.Run()
}
