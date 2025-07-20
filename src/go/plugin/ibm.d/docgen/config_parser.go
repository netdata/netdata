package main

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"strings"
	"unicode"
)

// parseConfigFromGoFile parses a Go config file to extract configuration fields
func (g *DocGenerator) parseConfigFromGoFile() ([]ConfigField, error) {
	fset := token.NewFileSet()
	node, err := parser.ParseFile(fset, g.ConfigFile, nil, parser.ParseComments)
	if err != nil {
		return nil, fmt.Errorf("failed to parse Go file: %w", err)
	}

	var fields []ConfigField
	var configStruct *ast.StructType

	// Find the Config struct
	ast.Inspect(node, func(n ast.Node) bool {
		switch x := n.(type) {
		case *ast.TypeSpec:
			if x.Name.Name == "Config" {
				if structType, ok := x.Type.(*ast.StructType); ok {
					configStruct = structType
					return false
				}
			}
		}
		return true
	})

	if configStruct != nil {
		fields = extractFieldsFromStruct(configStruct, node.Comments)
	}

	return fields, nil
}

func extractFieldsFromStruct(structType *ast.StructType, comments []*ast.CommentGroup) []ConfigField {
	var fields []ConfigField

	for _, field := range structType.Fields.List {
		// Skip embedded fields (like framework.Config)
		if len(field.Names) == 0 {
			continue
		}

		for _, name := range field.Names {
			// Skip unexported fields
			if !ast.IsExported(name.Name) {
				continue
			}

			configField := extractConfigField(name.Name, field, comments)
			if configField != nil {
				fields = append(fields, *configField)
			}
		}
	}

	return fields
}

func extractConfigField(fieldName string, field *ast.Field, comments []*ast.CommentGroup) *ConfigField {
	// Extract type information
	fieldType := extractGoType(field.Type)
	if fieldType == "" {
		return nil
	}

	// Extract struct tags
	yamlName, jsonName := extractStructTags(field.Tag)
	if yamlName == "" {
		yamlName = convertToSnakeCase(fieldName)
	}
	if jsonName == "" {
		jsonName = yamlName
	}

	// Skip fields marked as omitempty only or inline
	if yamlName == "-" || strings.Contains(yamlName, "inline") {
		return nil
	}

	// Convert Go type to JSON Schema type
	jsonType := convertToJSONType(fieldType)

	// Extract documentation from comments
	description := extractFieldDescription(fieldName, field, comments)

	// Create field
	configField := &ConfigField{
		Name:        fieldName,
		JSONName:    jsonName,
		Type:        jsonType,
		Required:    !strings.Contains(yamlName, "omitempty"),
		Description: description,
	}

	// Set defaults and constraints based on field name and type
	setFieldDefaults(configField)

	return configField
}

func extractGoType(expr ast.Expr) string {
	switch t := expr.(type) {
	case *ast.Ident:
		return t.Name
	case *ast.SelectorExpr:
		// Handle qualified types like time.Duration
		if ident, ok := t.X.(*ast.Ident); ok {
			return ident.Name + "." + t.Sel.Name
		}
	}
	return ""
}

func extractStructTags(tag *ast.BasicLit) (yaml, json string) {
	if tag == nil {
		return "", ""
	}

	tagValue := strings.Trim(tag.Value, "`")
	
	// Parse yaml tag
	if yamlStart := strings.Index(tagValue, "yaml:\""); yamlStart >= 0 {
		yamlPart := tagValue[yamlStart+6:] // Skip 'yaml:"'
		if quoteEnd := strings.Index(yamlPart, "\""); quoteEnd >= 0 {
			yaml = yamlPart[:quoteEnd]
		}
	}

	// Parse json tag
	if jsonStart := strings.Index(tagValue, "json:\""); jsonStart >= 0 {
		jsonPart := tagValue[jsonStart+6:] // Skip 'json:"'
		if quoteEnd := strings.Index(jsonPart, "\""); quoteEnd >= 0 {
			json = jsonPart[:quoteEnd]
		}
	}

	return yaml, json
}

func convertToSnakeCase(s string) string {
	var result strings.Builder
	for i, r := range s {
		if i > 0 && 'A' <= r && r <= 'Z' {
			result.WriteRune('_')
		}
		if 'A' <= r && r <= 'Z' {
			result.WriteRune(r - 'A' + 'a')
		} else {
			result.WriteRune(r)
		}
	}
	return result.String()
}

func convertToJSONType(goType string) string {
	switch goType {
	case "string":
		return "string"
	case "int", "int8", "int16", "int32", "int64", "uint", "uint8", "uint16", "uint32", "uint64":
		return "integer"
	case "float32", "float64":
		return "number"
	case "bool":
		return "boolean"
	default:
		// Handle complex types
		if strings.Contains(goType, "Duration") {
			return "integer"
		}
		return "string" // Default fallback
	}
}

func extractFieldDescription(fieldName string, field *ast.Field, comments []*ast.CommentGroup) string {
	// First, try to extract description from field comment
	if field.Comment != nil && len(field.Comment.List) > 0 {
		// Use the first comment line as description
		comment := field.Comment.List[0].Text
		// Clean up the comment (remove // or /*)
		comment = strings.TrimSpace(comment)
		comment = strings.TrimPrefix(comment, "//")
		comment = strings.TrimPrefix(comment, "/*")
		comment = strings.TrimSuffix(comment, "*/")
		comment = strings.TrimSpace(comment)
		if comment != "" {
			return comment
		}
	}
	
	// If no field comment, try to find comment before the field
	if field.Doc != nil && len(field.Doc.List) > 0 {
		// Use the last doc comment line as description
		comment := field.Doc.List[len(field.Doc.List)-1].Text
		// Clean up the comment
		comment = strings.TrimSpace(comment)
		comment = strings.TrimPrefix(comment, "//")
		comment = strings.TrimPrefix(comment, "/*")
		comment = strings.TrimSuffix(comment, "*/")
		comment = strings.TrimSpace(comment)
		if comment != "" {
			return comment
		}
	}
	
	// Check for exact matches for framework fields
	descriptions := map[string]string{
		"UpdateEvery":      "Data collection frequency in seconds",
		"Endpoint":         "Connection endpoint URL",
		"ConnectTimeout":   "Connection timeout in seconds", 
		"CollectItems":     "Enable collection of item metrics",
		"MaxItems":         "Maximum number of items to collect",
		"ObsoletionIterations": "Number of iterations after which charts become obsolete",
	}

	if desc, exists := descriptions[fieldName]; exists {
		return desc
	}

	// Generate intelligent descriptions based on field patterns
	fieldLower := strings.ToLower(fieldName)
	
	// Generic connection-related fields
	if strings.Contains(fieldLower, "host") {
		return "Server hostname or IP address"
	}
	if strings.Contains(fieldLower, "port") {
		return "Server port number"
	}
	if strings.Contains(fieldLower, "user") || strings.Contains(fieldLower, "username") {
		return "Username for authentication"
	}
	if strings.Contains(fieldLower, "password") || strings.Contains(fieldLower, "pass") {
		return "Password for authentication"
	}
	
	// Collection control fields
	if strings.HasPrefix(fieldLower, "collect") {
		resource := extractResourceFromFieldName(fieldName)
		return fmt.Sprintf("Enable collection of %s metrics", resource)
	}
	
	// Selector fields
	if strings.HasSuffix(fieldLower, "selector") {
		resource := extractResourceFromFieldName(fieldName)
		return fmt.Sprintf("Pattern to filter %s (wildcards supported)", resource)
	}
	
	// Timeout fields
	if strings.Contains(fieldLower, "timeout") {
		return "Connection timeout duration in seconds"
	}
	
	// SSL/TLS fields
	if strings.Contains(fieldLower, "ssl") || strings.Contains(fieldLower, "tls") {
		return "Enable SSL/TLS encrypted connection"
	}
	
	// URL/URI fields
	if strings.Contains(fieldLower, "url") || strings.Contains(fieldLower, "uri") {
		return "Connection URL or URI"
	}
	
	// DSN fields
	if strings.Contains(fieldLower, "dsn") {
		return "Data Source Name (DSN) for connection"
	}
	
	// Max/limit fields
	if strings.HasPrefix(fieldLower, "max") {
		resource := extractResourceFromFieldName(fieldName)
		return fmt.Sprintf("Maximum number of %s to monitor", resource)
	}
	
	// Default to a more descriptive generic description
	return fmt.Sprintf("%s", camelToWords(fieldName))
}

// Helper function to extract resource name from field name
func extractResourceFromFieldName(fieldName string) string {
	// Remove common prefixes/suffixes
	name := fieldName
	name = strings.TrimPrefix(name, "Collect")
	name = strings.TrimPrefix(name, "Max")
	name = strings.TrimSuffix(name, "Selector")
	name = strings.TrimSuffix(name, "Config")
	
	// Convert to lowercase and make plural if needed
	resource := camelToWords(name)
	resource = strings.ToLower(resource)
	
	// Handle some special cases
	if resource == "system queues" {
		return "system queues"
	}
	if resource == "system channels" {
		return "system channels"
	}
	if resource == "reset queue stats" {
		return "queue statistics (destructive)"
	}
	
	return resource
}

// Helper function to convert CamelCase to words
func camelToWords(s string) string {
	var result strings.Builder
	for i, r := range s {
		if i > 0 && 'A' <= r && r <= 'Z' {
			result.WriteRune(' ')
		}
		if i == 0 {
			result.WriteRune(r)
		} else {
			result.WriteRune(unicode.ToLower(r))
		}
	}
	return result.String()
}

func setFieldDefaults(field *ConfigField) {
	// Set defaults based on field name patterns
	switch field.Name {
	case "UpdateEvery":
		field.Default = 1
		field.Minimum = intPtr(1)
	case "ConnectTimeout":
		field.Default = 5
		field.Minimum = intPtr(1)
		field.Maximum = intPtr(300)
	case "CollectItems":
		field.Default = true
	case "MaxItems":
		field.Default = 10
		field.Minimum = intPtr(1)
		field.Maximum = intPtr(1000)
	case "Endpoint":
		field.Default = "dummy://localhost"
		field.Examples = []string{"dummy://localhost", "tcp://server:1414"}
	case "ObsoletionIterations":
		field.Default = 60
		field.Minimum = intPtr(1)
	
	// Generic defaults for common field patterns
	case "Port":
		field.Minimum = intPtr(1)
		field.Maximum = intPtr(65535)
	}
	
	// Set format hints for specific field types
	fieldLower := strings.ToLower(field.Name)
	if strings.Contains(fieldLower, "password") || strings.Contains(fieldLower, "pass") {
		field.Format = "password"
	}
	
	// TODO: In the future, extract defaults from SetDefaults method in config.go
	// This would make defaults module-specific rather than hardcoded in framework
}