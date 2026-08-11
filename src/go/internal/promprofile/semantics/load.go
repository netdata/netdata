// SPDX-License-Identifier: GPL-3.0-or-later

package promsemantics

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/netdata/netdata/go/plugins/internal/promprofile/yaml"
)

const (
	SourceFilename                    = "SOURCE-SEMANTICS.yaml"
	ProfileDesignFilename             = "PROFILE-DESIGN.yaml"
	SourceRegistryFilename            = "SOURCE-REGISTRY.yaml"
	SourceRegistryGeneratorFilename   = "SOURCE-REGISTRY.generator.yaml"
	SourceRegistryGeneratorDirectory  = "generator"
	SourceRegistryGeneratorEntrypoint = "generate.py"

	ProfileDesignVersion           = "v1"
	SourceSemanticsVersion         = "v1"
	SourceRegistryVersion          = "v1"
	SourceRegistryGeneratorVersion = "v1"
)

func LoadProfileDesign(path string) (ProfileDesignDocument, error) {
	document, err := promyaml.DecodeFile[ProfileDesignDocument](
		path,
		"version",
		"profile",
		"match",
		"namespace",
		"composition",
		"entities",
		"views",
	)
	if err != nil {
		return ProfileDesignDocument{}, fmt.Errorf("profile design: %w", err)
	}
	if err := document.validate(); err != nil {
		return ProfileDesignDocument{}, fmt.Errorf("profile design: %w", err)
	}
	return document, nil
}

func LoadSourceRegistry(path string) (SourceRegistryDocument, error) {
	document, err := promyaml.DecodeFile[SourceRegistryDocument](
		path,
		"version",
		"profile",
		"generated",
		"family_grammars",
		"groups",
	)
	if err != nil {
		return SourceRegistryDocument{}, fmt.Errorf("source registry: %w", err)
	}
	if err := document.validate(); err != nil {
		return SourceRegistryDocument{}, fmt.Errorf("source registry: %w", err)
	}
	return document, nil
}

func LoadSourceRegistryGenerator(path string) (SourceRegistryGeneratorDocument, error) {
	document, err := promyaml.DecodeFile[SourceRegistryGeneratorDocument](
		path,
		"version",
		"profile",
		"runner",
		"upstreams",
	)
	if err != nil {
		return SourceRegistryGeneratorDocument{}, fmt.Errorf("source registry generator: %w", err)
	}
	if err := document.validate(); err != nil {
		return SourceRegistryGeneratorDocument{}, fmt.Errorf("source registry generator: %w", err)
	}
	return document, nil
}

func LoadSourceRegistryPair(registryPath, generatorPath string) (SourceRegistryPair, error) {
	if err := validateSourceRegistryPairPaths(registryPath, generatorPath); err != nil {
		return SourceRegistryPair{}, fmt.Errorf("source registry pair: %w", err)
	}
	registry, err := LoadSourceRegistry(registryPath)
	if err != nil {
		return SourceRegistryPair{}, err
	}
	generator, err := LoadSourceRegistryGenerator(generatorPath)
	if err != nil {
		return SourceRegistryPair{}, err
	}
	pair := SourceRegistryPair{Registry: registry, Generator: generator}
	if err := pair.validate(); err != nil {
		return SourceRegistryPair{}, fmt.Errorf("source registry pair: %w", err)
	}
	return pair, nil
}

func validateSourceRegistryPairPaths(registryPath, generatorPath string) error {
	if filepath.Base(registryPath) != SourceRegistryFilename {
		return fmt.Errorf("registry path must end in %q", SourceRegistryFilename)
	}
	if filepath.Base(generatorPath) != SourceRegistryGeneratorFilename {
		return fmt.Errorf("generator manifest path must end in %q", SourceRegistryGeneratorFilename)
	}
	if filepath.Clean(filepath.Dir(registryPath)) != filepath.Clean(filepath.Dir(generatorPath)) {
		return fmt.Errorf("registry and generator manifest must be colocated")
	}
	return validateSourceRegistryGeneratorDirectory(
		filepath.Join(filepath.Dir(generatorPath), SourceRegistryGeneratorDirectory),
	)
}

func validateSourceRegistryGeneratorDirectory(path string) error {
	info, err := os.Lstat(path)
	if err != nil {
		return fmt.Errorf("generator directory %q: %w", path, err)
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.IsDir() {
		return fmt.Errorf("generator directory %q must be a real directory", path)
	}
	entries, err := os.ReadDir(path)
	if err != nil {
		return fmt.Errorf("read generator directory %q: %w", path, err)
	}
	entrypoint := false
	negativeTest := false
	for _, entry := range entries {
		name := entry.Name()
		if entry.Type()&os.ModeSymlink != 0 || entry.IsDir() {
			return fmt.Errorf("generator directory contains non-regular entry %q", name)
		}
		entryInfo, err := entry.Info()
		if err != nil {
			return fmt.Errorf("inspect generator file %q: %w", name, err)
		}
		if !entryInfo.Mode().IsRegular() || filepath.Ext(name) != ".py" || !validID(strings.TrimSuffix(name, ".py")) {
			return fmt.Errorf("generator directory contains unexpected file %q", name)
		}
		entrypoint = entrypoint || name == SourceRegistryGeneratorEntrypoint
		negativeTest = negativeTest || strings.HasPrefix(name, "test_negative_")
	}
	if !entrypoint {
		return fmt.Errorf("generator directory must contain %q", SourceRegistryGeneratorEntrypoint)
	}
	if !negativeTest {
		return fmt.Errorf("generator directory must contain a test_negative_*.py fail-closed test")
	}
	return nil
}

func LoadSourceSemantics(path string) (SourceSemanticsDocument, error) {
	document, err := promyaml.DecodeFile[SourceSemanticsDocument](
		path,
		"version",
		"profile",
		"upstreams",
		"evidence",
		"environment",
		"signals",
	)
	if err != nil {
		return SourceSemanticsDocument{}, fmt.Errorf("source semantics: %w", err)
	}
	if err := document.validate(); err != nil {
		return SourceSemanticsDocument{}, fmt.Errorf("source semantics: %w", err)
	}
	return document, nil
}
