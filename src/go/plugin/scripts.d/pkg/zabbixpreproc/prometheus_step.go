package zabbixpreproc

import (
	"encoding/json"
	"fmt"
	"regexp"
	"strconv"
	"strings"
)

// Prometheus regex patterns compiled once at package init
var (
	// Format: metric_name{label="value"} number
	prometheusMetricWithLabelsRegex = regexp.MustCompile(`^([a-zA-Z_:][a-zA-Z0-9_:]*)\{([^}]*)\}\s+([-+]?[\d.eE+-]+)`)
	// Format: metric_name number
	prometheusMetricWithoutLabelsRegex = regexp.MustCompile(`^([a-zA-Z_:][a-zA-Z0-9_:]*)\s+([-+]?[\d.eE+-]+)`)
	// Format: label_name="label_value" (NOTE: this regex doesn't handle escaped quotes)
	// Use parsePrometheusLabels() for proper escape handling
	prometheusLabelRegex = regexp.MustCompile(`([a-zA-Z_][a-zA-Z0-9_]*)="([^"]*)"`)
)

// parsePrometheusLabels parses Prometheus label string with proper escape handling.
// Handles \", \\, and \n escape sequences per Prometheus spec.
// Input: `method="GET",path="/foo\"bar",code="200"`
// Returns map and order of labels.
func parsePrometheusLabels(labelsStr string) (map[string]string, []string) {
	labels := make(map[string]string)
	var labelOrder []string

	if strings.TrimSpace(labelsStr) == "" {
		return labels, labelOrder
	}

	i := 0
	n := len(labelsStr)

	for i < n {
		// Skip whitespace and commas
		for i < n && (labelsStr[i] == ' ' || labelsStr[i] == ',' || labelsStr[i] == '\t') {
			i++
		}
		if i >= n {
			break
		}

		// Parse label name (alphanumeric + underscore, starts with letter or underscore)
		nameStart := i
		for i < n && (labelsStr[i] == '_' || (labelsStr[i] >= 'a' && labelsStr[i] <= 'z') ||
			(labelsStr[i] >= 'A' && labelsStr[i] <= 'Z') || (labelsStr[i] >= '0' && labelsStr[i] <= '9')) {
			i++
		}
		if i == nameStart {
			// No valid label name found, skip rest
			break
		}
		labelName := labelsStr[nameStart:i]

		// Skip whitespace
		for i < n && (labelsStr[i] == ' ' || labelsStr[i] == '\t') {
			i++
		}

		// Expect '='
		if i >= n || labelsStr[i] != '=' {
			break
		}
		i++

		// Skip whitespace
		for i < n && (labelsStr[i] == ' ' || labelsStr[i] == '\t') {
			i++
		}

		// Expect opening quote
		if i >= n || labelsStr[i] != '"' {
			break
		}
		i++

		// Parse label value with escape handling
		var valueBuilder strings.Builder
		for i < n {
			if labelsStr[i] == '\\' && i+1 < n {
				// Escape sequence
				nextChar := labelsStr[i+1]
				switch nextChar {
				case '"':
					valueBuilder.WriteByte('"')
					i += 2
				case '\\':
					valueBuilder.WriteByte('\\')
					i += 2
				case 'n':
					valueBuilder.WriteByte('\n')
					i += 2
				default:
					// Unknown escape, keep as-is
					valueBuilder.WriteByte(labelsStr[i])
					i++
				}
			} else if labelsStr[i] == '"' {
				// End of value
				i++
				break
			} else {
				valueBuilder.WriteByte(labelsStr[i])
				i++
			}
		}

		labels[labelName] = valueBuilder.String()
		labelOrder = append(labelOrder, labelName)
	}

	return labels, labelOrder
}

// prometheusMetric represents a parsed Prometheus metric
type prometheusMetric struct {
	Name       string
	Labels     map[string]string
	LabelOrder []string // Order of label keys as they appear in input
	Value      float64
	ValueStr   string // Original string representation of value
	LineRaw    string
}

// parsePrometheusText parses Prometheus text exposition format.
//
// Parses metrics in Prometheus text format as defined by the Prometheus documentation.
// Supports:
//   - Metrics with labels: http_requests_total{method="GET",code="200"} 1027
//   - Metrics without labels: process_cpu_seconds 4.2
//   - Scientific notation: very_large_value 1.23e+10
//   - Comment lines (ignored): # HELP and # TYPE comments
//   - Label validation: ensures all labels match pattern label_name="value"
//
// Example Prometheus format:
//
//	# HELP http_requests_total Total HTTP requests
//	# TYPE http_requests_total counter
//	http_requests_total{method="GET"} 100
//	http_requests_total{method="POST"} 50
//	memory_usage_bytes 1024
//
// Parameters:
//   - data: Prometheus text format data (one metric per line)
//
// Returns:
//   - []prometheusMetric: Parsed metrics with names, labels, and values
//   - error: Returns nil on success (invalid lines are silently skipped per Prometheus spec)
//
// Note: This parser uses pre-compiled package-level regexes for performance.
func parsePrometheusText(data string) ([]prometheusMetric, error) {
	var metrics []prometheusMetric
	lines := strings.Split(data, "\n")

	for _, line := range lines {
		originalLine := line
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		var metricName, labelsStr, valueStr string
		var labels map[string]string

		var labelOrder []string

		// Try with labels first
		matches := prometheusMetricWithLabelsRegex.FindStringSubmatch(line)
		if len(matches) >= 4 {
			metricName = matches[1]
			labelsStr = matches[2]
			valueStr = matches[3]

			labels = make(map[string]string)

			// Parse labels with proper escape handling (\", \\, \n)
			if strings.TrimSpace(labelsStr) != "" {
				labels, labelOrder = parsePrometheusLabels(labelsStr)
				if len(labels) == 0 {
					// Labels present but none valid - invalid syntax
					continue
				}
			}
		} else {
			// Try without labels
			matches = prometheusMetricWithoutLabelsRegex.FindStringSubmatch(line)
			if len(matches) < 3 {
				continue
			}
			metricName = matches[1]
			valueStr = matches[2]
			labels = make(map[string]string)
		}

		value, err := strconv.ParseFloat(valueStr, 64)
		if err != nil {
			continue
		}

		metrics = append(metrics, prometheusMetric{
			Name:       metricName,
			Labels:     labels,
			LabelOrder: labelOrder,
			Value:      value,
			ValueStr:   valueStr,
			LineRaw:    strings.TrimSpace(originalLine),
		})
	}

	return metrics, nil
}

// parsePrometheusMetadata extracts HELP and TYPE comments from Prometheus text
func parsePrometheusMetadata(data string) (map[string]string, map[string]string) {
	helpMap := make(map[string]string)
	typeMap := make(map[string]string)

	lines := strings.Split(data, "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if !strings.HasPrefix(line, "#") {
			continue
		}

		// Parse # HELP metric_name description
		if strings.HasPrefix(line, "# HELP ") {
			parts := strings.SplitN(line[7:], " ", 2)
			if len(parts) == 2 {
				helpMap[parts[0]] = parts[1]
			}
		}

		// Parse # TYPE metric_name type
		if strings.HasPrefix(line, "# TYPE ") {
			parts := strings.SplitN(line[7:], " ", 2)
			if len(parts) == 2 {
				typeMap[parts[0]] = parts[1]
			}
		}
	}

	return helpMap, typeMap
}

// prometheusPattern extracts metric values matching a label pattern.
func prometheusPattern(value Value, paramStr string) (Value, error) {
	// paramStr format: "metric_name{label_selectors}\noutput\nlabel_name"
	// output can be "value", "label"
	// If output is "label" and label_name is provided, return that specific label
	parts := strings.SplitN(paramStr, "\n", 3)
	if len(parts) < 1 {
		return Value{}, fmt.Errorf("prometheus pattern requires metric pattern")
	}

	// Parse metric pattern (e.g., "cpu_usage_system{cpu=\"cpu-total\",host=~\".*\"}")
	metricPattern := strings.TrimSpace(parts[0])
	outputType := "value" // Default
	labelName := ""
	if len(parts) > 1 {
		outputType = strings.TrimSpace(parts[1])
	}
	if len(parts) > 2 {
		labelName = strings.TrimSpace(parts[2])
	}

	// Extract metric name, label selectors, and value comparison from pattern
	metricName, labelSelectors, valueComp := parseMetricPattern(metricPattern)

	// Parse Prometheus text format
	metrics, err := parsePrometheusText(value.Data)
	if err != nil {
		return Value{}, fmt.Errorf("failed to parse prometheus data: %w", err)
	}

	// Find matching metric
	for _, m := range metrics {
		// Check if metric name matches
		// Priority: __name__ selector > explicit metric name > no filter
		nameSelector, hasNameSelector := labelSelectors["__name__"]
		if hasNameSelector {
			// Use __name__ selector to match metric name
			matched := false
			switch nameSelector.operator {
			case "=":
				matched = m.Name == nameSelector.value
			case "=~":
				re, err := compileRegexSafe(nameSelector.value, 0)
				if err != nil {
					continue
				}
				matched, err = matchWithTimeout(re, m.Name, defaultRegexTimeout)
				if err != nil {
					continue
				}
			case "!=":
				matched = m.Name != nameSelector.value
			}
			if !matched {
				continue
			}
		} else if metricName != "" {
			// Use metric name from pattern
			if !matchesPattern(m.Name, metricName) {
				continue
			}
		}

		// Filter out __name__ from label selectors (already checked above)
		actualLabelSelectors := make(map[string]labelSelector)
		for k, v := range labelSelectors {
			if k != "__name__" {
				actualLabelSelectors[k] = v
			}
		}

		if !matchesLabelSelectors(m.Labels, actualLabelSelectors) {
			continue
		}

		// Check value comparison if present
		if valueComp != nil {
			if !matchesValueComparison(m.Value, valueComp) {
				continue
			}
		}

		// Return based on output type
		switch outputType {
		case "value":
			return Value{Data: fmt.Sprintf("%g", m.Value), Type: ValueTypeStr}, nil
		case "label":
			if labelName != "" {
				// Return specific label value
				if val, ok := m.Labels[labelName]; ok {
					return Value{Data: val, Type: ValueTypeStr}, nil
				}
				return Value{}, fmt.Errorf("label %s not found", labelName)
			}
			// Return all labels as comma-separated key=value pairs
			var labelPairs []string
			for k, v := range m.Labels {
				labelPairs = append(labelPairs, fmt.Sprintf("%s=%s", k, v))
			}
			return Value{Data: strings.Join(labelPairs, ","), Type: ValueTypeStr}, nil
		default:
			// Unknown output type
			return Value{}, fmt.Errorf("unknown output type: %s", outputType)
		}
	}

	return Value{}, fmt.Errorf("prometheus pattern did not match any metrics")
}

// prometheusToJSON converts Prometheus text format to JSON.
// prometheusToJSONMulti converts Prometheus text format to multiple Result metrics
// Each Prometheus metric becomes a separate Result.Metric with labels
func prometheusToJSONMulti(value Value, paramStr string) (Result, error) {
	// Parse Prometheus text format
	metrics, err := parsePrometheusText(value.Data)
	if err != nil {
		return Result{Error: err}, err
	}

	// If input is non-empty but no metrics were parsed, it's invalid data
	if len(metrics) == 0 && strings.TrimSpace(value.Data) != "" {
		err := fmt.Errorf("failed to parse prometheus data: no valid metrics found")
		return Result{Error: err}, err
	}

	// Convert each Prometheus metric to Result.Metric
	resultMetrics := make([]Metric, 0, len(metrics))
	for _, pm := range metrics {
		resultMetrics = append(resultMetrics, Metric{
			Name:   pm.Name,
			Value:  pm.ValueStr,
			Type:   ValueTypeStr,
			Labels: pm.Labels, // Prometheus labels preserved
		})
	}

	return Result{Metrics: resultMetrics}, nil
}

func prometheusToJSON(value Value, paramStr string) (Value, error) {
	// Parse Prometheus text format
	metrics, err := parsePrometheusText(value.Data)
	if err != nil {
		return Value{}, fmt.Errorf("failed to parse prometheus data: %w", err)
	}

	// If input is non-empty but no metrics were parsed, it's invalid data
	if len(metrics) == 0 && strings.TrimSpace(value.Data) != "" {
		return Value{}, fmt.Errorf("failed to parse Prometheus data: no valid metrics found")
	}

	// Parse metadata (HELP and TYPE comments)
	helpMap, typeMap := parsePrometheusMetadata(value.Data)

	// Build result array with ordered fields
	var jsonParts []string

	for _, m := range metrics {
		// Build JSON manually to maintain field order: name, value, line_raw, labels, type, help
		var parts []string

		parts = append(parts, fmt.Sprintf(`"name":%s`, jsonString(m.Name)))
		parts = append(parts, fmt.Sprintf(`"value":%s`, jsonString(m.ValueStr)))
		parts = append(parts, fmt.Sprintf(`"line_raw":%s`, jsonString(m.LineRaw)))

		if len(m.Labels) > 0 {
			// Build labels JSON with preserved order
			var labelPairs []string
			for _, key := range m.LabelOrder {
				labelPairs = append(labelPairs, fmt.Sprintf(`%s:%s`, jsonString(key), jsonString(m.Labels[key])))
			}
			parts = append(parts, fmt.Sprintf(`"labels":{%s}`, strings.Join(labelPairs, ",")))
		}

		// Add type (default to "untyped")
		metricType := "untyped"
		if t, ok := typeMap[m.Name]; ok {
			metricType = t
		}
		parts = append(parts, fmt.Sprintf(`"type":%s`, jsonString(metricType)))

		// Add help if available
		if helpText, ok := helpMap[m.Name]; ok {
			parts = append(parts, fmt.Sprintf(`"help":%s`, jsonString(helpText)))
		}

		jsonParts = append(jsonParts, "{"+strings.Join(parts, ",")+"}")
	}

	resultJSON := "[" + strings.Join(jsonParts, ",") + "]"
	return Value{Data: resultJSON, Type: ValueTypeStr}, nil
}

// jsonString escapes a string for JSON
func jsonString(s string) string {
	b, _ := json.Marshal(s)
	return string(b)
}

// matchesPattern checks if a string matches a simple wildcard pattern
func matchesPattern(s, pattern string) bool {
	// Zabbix uses simple glob patterns, convert to regex
	regexPattern := regexp.QuoteMeta(pattern)
	regexPattern = strings.ReplaceAll(regexPattern, `\*`, ".*")
	regexPattern = strings.ReplaceAll(regexPattern, `\?`, ".")
	regexPattern = "^" + regexPattern + "$"
	re, err := compileRegexSafe(regexPattern, 0)
	if err != nil {
		return false
	}
	matched, err := matchWithTimeout(re, s, defaultRegexTimeout)
	if err != nil {
		return false
	}
	return matched
}

// parseMetricPattern parses a metric pattern like "metric_name{label=\"value\",label2=~\"regex\"} == 123.45"
// Returns the metric name, label selectors, and optional value comparison
func parseMetricPattern(pattern string) (string, map[string]labelSelector, *valueComparison) {
	// Check for value comparison operators (==, !=, >, <, >=, <=)
	var valueComp *valueComparison

	// Find value comparison operator outside of braces
	braceDepth := 0
	var compIdx int = -1
	var compOp string

	for i := 0; i < len(pattern); i++ {
		if pattern[i] == '{' {
			braceDepth++
		} else if pattern[i] == '}' {
			braceDepth--
		} else if braceDepth == 0 {
			// Check for comparison operators outside braces
			if i+2 <= len(pattern) {
				twoChar := pattern[i:min(i+2, len(pattern))]
				if twoChar == "==" || twoChar == "!=" || twoChar == ">=" || twoChar == "<=" {
					compOp = twoChar
					compIdx = i
					break
				}
			}
			if pattern[i] == '>' || pattern[i] == '<' {
				compOp = string(pattern[i])
				compIdx = i
				break
			}
		}
	}

	// Extract value comparison if present
	if compIdx != -1 {
		valueStr := strings.TrimSpace(pattern[compIdx+len(compOp):])
		pattern = strings.TrimSpace(pattern[:compIdx])
		if valueStr != "" {
			val, err := strconv.ParseFloat(valueStr, 64)
			if err == nil {
				valueComp = &valueComparison{
					operator: compOp,
					value:    val,
				}
			}
		}
	}

	// Find the opening brace
	braceIdx := strings.Index(pattern, "{")
	if braceIdx == -1 {
		// No labels specified
		return pattern, nil, valueComp
	}

	metricName := pattern[:braceIdx]
	labelsStr := pattern[braceIdx+1:]

	// Remove trailing }
	if strings.HasSuffix(labelsStr, "}") {
		labelsStr = labelsStr[:len(labelsStr)-1]
	}

	// Parse label selectors
	selectors := make(map[string]labelSelector)
	if labelsStr == "" {
		return metricName, selectors, valueComp
	}

	// Split by comma, but respect quoted strings
	labelParts := splitLabelSelectors(labelsStr)
	for _, part := range labelParts {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}

		// Check for regex operator =~ or !=
		var op string
		var splitIdx int
		if idx := strings.Index(part, "=~"); idx != -1 {
			op = "=~"
			splitIdx = idx
		} else if idx := strings.Index(part, "!="); idx != -1 {
			op = "!="
			splitIdx = idx
		} else if idx := strings.Index(part, "="); idx != -1 {
			op = "="
			splitIdx = idx
		} else {
			continue
		}

		labelName := strings.TrimSpace(part[:splitIdx])
		labelValue := strings.TrimSpace(part[splitIdx+len(op):])

		// Remove quotes from value
		labelValue = strings.Trim(labelValue, "\"")

		selectors[labelName] = labelSelector{
			operator: op,
			value:    labelValue,
		}
	}

	return metricName, selectors, valueComp
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

// labelSelector represents a label matching condition
type labelSelector struct {
	operator string // "=", "=~", "!="
	value    string
}

// valueComparison represents a metric value comparison
type valueComparison struct {
	operator string // "==", "!=", ">", "<", ">=", "<="
	value    float64
}

// splitLabelSelectors splits label selectors by comma, respecting quotes
func splitLabelSelectors(s string) []string {
	var parts []string
	var current strings.Builder
	inQuotes := false

	for i := 0; i < len(s); i++ {
		ch := s[i]

		if ch == '"' {
			inQuotes = !inQuotes
			current.WriteByte(ch)
		} else if ch == ',' && !inQuotes {
			parts = append(parts, current.String())
			current.Reset()
		} else {
			current.WriteByte(ch)
		}
	}

	if current.Len() > 0 {
		parts = append(parts, current.String())
	}

	return parts
}

// matchesLabelSelectors checks if labels match all selectors
// Also checks metric name if __name__ selector is present
func matchesLabelSelectors(labels map[string]string, selectors map[string]labelSelector) bool {
	if len(selectors) == 0 {
		return true
	}

	for labelName, selector := range selectors {
		labelValue, exists := labels[labelName]

		switch selector.operator {
		case "=":
			if !exists || labelValue != selector.value {
				return false
			}
		case "=~":
			if !exists {
				return false
			}
			// Treat as regex
			re, err := compileRegexSafe(selector.value, 0)
			if err != nil {
				return false
			}
			matched, err := matchWithTimeout(re, labelValue, defaultRegexTimeout)
			if err != nil || !matched {
				return false
			}
		case "!=":
			if exists && labelValue == selector.value {
				return false
			}
		}
	}

	return true
}

// matchesValueComparison checks if a metric value matches a comparison
func matchesValueComparison(metricValue float64, comp *valueComparison) bool {
	switch comp.operator {
	case "==":
		return metricValue == comp.value
	case "!=":
		return metricValue != comp.value
	case ">":
		return metricValue > comp.value
	case "<":
		return metricValue < comp.value
	case ">=":
		return metricValue >= comp.value
	case "<=":
		return metricValue <= comp.value
	default:
		return false
	}
}
