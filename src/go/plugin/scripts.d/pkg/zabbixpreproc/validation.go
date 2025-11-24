package zabbixpreproc

import (
	"fmt"
	"strings"
)

// ValidateStep performs early validation of step parameters.
// This catches common errors before execution, useful for pipeline validation.
// Note: Does not perform deep validation (regex compilation, JSONPath parsing, etc.)
// as those are checked during execution.
func ValidateStep(step Step) error {
	// Check step type is in valid range
	if step.Type < 0 {
		return fmt.Errorf("invalid step type: %d (must be >= 0)", step.Type)
	}

	// Validate parameters based on step type
	switch step.Type {
	// Steps that require non-empty parameters
	case StepTypeMultiplier:
		if step.Params == "" {
			return fmt.Errorf("multiplier step requires a numeric parameter")
		}

	case StepTypeRegexSubstitution:
		if step.Params == "" {
			return fmt.Errorf("regex substitution requires pattern and replacement (pattern\\nreplacement)")
		}
		if !strings.Contains(step.Params, "\n") {
			return fmt.Errorf("regex substitution requires newline-separated pattern and replacement")
		}

	case StepTypeStringReplace:
		if step.Params == "" {
			return fmt.Errorf("string replace requires search and replacement (search\\nreplacement)")
		}
		if !strings.Contains(step.Params, "\n") {
			return fmt.Errorf("string replace requires newline-separated search and replacement")
		}

	case StepTypeJSONPath, StepTypeJSONPathMulti:
		if step.Params == "" {
			return fmt.Errorf("jsonpath step requires a jsonpath expression parameter")
		}

	case StepTypeXPath:
		if step.Params == "" {
			return fmt.Errorf("xpath step requires an xpath expression parameter")
		}

	case StepTypePrometheusPattern:
		if step.Params == "" {
			return fmt.Errorf("prometheus pattern requires a pattern parameter")
		}

	case StepTypePrometheusToJSON, StepTypePrometheusToJSONMulti:
		if step.Params == "" {
			return fmt.Errorf("prometheus to json requires a pattern parameter")
		}

	case StepTypeCSVToJSON, StepTypeCSVToJSONMulti:
		parts := strings.Split(step.Params, "\n")
		if len(parts) < 3 {
			return fmt.Errorf("csv to json requires 3 parameters: delimiter, quote, header (got %d)", len(parts))
		}

	case StepTypeSNMPWalkValue:
		parts := strings.Split(step.Params, "\n")
		if len(parts) < 2 {
			return fmt.Errorf("snmp walk value requires 2 parameters: oid, format (got %d)", len(parts))
		}

	case StepTypeSNMPGetValue:
		if step.Params == "" {
			return fmt.Errorf("snmp get value requires format mode parameter")
		}

	case StepTypeSNMPWalkToJSON, StepTypeSNMPWalkToJSONMulti:
		if step.Params == "" {
			return fmt.Errorf("snmp walk to json requires at least one triplet (macro, oid, format)")
		}
		parts := strings.Split(step.Params, "\n")
		if len(parts)%3 != 0 {
			return fmt.Errorf("snmp walk to json requires parameters in triplets of (macro, oid, format), got %d parameters", len(parts))
		}

	case StepTypeValidateRange:
		if step.Params == "" {
			return fmt.Errorf("validate range requires min and max parameters")
		}

	case StepTypeValidateRegex:
		if step.Params == "" {
			return fmt.Errorf("validate regex requires a pattern parameter")
		}

	case StepTypeValidateNotRegex:
		if step.Params == "" {
			return fmt.Errorf("validate not regex requires a pattern parameter")
		}

	case StepTypeValidateNotSupported:
		if step.Params == "" {
			return fmt.Errorf("validate not supported requires an error message parameter")
		}

	case StepTypeErrorFieldJSON:
		if step.Params == "" {
			return fmt.Errorf("error field json requires a jsonpath expression parameter")
		}

	case StepTypeErrorFieldXML:
		if step.Params == "" {
			return fmt.Errorf("error field xml requires an xpath expression parameter")
		}

	case StepTypeErrorFieldRegex:
		if step.Params == "" {
			return fmt.Errorf("error field regex requires a pattern parameter")
		}

	case StepTypeThrottleValue:
		if step.Params == "" {
			return fmt.Errorf("throttle value requires a time period parameter")
		}

	case StepTypeThrottleTimedValue:
		if step.Params == "" {
			return fmt.Errorf("throttle timed value requires a time period parameter")
		}

	case StepTypeJavaScript:
		if step.Params == "" {
			return fmt.Errorf("javascript step requires code parameter")
		}

	case StepTypeDeltaValue, StepTypeDeltaSpeed:
		// Params optional for delta steps (defaults handled in execution)

	case StepTypeTrim, StepTypeRTrim, StepTypeLTrim:
		// Params optional for trim (defaults to whitespace)

	case StepTypeBool2Dec, StepTypeOct2Dec, StepTypeHex2Dec:
		// No parameters required for conversion steps

	default:
		// Unknown step type - not necessarily an error (future extensibility)
		// The actual execution will handle this
	}

	// Validate error handler
	switch step.ErrorHandler.Action {
	case ErrorActionDefault, ErrorActionDiscard:
		// No params required

	case ErrorActionSetValue:
		// Params should contain the value to set (can be empty string)

	case ErrorActionSetError:
		if step.ErrorHandler.Params == "" {
			return fmt.Errorf("error handler set_error requires an error message parameter")
		}

	default:
		return fmt.Errorf("invalid error handler action: %d", step.ErrorHandler.Action)
	}

	return nil
}

// ValidatePipeline validates all steps in a pipeline before execution.
// This is useful for catching configuration errors early, especially in multi-step pipelines.
func ValidatePipeline(steps []Step) error {
	if len(steps) == 0 {
		return fmt.Errorf("pipeline cannot be empty")
	}

	for i, step := range steps {
		if err := ValidateStep(step); err != nil {
			return fmt.Errorf("step %d: %w", i, err)
		}
	}

	return nil
}
