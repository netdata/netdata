package zabbixpreproc

import (
	"encoding/json"
	"testing"
)

// TestXMLToJSONZabbixDocumentation tests all examples from official Zabbix docs.
// Source: https://www.zabbix.com/documentation/6.0/en/manual/config/items/preprocessing/javascript/javascript_objects
func TestXMLToJSONZabbixDocumentation(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
		rule     string
	}{
		{
			name:     "Rule 1: Attributes prepended with @",
			input:    `<xml foo="FOO"><bar><baz>BAZ</baz></bar></xml>`,
			expected: `{"xml":{"@foo":"FOO","bar":{"baz":"BAZ"}}}`,
			rule:     "XML attributes will be converted to keys that have their names prepended with '@'",
		},
		{
			name:     "Rule 2: Self-closing elements become null",
			input:    `<xml><foo/></xml>`,
			expected: `{"xml":{"foo":null}}`,
			rule:     "Self-closing elements (<foo/>) will be converted as having 'null' value",
		},
		{
			name:     "Rule 3: Empty attributes preserved as empty strings",
			input:    `<xml><foo bar="" /></xml>`,
			expected: `{"xml":{"foo":{"@bar":""}}}`,
			rule:     "Empty attributes (with \"\" value) will be converted as having empty string ('') value",
		},
		{
			name:     "Rule 4: Multiple same-named children become arrays",
			input:    `<xml><foo>BAR</foo><foo>BAZ</foo><foo>QUX</foo></xml>`,
			expected: `{"xml":{"foo":["BAR","BAZ","QUX"]}}`,
			rule:     "Multiple child nodes with the same element name will be converted to a single key that has an array of values",
		},
		{
			name:     "Rule 5: Simple text elements become strings",
			input:    `<xml><foo>BAZ</foo></xml>`,
			expected: `{"xml":{"foo":"BAZ"}}`,
			rule:     "If a text element has no attributes and no children, it will be converted as a string",
		},
		{
			name:     "Rule 6: Text with attributes uses #text",
			input:    `<xml><foo bar="BAR">BAZ</foo></xml>`,
			expected: `{"xml":{"foo":{"@bar":"BAR","#text":"BAZ"}}}`,
			rule:     "If a text element has no children but has attributes: text content will be converted to an element with the key '#text'",
		},
	}

	p := NewPreprocessor("test-shard")

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			input := Value{Data: tt.input, Type: ValueTypeStr}
			step := Step{Type: StepTypeXMLToJSON}

			result, err := p.Execute("item1", input, step)
			if err != nil {
				t.Fatalf("XML to JSON failed: %v", err)
			}

			if len(result.Metrics) == 0 {
				t.Fatal("Expected metrics in result")
			}

			// Compare as JSON structures (order-independent)
			var expected, actual interface{}
			if err := json.Unmarshal([]byte(tt.expected), &expected); err != nil {
				t.Fatalf("Failed to parse expected JSON: %v", err)
			}
			if err := json.Unmarshal([]byte(result.Metrics[0].Value), &actual); err != nil {
				t.Fatalf("Failed to parse actual JSON: %v\nGot: %s", err, result.Metrics[0].Value)
			}

			// Marshal back to normalize for comparison
			expectedNorm, _ := json.Marshal(expected)
			actualNorm, _ := json.Marshal(actual)

			if string(expectedNorm) != string(actualNorm) {
				t.Errorf("Rule: %s\nExpected: %s\nGot:      %s", tt.rule, tt.expected, result.Metrics[0].Value)
			}
		})
	}
}

// TestXMLToJSONEdgeCases tests edge cases not in official docs
func TestXMLToJSONEdgeCases(t *testing.T) {
	p := NewPreprocessor("test-shard")

	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "Nested elements with attributes",
			input:    `<root><parent id="1"><child name="test">value</child></parent></root>`,
			expected: `{"root":{"parent":{"@id":"1","child":{"@name":"test","#text":"value"}}}}`,
		},
		{
			name:     "Multiple different children",
			input:    `<data><a>1</a><b>2</b><c>3</c></data>`,
			expected: `{"data":{"a":"1","b":"2","c":"3"}}`,
		},
		{
			name:     "Empty root element",
			input:    `<empty/>`,
			expected: `{"empty":null}`,
		},
		{
			name:     "Root with only attributes",
			input:    `<config version="1.0" debug="true"/>`,
			expected: `{"config":{"@version":"1.0","@debug":"true"}}`,
		},
		{
			name:     "Mixed repeated and unique children",
			input:    `<list><item>A</item><item>B</item><unique>C</unique></list>`,
			expected: `{"list":{"item":["A","B"],"unique":"C"}}`,
		},
		{
			name:     "Deeply nested structure",
			input:    `<a><b><c><d>deep</d></c></b></a>`,
			expected: `{"a":{"b":{"c":{"d":"deep"}}}}`,
		},
		{
			name:     "Numeric text content",
			input:    `<metric><value>42</value><timestamp>1234567890</timestamp></metric>`,
			expected: `{"metric":{"value":"42","timestamp":"1234567890"}}`,
		},
		{
			name:     "Special characters in text",
			input:    `<data><msg>Hello &amp; World</msg></data>`,
			expected: `{"data":{"msg":"Hello & World"}}`,
		},
		{
			name:     "Boolean-like text values",
			input:    `<flags><enabled>true</enabled><disabled>false</disabled></flags>`,
			expected: `{"flags":{"enabled":"true","disabled":"false"}}`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			input := Value{Data: tt.input, Type: ValueTypeStr}
			step := Step{Type: StepTypeXMLToJSON}

			result, err := p.Execute("item1", input, step)
			if err != nil {
				t.Fatalf("XML to JSON failed: %v", err)
			}

			if len(result.Metrics) == 0 {
				t.Fatal("Expected metrics in result")
			}

			// Compare as JSON structures
			var expected, actual interface{}
			if err := json.Unmarshal([]byte(tt.expected), &expected); err != nil {
				t.Fatalf("Failed to parse expected JSON: %v", err)
			}
			if err := json.Unmarshal([]byte(result.Metrics[0].Value), &actual); err != nil {
				t.Fatalf("Failed to parse actual JSON: %v\nGot: %s", err, result.Metrics[0].Value)
			}

			expectedNorm, _ := json.Marshal(expected)
			actualNorm, _ := json.Marshal(actual)

			if string(expectedNorm) != string(actualNorm) {
				t.Errorf("Expected: %s\nGot:      %s", tt.expected, result.Metrics[0].Value)
			}
		})
	}
}

// TestXMLToJSONErrorHandling tests invalid XML inputs
func TestXMLToJSONErrorHandling(t *testing.T) {
	p := NewPreprocessor("test-shard")

	tests := []struct {
		name  string
		input string
	}{
		{
			name:  "Malformed XML - unclosed tag",
			input: `<root><unclosed>`,
		},
		{
			name:  "Malformed XML - mismatched tags",
			input: `<root><child></wrong></root>`,
		},
		{
			name:  "Empty input",
			input: ``,
		},
		{
			name:  "Not XML at all",
			input: `This is just plain text`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			input := Value{Data: tt.input, Type: ValueTypeStr}
			step := Step{Type: StepTypeXMLToJSON}

			_, err := p.Execute("item1", input, step)
			if err == nil {
				t.Errorf("Expected error for invalid XML input: %s", tt.input)
			}
		})
	}
}

// TestXMLToJSONLLDCompatible tests that output is compatible with Zabbix LLD
func TestXMLToJSONLLDCompatible(t *testing.T) {
	p := NewPreprocessor("test-shard")

	// Example: XML that could be used for LLD discovery
	input := `<discovery>
		<host name="server1" ip="192.168.1.1"/>
		<host name="server2" ip="192.168.1.2"/>
		<host name="server3" ip="192.168.1.3"/>
	</discovery>`

	step := Step{Type: StepTypeXMLToJSON}
	result, err := p.Execute("item1", Value{Data: input, Type: ValueTypeStr}, step)
	if err != nil {
		t.Fatalf("XML to JSON failed: %v", err)
	}

	// Parse result
	var data map[string]interface{}
	if err := json.Unmarshal([]byte(result.Metrics[0].Value), &data); err != nil {
		t.Fatalf("Failed to parse JSON: %v", err)
	}

	// Verify structure is compatible with LLD
	discovery, ok := data["discovery"].(map[string]interface{})
	if !ok {
		t.Fatal("Expected 'discovery' key in output")
	}

	hosts, ok := discovery["host"].([]interface{})
	if !ok {
		t.Fatal("Expected 'host' to be an array (multiple hosts)")
	}

	if len(hosts) != 3 {
		t.Errorf("Expected 3 hosts, got %d", len(hosts))
	}

	// Each host should have @name and @ip attributes
	for i, h := range hosts {
		host := h.(map[string]interface{})
		if _, ok := host["@name"]; !ok {
			t.Errorf("Host %d missing @name attribute", i)
		}
		if _, ok := host["@ip"]; !ok {
			t.Errorf("Host %d missing @ip attribute", i)
		}
	}
}
