# New Features Documentation

This document describes the three major features added to enhance production-readiness, safety, and observability of the chaos engineering platform.

---

## Feature 1: Experiment Validator & Dry-Run Mode

### Overview
Pre-flight validation system that checks experiment configurations before execution, preventing misconfigurations and enabling safe dry-run testing.

### Problem Solved
Running chaos experiments without validation can lead to:
- Invalid Kubernetes resource names causing failures
- Excessive duration causing unintended disruption
- Misconfigured probes that never succeed
- Invalid target types that fail at runtime

### Implementation Details

**Location:** `pkg/validator/validator.go`

**Key Components:**
- `Validator` - Main validation engine with configurable options
- `ValidationResult` - Structured validation output with errors/warnings
- `ValidationLevel` - Error severity (error, warning, info)

**Validation Checks:**
1. **Name Validation** - RFC 1123 DNS label compliance
2. **Target Validation** - Supported fault injection types
3. **Duration Validation** - Min/max bounds with warnings
4. **Mode Validation** - Pod selection modes (one, all, fixed, etc.)
5. **Namespace Validation** - Kubernetes namespace format
6. **Label Validation** - Kubernetes label syntax
7. **Probe Validation** - Health check configuration

### Usage Example

```go
import (
    "context"
    "time"
    "github.com/vjranagit/chaos-mesh/pkg/validator"
)

// Create validator with custom options
v := validator.NewValidator(
    validator.WithMaxDuration(30 * time.Minute),
    validator.WithDryRun(true),
)

// Define experiment specification
spec := validator.ExperimentSpec{
    Name:      "pod-failure-test",
    Target:    "kubernetes.pod",
    Namespace: "production",
    Labels: map[string]string{
        "app": "api-server",
    },
    Duration: 60 * time.Second,
    Mode:     "one",
    Probes: []validator.ProbeSpec{
        {
            Name: "health-check",
            Type: "http",
            URL:  "http://api-service/health",
        },
    },
}

// Validate experiment
result, err := v.Validate(context.Background(), spec)
if err != nil {
    log.Fatal(err)
}

if !result.Valid {
    for _, validationErr := range result.Errors {
        log.Printf("[%s] %s: %s", validationErr.Level, validationErr.Field, validationErr.Message)
    }
    return
}

log.Printf("Validation passed: %d checks in %v", result.Checked, result.Elapsed)
```

### Dry-Run Mode

```go
// Enable dry-run mode
v := validator.NewValidator(validator.WithDryRun(true))

// Simulate experiment without execution
result, err := v.DryRun(context.Background(), spec)
if err != nil {
    log.Fatal(err)
}

if result.Valid {
    log.Println("✓ Experiment configuration is valid (not executed)")
}
```

### CLI Integration (Future)

```bash
# Validate experiment configuration
chaos validate -f experiment.hcl

# Dry-run mode (validate without execution)
chaos create -f experiment.hcl --dry-run

# Output:
# ✓ name: valid (pod-failure-test)
# ✓ target: valid (kubernetes.pod)
# ✓ duration: valid (60s)
# ⚠ duration: less than 5s may not be effective
# ✓ namespace: valid (production)
# ✓ labels: valid (1 selector)
# ✓ probes: valid (1 probe)
#
# Validation: PASS (7 checks in 2.3ms)
# Dry-run: experiment not executed
```

### Test Coverage

**Location:** `pkg/validator/validator_test.go`

- ✓ Name validation (valid, empty, uppercase, too long)
- ✓ Target validation (supported types, invalid)
- ✓ Duration validation (valid, too short, too long, negative)
- ✓ Mode validation (all modes, invalid)
- ✓ Full validation integration
- ✓ Dry-run mode

---

## Feature 2: Observability Layer (Metrics)

### Overview
Comprehensive metrics collection system using in-memory storage with Prometheus-compatible interface, enabling real-time monitoring of chaos experiments.

### Problem Solved
Without observability, operators cannot:
- Monitor experiment success/failure rates
- Track experiment duration and performance
- Identify failing probes
- Detect event bus bottlenecks
- Measure validation errors

This was identified as a key community concern in chaos engineering tools.

### Implementation Details

**Location:** `pkg/metrics/metrics.go`

**Key Components:**
- `Collector` - Interface for metric collection
- `InMemoryCollector` - Production-ready in-memory implementation
- `ExperimentMetrics` - Domain-specific metrics wrapper
- `MetricType` - Counter, Gauge, Histogram

**Metric Categories:**

1. **Experiment Metrics**
   - `chaos_experiments_created_total` - Experiments created
   - `chaos_experiments_state_total` - State distribution
   - `chaos_experiment_duration_seconds` - Execution time

2. **Fault Injection Metrics**
   - `chaos_faults_injected_total` - Successful injections
   - `chaos_faults_recovered_total` - Successful recoveries

3. **Probe Metrics**
   - `chaos_probes_executed_total` - Probe executions
   - `chaos_probe_duration_seconds` - Probe latency

4. **Event Bus Metrics**
   - `chaos_events_published_total` - Events published
   - `chaos_events_consumed_total` - Events consumed
   - `chaos_event_processing_seconds` - Processing time

5. **Validation Metrics**
   - `chaos_validation_errors_total` - Validation failures
   - `chaos_validations_success_total` - Successful validations

6. **State Machine Metrics**
   - `chaos_state_transitions_total` - State transitions

7. **Gauges**
   - `chaos_active_experiments` - Current active experiments

### Usage Example

```go
import (
    "time"
    "github.com/vjranagit/chaos-mesh/pkg/metrics"
)

// Create collector
collector := metrics.NewInMemoryCollector()
expMetrics := metrics.NewExperimentMetrics(collector)

// Record experiment creation
expMetrics.RecordExperimentCreated("pod-failure", "kubernetes.pod")

// Record state transitions
expMetrics.RecordExperimentState("exp-123", "idle")
expMetrics.RecordExperimentState("exp-123", "injecting")

// Record duration
duration := 30 * time.Second
expMetrics.RecordExperimentDuration("exp-123", "completed", duration)

// Record fault injection
expMetrics.RecordFaultInjected("exp-123", "pod-kill")
expMetrics.RecordFaultRecovered("exp-123", "pod-kill")

// Record probe execution
expMetrics.RecordProbeExecution("health-check", "success", 150*time.Millisecond)

// Set active experiments gauge
expMetrics.SetActiveExperiments(5)

// Get metric value
val, exists := collector.Get("chaos_experiments_created_total", metrics.MetricLabels{
    "name":   "pod-failure",
    "target": "kubernetes.pod",
})
if exists {
    log.Printf("Experiments created: %.0f", val)
}

// Snapshot all metrics
snapshot := collector.Snapshot()
for key, metric := range snapshot {
    log.Printf("%s{%v} = %v (type=%s)", key, metric.Labels, metric.Value, metric.Type)
}
```

### Timer Helper

```go
// Time experiment execution
err := expMetrics.ObserveExperimentDuration(ctx, "exp-123", "injecting", func() error {
    // Perform fault injection
    return injectFault()
})
```

### Prometheus Export (Future)

```go
// Export metrics in Prometheus format
GET /metrics

# HELP chaos_experiments_created_total Total chaos experiments created
# TYPE chaos_experiments_created_total counter
chaos_experiments_created_total{name="pod-failure",target="kubernetes.pod"} 42

# HELP chaos_experiment_duration_seconds Experiment execution duration
# TYPE chaos_experiment_duration_seconds histogram
chaos_experiment_duration_seconds{experiment_id="exp-123",state="completed"} 30.5

# HELP chaos_active_experiments Current active experiments
# TYPE chaos_active_experiments gauge
chaos_active_experiments 5
```

### Integration with State Machine

```go
// In state machine transitions
func (sm *StateMachine) Transition(ctx context.Context, from State, event Event) (State, error) {
    to, err := sm.doTransition(from, event)
    
    // Record metric
    if sm.metrics != nil {
        sm.metrics.RecordStateTransition(string(from), string(to), string(event))
    }
    
    return to, err
}
```

### Test Coverage

**Location:** `pkg/metrics/metrics_test.go`

- ✓ Counter increment/add
- ✓ Gauge set
- ✓ Histogram observe
- ✓ Duration recording
- ✓ Snapshot functionality
- ✓ Reset functionality
- ✓ Experiment-specific metrics
- ✓ Timer utilities

---

## Feature 3: Retry & Circuit Breaker

### Overview
Resilience layer implementing retry logic with exponential backoff and circuit breaker pattern, making the system production-ready and fault-tolerant.

### Problem Solved
Distributed systems need resilience:
- Event bus publish failures should retry
- Database connection issues should backoff
- Cascading failures should be prevented
- Transient errors should be handled gracefully

### Implementation Details

**Location:** `pkg/retry/retry.go`

**Key Components:**

1. **Retry Strategies**
   - `ExponentialBackoff` - Exponential delay with max cap
   - `ConstantDelay` - Fixed delay between retries

2. **Retry Logic**
   - `RetryConfig` - Configurable retry behavior
   - `Do()` - Execute function with retry
   - Context-aware cancellation

3. **Circuit Breaker**
   - `CircuitBreaker` - Three-state circuit breaker
   - States: Closed → Open → Half-Open → Closed
   - Automatic recovery after timeout

4. **Resilient Executor**
   - `ResilientExecutor` - Combines retry + circuit breaker
   - Production-ready error handling

### Retry Strategy Examples

```go
import (
    "context"
    "time"
    "github.com/vjranagit/chaos-mesh/pkg/retry"
)

// Exponential backoff: 100ms, 200ms, 400ms, 800ms, 1s (max)
backoff := retry.NewExponentialBackoff(100*time.Millisecond, 1*time.Second)

// Constant delay: 500ms between each retry
constant := retry.NewConstantDelay(500 * time.Millisecond)

// Custom retry configuration
config := &retry.RetryConfig{
    MaxAttempts: 5,
    Strategy:    backoff,
    RetryIf: func(err error) bool {
        // Only retry on transient errors
        return isTransientError(err)
    },
    OnRetry: func(attempt int, err error) {
        log.Printf("Retry attempt %d: %v", attempt, err)
    },
}

// Execute with retry
ctx := context.Background()
err := retry.Do(ctx, config, func() error {
    return publishToEventBus(event)
})
```

### Circuit Breaker Usage

```go
// Create circuit breaker
cb := retry.NewCircuitBreaker(retry.CircuitBreakerConfig{
    MaxFailures:  5,                  // Open after 5 failures
    ResetTimeout: 30 * time.Second,   // Try half-open after 30s
    HalfOpenMax:  1,                  // Allow 1 call in half-open
    OnStateChange: func(from, to retry.CircuitState) {
        log.Printf("Circuit: %s → %s", from, to)
    },
})

// Execute with circuit breaker
err := cb.Execute(func() error {
    return callExternalService()
})

if errors.Is(err, retry.ErrCircuitOpen) {
    log.Println("Circuit breaker is open, service unavailable")
}

// Check circuit state
switch cb.State() {
case retry.StateClosed:
    log.Println("Circuit healthy")
case retry.StateOpen:
    log.Println("Circuit open, rejecting calls")
case retry.StateHalfOpen:
    log.Println("Circuit testing recovery")
}

// Manual reset (for admin operations)
cb.Reset()
```

### Combined Resilient Executor

```go
// Best of both worlds: retry + circuit breaker
executor := retry.NewResilientExecutor(
    &retry.RetryConfig{
        MaxAttempts: 3,
        Strategy:    retry.NewExponentialBackoff(100*time.Millisecond, 5*time.Second),
        RetryIf:     func(err error) bool { return err != nil },
        OnRetry:     func(attempt int, err error) {
            log.Printf("Retrying (%d): %v", attempt, err)
        },
    },
    retry.CircuitBreakerConfig{
        MaxFailures:  5,
        ResetTimeout: 30 * time.Second,
    },
)

// Execute with full resilience
err := executor.Execute(ctx, func() error {
    return performCriticalOperation()
})

log.Printf("Circuit state: %s", executor.CircuitState())
```

### Integration with Event Bus

```go
// Make event bus resilient
type ResilientEventBus struct {
    bus      EventBus
    executor *retry.ResilientExecutor
}

func (r *ResilientEventBus) Publish(ctx context.Context, topic string, event Event) error {
    return r.executor.Execute(ctx, func() error {
        return r.bus.Publish(ctx, topic, event)
    })
}
```

### State Machine Integration

```go
// Retry state transitions
config := &retry.RetryConfig{
    MaxAttempts: 3,
    Strategy:    retry.NewExponentialBackoff(100*time.Millisecond, 1*time.Second),
}

err := retry.Do(ctx, config, func() error {
    newState, err := sm.Transition(ctx, currentState, event)
    if err != nil {
        return err
    }
    return persistState(newState)
})
```

### Circuit Breaker States

```
[Closed] ─────────────────────────────────────────┐
   │                                                │
   │ failures >= maxFailures                       │ success
   ▼                                                │
[Open] ──────────────────────────────────────────────┘
   │                                                
   │ time > resetTimeout
   ▼
[Half-Open] ────────────────┐
   │                         │
   │ success                 │ failure
   ▼                         ▼
[Closed]                   [Open]
```

### Test Coverage

**Location:** `pkg/retry/retry_test.go`

- ✓ Exponential backoff delays
- ✓ Constant delay behavior
- ✓ Successful retry after failures
- ✓ Max retries exceeded
- ✓ Non-retryable errors
- ✓ Context cancellation
- ✓ Circuit breaker: closed state
- ✓ Circuit breaker: open state
- ✓ Circuit breaker: half-open recovery
- ✓ Circuit breaker: half-open failure
- ✓ Manual circuit reset
- ✓ Resilient executor (retry + circuit breaker)

---

## Production Readiness Improvements

### Before These Features
- ❌ No validation → Runtime failures
- ❌ No metrics → Blind operations
- ❌ No retry → Brittle event bus
- ❌ No circuit breaker → Cascading failures

### After These Features
- ✅ Pre-flight validation → Early error detection
- ✅ Dry-run mode → Safe testing
- ✅ Comprehensive metrics → Full observability
- ✅ Retry logic → Transient error handling
- ✅ Circuit breaker → Fault isolation

---

## Future Enhancements

### Validator
- [ ] Kubernetes cluster connectivity checks
- [ ] Target resource existence validation
- [ ] RBAC permission verification
- [ ] Quota and resource limit checks

### Metrics
- [ ] Prometheus exporter endpoint
- [ ] Grafana dashboard templates
- [ ] OpenTelemetry tracing integration
- [ ] StatsD/DogStatsD support

### Retry/Circuit Breaker
- [ ] Adaptive retry delays (jitter)
- [ ] Per-operation circuit breakers
- [ ] Health check integration
- [ ] Bulkhead pattern support

---

## Summary

These three features significantly improve the production-readiness of the chaos engineering platform:

1. **Validator** - Prevents misconfigurations and enables safe dry-run testing
2. **Metrics** - Provides comprehensive observability for monitoring and debugging
3. **Retry/Circuit Breaker** - Adds resilience for production deployments

All features include comprehensive tests and are designed for easy integration into the existing codebase.
