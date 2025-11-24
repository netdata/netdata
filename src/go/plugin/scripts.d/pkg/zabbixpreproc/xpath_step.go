package zabbixpreproc

import (
	"fmt"
	"math"
	"regexp"
	"strings"

	"github.com/antchfx/xmlquery"
	"github.com/antchfx/xpath"
)

// XPath regex patterns compiled once at package init
var (
	// Matches empty XML tags like <tag></tag> or <tag attr="value"></tag>
	xpathEmptyTagRegex = regexp.MustCompile(`<([a-zA-Z][a-zA-Z0-9]*)(\s+[^>]*)?></([a-zA-Z][a-zA-Z0-9]*)>`)
)

// xpathExtract extracts values from XML using XPath expression.
func xpathExtract(value Value, paramStr string) (Value, error) {
	expr := strings.TrimSpace(paramStr)
	if expr == "" {
		return Value{}, fmt.Errorf("xpath expression is required")
	}

	// Parse XML
	doc, err := xmlquery.Parse(strings.NewReader(value.Data))
	if err != nil {
		return Value{}, fmt.Errorf("invalid XML: %w", err)
	}

	// Compile and evaluate XPath expression
	xpathExpr, err := xpath.Compile(expr)
	if err != nil {
		return Value{}, fmt.Errorf("invalid xpath expression: %w", err)
	}

	// Evaluate the expression
	result := xpathExpr.Evaluate(xmlquery.CreateXPathNavigator(doc))

	// Handle different result types
	switch v := result.(type) {
	case *xpath.NodeIterator:
		// Node selection - collect nodes
		var output strings.Builder
		for v.MoveNext() {
			node := v.Current().(*xmlquery.NodeNavigator).Current()

			// For text and attribute nodes, return the data
			// For element nodes, return the XML representation
			if node.Type == xmlquery.TextNode || node.Type == xmlquery.AttributeNode {
				output.WriteString(node.Data)
			} else {
				xml := node.OutputXML(true)
				// Convert empty tags to self-closing format (e.g., <a></a> to <a/>)
				xml = convertToSelfClosingTags(xml)
				output.WriteString(xml)
			}
		}
		return Value{Data: output.String(), Type: ValueTypeStr}, nil

	case string:
		// String result from string() function
		return Value{Data: v, Type: ValueTypeStr}, nil

	case float64:
		// Numeric result from arithmetic expressions
		// Check for invalid numeric results
		if math.IsInf(v, 0) || math.IsNaN(v) {
			return Value{}, fmt.Errorf("invalid numeric result from XPath expression")
		}
		return Value{Data: fmt.Sprintf("%g", v), Type: ValueTypeStr}, nil

	case bool:
		// Boolean result from boolean expressions
		if v {
			return Value{Data: "true", Type: ValueTypeStr}, nil
		}
		return Value{Data: "false", Type: ValueTypeStr}, nil

	default:
		// Unknown type - convert to string
		return Value{Data: fmt.Sprint(v), Type: ValueTypeStr}, nil
	}
}

// convertToSelfClosingTags converts empty XML tags to self-closing format
func convertToSelfClosingTags(xml string) string {
	// Match patterns like <tag></tag> or <tag attr="value"></tag>
	// Note: Go regex doesn't support backreferences in pattern, so we match and check manually
	return xpathEmptyTagRegex.ReplaceAllStringFunc(xml, func(match string) string {
		// Extract the tag names from opening and closing tags
		submatches := xpathEmptyTagRegex.FindStringSubmatch(match)
		if len(submatches) >= 4 {
			openTag := submatches[1]
			attrs := submatches[2]
			closeTag := submatches[3]

			// Only convert if opening and closing tags match
			if openTag == closeTag {
				return "<" + openTag + attrs + "/>"
			}
		}
		// Return unchanged if tags don't match
		return match
	})
}
