// SPDX-License-Identifier: GPL-3.0-or-later

package collecttest

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"
	"testing"

	"github.com/netdata/netdata/go/plugins/pkg/funcapi"
)

// Live Data drift checks: the Function implementation is the source of truth
// for ids, parameters and returned columns, and the metadata.yaml functions
// section must describe them. As with charts and alerts, the checks compare the
// implementation with the decoded document so no test restates a column.

// MetadataFunctionColumn is one metadata.yaml functions.list[].returns.columns[]
// row: Visibility is "hidden" for a column the table does not show by default.
type MetadataFunctionColumn struct {
	Name       string
	Type       string
	Unit       string
	Visibility string
}

// MetadataFunctionOption is one option of a documented select parameter.
type MetadataFunctionOption struct {
	ID      string
	Name    string
	Default bool
}

// MetadataFunctionParameter is one metadata.yaml functions.list[].parameters[]
// row. required and default are documentation-only and not decoded.
type MetadataFunctionParameter struct {
	ID      string
	Name    string
	Type    string
	Options []MetadataFunctionOption
}

// MetadataFunction is one documented Function. FunctionName is the documented
// public name; empty means the default "<module>:<id>".
type MetadataFunction struct {
	ID           string
	FunctionName string
	Parameters   []MetadataFunctionParameter
	Columns      []MetadataFunctionColumn
}

// ImplementedFunction is the shape of one Function as the collector implements
// it: its registered id, its effective parameters (static FunctionConfig
// parameters merged with the handler's dynamic ones, as the framework does)
// and the columns of a table response. A RawRequest Function owns its own
// response contract, so its columns are not compared.
type ImplementedFunction struct {
	ID         string
	PublicName string // as funcapi.FunctionName resolves it; empty skips the name comparison
	Parameters []funcapi.ParamConfig
	Columns    []MetadataFunctionColumn
	RawRequest bool
}

// FunctionJobParameter is the Instance selector the framework adds to every
// job-backed Function of a per-job collector; a single-instance collector has none.
const FunctionJobParameter = "__job"

// functionJobParameterName is the label the framework gives that selector.
const functionJobParameterName = "Instance"

// FunctionResponseColumns reduces a table response to the column rows the Live
// Data section documents, in index order.
func FunctionResponseColumns(response *funcapi.FunctionResponse) ([]MetadataFunctionColumn, error) {
	if response == nil {
		return nil, errors.New("no response")
	}
	// The framework treats an unset status as 200.
	if response.Status != 0 && response.Status != 200 {
		return nil, fmt.Errorf("status %d: %s", response.Status, response.Message)
	}
	if response.RawResponse != nil {
		return nil, errors.New("raw responses are not supported")
	}
	columns := make([]MetadataFunctionColumn, len(response.Columns))
	seen := make([]bool, len(response.Columns))
	for name, raw := range response.Columns {
		meta, ok := raw.(map[string]any)
		if !ok {
			return nil, fmt.Errorf("column %q: not a column definition", name)
		}
		index, ok := meta["index"].(int)
		if !ok || index < 0 || index >= len(columns) || seen[index] {
			return nil, fmt.Errorf("column %q: invalid or duplicate index %v", name, meta["index"])
		}
		seen[index] = true
		column := MetadataFunctionColumn{Name: name}
		column.Type, _ = meta["type"].(string)
		column.Unit, _ = meta["units"].(string)
		if visible, _ := meta["visible"].(bool); !visible {
			column.Visibility = "hidden"
		}
		columns[index] = column
	}
	return columns, nil
}

// CheckMetadataFunctionsMatch requires the documented Functions to be exactly
// the implemented ones: every id both ways, the same parameters (id, name,
// type, options) and the same columns in order (name, type, unit, visibility).
// When jobSelectable is set, every documented Function must also document the
// framework's Instance selector as a select parameter named Instance; its
// options are job names known only at runtime and are not compared. Otherwise documenting that
// selector is drift.
func CheckMetadataFunctionsMatch(documented []MetadataFunction, implemented []ImplementedFunction, jobSelectable bool) error {
	byID := make(map[string]ImplementedFunction, len(implemented))
	for _, function := range implemented {
		byID[function.ID] = function
	}
	var problems []error
	seen := make(map[string]bool, len(documented))
	for _, function := range documented {
		if seen[function.ID] {
			problems = append(problems, fmt.Errorf("%s: documented twice", function.ID))
			continue
		}
		seen[function.ID] = true
		actual, ok := byID[function.ID]
		if !ok {
			problems = append(problems, fmt.Errorf("%s: documented but not implemented", function.ID))
			continue
		}
		problems = append(problems, checkFunctionName(function, actual)...)
		problems = append(problems, checkFunctionParameters(function, actual, jobSelectable)...)
		problems = append(problems, checkFunctionColumns(function, actual)...)
	}
	for id := range byID {
		if !seen[id] {
			problems = append(problems, fmt.Errorf("%s: implemented but not documented", id))
		}
	}
	return errors.Join(sortedErrors(problems)...)
}

// checkFunctionName compares the documented public name with the resolved one.
// A documented name is required only when the collector overrides the default.
func checkFunctionName(documented MetadataFunction, actual ImplementedFunction) []error {
	if actual.PublicName == "" {
		return nil
	}
	module, _, _ := strings.Cut(actual.PublicName, ":")
	isDefault := actual.PublicName == module+":"+actual.ID
	switch {
	case documented.FunctionName == "" && isDefault:
		return nil
	case documented.FunctionName == "" && !isDefault:
		return []error{fmt.Errorf("%s: public name %q is not the default, document it as function_name", documented.ID, actual.PublicName)}
	case documented.FunctionName != actual.PublicName:
		return []error{fmt.Errorf("%s: function_name %q, public name %q", documented.ID, documented.FunctionName, actual.PublicName)}
	}
	return nil
}

func checkFunctionParameters(documented MetadataFunction, actual ImplementedFunction, jobSelectable bool) []error {
	var problems []error
	expected := make(map[string]funcapi.ParamConfig, len(actual.Parameters))
	for _, param := range actual.Parameters {
		expected[param.ID] = param
	}
	seen := make(map[string]bool, len(documented.Parameters))
	for _, param := range documented.Parameters {
		seen[param.ID] = true
		if param.ID == FunctionJobParameter {
			switch {
			case !jobSelectable:
				problems = append(problems, fmt.Errorf("%s: parameter %s documented, but the collector is single-instance", documented.ID, param.ID))
			case param.Type != funcapi.ParamSelect.String():
				problems = append(problems, fmt.Errorf("%s: parameter %s type %q, framework selector is %q", documented.ID, param.ID, param.Type, funcapi.ParamSelect))
			case param.Name != functionJobParameterName:
				problems = append(problems, fmt.Errorf("%s: parameter %s name %q, framework selector is %q", documented.ID, param.ID, param.Name, functionJobParameterName))
			}
			continue
		}
		config, ok := expected[param.ID]
		if !ok {
			problems = append(problems, fmt.Errorf("%s: parameter %s documented but not declared", documented.ID, param.ID))
			continue
		}
		if param.Name != config.Name {
			problems = append(problems, fmt.Errorf("%s: parameter %s name %q, declared %q", documented.ID, param.ID, param.Name, config.Name))
		}
		if param.Type != config.Selection.String() {
			problems = append(problems, fmt.Errorf("%s: parameter %s type %q, declared %q", documented.ID, param.ID, param.Type, config.Selection))
		}
		var options []MetadataFunctionOption
		for _, option := range config.Options {
			options = append(options, MetadataFunctionOption{ID: option.ID, Name: option.Name, Default: option.Default})
		}
		if !slices.Equal(param.Options, options) {
			problems = append(problems, fmt.Errorf("%s: parameter %s options %v, declared %v", documented.ID, param.ID, param.Options, options))
		}
	}
	if jobSelectable && !seen[FunctionJobParameter] {
		problems = append(problems, fmt.Errorf("%s: parameter %s not documented, but the framework adds it", documented.ID, FunctionJobParameter))
	}
	for id := range expected {
		if !seen[id] {
			problems = append(problems, fmt.Errorf("%s: parameter %s declared but not documented", documented.ID, id))
		}
	}
	return problems
}

func checkFunctionColumns(documented MetadataFunction, actual ImplementedFunction) []error {
	if actual.RawRequest {
		return nil
	}
	if len(documented.Columns) != len(actual.Columns) {
		return []error{fmt.Errorf("%s: %d columns documented, %d returned", documented.ID, len(documented.Columns), len(actual.Columns))}
	}
	var problems []error
	for i, column := range documented.Columns {
		if column != actual.Columns[i] {
			problems = append(problems, fmt.Errorf("%s: column %d documented as %+v, returned %+v", documented.ID, i, column, actual.Columns[i]))
		}
	}
	return problems
}

// MetadataFunctionsCheck reaches a collector's Functions for
// CheckMetadataDocumentsFunctions. The collector must already hold the data
// each method needs to answer with a 200 table (a collected snapshot, a fixture).
type MetadataFunctionsCheck struct {
	// Context for the handler calls; nil means context.Background().
	Context context.Context
	// ModuleID selects the module by meta.id in a multi-module metadata.yaml.
	ModuleID string
	// Module is the collector's registered module name, used to resolve each
	// method's public Function name; empty skips the name comparison.
	Module string
	// Methods are the Functions the collector registers (Creator.SharedFunctions or InstanceFunctions).
	Methods []funcapi.FunctionConfig
	// Handler declares the parameters and serves the table of every method.
	Handler funcapi.MethodHandler
	// Params supplies the resolved parameters of a method's request; nil means none.
	Params func(method string) funcapi.ResolvedParams
	// JobSelectable is true for per-job collectors, whose Functions get the framework's Instance selector.
	JobSelectable bool
}

// CheckMetadataDocumentsFunctions decodes metadata.yaml, resolves every
// registered method's effective parameters and table the way the framework
// does, and runs CheckMetadataFunctionsMatch.
func CheckMetadataDocumentsFunctions(metadataYAML []byte, check MetadataFunctionsCheck) error {
	module, err := DecodeMetadataModule(metadataYAML, check.ModuleID)
	if err != nil {
		return err
	}
	ctx := check.Context
	if ctx == nil {
		ctx = context.Background()
	}
	implemented := make([]ImplementedFunction, 0, len(check.Methods))
	for _, method := range check.Methods {
		function, err := implementedFunction(ctx, method, check)
		if err != nil {
			return err
		}
		implemented = append(implemented, function)
	}
	return CheckMetadataFunctionsMatch(module.Functions, implemented, check.JobSelectable)
}

// implementedFunction mirrors the framework's parameter resolution: static
// FunctionConfig.RequiredParams, overridden by the handler's MethodParams and
// then by the response's RequiredParams. Raw-request methods use only the
// static parameters and are not asked for a table.
func implementedFunction(ctx context.Context, method funcapi.FunctionConfig, check MetadataFunctionsCheck) (ImplementedFunction, error) {
	function := ImplementedFunction{ID: method.ID, Parameters: method.RequiredParams, RawRequest: method.RawRequest}
	if check.Module != "" {
		function.PublicName = funcapi.FunctionName(check.Module, method)
	}
	if method.RawRequest {
		return function, nil
	}
	dynamic, err := check.Handler.MethodParams(ctx, method.ID)
	if err != nil {
		return function, fmt.Errorf("%s: parameters: %w", method.ID, err)
	}
	function.Parameters = funcapi.MergeParamConfigs(function.Parameters, dynamic)
	var params funcapi.ResolvedParams
	if check.Params != nil {
		params = check.Params(method.ID)
	}
	response := check.Handler.Handle(ctx, method.ID, params)
	if function.Columns, err = FunctionResponseColumns(response); err != nil {
		return function, fmt.Errorf("%s: table: %w", method.ID, err)
	}
	if len(response.RequiredParams) != 0 {
		function.Parameters = funcapi.MergeParamConfigs(function.Parameters, response.RequiredParams)
	}
	return function, nil
}

// AssertMetadataDocumentsFunctions fails the test when
// CheckMetadataDocumentsFunctions reports drift.
func AssertMetadataDocumentsFunctions(t testing.TB, metadataYAML []byte, check MetadataFunctionsCheck) {
	t.Helper()
	if err := CheckMetadataDocumentsFunctions(metadataYAML, check); err != nil {
		t.Fatalf("metadata.yaml functions do not match the implementation:\n%v", err)
	}
}
