package validator

import (
	"context"
	"fmt"
	"regexp"
	"time"
)

// ValidationError represents a validation failure
type ValidationError struct {
	Field   string
	Message string
	Level   ValidationLevel
}

func (e ValidationError) Error() string {
	return fmt.Sprintf("%s: %s (level: %s)", e.Field, e.Message, e.Level)
}

// ValidationLevel indicates severity
type ValidationLevel string

const (
	ValidationLevelError   ValidationLevel = "error"
	ValidationLevelWarning ValidationLevel = "warning"
	ValidationLevelInfo    ValidationLevel = "info"
)

// ValidationResult contains validation output
type ValidationResult struct {
	Valid   bool
	Errors  []ValidationError
	Checked int
	Elapsed time.Duration
}

// ExperimentSpec defines the experiment to validate
type ExperimentSpec struct {
	Name      string
	Target    string
	Namespace string
	Labels    map[string]string
	Duration  time.Duration
	Mode      string
	Probes    []ProbeSpec
}

// ProbeSpec defines health checks
type ProbeSpec struct {
	Name string
	Type string
	URL  string
}

// Validator checks experiment validity before execution
type Validator struct {
	dryRun          bool
	skipProbeChecks bool
	maxDuration     time.Duration
}

// NewValidator creates a new experiment validator
func NewValidator(opts ...ValidatorOption) *Validator {
	v := &Validator{
		dryRun:      false,
		maxDuration: 1 * time.Hour,
	}

	for _, opt := range opts {
		opt(v)
	}

	return v
}

// ValidatorOption configures the validator
type ValidatorOption func(*Validator)

// WithDryRun enables dry-run mode (no actual execution)
func WithDryRun(enabled bool) ValidatorOption {
	return func(v *Validator) {
		v.dryRun = enabled
	}
}

// WithMaxDuration sets maximum allowed experiment duration
func WithMaxDuration(d time.Duration) ValidatorOption {
	return func(v *Validator) {
		v.maxDuration = d
	}
}

// WithSkipProbeChecks skips probe validation
func WithSkipProbeChecks(skip bool) ValidatorOption {
	return func(v *Validator) {
		v.skipProbeChecks = skip
	}
}

// Validate performs comprehensive validation
func (v *Validator) Validate(ctx context.Context, spec ExperimentSpec) (*ValidationResult, error) {
	start := time.Now()
	result := &ValidationResult{
		Valid:   true,
		Errors:  make([]ValidationError, 0),
		Checked: 0,
	}

	// Name validation
	if err := v.validateName(spec.Name); err != nil {
		result.Errors = append(result.Errors, *err)
		result.Valid = false
	}
	result.Checked++

	// Target validation
	if err := v.validateTarget(spec.Target); err != nil {
		result.Errors = append(result.Errors, *err)
		result.Valid = false
	}
	result.Checked++

	// Duration validation
	if err := v.validateDuration(spec.Duration); err != nil {
		result.Errors = append(result.Errors, *err)
		if err.Level == ValidationLevelError {
			result.Valid = false
		}
	}
	result.Checked++

	// Mode validation
	if err := v.validateMode(spec.Mode); err != nil {
		result.Errors = append(result.Errors, *err)
		result.Valid = false
	}
	result.Checked++

	// Namespace validation
	if err := v.validateNamespace(spec.Namespace); err != nil {
		result.Errors = append(result.Errors, *err)
		result.Valid = false
	}
	result.Checked++

	// Labels validation
	if err := v.validateLabels(spec.Labels); err != nil {
		result.Errors = append(result.Errors, *err)
	}
	result.Checked++

	// Probe validation
	if !v.skipProbeChecks {
		for _, probe := range spec.Probes {
			if err := v.validateProbe(probe); err != nil {
				result.Errors = append(result.Errors, *err)
			}
			result.Checked++
		}
	}

	result.Elapsed = time.Since(start)
	return result, nil
}

// validateName checks experiment name format
func (v *Validator) validateName(name string) *ValidationError {
	if name == "" {
		return &ValidationError{
			Field:   "name",
			Message: "experiment name cannot be empty",
			Level:   ValidationLevelError,
		}
	}

	// RFC 1123 DNS label (K8s convention)
	pattern := `^[a-z0-9]([-a-z0-9]*[a-z0-9])?$`
	matched, _ := regexp.MatchString(pattern, name)
	if !matched {
		return &ValidationError{
			Field:   "name",
			Message: "name must be lowercase alphanumeric with hyphens",
			Level:   ValidationLevelError,
		}
	}

	if len(name) > 63 {
		return &ValidationError{
			Field:   "name",
			Message: "name must be 63 characters or less",
			Level:   ValidationLevelError,
		}
	}

	return nil
}

// validateTarget checks if target type is supported
func (v *Validator) validateTarget(target string) *ValidationError {
	supportedTargets := map[string]bool{
		"kubernetes.pod":     true,
		"kubernetes.network": true,
		"kubernetes.stress":  true,
		"kubernetes.io":      true,
		"kubernetes.time":    true,
	}

	if !supportedTargets[target] {
		return &ValidationError{
			Field:   "target",
			Message: fmt.Sprintf("unsupported target type: %s", target),
			Level:   ValidationLevelError,
		}
	}

	return nil
}

// validateDuration checks experiment duration
func (v *Validator) validateDuration(duration time.Duration) *ValidationError {
	if duration <= 0 {
		return &ValidationError{
			Field:   "duration",
			Message: "duration must be positive",
			Level:   ValidationLevelError,
		}
	}

	if duration > v.maxDuration {
		return &ValidationError{
			Field:   "duration",
			Message: fmt.Sprintf("duration exceeds maximum allowed: %s > %s", duration, v.maxDuration),
			Level:   ValidationLevelError,
		}
	}

	if duration < 5*time.Second {
		return &ValidationError{
			Field:   "duration",
			Message: "duration less than 5s may not be effective",
			Level:   ValidationLevelWarning,
		}
	}

	return nil
}

// validateMode checks selection mode
func (v *Validator) validateMode(mode string) *ValidationError {
	validModes := map[string]bool{
		"one":        true,
		"all":        true,
		"fixed":      true,
		"percentage": true,
		"random":     true,
	}

	if !validModes[mode] {
		return &ValidationError{
			Field:   "mode",
			Message: fmt.Sprintf("invalid mode: %s (must be one|all|fixed|percentage|random)", mode),
			Level:   ValidationLevelError,
		}
	}

	return nil
}

// validateNamespace checks namespace format
func (v *Validator) validateNamespace(namespace string) *ValidationError {
	if namespace == "" {
		return &ValidationError{
			Field:   "namespace",
			Message: "namespace is required for Kubernetes targets",
			Level:   ValidationLevelError,
		}
	}

	// K8s namespace validation
	pattern := `^[a-z0-9]([-a-z0-9]*[a-z0-9])?$`
	matched, _ := regexp.MatchString(pattern, namespace)
	if !matched {
		return &ValidationError{
			Field:   "namespace",
			Message: "invalid Kubernetes namespace format",
			Level:   ValidationLevelError,
		}
	}

	return nil
}

// validateLabels checks label format
func (v *Validator) validateLabels(labels map[string]string) *ValidationError {
	if len(labels) == 0 {
		return &ValidationError{
			Field:   "labels",
			Message: "at least one label selector is recommended",
			Level:   ValidationLevelWarning,
		}
	}

	// K8s label validation
	keyPattern := `^[a-z0-9]([-a-z0-9./]*[a-z0-9])?$`
	valuePattern := `^[a-z0-9]([-a-z0-9._]*[a-z0-9])?$`

	for key, value := range labels {
		keyMatched, _ := regexp.MatchString(keyPattern, key)
		if !keyMatched {
			return &ValidationError{
				Field:   "labels",
				Message: fmt.Sprintf("invalid label key format: %s", key),
				Level:   ValidationLevelError,
			}
		}

		valueMatched, _ := regexp.MatchString(valuePattern, value)
		if !valueMatched {
			return &ValidationError{
				Field:   "labels",
				Message: fmt.Sprintf("invalid label value format: %s=%s", key, value),
				Level:   ValidationLevelError,
			}
		}
	}

	return nil
}

// validateProbe checks probe configuration
func (v *Validator) validateProbe(probe ProbeSpec) *ValidationError {
	if probe.Name == "" {
		return &ValidationError{
			Field:   "probe.name",
			Message: "probe name cannot be empty",
			Level:   ValidationLevelError,
		}
	}

	validProbeTypes := map[string]bool{
		"http":       true,
		"tcp":        true,
		"prometheus": true,
		"command":    true,
	}

	if !validProbeTypes[probe.Type] {
		return &ValidationError{
			Field:   "probe.type",
			Message: fmt.Sprintf("invalid probe type: %s", probe.Type),
			Level:   ValidationLevelError,
		}
	}

	if probe.Type == "http" && probe.URL == "" {
		return &ValidationError{
			Field:   "probe.url",
			Message: "HTTP probe requires URL",
			Level:   ValidationLevelError,
		}
	}

	return nil
}

// DryRun simulates experiment execution without actually running it
func (v *Validator) DryRun(ctx context.Context, spec ExperimentSpec) (*ValidationResult, error) {
	if !v.dryRun {
		return nil, fmt.Errorf("dry-run mode not enabled")
	}

	result, err := v.Validate(ctx, spec)
	if err != nil {
		return nil, err
	}

	// Add dry-run specific info
	result.Errors = append(result.Errors, ValidationError{
		Field:   "dry-run",
		Message: "experiment validated successfully (not executed)",
		Level:   ValidationLevelInfo,
	})

	return result, nil
}
