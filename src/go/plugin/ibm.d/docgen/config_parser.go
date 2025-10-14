package main

import (
	"bytes"
	"fmt"
	"go/ast"
	"go/format"
	"go/parser"
	"go/token"
	"log"
	"math"
	"os"
	"path/filepath"
	"reflect"
	"strconv"
	"strings"
	"time"
	"unicode"

	"github.com/netdata/netdata/go/plugins/pkg/confopt"
)

// parseConfigFromGoFile parses a Go config file to extract configuration fields
// and the defaults supplied by defaultConfig().
func (g *DocGenerator) parseConfigFromGoFile() ([]ConfigField, map[string]interface{}, error) {
	fset := token.NewFileSet()
	node, err := parser.ParseFile(fset, g.ConfigFile, nil, parser.ParseComments)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to parse Go file: %w", err)
	}

	g.consts = g.extractConstValues(node)

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
		fields = extractFieldsFromStruct(g, configStruct, node.Comments)
	}

	// Try to parse defaults from init.go
	defaults := g.parseDefaultsFromInitFile()
	if defaults == nil {
		return nil, nil, fmt.Errorf("failed to parse defaultConfig() function from init.go - all configuration fields must have defaults defined")
	}

	// Validate that ALL fields have defaults and apply them
	var missingDefaults []string
	for i := range fields {
		field := &fields[i]
		if isAutoBoolType(field.GoType) {
			field.Type = "string"
			field.Enum = toStringSlice(confopt.AutoBoolEnum)
		}

		if defaultValue, exists := defaults[field.Name]; exists {
			field.Default = normalizeDefaultValue(*field, defaultValue)
			field.Required = false
		} else if isAutoBoolType(field.GoType) {
			field.Default = confopt.AutoBoolAuto.String()
			field.Required = false
		} else if field.Pointer {
			field.Default = "<auto>"
			field.Required = false
		} else {
			missingDefaults = append(missingDefaults, field.Name)
		}

		assignDefaultUIGroup(field)
	}

	// FAIL HARD if any field lacks a default
	if len(missingDefaults) > 0 {
		return nil, nil, fmt.Errorf("defaultConfig() function must provide default values for ALL configuration fields. Missing defaults for: %v", missingDefaults)
	}

	return fields, defaults, nil
}

func extractFieldsFromStruct(g *DocGenerator, structType *ast.StructType, comments []*ast.CommentGroup) []ConfigField {
	var fields []ConfigField

	for _, field := range structType.Fields.List {
		// Skip embedded fields (like framework.Config)
		if len(field.Names) == 0 {
			typeName := extractGoType(field.Type)
			if typeName == "web.HTTPConfig" {
				g.hasHTTPConfig = true
			}
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
	yamlName, jsonName, uiTag := extractStructTags(field.Tag)
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
	isPointer := strings.HasPrefix(fieldType, "*")
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
		Pointer:     isPointer,
		GoType:      fieldType,
	}

	applyUITag(configField, uiTag)

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
	case *ast.StarExpr:
		inner := extractGoType(t.X)
		if inner == "" {
			return ""
		}
		return "*" + inner
	}
	return ""
}

func extractStructTags(tag *ast.BasicLit) (yaml, json, ui string) {
	if tag == nil {
		return "", "", ""
	}

	tagValue := strings.Trim(tag.Value, "`")
	structTag := reflect.StructTag(tagValue)

	yaml = structTag.Get("yaml")
	json = structTag.Get("json")
	ui = structTag.Get("ui")

	return yaml, json, ui
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
	baseType := strings.TrimPrefix(goType, "*")
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
		// Handle pointer wrappers of basic types
		switch baseType {
		case "string":
			return "string"
		case "int", "int8", "int16", "int32", "int64", "uint", "uint8", "uint16", "uint32", "uint64":
			return "integer"
		case "float32", "float64":
			return "number"
		case "bool":
			return "boolean"
		}
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
		"UpdateEvery":          "Data collection frequency in seconds",
		"Endpoint":             "Connection endpoint URL",
		"ConnectTimeout":       "Connection timeout in seconds",
		"CollectItems":         "Enable collection of item metrics",
		"MaxItems":             "Maximum number of items to collect",
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
		if field.UIWidget == "" {
			field.UIWidget = "password"
		}
	}

	// TODO: In the future, extract defaults from SetDefaults method in config.go
	// This would make defaults module-specific rather than hardcoded in framework
}

func applyUITag(field *ConfigField, tag string) {
	if tag == "" {
		return
	}
	opts := parseUIOptions(tag)
	if group, ok := opts["group"]; ok {
		field.UIGroup = group
	}
	if widget, ok := opts["widget"]; ok {
		field.UIWidget = widget
	}
	if help, ok := opts["help"]; ok {
		field.UIHelp = help
	}
	if placeholder, ok := opts["placeholder"]; ok {
		field.UIPlaceholder = placeholder
	}
}

func parseUIOptions(tag string) map[string]string {
	result := make(map[string]string)
	if tag == "" {
		return result
	}
	parts := strings.Split(tag, ",")
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		kv := strings.SplitN(part, ":", 2)
		key := strings.TrimSpace(kv[0])
		if key == "" {
			continue
		}
		value := ""
		if len(kv) == 2 {
			value = strings.TrimSpace(kv[1])
		}
		result[key] = value
	}
	return result
}

func assignDefaultUIGroup(field *ConfigField) {
	if field.UIGroup != "" {
		return
	}

	name := field.JSONName
	lower := strings.ToLower(name)

	switch {
	case name == "update_every" || name == "vnode" || name == "dsn" || strings.Contains(lower, "endpoint") || strings.Contains(lower, "url"):
		field.UIGroup = "Connection"
	case strings.HasPrefix(lower, "collect_"):
		field.UIGroup = "Collection"
	case lower == "timeout" || strings.HasSuffix(lower, "_timeout"):
		field.UIGroup = "Connection"
	case strings.HasSuffix(lower, "_matching") || strings.HasSuffix(lower, "_selector"):
		field.UIGroup = "Filters"
	case strings.HasPrefix(lower, "max_"):
		field.UIGroup = "Limits"
	case strings.HasPrefix(lower, "tls_") || strings.Contains(lower, "proxy") || strings.Contains(lower, "header") || strings.Contains(lower, "redirect"):
		field.UIGroup = "HTTP"
	case strings.Contains(lower, "password") || strings.Contains(lower, "username") || strings.Contains(lower, "auth") || strings.Contains(lower, "token") || strings.Contains(lower, "api_key"):
		field.UIGroup = "Auth"
	default:
		field.UIGroup = "Advanced"
	}
}

func toStringSlice(values []confopt.AutoBool) []string {
	result := make([]string, 0, len(values))
	for _, v := range values {
		result = append(result, v.String())
	}
	return result
}

func normalizeDefaultValue(field ConfigField, value interface{}) interface{} {
	if !isAutoBoolType(field.GoType) {
		return value
	}
	switch v := value.(type) {
	case string:
		return normalizeAutoBoolLiteral(v)
	case bool:
		if v {
			return confopt.AutoBoolEnabled.String()
		}
		return confopt.AutoBoolDisabled.String()
	case nil:
		return confopt.AutoBoolAuto.String()
	default:
		return confopt.AutoBoolAuto.String()
	}
}

func normalizeAutoBoolLiteral(value string) string {
	lower := strings.ToLower(strings.TrimSpace(value))
	switch lower {
	case "", confopt.AutoBoolAuto.String():
		return confopt.AutoBoolAuto.String()
	case confopt.AutoBoolEnabled.String():
		return confopt.AutoBoolEnabled.String()
	case confopt.AutoBoolDisabled.String():
		return confopt.AutoBoolDisabled.String()
	}

	if strings.HasSuffix(lower, "autoboolenabled") {
		return confopt.AutoBoolEnabled.String()
	}
	if strings.HasSuffix(lower, "autobooldisabled") {
		return confopt.AutoBoolDisabled.String()
	}
	return confopt.AutoBoolAuto.String()
}

// parseDefaultsFromInitFile parses init.go to find the defaultConfig() function
// and extract default values from the returned Config struct
func (g *DocGenerator) parseDefaultsFromInitFile() map[string]interface{} {
	// Construct path to init.go
	dir := filepath.Dir(g.ConfigFile)
	initFile := filepath.Join(dir, "init.go")

	// Check if init.go exists
	if _, err := os.Stat(initFile); os.IsNotExist(err) {
		log.Printf("ERROR: init.go not found at %s - defaultConfig() function is required", initFile)
		return nil
	}

	fset := token.NewFileSet()
	node, err := parser.ParseFile(fset, initFile, nil, parser.ParseComments)
	if err != nil {
		log.Printf("ERROR: Failed to parse init.go: %v", err)
		return nil
	}

	defaults := make(map[string]interface{})
	foundDefaultConfig := false

	// Find the defaultConfig function
	ast.Inspect(node, func(n ast.Node) bool {
		switch x := n.(type) {
		case *ast.FuncDecl:
			if x.Name.Name == "defaultConfig" {
				foundDefaultConfig = true
				// Look for return statement with Config literal
				foundReturn := false
				ast.Inspect(x.Body, func(bodyNode ast.Node) bool {
					switch stmt := bodyNode.(type) {
					case *ast.ReturnStmt:
						if len(stmt.Results) > 0 {
							if lit, ok := stmt.Results[0].(*ast.CompositeLit); ok {
								g.extractDefaultsFromLiteral(lit, defaults)
								foundReturn = true
								return false
							}
						}
					}
					return true
				})
				if !foundReturn {
					log.Printf("ERROR: defaultConfig() function found but no valid Config{} return statement")
				}
				return false
			}
		}
		return true
	})

	if !foundDefaultConfig {
		log.Printf("ERROR: defaultConfig() function not found in %s - this function is mandatory", initFile)
		return nil
	}

	if len(defaults) == 0 {
		log.Printf("ERROR: defaultConfig() function found but extracted no default values")
		return nil
	}

	log.Printf("Successfully extracted %d default values from defaultConfig()", len(defaults))
	return defaults
}

// extractDefaultsFromLiteral extracts field values from a Config{} literal
func (g *DocGenerator) extractDefaultsFromLiteral(lit *ast.CompositeLit, defaults map[string]interface{}) {
	for _, elt := range lit.Elts {
		if kv, ok := elt.(*ast.KeyValueExpr); ok {
			if ident, ok := kv.Key.(*ast.Ident); ok {
				fieldName := ident.Name
				switch val := kv.Value.(type) {
				case *ast.CompositeLit:
					nested := make(map[string]interface{})
					g.extractDefaultsFromLiteral(val, nested)
					// Flatten nested composite literals for embedded configs such as framework.Config.
					if fieldName == "Config" {
						for k, v := range nested {
							if _, exists := defaults[k]; !exists {
								defaults[k] = v
							}
						}
					} else {
						// Preserve nested context using fieldName prefix to avoid collisions.
						for k, v := range nested {
							compositeKey := fmt.Sprintf("%s.%s", fieldName, k)
							if _, exists := defaults[compositeKey]; !exists {
								defaults[compositeKey] = v
							}
						}
					}
				default:
					if value := g.extractValue(kv.Value); value != nil {
						defaults[fieldName] = value
					}
				}
			}
		}
	}
}

// extractValue converts an AST expression to a Go value
func (g *DocGenerator) extractValue(expr ast.Expr) interface{} {
	switch v := expr.(type) {
	case *ast.BasicLit:
		switch v.Kind {
		case token.STRING:
			// Remove quotes
			return strings.Trim(v.Value, `"`)
		case token.INT:
			if i, err := strconv.Atoi(v.Value); err == nil {
				return i
			}
		case token.FLOAT:
			if f, err := strconv.ParseFloat(v.Value, 64); err == nil {
				return f
			}
		}
	case *ast.Ident:
		// Handle boolean values
		switch v.Name {
		case "true":
			return true
		case "false":
			return false
		}
		if g != nil {
			if val, ok := g.consts[v.Name]; ok {
				return val
			}
		}
		return exprToString(v)
	case *ast.CallExpr:
		if len(v.Args) > 0 {
			val := g.extractValue(v.Args[0])
			if val != nil {
				return val
			}
		}
		return exprToString(v)
	case *ast.BinaryExpr:
		left := g.extractValue(v.X)
		right := g.extractValue(v.Y)
		if result, ok := evalBinaryExpr(v.Op, left, right); ok {
			return result
		}
		return exprToString(v)
	case *ast.SelectorExpr:
		if ident, ok := v.X.(*ast.Ident); ok {
			if ident.Name == "time" {
				if duration, ok := timeConstant(v.Sel.Name); ok {
					return duration
				}
			}
			if ident.Name == "framework" || ident.Name == "confopt" {
				switch v.Sel.Name {
				case "AutoBoolAuto":
					return confopt.AutoBoolAuto.String()
				case "AutoBoolEnabled":
					return confopt.AutoBoolEnabled.String()
				case "AutoBoolDisabled":
					return confopt.AutoBoolDisabled.String()
				}
			}
		}
		return exprToString(v)
	case *ast.UnaryExpr:
		return g.extractValue(v.X)
	}
	return nil
}

func exprToString(expr ast.Expr) string {
	if expr == nil {
		return ""
	}
	var buf bytes.Buffer
	if err := format.Node(&buf, token.NewFileSet(), expr); err != nil {
		return ""
	}
	return strings.TrimSpace(buf.String())
}

func (g *DocGenerator) extractConstValues(file *ast.File) map[string]interface{} {
	consts := make(map[string]interface{})
	if file == nil {
		return consts
	}

	prev := g.consts
	g.consts = consts

	for _, decl := range file.Decls {
		gen, ok := decl.(*ast.GenDecl)
		if !ok || gen.Tok != token.CONST {
			continue
		}
		for _, spec := range gen.Specs {
			vs, ok := spec.(*ast.ValueSpec)
			if !ok {
				continue
			}
			for i, name := range vs.Names {
				if name == nil || name.Name == "_" {
					continue
				}
				var value interface{}
				if len(vs.Values) > i {
					value = g.extractValue(vs.Values[i])
				} else if len(vs.Values) > 0 {
					value = g.extractValue(vs.Values[0])
				}
				if value != nil {
					consts[name.Name] = value
				}
			}
		}
	}

	g.consts = prev
	return consts
}

func evalBinaryExpr(op token.Token, left, right interface{}) (interface{}, bool) {
	if li, lok := toInt64(left); lok {
		if ri, rok := toInt64(right); rok {
			switch op {
			case token.MUL:
				return li * ri, true
			case token.QUO:
				if ri == 0 {
					return nil, false
				}
				return li / ri, true
			case token.ADD:
				return li + ri, true
			case token.SUB:
				return li - ri, true
			case token.REM:
				if ri == 0 {
					return nil, false
				}
				return li % ri, true
			}
		}
	}

	lf, lok := toFloat64(left)
	rf, rok := toFloat64(right)
	if !lok || !rok {
		return nil, false
	}

	var res float64
	switch op {
	case token.MUL:
		res = lf * rf
	case token.QUO:
		if rf == 0 {
			return nil, false
		}
		res = lf / rf
	case token.ADD:
		res = lf + rf
	case token.SUB:
		res = lf - rf
	default:
		return nil, false
	}

	if math.IsNaN(res) || math.IsInf(res, 0) {
		return nil, false
	}
	if math.Mod(res, 1) == 0 {
		return int64(res), true
	}
	return res, true
}

func toInt64(v interface{}) (int64, bool) {
	switch val := v.(type) {
	case int:
		return int64(val), true
	case int8:
		return int64(val), true
	case int16:
		return int64(val), true
	case int32:
		return int64(val), true
	case int64:
		return val, true
	case uint:
		return int64(val), true
	case uint8:
		return int64(val), true
	case uint16:
		return int64(val), true
	case uint32:
		return int64(val), true
	case uint64:
		if val > math.MaxInt64 {
			return 0, false
		}
		return int64(val), true
	case float64:
		if math.Mod(val, 1) == 0 {
			return int64(val), true
		}
	}
	return 0, false
}

func toFloat64(v interface{}) (float64, bool) {
	switch val := v.(type) {
	case int:
		return float64(val), true
	case int8:
		return float64(val), true
	case int16:
		return float64(val), true
	case int32:
		return float64(val), true
	case int64:
		return float64(val), true
	case uint:
		return float64(val), true
	case uint8:
		return float64(val), true
	case uint16:
		return float64(val), true
	case uint32:
		return float64(val), true
	case uint64:
		return float64(val), true
	case float32:
		return float64(val), true
	case float64:
		return val, true
	}
	return 0, false
}

func timeConstant(name string) (int64, bool) {
	switch name {
	case "Nanosecond":
		return int64(time.Nanosecond), true
	case "Microsecond":
		return int64(time.Microsecond), true
	case "Millisecond":
		return int64(time.Millisecond), true
	case "Second":
		return int64(time.Second), true
	case "Minute":
		return int64(time.Minute), true
	case "Hour":
		return int64(time.Hour), true
	}
	return 0, false
}

func isAutoBoolType(goType string) bool {
	return goType == "framework.AutoBool" || goType == "confopt.AutoBool"
}
