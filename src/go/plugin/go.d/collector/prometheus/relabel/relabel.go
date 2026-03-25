// SPDX-License-Identifier: GPL-3.0-or-later

package relabel

import (
	"crypto/md5"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"

	"github.com/grafana/regexp"
	commonmodel "github.com/prometheus/common/model"
	"github.com/prometheus/prometheus/model/labels"

	"github.com/netdata/netdata/go/plugins/pkg/prometheus/promscrapemodel"
)

var (
	relabelTargetLegacy = regexp.MustCompile(`^(?:(?:[a-zA-Z_]|\$(?:\{\w+\}|\w+))+\w*)+$`)

	DefaultConfig = Config{
		Action:      Replace,
		Separator:   ";",
		Regex:       MustNewRegexp("(.*)"),
		Replacement: "$1",
	}
)

const defaultNameValidationScheme = commonmodel.LegacyValidation

type Action string

const (
	Replace   Action = "replace"
	Keep      Action = "keep"
	Drop      Action = "drop"
	KeepEqual Action = "keepequal"
	DropEqual Action = "dropequal"
	HashMod   Action = "hashmod"
	LabelMap  Action = "labelmap"
	LabelDrop Action = "labeldrop"
	LabelKeep Action = "labelkeep"
	Lowercase Action = "lowercase"
	Uppercase Action = "uppercase"
)

type Config struct {
	SourceLabels []string                     `yaml:"source_labels,flow,omitempty" json:"source_labels,omitempty"`
	Separator    string                       `yaml:"separator,omitempty" json:"separator,omitempty"`
	Regex        Regexp                       `yaml:"regex,omitempty" json:"regex,omitempty"`
	Modulus      uint64                       `yaml:"modulus,omitempty" json:"modulus,omitempty"`
	TargetLabel  string                       `yaml:"target_label,omitempty" json:"target_label,omitempty"`
	Replacement  string                       `yaml:"replacement,omitempty" json:"replacement,omitempty"`
	Action       Action                       `yaml:"action,omitempty" json:"action,omitempty"`
	NameScheme   commonmodel.ValidationScheme `yaml:"-" json:"-"`

	separatorSet    bool
	replacementSet  bool
	sourceLabelsSet bool
}

type Regexp struct {
	*regexp.Regexp
}

type Processor struct {
	cfgs []Config

	builder *labels.Builder
	join    strings.Builder
	buf     []byte

	currentName       string
	currentNameScheme commonmodel.ValidationScheme
	labelsChanged     bool
	nameChanged       bool
}

func (a *Action) UnmarshalYAML(unmarshal func(any) error) error {
	var s string
	if err := unmarshal(&s); err != nil {
		return err
	}
	act, err := parseAction(s)
	if err != nil {
		return err
	}
	*a = act
	return nil
}

func (a *Action) UnmarshalJSON(b []byte) error {
	var s string
	if err := json.Unmarshal(b, &s); err != nil {
		return err
	}
	act, err := parseAction(s)
	if err != nil {
		return err
	}
	*a = act
	return nil
}

func (c *Config) UnmarshalYAML(unmarshal func(any) error) error {
	type rawConfig struct {
		SourceLabels *[]string `yaml:"source_labels,flow,omitempty"`
		Separator    *string   `yaml:"separator,omitempty"`
		Regex        *Regexp   `yaml:"regex,omitempty"`
		Modulus      *uint64   `yaml:"modulus,omitempty"`
		TargetLabel  *string   `yaml:"target_label,omitempty"`
		Replacement  *string   `yaml:"replacement,omitempty"`
		Action       *Action   `yaml:"action,omitempty"`
	}

	*c = DefaultConfig
	c.NameScheme = defaultNameValidationScheme

	var raw rawConfig
	if err := unmarshal(&raw); err != nil {
		return err
	}

	if raw.SourceLabels != nil {
		c.SourceLabels = append(c.SourceLabels[:0], (*raw.SourceLabels)...)
		c.sourceLabelsSet = true
	}
	if raw.Separator != nil {
		c.Separator = *raw.Separator
		c.separatorSet = true
	}
	if raw.Regex != nil {
		c.Regex = *raw.Regex
	}
	if raw.Modulus != nil {
		c.Modulus = *raw.Modulus
	}
	if raw.TargetLabel != nil {
		c.TargetLabel = *raw.TargetLabel
	}
	if raw.Replacement != nil {
		c.Replacement = *raw.Replacement
		c.replacementSet = true
	}
	if raw.Action != nil {
		c.Action = *raw.Action
	}

	return nil
}

func (c *Config) UnmarshalJSON(b []byte) error {
	type rawConfig struct {
		SourceLabels *[]string `json:"source_labels,omitempty"`
		Separator    *string   `json:"separator,omitempty"`
		Regex        *Regexp   `json:"regex,omitempty"`
		Modulus      *uint64   `json:"modulus,omitempty"`
		TargetLabel  *string   `json:"target_label,omitempty"`
		Replacement  *string   `json:"replacement,omitempty"`
		Action       *Action   `json:"action,omitempty"`
	}

	*c = DefaultConfig
	c.NameScheme = defaultNameValidationScheme

	var raw rawConfig
	if err := json.Unmarshal(b, &raw); err != nil {
		return err
	}

	if raw.SourceLabels != nil {
		c.SourceLabels = append(c.SourceLabels[:0], (*raw.SourceLabels)...)
		c.sourceLabelsSet = true
	}
	if raw.Separator != nil {
		c.Separator = *raw.Separator
		c.separatorSet = true
	}
	if raw.Regex != nil {
		c.Regex = *raw.Regex
	}
	if raw.Modulus != nil {
		c.Modulus = *raw.Modulus
	}
	if raw.TargetLabel != nil {
		c.TargetLabel = *raw.TargetLabel
	}
	if raw.Replacement != nil {
		c.Replacement = *raw.Replacement
		c.replacementSet = true
	}
	if raw.Action != nil {
		c.Action = *raw.Action
	}

	return nil
}

func NewRegexp(s string) (Regexp, error) {
	re, err := regexp.Compile("^(?s:" + s + ")$")
	return Regexp{Regexp: re}, err
}

func MustNewRegexp(s string) Regexp {
	re, err := NewRegexp(s)
	if err != nil {
		panic(err)
	}
	return re
}

func (re *Regexp) UnmarshalYAML(unmarshal func(any) error) error {
	var s string
	if err := unmarshal(&s); err != nil {
		return err
	}
	r, err := NewRegexp(s)
	if err != nil {
		return err
	}
	*re = r
	return nil
}

func (re Regexp) MarshalYAML() (any, error) {
	if re.String() != "" {
		return re.String(), nil
	}
	return nil, nil
}

func (re *Regexp) UnmarshalJSON(b []byte) error {
	var s string
	if err := json.Unmarshal(b, &s); err != nil {
		return err
	}
	r, err := NewRegexp(s)
	if err != nil {
		return err
	}
	*re = r
	return nil
}

func (re Regexp) MarshalJSON() ([]byte, error) {
	return json.Marshal(re.String())
}

func (re Regexp) IsZero() bool {
	return re.Regexp == DefaultConfig.Regex.Regexp
}

func (re Regexp) String() string {
	if re.Regexp == nil {
		return ""
	}

	str := re.Regexp.String()
	return str[5 : len(str)-2]
}

func New(cfgs []Config) (*Processor, error) {
	compiled := make([]Config, 0, len(cfgs))
	for i, cfg := range cfgs {
		cfg = withDefaults(cfg)
		if err := cfg.Validate(); err != nil {
			return nil, fmt.Errorf("rule %d: %w", i, err)
		}
		compiled = append(compiled, cfg)
	}

	return &Processor{
		cfgs:    compiled,
		builder: labels.NewBuilder(nil),
	}, nil
}

func (c Config) Validate() error {
	c = withDefaults(c)

	if _, err := parseAction(string(c.Action)); err != nil {
		return err
	}

	if err := validateNameScheme(c.NameScheme); err != nil {
		return err
	}

	scheme := c.NameScheme
	if scheme == commonmodel.UnsetValidation {
		scheme = defaultNameValidationScheme
	}

	if c.Modulus == 0 && c.Action == HashMod {
		return errors.New("relabel configuration for hashmod requires non-zero modulus")
	}
	if needsTargetLabel(c.Action) && c.TargetLabel == "" {
		return fmt.Errorf("relabel configuration for %s action requires 'target_label' value", c.Action)
	}

	if c.Action == Replace && !varInRegexTemplate(c.TargetLabel) && !scheme.IsValidLabelName(c.TargetLabel) {
		return fmt.Errorf("%q is invalid 'target_label' for %s action", c.TargetLabel, c.Action)
	}
	if c.Action == Replace && varInRegexTemplate(c.TargetLabel) && !isValidLabelNameWithRegexVar(c.TargetLabel, scheme) {
		return fmt.Errorf("%q is invalid 'target_label' for %s action", c.TargetLabel, c.Action)
	}
	if (c.Action == Lowercase || c.Action == Uppercase || c.Action == KeepEqual || c.Action == DropEqual) &&
		!scheme.IsValidLabelName(c.TargetLabel) {
		return fmt.Errorf("%q is invalid 'target_label' for %s action", c.TargetLabel, c.Action)
	}
	if (c.Action == Lowercase || c.Action == Uppercase || c.Action == KeepEqual || c.Action == DropEqual) &&
		c.Replacement != DefaultConfig.Replacement {
		return fmt.Errorf("'replacement' can not be set for %s action", c.Action)
	}
	if c.Action == LabelMap && !isValidLabelNameWithRegexVar(c.Replacement, scheme) {
		return fmt.Errorf("%q is invalid 'replacement' for %s action", c.Replacement, c.Action)
	}
	if c.Action == HashMod && !scheme.IsValidLabelName(c.TargetLabel) {
		return fmt.Errorf("%q is invalid 'target_label' for %s action", c.TargetLabel, c.Action)
	}
	if c.Action == DropEqual || c.Action == KeepEqual {
		if c.Regex.String() != DefaultConfig.Regex.String() ||
			c.Modulus != DefaultConfig.Modulus ||
			c.Separator != DefaultConfig.Separator ||
			c.Replacement != DefaultConfig.Replacement {
			return fmt.Errorf("%s action requires only 'source_labels' and `target_label`, and no other fields", c.Action)
		}
	}
	if c.Action == LabelDrop || c.Action == LabelKeep {
		if c.sourceLabelsSet ||
			len(c.SourceLabels) > 0 ||
			c.TargetLabel != DefaultConfig.TargetLabel ||
			c.Modulus != DefaultConfig.Modulus ||
			c.Separator != DefaultConfig.Separator ||
			c.Replacement != DefaultConfig.Replacement {
			return fmt.Errorf("%s action requires only 'regex', and no other fields", c.Action)
		}
	}

	return nil
}

func (p *Processor) Apply(sample promscrapemodel.Sample) (promscrapemodel.Sample, bool) {
	if len(p.cfgs) == 0 {
		return sample, true
	}

	p.builder.Reset(sample.Labels)
	p.currentName = sample.Name
	p.currentNameScheme = defaultNameValidationScheme
	p.labelsChanged = false
	p.nameChanged = false

	for i := range p.cfgs {
		if !p.applyConfig(&p.cfgs[i]) {
			return promscrapemodel.Sample{}, false
		}
	}

	if !p.currentNameScheme.IsValidMetricName(p.currentName) {
		return promscrapemodel.Sample{}, false
	}

	if !p.nameChanged && !p.labelsChanged {
		return sample, true
	}

	sample.Name = p.currentName
	if p.labelsChanged {
		sample.Labels = p.builder.Labels()
	}
	return sample, true
}

func (p *Processor) applyConfig(cfg *Config) bool {
	val := p.joinSourceLabels(cfg.SourceLabels, cfg.Separator)

	switch cfg.Action {
	case Drop:
		return !cfg.Regex.MatchString(val)
	case Keep:
		return cfg.Regex.MatchString(val)
	case DropEqual:
		return p.getLabel(cfg.TargetLabel) != val
	case KeepEqual:
		return p.getLabel(cfg.TargetLabel) == val
	case Replace:
		return p.applyReplace(cfg, val)
	case Lowercase:
		p.setLabel(cfg.TargetLabel, strings.ToLower(val), cfg.NameScheme)
	case Uppercase:
		p.setLabel(cfg.TargetLabel, strings.ToUpper(val), cfg.NameScheme)
	case HashMod:
		hash := md5.Sum([]byte(val))
		mod := binary.BigEndian.Uint64(hash[8:]) % cfg.Modulus
		p.setLabel(cfg.TargetLabel, strconv.FormatUint(mod, 10), cfg.NameScheme)
	case LabelMap:
		p.rangeLabels(func(l labels.Label) {
			if cfg.Regex.MatchString(l.Name) {
				p.setLabel(cfg.Regex.ReplaceAllString(l.Name, cfg.Replacement), l.Value, cfg.NameScheme)
			}
		})
	case LabelDrop:
		p.rangeLabels(func(l labels.Label) {
			if cfg.Regex.MatchString(l.Name) {
				p.delLabel(l.Name)
			}
		})
	case LabelKeep:
		p.rangeLabels(func(l labels.Label) {
			if !cfg.Regex.MatchString(l.Name) {
				p.delLabel(l.Name)
			}
		})
	default:
		panic(fmt.Errorf("unknown relabel action %q", cfg.Action))
	}

	return true
}

func (p *Processor) applyReplace(cfg *Config, val string) bool {
	if val == "" &&
		cfg.Regex.String() == DefaultConfig.Regex.String() &&
		!varInRegexTemplate(cfg.TargetLabel) &&
		!varInRegexTemplate(cfg.Replacement) {
		p.setLabel(cfg.TargetLabel, cfg.Replacement, cfg.NameScheme)
		return true
	}

	indexes := cfg.Regex.FindStringSubmatchIndex(val)
	if indexes == nil {
		return true
	}

	p.buf = cfg.Regex.ExpandString(p.buf[:0], cfg.TargetLabel, val, indexes)
	target := string(p.buf)
	if !cfg.NameScheme.IsValidLabelName(target) {
		return true
	}

	p.buf = cfg.Regex.ExpandString(p.buf[:0], cfg.Replacement, val, indexes)
	if len(p.buf) == 0 {
		p.delLabel(target)
		return true
	}

	p.setLabel(target, string(p.buf), cfg.NameScheme)
	return true
}

func (p *Processor) joinSourceLabels(sourceLabels []string, separator string) string {
	switch len(sourceLabels) {
	case 0:
		return ""
	case 1:
		return p.getLabel(sourceLabels[0])
	}

	p.join.Reset()
	for i, name := range sourceLabels {
		if i > 0 {
			p.join.WriteString(separator)
		}
		p.join.WriteString(p.getLabel(name))
	}

	return p.join.String()
}

func (p *Processor) getLabel(name string) string {
	if name == commonmodel.MetricNameLabel {
		return p.currentName
	}
	return p.builder.Get(name)
}

func (p *Processor) setLabel(name, value string, scheme commonmodel.ValidationScheme) {
	if name == commonmodel.MetricNameLabel {
		p.currentNameScheme = scheme
		if p.currentName != value {
			p.currentName = value
			p.nameChanged = true
		}
		return
	}

	if current, ok := p.lookupLabel(name); ok && current == value {
		return
	}

	p.builder.Set(name, value)
	p.labelsChanged = true
}

func (p *Processor) delLabel(name string) {
	if name == commonmodel.MetricNameLabel {
		if p.currentName != "" {
			p.currentName = ""
			p.nameChanged = true
		}
		return
	}

	if _, ok := p.lookupLabel(name); !ok {
		return
	}

	p.builder.Del(name)
	p.labelsChanged = true
}

func (p *Processor) rangeLabels(fn func(labels.Label)) {
	if p.currentName != "" {
		fn(labels.Label{Name: commonmodel.MetricNameLabel, Value: p.currentName})
	}
	p.builder.Range(fn)
}

func (p *Processor) lookupLabel(name string) (string, bool) {
	if name == commonmodel.MetricNameLabel {
		if p.currentName == "" {
			return "", false
		}
		return p.currentName, true
	}

	var (
		value string
		ok    bool
	)
	p.builder.Range(func(l labels.Label) {
		if l.Name == name {
			value = l.Value
			ok = true
		}
	})
	return value, ok
}

func parseAction(s string) (Action, error) {
	switch act := Action(strings.ToLower(s)); act {
	case Replace, Keep, Drop, KeepEqual, DropEqual, HashMod, LabelMap, LabelDrop, LabelKeep, Lowercase, Uppercase:
		return act, nil
	default:
		return "", fmt.Errorf("unknown relabel action %q", s)
	}
}

func withDefaults(cfg Config) Config {
	cfg.NameScheme = withNameScheme(cfg.NameScheme)
	if cfg.Action == "" {
		cfg.Action = DefaultConfig.Action
	}
	if !cfg.separatorSet && cfg.Separator == "" {
		cfg.Separator = DefaultConfig.Separator
	}
	if cfg.Regex.Regexp == nil {
		cfg.Regex = DefaultConfig.Regex
	}
	if !cfg.replacementSet && cfg.Replacement == "" {
		cfg.Replacement = DefaultConfig.Replacement
	}
	return cfg
}

func withNameScheme(scheme commonmodel.ValidationScheme) commonmodel.ValidationScheme {
	if scheme == commonmodel.UnsetValidation {
		return defaultNameValidationScheme
	}
	return scheme
}

func validateNameScheme(scheme commonmodel.ValidationScheme) error {
	switch scheme {
	case commonmodel.UnsetValidation, commonmodel.LegacyValidation, commonmodel.UTF8Validation:
		return nil
	default:
		return fmt.Errorf("unknown relabel config name validation method specified, must be either '', 'legacy' or 'utf8', got %s", scheme)
	}
}

func needsTargetLabel(action Action) bool {
	return action == Replace || action == HashMod || action == Lowercase || action == Uppercase || action == KeepEqual || action == DropEqual
}

func isValidLabelNameWithRegexVar(value string, scheme commonmodel.ValidationScheme) bool {
	if scheme == commonmodel.UTF8Validation {
		return scheme.IsValidLabelName(value)
	}
	return relabelTargetLegacy.MatchString(value)
}

func varInRegexTemplate(template string) bool {
	return strings.Contains(template, "$")
}
