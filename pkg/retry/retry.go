package retry

import (
	"context"
	"errors"
	"fmt"
	"math"
	"time"
)

var (
	// ErrMaxRetriesExceeded is returned when retry limit is reached
	ErrMaxRetriesExceeded = errors.New("maximum retries exceeded")

	// ErrCircuitOpen is returned when circuit breaker is open
	ErrCircuitOpen = errors.New("circuit breaker is open")
)

// Strategy defines retry behavior
type Strategy interface {
	// NextDelay returns the delay for the next retry
	NextDelay(attempt int) time.Duration
}

// ExponentialBackoff implements exponential backoff strategy
type ExponentialBackoff struct {
	InitialDelay time.Duration
	MaxDelay     time.Duration
	Multiplier   float64
}

// NewExponentialBackoff creates exponential backoff strategy
func NewExponentialBackoff(initial, max time.Duration) *ExponentialBackoff {
	return &ExponentialBackoff{
		InitialDelay: initial,
		MaxDelay:     max,
		Multiplier:   2.0,
	}
}

// NextDelay calculates exponential delay
func (e *ExponentialBackoff) NextDelay(attempt int) time.Duration {
	delay := float64(e.InitialDelay) * math.Pow(e.Multiplier, float64(attempt))
	if delay > float64(e.MaxDelay) {
		return e.MaxDelay
	}
	return time.Duration(delay)
}

// ConstantDelay implements fixed delay strategy
type ConstantDelay struct {
	Delay time.Duration
}

// NewConstantDelay creates constant delay strategy
func NewConstantDelay(delay time.Duration) *ConstantDelay {
	return &ConstantDelay{Delay: delay}
}

// NextDelay returns constant delay
func (c *ConstantDelay) NextDelay(attempt int) time.Duration {
	return c.Delay
}

// RetryConfig configures retry behavior
type RetryConfig struct {
	MaxAttempts int
	Strategy    Strategy
	RetryIf     func(error) bool
	OnRetry     func(attempt int, err error)
}

// DefaultRetryConfig returns sensible defaults
func DefaultRetryConfig() *RetryConfig {
	return &RetryConfig{
		MaxAttempts: 3,
		Strategy:    NewExponentialBackoff(100*time.Millisecond, 5*time.Second),
		RetryIf: func(err error) bool {
			return err != nil
		},
		OnRetry: func(attempt int, err error) {},
	}
}

// Do executes function with retry logic
func Do(ctx context.Context, config *RetryConfig, fn func() error) error {
	if config == nil {
		config = DefaultRetryConfig()
	}

	var lastErr error
	for attempt := 0; attempt < config.MaxAttempts; attempt++ {
		// Check context cancellation
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		// Execute function
		err := fn()
		if err == nil {
			return nil
		}

		lastErr = err

		// Check if we should retry
		if !config.RetryIf(err) {
			return err
		}

		// Last attempt, don't delay
		if attempt == config.MaxAttempts-1 {
			break
		}

		// Call retry callback
		config.OnRetry(attempt, err)

		// Wait before next attempt
		delay := config.Strategy.NextDelay(attempt)
		select {
		case <-time.After(delay):
		case <-ctx.Done():
			return ctx.Err()
		}
	}

	return fmt.Errorf("%w: %v", ErrMaxRetriesExceeded, lastErr)
}

// CircuitState represents circuit breaker state
type CircuitState string

const (
	StateClosed   CircuitState = "closed"
	StateOpen     CircuitState = "open"
	StateHalfOpen CircuitState = "half-open"
)

// CircuitBreaker implements the circuit breaker pattern
type CircuitBreaker struct {
	maxFailures    int
	resetTimeout   time.Duration
	halfOpenMax    int
	state          CircuitState
	failures       int
	lastFailTime   time.Time
	halfOpenCalls  int
	onStateChange  func(from, to CircuitState)
}

// CircuitBreakerConfig configures circuit breaker
type CircuitBreakerConfig struct {
	MaxFailures   int
	ResetTimeout  time.Duration
	HalfOpenMax   int
	OnStateChange func(from, to CircuitState)
}

// NewCircuitBreaker creates a circuit breaker
func NewCircuitBreaker(config CircuitBreakerConfig) *CircuitBreaker {
	if config.MaxFailures == 0 {
		config.MaxFailures = 5
	}
	if config.ResetTimeout == 0 {
		config.ResetTimeout = 30 * time.Second
	}
	if config.HalfOpenMax == 0 {
		config.HalfOpenMax = 1
	}
	if config.OnStateChange == nil {
		config.OnStateChange = func(from, to CircuitState) {}
	}

	return &CircuitBreaker{
		maxFailures:   config.MaxFailures,
		resetTimeout:  config.ResetTimeout,
		halfOpenMax:   config.HalfOpenMax,
		state:         StateClosed,
		onStateChange: config.OnStateChange,
	}
}

// Execute runs function with circuit breaker protection
func (cb *CircuitBreaker) Execute(fn func() error) error {
	// Check if circuit should transition from Open to Half-Open
	if cb.state == StateOpen {
		if time.Since(cb.lastFailTime) > cb.resetTimeout {
			cb.setState(StateHalfOpen)
			cb.halfOpenCalls = 0
		} else {
			return ErrCircuitOpen
		}
	}

	// Limit calls in half-open state
	if cb.state == StateHalfOpen {
		if cb.halfOpenCalls >= cb.halfOpenMax {
			return ErrCircuitOpen
		}
		cb.halfOpenCalls++
	}

	// Execute function
	err := fn()

	// Handle result
	if err != nil {
		cb.recordFailure()
		return err
	}

	cb.recordSuccess()
	return nil
}

// recordFailure handles failure
func (cb *CircuitBreaker) recordFailure() {
	cb.failures++
	cb.lastFailTime = time.Now()

	if cb.state == StateHalfOpen {
		// Single failure in half-open reopens circuit
		cb.setState(StateOpen)
		cb.failures = cb.maxFailures
	} else if cb.failures >= cb.maxFailures {
		cb.setState(StateOpen)
	}
}

// recordSuccess handles success
func (cb *CircuitBreaker) recordSuccess() {
	if cb.state == StateHalfOpen {
		// Success in half-open closes circuit
		cb.setState(StateClosed)
		cb.failures = 0
		cb.halfOpenCalls = 0
	} else if cb.state == StateClosed {
		// Reset failure count on success
		cb.failures = 0
	}
}

// setState transitions circuit state
func (cb *CircuitBreaker) setState(newState CircuitState) {
	if cb.state != newState {
		oldState := cb.state
		cb.state = newState
		cb.onStateChange(oldState, newState)
	}
}

// State returns current circuit state
func (cb *CircuitBreaker) State() CircuitState {
	if cb.state == StateOpen && time.Since(cb.lastFailTime) > cb.resetTimeout {
		return StateHalfOpen
	}
	return cb.state
}

// Failures returns current failure count
func (cb *CircuitBreaker) Failures() int {
	return cb.failures
}

// Reset manually resets the circuit breaker
func (cb *CircuitBreaker) Reset() {
	cb.setState(StateClosed)
	cb.failures = 0
	cb.halfOpenCalls = 0
}

// ResilientExecutor combines retry and circuit breaker
type ResilientExecutor struct {
	retryConfig    *RetryConfig
	circuitBreaker *CircuitBreaker
}

// NewResilientExecutor creates an executor with retry + circuit breaker
func NewResilientExecutor(retryConfig *RetryConfig, cbConfig CircuitBreakerConfig) *ResilientExecutor {
	if retryConfig == nil {
		retryConfig = DefaultRetryConfig()
	}

	return &ResilientExecutor{
		retryConfig:    retryConfig,
		circuitBreaker: NewCircuitBreaker(cbConfig),
	}
}

// Execute runs function with retry and circuit breaker
func (r *ResilientExecutor) Execute(ctx context.Context, fn func() error) error {
	return r.circuitBreaker.Execute(func() error {
		return Do(ctx, r.retryConfig, fn)
	})
}

// CircuitState returns current circuit breaker state
func (r *ResilientExecutor) CircuitState() CircuitState {
	return r.circuitBreaker.State()
}

// ResetCircuit manually resets the circuit breaker
func (r *ResilientExecutor) ResetCircuit() {
	r.circuitBreaker.Reset()
}
