// SPDX-License-Identifier: GPL-3.0-or-later

package promprofileproof

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"slices"

	"gopkg.in/yaml.v3"
)

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
	if err := verifyExternalLayout(testdataRoot, bundle); err != nil {
		return err
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
	if err := verifyExternalLayout(testdataRoot, bundle); err != nil {
		return Bundle{}, err
	}
	if err := verifySourceInventory(testdataRoot, bundle); err != nil {
		return Bundle{}, err
	}
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

func verifyExternalLayout(testdataRoot string, bundle Bundle) error {
	root := filepath.Join(testdataRoot, filepath.FromSlash(bundle.ExternalRoot()))
	want := []string{"SOURCE-INVENTORY.tsv"}
	seen := map[string]bool{"SOURCE-INVENTORY.tsv": true}
	for _, validationCase := range bundle.Descriptor.Validation.Cases {
		if !seen[validationCase.Fixture] {
			seen[validationCase.Fixture] = true
			want = append(want, validationCase.Fixture)
		}
	}
	slices.Sort(want)
	actualPaths, err := evidenceFiles(root)
	if err != nil {
		return fmt.Errorf("read external evidence directory %q: %w", bundle.ExternalRoot(), err)
	}
	if !slices.Equal(actualPaths, want) {
		return fmt.Errorf("external evidence files differ from descriptor: got %v, want %v", actualPaths, want)
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
		paths = append(paths, relative)
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
