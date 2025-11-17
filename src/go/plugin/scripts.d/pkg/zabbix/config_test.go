package zabbix

import (
	"encoding/json"
	"testing"

	zpre "github.com/netdata/netdata/go/plugins/pkg/zabbixpreproc"
	"gopkg.in/yaml.v3"
)

func TestJobConfigValidate(t *testing.T) {
	cfg := JobConfig{
		Name:       "disk",
		Collection: CollectionConfig{Type: CollectionCommand, Command: "/usr/lib/zabbix/get_disks"},
		LLD:        LLDConfig{InstanceTemplate: "disk_{#DEVNAME}"},
		Pipelines: []PipelineConfig{{
			Name:      "usage",
			Context:   "zabbix.disk.usage",
			Dimension: "used",
			Unit:      "%",
			Steps:     []zpre.Step{{Type: zpre.StepTypeJSONPath, Params: "$.value"}},
		}},
	}
	if err := cfg.Validate(); err != nil {
		t.Fatalf("expected config to be valid: %v", err)
	}
}

func TestJobConfigValidateFails(t *testing.T) {
	cfg := JobConfig{Name: "bad"}
	if err := cfg.Validate(); err == nil {
		t.Fatalf("expected validation failure")
	}
}

func TestJobConfigUpdateEveryValidation(t *testing.T) {
	cfg := JobConfig{
		Name:        "test",
		UpdateEvery: -1,
		Collection:  CollectionConfig{Type: CollectionCommand, Command: "/bin/true"},
		Pipelines: []PipelineConfig{{
			Name:      "value",
			Context:   "ctx",
			Dimension: "dim",
			Unit:      "value",
			Steps:     []zpre.Step{{Type: zpre.StepTypeJSONPath, Params: "$.v"}},
		}},
	}
	if err := cfg.Validate(); err == nil {
		t.Fatalf("expected error for negative update_every")
	}

	cfg.UpdateEvery = 1
	if err := cfg.Validate(); err != nil {
		t.Fatalf("unexpected error for valid update_every: %v", err)
	}

	cfg.UpdateEvery = 0
	if err := cfg.Validate(); err != nil {
		t.Fatalf("unexpected error when update_every omitted: %v", err)
	}
}

func TestPipelineStepStringTypeYAML(t *testing.T) {
	data := `pipelines:
- name: usage
  context: zabbix.fs.usage
  dimension: used
  unit: "%"
  steps:
    - type: jsonpath
      params: $.value
      error_handler: set-value
      error_handler_params: "0"
`
	var wrapper struct {
		Pipelines []PipelineConfig `yaml:"pipelines"`
	}
	if err := yaml.Unmarshal([]byte(data), &wrapper); err != nil {
		t.Fatalf("unmarshal failed: %v", err)
	}
	if len(wrapper.Pipelines) != 1 {
		t.Fatalf("expected 1 pipeline, got %d", len(wrapper.Pipelines))
	}
	steps := wrapper.Pipelines[0].Steps
	if len(steps) != 1 {
		t.Fatalf("expected 1 step, got %d", len(steps))
	}
	step := steps[0]
	if step.Type != zpre.StepTypeJSONPath {
		t.Fatalf("expected jsonpath step, got %v", step.Type)
	}
	if step.ErrorHandler.Action != zpre.ErrorActionSetValue || step.ErrorHandler.Params != "0" {
		t.Fatalf("unexpected error handler: %+v", step.ErrorHandler)
	}
}

func TestPipelineStepNumericJSON(t *testing.T) {
	payload := []byte(`{"pipelines":[{"name":"usage","context":"ctx","dimension":"dim","unit":"%","steps":[{"type":12,"params":"$.value"}]}]}`)
	var wrapper struct {
		Pipelines []PipelineConfig `json:"pipelines"`
	}
	if err := json.Unmarshal(payload, &wrapper); err != nil {
		t.Fatalf("json unmarshal failed: %v", err)
	}
	step := wrapper.Pipelines[0].Steps[0]
	if step.Type != zpre.StepTypeJSONPath {
		t.Fatalf("expected numeric type to map to jsonpath, got %v", step.Type)
	}
}

func TestParseErrorHandlerInvalidAction(t *testing.T) {
	_, err := parseErrorHandler("bogus", "")
	if err == nil {
		t.Fatalf("expected error for unknown handler")
	}

	if _, err := parseErrorHandler("set-value", ""); err == nil {
		t.Fatalf("expected error when params missing for set-value")
	}
	if _, err := parseErrorHandler("set-error", ""); err == nil {
		t.Fatalf("expected error when params missing for set-error")
	}
	if h, err := parseErrorHandler("set-error", "bad"); err != nil || h.Params != "bad" {
		t.Fatalf("expected valid handler, err=%v handler=%+v", err, h)
	}
}
