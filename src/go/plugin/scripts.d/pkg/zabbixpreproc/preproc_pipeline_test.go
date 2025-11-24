package zabbixpreproc

import "testing"

func TestExecutePipelineCSVToJSONMultiChained(t *testing.T) {
	proc := NewPreprocessor("csv-multi-test")
	steps := []Step{
		{Type: StepTypeCSVToJSONMulti, Params: ",\n\"\n1"},
		{Type: StepTypeJavaScript, Params: `var row = JSON.parse(value); value = row["value"]; return value;`},
	}
	payload := "name,value\nrow1,10\nrow2,20"
	res, err := proc.ExecutePipeline("item1", Value{Data: payload, Type: ValueTypeStr}, steps)
	if err != nil {
		t.Fatalf("ExecutePipeline error: %v", err)
	}
	if len(res.Metrics) != 2 {
		t.Fatalf("expected 2 metrics, got %d", len(res.Metrics))
	}
	if res.Metrics[0].Value != "10" || res.Metrics[1].Value != "20" {
		t.Fatalf("unexpected values: %+v", res.Metrics)
	}
}
