// SPDX-License-Identifier: GPL-3.0-or-later

package promproof

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/netdata/netdata/go/plugins/internal/promprofile/semantics"
)

type CompiledCatalog struct {
	Bundles map[string]*CompiledBundle
}

// Discover loads every proof descriptor directly below the proof root.
func Discover(repoRoot string) ([]Bundle, error) {
	proofRoot := filepath.Join(repoRoot, filepath.FromSlash(ProofRoot))
	entries, err := os.ReadDir(proofRoot)
	if err != nil {
		return nil, fmt.Errorf("read proof root: %w", err)
	}
	bundles := make([]Bundle, 0, len(entries))
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		path := filepath.Join(proofRoot, entry.Name(), DescriptorFilename)
		bundle, err := Load(repoRoot, path)
		if err != nil {
			return nil, fmt.Errorf("proof directory %q: %w", entry.Name(), err)
		}
		bundles = append(bundles, bundle)
	}
	if len(bundles) == 0 {
		return nil, errors.New("no Prometheus profile proof descriptors found")
	}
	slices.SortFunc(bundles, func(left, right Bundle) int {
		return strings.Compare(left.Descriptor.Profile, right.Descriptor.Profile)
	})
	return bundles, nil
}

// LoadCompiledCatalog parses and compiles each joined proof bundle once.
func LoadCompiledCatalog(
	ctx context.Context,
	repoRoot, testdataRoot string,
	bundles []Bundle,
) (*CompiledCatalog, error) {
	if ctx == nil {
		return nil, fmt.Errorf("compile proof catalog: context is nil")
	}
	indexed := make(map[string]Bundle, len(bundles))
	for _, bundle := range bundles {
		if err := bundle.validate(); err != nil {
			return nil, fmt.Errorf("compile proof catalog: %w", err)
		}
		profile := bundle.Descriptor.Profile
		if _, ok := indexed[profile]; ok {
			return nil, fmt.Errorf("compile proof catalog: duplicate profile %q", profile)
		}
		indexed[profile] = bundle
	}
	if len(indexed) == 0 {
		return nil, fmt.Errorf("compile proof catalog: no bundles supplied")
	}

	loader := &catalogLoader{
		ctx:          ctx,
		repoRoot:     repoRoot,
		testdataRoot: testdataRoot,
		bundles:      indexed,
		programs:     make(map[string]*promsemantics.CompiledSemanticContract, len(indexed)),
		visiting:     make(map[string]struct{}),
	}
	compiled := &CompiledCatalog{Bundles: make(map[string]*CompiledBundle, len(indexed))}
	for _, profile := range sortedMapKeys(indexed) {
		program, err := loader.compile(profile)
		if err != nil {
			return nil, err
		}
		proof, err := Compile(ctx, indexed[profile], program)
		if err != nil {
			return nil, err
		}
		compiled.Bundles[profile] = proof
	}
	return compiled, nil
}

type catalogLoader struct {
	ctx          context.Context
	repoRoot     string
	testdataRoot string
	bundles      map[string]Bundle
	programs     map[string]*promsemantics.CompiledSemanticContract
	visiting     map[string]struct{}
}

func (l *catalogLoader) compile(profile string) (*promsemantics.CompiledSemanticContract, error) {
	if err := l.ctx.Err(); err != nil {
		return nil, fmt.Errorf("compile proof profile %q: %w", profile, err)
	}
	if program := l.programs[profile]; program != nil {
		return program, nil
	}
	if _, ok := l.visiting[profile]; ok {
		return nil, fmt.Errorf("compile proof profile %q: composition cycle", profile)
	}
	bundle, ok := l.bundles[profile]
	if !ok {
		return nil, fmt.Errorf("compile proof: support profile %q has no proof bundle", profile)
	}
	l.visiting[profile] = struct{}{}
	defer delete(l.visiting, profile)

	contract, err := l.loadContract(bundle)
	if err != nil {
		return nil, err
	}
	supports := make(map[string]*promsemantics.CompiledSemanticContract, len(contract.Design.Composition.Supports))
	for _, support := range sortedMapKeys(contract.Design.Composition.Supports) {
		program, err := l.compile(support)
		if err != nil {
			return nil, fmt.Errorf("compile proof profile %q support %q: %w", profile, support, err)
		}
		supports[support] = program
	}
	program, err := promsemantics.CompileSemanticContract(l.ctx, promsemantics.SemanticCompileInput{
		Contract: contract,
		Supports: supports,
	})
	if err != nil {
		return nil, fmt.Errorf("compile proof profile %q: %w", profile, err)
	}
	l.programs[profile] = program
	return program, nil
}

func (l *catalogLoader) loadContract(bundle Bundle) (promsemantics.SemanticContract, error) {
	profile := bundle.Descriptor.Profile
	if err := verifyLocalLayout(l.repoRoot, bundle); err != nil {
		return promsemantics.SemanticContract{}, fmt.Errorf("compile proof profile %q: %w", profile, err)
	}
	operatorModel := filepath.Join(l.repoRoot, filepath.FromSlash(bundle.OperatorModelPath()))
	if err := requireRegularFile(operatorModel); err != nil {
		return promsemantics.SemanticContract{}, fmt.Errorf("compile proof profile %q operator model: %w", profile, err)
	}
	paths := promsemantics.SemanticContractPaths{
		ProfileDesign:   filepath.Join(l.repoRoot, filepath.FromSlash(bundle.ProfileDesignPath())),
		SourceSemantics: filepath.Join(l.testdataRoot, filepath.FromSlash(bundle.SourceSemanticsPath())),
	}
	registry := filepath.Join(l.testdataRoot, filepath.FromSlash(bundle.SourceRegistryPath()))
	generator := filepath.Join(l.testdataRoot, filepath.FromSlash(bundle.SourceRegistryGeneratorPath()))
	registryPresent, err := regularFilePresence(registry)
	if err != nil {
		return promsemantics.SemanticContract{}, fmt.Errorf("compile proof profile %q registry: %w", profile, err)
	}
	generatorPresent, err := regularFilePresence(generator)
	if err != nil {
		return promsemantics.SemanticContract{}, fmt.Errorf("compile proof profile %q registry generator: %w", profile, err)
	}
	if registryPresent != generatorPresent {
		return promsemantics.SemanticContract{}, fmt.Errorf(
			"compile proof profile %q: source registry and generator manifest must be present together", profile)
	}
	if err := verifyExternalLayout(l.testdataRoot, bundle, registryPresent); err != nil {
		return promsemantics.SemanticContract{}, fmt.Errorf("compile proof profile %q: %w", profile, err)
	}
	if registryPresent {
		paths.SourceRegistry = registry
		paths.SourceRegistryGenerator = generator
	}
	contract, err := promsemantics.LoadSemanticContract(l.ctx, paths)
	if err != nil {
		return promsemantics.SemanticContract{}, fmt.Errorf("compile proof profile %q: %w", profile, err)
	}
	return contract, nil
}

func verifyLocalLayout(repoRoot string, bundle Bundle) error {
	directory := filepath.Join(repoRoot, filepath.FromSlash(bundle.ProofDirectory()))
	expected := map[string]struct{}{
		DescriptorFilename:                        {},
		promsemantics.ProfileDesignFilename:       {},
		filepath.Base(bundle.OperatorModelPath()): {},
	}
	return verifyExactRegularFiles(directory, expected, "local proof")
}

func verifyExternalLayout(testdataRoot string, bundle Bundle, generated bool) error {
	directory := filepath.Join(testdataRoot, filepath.FromSlash(bundle.ExternalRoot()))
	expectedFiles := map[string]struct{}{
		promsemantics.SourceFilename: {},
	}
	expectedDirectories := map[string]struct{}{"fixtures": {}}
	if generated {
		expectedFiles[promsemantics.SourceRegistryFilename] = struct{}{}
		expectedFiles[promsemantics.SourceRegistryGeneratorFilename] = struct{}{}
		expectedDirectories[promsemantics.SourceRegistryGeneratorDirectory] = struct{}{}
	}
	entries, err := os.ReadDir(directory)
	if err != nil {
		return fmt.Errorf("read external proof directory: %w", err)
	}
	seenFiles := make(map[string]struct{}, len(expectedFiles))
	seenDirectories := make(map[string]struct{}, len(expectedDirectories))
	for _, entry := range entries {
		name := entry.Name()
		if _, ok := expectedDirectories[name]; ok && entry.IsDir() && entry.Type()&os.ModeSymlink == 0 {
			seenDirectories[name] = struct{}{}
			continue
		}
		if _, ok := expectedFiles[name]; ok && entry.Type()&os.ModeSymlink == 0 && !entry.IsDir() {
			info, err := entry.Info()
			if err != nil {
				return fmt.Errorf("inspect external proof artifact %q: %w", name, err)
			}
			if info.Mode().IsRegular() {
				seenFiles[name] = struct{}{}
				continue
			}
		}
		return fmt.Errorf("unexpected external proof artifact %q", name)
	}
	for _, name := range sortedSetKeys(expectedFiles) {
		if _, ok := seenFiles[name]; !ok {
			return fmt.Errorf("external proof artifact %q is missing", name)
		}
	}
	for _, name := range sortedSetKeys(expectedDirectories) {
		if _, ok := seenDirectories[name]; !ok {
			return fmt.Errorf("external proof directory %q is missing", name)
		}
	}
	return verifyFixtureLayout(directory, bundle)
}

func verifyFixtureLayout(externalRoot string, bundle Bundle) error {
	expected := make(map[string]struct{})
	for _, definition := range bundle.Descriptor.Cases {
		if definition.Fixture != "" {
			expected[filepath.ToSlash(definition.Fixture)] = struct{}{}
		}
		for _, step := range definition.Steps {
			expected[filepath.ToSlash(step.Fixture)] = struct{}{}
		}
	}
	seen := make(map[string]struct{}, len(expected))
	fixtureRoot := filepath.Join(externalRoot, "fixtures")
	err := filepath.WalkDir(fixtureRoot, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if path == fixtureRoot || entry.IsDir() {
			return nil
		}
		relative, err := filepath.Rel(externalRoot, path)
		if err != nil {
			return err
		}
		name := filepath.ToSlash(relative)
		if entry.Type()&os.ModeSymlink != 0 {
			return fmt.Errorf("unexpected external proof artifact %q", name)
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		if !info.Mode().IsRegular() {
			return fmt.Errorf("unexpected external proof artifact %q", name)
		}
		if _, ok := expected[name]; !ok {
			return fmt.Errorf("unexpected external proof artifact %q", name)
		}
		seen[name] = struct{}{}
		return nil
	})
	if err != nil {
		return fmt.Errorf("verify external fixtures: %w", err)
	}
	for _, name := range sortedSetKeys(expected) {
		if _, ok := seen[name]; !ok {
			return fmt.Errorf("external proof artifact %q is missing", name)
		}
	}
	return nil
}

func verifyExactRegularFiles(directory string, expected map[string]struct{}, role string) error {
	entries, err := os.ReadDir(directory)
	if err != nil {
		return fmt.Errorf("read %s directory: %w", role, err)
	}
	seen := make(map[string]struct{}, len(expected))
	for _, entry := range entries {
		name := entry.Name()
		if _, ok := expected[name]; !ok || entry.Type()&os.ModeSymlink != 0 || entry.IsDir() {
			return fmt.Errorf("unexpected %s artifact %q", role, name)
		}
		info, err := entry.Info()
		if err != nil {
			return fmt.Errorf("inspect %s artifact %q: %w", role, name, err)
		}
		if !info.Mode().IsRegular() {
			return fmt.Errorf("unexpected %s artifact %q", role, name)
		}
		seen[name] = struct{}{}
	}
	for _, name := range sortedSetKeys(expected) {
		if _, ok := seen[name]; !ok {
			return fmt.Errorf("%s artifact %q is missing", role, name)
		}
	}
	return nil
}

func sortedSetKeys(values map[string]struct{}) []string {
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	slices.Sort(keys)
	return keys
}

func requireRegularFile(path string) error {
	present, err := regularFilePresence(path)
	if err != nil {
		return err
	}
	if !present {
		return fmt.Errorf("required regular file %q is missing", path)
	}
	return nil
}

func regularFilePresence(path string) (bool, error) {
	info, err := os.Stat(path)
	if errors.Is(err, os.ErrNotExist) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	if !info.Mode().IsRegular() {
		return false, fmt.Errorf("%q is not a regular file", path)
	}
	return true, nil
}
