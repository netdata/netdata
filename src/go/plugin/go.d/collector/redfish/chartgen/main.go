// SPDX-License-Identifier: GPL-3.0-or-later

// Command chartgen renders the immutable Redfish chart and health manifests
// from the executable registry and the approved Redfish alert policy.
package main

import (
	"bytes"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"sort"
	"strconv"
	"strings"

	"github.com/netdata/netdata/go/plugins/plugin/framework/charttpl"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/redfish/internal/registry"
	"gopkg.in/yaml.v3"
)

var check = flag.Bool("check", false, "fail when a checked-in manifest differs")

type groupKey struct {
	top, leaf, namespace, instances, promotions string
}

type artifactTarget struct {
	path   string
	render func() ([]byte, error)
}

func main() {
	flag.Parse()
	root, err := packageRoot()
	fatal(err)
	repository, err := repositoryRoot(root)
	fatal(err)
	contract, err := registry.Compile()
	fatal(err)
	for _, target := range artifactTargets(root, repository, contract) {
		raw, err := target.render()
		fatal(err)
		if *check {
			current, err := os.ReadFile(target.path)
			fatal(err)
			if !bytes.Equal(current, raw) {
				fatal(fmt.Errorf("%s is stale; run go generate ./plugin/go.d/collector/redfish", target.path))
			}
			continue
		}
		fatal(os.WriteFile(target.path, raw, 0o644))
	}
}

func artifactTargets(root, repository string, contract registry.Contract) []artifactTarget {
	return []artifactTarget{
		{
			path: filepath.Join(root, "charts.yaml"),
			render: func() ([]byte, error) {
				return renderCharts(contract, "redfish")
			},
		},
		{
			path: filepath.Join(filepath.Dir(root), "redfish_logs", "charts.yaml"),
			render: func() ([]byte, error) {
				return renderCharts(contract, "redfish_logs")
			},
		},
		{
			path: filepath.Join(repository, "src", "health", "health.d", "redfish.conf"),
			render: func() ([]byte, error) {
				return renderHealth(contract)
			},
		},
		{
			path: filepath.Join(root, "metadata.yaml"),
			render: func() ([]byte, error) {
				return renderMetadata(contract, "redfish", filepath.Join(root, "metadata.yaml.in"))
			},
		},
		{
			path: filepath.Join(filepath.Dir(root), "redfish_logs", "metadata.yaml"),
			render: func() ([]byte, error) {
				return renderMetadata(
					contract,
					"redfish_logs",
					filepath.Join(filepath.Dir(root), "redfish_logs", "metadata.yaml.in"),
				)
			},
		},
	}
}

func renderMetadata(contract registry.Contract, module, templatePath string) ([]byte, error) {
	template, err := os.ReadFile(templatePath)
	if err != nil {
		return nil, err
	}
	const marker = "{{GENERATED_ALERTS}}"
	if bytes.Count(template, []byte(marker)) != 1 {
		return nil, fmt.Errorf("%s must contain exactly one %s marker", templatePath, marker)
	}

	chartModules := make(map[string]string)
	for _, chart := range contract.Charts {
		if current, ok := chartModules[chart.Context]; ok && current != chart.Module {
			return nil, fmt.Errorf("context %q belongs to both %q and %q", chart.Context, current, chart.Module)
		}
		chartModules[chart.Context] = chart.Module
	}
	rules, err := compileHealthRules(contract)
	if err != nil {
		return nil, err
	}

	var alerts strings.Builder
	for _, rule := range rules {
		if chartModules[rule.Context] != module {
			continue
		}
		fmt.Fprintf(&alerts, "      - name: %s\n", rule.Name)
		fmt.Fprintf(&alerts, "        metric: %s\n", rule.Context)
		fmt.Fprintf(&alerts, "        info: %s\n", strconv.Quote(rule.Info))
		fmt.Fprintln(
			&alerts,
			"        link: https://github.com/netdata/netdata/blob/master/src/health/health.d/redfish.conf",
		)
	}
	return bytes.Replace(template, []byte(marker), []byte(strings.TrimSuffix(alerts.String(), "\n")), 1), nil
}

func renderCharts(contract registry.Contract, module string) ([]byte, error) {
	grouped := make(map[groupKey][]registry.ChartSpec)
	for _, chart := range contract.Charts {
		if chart.Module != module {
			continue
		}
		namespace, _, err := chartContext(chart.Context)
		if err != nil {
			return nil, err
		}
		key := groupKey{
			top: chart.TopFamily, leaf: chart.LeafFamily, namespace: namespace,
			instances:  strings.Join(chart.InstanceLabels, "\x00"),
			promotions: strings.Join(chart.PromotedLabels, "\x00"),
		}
		grouped[key] = append(grouped[key], chart)
	}
	keys := make([]groupKey, 0, len(grouped))
	for key := range grouped {
		keys = append(keys, key)
	}
	sort.Slice(keys, func(i, j int) bool {
		left, right := keys[i], keys[j]
		if compared := registry.CompareTopFamily(left.top, right.top); compared != 0 {
			return compared < 0
		}
		if left.leaf != right.leaf {
			return left.leaf < right.leaf
		}
		if left.namespace != right.namespace {
			return left.namespace < right.namespace
		}
		if left.instances != right.instances {
			return left.instances < right.instances
		}
		return left.promotions < right.promotions
	})

	byTop := make(map[string][]charttpl.Group)
	for _, key := range keys {
		charts := grouped[key]
		sort.Slice(charts, func(i, j int) bool { return charts[i].Priority < charts[j].Priority })
		group := charttpl.Group{
			Family: key.leaf, ContextNamespace: key.namespace,
			ChartDefaults: &charttpl.ChartDefaults{
				Instances:     &charttpl.Instances{ByLabels: splitKey(key.instances)},
				LabelPromoted: splitKey(key.promotions),
			},
		}
		metricSet := make(map[string]struct{})
		for _, source := range charts {
			for _, dimension := range source.Dimensions {
				metricSet[dimension.Metric] = struct{}{}
			}
			_, relative, err := chartContext(source.Context)
			if err != nil {
				return nil, err
			}
			chart := charttpl.Chart{
				ID: source.ID, Title: source.Title, Context: relative,
				Units: source.Units, Type: string(source.Type), Priority: source.Priority,
				Lifecycle: &charttpl.Lifecycle{ExpireAfterCycles: source.ExpireAfter},
			}
			if len(source.Dimensions) > 0 {
				chart.Algorithm = source.Dimensions[0].Algorithm
			}
			for _, dimension := range source.Dimensions {
				var options *charttpl.DimensionOptions
				if dimension.Float {
					options = &charttpl.DimensionOptions{Float: true}
				}
				chart.Dimensions = append(chart.Dimensions, charttpl.Dimension{
					Selector: dimension.Selector, Name: dimension.Name, Options: options,
				})
			}
			group.Charts = append(group.Charts, chart)
		}
		for metric := range metricSet {
			group.Metrics = append(group.Metrics, metric)
		}
		sort.Strings(group.Metrics)
		byTop[key.top] = append(byTop[key.top], group)
	}

	spec := charttpl.Spec{Version: charttpl.VersionV1}
	topNames := make([]string, 0, len(byTop))
	for top := range byTop {
		topNames = append(topNames, top)
	}
	sort.Slice(topNames, func(i, j int) bool {
		return registry.CompareTopFamily(topNames[i], topNames[j]) < 0
	})
	for _, top := range topNames {
		spec.Groups = append(spec.Groups, charttpl.Group{Family: top, Groups: byTop[top]})
	}
	return marshalChartTemplate(spec)
}

func marshalChartTemplate(spec charttpl.Spec) ([]byte, error) {
	if err := spec.Validate(); err != nil {
		return nil, err
	}

	var output bytes.Buffer
	encoder := yaml.NewEncoder(&output)
	encoder.SetIndent(2)
	if err := encoder.Encode(spec); err != nil {
		_ = encoder.Close()
		return nil, err
	}
	if err := encoder.Close(); err != nil {
		return nil, err
	}
	return output.Bytes(), nil
}

type healthRule struct {
	Name        string
	Context     string
	Class       string
	Component   string
	ChartLabels string
	Calculation string
	Units       string
	Warning     string
	Critical    string
	Delay       string
	Summary     string
	Info        string
}

func renderHealth(contract registry.Contract) ([]byte, error) {
	rules, err := compileHealthRules(contract)
	if err != nil {
		return nil, err
	}

	var output strings.Builder
	output.WriteString("# SPDX-License-Identifier: GPL-3.0-or-later\n")
	output.WriteString("#\n")
	output.WriteString("# Generated by src/go/plugin/go.d/collector/redfish/chartgen.\n")
	output.WriteString("# Do not edit directly; update the executable Redfish registry or approved alert policy.\n")
	for _, rule := range rules {
		fmt.Fprintf(&output, "\n template: %s\n", rule.Name)
		fmt.Fprintf(&output, "       on: %s\n", rule.Context)
		fmt.Fprintf(&output, "    class: %s\n", rule.Class)
		fmt.Fprintln(&output, "     type: System")
		fmt.Fprintf(&output, "component: %s\n", rule.Component)
		if rule.ChartLabels != "" {
			fmt.Fprintf(&output, "chart labels: %s\n", rule.ChartLabels)
		}
		fmt.Fprintf(&output, "     calc: %s\n", rule.Calculation)
		fmt.Fprintf(&output, "    units: %s\n", rule.Units)
		fmt.Fprintln(&output, "    every: 10s")
		if rule.Warning != "" {
			fmt.Fprintf(&output, "     warn: %s\n", rule.Warning)
		}
		if rule.Critical != "" {
			fmt.Fprintf(&output, "     crit: %s\n", rule.Critical)
		}
		fmt.Fprintf(&output, "    delay: %s\n", rule.Delay)
		fmt.Fprintf(&output, "  summary: %s\n", rule.Summary)
		fmt.Fprintf(&output, "     info: %s\n", rule.Info)
		fmt.Fprintln(&output, "       to: sysadmin")
	}
	return []byte(output.String()), nil
}

func compileHealthRules(contract registry.Contract) ([]healthRule, error) {
	byContext := make(map[string]registry.ChartSpec)
	for _, chart := range contract.Charts {
		if chart.Module != "redfish" && chart.Module != "redfish_logs" {
			continue
		}
		if current, ok := byContext[chart.Context]; ok {
			if !sameDimensionIDs(current.Dimensions, chart.Dimensions) {
				return nil, fmt.Errorf("context %q has incompatible chart dimensions", chart.Context)
			}
			continue
		}
		byContext[chart.Context] = chart
	}

	var rules []healthRule
	for _, chart := range byContext {
		rules = append(rules, healthRulesForChart(chart)...)
	}
	sort.Slice(rules, func(i, j int) bool {
		if rules[i].Context != rules[j].Context {
			return rules[i].Context < rules[j].Context
		}
		return rules[i].Name < rules[j].Name
	})

	seenNames := make(map[string]struct{}, len(rules))
	for _, rule := range rules {
		if _, ok := seenNames[rule.Name]; ok {
			return nil, fmt.Errorf("duplicate health template name %q", rule.Name)
		}
		seenNames[rule.Name] = struct{}{}
		if _, ok := byContext[rule.Context]; !ok {
			return nil, fmt.Errorf("health template %q references unknown context %q", rule.Name, rule.Context)
		}
	}
	return rules, nil
}

func healthRulesForChart(chart registry.ChartSpec) []healthRule {
	context := chart.Context
	hardware := func(calc, warn, crit string) healthRule {
		return newHealthRule(chart, "Errors", "Hardware", calc, warn, crit, hardwareAlertDelay)
	}
	operational := func(class, calc, warn, crit string) healthRule {
		return newHealthRule(chart, class, "Redfish", calc, warn, crit, operationalAlertDelay)
	}

	switch context {
	case "redfish.collection.status":
		return []healthRule{operational("Availability", "$partial + $unavailable", "$partial > 0", "$unavailable > 0")}
	case "redfish.collection.logs_admission":
		return []healthRule{operational(
			"Errors",
			"$over_limit + $duplicate_owner",
			"$over_limit > 0 OR $duplicate_owner > 0",
			"",
		)}
	case "redfish.collection.selected_system":
		return []healthRule{operational("Availability", "$unreadable + $absent", "$unreadable > 0", "$absent > 0")}
	case "redfish.log_backend.state":
		return []healthRule{operational("Availability", "$unavailable", "", "$unavailable > 0")}
	case "redfish.log_backend.pipeline":
		return []healthRule{operational("Errors", "$write_failed", "$this > 0", "")}
	case "redfish.log_service.ingestion_state":
		subject := healthSubject(chart)
		warning := operational("Errors", "$blocked_source", "$this > 0", "")
		warning.Name += "_warning"
		warning.Summary = subject + " log ingestion blocked"
		warning.Info = subject + " log ingestion has remained blocked at its source for at least five minutes"
		critical := operational("Errors", "$blocked_source", "", "$this > 0")
		critical.Name += "_critical"
		critical.Delay = "up 30m down 5m multiplier 1.5 max 1h"
		critical.Summary = subject + " log ingestion persistently blocked"
		critical.Info = subject + " log ingestion has remained blocked at its source for at least thirty minutes"
		return []healthRule{warning, critical}
	case "redfish.chassis.intrusion_state":
		return []healthRule{hardware(
			"$hardware_intrusion + $tampering_detected",
			"",
			"$hardware_intrusion > 0 OR $tampering_detected > 0",
		)}
	case "redfish.processor.throttling_state":
		return []healthRule{hardware("$throttled", "$throttled > 0", "")}
	case "redfish.drive.status_indicator":
		return []healthRule{hardware(
			"$rebuild + $predictive_failure_analysis + $fail + $in_a_critical_array + $in_a_failed_array",
			"$rebuild > 0 OR $predictive_failure_analysis > 0",
			"$fail > 0 OR $in_a_critical_array > 0 OR $in_a_failed_array > 0",
		)}
	case "redfish.volume.write_cache_state":
		return []healthRule{hardware("$degraded + $unprotected", "$degraded > 0", "$unprotected > 0")}
	case "redfish.power_supply.line_input_status":
		return []healthRule{hardware("$out_of_range + $loss_of_input", "$out_of_range > 0", "$loss_of_input > 0")}
	case "redfish.leak_detector.detector_state":
		return []healthRule{hardware("$warning + $critical", "$warning > 0", "$critical > 0")}
	case "redfish.log_service.overflow_state":
		return []healthRule{hardware("$overflow", "", "$overflow > 0")}
	}

	switch {
	case chart.Class == registry.ClassReadingAlarm ||
		(chart.Class == registry.ClassCategoricalParent &&
			(context == "redfish.aggregate.reading_alarm" || strings.HasSuffix(context, ".alarm"))):
		return []healthRule{hardware(
			"$warning + $cap + $alarm + $critical + $emergency + $fault",
			"$warning > 0 OR $cap > 0 OR $alarm > 0",
			"$critical > 0 OR $emergency > 0 OR $fault > 0",
		)}
	case isHealthContext(context):
		return []healthRule{hardware("$warning + $critical", "$warning > 0", "$critical > 0")}
	case strings.HasSuffix(context, ".failure_predicted"):
		return []healthRule{hardware("$predicted", "", "$predicted > 0")}
	case strings.HasSuffix(context, ".conditions"):
		return []healthRule{hardware("$warning + $critical", "$warning > 0", "$critical > 0")}
	default:
		return nil
	}
}

const (
	hardwareAlertDelay    = "up 5m down 15m multiplier 1.5 max 1h"
	operationalAlertDelay = "up 5m down 5m multiplier 1.5 max 1h"
)

func newHealthRule(
	chart registry.ChartSpec,
	class, component, calculation, warning, critical, delay string,
) healthRule {
	labels := ""
	if strings.HasPrefix(chart.Context, "system.hw.sensor.") {
		labels = "_collect_module=redfish"
	}
	name := "redfish_" + sanitizeHealthName(strings.TrimPrefix(chart.Context, "redfish."))
	subject := healthSubject(chart)
	return healthRule{
		Name:        name,
		Context:     chart.Context,
		Class:       class,
		Component:   component,
		ChartLabels: labels,
		Calculation: calculation,
		Units:       chart.Units,
		Warning:     warning,
		Critical:    critical,
		Delay:       delay,
		Summary:     subject + " " + chart.Title,
		Info:        subject + " reports a non-healthy state for " + chart.Title,
	}
}

func healthSubject(chart registry.ChartSpec) string {
	switch {
	case slices.Contains(chart.InstanceLabels, "backend_key"):
		return "Redfish backend ${label:backend_name}"
	case slices.Contains(chart.InstanceLabels, "aggregate_key"):
		return "Redfish ${label:rollup_owner_name}"
	case slices.Contains(chart.InstanceLabels, "reading_key"):
		return "Redfish ${label:resource_name} reading ${label:reading_key}"
	case slices.Contains(chart.InstanceLabels, "resource_key"):
		return "Redfish ${label:resource_name}"
	default:
		return "Redfish endpoint ${label:endpoint_job}"
	}
}

func isHealthContext(context string) bool {
	return strings.HasSuffix(context, ".health") || strings.HasSuffix(context, ".health_rollup")
}

func sanitizeHealthName(value string) string {
	return strings.NewReplacer(".", "_", "-", "_").Replace(value)
}

func sameDimensionIDs(left, right []registry.DimensionSpec) bool {
	if len(left) != len(right) {
		return false
	}
	for index := range left {
		if left[index].ID != right[index].ID {
			return false
		}
	}
	return true
}

func chartContext(context string) (string, string, error) {
	if strings.HasPrefix(context, "system.hw.sensor.") {
		parts := strings.Split(context, ".")
		if len(parts) <= 4 {
			return "", "", fmt.Errorf("registry produced incomplete chart context %q", context)
		}
		return strings.Join(parts[:4], "."), strings.Join(parts[4:], "."), nil
	}
	if after, ok := strings.CutPrefix(context, "redfish."); ok {
		if after == "" {
			return "", "", fmt.Errorf("registry produced incomplete chart context %q", context)
		}
		return "redfish", after, nil
	}
	return "", "", fmt.Errorf("registry produced unsupported chart context %q", context)
}

func splitKey(value string) []string {
	if value == "" {
		return nil
	}
	return slices.DeleteFunc(strings.Split(value, "\x00"), func(item string) bool { return item == "" })
}

func packageRoot() (string, error) {
	root, err := os.Getwd()
	if err != nil {
		return "", err
	}
	for filepath.Base(root) != "redfish" {
		parent := filepath.Dir(root)
		if parent == root {
			return "", fmt.Errorf("run chartgen from the redfish package tree")
		}
		root = parent
	}
	return root, nil
}

func repositoryRoot(packageRoot string) (string, error) {
	root := packageRoot
	for {
		healthDirectory := filepath.Join(root, "src", "health", "health.d")
		info, err := os.Stat(healthDirectory)
		if err == nil && info.IsDir() {
			return root, nil
		}
		parent := filepath.Dir(root)
		if parent == root {
			return "", fmt.Errorf("repository root containing src/health/health.d was not found")
		}
		root = parent
	}
}

func fatal(err error) {
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
