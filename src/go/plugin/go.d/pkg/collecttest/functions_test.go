// SPDX-License-Identifier: GPL-3.0-or-later

package collecttest

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/netdata/netdata/go/plugins/pkg/funcapi"
)

func testFunctionColumns() []MetadataFunctionColumn {
	return []MetadataFunctionColumn{
		{Name: "Key", Type: "string", Visibility: "hidden"},
		{Name: "Name", Type: "string"},
		{Name: "Size", Type: "integer", Unit: "bytes"},
	}
}

func testDocumentedFunction() MetadataFunction {
	return MetadataFunction{
		ID: "inventory",
		Parameters: []MetadataFunctionParameter{{
			ID: "kind", Name: "Kind", Type: "select",
			Options: []MetadataFunctionOption{{ID: "all", Name: "All", Default: true}, {ID: "disk", Name: "Disks"}},
		}},
		Columns: testFunctionColumns(),
	}
}

func testImplementedFunction() ImplementedFunction {
	return ImplementedFunction{
		ID: "inventory",
		Parameters: []funcapi.ParamConfig{{
			ID: "kind", Name: "Kind", Selection: funcapi.ParamSelect,
			Options: []funcapi.ParamOption{{ID: "all", Name: "All", Default: true}, {ID: "disk", Name: "Disks"}},
		}},
		Columns: testFunctionColumns(),
	}
}

func TestFunctionResponseColumns(t *testing.T) {
	columns := []funcapi.ColumnMeta{
		{Name: "Key", Type: funcapi.FieldTypeString, UniqueKey: true},
		{Name: "Name", Type: funcapi.FieldTypeString, Visible: true},
		{Name: "Size", Type: funcapi.FieldTypeInteger, Units: "bytes", Visible: true},
	}
	table := func(status int) *funcapi.FunctionResponse {
		return &funcapi.FunctionResponse{
			Status:  status,
			Columns: funcapi.Columns(columns, func(c funcapi.ColumnMeta) funcapi.ColumnMeta { return c }).BuildColumns(),
		}
	}
	tests := map[string]struct {
		response *funcapi.FunctionResponse
		want     []MetadataFunctionColumn
		wantErr  string
	}{
		"table":         {response: table(200), want: testFunctionColumns()},
		"unset status":  {response: table(0), want: testFunctionColumns()},
		"no response":   {wantErr: "no response"},
		"error status":  {response: funcapi.UnavailableResponse("waiting"), wantErr: "status 503: waiting"},
		"raw response":  {response: &funcapi.FunctionResponse{Status: 200, RawResponse: map[string]any{}}, wantErr: "raw responses"},
		"broken column": {response: &funcapi.FunctionResponse{Status: 200, Columns: map[string]any{"Key": "x"}}, wantErr: `column "Key": not a column definition`},
		"duplicate index": {
			response: &funcapi.FunctionResponse{Status: 200, Columns: map[string]any{
				"Key": map[string]any{"index": 0}, "Name": map[string]any{"index": 0},
			}},
			wantErr: "invalid or duplicate index 0",
		},
	}
	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			got, err := FunctionResponseColumns(test.response)
			if test.wantErr != "" {
				require.ErrorContains(t, err, test.wantErr)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, test.want, got)
		})
	}
}

func TestCheckMetadataFunctionsMatch(t *testing.T) {
	documented := func(mutate func(*MetadataFunction)) []MetadataFunction {
		f := testDocumentedFunction()
		mutate(&f)
		return []MetadataFunction{f}
	}
	jobParam := MetadataFunctionParameter{ID: FunctionJobParameter, Name: "Instance", Type: "select"}
	tests := map[string]struct {
		documented    []MetadataFunction
		implemented   []ImplementedFunction
		jobSelectable bool
		wantErr       string
	}{
		"aligned": {documented: []MetadataFunction{testDocumentedFunction()}, implemented: []ImplementedFunction{testImplementedFunction()}},
		"documented twice": {
			documented:  []MetadataFunction{testDocumentedFunction(), testDocumentedFunction()},
			implemented: []ImplementedFunction{testImplementedFunction()},
			wantErr:     "inventory: documented twice",
		},
		"not implemented": {
			documented: []MetadataFunction{testDocumentedFunction()},
			wantErr:    "inventory: documented but not implemented",
		},
		"not documented": {
			implemented: []ImplementedFunction{testImplementedFunction()},
			wantErr:     "inventory: implemented but not documented",
		},
		"column drift": {
			documented:  documented(func(f *MetadataFunction) { f.Columns[1].Visibility = "hidden" }),
			implemented: []ImplementedFunction{testImplementedFunction()},
			wantErr:     "inventory: column 1 documented as",
		},
		"column count drift": {
			documented:  documented(func(f *MetadataFunction) { f.Columns = f.Columns[:2] }),
			implemented: []ImplementedFunction{testImplementedFunction()},
			wantErr:     "inventory: 2 columns documented, 3 returned",
		},
		"parameter name drift": {
			documented:  documented(func(f *MetadataFunction) { f.Parameters[0].Name = "Type" }),
			implemented: []ImplementedFunction{testImplementedFunction()},
			wantErr:     `inventory: parameter kind name "Type", declared "Kind"`,
		},
		"parameter type drift": {
			documented:  documented(func(f *MetadataFunction) { f.Parameters[0].Type = "multiselect" }),
			implemented: []ImplementedFunction{testImplementedFunction()},
			wantErr:     `inventory: parameter kind type "multiselect", declared "select"`,
		},
		"parameter options drift": {
			documented:  documented(func(f *MetadataFunction) { f.Parameters[0].Options[0].Default = false }),
			implemented: []ImplementedFunction{testImplementedFunction()},
			wantErr:     "inventory: parameter kind options",
		},
		"parameter not declared": {
			documented:  documented(func(f *MetadataFunction) { f.Parameters[0].ID = "type" }),
			implemented: []ImplementedFunction{testImplementedFunction()},
			wantErr:     "inventory: parameter type documented but not declared",
		},
		"parameter not documented": {
			documented:  documented(func(f *MetadataFunction) { f.Parameters = nil }),
			implemented: []ImplementedFunction{testImplementedFunction()},
			wantErr:     "inventory: parameter kind declared but not documented",
		},
		"job selector on a single-instance collector": {
			documented:  documented(func(f *MetadataFunction) { f.Parameters = append(f.Parameters, jobParam) }),
			implemented: []ImplementedFunction{testImplementedFunction()},
			wantErr:     "inventory: parameter __job documented, but the collector is single-instance",
		},
		"job selector documented for a per-job collector": {
			documented:    documented(func(f *MetadataFunction) { f.Parameters = append(f.Parameters, jobParam) }),
			implemented:   []ImplementedFunction{testImplementedFunction()},
			jobSelectable: true,
		},
		"job selector missing for a per-job collector": {
			documented:    []MetadataFunction{testDocumentedFunction()},
			implemented:   []ImplementedFunction{testImplementedFunction()},
			jobSelectable: true,
			wantErr:       "inventory: parameter __job not documented, but the framework adds it",
		},
		"default public name needs no function_name": {
			documented: []MetadataFunction{testDocumentedFunction()},
			implemented: []ImplementedFunction{func() ImplementedFunction {
				f := testImplementedFunction()
				f.PublicName = "app:inventory"
				return f
			}()},
		},
		"overridden public name must be documented": {
			documented: []MetadataFunction{testDocumentedFunction()},
			implemented: []ImplementedFunction{func() ImplementedFunction {
				f := testImplementedFunction()
				f.PublicName = "app-inventory"
				return f
			}()},
			wantErr: `inventory: public name "app-inventory" is not the default, document it as function_name`,
		},
		"function_name drift": {
			documented: documented(func(f *MetadataFunction) { f.FunctionName = "app-inv" }),
			implemented: []ImplementedFunction{func() ImplementedFunction {
				f := testImplementedFunction()
				f.PublicName = "app-inventory"
				return f
			}()},
			wantErr: `inventory: function_name "app-inv", public name "app-inventory"`,
		},
		"raw request skips columns": {
			documented: documented(func(f *MetadataFunction) { f.Columns = nil }),
			implemented: []ImplementedFunction{func() ImplementedFunction {
				f := testImplementedFunction()
				f.Columns, f.RawRequest = nil, true
				return f
			}()},
		},
		"job selector name drift": {
			documented: documented(func(f *MetadataFunction) {
				p := jobParam
				p.Name = "Job"
				f.Parameters = append(f.Parameters, p)
			}),
			implemented:   []ImplementedFunction{testImplementedFunction()},
			jobSelectable: true,
			wantErr:       `inventory: parameter __job name "Job", framework selector is "Instance"`,
		},
		"job selector type drift": {
			documented: documented(func(f *MetadataFunction) {
				p := jobParam
				p.Type = "multiselect"
				f.Parameters = append(f.Parameters, p)
			}),
			implemented:   []ImplementedFunction{testImplementedFunction()},
			jobSelectable: true,
			wantErr:       `inventory: parameter __job type "multiselect", framework selector is "select"`,
		},
	}
	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			err := CheckMetadataFunctionsMatch(test.documented, test.implemented, test.jobSelectable)
			if test.wantErr == "" {
				require.NoError(t, err)
				return
			}
			require.ErrorContains(t, err, test.wantErr)
		})
	}
}

type testFunctionHandler struct {
	params  []funcapi.ParamConfig
	columns []funcapi.ColumnMeta
}

func (h testFunctionHandler) MethodParams(context.Context, string) ([]funcapi.ParamConfig, error) {
	return h.params, nil
}

func (h testFunctionHandler) Handle(context.Context, string, funcapi.ResolvedParams) *funcapi.FunctionResponse {
	return &funcapi.FunctionResponse{
		Status:  200,
		Columns: funcapi.Columns(h.columns, func(c funcapi.ColumnMeta) funcapi.ColumnMeta { return c }).BuildColumns(),
	}
}

func (testFunctionHandler) Cleanup(context.Context) {}

func TestCheckMetadataDocumentsFunctions(t *testing.T) {
	const metadata = `
modules:
  - meta:
      id: collector-app
    functions:
      list:
        - id: inventory
          parameters:
            - id: kind
              name: Kind
              type: select
              options:
                - id: all
                  name: All
                  default: true
                - id: disk
                  name: Disks
          returns:
            columns:
              - name: Key
                type: string
                unit: ""
                visibility: hidden
              - name: Name
                type: string
                unit: ""
              - name: Size
                type: integer
                unit: bytes
`
	handler := testFunctionHandler{
		params: testImplementedFunction().Parameters,
		columns: []funcapi.ColumnMeta{
			{Name: "Key", Type: funcapi.FieldTypeString, UniqueKey: true},
			{Name: "Name", Type: funcapi.FieldTypeString, Visible: true},
			{Name: "Size", Type: funcapi.FieldTypeInteger, Units: "bytes", Visible: true},
		},
	}
	methods := []funcapi.FunctionConfig{{ID: "inventory"}}

	require.NoError(t, CheckMetadataDocumentsFunctions([]byte(metadata), MetadataFunctionsCheck{Methods: methods, Handler: handler}))
	require.NoError(t, CheckMetadataDocumentsFunctions([]byte(metadata), MetadataFunctionsCheck{Module: "app", Methods: methods, Handler: handler}))
	flat := []funcapi.FunctionConfig{{ID: "inventory", FunctionName: "app-inventory"}}
	err := CheckMetadataDocumentsFunctions([]byte(metadata), MetadataFunctionsCheck{Module: "app", Methods: flat, Handler: handler})
	require.ErrorContains(t, err, "document it as function_name")

	// Static FunctionConfig parameters count when the handler declares none, as in the framework.
	static := []funcapi.FunctionConfig{{ID: "inventory", RequiredParams: testImplementedFunction().Parameters}}
	noDynamic := handler
	noDynamic.params = nil
	require.NoError(t, CheckMetadataDocumentsFunctions([]byte(metadata), MetadataFunctionsCheck{Methods: static, Handler: noDynamic}))
	err = CheckMetadataDocumentsFunctions([]byte(metadata), MetadataFunctionsCheck{Methods: methods, Handler: noDynamic})
	require.ErrorContains(t, err, "inventory: parameter kind documented but not declared")

	extra := append(methods, funcapi.FunctionConfig{ID: "logs"})
	err = CheckMetadataDocumentsFunctions([]byte(metadata), MetadataFunctionsCheck{Methods: extra, Handler: handler})
	require.ErrorContains(t, err, "logs: implemented but not documented")
}
