// SPDX-License-Identifier: GPL-3.0-or-later

// Package relabel applies Prometheus-compatible metric-relabeling rules to
// scraped samples (the metric name plus labels, including le/quantile) before
// typed-family assembly. It is collector-local to the prometheus collector.
package relabel

import (
	"crypto/md5"
	"encoding/binary"
	"errors"
	"fmt"
	"strconv"
	"strings"

	"github.com/grafana/regexp"
	commonmodel "github.com/prometheus/common/model"
	"github.com/prometheus/prometheus/model/labels"

	prompkg "github.com/netdata/netdata/go/plugins/pkg/prometheus"
)

var (
	relabelTargetLegacy = regexp.MustCompile(`^(?:(?:[a-zA-Z_]|\$(?:\{\w+\}|\w+))+\w*)+$`)

	defaultConfig = Config{
		Action:      Replace,
		Separator:   ";",
		Regex:       MustNewRegexp("(.*)"),
		Replacement: "$1",
	}
)

// defaultNameValidationScheme is the name-validation scheme applied when a rule
// does not set one. It is UTF-8: relabeling may legitimately produce dotted or
// otherwise non-legacy metric and label names, and only an empty name is
// rejected. A rule may still opt into commonmodel.LegacyValidation via
// Config.NameScheme.
const defaultNameValidationScheme = commonmodel.UTF8Validation

// Action is the operation a relabel rule performs on a sample's labels and metric
// name. It mirrors Prometheus's relabel actions; New canonicalizes the value, so it
// is case-insensitive. "Joined value" below means the SourceLabels values joined by
// Separator.
type Action string

const (
	// Replace sets TargetLabel from the regex match of the joined value (Replacement
	// is the template; an empty result deletes the target label).
	Replace Action = "replace"
	// Keep keeps the sample only when Regex matches the joined value.
	Keep Action = "keep"
	// Drop drops the sample when Regex matches the joined value.
	Drop Action = "drop"
	// KeepEqual keeps the sample only when TargetLabel equals the joined value.
	KeepEqual Action = "keepequal"
	// DropEqual drops the sample when TargetLabel equals the joined value.
	DropEqual Action = "dropequal"
	// HashMod sets TargetLabel to the MD5 of the joined value modulo Modulus.
	HashMod Action = "hashmod"
	// LabelMap copies each label whose name matches Regex to a new name from Replacement.
	LabelMap Action = "labelmap"
	// LabelDrop removes every label whose name matches Regex.
	LabelDrop Action = "labeldrop"
	// LabelKeep removes every label whose name does not match Regex.
	LabelKeep Action = "labelkeep"
	// Lowercase sets TargetLabel to the lowercased joined value.
	Lowercase Action = "lowercase"
	// Uppercase sets TargetLabel to the uppercased joined value.
	Uppercase Action = "uppercase"
)

// DropReason explains why Apply dropped a sample, for the caller to log.
type DropReason string

const (
	DropReasonNone              DropReason = ""
	DropReasonDropRuleMatched   DropReason = "drop rule matched"
	DropReasonKeepRuleMismatch  DropReason = "keep rule did not match"
	DropReasonDropEqualMatched  DropReason = "dropequal rule matched"
	DropReasonKeepEqualMismatch DropReason = "keepequal rule did not match"
	DropReasonInvalidMetricName DropReason = "resulting metric name is empty or invalid"
)

// DropInfo is the outcome of Apply. Dropped reports whether the sample was
// dropped; when it was, Reason says why and, for a rule-driven drop, RuleIndex
// and Action identify the rule. RuleIndex is -1 when the drop is not tied to a
// single rule (an invalid final metric name).
type DropInfo struct {
	Reason    DropReason
	RuleIndex int
	Action    Action
}

// Dropped reports whether the sample was dropped.
func (d DropInfo) Dropped() bool { return d.Reason != DropReasonNone }

// DropObserver is called by the SampleTransform returned from NewTransform for
// each dropped sample. Implementations SHOULD log the reason/rule/action but
// MUST NOT log label values (cardinality and privacy).
type DropObserver func(sample prompkg.Sample, drop DropInfo)

// Config is one relabeling rule. Construct it directly using the Action constants
// and exported fields; rules are validated (and the Action canonicalized) by New.
//
// The unexported separatorSet/replacementSet/sourceLabelsSet fields distinguish an
// explicitly-empty field from an unset one (they are read by withDefaults, validate
// and applyReplace). They are settable only within this package; callers outside the
// package cannot express explicit-empty Separator/Replacement/SourceLabels via normal
// struct literals or standard YAML/JSON unmarshaling (unexported fields are ignored),
// so a dedicated config loader must set them when that behavior is needed.
type Config struct {
	SourceLabels []string
	Separator    string
	Regex        Regexp
	Modulus      uint64
	TargetLabel  string
	Replacement  string
	Action       Action
	NameScheme   commonmodel.ValidationScheme

	separatorSet    bool
	replacementSet  bool
	sourceLabelsSet bool
}

// Regexp is a relabel regular expression: a regexp.Regexp compiled fully anchored
// (see NewRegexp). The zero value has no pattern; build one with NewRegexp or
// MustNewRegexp. String returns the original, un-anchored source.
type Regexp struct {
	*regexp.Regexp
	original string // un-anchored source passed to NewRegexp; returned by String
}

// Processor applies an ordered list of rules to samples. It reuses internal
// buffers across calls, so a Processor is single-threaded per scrape and is NOT
// goroutine-safe.
type Processor struct {
	cfgs []Config

	builder  *labels.Builder
	join     strings.Builder
	buf      []byte
	rangeBuf []labels.Label

	currentName       string
	currentNameScheme commonmodel.ValidationScheme
	labelsChanged     bool
	nameChanged       bool
}

// New validates and compiles the rules into a Processor.
func New(cfgs []Config) (*Processor, error) {
	compiled, err := normalizeAndValidateConfigs(cfgs)
	if err != nil {
		return nil, err
	}

	return &Processor{
		cfgs:    compiled,
		builder: labels.NewBuilder(nil),
	}, nil
}

// NewTransform builds a prompkg.SampleTransform from the rules. It returns a nil
// transform when there are no rules, so the scraper keeps its no-transform fast
// path. onDrop, if non-nil, is called for each dropped sample. The returned
// transform closes over a single reusable Processor, so it is NOT goroutine-safe;
// use one transform per scrape goroutine.
func NewTransform(cfgs []Config, onDrop DropObserver) (prompkg.SampleTransform, error) {
	p, err := New(cfgs)
	if err != nil {
		return nil, err
	}
	if len(p.cfgs) == 0 {
		return nil, nil
	}

	return func(s prompkg.Sample) (prompkg.Sample, bool, error) {
		out, drop := p.Apply(s)
		if drop.Dropped() {
			if onDrop != nil {
				onDrop(out, drop)
			}
			return prompkg.Sample{}, false, nil
		}
		return out, true, nil
	}, nil
}

func normalizeAndValidateConfigs(cfgs []Config) ([]Config, error) {
	compiled := make([]Config, 0, len(cfgs))
	for i, cfg := range cfgs {
		cfg = withDefaults(cfg)
		if err := cfg.validate(); err != nil {
			return nil, fmt.Errorf("rule %d: %w", i, err)
		}
		compiled = append(compiled, cfg)
	}
	return compiled, nil
}

func (c Config) validate() error {
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
		c.Replacement != defaultConfig.Replacement {
		return fmt.Errorf("'replacement' can not be set for %s action", c.Action)
	}
	if c.Action == LabelMap && !isValidLabelNameWithRegexVar(c.Replacement, scheme) {
		return fmt.Errorf("%q is invalid 'replacement' for %s action", c.Replacement, c.Action)
	}
	if c.Action == HashMod && !scheme.IsValidLabelName(c.TargetLabel) {
		return fmt.Errorf("%q is invalid 'target_label' for %s action", c.TargetLabel, c.Action)
	}
	if c.Action == DropEqual || c.Action == KeepEqual {
		if c.Regex.String() != defaultConfig.Regex.String() ||
			c.Modulus != defaultConfig.Modulus ||
			c.Separator != defaultConfig.Separator ||
			c.Replacement != defaultConfig.Replacement {
			return fmt.Errorf("%s action requires only 'source_labels' and 'target_label', and no other fields", c.Action)
		}
	}
	if c.Action == LabelDrop || c.Action == LabelKeep {
		if c.sourceLabelsSet ||
			len(c.SourceLabels) > 0 ||
			c.TargetLabel != defaultConfig.TargetLabel ||
			c.Modulus != defaultConfig.Modulus ||
			c.Separator != defaultConfig.Separator ||
			c.Replacement != defaultConfig.Replacement {
			return fmt.Errorf("%s action requires only 'regex', and no other fields", c.Action)
		}
	}

	return nil
}

func NewRegexp(s string) (Regexp, error) {
	re, err := regexp.Compile("^(?s:" + s + ")$")
	return Regexp{Regexp: re, original: s}, err
}

func MustNewRegexp(s string) Regexp {
	re, err := NewRegexp(s)
	if err != nil {
		panic(err)
	}
	return re
}

// String returns the original, un-anchored pattern passed to NewRegexp. It returns
// "" for the zero value or any Regexp not built via NewRegexp, and never inspects the
// compiled form, so it is safe on a Regexp wrapping an arbitrary *regexp.Regexp.
func (re Regexp) String() string {
	return re.original
}

// Apply runs the rules against one sample. It returns the (possibly mutated)
// sample and a DropInfo. When DropInfo.Dropped() is true the sample must be
// discarded; the returned sample is the original (unmutated) so the caller can
// log its name. Value, Kind and FamilyType are passed through unchanged — a
// relabeled sample is never re-typed.
func (p *Processor) Apply(sample prompkg.Sample) (prompkg.Sample, DropInfo) {
	if len(p.cfgs) == 0 {
		return sample, DropInfo{}
	}

	p.builder.Reset(sample.Labels)
	p.currentName = sample.Name
	p.currentNameScheme = defaultNameValidationScheme
	p.labelsChanged = false
	p.nameChanged = false

	for i := range p.cfgs {
		if keep, drop := p.applyConfig(&p.cfgs[i], i); !keep {
			return sample, drop
		}
	}

	if !p.currentNameScheme.IsValidMetricName(p.currentName) {
		return sample, DropInfo{Reason: DropReasonInvalidMetricName, RuleIndex: -1}
	}

	if !p.nameChanged && !p.labelsChanged {
		return sample, DropInfo{}
	}

	sample.Name = p.currentName
	if p.labelsChanged {
		sample.Labels = p.builder.Labels()
	}
	return sample, DropInfo{}
}

func (p *Processor) applyConfig(cfg *Config, idx int) (bool, DropInfo) {
	val := p.joinSourceLabels(cfg.SourceLabels, cfg.Separator)

	switch cfg.Action {
	case Drop:
		if cfg.Regex.MatchString(val) {
			return false, DropInfo{Reason: DropReasonDropRuleMatched, RuleIndex: idx, Action: Drop}
		}
	case Keep:
		if !cfg.Regex.MatchString(val) {
			return false, DropInfo{Reason: DropReasonKeepRuleMismatch, RuleIndex: idx, Action: Keep}
		}
	case DropEqual:
		if p.getLabel(cfg.TargetLabel) == val {
			return false, DropInfo{Reason: DropReasonDropEqualMatched, RuleIndex: idx, Action: DropEqual}
		}
	case KeepEqual:
		if p.getLabel(cfg.TargetLabel) != val {
			return false, DropInfo{Reason: DropReasonKeepEqualMismatch, RuleIndex: idx, Action: KeepEqual}
		}
	case Replace:
		p.applyReplace(cfg, val)
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

	return true, DropInfo{}
}

func (p *Processor) applyReplace(cfg *Config, val string) {
	if val == "" &&
		cfg.Regex.String() == defaultConfig.Regex.String() &&
		!varInRegexTemplate(cfg.TargetLabel) &&
		!varInRegexTemplate(cfg.Replacement) {
		p.setLabel(cfg.TargetLabel, cfg.Replacement, cfg.NameScheme)
		return
	}

	indexes := cfg.Regex.FindStringSubmatchIndex(val)
	if indexes == nil {
		return
	}

	p.buf = cfg.Regex.ExpandString(p.buf[:0], cfg.TargetLabel, val, indexes)
	target := string(p.buf)
	if !cfg.NameScheme.IsValidLabelName(target) {
		return
	}

	p.buf = cfg.Regex.ExpandString(p.buf[:0], cfg.Replacement, val, indexes)
	if len(p.buf) == 0 {
		p.delLabel(target)
		return
	}

	p.setLabel(target, string(p.buf), cfg.NameScheme)
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
	// Snapshot the current label set (including __name__) before invoking fn, so a
	// callback that adds labels (labelmap) does not re-process labels it creates in
	// the same rule — matching Prometheus, which ranges one snapshot per rule. The
	// scratch buffer is reused across calls.
	p.rangeBuf = p.rangeBuf[:0]
	if p.currentName != "" {
		p.rangeBuf = append(p.rangeBuf, labels.Label{Name: commonmodel.MetricNameLabel, Value: p.currentName})
	}
	p.builder.Range(func(l labels.Label) {
		p.rangeBuf = append(p.rangeBuf, l)
	})
	for _, l := range p.rangeBuf {
		fn(l)
	}
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
		cfg.Action = defaultConfig.Action
	} else if act, err := parseAction(string(cfg.Action)); err == nil {
		// Canonicalize a valid action (e.g. "KEEP" -> "keep") so Apply's switch
		// matches; an invalid action is left for validate to reject.
		cfg.Action = act
	}
	if !cfg.separatorSet && cfg.Separator == "" {
		cfg.Separator = defaultConfig.Separator
	}
	if cfg.Regex.Regexp == nil {
		cfg.Regex = defaultConfig.Regex
	}
	if !cfg.replacementSet && cfg.Replacement == "" {
		cfg.Replacement = defaultConfig.Replacement
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
