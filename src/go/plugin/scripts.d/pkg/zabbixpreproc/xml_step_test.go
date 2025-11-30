package zabbixpreproc

import (
	"encoding/json"
	"testing"
	"time"
)

// TestXMLToJSON_Rule1_Attributes tests Rule 1: Attributes prepended with '@'
func TestXMLToJSON_Rule1_Attributes(t *testing.T) {
	tests := []struct {
		name     string
		xml      string
		expected string
	}{
		{
			name:     "Single attribute",
			xml:      `<xml foo="FOO"></xml>`,
			expected: `{"xml":{"@foo":"FOO"}}`,
		},
		{
			name:     "Multiple attributes",
			xml:      `<root id="123" name="test" active="true"></root>`,
			expected: `{"root":{"@active":"true","@id":"123","@name":"test"}}`,
		},
		{
			name:     "Nested element with attributes",
			xml:      `<root><child attr="value"/></root>`,
			expected: `{"root":{"child":{"@attr":"value"}}}`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			value := Value{
				Data:      tt.xml,
				Type:      ValueTypeStr,
				Timestamp: time.Now(),
			}

			result, err := xmlToJSON(value, "")
			if err != nil {
				t.Fatalf("xmlToJSON() error = %v", err)
			}

			// Compare JSON (normalize by parsing and re-marshaling)
			var got, want interface{}
			if err := json.Unmarshal([]byte(result.Data), &got); err != nil {
				t.Fatalf("Failed to parse result JSON: %v", err)
			}
			if err := json.Unmarshal([]byte(tt.expected), &want); err != nil {
				t.Fatalf("Failed to parse expected JSON: %v", err)
			}

			gotJSON, _ := json.Marshal(got)
			wantJSON, _ := json.Marshal(want)

			if string(gotJSON) != string(wantJSON) {
				t.Errorf("xmlToJSON() = %s, want %s", gotJSON, wantJSON)
			}
		})
	}
}

// TestXMLToJSON_Rule2_SelfClosingElements tests Rule 2: Self-closing elements become null
func TestXMLToJSON_Rule2_SelfClosingElements(t *testing.T) {
	tests := []struct {
		name     string
		xml      string
		expected string
	}{
		{
			name:     "Single self-closing element",
			xml:      `<root><foo/></root>`,
			expected: `{"root":{"foo":null}}`,
		},
		{
			name:     "Multiple self-closing elements",
			xml:      `<root><foo/><bar/><baz/></root>`,
			expected: `{"root":{"bar":null,"baz":null,"foo":null}}`,
		},
		{
			name:     "Root self-closing",
			xml:      `<empty/>`,
			expected: `{"empty":null}`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			value := Value{
				Data:      tt.xml,
				Type:      ValueTypeStr,
				Timestamp: time.Now(),
			}

			result, err := xmlToJSON(value, "")
			if err != nil {
				t.Fatalf("xmlToJSON() error = %v", err)
			}

			var got, want interface{}
			json.Unmarshal([]byte(result.Data), &got)
			json.Unmarshal([]byte(tt.expected), &want)

			gotJSON, _ := json.Marshal(got)
			wantJSON, _ := json.Marshal(want)

			if string(gotJSON) != string(wantJSON) {
				t.Errorf("xmlToJSON() = %s, want %s", gotJSON, wantJSON)
			}
		})
	}
}

// TestXMLToJSON_Rule3_EmptyAttributes tests Rule 3: Empty attributes preserved as empty strings
func TestXMLToJSON_Rule3_EmptyAttributes(t *testing.T) {
	tests := []struct {
		name     string
		xml      string
		expected string
	}{
		{
			name:     "Empty attribute value",
			xml:      `<root bar=""></root>`,
			expected: `{"root":{"@bar":""}}`,
		},
		{
			name:     "Mix of empty and non-empty attributes",
			xml:      `<root foo="FOO" bar="" baz="BAZ"></root>`,
			expected: `{"root":{"@bar":"","@baz":"BAZ","@foo":"FOO"}}`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			value := Value{
				Data:      tt.xml,
				Type:      ValueTypeStr,
				Timestamp: time.Now(),
			}

			result, err := xmlToJSON(value, "")
			if err != nil {
				t.Fatalf("xmlToJSON() error = %v", err)
			}

			var got, want interface{}
			json.Unmarshal([]byte(result.Data), &got)
			json.Unmarshal([]byte(tt.expected), &want)

			gotJSON, _ := json.Marshal(got)
			wantJSON, _ := json.Marshal(want)

			if string(gotJSON) != string(wantJSON) {
				t.Errorf("xmlToJSON() = %s, want %s", gotJSON, wantJSON)
			}
		})
	}
}

// TestXMLToJSON_Rule4_RepeatedElements tests Rule 4: Repeated elements consolidated into arrays
func TestXMLToJSON_Rule4_RepeatedElements(t *testing.T) {
	tests := []struct {
		name     string
		xml      string
		expected string
	}{
		{
			name:     "Three identical elements",
			xml:      `<root><foo>BAR</foo><foo>BAZ</foo><foo>QUX</foo></root>`,
			expected: `{"root":{"foo":["BAR","BAZ","QUX"]}}`,
		},
		{
			name:     "Two identical elements",
			xml:      `<root><item>A</item><item>B</item></root>`,
			expected: `{"root":{"item":["A","B"]}}`,
		},
		{
			name:     "Mixed single and repeated elements",
			xml:      `<root><foo>A</foo><bar>B</bar><foo>C</foo></root>`,
			expected: `{"root":{"bar":"B","foo":["A","C"]}}`,
		},
		{
			name:     "Repeated complex elements",
			xml:      `<root><item id="1">A</item><item id="2">B</item></root>`,
			expected: `{"root":{"item":[{"#text":"A","@id":"1"},{"#text":"B","@id":"2"}]}}`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			value := Value{
				Data:      tt.xml,
				Type:      ValueTypeStr,
				Timestamp: time.Now(),
			}

			result, err := xmlToJSON(value, "")
			if err != nil {
				t.Fatalf("xmlToJSON() error = %v", err)
			}

			var got, want interface{}
			json.Unmarshal([]byte(result.Data), &got)
			json.Unmarshal([]byte(tt.expected), &want)

			gotJSON, _ := json.Marshal(got)
			wantJSON, _ := json.Marshal(want)

			if string(gotJSON) != string(wantJSON) {
				t.Errorf("xmlToJSON() = %s, want %s", gotJSON, wantJSON)
			}
		})
	}
}

// TestXMLToJSON_Rule5_SimpleTextElements tests Rule 5: Simple text elements become direct strings
func TestXMLToJSON_Rule5_SimpleTextElements(t *testing.T) {
	tests := []struct {
		name     string
		xml      string
		expected string
	}{
		{
			name:     "Simple text element",
			xml:      `<root><foo>BAZ</foo></root>`,
			expected: `{"root":{"foo":"BAZ"}}`,
		},
		{
			name:     "Multiple simple text elements",
			xml:      `<root><name>John</name><age>30</age><city>NYC</city></root>`,
			expected: `{"root":{"age":"30","city":"NYC","name":"John"}}`,
		},
		{
			name:     "Nested simple text",
			xml:      `<root><outer><inner>value</inner></outer></root>`,
			expected: `{"root":{"outer":{"inner":"value"}}}`,
		},
		{
			name:     "Text with whitespace trimming",
			xml:      `<root><foo>  BAR  </foo></root>`,
			expected: `{"root":{"foo":"BAR"}}`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			value := Value{
				Data:      tt.xml,
				Type:      ValueTypeStr,
				Timestamp: time.Now(),
			}

			result, err := xmlToJSON(value, "")
			if err != nil {
				t.Fatalf("xmlToJSON() error = %v", err)
			}

			var got, want interface{}
			json.Unmarshal([]byte(result.Data), &got)
			json.Unmarshal([]byte(tt.expected), &want)

			gotJSON, _ := json.Marshal(got)
			wantJSON, _ := json.Marshal(want)

			if string(gotJSON) != string(wantJSON) {
				t.Errorf("xmlToJSON() = %s, want %s", gotJSON, wantJSON)
			}
		})
	}
}

// TestXMLToJSON_Rule6_TextWithAttributes tests Rule 6: Text with attributes uses #text key
func TestXMLToJSON_Rule6_TextWithAttributes(t *testing.T) {
	tests := []struct {
		name     string
		xml      string
		expected string
	}{
		{
			name:     "Text with single attribute",
			xml:      `<root><foo bar="BAR">BAZ</foo></root>`,
			expected: `{"root":{"foo":{"#text":"BAZ","@bar":"BAR"}}}`,
		},
		{
			name:     "Text with multiple attributes",
			xml:      `<root><item id="1" type="text">Value</item></root>`,
			expected: `{"root":{"item":{"#text":"Value","@id":"1","@type":"text"}}}`,
		},
		{
			name:     "Root element with text and attributes",
			xml:      `<root version="1.0">Content</root>`,
			expected: `{"root":{"#text":"Content","@version":"1.0"}}`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			value := Value{
				Data:      tt.xml,
				Type:      ValueTypeStr,
				Timestamp: time.Now(),
			}

			result, err := xmlToJSON(value, "")
			if err != nil {
				t.Fatalf("xmlToJSON() error = %v", err)
			}

			var got, want interface{}
			json.Unmarshal([]byte(result.Data), &got)
			json.Unmarshal([]byte(tt.expected), &want)

			gotJSON, _ := json.Marshal(got)
			wantJSON, _ := json.Marshal(want)

			if string(gotJSON) != string(wantJSON) {
				t.Errorf("xmlToJSON() = %s, want %s", gotJSON, wantJSON)
			}
		})
	}
}

// TestXMLToJSON_ComplexCombinations tests complex combinations of all rules
func TestXMLToJSON_ComplexCombinations(t *testing.T) {
	tests := []struct {
		name     string
		xml      string
		expected string
	}{
		{
			name: "Complex nested structure",
			xml: `<root>
				<user id="123" active="true">
					<name>John Doe</name>
					<email/>
					<tags>
						<tag>admin</tag>
						<tag>user</tag>
					</tags>
				</user>
			</root>`,
			expected: `{"root":{"user":{"@active":"true","@id":"123","email":null,"name":"John Doe","tags":{"tag":["admin","user"]}}}}`,
		},
		{
			name: "Mixed attributes, text, and children",
			xml: `<config version="2.0">
				<setting name="timeout">30</setting>
				<setting name="retries">3</setting>
				<debug enabled="false"/>
			</config>`,
			expected: `{"config":{"@version":"2.0","debug":{"@enabled":"false"},"setting":[{"#text":"30","@name":"timeout"},{"#text":"3","@name":"retries"}]}}`,
		},
		{
			name: "Real-world API response example",
			xml: `<response status="success">
				<data>
					<item id="1" type="product">Widget</item>
					<item id="2" type="product">Gadget</item>
					<metadata>
						<total>2</total>
						<page>1</page>
					</metadata>
				</data>
			</response>`,
			expected: `{"response":{"@status":"success","data":{"item":[{"#text":"Widget","@id":"1","@type":"product"},{"#text":"Gadget","@id":"2","@type":"product"}],"metadata":{"page":"1","total":"2"}}}}`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			value := Value{
				Data:      tt.xml,
				Type:      ValueTypeStr,
				Timestamp: time.Now(),
			}

			result, err := xmlToJSON(value, "")
			if err != nil {
				t.Fatalf("xmlToJSON() error = %v", err)
			}

			var got, want interface{}
			json.Unmarshal([]byte(result.Data), &got)
			json.Unmarshal([]byte(tt.expected), &want)

			gotJSON, _ := json.Marshal(got)
			wantJSON, _ := json.Marshal(want)

			if string(gotJSON) != string(wantJSON) {
				t.Errorf("xmlToJSON() = %s, want %s", gotJSON, wantJSON)
			}
		})
	}
}

// TestXMLToJSON_ErrorCases tests error handling
func TestXMLToJSON_ErrorCases(t *testing.T) {
	tests := []struct {
		name    string
		xml     string
		wantErr bool
	}{
		{
			name:    "Invalid XML - unclosed tag",
			xml:     `<root><foo>`,
			wantErr: true,
		},
		{
			name:    "Invalid XML - mismatched tags",
			xml:     `<root><foo></bar></root>`,
			wantErr: true,
		},
		{
			name:    "Empty input",
			xml:     ``,
			wantErr: true,
		},
		{
			name:    "Not XML",
			xml:     `{"json": "data"}`,
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			value := Value{
				Data:      tt.xml,
				Type:      ValueTypeStr,
				Timestamp: time.Now(),
			}

			_, err := xmlToJSON(value, "")
			if (err != nil) != tt.wantErr {
				t.Errorf("xmlToJSON() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

// TestXMLToJSON_EdgeCases tests edge cases and special scenarios
func TestXMLToJSON_EdgeCases(t *testing.T) {
	tests := []struct {
		name     string
		xml      string
		expected string
	}{
		{
			name:     "Empty root element with attributes",
			xml:      `<root id="1"/>`,
			expected: `{"root":{"@id":"1"}}`,
		},
		{
			name:     "Only whitespace text",
			xml:      `<root>   </root>`,
			expected: `{"root":null}`,
		},
		{
			name:     "CDATA section",
			xml:      `<root><![CDATA[Special <chars> & stuff]]></root>`,
			expected: `{"root":"Special <chars> & stuff"}`,
		},
		{
			name:     "XML with namespaces",
			xml:      `<root xmlns:foo="http://example.com"><bar>value</bar></root>`,
			expected: `{"root":{"@foo":"http://example.com","bar":"value"}}`,
		},
		{
			name:     "Numeric-looking values",
			xml:      `<root><num>123</num><float>45.67</float></root>`,
			expected: `{"root":{"float":"45.67","num":"123"}}`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			value := Value{
				Data:      tt.xml,
				Type:      ValueTypeStr,
				Timestamp: time.Now(),
			}

			result, err := xmlToJSON(value, "")
			if err != nil {
				t.Fatalf("xmlToJSON() error = %v", err)
			}

			var got, want interface{}
			json.Unmarshal([]byte(result.Data), &got)
			json.Unmarshal([]byte(tt.expected), &want)

			gotJSON, _ := json.Marshal(got)
			wantJSON, _ := json.Marshal(want)

			if string(gotJSON) != string(wantJSON) {
				t.Errorf("xmlToJSON() = %s, want %s", gotJSON, wantJSON)
			}
		})
	}
}
