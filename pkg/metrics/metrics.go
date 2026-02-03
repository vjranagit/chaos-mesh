package metrics

import (
	"context"
	"sync"
	"time"
)

// MetricType defines the kind of metric
type MetricType string

const (
	MetricTypeCounter   MetricType = "counter"
	MetricTypeGauge     MetricType = "gauge"
	MetricTypeHistogram MetricType = "histogram"
)

// MetricLabels are key-value pairs for metric dimensions
type MetricLabels map[string]string

// Collector collects and exposes metrics
type Collector interface {
	// Inc increments a counter metric
	Inc(name string, labels MetricLabels)

	// Add adds a value to a counter
	Add(name string, value float64, labels MetricLabels)

	// Set sets a gauge value
	Set(name string, value float64, labels MetricLabels)

	// Observe records a histogram observation
	Observe(name string, value float64, labels MetricLabels)

	// RecordDuration records timing for histogram
	RecordDuration(name string, duration time.Duration, labels MetricLabels)

	// Get retrieves current metric value
	Get(name string, labels MetricLabels) (float64, bool)

	// Snapshot returns all current metrics
	Snapshot() map[string]MetricValue

	// Reset clears all metrics
	Reset()
}

// MetricValue holds metric data
type MetricValue struct {
	Type   MetricType
	Value  float64
	Labels MetricLabels
	Count  int64 // For histograms
	Sum    float64
	Last   time.Time
}

// InMemoryCollector is a simple in-memory metrics collector
type InMemoryCollector struct {
	metrics map[string]*MetricValue
	mu      sync.RWMutex
}

// NewInMemoryCollector creates a new in-memory collector
func NewInMemoryCollector() *InMemoryCollector {
	return &InMemoryCollector{
		metrics: make(map[string]*MetricValue),
	}
}

// metricKey generates a unique key for metric + labels
func metricKey(name string, labels MetricLabels) string {
	key := name
	for k, v := range labels {
		key += ":" + k + "=" + v
	}
	return key
}

// Inc increments a counter
func (c *InMemoryCollector) Inc(name string, labels MetricLabels) {
	c.Add(name, 1, labels)
}

// Add adds to a counter
func (c *InMemoryCollector) Add(name string, value float64, labels MetricLabels) {
	c.mu.Lock()
	defer c.mu.Unlock()

	key := metricKey(name, labels)
	if metric, exists := c.metrics[key]; exists {
		metric.Value += value
		metric.Last = time.Now()
	} else {
		c.metrics[key] = &MetricValue{
			Type:   MetricTypeCounter,
			Value:  value,
			Labels: labels,
			Last:   time.Now(),
		}
	}
}

// Set sets a gauge value
func (c *InMemoryCollector) Set(name string, value float64, labels MetricLabels) {
	c.mu.Lock()
	defer c.mu.Unlock()

	key := metricKey(name, labels)
	c.metrics[key] = &MetricValue{
		Type:   MetricTypeGauge,
		Value:  value,
		Labels: labels,
		Last:   time.Now(),
	}
}

// Observe records a histogram observation
func (c *InMemoryCollector) Observe(name string, value float64, labels MetricLabels) {
	c.mu.Lock()
	defer c.mu.Unlock()

	key := metricKey(name, labels)
	if metric, exists := c.metrics[key]; exists {
		metric.Count++
		metric.Sum += value
		metric.Value = metric.Sum / float64(metric.Count)
		metric.Last = time.Now()
	} else {
		c.metrics[key] = &MetricValue{
			Type:   MetricTypeHistogram,
			Value:  value,
			Labels: labels,
			Count:  1,
			Sum:    value,
			Last:   time.Now(),
		}
	}
}

// RecordDuration records timing
func (c *InMemoryCollector) RecordDuration(name string, duration time.Duration, labels MetricLabels) {
	c.Observe(name, duration.Seconds(), labels)
}

// Get retrieves a metric value
func (c *InMemoryCollector) Get(name string, labels MetricLabels) (float64, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	key := metricKey(name, labels)
	if metric, exists := c.metrics[key]; exists {
		return metric.Value, true
	}
	return 0, false
}

// Snapshot returns all metrics
func (c *InMemoryCollector) Snapshot() map[string]MetricValue {
	c.mu.RLock()
	defer c.mu.RUnlock()

	snapshot := make(map[string]MetricValue, len(c.metrics))
	for key, value := range c.metrics {
		snapshot[key] = *value
	}
	return snapshot
}

// Reset clears all metrics
func (c *InMemoryCollector) Reset() {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.metrics = make(map[string]*MetricValue)
}

// ExperimentMetrics provides chaos experiment specific metrics
type ExperimentMetrics struct {
	collector Collector
}

// NewExperimentMetrics creates experiment metrics wrapper
func NewExperimentMetrics(collector Collector) *ExperimentMetrics {
	return &ExperimentMetrics{
		collector: collector,
	}
}

// RecordExperimentCreated records experiment creation
func (m *ExperimentMetrics) RecordExperimentCreated(experimentName, target string) {
	m.collector.Inc("chaos_experiments_created_total", MetricLabels{
		"name":   experimentName,
		"target": target,
	})
}

// RecordExperimentState records current experiment state
func (m *ExperimentMetrics) RecordExperimentState(experimentID, state string) {
	m.collector.Inc("chaos_experiments_state_total", MetricLabels{
		"experiment_id": experimentID,
		"state":         state,
	})
}

// RecordExperimentDuration records experiment execution time
func (m *ExperimentMetrics) RecordExperimentDuration(experimentID, state string, duration time.Duration) {
	m.collector.RecordDuration("chaos_experiment_duration_seconds", duration, MetricLabels{
		"experiment_id": experimentID,
		"state":         state,
	})
}

// RecordFaultInjected records successful fault injection
func (m *ExperimentMetrics) RecordFaultInjected(experimentID, faultType string) {
	m.collector.Inc("chaos_faults_injected_total", MetricLabels{
		"experiment_id": experimentID,
		"type":          faultType,
	})
}

// RecordFaultRecovered records successful recovery
func (m *ExperimentMetrics) RecordFaultRecovered(experimentID, faultType string) {
	m.collector.Inc("chaos_faults_recovered_total", MetricLabels{
		"experiment_id": experimentID,
		"type":          faultType,
	})
}

// RecordProbeExecution records probe check
func (m *ExperimentMetrics) RecordProbeExecution(probeName, status string, duration time.Duration) {
	m.collector.Inc("chaos_probes_executed_total", MetricLabels{
		"probe":  probeName,
		"status": status,
	})
	m.collector.RecordDuration("chaos_probe_duration_seconds", duration, MetricLabels{
		"probe": probeName,
	})
}

// RecordEventPublished records event bus publish
func (m *ExperimentMetrics) RecordEventPublished(topic, eventType string) {
	m.collector.Inc("chaos_events_published_total", MetricLabels{
		"topic": topic,
		"type":  eventType,
	})
}

// RecordEventConsumed records event consumption
func (m *ExperimentMetrics) RecordEventConsumed(topic, eventType string, duration time.Duration) {
	m.collector.Inc("chaos_events_consumed_total", MetricLabels{
		"topic": topic,
		"type":  eventType,
	})
	m.collector.RecordDuration("chaos_event_processing_seconds", duration, MetricLabels{
		"topic": topic,
		"type":  eventType,
	})
}

// RecordValidationError records validation failures
func (m *ExperimentMetrics) RecordValidationError(field, level string) {
	m.collector.Inc("chaos_validation_errors_total", MetricLabels{
		"field": field,
		"level": level,
	})
}

// RecordValidationSuccess records successful validations
func (m *ExperimentMetrics) RecordValidationSuccess(experimentName string) {
	m.collector.Inc("chaos_validations_success_total", MetricLabels{
		"experiment": experimentName,
	})
}

// SetActiveExperiments sets the current number of active experiments
func (m *ExperimentMetrics) SetActiveExperiments(count int) {
	m.collector.Set("chaos_active_experiments", float64(count), MetricLabels{})
}

// RecordStateTransition records state machine transitions
func (m *ExperimentMetrics) RecordStateTransition(from, to, event string) {
	m.collector.Inc("chaos_state_transitions_total", MetricLabels{
		"from":  from,
		"to":    to,
		"event": event,
	})
}

// Timer provides timing utilities for metrics
type Timer struct {
	start   time.Time
	metrics *ExperimentMetrics
}

// NewTimer creates a timer
func NewTimer() *Timer {
	return &Timer{
		start: time.Now(),
	}
}

// Elapsed returns duration since timer start
func (t *Timer) Elapsed() time.Duration {
	return time.Since(t.start)
}

// ObserveExperimentDuration is a helper to time experiments
func (m *ExperimentMetrics) ObserveExperimentDuration(ctx context.Context, experimentID, state string, fn func() error) error {
	start := time.Now()
	err := fn()
	duration := time.Since(start)
	
	m.RecordExperimentDuration(experimentID, state, duration)
	
	return err
}
