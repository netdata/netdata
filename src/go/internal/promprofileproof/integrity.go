// SPDX-License-Identifier: GPL-3.0-or-later

package promprofileproof

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"gopkg.in/yaml.v3"
)

type evidenceManifest struct {
	Version       int                    `yaml:"version"`
	Profile       string                 `yaml:"profile"`
	EvidenceClass string                 `yaml:"evidence_class"`
	Sanitized     bool                   `yaml:"sanitized"`
	Files         []evidenceManifestFile `yaml:"files"`
}

type evidenceManifestFile struct {
	Path   string `yaml:"path"`
	Kind   string `yaml:"kind"`
	SHA256 string `yaml:"sha256"`
	Bytes  int64  `yaml:"bytes"`
}

func Verify(repoRoot, testdataRoot string, bundle Bundle) error {
	if err := VerifyLocal(repoRoot, bundle); err != nil {
		return err
	}
	return VerifyExternal(testdataRoot, bundle)
}

func VerifyLocal(repoRoot string, bundle Bundle) error {
	if err := verifyLocalLayout(repoRoot, bundle); err != nil {
		return err
	}
	for _, entry := range bundle.IntegrityEntries() {
		if err := verifyFile(filepath.Join(repoRoot, filepath.FromSlash(entry.Path)), entry.SHA256, entry.Bytes); err != nil {
			return fmt.Errorf("local artifact %q: %w", entry.Path, err)
		}
	}
	return nil
}

func VerifyExternal(testdataRoot string, bundle Bundle) error {
	manifest := bundle.Descriptor.External.Manifest
	manifestPath := filepath.Join(testdataRoot, filepath.FromSlash(bundle.ExternalManifestPath()))
	if err := verifyFile(manifestPath, manifest.SHA256, manifest.Bytes); err != nil {
		return fmt.Errorf("external manifest %q: %w", bundle.ExternalManifestPath(), err)
	}
	if err := verifyEvidenceManifest(manifestPath, bundle); err != nil {
		return fmt.Errorf("external manifest %q: %w", bundle.ExternalManifestPath(), err)
	}
	return verifySourceInventory(testdataRoot, bundle)
}

func Refresh(repoRoot, testdataRoot string, bundle Bundle) (Bundle, error) {
	if err := verifyLocalLayout(repoRoot, bundle); err != nil {
		return Bundle{}, err
	}
	for _, target := range bundle.integrityTargets() {
		digest, size, err := digestFile(filepath.Join(repoRoot, filepath.FromSlash(target.path)))
		if err != nil {
			return Bundle{}, fmt.Errorf("local artifact %q: %w", target.path, err)
		}
		*target.digest = FileDigest{SHA256: digest, Bytes: size}
	}
	manifest := &bundle.Descriptor.External.Manifest
	manifestPath := filepath.Join(testdataRoot, filepath.FromSlash(bundle.ExternalManifestPath()))
	if err := verifyEvidenceManifest(manifestPath, bundle); err != nil {
		return Bundle{}, fmt.Errorf("external manifest %q: %w", bundle.ExternalManifestPath(), err)
	}
	if err := verifySourceInventory(testdataRoot, bundle); err != nil {
		return Bundle{}, err
	}
	digest, size, err := digestFile(manifestPath)
	if err != nil {
		return Bundle{}, fmt.Errorf("external manifest %q: %w", bundle.ExternalManifestPath(), err)
	}
	manifest.SHA256 = digest
	manifest.Bytes = size
	if err := bundle.validate(); err != nil {
		return Bundle{}, fmt.Errorf("refreshed descriptor: %w", err)
	}
	return bundle, nil
}

func verifySourceInventory(testdataRoot string, bundle Bundle) error {
	path := filepath.Join(testdataRoot, filepath.FromSlash(bundle.SourceInventoryPath()))
	inventory, err := LoadSourceInventory(path)
	if err != nil {
		return fmt.Errorf("external source inventory %q: %w", bundle.SourceInventoryPath(), err)
	}
	if err := inventory.VerifyExpected(bundle.Descriptor.Inventory); err != nil {
		return fmt.Errorf("external source inventory %q: %w", bundle.SourceInventoryPath(), err)
	}
	return nil
}

func verifyLocalLayout(repoRoot string, bundle Bundle) error {
	directory := filepath.ToSlash(filepath.Dir(bundle.Path))
	want := []string{bundle.Path}
	for _, entry := range bundle.IntegrityEntries() {
		if filepath.ToSlash(filepath.Dir(filepath.FromSlash(entry.Path))) == directory {
			want = append(want, entry.Path)
		}
	}
	slices.Sort(want)

	entries, err := os.ReadDir(filepath.Join(repoRoot, filepath.FromSlash(directory)))
	if err != nil {
		return fmt.Errorf("read local proof directory %q: %w", directory, err)
	}
	actual := make([]string, 0, len(entries))
	for _, entry := range entries {
		path := directory + "/" + entry.Name()
		if entry.IsDir() {
			return fmt.Errorf("local proof path %q is an undeclared directory", path)
		}
		info, err := entry.Info()
		if err != nil {
			return fmt.Errorf("inspect local proof path %q: %w", path, err)
		}
		if !info.Mode().IsRegular() {
			return fmt.Errorf("local proof path %q is not a regular file", path)
		}
		actual = append(actual, path)
	}
	slices.Sort(actual)
	if !slices.Equal(actual, want) {
		return fmt.Errorf("local proof files differ from descriptor: got %v, want %v", actual, want)
	}
	return nil
}

func Write(repoRoot string, bundle Bundle) error {
	if err := bundle.validate(); err != nil {
		return err
	}
	var encoded bytes.Buffer
	encoder := yaml.NewEncoder(&encoded)
	encoder.SetIndent(2)
	if err := encoder.Encode(bundle.Descriptor); err != nil {
		return fmt.Errorf("encode descriptor: %w", err)
	}
	if err := encoder.Close(); err != nil {
		return fmt.Errorf("close descriptor encoder: %w", err)
	}
	content := append([]byte("# SPDX-License-Identifier: GPL-3.0-or-later\n\n"), encoded.Bytes()...)
	path := filepath.Join(repoRoot, filepath.FromSlash(bundle.Path))
	temporary, err := os.CreateTemp(filepath.Dir(path), ".proof-*.yaml")
	if err != nil {
		return fmt.Errorf("create temporary descriptor: %w", err)
	}
	temporaryPath := temporary.Name()
	cleanup := func() { _ = os.Remove(temporaryPath) }
	defer cleanup()
	if err := temporary.Chmod(0o644); err != nil {
		_ = temporary.Close()
		return fmt.Errorf("set temporary descriptor mode: %w", err)
	}
	if _, err := temporary.Write(content); err != nil {
		_ = temporary.Close()
		return fmt.Errorf("write temporary descriptor: %w", err)
	}
	if err := temporary.Sync(); err != nil {
		_ = temporary.Close()
		return fmt.Errorf("sync temporary descriptor: %w", err)
	}
	if err := temporary.Close(); err != nil {
		return fmt.Errorf("close temporary descriptor: %w", err)
	}
	if err := os.Rename(temporaryPath, path); err != nil {
		return fmt.Errorf("replace descriptor: %w", err)
	}
	return nil
}

func EvidenceDirectories(bundles []Bundle) []string {
	directories := make([]string, 0, len(bundles))
	seen := make(map[string]bool, len(bundles))
	for _, bundle := range bundles {
		directory := bundle.ExternalRoot()
		if !seen[directory] {
			seen[directory] = true
			directories = append(directories, directory)
		}
	}
	slices.Sort(directories)
	return directories
}

func verifyEvidenceManifest(path string, bundle Bundle) error {
	descriptor := bundle.Descriptor
	content, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	var manifest evidenceManifest
	decoder := yaml.NewDecoder(bytes.NewReader(content))
	decoder.KnownFields(true)
	if err := decoder.Decode(&manifest); err != nil {
		return fmt.Errorf("decode: %w", err)
	}
	var extra any
	if err := decoder.Decode(&extra); !errors.Is(err, io.EOF) {
		if err == nil {
			return errors.New("must contain exactly one YAML document")
		}
		return fmt.Errorf("decode trailing content: %w", err)
	}
	if manifest.Version != 1 {
		return fmt.Errorf("version: got %d, want 1", manifest.Version)
	}
	if manifest.Profile != descriptor.Profile {
		return fmt.Errorf("profile: got %q, want %q", manifest.Profile, descriptor.Profile)
	}
	if manifest.EvidenceClass != "source-derived-synthetic" {
		return fmt.Errorf("evidence_class: got %q, want source-derived-synthetic", manifest.EvidenceClass)
	}
	if !manifest.Sanitized {
		return errors.New("sanitized must be true")
	}
	if len(manifest.Files) == 0 {
		return errors.New("files must not be empty")
	}

	root := filepath.Dir(path)
	seen := make(map[string]bool, len(manifest.Files))
	declaredPaths := make([]string, 0, len(manifest.Files))
	foundInventory := false
	foundFixtures := make(map[string]bool, len(descriptor.Validation.Cases))
	for _, validationCase := range descriptor.Validation.Cases {
		foundFixtures[validationCase.Fixture] = false
	}
	for index, file := range manifest.Files {
		field := fmt.Sprintf("files[%d]", index)
		if err := validateRelativePath(field+".path", file.Path); err != nil {
			return err
		}
		if seen[file.Path] {
			return fmt.Errorf("duplicate evidence path %q", file.Path)
		}
		seen[file.Path] = true
		declaredPaths = append(declaredPaths, file.Path)
		if err := validateDigest(field, file.SHA256, file.Bytes); err != nil {
			return err
		}
		switch file.Kind {
		case "source_inventory":
			if file.Path != "SOURCE-INVENTORY.tsv" {
				return fmt.Errorf("source inventory %q must be SOURCE-INVENTORY.tsv", file.Path)
			}
			foundInventory = true
		case "prometheus_exposition":
			if !strings.HasPrefix(file.Path, "fixtures/") || !strings.HasSuffix(file.Path, ".prom") {
				return fmt.Errorf("Prometheus exposition %q must be a fixtures/*.prom file", file.Path)
			}
			if _, ok := foundFixtures[file.Path]; ok {
				foundFixtures[file.Path] = true
			}
		default:
			return fmt.Errorf("unsupported evidence kind %q for %q", file.Kind, file.Path)
		}
		if err := verifyFile(filepath.Join(root, filepath.FromSlash(file.Path)), file.SHA256, file.Bytes); err != nil {
			return fmt.Errorf("evidence file %q: %w", file.Path, err)
		}
	}
	if !foundInventory {
		return fmt.Errorf("descriptor source inventory %q is not declared", bundle.SourceInventoryPath())
	}
	for fixture, found := range foundFixtures {
		if !found {
			return fmt.Errorf("descriptor validation fixture %q is not declared", bundle.ExternalRoot()+"/"+fixture)
		}
	}
	slices.Sort(declaredPaths)
	actualPaths, err := evidenceFiles(root)
	if err != nil {
		return err
	}
	if !slices.Equal(declaredPaths, actualPaths) {
		return fmt.Errorf("declared evidence files differ from directory: got %v, want %v", declaredPaths, actualPaths)
	}
	return nil
}

func evidenceFiles(root string) ([]string, error) {
	var paths []string
	err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if path == root || entry.IsDir() {
			return nil
		}
		relative, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		relative = filepath.ToSlash(relative)
		if entry.Type()&os.ModeSymlink != 0 {
			return fmt.Errorf("evidence path %q is a symlink", relative)
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		if !info.Mode().IsRegular() {
			return fmt.Errorf("evidence path %q is not a regular file", relative)
		}
		if relative != "manifest.yaml" {
			paths = append(paths, relative)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	slices.Sort(paths)
	return paths, nil
}

func verifyFile(path, wantDigest string, wantSize int64) error {
	digest, size, err := digestFile(path)
	if err != nil {
		return err
	}
	if size != wantSize {
		return fmt.Errorf("byte count: got %d, want %d", size, wantSize)
	}
	if digest != wantDigest {
		return fmt.Errorf("SHA-256: got %s, want %s", digest, wantDigest)
	}
	return nil
}

func digestFile(path string) (string, int64, error) {
	file, err := os.Open(path)
	if err != nil {
		return "", 0, err
	}
	defer file.Close()
	hasher := sha256.New()
	size, err := io.Copy(hasher, file)
	if err != nil {
		return "", 0, err
	}
	return hex.EncodeToString(hasher.Sum(nil)), size, nil
}
