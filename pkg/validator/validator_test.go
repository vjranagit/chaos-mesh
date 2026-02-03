package validator

import (
	"context"
	"testing"
	"time"
)

func TestValidateName(t *testing.T) {
	v := NewValidator()
	
	tests := []struct {
		name     string
		input    string
		wantErr  bool
		errLevel ValidationLevel
	}{
		{"valid name", "pod-failure", false, ""},
		{"empty name", "", true, ValidationLevelError},
		{"uppercase", "Pod-Failure", true, ValidationLevelError},
		{"too long", "this-is-a-very-long-experiment-name-that-exceeds-the-kubernetes-limit-of-sixty-three-characters", true, ValidationLevelError},
		{"valid short", "test", false, ""},
	}
	
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := v.validateName(tt.input)
			if (err != nil) != tt.wantErr {
				t.Errorf("validateName() error = %v, wantErr %v", err, tt.wantErr)
			}
			if err != nil && err.Level != tt.errLevel {
				t.Errorf("validateName() level = %v, want %v", err.Level, tt.errLevel)
			}
		})
	}
}

func TestValidateTarget(t *testing.T) {
	v := NewValidator()
	
	tests := []struct {
		name    string
		target  string
		wantErr bool
	}{
		{"valid pod", "kubernetes.pod", false},
		{"valid network", "kubernetes.network", false},
		{"invalid target", "docker.container", true},
		{"empty target", "", true},
	}
	
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := v.validateTarget(tt.target)
			if (err != nil) != tt.wantErr {
				t.Errorf("validateTarget() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestValidateDuration(t *testing.T) {
	v := NewValidator(WithMaxDuration(10 * time.Minute))
	
	tests := []struct {
		name     string
		duration time.Duration
		wantErr  bool
		errLevel ValidationLevel
	}{
		{"valid duration", 30 * time.Second, false, ""},
		{"too short", 2 * time.Second, true, ValidationLevelWarning},
		{"too long", 15 * time.Minute, true, ValidationLevelError},
		{"negative", -5 * time.Second, true, ValidationLevelError},
	}
	
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := v.validateDuration(tt.duration)
			if (err != nil) != tt.wantErr {
				t.Errorf("validateDuration() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestValidateMode(t *testing.T) {
	v := NewValidator()
	
	tests := []struct {
		name    string
		mode    string
		wantErr bool
	}{
		{"valid one", "one", false},
		{"valid all", "all", false},
		{"valid fixed", "fixed", false},
		{"invalid mode", "some", true},
	}
	
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := v.validateMode(tt.mode)
			if (err != nil) != tt.wantErr {
				t.Errorf("validateMode() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestValidate(t *testing.T) {
	v := NewValidator()
	ctx := context.Background()
	
	validSpec := ExperimentSpec{
		Name:      "test-experiment",
		Target:    "kubernetes.pod",
		Namespace: "default",
		Labels: map[string]string{
			"app": "nginx",
		},
		Duration: 30 * time.Second,
		Mode:     "one",
		Probes: []ProbeSpec{
			{
				Name: "health",
				Type: "http",
				URL:  "http://service/health",
			},
		},
	}
	
	result, err := v.Validate(ctx, validSpec)
	if err != nil {
		t.Fatalf("Validate() unexpected error: %v", err)
	}
	
	if !result.Valid {
		t.Errorf("Validate() expected valid, got invalid with errors: %v", result.Errors)
	}
	
	if result.Checked == 0 {
		t.Error("Validate() should have checked at least one field")
	}
}

func TestDryRun(t *testing.T) {
	v := NewValidator(WithDryRun(true))
	ctx := context.Background()
	
	spec := ExperimentSpec{
		Name:      "test",
		Target:    "kubernetes.pod",
		Namespace: "default",
		Labels:    map[string]string{"app": "test"},
		Duration:  30 * time.Second,
		Mode:      "one",
	}
	
	result, err := v.DryRun(ctx, spec)
	if err != nil {
		t.Fatalf("DryRun() unexpected error: %v", err)
	}
	
	// Should have dry-run info message
	foundDryRunInfo := false
	for _, e := range result.Errors {
		if e.Field == "dry-run" && e.Level == ValidationLevelInfo {
			foundDryRunInfo = true
			break
		}
	}
	
	if !foundDryRunInfo {
		t.Error("DryRun() should add dry-run info message")
	}
}
