// SPDX-License-Identifier: GPL-3.0-or-later

package zabbix

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/netdata/netdata/go/plugins/pkg/confopt"
	zpre "github.com/netdata/netdata/go/plugins/plugin/scripts.d/pkg/zabbixpreproc"
)

// CollectionType enumerates supported Zabbix collection methods.
type CollectionType string

const (
	CollectionCommand CollectionType = "command"
	CollectionHTTP    CollectionType = "http"
	CollectionSNMP    CollectionType = "snmp"
)

var stepTypeTokens = map[string]zpre.StepType{
	"multiplier":               zpre.StepTypeMultiplier,
	"rtrim":                    zpre.StepTypeRTrim,
	"ltrim":                    zpre.StepTypeLTrim,
	"trim":                     zpre.StepTypeTrim,
	"regex_substitution":       zpre.StepTypeRegexSubstitution,
	"bool_to_decimal":          zpre.StepTypeBool2Dec,
	"octal_to_decimal":         zpre.StepTypeOct2Dec,
	"hex_to_decimal":           zpre.StepTypeHex2Dec,
	"delta_value":              zpre.StepTypeDeltaValue,
	"delta_speed":              zpre.StepTypeDeltaSpeed,
	"xpath":                    zpre.StepTypeXPath,
	"jsonpath":                 zpre.StepTypeJSONPath,
	"validate_range":           zpre.StepTypeValidateRange,
	"validate_regex":           zpre.StepTypeValidateRegex,
	"validate_not_regex":       zpre.StepTypeValidateNotRegex,
	"error_field_json":         zpre.StepTypeErrorFieldJSON,
	"error_field_xml":          zpre.StepTypeErrorFieldXML,
	"error_field_regex":        zpre.StepTypeErrorFieldRegex,
	"throttle_value":           zpre.StepTypeThrottleValue,
	"throttle_timed_value":     zpre.StepTypeThrottleTimedValue,
	"javascript":               zpre.StepTypeJavaScript,
	"prometheus_pattern":       zpre.StepTypePrometheusPattern,
	"prometheus_to_json":       zpre.StepTypePrometheusToJSON,
	"csv_to_json":              zpre.StepTypeCSVToJSON,
	"string_replace":           zpre.StepTypeStringReplace,
	"validate_not_supported":   zpre.StepTypeValidateNotSupported,
	"xml_to_json":              zpre.StepTypeXMLToJSON,
	"snmp_walk_value":          zpre.StepTypeSNMPWalkValue,
	"snmp_walk_to_json":        zpre.StepTypeSNMPWalkToJSON,
	"snmp_get_value":           zpre.StepTypeSNMPGetValue,
	"prometheus_to_json_multi": zpre.StepTypePrometheusToJSONMulti,
	"jsonpath_multi":           zpre.StepTypeJSONPathMulti,
	"snmp_walk_to_json_multi":  zpre.StepTypeSNMPWalkToJSONMulti,
	"csv_to_json_multi":        zpre.StepTypeCSVToJSONMulti,
}

var errorHandlerTokens = map[string]zpre.ErrorAction{
	"":          zpre.ErrorActionDefault,
	"default":   zpre.ErrorActionDefault,
	"discard":   zpre.ErrorActionDiscard,
	"set-value": zpre.ErrorActionSetValue,
	"set-error": zpre.ErrorActionSetError,
}

// CollectionConfig describes how raw data is fetched before preprocessing.
type CollectionConfig struct {
	Type        CollectionType    `yaml:"type" json:"type"`
	Command     string            `yaml:"command,omitempty" json:"command,omitempty"`
	Args        []string          `yaml:"args,omitempty" json:"args,omitempty"`
	Timeout     confopt.Duration  `yaml:"timeout,omitempty" json:"timeout,omitempty"`
	Environment map[string]string `yaml:"environment,omitempty" json:"environment,omitempty"`
	HTTP        HTTPConfig        `yaml:"http,omitempty" json:"http,omitempty"`
	SNMP        SNMPConfig        `yaml:"snmp,omitempty" json:"snmp,omitempty"`
	Headers     map[string]string `yaml:"headers,omitempty" json:"headers,omitempty"` // legacy alias for HTTP headers
	Body        string            `yaml:"body,omitempty" json:"body,omitempty"`
	Method      string            `yaml:"method,omitempty" json:"method,omitempty"` // legacy alias for HTTP method
	URL         string            `yaml:"url,omitempty" json:"url,omitempty"`
}

// HTTPConfig captures HTTP(S) collection settings.
type HTTPConfig struct {
	URL      string            `yaml:"url" json:"url"`
	Method   string            `yaml:"method,omitempty" json:"method,omitempty"`
	Headers  map[string]string `yaml:"headers,omitempty" json:"headers,omitempty"`
	Body     string            `yaml:"body,omitempty" json:"body,omitempty"`
	TLS      bool              `yaml:"tls,omitempty" json:"tls,omitempty"`
	Username string            `yaml:"username,omitempty" json:"username,omitempty"`
	Password string            `yaml:"password,omitempty" json:"password,omitempty"`
}

// SNMPConfig captures SNMP collection settings.
type SNMPConfig struct {
	Target    string `yaml:"target" json:"target"`
	Community string `yaml:"community,omitempty" json:"community,omitempty"`
	Version   string `yaml:"version,omitempty" json:"version,omitempty"`
	User      string `yaml:"user,omitempty" json:"user,omitempty"`
	AuthPass  string `yaml:"auth_password,omitempty" json:"auth_password,omitempty"`
	PrivPass  string `yaml:"privacy_password,omitempty" json:"privacy_password,omitempty"`
	AuthProto string `yaml:"auth_protocol,omitempty" json:"auth_protocol,omitempty"`
	PrivProto string `yaml:"privacy_protocol,omitempty" json:"privacy_protocol,omitempty"`
	Context   string `yaml:"context,omitempty" json:"context,omitempty"`
	OID       string `yaml:"oid" json:"oid"`
}

// LLDConfig describes the discovery pipeline.
type LLDConfig struct {
	Steps            []zpre.Step       `yaml:"steps" json:"steps"`
	Interval         confopt.Duration  `yaml:"discovery_interval,omitempty" json:"discovery_interval,omitempty"`
	InstanceTemplate string            `yaml:"instance_id_template" json:"instance_id_template"`
	Family           string            `yaml:"family" json:"family"`
	Labels           map[string]string `yaml:"labels,omitempty" json:"labels,omitempty"`
	MaxMissing       int               `yaml:"max_missing,omitempty" json:"max_missing,omitempty"`
}

type lldConfigDTO struct {
	Steps            []rawStep         `yaml:"steps" json:"steps"`
	Interval         confopt.Duration  `yaml:"discovery_interval,omitempty" json:"discovery_interval,omitempty"`
	InstanceTemplate string            `yaml:"instance_id_template" json:"instance_id_template"`
	Family           string            `yaml:"family" json:"family"`
	Labels           map[string]string `yaml:"labels,omitempty" json:"labels,omitempty"`
	MaxMissing       int               `yaml:"max_missing,omitempty" json:"max_missing,omitempty"`
}

func (l *LLDConfig) fromDTO(dto lldConfigDTO) error {
	steps, err := decodeSteps(dto.Steps)
	if err != nil {
		return err
	}
	l.Steps = steps
	l.Interval = dto.Interval
	l.InstanceTemplate = dto.InstanceTemplate
	l.Family = dto.Family
	l.Labels = dto.Labels
	l.MaxMissing = dto.MaxMissing
	return nil
}

func (l *LLDConfig) UnmarshalYAML(unmarshal func(any) error) error {
	var dto lldConfigDTO
	if err := unmarshal(&dto); err != nil {
		return err
	}
	return l.fromDTO(dto)
}

func (l *LLDConfig) UnmarshalJSON(data []byte) error {
	var dto lldConfigDTO
	if err := json.Unmarshal(data, &dto); err != nil {
		return err
	}
	return l.fromDTO(dto)
}

// PipelineConfig defines a dependent pipeline that produces a single metric per instance.
type PipelineConfig struct {
	Name      string      `yaml:"name" json:"name"`
	Context   string      `yaml:"context" json:"context"`
	Title     string      `yaml:"title,omitempty" json:"title,omitempty"`
	Dimension string      `yaml:"dimension" json:"dimension"`
	Unit      string      `yaml:"unit" json:"unit"`
	Family    string      `yaml:"family,omitempty" json:"family,omitempty"`
	ChartType string      `yaml:"chart_type,omitempty" json:"chart_type,omitempty"`
	DataType  string      `yaml:"data_type,omitempty" json:"data_type,omitempty"`
	Precision int         `yaml:"precision,omitempty" json:"precision,omitempty"`
	Algorithm string      `yaml:"algorithm,omitempty" json:"algorithm,omitempty"`
	Steps     []zpre.Step `yaml:"steps" json:"steps"`
}

type pipelineConfigDTO struct {
	Name      string    `yaml:"name" json:"name"`
	Context   string    `yaml:"context" json:"context"`
	Title     string    `yaml:"title,omitempty" json:"title,omitempty"`
	Dimension string    `yaml:"dimension" json:"dimension"`
	Unit      string    `yaml:"unit" json:"unit"`
	Family    string    `yaml:"family,omitempty" json:"family,omitempty"`
	ChartType string    `yaml:"chart_type,omitempty" json:"chart_type,omitempty"`
	DataType  string    `yaml:"data_type,omitempty" json:"data_type,omitempty"`
	Precision int       `yaml:"precision,omitempty" json:"precision,omitempty"`
	Algorithm string    `yaml:"algorithm,omitempty" json:"algorithm,omitempty"`
	Steps     []rawStep `yaml:"steps" json:"steps"`
}

func (pc *PipelineConfig) fromDTO(dto pipelineConfigDTO) error {
	pc.Name = dto.Name
	pc.Context = dto.Context
	pc.Title = dto.Title
	pc.Dimension = dto.Dimension
	pc.Unit = dto.Unit
	pc.Family = dto.Family
	pc.ChartType = dto.ChartType
	pc.DataType = dto.DataType
	pc.Precision = dto.Precision
	pc.Algorithm = dto.Algorithm
	steps, err := decodeSteps(dto.Steps)
	if err != nil {
		return err
	}
	pc.Steps = steps
	return nil
}

func (pc *PipelineConfig) UnmarshalYAML(unmarshal func(any) error) error {
	var dto pipelineConfigDTO
	if err := unmarshal(&dto); err != nil {
		return err
	}
	return pc.fromDTO(dto)
}

func (pc *PipelineConfig) UnmarshalJSON(data []byte) error {
	var dto pipelineConfigDTO
	if err := json.Unmarshal(data, &dto); err != nil {
		return err
	}
	return pc.fromDTO(dto)
}

// JobConfig represents the full user configuration for a Zabbix job.
type JobConfig struct {
	Name        string            `yaml:"name" json:"name"`
	Scheduler   string            `yaml:"scheduler,omitempty" json:"scheduler,omitempty"`
	Vnode       string            `yaml:"vnode,omitempty" json:"vnode,omitempty"`
	UpdateEvery int               `yaml:"update_every,omitempty" json:"update_every,omitempty"`
	UserMacros  map[string]string `yaml:"user_macros,omitempty" json:"user_macros,omitempty"`
	Collection  CollectionConfig  `yaml:"collection" json:"collection"`
	LLD         LLDConfig         `yaml:"lld,omitempty" json:"lld,omitempty"`
	Pipelines   []PipelineConfig  `yaml:"dependent_pipelines" json:"dependent_pipelines"`
	Notes       string            `yaml:"notes,omitempty" json:"notes,omitempty"`
}

const defaultUpdateEverySeconds = 60

type rawStep struct {
	Type              any    `yaml:"type" json:"type"`
	Params            string `yaml:"params,omitempty" json:"params,omitempty"`
	ErrorHandler      string `yaml:"error_handler,omitempty" json:"error_handler,omitempty"`
	ErrorHandlerValue string `yaml:"error_handler_params,omitempty" json:"error_handler_params,omitempty"`
}

func decodeSteps(raw []rawStep) ([]zpre.Step, error) {
	if len(raw) == 0 {
		return nil, nil
	}
	steps := make([]zpre.Step, len(raw))
	for i, r := range raw {
		step, err := r.toZabbixStep()
		if err != nil {
			return nil, fmt.Errorf("step[%d]: %w", i, err)
		}
		steps[i] = step
	}
	return steps, nil
}

func (rs rawStep) toZabbixStep() (zpre.Step, error) {
	st, err := parseStepType(rs.Type)
	if err != nil {
		return zpre.Step{}, err
	}
	handler, err := parseErrorHandler(rs.ErrorHandler, rs.ErrorHandlerValue)
	if err != nil {
		return zpre.Step{}, err
	}
	return zpre.Step{
		Type:         st,
		Params:       rs.Params,
		ErrorHandler: handler,
	}, nil
}

func parseStepType(value any) (zpre.StepType, error) {
	switch v := value.(type) {
	case string:
		key := strings.ToLower(strings.TrimSpace(v))
		if key == "" {
			return 0, fmt.Errorf("step type is required")
		}
		if st, ok := stepTypeTokens[key]; ok {
			return st, nil
		}
		if n, err := strconv.Atoi(key); err == nil {
			return zpre.StepType(n), nil
		}
		return 0, fmt.Errorf("unknown step type %q", v)
	case int:
		return zpre.StepType(v), nil
	case int64:
		return zpre.StepType(v), nil
	case float64:
		return zpre.StepType(int(v)), nil
	case json.Number:
		i, err := v.Int64()
		if err != nil {
			return 0, err
		}
		return zpre.StepType(i), nil
	case nil:
		return 0, fmt.Errorf("step type is required")
	default:
		return 0, fmt.Errorf("unsupported step type %T", v)
	}
}

func parseErrorHandler(action, params string) (zpre.ErrorHandler, error) {
	action = strings.ToLower(strings.TrimSpace(action))
	act, ok := errorHandlerTokens[action]
	if !ok {
		return zpre.ErrorHandler{}, fmt.Errorf("unknown error handler %q", action)
	}
	switch act {
	case zpre.ErrorActionSetValue, zpre.ErrorActionSetError:
		if strings.TrimSpace(params) == "" {
			return zpre.ErrorHandler{}, fmt.Errorf("error_handler_params required when error_handler=%q", action)
		}
		return zpre.ErrorHandler{Action: act, Params: params}, nil
	default:
		return zpre.ErrorHandler{Action: act}, nil
	}
}

// Validate ensures the job definition is usable.
func (cfg *JobConfig) Validate() error {
	if strings.TrimSpace(cfg.Name) == "" {
		return fmt.Errorf("job name is required")
	}
	if strings.TrimSpace(cfg.Collection.Type.String()) == "" {
		return fmt.Errorf("job '%s': collection.type is required", cfg.Name)
	}
	if err := cfg.Collection.validate(); err != nil {
		return fmt.Errorf("job '%s': %w", cfg.Name, err)
	}
	if len(cfg.Pipelines) == 0 {
		return fmt.Errorf("job '%s': at least one dependent pipeline is required", cfg.Name)
	}
	for i := range cfg.Pipelines {
		if err := cfg.Pipelines[i].validate(); err != nil {
			return fmt.Errorf("job '%s': pipeline[%d]: %w", cfg.Name, i, err)
		}
	}
	if cfg.UpdateEvery < 0 {
		return fmt.Errorf("job '%s': update_every must be >= 1 second when provided", cfg.Name)
	}
	if cfg.LLD.InstanceTemplate == "" && cfg.hasTemplateLabels() {
		return fmt.Errorf("job '%s': lld.instance_id_template is required when labels reference macros", cfg.Name)
	}
	return nil
}

func (c CollectionConfig) validate() error {
	switch c.Type {
	case CollectionCommand:
		if strings.TrimSpace(c.Command) == "" {
			return fmt.Errorf("collection.command is required for command type")
		}
	case CollectionHTTP:
		if strings.TrimSpace(c.HTTP.URL) == "" && strings.TrimSpace(c.URL) == "" {
			return fmt.Errorf("http.url is required")
		}
	case CollectionSNMP:
		if strings.TrimSpace(c.SNMP.Target) == "" || strings.TrimSpace(c.SNMP.OID) == "" {
			return fmt.Errorf("snmp.target and snmp.oid are required")
		}
	default:
		return fmt.Errorf("unsupported collection type %q", c.Type)
	}
	return nil
}

func (c CollectionType) String() string { return string(c) }

func (cfg *JobConfig) hasTemplateLabels() bool {
	for _, v := range cfg.LLD.Labels {
		if strings.Contains(v, "{#") {
			return true
		}
	}
	return false
}

func (p *PipelineConfig) validate() error {
	if p.Name == "" {
		return fmt.Errorf("name is required")
	}
	if p.Context == "" {
		return fmt.Errorf("context is required")
	}
	if p.Dimension == "" {
		return fmt.Errorf("dimension is required")
	}
	if strings.TrimSpace(p.Unit) == "" {
		return fmt.Errorf("unit is required")
	}
	if len(p.Steps) == 0 {
		return fmt.Errorf("at least one preprocessing step is required")
	}
	return nil
}

// Interval returns the discovery interval or zero for per-collection execution.
func (l LLDConfig) IntervalDuration() time.Duration {
	return time.Duration(l.Interval)
}

// Timeout returns the collection timeout.
func (c CollectionConfig) TimeoutDuration() time.Duration {
	if c.Timeout <= 0 {
		return 0
	}
	return time.Duration(c.Timeout)
}

// IntervalDuration returns the scheduling interval for the job.
func (cfg JobConfig) IntervalDuration() time.Duration {
	if cfg.UpdateEvery >= 1 {
		return time.Duration(cfg.UpdateEvery) * time.Second
	}
	return time.Duration(defaultUpdateEverySeconds) * time.Second
}
