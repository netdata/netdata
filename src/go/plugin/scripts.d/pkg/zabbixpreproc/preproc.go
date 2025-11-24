// Package zabbixpreproc provides Zabbix preprocessing functionality in pure Go.
package zabbixpreproc

import (
	"fmt"
	"strings"
	"sync"
	"time"
)

// ValueType represents Zabbix item value types.
type ValueType int

const (
	ValueTypeStr    ValueType = 0 // ITEM_VALUE_TYPE_STR
	ValueTypeUint64 ValueType = 3 // ITEM_VALUE_TYPE_UINT64
	ValueTypeFloat  ValueType = 1 // ITEM_VALUE_TYPE_FLOAT
)

// Value represents a preprocessing input/output value.
type Value struct {
	Data      string    // Raw string data
	Type      ValueType // Value type
	Timestamp time.Time // Timestamp for stateful operations
	IsError   bool      // True if this value represents an error from a previous step
}

// StepType represents the type of preprocessing step.
// Type IDs match Zabbix preprocessing types exactly (ZBX_PREPROC_* from zbxcommon.h)
type StepType int

const (
	// Zabbix preprocessing types (1-30) - MUST match zbxcommon.h exactly
	StepTypeMultiplier           StepType = 1  // ZBX_PREPROC_MULTIPLIER
	StepTypeRTrim                StepType = 2  // ZBX_PREPROC_RTRIM
	StepTypeLTrim                StepType = 3  // ZBX_PREPROC_LTRIM
	StepTypeTrim                 StepType = 4  // ZBX_PREPROC_TRIM
	StepTypeRegexSubstitution    StepType = 5  // ZBX_PREPROC_REGSUB
	StepTypeBool2Dec             StepType = 6  // ZBX_PREPROC_BOOL2DEC
	StepTypeOct2Dec              StepType = 7  // ZBX_PREPROC_OCT2DEC
	StepTypeHex2Dec              StepType = 8  // ZBX_PREPROC_HEX2DEC
	StepTypeDeltaValue           StepType = 9  // ZBX_PREPROC_DELTA_VALUE
	StepTypeDeltaSpeed           StepType = 10 // ZBX_PREPROC_DELTA_SPEED
	StepTypeXPath                StepType = 11 // ZBX_PREPROC_XPATH
	StepTypeJSONPath             StepType = 12 // ZBX_PREPROC_JSONPATH
	StepTypeValidateRange        StepType = 13 // ZBX_PREPROC_VALIDATE_RANGE
	StepTypeValidateRegex        StepType = 14 // ZBX_PREPROC_VALIDATE_REGEX
	StepTypeValidateNotRegex     StepType = 15 // ZBX_PREPROC_VALIDATE_NOT_REGEX
	StepTypeErrorFieldJSON       StepType = 16 // ZBX_PREPROC_ERROR_FIELD_JSON
	StepTypeErrorFieldXML        StepType = 17 // ZBX_PREPROC_ERROR_FIELD_XML
	StepTypeErrorFieldRegex      StepType = 18 // ZBX_PREPROC_ERROR_FIELD_REGEX
	StepTypeThrottleValue        StepType = 19 // ZBX_PREPROC_THROTTLE_VALUE
	StepTypeThrottleTimedValue   StepType = 20 // ZBX_PREPROC_THROTTLE_TIMED_VALUE
	StepTypeJavaScript           StepType = 21 // ZBX_PREPROC_SCRIPT
	StepTypePrometheusPattern    StepType = 22 // ZBX_PREPROC_PROMETHEUS_PATTERN
	StepTypePrometheusToJSON     StepType = 23 // ZBX_PREPROC_PROMETHEUS_TO_JSON
	StepTypeCSVToJSON            StepType = 24 // ZBX_PREPROC_CSV_TO_JSON
	StepTypeStringReplace        StepType = 25 // ZBX_PREPROC_STR_REPLACE
	StepTypeValidateNotSupported StepType = 26 // ZBX_PREPROC_VALIDATE_NOT_SUPPORTED
	StepTypeXMLToJSON            StepType = 27 // ZBX_PREPROC_XML_TO_JSON
	StepTypeSNMPWalkValue        StepType = 28 // ZBX_PREPROC_SNMP_WALK_VALUE
	StepTypeSNMPWalkToJSON       StepType = 29 // ZBX_PREPROC_SNMP_WALK_TO_JSON
	StepTypeSNMPGetValue         StepType = 30 // ZBX_PREPROC_SNMP_GET_VALUE

	// Multi-metric step types (60+) - return multiple Metric objects instead of single JSON string
	// These are EXTENSIONS to Zabbix, not part of official spec
	StepTypePrometheusToJSONMulti StepType = 60 // Extended: Prometheus to multi-metric array
	StepTypeJSONPathMulti         StepType = 61 // Extended: JSONPath array extraction
	StepTypeSNMPWalkToJSONMulti   StepType = 62 // Extended: SNMP walk to multi-metric array
	StepTypeCSVToJSONMulti        StepType = 63 // Extended: CSV to multi-metric array
)

// ErrorAction defines how preprocessing errors are handled.
type ErrorAction int

const (
	ErrorActionDefault  ErrorAction = 0 // Return error
	ErrorActionDiscard  ErrorAction = 1 // Discard value (return empty)
	ErrorActionSetValue ErrorAction = 2 // Set custom value
	ErrorActionSetError ErrorAction = 3 // Set custom error message
)

// ErrorHandler defines error handling behavior for a preprocessing step.
type ErrorHandler struct {
	Action ErrorAction
	Params string
}

// Step represents a single preprocessing step.
type Step struct {
	Type         StepType
	Params       string
	ErrorHandler ErrorHandler
}

// Metric represents a single metric output from preprocessing.
type Metric struct {
	Name   string            // Metric name
	Value  string            // Metric value
	Type   ValueType         // Value type
	Labels map[string]string // Optional labels
}

// Result represents the result of preprocessing a value.
type Result struct {
	Metrics   []Metric // Array of extracted metrics
	Logs      []string // Optional log entries (future use)
	Error     error    // Overall error if preprocessing failed
	Discarded bool     // True if value was intentionally discarded (e.g., throttling)
}

// Preprocessor executes preprocessing steps.
// It maintains state for stateful operations (per shard instance).
// Thread-safe for concurrent use.
type Preprocessor struct {
	shardID       string
	logger        Logger                     // Logger for debugging (defaults to NoopLogger)
	limits        Limits                     // Configurable limits for preprocessing operations
	mu            sync.RWMutex               // Protects state map access
	state         map[string]*OperationState // Per-operation state keyed by operation ID
	cleanupTicker *time.Ticker               // Ticker for periodic state cleanup (can be nil if disabled)
	cleanupDone   chan struct{}              // Signal to stop cleanup goroutine
	stateTTL      time.Duration              // TTL for inactive state entries (0 = no cleanup)
}

// OperationState tracks state for stateful operations like Delta and Throttle.
type OperationState struct {
	LastValue     string
	LastTimestamp time.Time
	LastValueTime time.Time
	LastAccess    time.Time // Track last access for TTL-based cleanup
}

// Config holds configuration options for a Preprocessor instance.
// All fields are optional - zero values use sensible defaults.
type Config struct {
	// Logger for debugging (default: NoopLogger - zero overhead)
	Logger Logger

	// Limits for preprocessing operations (default: Zabbix-compatible limits)
	Limits Limits

	// StateTTL is the time-to-live for inactive state entries.
	// Default: 0 (no automatic cleanup)
	// Set to positive duration to enable automatic eviction.
	StateTTL time.Duration

	// StateCleanupInterval is how often to run the cleanup process.
	// Default: 0 (no automatic cleanup)
	// Only used if StateTTL > 0.
	StateCleanupInterval time.Duration
}

// NewPreprocessor creates a new preprocessor instance for a specific shard.
// By default, no state cleanup is performed (stateTTL = 0).
// Call EnableStateCleanup() to enable TTL-based eviction.
func NewPreprocessor(shardID string) *Preprocessor {
	return &Preprocessor{
		shardID:  shardID,
		logger:   NoopLogger{}, // Default: no logging overhead
		limits:   DefaultLimits(),
		state:    make(map[string]*OperationState),
		stateTTL: 0, // Default: no cleanup
	}
}

// NewPreprocessorWithConfig creates a new preprocessor with custom configuration.
// This allows per-shard customization of logging, timeouts, and cleanup policies.
func NewPreprocessorWithConfig(shardID string, cfg Config) *Preprocessor {
	// Use default limits if not provided (zero value check)
	limits := cfg.Limits
	if limits.JavaScript.Timeout == 0 {
		limits = DefaultLimits()
	}

	p := &Preprocessor{
		shardID:  shardID,
		limits:   limits,
		state:    make(map[string]*OperationState),
		stateTTL: cfg.StateTTL,
	}

	// Set logger (default to NoopLogger if not provided)
	if cfg.Logger != nil {
		p.logger = cfg.Logger
	} else {
		p.logger = NoopLogger{}
	}

	// Enable automatic state cleanup if configured
	if cfg.StateTTL > 0 && cfg.StateCleanupInterval > 0 {
		p.EnableStateCleanup(cfg.StateTTL, cfg.StateCleanupInterval)
	}

	return p
}

// EnableStateCleanup enables periodic cleanup of inactive state entries.
// ttl: How long an entry can be inactive before being removed
// cleanupInterval: How often to run the cleanup process
// Returns a function to stop the cleanup goroutine (call on shutdown).
func (p *Preprocessor) EnableStateCleanup(ttl, cleanupInterval time.Duration) func() {
	p.mu.Lock()
	defer p.mu.Unlock()

	// Stop existing cleanup if any
	if p.cleanupTicker != nil {
		p.cleanupTicker.Stop()
		if p.cleanupDone != nil {
			close(p.cleanupDone)
		}
	}

	p.stateTTL = ttl
	p.cleanupTicker = time.NewTicker(cleanupInterval)
	p.cleanupDone = make(chan struct{})

	// Start cleanup goroutine
	go p.runStateCleanup()

	// Return stop function
	return func() {
		p.mu.Lock()
		defer p.mu.Unlock()
		if p.cleanupTicker != nil {
			p.cleanupTicker.Stop()
			close(p.cleanupDone)
			p.cleanupTicker = nil
			p.cleanupDone = nil
		}
	}
}

// runStateCleanup periodically removes inactive state entries.
func (p *Preprocessor) runStateCleanup() {
	for {
		select {
		case <-p.cleanupTicker.C:
			p.cleanupInactiveState()
		case <-p.cleanupDone:
			return
		}
	}
}

// cleanupInactiveState removes state entries that haven't been accessed within TTL.
func (p *Preprocessor) cleanupInactiveState() {
	if p.stateTTL == 0 {
		return // Cleanup disabled
	}

	p.mu.Lock()
	defer p.mu.Unlock()

	now := time.Now()
	removed := 0

	for key, state := range p.state {
		if now.Sub(state.LastAccess) > p.stateTTL {
			delete(p.state, key)
			removed++
		}
	}

	if removed > 0 {
		p.logger.Debug("state cleanup completed",
			"shard", p.shardID,
			"removed", removed,
			"remaining", len(p.state))
	}
}

// ClearState removes all stored state entries for the given itemID within this shard.
// Used when an item/instance is obsoleted so historical state does not leak to new entities.
func (p *Preprocessor) ClearState(itemID string) {
	if itemID == "" {
		return
	}
	prefix := fmt.Sprintf("%s:%s:", p.shardID, itemID)
	p.mu.Lock()
	defer p.mu.Unlock()
	for key := range p.state {
		if strings.HasPrefix(key, prefix) {
			delete(p.state, key)
		}
	}
}

// SetLogger sets the logger for this preprocessor instance.
// By default, NoopLogger is used (zero overhead).
// This can be called to enable debug logging for troubleshooting.
func (p *Preprocessor) SetLogger(logger Logger) {
	p.logger = logger
}

// PreloadState sets the history state for a stateful operation.
// This is primarily for testing - allows proper setup of delta/throttle state.
// itemID: The item identifier
// stepType: The step type (must be stateful: Delta, Throttle)
// lastValue: The previous value
// lastTimestamp: The timestamp of the previous value
func (p *Preprocessor) PreloadState(itemID string, stepType StepType, lastValue string, lastTimestamp time.Time) error {
	var stateKey string
	switch stepType {
	case StepTypeDeltaValue:
		stateKey = fmt.Sprintf("%s:%s:delta_value", p.shardID, itemID)
	case StepTypeDeltaSpeed:
		stateKey = fmt.Sprintf("%s:%s:delta_speed", p.shardID, itemID)
	case StepTypeThrottleValue:
		stateKey = fmt.Sprintf("%s:%s:throttle_value", p.shardID, itemID)
	case StepTypeThrottleTimedValue:
		stateKey = fmt.Sprintf("%s:%s:throttle_timed", p.shardID, itemID)
	default:
		return fmt.Errorf("step type %d is not stateful", stepType)
	}

	p.mu.Lock()
	p.state[stateKey] = &OperationState{
		LastValue:     lastValue,
		LastValueTime: lastTimestamp,
		LastTimestamp: lastTimestamp,
		LastAccess:    time.Now(),
	}
	p.mu.Unlock()

	return nil
}

// Execute applies a single preprocessing step to a value for a specific item.
// itemID: Unique identifier for the item within this shard (used for state isolation)
// value: The input value to process
// step: The preprocessing step to apply
// Returns: Result containing metrics array, optional logs, and error if failed
func (p *Preprocessor) Execute(itemID string, value Value, step Step) (Result, error) {
	if err := validateStep(step); err != nil {
		return Result{Error: err}, err
	}

	result, err := p.executeStep(itemID, value, step)
	if err != nil {
		handledValue, handledErr := p.handleError(err, step.ErrorHandler)
		return valueToResult(handledValue, handledErr), handledErr
	}
	return result, nil
}

// ExecutePipeline applies multiple preprocessing steps in sequence for a specific item.
// itemID: Unique identifier for the item within this shard (used for state isolation)
// value: The input value to process
// steps: The preprocessing steps to apply in order
// Returns: Result containing metrics array, optional logs, and error if failed
//
// IsError Semantics:
// The Value.IsError flag indicates whether the INPUT value originated from an error condition
// (e.g., failed data collection in Zabbix). This flag is preserved through the entire pipeline
// because it's a property of the original data source, not the preprocessing results.
// - IsError=true: Original data came from an error (used by validateNotSupported)
// - IsError=false: Original data was successfully collected
// This flag does NOT change based on step success/failure - those are separate concerns.
// Step failures return errors; IsError tracks input data provenance.
type pipelineValue struct {
	value Value
	meta  Metric
}

func (p *Preprocessor) ExecutePipeline(itemID string, value Value, steps []Step) (Result, error) {
	values := []pipelineValue{{value: value}}
	for i, step := range steps {
		next := make([]pipelineValue, 0, len(values))
		for _, pv := range values {
			result, err := p.Execute(itemID, pv.value, step)
			if err != nil {
				pipelineErr := fmt.Errorf("step %d failed: %w", i, err)
				return Result{Error: pipelineErr}, pipelineErr
			}
			if result.Discarded || len(result.Metrics) == 0 {
				return Result{Discarded: true}, nil
			}
			if isMultiStepType(step.Type) {
				for _, metric := range result.Metrics {
					next = append(next, pipelineValue{
						value: Value{
							Data:      metric.Value,
							Type:      metric.Type,
							Timestamp: pv.value.Timestamp,
							IsError:   pv.value.IsError,
						},
						meta: metric,
					})
				}
			} else {
				metric := result.Metrics[0]
				next = append(next, pipelineValue{
					value: Value{
						Data:      metric.Value,
						Type:      metric.Type,
						Timestamp: pv.value.Timestamp,
						IsError:   pv.value.IsError,
					},
				})
			}
		}
		if len(next) == 0 {
			return Result{Discarded: true}, nil
		}
		values = next
	}
	metrics := make([]Metric, len(values))
	for i, pv := range values {
		meta := pv.meta
		metrics[i] = Metric{
			Name:   meta.Name,
			Value:  pv.value.Data,
			Type:   pv.value.Type,
			Labels: meta.Labels,
		}
	}
	return Result{Metrics: metrics}, nil
}

func isMultiStepType(t StepType) bool {
	switch t {
	case StepTypePrometheusToJSONMulti,
		StepTypeJSONPathMulti,
		StepTypeSNMPWalkToJSONMulti,
		StepTypeCSVToJSONMulti:
		return true
	default:
		return false
	}
}

// valueToResult converts a Value to a Result with a single metric
func valueToResult(value Value, err error) Result {
	if err != nil {
		return Result{Error: err}
	}
	return Result{
		Metrics: []Metric{{
			Name:  "",
			Value: value.Data,
			Type:  value.Type,
		}},
		Error: nil,
	}
}

func (p *Preprocessor) executeStep(itemID string, value Value, step Step) (Result, error) {
	// Multi-metric steps - return Result with multiple Metric objects directly
	// These are NEW step types (60+) that don't break Zabbix compatibility
	switch step.Type {
	case StepTypePrometheusToJSONMulti:
		return prometheusToJSONMulti(value, step.Params)
	case StepTypeJSONPathMulti:
		return jsonPathMulti(value, step.Params)
	case StepTypeSNMPWalkToJSONMulti:
		return snmpWalkToJSONMulti(value, step.Params, p.logger)
	case StepTypeCSVToJSONMulti:
		return csvToJSONMulti(value, step.Params)
	}

	// Single-metric steps - execute and wrap with valueToResult()
	// These maintain Zabbix compatibility (original step types 1-29)
	var result Value
	var err error

	switch step.Type {
	case StepTypeMultiplier:
		result, err = multiplyValue(value, step.Params)
	case StepTypeTrim:
		result, err = trimValue(value, step.Params, "both")
	case StepTypeRTrim:
		result, err = trimValue(value, step.Params, "right")
	case StepTypeLTrim:
		result, err = trimValue(value, step.Params, "left")
	case StepTypeRegexSubstitution:
		result, err = regexSubstitute(value, step.Params)
	case StepTypeBool2Dec:
		result, err = bool2Dec(value)
	case StepTypeOct2Dec:
		result, err = oct2Dec(value)
	case StepTypeHex2Dec:
		result, err = hex2Dec(value)
	case StepTypeDeltaValue:
		result, err = p.deltaValue(itemID, value, step.Params)
	case StepTypeDeltaSpeed:
		result, err = p.deltaSpeed(itemID, value, step.Params)
	case StepTypeStringReplace:
		result, err = stringReplace(value, step.Params)
	case StepTypeValidateRange:
		result, err = validateRange(value, step.Params)
	case StepTypeValidateRegex:
		result, err = validateRegex(value, step.Params)
	case StepTypeValidateNotRegex:
		result, err = validateNotRegex(value, step.Params)
	case StepTypeValidateNotSupported:
		result, err = validateNotSupported(value, step.Params)
	case StepTypeJSONPath:
		result, err = jsonpathExtract(value, step.Params)
	case StepTypeXPath:
		result, err = xpathExtract(value, step.Params)
	case StepTypePrometheusPattern:
		result, err = prometheusPattern(value, step.Params)
	case StepTypePrometheusToJSON:
		result, err = prometheusToJSON(value, step.Params)
	case StepTypeCSVToJSON:
		result, err = csvToJSON(value, step.Params)
	case StepTypeXMLToJSON:
		result, err = xmlToJSON(value, step.Params)
	case StepTypeErrorFieldJSON:
		result, err = errorFieldJSON(value, step.Params)
	case StepTypeErrorFieldXML:
		result, err = errorFieldXML(value, step.Params)
	case StepTypeErrorFieldRegex:
		result, err = errorFieldRegex(value, step.Params)
	case StepTypeThrottleValue:
		result, err = p.throttleValue(itemID, value, step.Params)
	case StepTypeThrottleTimedValue:
		result, err = p.throttleTimedValue(itemID, value, step.Params)
	case StepTypeJavaScript:
		result, err = javascriptExecute(value, step.Params, p.limits.JavaScript)
	case StepTypeSNMPWalkValue:
		result, err = snmpWalkToValue(value, step.Params)
	case StepTypeSNMPGetValue:
		result, err = snmpGetValue(value, step.Params)
	case StepTypeSNMPWalkToJSON:
		result, err = snmpWalkToJSON(value, step.Params, p.logger)
	default:
		return Result{Error: fmt.Errorf("unsupported preprocessing type: %d", step.Type)}, fmt.Errorf("unsupported preprocessing type: %d", step.Type)
	}

	return valueToResult(result, err), err
}

func (p *Preprocessor) handleError(err error, handler ErrorHandler) (Value, error) {
	switch handler.Action {
	case ErrorActionDefault:
		return Value{}, err
	case ErrorActionDiscard:
		// Zabbix behavior: discard means replace error with empty string value
		return Value{Data: "", Type: ValueTypeStr}, nil
	case ErrorActionSetValue:
		return Value{Data: handler.Params, Type: ValueTypeStr}, nil
	case ErrorActionSetError:
		return Value{}, fmt.Errorf("%s", handler.Params)
	default:
		return Value{}, err
	}
}

func validateStep(step Step) error {
	if step.Type < 0 {
		return fmt.Errorf("invalid step type: %d", step.Type)
	}
	return nil
}

func parseValueType(s string) (ValueType, error) {
	switch s {
	case "ITEM_VALUE_TYPE_STR":
		return ValueTypeStr, nil
	case "ITEM_VALUE_TYPE_UINT64":
		return ValueTypeUint64, nil
	case "ITEM_VALUE_TYPE_FLOAT":
		return ValueTypeFloat, nil
	default:
		return ValueTypeStr, fmt.Errorf("unknown value type: %s", s)
	}
}

func valueTypeToString(vt ValueType) string {
	switch vt {
	case ValueTypeStr:
		return "ITEM_VALUE_TYPE_STR"
	case ValueTypeUint64:
		return "ITEM_VALUE_TYPE_UINT64"
	case ValueTypeFloat:
		return "ITEM_VALUE_TYPE_FLOAT"
	default:
		return "UNKNOWN"
	}
}
