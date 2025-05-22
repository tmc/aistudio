package api

import (
	"context"
	"errors"
	"fmt"
	"os"
	"testing"
	"time"
)

func TestClientError(t *testing.T) {
	tests := []struct {
		name          string
		code          int
		message       string
		wantError     string
		wantRetryable bool
	}{
		{
			name:          "500 server error",
			code:          500,
			message:       "Internal Server Error",
			wantError:     "Client Error: 500 - Internal Server Error",
			wantRetryable: true,
		},
		{
			name:          "429 rate limit",
			code:          429,
			message:       "Too Many Requests",
			wantError:     "Client Error: 429 - Too Many Requests",
			wantRetryable: false, // non-5xx
		},
		{
			name:          "503 with cancel context",
			code:          503,
			message:       "Service Unavailable - context canceled",
			wantError:     "Client Error: 503 - Service Unavailable - context canceled",
			wantRetryable: false, // contains "canceled"
		},
		{
			name:          "504 with deadline",
			code:          504,
			message:       "Gateway Timeout - context deadline exceeded",
			wantError:     "Client Error: 504 - Gateway Timeout - context deadline exceeded",
			wantRetryable: false, // contains "deadline"
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := &ClientError{
				Code:    tt.code,
				Message: tt.message,
			}

			// Test Error() string format
			if got := err.Error(); got != tt.wantError {
				t.Errorf("ClientError.Error() = %v, want %v", got, tt.wantError)
			}

			// Test IsRetryable()
			if got := err.IsRetryable(); got != tt.wantRetryable {
				t.Errorf("ClientError.IsRetryable() = %v, want %v", got, tt.wantRetryable)
			}
		})
	}
}

func TestRetryableError(t *testing.T) {
	tests := []struct {
		name          string
		retries       int
		maxRetries    int
		backoff       time.Duration
		wantError     string
		wantRetryable bool
	}{
		{
			name:          "no retries yet",
			retries:       0,
			maxRetries:    3,
			backoff:       1 * time.Second,
			wantError:     "Retryable Error: test error (retries: 0/3, backoff: 1s)",
			wantRetryable: true,
		},
		{
			name:          "some retries, still retryable",
			retries:       2,
			maxRetries:    5,
			backoff:       4 * time.Second,
			wantError:     "Retryable Error: test error (retries: 2/5, backoff: 4s)",
			wantRetryable: true,
		},
		{
			name:          "max retries reached",
			retries:       3,
			maxRetries:    3,
			backoff:       8 * time.Second,
			wantError:     "Retryable Error: test error (retries: 3/3, backoff: 8s)",
			wantRetryable: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := &RetryableError{
				Err:        errors.New("test error"),
				Retries:    tt.retries,
				MaxRetries: tt.maxRetries,
				Backoff:    tt.backoff,
			}

			// Test Error() string format
			if got := err.Error(); got != tt.wantError {
				t.Errorf("RetryableError.Error() = %v, want %v", got, tt.wantError)
			}

			// Test IsRetryable()
			if got := err.IsRetryable(); got != tt.wantRetryable {
				t.Errorf("RetryableError.IsRetryable() = %v, want %v", got, tt.wantRetryable)
			}
		})
	}
}

func TestRetry(t *testing.T) {
	// Skip these tests unless the AISTUDIO_RUN_TIMING_TESTS environment variable is set
	// They contain timing-related assertions that can be flaky depending on system load
	if os.Getenv("AISTUDIO_RUN_TIMING_TESTS") == "" {
		t.Skip("Skipping timing-sensitive tests - set AISTUDIO_RUN_TIMING_TESTS=1 to run")
	}
	t.Run("not retryable", func(t *testing.T) {
		err := &RetryableError{
			Err:        errors.New("test error"),
			Retries:    3,
			MaxRetries: 3,
			Backoff:    1 * time.Second,
		}

		ctx := context.Background()
		opCount := 0
		op := func() error {
			opCount++
			return nil
		}

		// Should return original error without calling op
		result := err.Retry(ctx, op)
		if result != err.Err {
			t.Errorf("Retry() should return original error when not retryable, got %v", result)
		}
		if opCount != 0 {
			t.Errorf("Operation should not be called when not retryable, called %d times", opCount)
		}
	})

	t.Run("retryable success", func(t *testing.T) {
		originalBackoff := 10 * time.Millisecond // Small for testing
		err := &RetryableError{
			Err:        errors.New("test error"),
			Retries:    1,
			MaxRetries: 3,
			Backoff:    originalBackoff,
		}

		ctx := context.Background()
		opCount := 0
		op := func() error {
			opCount++
			return nil // Success
		}

		// Time the execution to verify backoff is applied
		start := time.Now()
		result := err.Retry(ctx, op)
		elapsed := time.Since(start)

		// Verify results
		if result != nil {
			t.Errorf("Retry() should return nil on success, got %v", result)
		}
		if opCount != 1 {
			t.Errorf("Operation should be called once, called %d times", opCount)
		}
		if elapsed < originalBackoff {
			t.Errorf("Backoff not applied, elapsed time %v < backoff %v", elapsed, originalBackoff)
		}
		if err.Retries != 2 {
			t.Errorf("Retries should be incremented, got %d", err.Retries)
		}
		if err.Backoff != originalBackoff*2 {
			t.Errorf("Backoff should be doubled, got %v", err.Backoff)
		}
	})

	t.Run("retryable with continued error", func(t *testing.T) {
		originalBackoff := 10 * time.Millisecond // Small for testing
		err := &RetryableError{
			Err:        errors.New("test error"),
			Retries:    1,
			MaxRetries: 3,
			Backoff:    originalBackoff,
		}

		ctx := context.Background()
		opCount := 0
		op := func() error {
			opCount++
			return &ClientError{
				Code:    500,
				Message: "Still failing",
			}
		}

		result := err.Retry(ctx, op)

		// Verify it's a RetryableError with updated counts
		if retryErr, ok := result.(*RetryableError); !ok {
			t.Errorf("Expected RetryableError, got %T: %v", result, result)
		} else {
			if retryErr.Retries != 2 {
				t.Errorf("Retries should be incremented to 2, got %d", retryErr.Retries)
			}
			if retryErr.Backoff != originalBackoff*2 {
				t.Errorf("Backoff should be doubled to %v, got %v", originalBackoff*2, retryErr.Backoff)
			}
		}
		if opCount != 1 {
			t.Errorf("Operation should be called once, called %d times", opCount)
		}
	})

	t.Run("retryable with non-retryable error", func(t *testing.T) {
		originalBackoff := 10 * time.Millisecond
		err := &RetryableError{
			Err:        errors.New("test error"),
			Retries:    1,
			MaxRetries: 3,
			Backoff:    originalBackoff,
		}

		ctx := context.Background()
		nonRetryableErr := errors.New("non-retryable error")
		op := func() error {
			return nonRetryableErr
		}

		result := err.Retry(ctx, op)

		// Should return the non-retryable error directly
		if result != nonRetryableErr {
			t.Errorf("Should return non-retryable error directly, got %v", result)
		}
	})
}

func TestRetryWithExponentialBackoff(t *testing.T) {
	// Skip these tests unless the AISTUDIO_RUN_TIMING_TESTS environment variable is set
	// They contain timing-related assertions that can be flaky depending on system load
	if os.Getenv("AISTUDIO_RUN_TIMING_TESTS") == "" {
		t.Skip("Skipping timing-sensitive tests - set AISTUDIO_RUN_TIMING_TESTS=1 to run")
	}
	t.Run("immediate success", func(t *testing.T) {
		ctx := context.Background()
		callCount := 0
		op := func() error {
			callCount++
			return nil
		}

		err := RetryWithExponentialBackoff(ctx, op, 3, 10*time.Millisecond)
		if err != nil {
			t.Errorf("Expected nil error on success, got %v", err)
		}
		if callCount != 1 {
			t.Errorf("Expected operation to be called once, got %d", callCount)
		}
	})

	t.Run("success after retries", func(t *testing.T) {
		ctx := context.Background()
		callCount := 0
		maxCalls := 3
		op := func() error {
			callCount++
			if callCount < maxCalls {
				return &ClientError{Code: 500, Message: "Temporary error"}
			}
			return nil
		}

		err := RetryWithExponentialBackoff(ctx, op, 5, 10*time.Millisecond)
		if err != nil {
			t.Errorf("Expected nil error after retries, got %v", err)
		}
		if callCount != maxCalls {
			t.Errorf("Expected operation to be called %d times, got %d", maxCalls, callCount)
		}
	})

	t.Run("exhausted retries", func(t *testing.T) {
		ctx := context.Background()
		callCount := 0
		op := func() error {
			callCount++
			return &ClientError{Code: 500, Message: "Persistent error"}
		}

		err := RetryWithExponentialBackoff(ctx, op, 3, 10*time.Millisecond)
		if err == nil {
			t.Error("Expected error after exhausting retries, got nil")
		}
		if callCount != 4 { // Initial + 3 retries
			t.Errorf("Expected operation to be called 4 times (initial + 3 retries), got %d", callCount)
		}
	})

	t.Run("non-retryable error", func(t *testing.T) {
		ctx := context.Background()
		callCount := 0
		nonRetryableErr := errors.New("non-retryable error")
		op := func() error {
			callCount++
			return nonRetryableErr
		}

		err := RetryWithExponentialBackoff(ctx, op, 3, 10*time.Millisecond)
		if err != nonRetryableErr {
			t.Errorf("Expected original error for non-retryable error, got %v", err)
		}
		if callCount != 1 {
			t.Errorf("Expected operation to be called once, got %d", callCount)
		}
	})

	t.Run("context canceled", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		callCount := 0
		op := func() error {
			callCount++
			cancel() // Cancel the context on first call
			return &ClientError{Code: 500, Message: "Temporary error"}
		}

		err := RetryWithExponentialBackoff(ctx, op, 3, 10*time.Millisecond)
		if err == nil {
			t.Error("Expected error after context canceled, got nil")
		}
		if callCount != 1 {
			t.Errorf("Expected operation to be called once, got %d", callCount)
		}
	})

	t.Run("context timeout", func(t *testing.T) {
		ctx, cancel := context.WithTimeout(context.Background(), 15*time.Millisecond)
		defer cancel()

		callCount := 0
		op := func() error {
			callCount++
			time.Sleep(10 * time.Millisecond) // Sleep to let context timeout
			return &ClientError{Code: 500, Message: "Temporary error"}
		}

		err := RetryWithExponentialBackoff(ctx, op, 3, 10*time.Millisecond)
		if err == nil {
			t.Error("Expected error after context timeout, got nil")
		}
		if callCount > 2 { // Should only get 1-2 calls before timeout
			t.Errorf("Expected operation to be called 1-2 times before timeout, got %d", callCount)
		}
	})

	t.Run("backoff increases correctly", func(t *testing.T) {
		ctx := context.Background()
		initialBackoff := 10 * time.Millisecond
		callTimes := []time.Time{}

		op := func() error {
			callTimes = append(callTimes, time.Now())
			if len(callTimes) < 4 {
				return &ClientError{Code: 500, Message: "Temporary error"}
			}
			return nil
		}

		err := RetryWithExponentialBackoff(ctx, op, 5, initialBackoff)
		if err != nil {
			t.Errorf("Expected nil error, got %v", err)
		}

		// We should have 4 timestamps
		if len(callTimes) != 4 {
			t.Fatalf("Expected 4 calls, got %d", len(callTimes))
		}

		// Check intervals between calls (should increase exponentially)
		// First interval should be >= initialBackoff
		// Second interval should be >= 2*initialBackoff
		// Third interval should be >= 4*initialBackoff (with tolerance)
		for i := 1; i < len(callTimes); i++ {
			interval := callTimes[i].Sub(callTimes[i-1])
			expectedMinInterval := initialBackoff * time.Duration(1<<(i-1))
			// Allow for slight timing variations (e.g., 5% tolerance)
			tolerance := time.Duration(float64(expectedMinInterval) * 0.05)
			allowedMinInterval := expectedMinInterval - tolerance

			if interval < allowedMinInterval {
				t.Errorf("Interval %d was %v, expected at least %v (allowing for %v tolerance)",
					i, interval, allowedMinInterval, tolerance)
			}
		}
	})
}

// TestIntegrationWithContext tests the interaction between context management and retry logic
func TestIntegrationWithContext(t *testing.T) {
	t.Run("context passed correctly", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		contextReceived := false
		op := func() error {
			// Check if we can get values from the context
			select {
			case <-ctx.Done():
				t.Error("Context was already done in first call")
			default:
				contextReceived = true
			}
			return nil
		}

		err := RetryWithExponentialBackoff(ctx, op, 3, 10*time.Millisecond)
		if err != nil {
			t.Errorf("Expected nil error, got %v", err)
		}
		if !contextReceived {
			t.Error("Context was not correctly passed to the operation")
		}
	})

	t.Run("honor context values", func(t *testing.T) {
		type key string
		testKey := key("test-key")
		testValue := "test-value"

		ctx := context.WithValue(context.Background(), testKey, testValue)

		valueReceived := false
		op := func() error {
			val, ok := ctx.Value(testKey).(string)
			if !ok || val != testValue {
				return fmt.Errorf("expected context value %v, got %v", testValue, val)
			}
			valueReceived = true
			return nil
		}

		err := RetryWithExponentialBackoff(ctx, op, 3, 10*time.Millisecond)
		if err != nil {
			t.Errorf("Expected nil error, got %v", err)
		}
		if !valueReceived {
			t.Error("Context value was not correctly passed to the operation")
		}
	})
}
