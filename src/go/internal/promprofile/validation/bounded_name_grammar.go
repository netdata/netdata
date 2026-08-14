// SPDX-License-Identifier: GPL-3.0-or-later

package promvalidation

import (
	"maps"
	"regexp/syntax"
	"slices"
	"strings"

	"github.com/netdata/netdata/go/plugins/pkg/matcher"
	metrixselector "github.com/netdata/netdata/go/plugins/pkg/metrix/selector"
	"github.com/netdata/netdata/go/plugins/plugin/framework/charttpl"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/prometheus/promprofiles"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/prometheus/relabel"
	commonmodel "github.com/prometheus/common/model"
	"github.com/prometheus/prometheus/model/labels"
)

// maxBoundedMetricNameGrammarBranches prevents finite grammar expansion from
// consuming the complete relabel-analysis budget on one declaration.
const maxBoundedMetricNameGrammarBranches = 256

type boundedMetricNameGrammar struct {
	exactNames          []string
	prefixes            []string
	suffixes            []string
	dynamicTailPrefixes []string
	dynamicCaptureIDs   []int
}

func (g boundedMetricNameGrammar) hasInternalDynamicKey() bool {
	return len(g.prefixes) > 0
}

func (g boundedMetricNameGrammar) hasDynamicIdentity() bool {
	return g.hasInternalDynamicKey() || len(g.dynamicTailPrefixes) > 0
}

func analyzeBoundedMetricNameDiscardGrammar(
	analyzer *relabel.Analyzer,
	rule relabel.Config,
	action relabel.Action,
) (boundedMetricNameGrammar, bool, error) {
	rule = rule.WithDefaults()
	if action != relabel.Drop {
		return boundedMetricNameGrammar{}, false, nil
	}
	return analyzeBoundedMetricNameGrammar(analyzer, rule.Regex.String(), false)
}

func analyzeBoundedMetricNameRewriteGrammar(
	analyzer *relabel.Analyzer,
	rule relabel.Config,
	action relabel.Action,
) (boundedMetricNameGrammar, bool, error) {
	rule = rule.WithDefaults()
	if action != relabel.Replace || len(rule.SourceLabels) != 1 || rule.SourceLabels[0] != labels.MetricName ||
		rule.TargetLabel != labels.MetricName {
		return boundedMetricNameGrammar{}, false, nil
	}
	grammar, ok, err := analyzeBoundedMetricNameGrammar(analyzer, rule.Regex.String(), true)
	if err != nil || !ok {
		return boundedMetricNameGrammar{}, false, err
	}
	if len(grammar.dynamicTailPrefixes) == 0 {
		return grammar, true, nil
	}
	if len(grammar.dynamicTailPrefixes) != 1 {
		return boundedMetricNameGrammar{}, false, nil
	}
	prefix := grammar.dynamicTailPrefixes[0]
	if !strings.HasSuffix(prefix, "_") || rule.Replacement != strings.TrimSuffix(prefix, "_") {
		return boundedMetricNameGrammar{}, false, nil
	}
	return grammar, true, nil
}

func analyzeBoundedMetricNameGrammar(
	analyzer *relabel.Analyzer,
	expr string,
	allowDynamicTail bool,
) (boundedMetricNameGrammar, bool, error) {
	parsed, err := syntax.Parse(expr, syntax.Perl)
	if err != nil {
		return boundedMetricNameGrammar{}, false, nil
	}
	parsed = parsed.Simplify()
	if names, finite, err := analyzer.EnumerateFiniteSyntax(parsed); err != nil {
		return boundedMetricNameGrammar{}, false, err
	} else if finite {
		if len(names) == 0 {
			return boundedMetricNameGrammar{}, false, nil
		}
		for _, name := range names {
			if !commonmodel.UTF8Validation.IsValidMetricName(name) {
				return boundedMetricNameGrammar{}, false, nil
			}
		}
		return boundedMetricNameGrammar{exactNames: names}, true, nil
	}

	parts := flattenRegexpConcat(parsed, nil)
	dynamicIndex := -1
	for i, part := range parts {
		if _, finite, err := analyzer.EnumerateFiniteSyntax(part.expr); err != nil {
			return boundedMetricNameGrammar{}, false, err
		} else if finite {
			continue
		}
		if dynamicIndex >= 0 {
			// Stock alias normalization has one dynamic entity key. Multiple
			// open-ended regions cannot be reviewed as a bounded metric grammar.
			return boundedMetricNameGrammar{}, false, nil
		}
		dynamicIndex = i
	}
	if dynamicIndex <= 0 || regexpCanMatchEmpty(parts[dynamicIndex].expr) {
		return boundedMetricNameGrammar{}, false, nil
	}

	prefixes, ok, err := enumerateFiniteRegexpSequence(analyzer, parts[:dynamicIndex])
	if err != nil {
		return boundedMetricNameGrammar{}, false, err
	}
	if !ok || len(prefixes) == 0 {
		return boundedMetricNameGrammar{}, false, nil
	}
	for _, prefix := range prefixes {
		if prefix == "" || !commonmodel.UTF8Validation.IsValidMetricName(prefix) {
			return boundedMetricNameGrammar{}, false, nil
		}
	}
	if dynamicIndex == len(parts)-1 {
		if !allowDynamicTail {
			return boundedMetricNameGrammar{}, false, nil
		}
		return boundedMetricNameGrammar{
			dynamicTailPrefixes: prefixes,
			dynamicCaptureIDs:   slices.Clone(parts[dynamicIndex].enclosingCaptureIDs),
		}, true, nil
	}

	suffixes, ok, err := enumerateFiniteRegexpSequence(analyzer, parts[dynamicIndex+1:])
	if err != nil {
		return boundedMetricNameGrammar{}, false, err
	}
	if !ok || len(suffixes) == 0 || len(prefixes) > maxBoundedMetricNameGrammarBranches/len(suffixes) {
		return boundedMetricNameGrammar{}, false, nil
	}
	for _, suffix := range suffixes {
		if !strings.HasPrefix(suffix, "_") ||
			!commonmodel.UTF8Validation.IsValidMetricName(strings.TrimPrefix(suffix, "_")) {
			return boundedMetricNameGrammar{}, false, nil
		}
	}
	return boundedMetricNameGrammar{
		prefixes:          prefixes,
		suffixes:          suffixes,
		dynamicCaptureIDs: slices.Clone(parts[dynamicIndex].enclosingCaptureIDs),
	}, true, nil
}

type regexpConcatPart struct {
	expr                *syntax.Regexp
	enclosingCaptureIDs []int
}

func flattenRegexpConcat(re *syntax.Regexp, enclosingCaptureIDs []int) []regexpConcatPart {
	if re.Op == syntax.OpCapture && len(re.Sub) == 1 {
		return flattenRegexpConcat(re.Sub[0], append(slices.Clone(enclosingCaptureIDs), re.Cap))
	}
	if re.Op != syntax.OpConcat {
		return []regexpConcatPart{{expr: re, enclosingCaptureIDs: slices.Clone(enclosingCaptureIDs)}}
	}
	var out []regexpConcatPart
	for _, sub := range re.Sub {
		out = append(out, flattenRegexpConcat(sub, enclosingCaptureIDs)...)
	}
	return out
}

func enumerateFiniteRegexpSequence(
	analyzer *relabel.Analyzer,
	parts []regexpConcatPart,
) ([]string, bool, error) {
	if len(parts) == 0 {
		return []string{""}, true, nil
	}
	subs := make([]*syntax.Regexp, 0, len(parts))
	for _, part := range parts {
		subs = append(subs, part.expr)
	}
	return analyzer.EnumerateFiniteSyntax(&syntax.Regexp{Op: syntax.OpConcat, Sub: subs})
}

func regexpCanMatchEmpty(re *syntax.Regexp) bool {
	switch re.Op {
	case syntax.OpNoMatch, syntax.OpLiteral, syntax.OpCharClass, syntax.OpAnyCharNotNL, syntax.OpAnyChar:
		return false
	case syntax.OpEmptyMatch, syntax.OpBeginLine, syntax.OpEndLine, syntax.OpBeginText,
		syntax.OpEndText, syntax.OpWordBoundary, syntax.OpNoWordBoundary, syntax.OpQuest, syntax.OpStar:
		return true
	case syntax.OpCapture, syntax.OpPlus:
		return len(re.Sub) == 0 || regexpCanMatchEmpty(re.Sub[0])
	case syntax.OpConcat:
		for _, sub := range re.Sub {
			if !regexpCanMatchEmpty(sub) {
				return false
			}
		}
		return true
	case syntax.OpAlternate:
		return slices.ContainsFunc(re.Sub, regexpCanMatchEmpty)
	case syntax.OpRepeat:
		return re.Min == 0 || len(re.Sub) == 0 || regexpCanMatchEmpty(re.Sub[0])
	default:
		return false
	}
}

func (g boundedMetricNameGrammar) missingEvidence(observed map[string]struct{}) []string {
	if len(g.exactNames) > 0 {
		var missing []string
		for _, name := range g.exactNames {
			if _, ok := observed[name]; !ok {
				missing = append(missing, name)
			}
		}
		return missing
	}
	if len(g.dynamicTailPrefixes) > 0 {
		var missing []string
		for _, prefix := range g.dynamicTailPrefixes {
			found := false
			for name := range observed {
				if strings.HasPrefix(name, prefix) && len(name) > len(prefix) {
					found = true
					break
				}
			}
			if !found {
				missing = append(missing, prefix+"<dynamic>")
			}
		}
		return missing
	}

	var missing []string
	for _, prefix := range g.prefixes {
		for _, suffix := range g.suffixes {
			found := false
			for name := range observed {
				if strings.HasPrefix(name, prefix) && strings.HasSuffix(name, suffix) &&
					len(name) > len(prefix)+len(suffix) {
					found = true
					break
				}
			}
			if !found {
				missing = append(missing, prefix+"<dynamic>"+suffix)
			}
		}
	}
	return missing
}

func (g boundedMetricNameGrammar) nonCanonicalRewriteOutputs(
	possibleOutputs []string,
	authoredMetricNames map[string]struct{},
) []string {
	if !g.hasInternalDynamicKey() && len(g.dynamicTailPrefixes) == 0 {
		return nil
	}

	outputs := make(map[string]struct{})
	for _, output := range possibleOutputs {
		if !commonmodel.UTF8Validation.IsValidMetricName(output) {
			outputs[output] = struct{}{}
			continue
		}
		if _, ok := authoredMetricNames[output]; !ok {
			outputs[output] = struct{}{}
		}
	}
	return slices.Sorted(maps.Keys(outputs))
}

func (g boundedMetricNameGrammar) finiteBranchCount() int {
	if len(g.dynamicTailPrefixes) > 0 {
		return len(g.dynamicTailPrefixes)
	}
	return len(g.prefixes) * len(g.suffixes)
}

func simplePatternScopesMayOverlap(analyzer *matcher.Analyzer, leftExpr, rightExpr string) (bool, error) {
	_, intersects, err := analyzer.SimplePatternIntersectionWitness(leftExpr, rightExpr, true)
	return intersects, err
}

func profileAuthoredMetricNames(profile promprofiles.Profile) map[string]struct{} {
	root, err := profile.Template()
	if err != nil {
		return nil
	}
	names := make(map[string]struct{})
	var walk func(charttpl.Group)
	walk = func(group charttpl.Group) {
		for _, chart := range group.Charts {
			for _, dimension := range chart.Dimensions {
				compiled, err := metrixselector.ParseCompiled(dimension.Selector)
				if err != nil {
					continue
				}
				for _, name := range compiled.Meta().MetricNames {
					names[name] = struct{}{}
				}
			}
		}
		for _, child := range group.Groups {
			walk(child)
		}
	}
	walk(root)
	return names
}

func exactMetricFamilySelector(expr string) (string, bool) {
	name := strings.TrimSpace(expr)
	if strings.Contains(name, "{") {
		return "", false
	}
	return name, !hasUnescapedGlobMeta(name) && commonmodel.UTF8Validation.IsValidMetricName(name)
}

func hasUnescapedGlobMeta(pattern string) bool {
	for i := 0; i < len(pattern); i++ {
		if pattern[i] == '\\' {
			i++
			continue
		}
		if pattern[i] == '*' || pattern[i] == '?' || pattern[i] == '[' {
			return true
		}
	}
	return false
}
