// SPDX-License-Identifier: GPL-3.0-or-later

package catalog

import (
	"errors"
	"fmt"
	"regexp"
	"strings"

	"github.com/netdata/netdata/go/plugins/plugin/framework/charttpl"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp_traps/internal/model"
)

var (
	profileMetricRuleNameRE      = regexp.MustCompile(`^[A-Za-z0-9_.:-]+::[A-Za-z0-9_.:-]+$|^[A-Za-z0-9_.:-]+$`)
	profileMetricOutputNameRE    = regexp.MustCompile(`^[a-z][a-z0-9_]*$`)
	profileMetricChartIDRE       = regexp.MustCompile(`^[a-z][a-z0-9_]*$`)
	profileMetricResourceClassRE = regexp.MustCompile(`^[a-z][a-z0-9_]*$`)
	profileMetricSiteClassRE     = regexp.MustCompile(`^site_[a-z0-9][a-z0-9_]*$`)
)

var stockProfileMetricResourceClasses = map[string]bool{
	"interface":   true,
	"peer":        true,
	"neighbor":    true,
	"sensor":      true,
	"alarm":       true,
	"pool":        true,
	"l2_topology": true,
	"component":   true,
}

var reservedProfileMetricPrefixes = []string{
	"snmp_trap_events_",
	"snmp_trap_severity_",
	"snmp_trap_errors_",
	"snmp_trap_dedup_",
	"snmp_trap_pipeline_",
	"snmp_trap_metric_",
	"snmp_trap_profile_metrics_",
}

var builtInProfileMetricChartIDs = map[string]bool{
	"events":                     true,
	"severity":                   true,
	"errors":                     true,
	"dedup_suppressed":           true,
	"pipeline":                   true,
	"profile_metric_diagnostics": true,
}

var builtInProfileMetricChartContexts = map[string]bool{
	"snmp.trap.events":                     true,
	"snmp.trap.severity":                   true,
	"snmp.trap.errors":                     true,
	"snmp.trap.dedup_suppressed":           true,
	"snmp.trap.pipeline":                   true,
	"snmp.trap.profile_metric_diagnostics": true,
	"profile_metric_diagnostics":           true,
}

func normalizeProfileMetricRule(rule *profileMetricRule) error {
	if rule == nil {
		return errors.New("nil metric rule")
	}
	if rule.Name == "" {
		return errors.New("name is required")
	}
	if !profileMetricRuleNameRE.MatchString(rule.Name) {
		return fmt.Errorf("name %q contains invalid characters", rule.Name)
	}
	rule.Type = strings.ToLower(strings.TrimSpace(rule.Type))
	switch rule.Type {
	case MetricTypeCounter, MetricTypeSample, MetricTypeState:
	default:
		return fmt.Errorf("rule %q type must be counter, sample, or state", rule.Name)
	}
	if rule.Identity.Device == "" {
		rule.Identity.Device = MetricIdentitySource
	}
	if rule.Output.Metric == "" {
		rule.Output.Metric = "snmp_trap_" + slugForMetric(rule.Name)
		if !strings.HasSuffix(rule.Output.Metric, "_events") && rule.Type == MetricTypeCounter {
			rule.Output.Metric += "_events"
		}
	}
	if rule.Output.Dimension == "" {
		switch rule.Type {
		case MetricTypeCounter:
			rule.Output.Dimension = "events"
		case MetricTypeSample:
			rule.Output.Dimension = "value"
		case MetricTypeState:
			rule.Output.Dimension = "state"
		}
	}
	if rule.Output.Chart == "" {
		rule.Output.Chart = slugForMetric(rule.Name)
	}
	if rule.Missing == "" {
		rule.Missing = MetricMissingDrop
	}
	rule.Missing = strings.ToLower(strings.TrimSpace(rule.Missing))
	if rule.Scale.Multiplier == 0 {
		rule.Scale.Multiplier = 1
	}
	if rule.Scale.Divisor == 0 {
		rule.Scale.Divisor = 1
	}
	return nil
}

func validateProfileMetricRule(rule *profileMetricRule, idx *Epoch, charts map[string]*profileMetricChart) error {
	if err := normalizeProfileMetricRule(rule); err != nil {
		return fmt.Errorf("%s: metric rule: %w", rule.SourceFile, err)
	}
	if !profileMetricOutputNameRE.MatchString(rule.Output.Metric) {
		return fmt.Errorf("%s: metric rule %q output.metric %q must match ^[a-z][a-z0-9_]*$", rule.SourceFile, rule.Name, rule.Output.Metric)
	}
	for _, prefix := range reservedProfileMetricPrefixes {
		if strings.HasPrefix(rule.Output.Metric, prefix) {
			return fmt.Errorf("%s: metric rule %q output.metric %q uses reserved prefix %q", rule.SourceFile, rule.Name, rule.Output.Metric, prefix)
		}
	}
	if !profileMetricOutputNameRE.MatchString(rule.Output.Dimension) {
		return fmt.Errorf("%s: metric rule %q output.dimension %q must match ^[a-z][a-z0-9_]*$", rule.SourceFile, rule.Name, rule.Output.Dimension)
	}
	if !profileMetricChartIDRE.MatchString(rule.Output.Chart) {
		return fmt.Errorf("%s: metric rule %q output.chart %q must match ^[a-z][a-z0-9_]*$", rule.SourceFile, rule.Name, rule.Output.Chart)
	}
	if charts[rule.Output.Chart] == nil {
		return fmt.Errorf("%s: metric rule %q references unknown chart %q", rule.SourceFile, rule.Name, rule.Output.Chart)
	}
	switch rule.Missing {
	case MetricMissingDrop, MetricMissingZero, MetricMissingUnknownDimension, MetricMissingError:
	default:
		return fmt.Errorf("%s: metric rule %q missing must be drop, zero, unknown_dimension, or error", rule.SourceFile, rule.Name)
	}
	switch rule.Type {
	case MetricTypeCounter:
		if rule.OnTrap == "" || rule.ProblemTrap != "" || rule.ClearTrap != "" || rule.ValueFromVarbind != "" {
			return fmt.Errorf("%s: counter rule %q requires only on_trap", rule.SourceFile, rule.Name)
		}
		if _, err := resolveProfileMetricTrap(idx, rule.OnTrap); err != nil {
			return fmt.Errorf("%s: counter rule %q on_trap: %w", rule.SourceFile, rule.Name, err)
		}
	case MetricTypeSample:
		if rule.OnTrap == "" || rule.ValueFromVarbind == "" {
			return fmt.Errorf("%s: sample rule %q requires on_trap and value_from_varbind", rule.SourceFile, rule.Name)
		}
		td, err := resolveProfileMetricTrap(idx, rule.OnTrap)
		if err != nil {
			return fmt.Errorf("%s: sample rule %q on_trap: %w", rule.SourceFile, rule.Name, err)
		}
		vb := trapMetricVarbindByName(td, rule.ValueFromVarbind)
		if vb == nil {
			return fmt.Errorf("%s: sample rule %q value_from_varbind %q not found", rule.SourceFile, rule.Name, rule.ValueFromVarbind)
		}
		if !isProfileMetricNumericVarbind(vb) {
			return fmt.Errorf("%s: sample rule %q value_from_varbind %q is non-numeric type %q", rule.SourceFile, rule.Name, rule.ValueFromVarbind, vb.Type)
		}
		if rule.Missing == MetricMissingUnknownDimension {
			return fmt.Errorf("%s: sample rule %q missing unknown_dimension requires identity.resource", rule.SourceFile, rule.Name)
		}
		if charts[rule.Output.Chart].Algorithm != "absolute" {
			return fmt.Errorf("%s: sample rule %q chart %q must use absolute algorithm", rule.SourceFile, rule.Name, rule.Output.Chart)
		}
	case MetricTypeState:
		if rule.OnTrap != "" {
			if rule.State.SetWhen == nil || rule.State.ClearWhen == nil || rule.ProblemTrap != "" || rule.ClearTrap != "" {
				return fmt.Errorf("%s: same-OID state rule %q requires on_trap with state set_when and clear_when only", rule.SourceFile, rule.Name)
			}
			if _, err := resolveProfileMetricTrap(idx, rule.OnTrap); err != nil {
				return fmt.Errorf("%s: state rule %q on_trap: %w", rule.SourceFile, rule.Name, err)
			}
		} else {
			if rule.ProblemTrap == "" || rule.ClearTrap == "" {
				return fmt.Errorf("%s: separate-OID state rule %q requires problem_trap and clear_trap", rule.SourceFile, rule.Name)
			}
			if rule.State.SetWhen != nil || rule.State.ClearWhen != nil {
				return fmt.Errorf("%s: separate-OID state rule %q cannot use state.set_when or state.clear_when", rule.SourceFile, rule.Name)
			}
			if _, err := resolveProfileMetricTrap(idx, rule.ProblemTrap); err != nil {
				return fmt.Errorf("%s: state rule %q problem_trap: %w", rule.SourceFile, rule.Name, err)
			}
			if _, err := resolveProfileMetricTrap(idx, rule.ClearTrap); err != nil {
				return fmt.Errorf("%s: state rule %q clear_trap: %w", rule.SourceFile, rule.Name, err)
			}
		}
		if charts[rule.Output.Chart].Algorithm != "absolute" {
			return fmt.Errorf("%s: state rule %q chart %q must use absolute algorithm", rule.SourceFile, rule.Name, rule.Output.Chart)
		}
	}
	if rule.Type != MetricTypeSample && rule.Missing == MetricMissingZero {
		return fmt.Errorf("%s: metric rule %q missing zero is supported only for sample rules", rule.SourceFile, rule.Name)
	}
	if rule.Missing == MetricMissingUnknownDimension && rule.Identity.Resource == nil {
		return fmt.Errorf("%s: metric rule %q missing unknown_dimension requires identity.resource", rule.SourceFile, rule.Name)
	}
	if rule.Scale.Divisor <= 0 {
		return fmt.Errorf("%s: metric rule %q scale.divisor must be greater than zero", rule.SourceFile, rule.Name)
	}
	if rule.Scale.Multiplier < 0 {
		return fmt.Errorf("%s: metric rule %q scale.multiplier must be zero or greater", rule.SourceFile, rule.Name)
	}
	if rule.State.TTL != "" {
		if _, err := ParseMetricStateTTL(rule.State.TTL); err != nil {
			return fmt.Errorf("%s: metric rule %q state.ttl %q is invalid: %w", rule.SourceFile, rule.Name, rule.State.TTL, err)
		}
	}
	for _, pred := range rule.Where {
		if err := validateProfileMetricPredicate(pred); err != nil {
			return fmt.Errorf("%s: metric rule %q where: %w", rule.SourceFile, rule.Name, err)
		}
	}
	if rule.State.SetWhen != nil {
		if err := validateProfileMetricPredicate(*rule.State.SetWhen); err != nil {
			return fmt.Errorf("%s: metric rule %q state.set_when: %w", rule.SourceFile, rule.Name, err)
		}
	}
	if rule.State.ClearWhen != nil {
		if err := validateProfileMetricPredicate(*rule.State.ClearWhen); err != nil {
			return fmt.Errorf("%s: metric rule %q state.clear_when: %w", rule.SourceFile, rule.Name, err)
		}
	}
	if err := validateProfileMetricPredicateReferences(rule, idx); err != nil {
		return fmt.Errorf("%s: metric rule %q: %w", rule.SourceFile, rule.Name, err)
	}
	if err := validateProfileMetricIdentity(rule); err != nil {
		return fmt.Errorf("%s: metric rule %q: %w", rule.SourceFile, rule.Name, err)
	}
	if err := validateProfileMetricResourceVarbind(rule, idx); err != nil {
		return fmt.Errorf("%s: metric rule %q: %w", rule.SourceFile, rule.Name, err)
	}
	return nil
}

func resolveProfileMetricTrap(idx *Epoch, ref string) (*TrapDef, error) {
	ref = strings.TrimSpace(ref)
	if ref == "" {
		return nil, errors.New("trap reference is empty")
	}
	if model.IsNumericOID(ref) {
		td, err := idx.LookupWithError(ref)
		if err != nil {
			return nil, err
		}
		if td == nil {
			return nil, fmt.Errorf("trap oid %q not found", ref)
		}
		return td, nil
	}
	if idx.stock != nil {
		if err := idx.stock.loadForTrapName(idx, ref); err != nil {
			return nil, err
		}
	}
	if td := idx.lookupTrapName(ref); td != nil {
		return td, nil
	}
	return nil, fmt.Errorf("trap name %q not found", ref)
}

func trapMetricVarbindByName(td *TrapDef, name string) *VarbindDef {
	if td == nil || name == "" {
		return nil
	}
	if vb := td.varbindByName(name); vb != nil {
		return vb
	}
	return td.inlineVarbindByName(name)
}

func validateProfileMetricPredicateReferences(rule *profileMetricRule, idx *Epoch) error {
	traps, err := profileMetricRuleTrapDefs(rule, idx)
	if err != nil {
		return err
	}
	for i, pred := range rule.Where {
		if err := validateProfileMetricPredicateReference(pred, traps); err != nil {
			return fmt.Errorf("where[%d]: %w", i, err)
		}
	}
	if rule.State.SetWhen != nil {
		if err := validateProfileMetricPredicateReference(*rule.State.SetWhen, traps); err != nil {
			return fmt.Errorf("state.set_when: %w", err)
		}
	}
	if rule.State.ClearWhen != nil {
		if err := validateProfileMetricPredicateReference(*rule.State.ClearWhen, traps); err != nil {
			return fmt.Errorf("state.clear_when: %w", err)
		}
	}
	return nil
}

func validateProfileMetricPredicateReference(pred profileMetricPredicate, traps []*TrapDef) error {
	if pred.Field != "" {
		if isProfileMetricSyntheticField(pred.Field) {
			return nil
		}
		return fmt.Errorf("field %q is not supported", pred.Field)
	}
	if pred.Varbind == "" {
		return fmt.Errorf("varbind or field is required")
	}
	for _, td := range traps {
		if trapMetricVarbindByName(td, pred.Varbind) == nil {
			return fmt.Errorf("varbind %q not found on trap %q", pred.Varbind, td.Name)
		}
	}
	return nil
}

func isProfileMetricSyntheticField(field string) bool {
	switch field {
	case "category", "severity", "trap_name", "trap_oid":
		return true
	default:
		return false
	}
}

func validateProfileMetricPredicate(pred profileMetricPredicate) error {
	if err := validateMetricPredicateSelector(pred); err != nil {
		return err
	}
	if pred.Not && (pred.Exists != nil || pred.Absent != nil) {
		return fmt.Errorf("not cannot be combined with exists or absent")
	}
	if pred.Range != nil && len(pred.Range) != 2 {
		return fmt.Errorf("range requires exactly two values")
	}
	if pred.GreaterThan != nil {
		if err := validateProfileMetricPredicateNumber("greater_than", pred.GreaterThan); err != nil {
			return err
		}
	}
	if pred.LessThan != nil {
		if err := validateProfileMetricPredicateNumber("less_than", pred.LessThan); err != nil {
			return err
		}
	}
	if len(pred.Range) == 2 {
		var bounds [2]float64
		for i, value := range pred.Range {
			if err := validateProfileMetricPredicateNumber(fmt.Sprintf("range[%d]", i), value); err != nil {
				return err
			}
			bounds[i], _ = ParseMetricFloat(value)
		}
		if bounds[0] > bounds[1] {
			return fmt.Errorf("range[0] must be less than or equal to range[1]")
		}
	}
	if pred.Equals == nil && len(pred.In) == 0 && pred.Exists == nil && pred.Absent == nil &&
		pred.GreaterThan == nil && pred.LessThan == nil && len(pred.Range) == 0 {
		return fmt.Errorf("predicate requires at least one condition")
	}
	return nil
}

func validateProfileMetricPredicateNumber(field string, value any) error {
	if _, ok := ParseMetricFloat(value); !ok {
		return fmt.Errorf("%s must be a finite number", field)
	}
	return nil
}

func validateProfileMetricResourceVarbind(rule *profileMetricRule, idx *Epoch) error {
	if rule.Identity.Resource == nil {
		return nil
	}
	traps, err := profileMetricRuleTrapDefs(rule, idx)
	if err != nil {
		return err
	}
	key := rule.Identity.Resource.KeyFromVarbind
	var resourceOID string
	for _, td := range traps {
		vb := trapMetricVarbindByName(td, key)
		if vb == nil {
			return fmt.Errorf("identity.resource.key_from_varbind %q not found on trap %q", key, td.Name)
		}
		if !isProfileMetricResourceKeyVarbind(vb) {
			return fmt.Errorf("identity.resource.key_from_varbind %q has unsupported type %q on trap %q; use an integer-like bounded resource key", key, vb.Type, td.Name)
		}
		if resourceOID == "" {
			resourceOID = vb.OID
			continue
		}
		if vb.OID != resourceOID {
			return fmt.Errorf("identity.resource.key_from_varbind %q resolves to different OIDs across state traps", key)
		}
	}
	return nil
}

func profileMetricRuleTrapDefs(rule *profileMetricRule, idx *Epoch) ([]*TrapDef, error) {
	var refs []string
	switch rule.Type {
	case MetricTypeCounter, MetricTypeSample:
		refs = append(refs, rule.OnTrap)
	case MetricTypeState:
		if rule.OnTrap != "" {
			refs = append(refs, rule.OnTrap)
		} else {
			refs = append(refs, rule.ProblemTrap, rule.ClearTrap)
		}
	}
	traps := make([]*TrapDef, 0, len(refs))
	for _, ref := range refs {
		td, err := resolveProfileMetricTrap(idx, ref)
		if err != nil {
			return nil, err
		}
		traps = append(traps, td)
	}
	return traps, nil
}

func isProfileMetricResourceKeyVarbind(vb *VarbindDef) bool {
	if vb == nil {
		return false
	}
	switch strings.ToLower(strings.TrimSpace(vb.Type)) {
	case "integer", "integer32", "unsigned32", "gauge32":
		return true
	default:
		return false
	}
}

func validateProfileMetricIdentity(rule *profileMetricRule) error {
	switch rule.Identity.Device {
	case "", MetricIdentitySource, MetricIdentitySourceLabel, MetricIdentityListener:
	default:
		return fmt.Errorf("identity.device must be source, source_label, or listener")
	}
	if rule.Identity.Resource == nil {
		return nil
	}
	res := rule.Identity.Resource
	if res.Class == "" || !profileMetricResourceClassRE.MatchString(res.Class) {
		return fmt.Errorf("identity.resource.class %q must match ^[a-z][a-z0-9_]*$", res.Class)
	}
	if !stockProfileMetricResourceClasses[res.Class] && !profileMetricSiteClassRE.MatchString(res.Class) {
		return fmt.Errorf("identity.resource.class %q must be a stock class or match ^site_[a-z0-9][a-z0-9_]*$", res.Class)
	}
	if res.KeyFromVarbind == "" {
		return fmt.Errorf("identity.resource.key_from_varbind is required")
	}
	if res.MaxPerSource < 0 {
		return fmt.Errorf("identity.resource.max_per_source must be zero or greater")
	}
	return nil
}

func isProfileMetricNumericVarbind(vb *VarbindDef) bool {
	if vb == nil {
		return false
	}
	switch strings.ToLower(strings.TrimSpace(vb.Type)) {
	case "integer", "integer32", "unsigned32", "gauge32", "counter32", "counter64", "timeticks":
		return true
	default:
		return false
	}
}

func normalizeProfileMetricChart(chart *profileMetricChart) error {
	if chart == nil {
		return errors.New("nil chart")
	}
	if chart.ID == "" || !profileMetricChartIDRE.MatchString(chart.ID) {
		return fmt.Errorf("chart id %q must match ^[a-z][a-z0-9_]*$", chart.ID)
	}
	if builtInProfileMetricChartIDs[chart.ID] {
		return fmt.Errorf("chart id %q collides with built-in SNMP trap chart", chart.ID)
	}
	if chart.Title == "" {
		return fmt.Errorf("chart %q title is required", chart.ID)
	}
	if chart.Context == "" {
		chart.Context = "snmp.trap." + chart.ID
	}
	if !strings.HasPrefix(chart.Context, "snmp.trap.") {
		return fmt.Errorf("chart %q context %q must start with snmp.trap.", chart.ID, chart.Context)
	}
	if builtInProfileMetricChartContexts[chart.Context] {
		return fmt.Errorf("chart %q context %q collides with built-in SNMP trap chart", chart.ID, chart.Context)
	}
	if chart.Units == "" {
		return fmt.Errorf("chart %q units is required", chart.ID)
	}
	if chart.Algorithm == "" {
		chart.Algorithm = "incremental"
	}
	switch chart.Algorithm {
	case "incremental", "absolute":
	default:
		return fmt.Errorf("chart %q algorithm %q is unsupported", chart.ID, chart.Algorithm)
	}
	if chart.Type == "" {
		chart.Type = "line"
	}
	switch chart.Type {
	case "line", "area", "stacked", "heatmap":
	default:
		return fmt.Errorf("chart %q type %q is unsupported", chart.ID, chart.Type)
	}
	if chart.Lifecycle == nil {
		chart.Lifecycle = &charttpl.Lifecycle{
			MaxInstances:      DefaultMetricChartMaxInstances,
			ExpireAfterCycles: DefaultMetricExpireAfterCycles,
		}
	}
	if chart.Lifecycle.MaxInstances <= 0 {
		chart.Lifecycle.MaxInstances = DefaultMetricChartMaxInstances
	}
	if chart.Lifecycle.ExpireAfterCycles <= 0 {
		chart.Lifecycle.ExpireAfterCycles = DefaultMetricExpireAfterCycles
	}
	return nil
}

func slugForMetric(name string) string {
	name = strings.ToLower(name)
	var b strings.Builder
	lastUnderscore := false
	for _, r := range name {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') {
			b.WriteRune(r)
			lastUnderscore = false
			continue
		}
		if !lastUnderscore {
			b.WriteByte('_')
			lastUnderscore = true
		}
	}
	return strings.Trim(b.String(), "_")
}
