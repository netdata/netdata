package main

import (
	"os"
	"path/filepath"
	"testing"

	"gopkg.in/yaml.v3"
)

func TestGenerateMetadataSerializesPageDescription(t *testing.T) {
	const pageDescription = "Monitor service health by state: ready, degraded, and failed across reliable production systems."

	dir := t.TempDir()
	contextsPath := filepath.Join(dir, "contexts.yaml")
	configPath := filepath.Join(dir, "config.go")
	initPath := filepath.Join(dir, "init.go")
	modulePath := filepath.Join(dir, "module.yaml")

	writeTestFile(t, contextsPath, "{}\n")
	writeTestFile(t, configPath, "package fixture\n\ntype Config struct {\n\tEndpoint string `yaml:\"endpoint,omitempty\" json:\"endpoint\"`\n}\n")
	writeTestFile(t, initPath, "package fixture\n\nfunc defaultConfig() Config {\n\treturn Config{Endpoint: \"\"}\n}\n")
	writeTestFile(t, modulePath, `name: fixture
display_name: Fixture
page_description: "Monitor service health by state: ready, degraded, and failed across reliable production systems."
description: |
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

func writeTestFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}
