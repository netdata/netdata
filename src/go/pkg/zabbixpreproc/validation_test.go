package zabbixpreproc

import (
	"strings"
	"testing"
)

func TestValidateStep(t *testing.T) {
	tests := []struct {
		name    string
		step    Step
		wantErr bool
		errMsg  string
	}{
		// Valid steps
		{
			name:    "valid multiplier",
			step:    Step{Type: StepTypeMultiplier, Params: "2.5"},
			wantErr: false,
		},
		{
			name:    "valid jsonpath",
			step:    Step{Type: StepTypeJSONPath, Params: "$.cpu"},
			wantErr: false,
		},
		{
			name:    "valid csv to json",
			step:    Step{Type: StepTypeCSVToJSON, Params: ",\n\"\n1"},
			wantErr: false,
		},
		{
			name:    "valid snmp walk to json",
			step:    Step{Type: StepTypeSNMPWalkToJSON, Params: "{#NAME}\n.1.3.6.1.2.1.1\n1"},
			wantErr: false,
		},
		{
			name:    "valid trim with params",
			step:    Step{Type: StepTypeTrim, Params: " "},
			wantErr: false,
		},
		{
			name:    "valid trim without params",
			step:    Step{Type: StepTypeTrim, Params: ""},
			wantErr: false,
		},
		{
			name:    "valid bool2dec",
			step:    Step{Type: StepTypeBool2Dec, Params: ""},
			wantErr: false,
		},

		// Invalid steps - negative type
		{
			name:    "negative step type",
			step:    Step{Type: StepType(-1), Params: ""},
			wantErr: true,
			errMsg:  "invalid step type",
		},

		// Invalid steps - missing required params
		{
			name:    "multiplier missing params",
			step:    Step{Type: StepTypeMultiplier, Params: ""},
			wantErr: true,
			errMsg:  "multiplier step requires a numeric parameter",
		},
		{
			name:    "jsonpath missing params",
			step:    Step{Type: StepTypeJSONPath, Params: ""},
			wantErr: true,
			errMsg:  "jsonpath step requires",
		},
		{
			name:    "xpath missing params",
			step:    Step{Type: StepTypeXPath, Params: ""},
			wantErr: true,
			errMsg:  "xpath step requires",
		},
		{
			name:    "prometheus pattern missing params",
			step:    Step{Type: StepTypePrometheusPattern, Params: ""},
			wantErr: true,
			errMsg:  "prometheus pattern requires",
		},
		{
			name:    "javascript missing params",
			step:    Step{Type: StepTypeJavaScript, Params: ""},
			wantErr: true,
			errMsg:  "javascript step requires",
		},

		// Invalid steps - malformed params
		{
			name:    "regex substitution missing newline",
			step:    Step{Type: StepTypeRegexSubstitution, Params: "pattern"},
			wantErr: true,
			errMsg:  "requires newline-separated",
		},
		{
			name:    "string replace missing newline",
			step:    Step{Type: StepTypeStringReplace, Params: "search"},
			wantErr: true,
			errMsg:  "requires newline-separated",
		},
		{
			name:    "csv to json insufficient params",
			step:    Step{Type: StepTypeCSVToJSON, Params: ",\n\""},
			wantErr: true,
			errMsg:  "requires 3 parameters",
		},
		{
			name:    "snmp walk value insufficient params",
			step:    Step{Type: StepTypeSNMPWalkValue, Params: ".1.3.6.1"},
			wantErr: true,
			errMsg:  "requires 2 parameters",
		},
		{
			name:    "snmp walk to json non-triplet params",
			step:    Step{Type: StepTypeSNMPWalkToJSON, Params: "{#NAME}\n.1.3.6.1"},
			wantErr: true,
			errMsg:  "requires parameters in triplets",
		},
		{
			name:    "snmp walk to json empty params",
			step:    Step{Type: StepTypeSNMPWalkToJSON, Params: ""},
			wantErr: true,
			errMsg:  "requires at least one triplet",
		},

		// Error handler validation
		{
			name: "valid error handler - default",
			step: Step{
				Type:         StepTypeJSONPath,
				Params:       "$.value",
				ErrorHandler: ErrorHandler{Action: ErrorActionDefault},
			},
			wantErr: false,
		},
		{
			name: "valid error handler - discard",
			step: Step{
				Type:         StepTypeJSONPath,
				Params:       "$.value",
				ErrorHandler: ErrorHandler{Action: ErrorActionDiscard},
			},
			wantErr: false,
		},
		{
			name: "valid error handler - set value",
			step: Step{
				Type:         StepTypeJSONPath,
				Params:       "$.value",
				ErrorHandler: ErrorHandler{Action: ErrorActionSetValue, Params: "0"},
			},
			wantErr: false,
		},
		{
			name: "valid error handler - set value empty",
			step: Step{
				Type:         StepTypeJSONPath,
				Params:       "$.value",
				ErrorHandler: ErrorHandler{Action: ErrorActionSetValue, Params: ""},
			},
			wantErr: false,
		},
		{
			name: "valid error handler - set error",
			step: Step{
				Type:         StepTypeJSONPath,
				Params:       "$.value",
				ErrorHandler: ErrorHandler{Action: ErrorActionSetError, Params: "Custom error"},
			},
			wantErr: false,
		},
		{
			name: "invalid error handler - set error missing message",
			step: Step{
				Type:         StepTypeJSONPath,
				Params:       "$.value",
				ErrorHandler: ErrorHandler{Action: ErrorActionSetError, Params: ""},
			},
			wantErr: true,
			errMsg:  "requires an error message parameter",
		},
		{
			name: "invalid error handler - unknown action",
			step: Step{
				Type:         StepTypeJSONPath,
				Params:       "$.value",
				ErrorHandler: ErrorHandler{Action: ErrorAction(99)},
			},
			wantErr: true,
			errMsg:  "invalid error handler action",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateStep(tt.step)
			if tt.wantErr {
				if err == nil {
					t.Errorf("ValidateStep() expected error containing %q, got nil", tt.errMsg)
					return
				}
				if !strings.Contains(err.Error(), tt.errMsg) {
					t.Errorf("ValidateStep() error = %v, want error containing %q", err, tt.errMsg)
				}
			} else {
				if err != nil {
					t.Errorf("ValidateStep() unexpected error = %v", err)
				}
			}
		})
	}
}

func TestValidatePipeline(t *testing.T) {
	tests := []struct {
		name    string
		steps   []Step
		wantErr bool
		errMsg  string
	}{
		{
			name: "valid pipeline",
			steps: []Step{
				{Type: StepTypeJSONPath, Params: "$.cpu"},
				{Type: StepTypeMultiplier, Params: "100"},
				{Type: StepTypeTrim, Params: ""},
			},
			wantErr: false,
		},
		{
			name:    "empty pipeline",
			steps:   []Step{},
			wantErr: true,
			errMsg:  "pipeline cannot be empty",
		},
		{
			name: "pipeline with invalid step at index 1",
			steps: []Step{
				{Type: StepTypeJSONPath, Params: "$.cpu"},
				{Type: StepTypeMultiplier, Params: ""}, // Missing params
				{Type: StepTypeTrim, Params: ""},
			},
			wantErr: true,
			errMsg:  "step 1:",
		},
		{
			name: "pipeline with invalid step at index 0",
			steps: []Step{
				{Type: StepTypeJSONPath, Params: ""}, // Missing params
				{Type: StepTypeMultiplier, Params: "100"},
			},
			wantErr: true,
			errMsg:  "step 0:",
		},
		{
			name: "pipeline with invalid error handler",
			steps: []Step{
				{Type: StepTypeJSONPath, Params: "$.cpu"},
				{
					Type:         StepTypeMultiplier,
					Params:       "100",
					ErrorHandler: ErrorHandler{Action: ErrorActionSetError, Params: ""},
				},
			},
			wantErr: true,
			errMsg:  "step 1:",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidatePipeline(tt.steps)
			if tt.wantErr {
				if err == nil {
					t.Errorf("ValidatePipeline() expected error containing %q, got nil", tt.errMsg)
					return
				}
				if !strings.Contains(err.Error(), tt.errMsg) {
					t.Errorf("ValidatePipeline() error = %v, want error containing %q", err, tt.errMsg)
				}
			} else {
				if err != nil {
					t.Errorf("ValidatePipeline() unexpected error = %v", err)
				}
			}
		})
	}
}
