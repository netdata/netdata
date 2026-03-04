// SPDX-License-Identifier: GPL-3.0-or-later

package prometheus

import (
	"fmt"
	"regexp"
	"sort"
	"strings"

	"github.com/prometheus/prometheus/model/labels"

	"github.com/netdata/netdata/go/plugins/pkg/prometheus/selector"
	"github.com/netdata/netdata/go/plugins/plugin/framework/collectorapi"
)

var (
	defaultRelabelRegex = regexp.MustCompile("(.*)")
	dimVarRe            = regexp.MustCompile(`\$\{([^}]+)}`)
	// quantile/le are synthetic labels used only on summary/histogram sub-series.
	// They are intentionally blocked in dimension templates to avoid cross-series
	// dimension ID collisions for related _sum/_count lines.
	reservedDimVars = map[string]bool{
		"quantile": true,
		"le":       true,
	}
)

type relabelAction string

const (
	relabelReplace   relabelAction = "replace"
	relabelLabelMap  relabelAction = "labelmap"
	relabelLabelDrop relabelAction = "labeldrop"
	relabelLabelKeep relabelAction = "labelkeep"
)

type compiledRelabelRule struct {
	sourceLabels []string
	re           *regexp.Regexp
	targetLabel  string
	replacement  string
	action       relabelAction
}

type compiledContextRule struct {
	re      *regexp.Regexp
	context string
	title   string
	units   string
	cType   collectorapi.ChartType
	hasType bool
}

type compiledDimensionRule struct {
	re       *regexp.Regexp
	template string
	vars     []string
}

type chartOverrides struct {
	context string
	title   string
	units   string
	cType   collectorapi.ChartType
	hasType bool
}

type compiledSelectorGroup struct {
	name           string
	chartIDPrefix  string
	selector       selector.Selector
	labelRelabel   []compiledRelabelRule
	contextRules   []compiledContextRule
	dimensionRules []compiledDimensionRule
}

func (c *Collector) initRules() error {
	lr, err := compileRelabelRules(c.LabelRelabel)
	if err != nil {
		return fmt.Errorf("label_relabel: %v", err)
	}
	cr, err := compileContextRules(c.ContextRules)
	if err != nil {
		return fmt.Errorf("context_rules: %v", err)
	}
	dr, err := compileDimensionRules(c.DimensionRules)
	if err != nil {
		return fmt.Errorf("dimension_rules: %v", err)
	}
	sg, err := compileSelectorGroups(c.SelectorGroups)
	if err != nil {
		return fmt.Errorf("selector_groups: %v", err)
	}

	c.labelRelabelRules = lr
	c.contextRules = cr
	c.dimensionRules = dr
	c.selectorGroups = sg
	return nil
}

func compileSelectorGroups(groups []SelectorGroup) ([]compiledSelectorGroup, error) {
	if len(groups) == 0 {
		return nil, nil
	}

	out := make([]compiledSelectorGroup, 0, len(groups))
	seen := make(map[string]bool, len(groups))
	seenPrefix := make(map[string]string, len(groups))

	for i, g := range groups {
		name := strings.TrimSpace(g.Name)
		if name == "" {
			name = fmt.Sprintf("group_%d", i+1)
		}
		if seen[name] {
			return nil, fmt.Errorf("group %d: duplicate name %q", i+1, name)
		}
		seen[name] = true

		sr, err := g.Selector.Parse()
		if err != nil {
			return nil, fmt.Errorf("group %q: parsing selector: %v", name, err)
		}
		lr, err := compileRelabelRules(g.LabelRelabel)
		if err != nil {
			return nil, fmt.Errorf("group %q: label_relabel: %v", name, err)
		}
		cr, err := compileContextRules(g.ContextRules)
		if err != nil {
			return nil, fmt.Errorf("group %q: context_rules: %v", name, err)
		}
		dr, err := compileDimensionRules(g.DimensionRules)
		if err != nil {
			return nil, fmt.Errorf("group %q: dimension_rules: %v", name, err)
		}

		prefix := "sg_" + sanitizeMetricIDPart(name)
		if prevName, ok := seenPrefix[prefix]; ok {
			return nil, fmt.Errorf("group %q collides with group %q after name sanitization (%q)", name, prevName, prefix)
		}
		seenPrefix[prefix] = name

		out = append(out, compiledSelectorGroup{
			name:           name,
			chartIDPrefix:  prefix,
			selector:       sr,
			labelRelabel:   lr,
			contextRules:   cr,
			dimensionRules: dr,
		})
	}

	return out, nil
}

func compileRelabelRules(rules []RelabelRule) ([]compiledRelabelRule, error) {
	compiled := make([]compiledRelabelRule, 0, len(rules))
	for i, rule := range rules {
		act := strings.ToLower(strings.TrimSpace(rule.Action))
		if act == "" {
			act = string(relabelReplace)
		}

		var action relabelAction
		switch relabelAction(act) {
		case relabelReplace, relabelLabelMap, relabelLabelDrop, relabelLabelKeep:
			action = relabelAction(act)
		default:
			return nil, fmt.Errorf("rule %d: unsupported action %q", i, rule.Action)
		}

		var re *regexp.Regexp
		if strings.TrimSpace(rule.Regex) == "" {
			re = defaultRelabelRegex
		} else {
			r, err := regexp.Compile(rule.Regex)
			if err != nil {
				return nil, fmt.Errorf("rule %d: compiling regex %q: %v", i, rule.Regex, err)
			}
			re = r
		}

		replacement := rule.Replacement
		if replacement == "" {
			replacement = "$1"
		}

		targetLabel := strings.TrimSpace(rule.TargetLabel)
		if action == relabelReplace && targetLabel == "" {
			return nil, fmt.Errorf("rule %d: target_label is required for replace action", i)
		}

		compiled = append(compiled, compiledRelabelRule{
			sourceLabels: append([]string(nil), rule.SourceLabels...),
			re:           re,
			targetLabel:  targetLabel,
			replacement:  replacement,
			action:       action,
		})
	}
	return compiled, nil
}

func compileContextRules(rules []ContextRule) ([]compiledContextRule, error) {
	compiled := make([]compiledContextRule, 0, len(rules))
	for i, rule := range rules {
		if strings.TrimSpace(rule.Match) == "" {
			return nil, fmt.Errorf("rule %d: match is required", i)
		}
		re, err := regexp.Compile(rule.Match)
		if err != nil {
			return nil, fmt.Errorf("rule %d: compiling match regex %q: %v", i, rule.Match, err)
		}

		cRule := compiledContextRule{
			re:      re,
			context: strings.TrimSpace(rule.Context),
			title:   strings.TrimSpace(rule.Title),
			units:   strings.TrimSpace(rule.Units),
		}

		if v := strings.ToLower(strings.TrimSpace(rule.Type)); v != "" {
			switch v {
			case string(collectorapi.Line), string(collectorapi.Area), string(collectorapi.Stacked), string(collectorapi.Heatmap):
				cRule.cType = collectorapi.ChartType(v)
				cRule.hasType = true
			default:
				return nil, fmt.Errorf("rule %d: unsupported chart type %q", i, rule.Type)
			}
		}

		compiled = append(compiled, cRule)
	}
	return compiled, nil
}

func compileDimensionRules(rules []DimensionRule) ([]compiledDimensionRule, error) {
	compiled := make([]compiledDimensionRule, 0, len(rules))
	for i, rule := range rules {
		if strings.TrimSpace(rule.Match) == "" {
			return nil, fmt.Errorf("rule %d: match is required", i)
		}
		if strings.TrimSpace(rule.Dimension) == "" {
			return nil, fmt.Errorf("rule %d: dimension is required", i)
		}

		re, err := regexp.Compile(rule.Match)
		if err != nil {
			return nil, fmt.Errorf("rule %d: compiling match regex %q: %v", i, rule.Match, err)
		}

		template := strings.TrimSpace(rule.Dimension)
		matches := dimVarRe.FindAllStringSubmatch(template, -1)
		vars := make([]string, 0, len(matches))
		seen := make(map[string]bool, len(matches))
		for _, m := range matches {
			if len(m) < 2 {
				continue
			}
			v := m[1]
			if reservedDimVars[v] {
				return nil, fmt.Errorf("rule %d: dimension template uses reserved label %q", i, v)
			}
			if !seen[v] {
				seen[v] = true
				vars = append(vars, v)
			}
		}

		compiled = append(compiled, compiledDimensionRule{
			re:       re,
			template: template,
			vars:     vars,
		})
	}
	return compiled, nil
}

func (c *Collector) applyRelabel(in labels.Labels) labels.Labels {
	if len(c.labelRelabelRules) == 0 {
		return in
	}

	lbls := make(map[string]string, len(in))
	for _, lbl := range in {
		if lbl.Name == "" {
			continue
		}
		lbls[lbl.Name] = lbl.Value
	}

	for _, rule := range c.labelRelabelRules {
		switch rule.action {
		case relabelReplace:
			values := make([]string, 0, len(rule.sourceLabels))
			for _, name := range rule.sourceLabels {
				values = append(values, lbls[name])
			}
			src := strings.Join(values, ";")
			if !rule.re.MatchString(src) {
				continue
			}
			out := rule.re.ReplaceAllString(src, rule.replacement)
			if out == "" {
				delete(lbls, rule.targetLabel)
			} else {
				lbls[rule.targetLabel] = out
			}
		case relabelLabelMap:
			keys := sortedLabelKeys(lbls)
			for _, key := range keys {
				if !rule.re.MatchString(key) {
					continue
				}
				newName := rule.re.ReplaceAllString(key, rule.replacement)
				if newName == "" {
					continue
				}
				lbls[newName] = lbls[key]
			}
		case relabelLabelDrop:
			for _, key := range sortedLabelKeys(lbls) {
				if rule.re.MatchString(key) {
					delete(lbls, key)
				}
			}
		case relabelLabelKeep:
			for _, key := range sortedLabelKeys(lbls) {
				if !rule.re.MatchString(key) {
					delete(lbls, key)
				}
			}
		}
	}

	out := make(labels.Labels, 0, len(lbls))
	for _, key := range sortedLabelKeys(lbls) {
		out = append(out, labels.Label{Name: key, Value: lbls[key]})
	}
	return out
}

func sortedLabelKeys(lbls map[string]string) []string {
	keys := make([]string, 0, len(lbls))
	for k := range lbls {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

func (c *Collector) chartOverrides(metricName string) chartOverrides {
	for _, rule := range c.contextRules {
		if !rule.re.MatchString(metricName) {
			continue
		}
		return chartOverrides{
			context: rule.context,
			title:   rule.title,
			units:   rule.units,
			cType:   rule.cType,
			hasType: rule.hasType,
		}
	}
	return chartOverrides{}
}

func (c *Collector) matchDimensionRule(metricName string) *compiledDimensionRule {
	for i := range c.dimensionRules {
		if c.dimensionRules[i].re.MatchString(metricName) {
			return &c.dimensionRules[i]
		}
	}
	return nil
}

func (r compiledDimensionRule) render(labels labels.Labels) (string, map[string]bool, bool) {
	consumed := make(map[string]bool, len(r.vars))
	missing := false
	repl := dimVarRe.ReplaceAllStringFunc(r.template, func(match string) string {
		sub := dimVarRe.FindStringSubmatch(match)
		if len(sub) < 2 {
			return match
		}
		name := sub[1]
		value, ok := getLabelValue(labels, name)
		if !ok {
			missing = true
			return ""
		}
		consumed[name] = true
		return value
	})
	if missing {
		// Missing template variables make the dimension ambiguous; callers skip this sample.
		return "", nil, false
	}
	repl = strings.TrimSpace(repl)
	if repl == "" {
		return "", nil, false
	}
	return repl, consumed, true
}

func splitInstanceLabels(lbls labels.Labels, consumed map[string]bool) labels.Labels {
	if len(consumed) == 0 {
		return lbls
	}
	out := make(labels.Labels, 0, len(lbls))
	for _, lbl := range lbls {
		if !consumed[lbl.Name] {
			out = append(out, lbl)
		}
	}
	return out
}

func getLabelValue(lbls labels.Labels, name string) (string, bool) {
	for _, lbl := range lbls {
		if lbl.Name == name {
			return lbl.Value, true
		}
	}
	return "", false
}
