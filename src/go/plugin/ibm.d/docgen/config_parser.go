package main

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"strings"
)

// parseConfigFromGoFile parses a Go config file to extract configuration fields
func (g *DocGenerator) parseConfigFromGoFile() ([]ConfigField, error) {
	fset := token.NewFileSet()
	node, err := parser.ParseFile(fset, g.ConfigFile, nil, parser.ParseComments)
	if err != nil {
		return nil, fmt.Errorf("failed to parse Go file: %w", err)
	}

	var fields []ConfigField

	// Find the Config struct
	ast.Inspect(node, func(n ast.Node) bool {
		switch x := n.(type) {
		case *ast.TypeSpec:
			if x.Name.Name == "Config" {
				if structType, ok := x.Type.(*ast.StructType); ok {
					fields = extractFieldsFromStruct(structType)
					return false
				}
			}
		}
		return true
	})

	return fields, nil
}

func extractFieldsFromStruct(structType *ast.StructType) []ConfigField {
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

			configField := extractConfigField(name.Name, field)
			if configField != nil {
				fields = append(fields, *configField)
			}
		}
	}

	return fields
}

func extractConfigField(fieldName string, field *ast.Field) *ConfigField {
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
	description := extractFieldDescription(fieldName, field)

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

func extractFieldDescription(fieldName string, field *ast.Field) string {
	// For now, generate descriptions based on field names
	// In a full implementation, this would parse comments above the field
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

	// Generate a generic description
	return fmt.Sprintf("%s configuration option", fieldName)
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
	}
}