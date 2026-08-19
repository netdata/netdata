// SPDX-License-Identifier: GPL-3.0-or-later

package relabel

import (
	"context"
	"errors"
	"fmt"
	"maps"
	"regexp/syntax"
	"slices"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/netdata/netdata/go/plugins/pkg/matcher"
	commonmodel "github.com/prometheus/common/model"
	"github.com/prometheus/prometheus/model/labels"
)

var ErrAnalysisBudgetExceeded = errors.New("relabel analysis budget exceeded")

const (
	defaultAnalysisMaxValues     = 256
	defaultAnalysisMaxOperations = 100_000
)

// AnalysisBudget bounds one Analyzer. MaxValues limits each materialized
// finite set; MaxOperations limits aggregate work across all method calls. A
// zero field uses its default; a negative field is invalid.
type AnalysisBudget struct {
	MaxValues     int
	MaxOperations int
}

// Analyzer performs bounded, cancelable static analysis using the same relabel
// regexp/replacement and name-validation semantics as Processor. It is not
// goroutine-safe.
type Analyzer struct {
	ctx        context.Context
	budget     AnalysisBudget
	operations int
}

// NewAnalyzer creates a bounded analysis session.
func NewAnalyzer(ctx context.Context, budget AnalysisBudget) (*Analyzer, error) {
	if ctx == nil {
		return nil, errors.New("nil relabel analysis context")
	}
	if budget.MaxValues < 0 || budget.MaxOperations < 0 {
		return nil, errors.New("invalid negative relabel analysis budget")
	}
	if budget.MaxValues == 0 {
		budget.MaxValues = defaultAnalysisMaxValues
	}
	if budget.MaxOperations == 0 {
		budget.MaxOperations = defaultAnalysisMaxOperations
	}
	return &Analyzer{ctx: ctx, budget: budget}, nil
}

// EnumerateFiniteRegexp returns the complete, sorted language when it is finite
// and fits MaxValues. finite is false for an unbounded or larger language.
func (a *Analyzer) EnumerateFiniteRegexp(expr string) (values []string, finite bool, err error) {
	parsed, err := a.parseRegexp(expr)
	if err != nil {
		return nil, false, err
	}
	return a.EnumerateFiniteSyntax(parsed.Simplify())
}

// EnumerateFiniteSyntax is EnumerateFiniteRegexp for an already parsed RE2
// syntax tree.
func (a *Analyzer) EnumerateFiniteSyntax(re *syntax.Regexp) (values []string, finite bool, err error) {
	if a == nil || re == nil {
		return nil, false, errors.New("nil relabel regexp analysis")
	}
	values, finite, err = a.enumerateFiniteSyntax(re)
	if err != nil || !finite {
		return nil, finite, err
	}
	slices.Sort(values)
	return slices.Compact(values), true, nil
}

// ReplacementOutputs returns every possible output of replacement for regexp.
// It is exact for a finite input language and for an unbounded input language
// whose replacement is constant or references one finite capture. Other
// unbounded/correlated capture projections return finite=false.
func (a *Analyzer) ReplacementOutputs(regexp Regexp, replacement string) (outputs []string, finite bool, err error) {
	if regexp.Regexp == nil {
		return nil, false, errors.New("unset relabel regexp")
	}
	metadata, err := a.captureMetadata(regexp.String())
	if err != nil {
		return nil, false, err
	}
	tokens, ok, err := a.replacementTokens(replacement, metadata)
	if err != nil {
		return nil, false, err
	}
	if !ok {
		return nil, false, nil
	}

	inputs, inputsFinite, err := a.EnumerateFiniteSyntax(metadata.parsed.Simplify())
	if err != nil {
		return nil, false, err
	}
	if inputsFinite {
		result := make(map[string]struct{}, len(inputs))
		for _, input := range inputs {
			if err := a.step(1); err != nil {
				return nil, false, err
			}
			if regexp.MatchString(input) {
				result[regexp.ReplaceAllString(input, replacement)] = struct{}{}
				if len(result) > a.budget.MaxValues {
					return nil, false, nil
				}
			}
		}
		return slices.Sorted(maps.Keys(result)), true, nil
	}

	references := referencedCaptureIDs(tokens)
	switch len(references) {
	case 0:
		return []string{renderReplacementTokens(tokens, nil)}, true, nil
	case 1:
		capture := references[0]
		if _, nested := metadata.nestedInOpen[capture]; nested {
			return nil, false, nil
		}
		values, finite, err := a.EnumerateFiniteSyntax(metadata.captures[capture])
		if err != nil || !finite {
			return nil, false, err
		}
		if _, required := regexpRequiredCaptures(metadata.parsed)[capture]; !required && !slices.Contains(values, "") {
			if len(values) >= a.budget.MaxValues {
				return nil, false, nil
			}
			if err := a.step(1); err != nil {
				return nil, false, err
			}
			values = append(values, "")
		}
		result := make(map[string]struct{}, len(values))
		for _, value := range values {
			if err := a.step(1); err != nil {
				return nil, false, err
			}
			result[renderReplacementTokens(tokens, map[int]string{capture: value})] = struct{}{}
		}
		return slices.Sorted(maps.Keys(result)), true, nil
	default:
		return nil, false, nil
	}
}

// ExactCaptureReference reports whether replacement consists of exactly one
// valid capture reference and returns its numeric capture ID.
func (a *Analyzer) ExactCaptureReference(regexp Regexp, replacement string) (capture int, exact bool, err error) {
	metadata, err := a.captureMetadata(regexp.String())
	if err != nil {
		return 0, false, err
	}
	tokens, ok, err := a.replacementTokens(replacement, metadata)
	if err != nil {
		return 0, false, err
	}
	if !ok || len(tokens) != 1 || !tokens[0].isCapture {
		return 0, false, nil
	}
	return tokens[0].capture, true, nil
}

// ReplacementGlob returns a glob over every syntactically possible replacement
// output by preserving literal text and replacing capture references with '*'.
// The bool is false when the replacement is ambiguous or always empty.
func (a *Analyzer) ReplacementGlob(regexp Regexp, replacement string) (pattern string, possible bool, err error) {
	metadata, err := a.captureMetadata(regexp.String())
	if err != nil {
		return "", false, err
	}
	tokens, ok, err := a.replacementTokens(replacement, metadata)
	if err != nil {
		return "", false, err
	}
	if !ok {
		return "", false, nil
	}
	var output strings.Builder
	lastWildcard := false
	for _, token := range tokens {
		if token.isCapture {
			if !lastWildcard {
				output.WriteByte('*')
			}
			lastWildcard = true
			continue
		}
		output.WriteString(matcher.QuoteGlobLiteral(token.literal))
		lastWildcard = false
	}
	if output.Len() == 0 {
		return "", false, nil
	}
	return output.String(), true, nil
}

// RuleMayWriteLabel conservatively reports whether rule can write labelName.
func (a *Analyzer) RuleMayWriteLabel(rule Config, labelName string) (bool, error) {
	if err := a.step(1); err != nil {
		return false, err
	}
	rule = rule.WithDefaults()
	action := EffectiveAction(rule)
	switch action {
	case Replace:
		return a.templateMayExpandToLabel(rule.Regex, rule.TargetLabel, labelName, false)
	case HashMod, Lowercase, Uppercase:
		return rule.TargetLabel == labelName, nil
	case LabelMap:
		if labelMapPreservesInputName(rule.Regex.String(), rule.Replacement) {
			return false, nil
		}
		return a.templateMayExpandToLabel(rule.Regex, rule.Replacement, labelName, true)
	default:
		return false, nil
	}
}

// RuleNameDerivedOnly reports whether a name-writing rule derives its value
// exclusively from the current metric name.
func RuleNameDerivedOnly(rule Config) bool {
	if EffectiveAction(rule) == LabelMap || len(rule.SourceLabels) == 0 {
		return false
	}
	for _, source := range rule.SourceLabels {
		if source != labels.MetricName {
			return false
		}
	}
	return true
}

// RulesMayAffectFutureRouting reports whether any rule can drop a sample or
// write the metric name.
func (a *Analyzer) RulesMayAffectFutureRouting(rules []Config) (bool, error) {
	for _, rule := range rules {
		if err := a.step(1); err != nil {
			return false, err
		}
		switch EffectiveAction(rule) {
		case Drop, DropEqual, Keep, KeepEqual:
			return true, nil
		default:
			writes, err := a.RuleMayWriteLabel(rule, labels.MetricName)
			if err != nil {
				return false, err
			}
			if writes {
				return true, nil
			}
		}
	}
	return false, nil
}

// RulesPreserveLabel conservatively reports whether rules leave labelName
// unchanged for possibleMetricNames. A nil name set means any name is possible.
func (a *Analyzer) RulesPreserveLabel(rules []Config, labelName string, possibleMetricNames []string) (bool, error) {
	for _, rule := range rules {
		if err := a.step(1); err != nil {
			return false, err
		}
		rule = rule.WithDefaults()
		action := EffectiveAction(rule)
		mayApply := ruleMayApplyToMetricNames(rule, action, possibleMetricNames)
		switch action {
		case Replace, HashMod, Lowercase, Uppercase:
			writes, err := a.RuleMayWriteLabel(rule, labelName)
			if err != nil {
				return false, err
			}
			if mayApply && writes {
				return false, nil
			}
		case LabelDrop:
			if rule.Regex.MatchString(labelName) {
				return false, nil
			}
		case LabelKeep:
			if !rule.Regex.MatchString(labelName) {
				return false, nil
			}
		case LabelMap:
			writes, err := a.RuleMayWriteLabel(rule, labelName)
			if err != nil {
				return false, err
			}
			if writes {
				return false, nil
			}
		}

		writesName, err := a.RuleMayWriteLabel(rule, labels.MetricName)
		if err != nil {
			return false, err
		}
		if writesName {
			possibleMetricNames = finiteMetricNamesAfterRule(rule, action, possibleMetricNames)
		}
	}
	return true, nil
}

// EffectiveAction returns the runtime action default and canonical lowercase.
func EffectiveAction(rule Config) Action {
	return NormalizeAction(rule.Action)
}

// NormalizeAction returns the runtime action default and canonical lowercase.
func NormalizeAction(action Action) Action {
	action = Action(strings.ToLower(strings.TrimSpace(string(action))))
	if action == "" {
		return Replace
	}
	return action
}

func (a *Analyzer) templateMayExpandToLabel(
	regexp Regexp,
	template string,
	labelName string,
	labelMap bool,
) (bool, error) {
	if !strings.Contains(template, "$") {
		return template == labelName, nil
	}
	metadata, err := a.captureMetadata(regexp.String())
	if err != nil {
		return false, err
	}
	tokens, ok, err := a.replacementTokens(template, metadata)
	if err != nil {
		return false, err
	}
	if !ok {
		return true, nil
	}
	if len(tokens) > 0 && !tokens[0].isCapture && !strings.HasPrefix(labelName, tokens[0].literal) {
		return false, nil
	}
	if len(tokens) > 0 && !tokens[len(tokens)-1].isCapture && !strings.HasSuffix(labelName, tokens[len(tokens)-1].literal) {
		return false, nil
	}

	inputs, finite, err := a.EnumerateFiniteSyntax(metadata.parsed.Simplify())
	if err != nil {
		return false, err
	}
	if finite {
		for _, input := range inputs {
			if labelMap && input == labels.MetricName {
				continue
			}
			if regexp.MatchString(input) && regexp.ReplaceAllString(input, template) == labelName &&
				(!labelMap || input != labelName) {
				return true, nil
			}
		}
		return false, nil
	}
	return true, nil
}

func (a *Analyzer) step(operations int) error {
	if err := a.ctx.Err(); err != nil {
		return err
	}
	if operations < 0 || a.operations > a.budget.MaxOperations-operations {
		return fmt.Errorf("%w: operations reached %d", ErrAnalysisBudgetExceeded, a.budget.MaxOperations)
	}
	a.operations += operations
	return nil
}

func (a *Analyzer) parseRegexp(expr string) (*syntax.Regexp, error) {
	if err := a.consumeParserInput(expr); err != nil {
		return nil, err
	}
	return syntax.Parse(expr, syntax.Perl)
}

// consumeParserInput charges linear parser/tokenizer work before it begins, so
// cancellation and the aggregate operation budget bound even malformed input.
func (a *Analyzer) consumeParserInput(input string) error {
	if err := a.step(1); err != nil {
		return err
	}
	if len(input) == 0 {
		return nil
	}
	return a.step(len(input))
}

func (a *Analyzer) enumerateFiniteSyntax(re *syntax.Regexp) ([]string, bool, error) {
	if err := a.step(1); err != nil {
		return nil, false, err
	}
	switch re.Op {
	case syntax.OpNoMatch:
		return nil, true, nil
	case syntax.OpEmptyMatch, syntax.OpBeginLine, syntax.OpEndLine, syntax.OpBeginText,
		syntax.OpEndText, syntax.OpWordBoundary, syntax.OpNoWordBoundary:
		return []string{""}, true, nil
	case syntax.OpLiteral:
		if re.Flags&syntax.FoldCase != 0 {
			return nil, false, nil
		}
		return []string{string(re.Rune)}, true, nil
	case syntax.OpCharClass:
		count := 0
		for i := 0; i+1 < len(re.Rune); i += 2 {
			size := int64(re.Rune[i+1]) - int64(re.Rune[i]) + 1
			if size > int64(a.budget.MaxValues-count) {
				return nil, false, nil
			}
			count += int(size)
		}
		values := make([]string, 0, count)
		for i := 0; i+1 < len(re.Rune); i += 2 {
			for value := re.Rune[i]; value <= re.Rune[i+1]; value++ {
				if err := a.step(1); err != nil {
					return nil, false, err
				}
				values = append(values, string(value))
			}
		}
		return values, true, nil
	case syntax.OpCapture:
		return a.enumerateFiniteSyntax(re.Sub[0])
	case syntax.OpConcat:
		values := []string{""}
		for _, sub := range re.Sub {
			part, finite, err := a.enumerateFiniteSyntax(sub)
			if err != nil || !finite {
				return nil, finite, err
			}
			values, finite, err = a.concatFiniteValues(values, part)
			if err != nil || !finite {
				return nil, finite, err
			}
		}
		return values, true, nil
	case syntax.OpAlternate:
		seen := make(map[string]struct{})
		for _, sub := range re.Sub {
			part, finite, err := a.enumerateFiniteSyntax(sub)
			if err != nil || !finite {
				return nil, finite, err
			}
			for _, value := range part {
				if err := a.step(1); err != nil {
					return nil, false, err
				}
				seen[value] = struct{}{}
				if len(seen) > a.budget.MaxValues {
					return nil, false, nil
				}
			}
		}
		return slices.Sorted(maps.Keys(seen)), true, nil
	case syntax.OpQuest:
		part, finite, err := a.enumerateFiniteSyntax(re.Sub[0])
		if err != nil || !finite {
			return nil, finite, err
		}
		if !slices.Contains(part, "") {
			if len(part) >= a.budget.MaxValues {
				return nil, false, nil
			}
			if err := a.step(1); err != nil {
				return nil, false, err
			}
			part = append(part, "")
		}
		slices.Sort(part)
		return slices.Compact(part), true, nil
	case syntax.OpRepeat:
		if re.Max < 0 {
			return nil, false, nil
		}
		part, finite, err := a.enumerateFiniteSyntax(re.Sub[0])
		if err != nil || !finite {
			return nil, finite, err
		}
		all := make(map[string]struct{})
		current := []string{""}
		for count := 0; count <= re.Max; count++ {
			if count >= re.Min {
				for _, value := range current {
					if err := a.step(1); err != nil {
						return nil, false, err
					}
					all[value] = struct{}{}
					if len(all) > a.budget.MaxValues {
						return nil, false, nil
					}
				}
			}
			if count == re.Max {
				break
			}
			current, finite, err = a.concatFiniteValues(current, part)
			if err != nil || !finite {
				return nil, finite, err
			}
		}
		return slices.Sorted(maps.Keys(all)), true, nil
	default:
		// AnyChar, AnyCharNotNL, Star, and Plus describe an unbounded language.
		return nil, false, nil
	}
}

func (a *Analyzer) concatFiniteValues(left, right []string) ([]string, bool, error) {
	if len(left) == 0 || len(right) == 0 {
		return nil, true, nil
	}
	capacity := a.budget.MaxValues
	if len(left) <= a.budget.MaxValues/len(right) {
		capacity = len(left) * len(right)
	}
	values := make(map[string]struct{}, capacity)
	for _, prefix := range left {
		for _, suffix := range right {
			if err := a.step(1); err != nil {
				return nil, false, err
			}
			values[prefix+suffix] = struct{}{}
			if len(values) > a.budget.MaxValues {
				return nil, false, nil
			}
		}
	}
	return slices.Sorted(maps.Keys(values)), true, nil
}

type replacementToken struct {
	literal   string
	capture   int
	isCapture bool
}

type captureMetadata struct {
	parsed         *syntax.Regexp
	captures       map[int]*syntax.Regexp
	named          map[string]int
	ambiguousNames map[string]struct{}
	nestedInOpen   map[int]struct{}
}

func (a *Analyzer) captureMetadata(expr string) (captureMetadata, error) {
	parsed, err := a.parseRegexp(expr)
	if err != nil {
		return captureMetadata{}, err
	}
	metadata := captureMetadata{
		parsed:         parsed,
		captures:       map[int]*syntax.Regexp{0: parsed},
		named:          make(map[string]int),
		ambiguousNames: make(map[string]struct{}),
		nestedInOpen:   make(map[int]struct{}),
	}
	var walk func(*syntax.Regexp, bool) error
	walk = func(re *syntax.Regexp, openAncestor bool) error {
		if err := a.step(1); err != nil {
			return err
		}
		openHere := false
		if re.Op == syntax.OpCapture {
			metadata.captures[re.Cap] = re.Sub[0]
			if openAncestor {
				metadata.nestedInOpen[re.Cap] = struct{}{}
			}
			openHere = !regexpLanguageIsFinite(re.Sub[0])
			if re.Name != "" {
				if _, ok := metadata.named[re.Name]; ok {
					metadata.ambiguousNames[re.Name] = struct{}{}
				} else {
					metadata.named[re.Name] = re.Cap
				}
			}
		}
		for _, sub := range re.Sub {
			if err := walk(sub, openAncestor || openHere); err != nil {
				return err
			}
		}
		return nil
	}
	if err := walk(parsed, false); err != nil {
		return captureMetadata{}, err
	}
	return metadata, nil
}

func (a *Analyzer) replacementTokens(template string, metadata captureMetadata) ([]replacementToken, bool, error) {
	if err := a.consumeParserInput(template); err != nil {
		return nil, false, err
	}
	tokens, ok := parseReplacementTemplate(template, metadata)
	return tokens, ok, nil
}

func parseReplacementTemplate(template string, metadata captureMetadata) ([]replacementToken, bool) {
	var tokens []replacementToken
	var literal strings.Builder
	flushLiteral := func() {
		if literal.Len() == 0 {
			return
		}
		tokens = append(tokens, replacementToken{literal: literal.String()})
		literal.Reset()
	}
	appendLiteral := func(value string) {
		literal.WriteString(value)
	}
	for len(template) > 0 {
		before, after, ok := strings.Cut(template, "$")
		appendLiteral(before)
		if !ok {
			break
		}
		template = after
		if strings.HasPrefix(template, "$") {
			appendLiteral("$")
			template = template[1:]
			continue
		}
		name, num, rest, ok := extractReplacementReference(template)
		if !ok {
			appendLiteral("$")
			continue
		}
		template = rest
		capture := num
		if capture < 0 {
			if _, ambiguous := metadata.ambiguousNames[name]; ambiguous {
				return nil, false
			}
			var exists bool
			capture, exists = metadata.named[name]
			if !exists {
				continue
			}
		}
		if _, exists := metadata.captures[capture]; exists {
			flushLiteral()
			tokens = append(tokens, replacementToken{capture: capture, isCapture: true})
		}
	}
	flushLiteral()
	return tokens, true
}

func referencedCaptureIDs(tokens []replacementToken) []int {
	ids := make(map[int]struct{})
	for _, token := range tokens {
		if token.isCapture {
			ids[token.capture] = struct{}{}
		}
	}
	return slices.Sorted(maps.Keys(ids))
}

func renderReplacementTokens(tokens []replacementToken, assignments map[int]string) string {
	var output strings.Builder
	for _, token := range tokens {
		if token.isCapture {
			output.WriteString(assignments[token.capture])
		} else {
			output.WriteString(token.literal)
		}
	}
	return output.String()
}

func regexpRequiredCaptures(re *syntax.Regexp) map[int]struct{} {
	switch re.Op {
	case syntax.OpCapture:
		required := regexpRequiredCaptures(re.Sub[0])
		required[re.Cap] = struct{}{}
		return required
	case syntax.OpConcat:
		required := make(map[int]struct{})
		for _, sub := range re.Sub {
			for capture := range regexpRequiredCaptures(sub) {
				required[capture] = struct{}{}
			}
		}
		return required
	case syntax.OpAlternate:
		if len(re.Sub) == 0 {
			return map[int]struct{}{}
		}
		required := regexpRequiredCaptures(re.Sub[0])
		for _, sub := range re.Sub[1:] {
			branch := regexpRequiredCaptures(sub)
			for capture := range required {
				if _, ok := branch[capture]; !ok {
					delete(required, capture)
				}
			}
		}
		return required
	case syntax.OpPlus:
		return regexpRequiredCaptures(re.Sub[0])
	case syntax.OpRepeat:
		if re.Min > 0 {
			return regexpRequiredCaptures(re.Sub[0])
		}
	}
	return map[int]struct{}{}
}

func regexpLanguageIsFinite(re *syntax.Regexp) bool {
	switch re.Op {
	case syntax.OpAnyChar, syntax.OpAnyCharNotNL, syntax.OpStar, syntax.OpPlus:
		return false
	case syntax.OpRepeat:
		return re.Max >= 0 && regexpLanguageIsFinite(re.Sub[0])
	default:
		for _, sub := range re.Sub {
			if !regexpLanguageIsFinite(sub) {
				return false
			}
		}
		return true
	}
}

// extractReplacementReference mirrors regexp's $name/${name} parsing.
func extractReplacementReference(str string) (name string, num int, rest string, ok bool) {
	if str == "" {
		return "", 0, "", false
	}
	brace := false
	if str[0] == '{' {
		brace = true
		str = str[1:]
	}
	i := 0
	for i < len(str) {
		r, size := utf8.DecodeRuneInString(str[i:])
		if !unicode.IsLetter(r) && !unicode.IsDigit(r) && r != '_' {
			break
		}
		i += size
	}
	if i == 0 {
		return "", 0, "", false
	}
	name = str[:i]
	if brace {
		if i >= len(str) || str[i] != '}' {
			return "", 0, "", false
		}
		i++
	}
	num = 0
	for i := range len(name) {
		if name[i] < '0' || name[i] > '9' || num >= 1e8 {
			num = -1
			break
		}
		num = num*10 + int(name[i]) - '0'
	}
	if name[0] == '0' && len(name) > 1 {
		num = -1
	}
	return name, num, str[i:], true
}

func labelMapPreservesInputName(regex, replacement string) bool {
	switch replacement {
	case "$0", "${0}":
		return true
	case "$1", "${1}":
		return regex == "(.*)" || regex == "(.+)"
	default:
		return false
	}
}

func ruleMayApplyToMetricNames(rule Config, action Action, possibleNames []string) bool {
	if possibleNames == nil || action != Replace ||
		len(rule.SourceLabels) != 1 || rule.SourceLabels[0] != labels.MetricName {
		return true
	}
	return slices.ContainsFunc(possibleNames, rule.Regex.MatchString)
}

func finiteMetricNamesAfterRule(rule Config, action Action, possibleNames []string) []string {
	if possibleNames == nil || action != Replace ||
		len(rule.SourceLabels) != 1 || rule.SourceLabels[0] != labels.MetricName ||
		rule.TargetLabel != labels.MetricName {
		return nil
	}
	scheme := rule.NameScheme
	if scheme == commonmodel.UnsetValidation {
		scheme = defaultNameValidationScheme
	}
	outputs := make(map[string]struct{}, len(possibleNames))
	for _, name := range possibleNames {
		output := name
		if rule.Regex.MatchString(name) {
			output = rule.Regex.ReplaceAllString(name, rule.Replacement)
		}
		if scheme.IsValidMetricName(output) {
			outputs[output] = struct{}{}
		}
	}
	return slices.Sorted(maps.Keys(outputs))
}
