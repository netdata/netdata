package main

import (
	"os"
	"path/filepath"
	"testing"

	"gopkg.in/yaml.v3"
)

func TestGenerateMetadataSerializesPageDescription(t *testing.T) {
	tests := map[string]string{
		"internal colon": "Monitor service health by state: ready, degraded, and failed across reliable production systems.",
		// The shared schema rejects multiline descriptions later. Docgen must
		// still serialize source YAML safely so validation receives valid YAML.
		"multiline rejected by central schema": "Monitor service health across reliable production systems.\nTrack degraded states.",
	}
	for name, pageDescription := range tests {
		t.Run(name, func(t *testing.T) {
			testGenerateMetadataSerializesPageDescription(t, pageDescription)
		})
	}
}

func testGenerateMetadataSerializesPageDescription(t *testing.T, pageDescription string) {
	t.Helper()
	dir := t.TempDir()
	contextsPath := filepath.Join(dir, "contexts.yaml")
	configPath := filepath.Join(dir, "config.go")
	initPath := filepath.Join(dir, "init.go")
	modulePath := filepath.Join(dir, "module.yaml")

	writeTestFile(t, contextsPath, "{}\n")
	writeTestFile(t, configPath, "package fixture\n\ntype Config struct {\n\tEndpoint string `yaml:\"endpoint,omitempty\" json:\"endpoint\"`\n}\n")
	writeTestFile(t, initPath, "package fixture\n\nfunc defaultConfig() Config {\n\treturn Config{Endpoint: \"\"}\n}\n")
	writeTestFile(t, modulePath, "name: fixture\n"+
		"display_name: Fixture\n"+
		"page_description: "+string(mustYAML(t, pageDescription))+
		`description: |
  Monitor fixture service health and performance.
icon: fixture.svg
categories:
  - data-collection.generic
link: https://example.com
`)

	generator := &DocGenerator{
		ModuleName:  "fixture",
		ContextFile: contextsPath,
		ConfigFile:  configPath,
		OutputDir:   dir,
		ModuleInfo:  modulePath,
	}
	if err := generator.Generate(); err != nil {
		t.Fatalf("docgen failed: %v", err)
	}

	data, err := os.ReadFile(filepath.Join(dir, "metadata.yaml"))
	if err != nil {
		t.Fatalf("read generated metadata: %v", err)
	}

	var metadata struct {
		Modules []struct {
			Meta struct {
				MonitoredInstance struct {
					Description string `yaml:"description"`
				} `yaml:"monitored_instance"`
			} `yaml:"meta"`
		} `yaml:"modules"`
	}
	if err := yaml.Unmarshal(data, &metadata); err != nil {
		t.Fatalf("parse generated metadata: %v\n%s", err, data)
	}
	if len(metadata.Modules) != 1 {
		t.Fatalf("generated %d modules, want 1", len(metadata.Modules))
	}
	if got := metadata.Modules[0].Meta.MonitoredInstance.Description; got != pageDescription {
		t.Fatalf("generated page description %q, want %q", got, pageDescription)
	}
}

func mustYAML(t *testing.T, value string) []byte {
	t.Helper()
	data, err := yaml.Marshal(value)
	if err != nil {
		t.Fatalf("marshal fixture YAML: %v", err)
	}
	return data
}

func writeTestFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}
