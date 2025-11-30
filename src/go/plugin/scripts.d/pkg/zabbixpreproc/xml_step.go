package zabbixpreproc

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/antchfx/xmlquery"
)

// xmlToJSON converts XML to JSON following Zabbix serialization rules.
//
// Zabbix XML to JSON Serialization Rules:
//  1. Attributes: Prepended with '@'
//     <xml foo="FOO"> → {"xml": {"@foo": "FOO"}}
//  2. Self-closing elements: Become null
//     <foo/> → {"foo": null}
//  3. Empty attributes: Preserved as empty strings
//     bar="" → "@bar": ""
//  4. Repeated elements: Consolidated into arrays
//     <foo>A</foo><foo>B</foo> → "foo": ["A", "B"]
//  5. Simple text elements: Direct string values
//     <foo>BAZ</foo> → {"foo": "BAZ"}
//  6. Text with attributes: Uses #text key
//     <foo bar="BAR">BAZ</foo> → {"foo": {"@bar": "BAR", "#text": "BAZ"}}
func xmlToJSON(value Value, paramStr string) (Value, error) {
	// Parse XML
	doc, err := xmlquery.Parse(strings.NewReader(value.Data))
	if err != nil {
		return Value{}, fmt.Errorf("invalid XML: %w", err)
	}

	// Convert to JSON structure
	result := convertNodeToJSON(doc)

	// Serialize to JSON string
	jsonBytes, err := json.Marshal(result)
	if err != nil {
		return Value{}, fmt.Errorf("JSON serialization failed: %w", err)
	}

	return Value{Data: string(jsonBytes), Type: ValueTypeStr}, nil
}

// convertNodeToJSON converts an XML node to a JSON-compatible structure
func convertNodeToJSON(node *xmlquery.Node) interface{} {
	if node == nil {
		return nil
	}

	// Handle document node - process root element
	if node.Type == xmlquery.DocumentNode {
		for child := node.FirstChild; child != nil; child = child.NextSibling {
			if child.Type == xmlquery.ElementNode {
				// Return the root element as a map with element name as key
				return map[string]interface{}{
					child.Data: convertElementToJSON(child),
				}
			}
		}
		return nil
	}

	// For element nodes, wrap in a map with element name
	if node.Type == xmlquery.ElementNode {
		return map[string]interface{}{
			node.Data: convertElementToJSON(node),
		}
	}

	return nil
}

// convertElementToJSON converts an XML element to JSON following Zabbix rules
func convertElementToJSON(node *xmlquery.Node) interface{} {
	// Collect attributes (Rule 1: prepend with '@')
	attrs := make(map[string]interface{})
	for _, attr := range node.Attr {
		// Rule 3: Empty attributes preserved as empty strings
		attrs["@"+attr.Name.Local] = attr.Value
	}

	// Collect child elements grouped by name
	children := make(map[string][]interface{})
	var textContent strings.Builder
	hasChildren := false

	for child := node.FirstChild; child != nil; child = child.NextSibling {
		switch child.Type {
		case xmlquery.ElementNode:
			hasChildren = true
			childValue := convertElementToJSON(child)
			children[child.Data] = append(children[child.Data], childValue)

		case xmlquery.TextNode, xmlquery.CharDataNode:
			// Collect text content (trim each segment but preserve structure)
			text := strings.TrimSpace(child.Data)
			if text != "" {
				if textContent.Len() > 0 {
					textContent.WriteString(" ")
				}
				textContent.WriteString(text)
			}
		}
	}

	text := textContent.String()

	// Rule 2: Self-closing elements (no attributes, no children, no text) → null
	if len(attrs) == 0 && !hasChildren && text == "" {
		return nil
	}

	// Rule 5: Simple text elements (no attributes, no children, only text) → direct string
	if len(attrs) == 0 && !hasChildren && text != "" {
		return text
	}

	// Build result object
	result := make(map[string]interface{})

	// Add attributes first
	for k, v := range attrs {
		result[k] = v
	}

	// Rule 6: Text with attributes → use #text key
	if text != "" {
		result["#text"] = text
	}

	// Rule 4: Repeated elements → arrays
	for name, values := range children {
		if len(values) == 1 {
			// Single child - add directly
			result[name] = values[0]
		} else {
			// Multiple children with same name - create array
			result[name] = values
		}
	}

	return result
}
