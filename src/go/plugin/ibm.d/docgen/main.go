// docgen - Automated documentation generator for ibm.d modules
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
	"text/template"

	"gopkg.in/yaml.v3"
)

// ModuleInfo contains metadata about a module
type ModuleInfo struct {
	Name        string
	DisplayName string
	Description string
	Icon        string
	Categories  []string
	Link        string
}

// Config represents the YAML structure from contexts.yaml
type Config struct {
	Classes map[string]Class `yaml:",inline"`
}

type Class struct {
	Labels   []string  `yaml:"labels"`
	Contexts []Context `yaml:"contexts"`
}

type Context struct {
	Name        string      `yaml:"name"`
	Context     string      `yaml:"context"`
	Family      string      `yaml:"family"`
	Title       string      `yaml:"title"`
	Units       string      `yaml:"units"`
	Type        string      `yaml:"type"`
	Priority    int         `yaml:"priority"`
	UpdateEvery int         `yaml:"update_every"`
	Dimensions  []Dimension `yaml:"dimensions"`
}

type Dimension struct {
	Name      string `yaml:"name"`
	Algorithm string `yaml:"algo"`
	Mul       int    `yaml:"mul"`
	Div       int    `yaml:"div"`
	Precision int    `yaml:"precision"`
}

// ConfigField represents a configuration field for schema generation
type ConfigField struct {
	Name          string
	JSONName      string
	Type          string
	Required      bool
	Default       interface{}
	Description   string
	Format        string
	Minimum       *int
	Maximum       *int
	Examples      []string
	Pointer       bool
	Enum          []string
	GoType        string
	UIGroup       string
	UIWidget      string
	UIHelp        string
	UIPlaceholder string
}

func main() {
	var (
		module      = flag.String("module", "", "Module name (required)")
		contextFile = flag.String("contexts", "contexts/contexts.yaml", "Path to contexts.yaml")
		configFile  = flag.String("config", "config.go", "Path to config.go")
		outputDir   = flag.String("output", ".", "Output directory")
		moduleInfo  = flag.String("module-info", "module.yaml", "Module info file")
	)
	flag.Parse()

	if *module == "" {
		log.Fatal("module name is required")
	}

	generator := &DocGenerator{
		ModuleName:  *module,
		ContextFile: *contextFile,
		ConfigFile:  *configFile,
		OutputDir:   *outputDir,
		ModuleInfo:  *moduleInfo,
	}

	if err := generator.Generate(); err != nil {
		log.Fatalf("failed to generate documentation: %v", err)
	}

	log.Printf("Generated documentation for module '%s'", *module)
}

type DocGenerator struct {
	ModuleName    string
	ContextFile   string
	ConfigFile    string
	OutputDir     string
	ModuleInfo    string
	consts        map[string]interface{}
	hasHTTPConfig bool
}

func (g *DocGenerator) Generate() error {
	// Parse contexts.yaml
	contexts, err := g.parseContexts()
	if err != nil {
		return fmt.Errorf("failed to parse contexts: %w", err)
	}

	// Parse module info
	moduleInfo, err := g.parseModuleInfo()
	if err != nil {
		return fmt.Errorf("failed to parse module info: %w", err)
	}

	// Parse config.go for schema generation
	configFields, err := g.parseConfig()
	if err != nil {
		return fmt.Errorf("failed to parse config: %w", err)
	}

	// Generate metadata.yaml
	if err := g.generateMetadata(contexts, moduleInfo); err != nil {
		return fmt.Errorf("failed to generate metadata.yaml: %w", err)
	}

	// Generate config_schema.json
	if err := g.generateConfigSchema(configFields); err != nil {
		return fmt.Errorf("failed to generate config_schema.json: %w", err)
	}

	// Generate README.md
	if err := g.generateReadme(contexts, moduleInfo, configFields); err != nil {
		return fmt.Errorf("failed to generate README.md: %w", err)
	}

	return nil
}

func (g *DocGenerator) parseContexts() (*Config, error) {
	data, err := os.ReadFile(g.ContextFile)
	if err != nil {
		return nil, err
	}

	var config Config
	if err := yaml.Unmarshal(data, &config.Classes); err != nil {
		return nil, err
	}

	return &config, nil
}

func (g *DocGenerator) parseModuleInfo() (*ModuleInfo, error) {
	// Try to read module.yaml file with module-specific info
	if _, err := os.Stat(g.ModuleInfo); os.IsNotExist(err) {
		// Create default module info
		return &ModuleInfo{
			Name:        g.ModuleName,
			DisplayName: strings.Title(g.ModuleName),
			Description: fmt.Sprintf("Monitor %s metrics", strings.Title(g.ModuleName)),
			Icon:        "icon.svg",
			Categories:  []string{"data-collection.generic"},
			Link:        "https://example.com",
		}, nil
	}

	data, err := os.ReadFile(g.ModuleInfo)
	if err != nil {
		return nil, err
	}

	var info struct {
		Name        string   `yaml:"name"`
		DisplayName string   `yaml:"display_name"`
		Description string   `yaml:"description"`
		Icon        string   `yaml:"icon"`
		Categories  []string `yaml:"categories"`
		Link        string   `yaml:"link"`
	}
	if err := yaml.Unmarshal(data, &info); err != nil {
		return nil, err
	}

	return &ModuleInfo{
		Name:        info.Name,
		DisplayName: info.DisplayName,
		Description: info.Description,
		Icon:        info.Icon,
		Categories:  info.Categories,
		Link:        info.Link,
	}, nil
}

func (g *DocGenerator) parseConfig() ([]ConfigField, error) {
	// Try to parse the actual Go file - this MUST succeed
	fields, defaults, err := g.parseConfigFromGoFile()
	if err != nil {
		// FAIL HARD - no fallback for validation errors
		return nil, fmt.Errorf("config validation failed: %w", err)
	}

	// If we got no fields from parsing, this is also an error
	if len(fields) == 0 {
		return nil, fmt.Errorf("no configuration fields found in config.go")
	}

	// Ensure standard fields are present even if they come from embedded structs
	ensureField := func(name string, desc string, defaultKey string, fallback interface{}, fieldType string, uiGroup string, uiWidget string, min *int, max *int) {
		for _, f := range fields {
			if f.JSONName == name {
				return
			}
		}

		defaultVal := fallback
		if defaultKey != "" && defaults != nil {
			if val, ok := defaults[defaultKey]; ok {
				defaultVal = val
			}
		}
		fields = append([]ConfigField{ConfigField{
			Name:        name,
			JSONName:    name,
			Type:        fieldType,
			Required:    false,
			Default:     defaultVal,
			Description: desc,
			Minimum:     min,
			Maximum:     max,
			GoType:      fieldType,
			UIGroup:     uiGroup,
			UIWidget:    uiWidget,
		}}, fields...)
	}

	ensureField("update_every", "Data collection frequency", "UpdateEvery", 10, "integer", "Connection", "", intPtr(1), nil)

	if g.hasHTTPConfig {
		ensureField("url", "Target URL", "", "", "string", "Connection", "", nil, nil)
		ensureField("username", "Username for authentication", "", "", "string", "Connection", "", nil, nil)
		ensureField("password", "Password for authentication", "", "", "string", "Connection", "password", nil, nil)
		ensureField("timeout", "Request timeout in seconds", "HTTPConfig.ClientConfig.Timeout", 0, "integer", "Connection", "", nil, nil)
		ensureField("not_follow_redirects", "Disable HTTP redirects", "", false, "boolean", "HTTP", "", nil, nil)
		ensureField("proxy_url", "Proxy URL", "", "", "string", "HTTP", "", nil, nil)
		ensureField("proxy_username", "Proxy username", "", "", "string", "HTTP", "", nil, nil)
		ensureField("proxy_password", "Proxy password", "", "", "string", "HTTP", "password", nil, nil)
		ensureField("headers", "Custom headers", "", nil, "object", "HTTP", "", nil, nil)
		ensureField("tls_skip_verify", "Skip TLS certificate verification", "", false, "boolean", "HTTP", "", nil, nil)
		ensureField("tls_ca", "Custom CA bundle path", "", "", "string", "HTTP", "", nil, nil)
		ensureField("tls_cert", "Client certificate path", "", "", "string", "HTTP", "", nil, nil)
		ensureField("tls_key", "Client key path", "", "", "string", "HTTP", "", nil, nil)
	}

	return fields, nil
}

func (g *DocGenerator) getFallbackConfigFields() []ConfigField {
	return []ConfigField{
		{
			Name:        "update_every",
			JSONName:    "update_every",
			Type:        "integer",
			Required:    false,
			Default:     10,
			Description: "Data collection frequency",
			Minimum:     intPtr(1),
		},
		{
			Name:        "reset_statistics",
			JSONName:    "reset_statistics",
			Type:        "boolean",
			Required:    false,
			Default:     false,
			Description: "ResetStatistics enables SQL calls that reset IBM i system statistics on each run.",
		},
		{
			Name:        "endpoint",
			JSONName:    "endpoint",
			Type:        "string",
			Required:    false,
			Default:     "dummy://localhost",
			Description: "Connection endpoint",
			Examples:    []string{"dummy://localhost", "tcp://server:1414"},
		},
		{
			Name:        "connect_timeout",
			JSONName:    "connect_timeout",
			Type:        "integer",
			Required:    false,
			Default:     5,
			Description: "Connection timeout in seconds",
			Minimum:     intPtr(1),
			Maximum:     intPtr(300),
		},
		{
			Name:        "collect_items",
			JSONName:    "collect_items",
			Type:        "boolean",
			Required:    false,
			Default:     true,
			Description: "Enable collection of item metrics",
		},
		{
			Name:        "max_items",
			JSONName:    "max_items",
			Type:        "integer",
			Required:    false,
			Default:     10,
			Description: "Maximum number of items to collect",
			Minimum:     intPtr(1),
			Maximum:     intPtr(1000),
		},
	}
}

func intPtr(i int) *int {
	return &i
}

func indent(spaces int, text string) string {
	if text == "" {
		return ""
	}
	prefix := strings.Repeat(" ", spaces)
	lines := strings.Split(text, "\n")
	for i, line := range lines {
		if line == "" {
			lines[i] = ""
		} else {
			lines[i] = prefix + line
		}
	}
	return strings.Join(lines, "\n")
}

func (g *DocGenerator) generateMetadata(contexts *Config, moduleInfo *ModuleInfo) error {
	tmpl := template.Must(template.New("metadata").Funcs(template.FuncMap{
		"lower":  strings.ToLower,
		"title":  strings.Title,
		"indent": indent,
	}).Parse(metadataTemplate))

	// Prepare template data
	data := struct {
		ModuleInfo *ModuleInfo
		Contexts   *Config
		ModuleName string
	}{
		ModuleInfo: moduleInfo,
		Contexts:   contexts,
		ModuleName: g.ModuleName,
	}

	// Create output file
	outFile := filepath.Join(g.OutputDir, "metadata.yaml")
	file, err := os.Create(outFile)
	if err != nil {
		return err
	}
	defer file.Close()

	return tmpl.Execute(file, data)
}

func (g *DocGenerator) generateConfigSchema(fields []ConfigField) error {
	// Create schema manually to avoid template issues
	schema := map[string]interface{}{
		"jsonSchema": map[string]interface{}{
			"$schema": "http://json-schema.org/draft-07/schema#",
			"title":   fmt.Sprintf("%s collector configuration", g.ModuleName),
			"type":    "object",
		},
	}

	properties := make(map[string]interface{})
	var required []string
	groupOrder := make([]string, 0)
	groupFields := make(map[string][]string)
	fieldUIOptions := make(map[string]map[string]interface{})

	for _, field := range fields {
		prop := map[string]interface{}{
			"title": field.Description,
			"type":  field.Type,
		}

		if field.Default != nil {
			prop["default"] = field.Default
		}

		if field.Format != "" {
			prop["format"] = field.Format
		}

		if field.Minimum != nil {
			prop["minimum"] = *field.Minimum
		}

		if field.Maximum != nil {
			prop["maximum"] = *field.Maximum
		}

		if len(field.Examples) > 0 {
			prop["examples"] = field.Examples
		}

		if len(field.Enum) > 0 {
			prop["enum"] = field.Enum
		}

		properties[field.JSONName] = prop

		if field.Required {
			required = append(required, field.JSONName)
		}

		group := field.UIGroup
		if group == "" {
			group = "Advanced"
		}
		if _, exists := groupFields[group]; !exists {
			groupOrder = append(groupOrder, group)
		}
		groupFields[group] = append(groupFields[group], field.JSONName)

		if field.UIWidget != "" || field.UIHelp != "" || field.UIPlaceholder != "" {
			opts := make(map[string]interface{})
			if field.UIWidget != "" {
				opts["ui:widget"] = field.UIWidget
			}
			if field.UIHelp != "" {
				opts["ui:help"] = field.UIHelp
			}
			if field.UIPlaceholder != "" {
				opts["ui:placeholder"] = field.UIPlaceholder
			}
			fieldUIOptions[field.JSONName] = opts
		}
	}

	schema["jsonSchema"].(map[string]interface{})["properties"] = properties
	if len(required) > 0 {
		schema["jsonSchema"].(map[string]interface{})["required"] = required
	}

	uiSchema := map[string]interface{}{
		"uiOptions": map[string]interface{}{
			"fullPage": true,
		},
	}

	if len(groupOrder) > 0 {
		tabs := make([]map[string]interface{}, 0, len(groupOrder))
		for _, group := range groupOrder {
			fieldsForGroup := groupFields[group]
			if len(fieldsForGroup) == 0 {
				continue
			}
			tabs = append(tabs, map[string]interface{}{
				"title":  group,
				"fields": fieldsForGroup,
			})
		}
		if len(tabs) > 0 {
			uiSchema["ui:flavour"] = "tabs"
			uiSchema["ui:options"] = map[string]interface{}{
				"tabs": tabs,
			}
		}
	}

	for fieldName, opts := range fieldUIOptions {
		uiSchema[fieldName] = opts
	}

	schema["uiSchema"] = uiSchema

	// Marshal to JSON with proper formatting
	data, err := json.MarshalIndent(schema, "", "  ")
	if err != nil {
		return err
	}

	// Create output file
	outFile := filepath.Join(g.OutputDir, "config_schema.json")
	return os.WriteFile(outFile, data, 0644)
}

func (g *DocGenerator) generateReadme(contexts *Config, moduleInfo *ModuleInfo, fields []ConfigField) error {
	tmpl := template.Must(template.New("readme").Funcs(template.FuncMap{
		"lower": strings.ToLower,
		"title": strings.Title,
	}).Parse(readmeTemplate))

	// Prepare template data
	data := struct {
		ModuleInfo *ModuleInfo
		Contexts   *Config
		Fields     []ConfigField
		ModuleName string
	}{
		ModuleInfo: moduleInfo,
		Contexts:   contexts,
		Fields:     fields,
		ModuleName: g.ModuleName,
	}

	// Create output file
	outFile := filepath.Join(g.OutputDir, "README.md")
	file, err := os.Create(outFile)
	if err != nil {
		return err
	}
	defer file.Close()

	return tmpl.Execute(file, data)
}

const metadataTemplate = `# Generated metadata.yaml for {{.ModuleName}} module
plugin_name: ibm.d.plugin
modules:
  - meta:
      plugin_name: ibm.d.plugin
      module_name: {{.ModuleName}}
      monitored_instance:
        name: {{.ModuleInfo.DisplayName}}
        link: {{.ModuleInfo.Link}}
        categories:{{range .ModuleInfo.Categories}}
          - {{.}}{{end}}
        icon_filename: "{{.ModuleInfo.Icon}}"
      related_resources:
        integrations:
          list: []
      info_provided_to_referring_integrations:
        description: ""
      keywords:
        - {{.ModuleName}}
      most_popular: false
    overview:
      data_collection:
        metrics_description: |
{{ indent 10 .ModuleInfo.Description }}
        method_description: |
{{ indent 10 (printf "The collector connects to %s and collects metrics via its monitoring interface." .ModuleInfo.DisplayName) }}
      supported_platforms:
        include: []
        exclude: []
      multi_instance: true
      additional_permissions:
        description: ""
    setup:
      prerequisites:
        list:
          - title: Enable monitoring interface
            description: |
              Ensure the {{.ModuleInfo.DisplayName}} monitoring interface is accessible.
      configuration:
        file:
          name: ibm.d/{{.ModuleName}}.conf
        options:
          description: |
            Configuration options for the {{.ModuleName}} collector.
          folding:
            title: Config options
            enabled: true
          list:
            - name: update_every
              description: Data collection frequency.
              default_value: 1
              required: false
            - name: endpoint
              description: Connection endpoint.
              default_value: "dummy://localhost"
              required: false
        examples:
          folding:
            enabled: true
            title: Config
          list:
            - name: Basic
              description: Basic configuration example.
              config: |
                jobs:
                  - name: local
                    endpoint: dummy://localhost
    troubleshooting:
      problems:
        list: []
    alerts: []
    metrics:
      folding:
        title: Metrics
        enabled: false
      description: ""
      availability: []
      scopes:{{range $className, $class := .Contexts.Classes}}{{if not $class.Labels}}
        - name: global
          description: These metrics refer to the entire monitored instance.
          labels: []
          metrics:{{range $class.Contexts}}
            - name: {{.Context}}
              description: {{.Title}}
              unit: {{.Units}}
              chart_type: {{.Type}}
              dimensions:{{range .Dimensions}}
                - name: {{.Name}}{{end}}{{end}}{{else}}
        - name: {{lower $className}}
          description: These metrics refer to {{lower $className}} instances.
          labels:{{range $class.Labels}}
            - name: {{.}}
              description: {{title .}} identifier{{end}}
          metrics:{{range $class.Contexts}}
            - name: {{.Context}}
              description: {{.Title}}
              unit: {{.Units}}
              chart_type: {{.Type}}
              dimensions:{{range .Dimensions}}
                - name: {{.Name}}{{end}}{{end}}{{end}}{{end}}
`

const readmeTemplate = `# {{.ModuleInfo.DisplayName}} collector

## Overview

{{.ModuleInfo.Description}}

This collector is part of the [Netdata](https://github.com/netdata/netdata) monitoring solution.

## Collected metrics

Metrics grouped by scope.

The scope defines the instance that the metric belongs to. An instance is uniquely identified by a set of labels.

### Per {{.ModuleInfo.DisplayName}} instance

{{range $className, $class := .Contexts.Classes}}{{if not $class.Labels}}
These metrics refer to the entire monitored {{$.ModuleInfo.DisplayName}} instance.

This scope has no labels.

Metrics:

| Metric | Dimensions | Unit |
|:-------|:-----------|:-----|
{{range $class.Contexts}}| {{.Context}} | {{range $i, $dim := .Dimensions}}{{if $i}}, {{end}}{{$dim.Name}}{{end}} | {{.Units}} |
{{end}}{{end}}{{end}}

{{range $className, $class := .Contexts.Classes}}{{if $class.Labels}}
### Per {{lower $className}}

These metrics refer to individual {{lower $className}} instances.

Labels:

| Label | Description |
|:------|:------------|{{range $class.Labels}}
| {{.}} | {{title .}} identifier |{{end}}

Metrics:

| Metric | Dimensions | Unit |
|:-------|:-----------|:-----|
{{range $class.Contexts}}| {{.Context}} | {{range $i, $dim := .Dimensions}}{{if $i}}, {{end}}{{$dim.Name}}{{end}} | {{.Units}} |
{{end}}{{end}}{{end}}

## Configuration

### File

The configuration file name for this integration is ` + "`" + `ibm.d/{{.ModuleName}}.conf` + "`" + `.

You can edit the configuration file using the ` + "`" + `edit-config` + "`" + ` script from the
Netdata [config directory](https://github.com/netdata/netdata/blob/master/docs/netdata-agent/configuration.md#the-netdata-config-directory).

` + "```" + `bash
cd /etc/netdata 2>/dev/null || cd /opt/netdata/etc/netdata
sudo ./edit-config ibm.d/{{.ModuleName}}.conf
` + "```" + `

### Options

The following options can be defined globally or per job.

| Name | Description | Default | Required | Min | Max |
|:-----|:------------|:--------|:---------|:----|:----|{{range .Fields}}
| {{.Name}} | {{.Description}} | ` + "`" + `{{.Default}}` + "`" + ` | {{if .Required}}yes{{else}}no{{end}} | {{if .Minimum}}{{.Minimum}}{{else}}-{{end}} | {{if .Maximum}}{{.Maximum}}{{else}}-{{end}} |{{end}}

### Examples

#### Basic configuration

{{$.ModuleInfo.DisplayName}} monitoring with default settings.

<details>
<summary>Config</summary>

` + "```" + `yaml
jobs:
  - name: local
    endpoint: dummy://localhost
` + "```" + `

</details>

## Troubleshooting

### Debug Mode

To troubleshoot issues with the ` + "`" + `{{.ModuleName}}` + "`" + ` collector, run the ` + "`" + `ibm.d.plugin` + "`" + ` with the debug option enabled.
The output should give you clues as to why the collector isn't working.

- Navigate to the ` + "`" + `plugins.d` + "`" + ` directory, usually at ` + "`" + `/usr/libexec/netdata/plugins.d/` + "`" + `
- Switch to the ` + "`" + `netdata` + "`" + ` user
- Run the ` + "`" + `ibm.d.plugin` + "`" + ` to debug the collector:

` + "```" + `bash
sudo -u netdata ./ibm.d.plugin -d -m {{.ModuleName}}
` + "```" + `

## Getting Logs

If you're encountering problems with the ` + "`" + `{{.ModuleName}}` + "`" + ` collector, follow these steps to retrieve logs and identify potential issues:

- **Run the command** specific to your system (systemd, non-systemd, or Docker container).
- **Examine the output** for any warnings or error messages that might indicate issues. These messages will typically provide clues about the root cause of the problem.

### For systemd systems (most Linux distributions)

` + "```" + `bash
sudo journalctl -u netdata --reverse | grep {{.ModuleName}}
` + "```" + `

### For non-systemd systems

` + "```" + `bash
sudo grep {{.ModuleName}} /var/log/netdata/error.log
sudo grep {{.ModuleName}} /var/log/netdata/collector.log
` + "```" + `

### For Docker containers

` + "```" + `bash
sudo docker logs netdata 2>&1 | grep {{.ModuleName}}
` + "```" + `
`
