// SPDX-License-Identifier: GPL-3.0-or-later

package promsemantics

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLoadSourceRegistryPair(t *testing.T) {
	directory := t.TempDir()
	registryPath := filepath.Join(directory, SourceRegistryFilename)
	generatorPath := filepath.Join(directory, SourceRegistryGeneratorFilename)
	writeValidGeneratorDirectory(t, directory)
	writeTextFile(t, registryPath, validSourceRegistryV1)
	writeTextFile(t, generatorPath, validSourceRegistryGeneratorV1)

	pair, err := LoadSourceRegistryPair(registryPath, generatorPath)
	if err != nil {
		t.Fatalf("LoadSourceRegistryPair() error = %v", err)
	}
	if len(pair.Registry.Groups["core"].Registrations) != 2 {
		t.Fatalf("registrations = %#v", pair.Registry.Groups["core"].Registrations)
	}
}

func TestSourceRegistryRequiresExplicitRawGrammarBranches(t *testing.T) {
	path := filepath.Join(t.TempDir(), SourceRegistryFilename)
	writeTextFile(t, path, validSourceRegistryV1)
	if _, err := LoadSourceRegistry(path); err != nil {
		t.Fatalf("LoadSourceRegistry(raw branches) error = %v", err)
	}

	content := strings.Replace(validSourceRegistryV1,
		"        raw_branches: {canonical: {}, embedded: {}}\n", "", 1)
	writeTextFile(t, path, content)
	if _, err := LoadSourceRegistry(path); err == nil || !strings.Contains(err.Error(), "raw_branches") {
		t.Fatalf("LoadSourceRegistry(missing raw branches) error = %v, want raw_branches failure", err)
	}
}

func TestSourceRegistryRejectsUnknownRawGrammarBranch(t *testing.T) {
	content := strings.Replace(validSourceRegistryV1,
		"raw_branches: {canonical: {}, embedded: {}}",
		"raw_branches: {canonical: {}, normalized: {}}", 1)
	path := filepath.Join(t.TempDir(), SourceRegistryFilename)
	writeTextFile(t, path, content)
	if _, err := LoadSourceRegistry(path); err == nil || !strings.Contains(err.Error(), "normalized") {
		t.Fatalf("LoadSourceRegistry(unknown raw branch) error = %v, want normalized failure", err)
	}
}

func TestSourceRegistryRejectsCuratedRawBranchCondition(t *testing.T) {
	content := strings.Replace(validSourceRegistryV1,
		"raw_branches: {canonical: {}, embedded: {}}",
		"raw_branches: {canonical: {when: curated_policy}, embedded: {}}", 1)
	path := filepath.Join(t.TempDir(), SourceRegistryFilename)
	writeTextFile(t, path, content)
	if _, err := LoadSourceRegistry(path); err == nil || !strings.Contains(err.Error(), "inline mechanical condition") {
		t.Fatalf("LoadSourceRegistry(curated raw branch condition) error = %v, want mechanical-condition failure", err)
	}
}

func TestSourceRegistryRequiresAtomicGrammarForms(t *testing.T) {
	content := strings.Replace(
		validSourceRegistryV1,
		"family: {grammar: operation_family, form: latency}",
		"family: {grammar: operation_family}",
		1,
	)
	path := filepath.Join(t.TempDir(), SourceRegistryFilename)
	writeTextFile(t, path, content)
	if _, err := LoadSourceRegistry(path); err == nil || !strings.Contains(err.Error(), "form") {
		t.Fatalf("LoadSourceRegistry() error = %v, want form failure", err)
	}
}

func TestSourceRegistryLongestKnownSuffixIsExplicit(t *testing.T) {
	path := filepath.Join(t.TempDir(), SourceRegistryFilename)
	writeTextFile(t, path, validSourceRegistryV1)
	if _, err := LoadSourceRegistry(path); err != nil {
		t.Fatalf("LoadSourceRegistry(longest_known_suffix) error = %v", err)
	}

	content := strings.Replace(validSourceRegistryV1, "longest_known_suffix", "injective", 1)
	writeTextFile(t, path, content)
	if _, err := LoadSourceRegistry(path); err == nil || !strings.Contains(err.Error(), "noninjective") {
		t.Fatalf("LoadSourceRegistry(injective) error = %v, want noninjective failure", err)
	}
}

func TestSourceRegistryAcceptsTerminalEmbeddedIdentityGrammar(t *testing.T) {
	content := strings.Replace(validSourceRegistryV1,
		"canonical: {prefix: example_, suffix: latency}",
		"canonical: {prefix: example_, suffix: ''}", 1)
	content = strings.Replace(content,
		"suffix: latency\n          separator: _",
		"suffix: ''\n          separator: ''", 1)
	path := filepath.Join(t.TempDir(), SourceRegistryFilename)
	writeTextFile(t, path, content)
	if _, err := LoadSourceRegistry(path); err != nil {
		t.Fatalf("LoadSourceRegistry(terminal identity) error = %v", err)
	}
}

func TestSourceRegistryAcceptsNestedEmbeddedNamespaceExclusion(t *testing.T) {
	content := strings.Replace(validSourceRegistryV1,
		"          prefix: example_\n          suffix: latency",
		"          prefix: example_\n          excluded_prefixes: [example_special_]\n          suffix: latency", 1)
	path := filepath.Join(t.TempDir(), SourceRegistryFilename)
	writeTextFile(t, path, content)
	if _, err := LoadSourceRegistry(path); err != nil {
		t.Fatalf("LoadSourceRegistry(nested exclusion) error = %v", err)
	}
}

func TestSourceRegistryRejectsInvalidNestedEmbeddedNamespaceExclusion(t *testing.T) {
	tests := map[string]struct {
		prefix string
		want   string
	}{
		"same namespace": {
			prefix: "example_",
			want:   "must be a proper nested prefix",
		},
		"unrelated namespace": {
			prefix: "other_",
			want:   "must be a proper nested prefix",
		},
		"duplicate namespace": {
			prefix: "example_special_, example_special_",
			want:   "duplicate value",
		},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			content := strings.Replace(validSourceRegistryV1,
				"          prefix: example_\n          suffix: latency",
				"          prefix: example_\n          excluded_prefixes: ["+tc.prefix+"]\n          suffix: latency", 1)
			path := filepath.Join(t.TempDir(), SourceRegistryFilename)
			writeTextFile(t, path, content)
			if _, err := LoadSourceRegistry(path); err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("LoadSourceRegistry() error = %v, want %q", err, tc.want)
			}
		})
	}
}

func TestSourceRegistryRejectsPartialTerminalEmbeddedIdentityGrammar(t *testing.T) {
	content := strings.Replace(validSourceRegistryV1,
		"suffix: latency\n          separator: _",
		"suffix: ''\n          separator: _", 1)
	path := filepath.Join(t.TempDir(), SourceRegistryFilename)
	writeTextFile(t, path, content)
	if _, err := LoadSourceRegistry(path); err == nil || !strings.Contains(err.Error(), "both empty or both nonempty") {
		t.Fatalf("LoadSourceRegistry(partial terminal identity) error = %v", err)
	}
}

func TestSourceRegistryInjectivityIncludesDifferentSeparators(t *testing.T) {
	content := strings.Replace(validSourceRegistryV1, "longest_known_suffix", "injective", 1)
	content = strings.Replace(content,
		"suffix: write_latency\n          separator: _",
		"suffix: write_latency\n          separator: __", 1)
	path := filepath.Join(t.TempDir(), SourceRegistryFilename)
	writeTextFile(t, path, content)
	if _, err := LoadSourceRegistry(path); err == nil || !strings.Contains(err.Error(), "noninjective") {
		t.Fatalf("LoadSourceRegistry() error = %v, want noninjective failure", err)
	}
}

func TestSourceRegistryRejectsCuratedConditionReference(t *testing.T) {
	content := strings.Replace(validSourceRegistryV1,
		"components:\n          value: {wire_role: scalar}\n        source_locations:",
		"components:\n          value: {wire_role: scalar}\n        when: curated_policy\n        source_locations:", 1)
	path := filepath.Join(t.TempDir(), SourceRegistryFilename)
	writeTextFile(t, path, content)
	if _, err := LoadSourceRegistry(path); err == nil || !strings.Contains(err.Error(), "inline mechanical condition") {
		t.Fatalf("LoadSourceRegistry() error = %v, want mechanical-condition failure", err)
	}
}

func TestSourceRegistryPairRejectsUnknownUpstreamPath(t *testing.T) {
	directory := t.TempDir()
	registryPath := filepath.Join(directory, SourceRegistryFilename)
	generatorPath := filepath.Join(directory, SourceRegistryGeneratorFilename)
	writeValidGeneratorDirectory(t, directory)
	writeTextFile(t, registryPath, strings.Replace(validSourceRegistryV1, "metrics.go", "other.go", 1))
	writeTextFile(t, generatorPath, validSourceRegistryGeneratorV1)
	if _, err := LoadSourceRegistryPair(registryPath, generatorPath); err == nil ||
		!strings.Contains(err.Error(), "declared path closure") {
		t.Fatalf("LoadSourceRegistryPair() error = %v, want path-closure failure", err)
	}
}

func TestSourceRegistryPairRequiresConventionalGeneratorClosure(t *testing.T) {
	tests := map[string]struct {
		files map[string]string
		want  string
	}{
		"missing directory": {
			want: "generator directory",
		},
		"missing entrypoint": {
			files: map[string]string{"test_negative_parser.py": ""},
			want:  "generate.py",
		},
		"missing negative test": {
			files: map[string]string{"generate.py": ""},
			want:  "test_negative_*.py",
		},
		"unexpected file": {
			files: map[string]string{"generate.py": "", "test_negative_parser.py": "", "README.md": ""},
			want:  "unexpected file",
		},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			directory := t.TempDir()
			writeTextFile(t, filepath.Join(directory, SourceRegistryFilename), validSourceRegistryV1)
			writeTextFile(t, filepath.Join(directory, SourceRegistryGeneratorFilename), validSourceRegistryGeneratorV1)
			if tc.files != nil {
				generatorDirectory := filepath.Join(directory, SourceRegistryGeneratorDirectory)
				if err := os.Mkdir(generatorDirectory, 0o755); err != nil {
					t.Fatal(err)
				}
				for name, content := range tc.files {
					writeTextFile(t, filepath.Join(generatorDirectory, name), content)
				}
			}
			if _, err := LoadSourceRegistryPair(
				filepath.Join(directory, SourceRegistryFilename),
				filepath.Join(directory, SourceRegistryGeneratorFilename),
			); err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("LoadSourceRegistryPair() error = %v, want %q", err, tc.want)
			}
		})
	}
}

func writeValidGeneratorDirectory(t *testing.T, directory string) {
	t.Helper()
	generatorDirectory := filepath.Join(directory, SourceRegistryGeneratorDirectory)
	if err := os.Mkdir(generatorDirectory, 0o755); err != nil {
		t.Fatal(err)
	}
	writeTextFile(t, filepath.Join(generatorDirectory, SourceRegistryGeneratorEntrypoint), "")
	writeTextFile(t, filepath.Join(generatorDirectory, "test_negative_parser.py"), "")
}

const validSourceRegistryV1 = `
version: v1
profile: example
generated: true
family_grammars:
  operation_family:
    interpretation: longest_known_suffix
    forms:
      latency:
        canonical: {prefix: example_, suffix: latency}
        embedded:
          prefix: example_
          suffix: latency
          separator: _
          identity_slot: {name: worker, nonempty: true}
      write_latency:
        canonical: {prefix: example_, suffix: write_latency}
        embedded:
          prefix: example_
          suffix: write_latency
          separator: _
          identity_slot: {name: worker, nonempty: true}
groups:
  core:
    registrations:
      operation_latency:
        family: {grammar: operation_family, form: latency}
        raw_branches: {canonical: {}, embedded: {}}
        prometheus: {type: gauge, shape: scalar}
        components:
          value: {wire_role: scalar}
        source_locations:
          - {upstream: exporter, path: metrics.go, line: 10}
      operation_write_latency:
        family: {grammar: operation_family, form: write_latency}
        raw_branches: {canonical: {}, embedded: {}}
        prometheus: {type: gauge, shape: scalar}
        components:
          value: {wire_role: scalar}
        source_locations:
          - {upstream: exporter, path: metrics.go, range: {start: 11, end: 12}}
`

const validSourceRegistryGeneratorV1 = `
version: v1
profile: example
runner: netdata-prometheus-source-registry-v1
upstreams:
  exporter:
    repository: owner/exporter
    commit: 0123456789abcdef0123456789abcdef01234567
    paths: [metrics.go]
`
