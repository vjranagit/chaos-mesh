package metrics

import (
	"testing"
	"time"
)

func TestInMemoryCollector_Inc(t *testing.T) {
	collector := NewInMemoryCollector()
	
	labels := MetricLabels{"experiment": "test"}
	
	collector.Inc("experiments_total", labels)
	collector.Inc("experiments_total", labels)
	
	val, exists := collector.Get("experiments_total", labels)
	if !exists {
		t.Fatal("metric should exist")
	}
	
	if val != 2 {
		t.Errorf("expected 2, got %v", val)
	}
}

func TestInMemoryCollector_Add(t *testing.T) {
	collector := NewInMemoryCollector()
	
	labels := MetricLabels{"type": "counter"}
	
	collector.Add("test_counter", 5.5, labels)
	collector.Add("test_counter", 3.5, labels)
	
	val, exists := collector.Get("test_counter", labels)
	if !exists {
		t.Fatal("metric should exist")
	}
	
	if val != 9.0 {
		t.Errorf("expected 9.0, got %v", val)
	}
}

func TestInMemoryCollector_Set(t *testing.T) {
	collector := NewInMemoryCollector()
	
	labels := MetricLabels{"type": "gauge"}
	
	collector.Set("active_experiments", 10, labels)
	collector.Set("active_experiments", 5, labels)
	
	val, exists := collector.Get("active_experiments", labels)
	if !exists {
		t.Fatal("metric should exist")
	}
	
	if val != 5 {
		t.Errorf("expected 5, got %v", val)
	}
}

func TestInMemoryCollector_Observe(t *testing.T) {
	collector := NewInMemoryCollector()
	
	labels := MetricLabels{"operation": "test"}
	
	collector.Observe("duration_seconds", 1.5, labels)
	collector.Observe("duration_seconds", 2.5, labels)
	collector.Observe("duration_seconds", 2.0, labels)
	
	val, exists := collector.Get("duration_seconds", labels)
	if !exists {
		t.Fatal("metric should exist")
	}
	
	expected := (1.5 + 2.5 + 2.0) / 3
	if val != expected {
		t.Errorf("expected %v, got %v", expected, val)
	}
}

func TestInMemoryCollector_RecordDuration(t *testing.T) {
	collector := NewInMemoryCollector()
	
	labels := MetricLabels{"function": "test"}
	
	duration := 100 * time.Millisecond
	collector.RecordDuration("processing_time", duration, labels)
	
	val, exists := collector.Get("processing_time", labels)
	if !exists {
		t.Fatal("metric should exist")
	}
	
	expected := duration.Seconds()
	if val != expected {
		t.Errorf("expected %v, got %v", expected, val)
	}
}

func TestInMemoryCollector_Snapshot(t *testing.T) {
	collector := NewInMemoryCollector()
	
	collector.Inc("metric1", MetricLabels{"label": "a"})
	collector.Set("metric2", 42, MetricLabels{"label": "b"})
	
	snapshot := collector.Snapshot()
	
	if len(snapshot) != 2 {
		t.Errorf("expected 2 metrics, got %d", len(snapshot))
	}
}

func TestInMemoryCollector_Reset(t *testing.T) {
	collector := NewInMemoryCollector()
	
	collector.Inc("test", MetricLabels{})
	collector.Reset()
	
	_, exists := collector.Get("test", MetricLabels{})
	if exists {
		t.Error("metric should not exist after reset")
	}
}

func TestExperimentMetrics(t *testing.T) {
	collector := NewInMemoryCollector()
	metrics := NewExperimentMetrics(collector)
	
	metrics.RecordExperimentCreated("test-exp", "kubernetes.pod")
	
	val, exists := collector.Get("chaos_experiments_created_total", MetricLabels{
		"name":   "test-exp",
		"target": "kubernetes.pod",
	})
	
	if !exists {
		t.Fatal("metric should exist")
	}
	
	if val != 1 {
		t.Errorf("expected 1, got %v", val)
	}
}

func TestExperimentMetrics_State(t *testing.T) {
	collector := NewInMemoryCollector()
	metrics := NewExperimentMetrics(collector)
	
	metrics.RecordExperimentState("exp-123", "idle")
	metrics.RecordExperimentState("exp-123", "idle")
	
	val, _ := collector.Get("chaos_experiments_state_total", MetricLabels{
		"experiment_id": "exp-123",
		"state":         "idle",
	})
	
	if val != 2 {
		t.Errorf("expected 2, got %v", val)
	}
}

func TestExperimentMetrics_Duration(t *testing.T) {
	collector := NewInMemoryCollector()
	metrics := NewExperimentMetrics(collector)
	
	duration := 5 * time.Second
	metrics.RecordExperimentDuration("exp-123", "completed", duration)
	
	val, exists := collector.Get("chaos_experiment_duration_seconds", MetricLabels{
		"experiment_id": "exp-123",
		"state":         "completed",
	})
	
	if !exists {
		t.Fatal("metric should exist")
	}
	
	if val != duration.Seconds() {
		t.Errorf("expected %v, got %v", duration.Seconds(), val)
	}
}

func TestExperimentMetrics_Faults(t *testing.T) {
	collector := NewInMemoryCollector()
	metrics := NewExperimentMetrics(collector)
	
	metrics.RecordFaultInjected("exp-123", "pod-kill")
	metrics.RecordFaultRecovered("exp-123", "pod-kill")
	
	injected, _ := collector.Get("chaos_faults_injected_total", MetricLabels{
		"experiment_id": "exp-123",
		"type":          "pod-kill",
	})
	
	recovered, _ := collector.Get("chaos_faults_recovered_total", MetricLabels{
		"experiment_id": "exp-123",
		"type":          "pod-kill",
	})
	
	if injected != 1 || recovered != 1 {
		t.Errorf("expected injected=1 and recovered=1, got injected=%v recovered=%v", injected, recovered)
	}
}

func TestTimer(t *testing.T) {
	timer := NewTimer()
	
	time.Sleep(10 * time.Millisecond)
	
	elapsed := timer.Elapsed()
	if elapsed < 10*time.Millisecond {
		t.Errorf("expected at least 10ms, got %v", elapsed)
	}
}
