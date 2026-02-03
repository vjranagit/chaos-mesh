package retry

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestExponentialBackoff_NextDelay(t *testing.T) {
	backoff := NewExponentialBackoff(100*time.Millisecond, 1*time.Second)
	
	tests := []struct {
		attempt int
		min     time.Duration
		max     time.Duration
	}{
		{0, 100 * time.Millisecond, 100 * time.Millisecond},
		{1, 200 * time.Millisecond, 200 * time.Millisecond},
		{2, 400 * time.Millisecond, 400 * time.Millisecond},
		{5, 1 * time.Second, 1 * time.Second}, // Max capped
	}
	
	for _, tt := range tests {
		delay := backoff.NextDelay(tt.attempt)
		if delay < tt.min || delay > tt.max {
			t.Errorf("attempt %d: expected %v-%v, got %v", tt.attempt, tt.min, tt.max, delay)
		}
	}
}

func TestConstantDelay_NextDelay(t *testing.T) {
	delay := NewConstantDelay(500 * time.Millisecond)
	
	for i := 0; i < 5; i++ {
		d := delay.NextDelay(i)
		if d != 500*time.Millisecond {
			t.Errorf("expected 500ms, got %v", d)
		}
	}
}

func TestDo_Success(t *testing.T) {
	ctx := context.Background()
	config := DefaultRetryConfig()
	
	attempts := 0
	err := Do(ctx, config, func() error {
		attempts++
		if attempts < 3 {
			return errors.New("temporary error")
		}
		return nil
	})
	
	if err != nil {
		t.Errorf("expected success after retries, got error: %v", err)
	}
	
	if attempts != 3 {
		t.Errorf("expected 3 attempts, got %d", attempts)
	}
}

func TestDo_MaxRetriesExceeded(t *testing.T) {
	ctx := context.Background()
	config := &RetryConfig{
		MaxAttempts: 3,
		Strategy:    NewConstantDelay(10 * time.Millisecond),
		RetryIf:     func(err error) bool { return true },
		OnRetry:     func(attempt int, err error) {},
	}
	
	attempts := 0
	err := Do(ctx, config, func() error {
		attempts++
		return errors.New("persistent error")
	})
	
	if !errors.Is(err, ErrMaxRetriesExceeded) {
		t.Errorf("expected ErrMaxRetriesExceeded, got %v", err)
	}
	
	if attempts != 3 {
		t.Errorf("expected 3 attempts, got %d", attempts)
	}
}

func TestDo_NonRetryableError(t *testing.T) {
	ctx := context.Background()
	
	nonRetryable := errors.New("non-retryable")
	config := &RetryConfig{
		MaxAttempts: 5,
		Strategy:    NewConstantDelay(10 * time.Millisecond),
		RetryIf: func(err error) bool {
			return err != nonRetryable
		},
	}
	
	attempts := 0
	err := Do(ctx, config, func() error {
		attempts++
		return nonRetryable
	})
	
	if err != nonRetryable {
		t.Errorf("expected non-retryable error, got %v", err)
	}
	
	if attempts != 1 {
		t.Errorf("expected 1 attempt (no retry), got %d", attempts)
	}
}

func TestDo_ContextCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	
	config := &RetryConfig{
		MaxAttempts: 10,
		Strategy:    NewConstantDelay(100 * time.Millisecond),
		RetryIf:     func(err error) bool { return true },
		OnRetry:     func(attempt int, err error) {},
	}
	
	attempts := 0
	go func() {
		time.Sleep(150 * time.Millisecond)
		cancel()
	}()
	
	err := Do(ctx, config, func() error {
		attempts++
		return errors.New("error")
	})
	
	if !errors.Is(err, context.Canceled) {
		t.Errorf("expected context.Canceled, got %v", err)
	}
	
	if attempts > 3 {
		t.Errorf("expected few attempts before cancellation, got %d", attempts)
	}
}

func TestCircuitBreaker_Closed(t *testing.T) {
	cb := NewCircuitBreaker(CircuitBreakerConfig{
		MaxFailures:  3,
		ResetTimeout: 1 * time.Second,
	})
	
	if cb.State() != StateClosed {
		t.Errorf("expected StateClosed, got %v", cb.State())
	}
	
	err := cb.Execute(func() error {
		return nil
	})
	
	if err != nil {
		t.Errorf("expected no error, got %v", err)
	}
	
	if cb.State() != StateClosed {
		t.Errorf("expected to remain StateClosed, got %v", cb.State())
	}
}

func TestCircuitBreaker_Open(t *testing.T) {
	cb := NewCircuitBreaker(CircuitBreakerConfig{
		MaxFailures:  3,
		ResetTimeout: 1 * time.Second,
	})
	
	testErr := errors.New("test error")
	
	// Cause failures to open circuit
	for i := 0; i < 3; i++ {
		cb.Execute(func() error {
			return testErr
		})
	}
	
	if cb.State() != StateOpen {
		t.Errorf("expected StateOpen, got %v", cb.State())
	}
	
	// Circuit should reject calls
	err := cb.Execute(func() error {
		return nil
	})
	
	if !errors.Is(err, ErrCircuitOpen) {
		t.Errorf("expected ErrCircuitOpen, got %v", err)
	}
}

func TestCircuitBreaker_HalfOpen(t *testing.T) {
	cb := NewCircuitBreaker(CircuitBreakerConfig{
		MaxFailures:  2,
		ResetTimeout: 100 * time.Millisecond,
		HalfOpenMax:  1,
	})
	
	testErr := errors.New("test error")
	
	// Open circuit
	cb.Execute(func() error { return testErr })
	cb.Execute(func() error { return testErr })
	
	if cb.State() != StateOpen {
		t.Fatal("circuit should be open")
	}
	
	// Wait for reset timeout
	time.Sleep(150 * time.Millisecond)
	
	if cb.State() != StateHalfOpen {
		t.Errorf("expected StateHalfOpen after timeout, got %v", cb.State())
	}
	
	// Success in half-open should close circuit
	err := cb.Execute(func() error {
		return nil
	})
	
	if err != nil {
		t.Errorf("expected success, got %v", err)
	}
	
	if cb.State() != StateClosed {
		t.Errorf("expected StateClosed after success, got %v", cb.State())
	}
}

func TestCircuitBreaker_HalfOpenFailure(t *testing.T) {
	cb := NewCircuitBreaker(CircuitBreakerConfig{
		MaxFailures:  2,
		ResetTimeout: 100 * time.Millisecond,
	})
	
	testErr := errors.New("test error")
	
	// Open circuit
	cb.Execute(func() error { return testErr })
	cb.Execute(func() error { return testErr })
	
	// Wait for reset timeout
	time.Sleep(150 * time.Millisecond)
	
	// Failure in half-open should reopen circuit
	cb.Execute(func() error { return testErr })
	
	if cb.State() != StateOpen {
		t.Errorf("expected StateOpen after half-open failure, got %v", cb.State())
	}
}

func TestCircuitBreaker_Reset(t *testing.T) {
	cb := NewCircuitBreaker(CircuitBreakerConfig{
		MaxFailures: 2,
	})
	
	testErr := errors.New("test error")
	
	// Open circuit
	cb.Execute(func() error { return testErr })
	cb.Execute(func() error { return testErr })
	
	if cb.State() != StateOpen {
		t.Fatal("circuit should be open")
	}
	
	cb.Reset()
	
	if cb.State() != StateClosed {
		t.Errorf("expected StateClosed after reset, got %v", cb.State())
	}
	
	if cb.Failures() != 0 {
		t.Errorf("expected 0 failures after reset, got %d", cb.Failures())
	}
}

func TestResilientExecutor(t *testing.T) {
	executor := NewResilientExecutor(
		&RetryConfig{
			MaxAttempts: 3,
			Strategy:    NewConstantDelay(10 * time.Millisecond),
			RetryIf:     func(err error) bool { return err != nil },
			OnRetry:     func(attempt int, err error) {},
		},
		CircuitBreakerConfig{
			MaxFailures:  5,
			ResetTimeout: 1 * time.Second,
		},
	)
	
	attempts := 0
	err := executor.Execute(context.Background(), func() error {
		attempts++
		if attempts < 2 {
			return errors.New("temporary error")
		}
		return nil
	})
	
	if err != nil {
		t.Errorf("expected success after retry, got %v", err)
	}
	
	if executor.CircuitState() != StateClosed {
		t.Errorf("expected circuit closed, got %v", executor.CircuitState())
	}
}
