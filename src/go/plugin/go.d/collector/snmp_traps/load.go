// SPDX-License-Identifier: GPL-3.0-or-later

package snmp_traps

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"gopkg.in/yaml.v2"

	"github.com/netdata/netdata/go/plugins/pkg/executable"
	"github.com/netdata/netdata/go/plugins/pkg/multipath"
	"github.com/netdata/netdata/go/plugins/pkg/pluginconfig"
)

// getProfileDirs returns the multipath of profile directories.
// Order: user dirs first, then stock dir last.
func getProfileDirs() multipath.MultiPath {
	if testDirsOverride != nil {
		return multipath.New(testDirsOverride...)
	}

	if executable.Name == "test" {
		return multipath.New(trapProfilesDirFromThisFile())
	}

	if dir := filepath.Join(executable.Directory, "../config/go.d/snmp.trap-profiles/default"); isDir(dir) {
		return multipath.New(dir)
	}

	var dirs []string
	for _, dir := range pluginconfig.CollectorsUserDirs() {
		dirs = append(dirs, trapProfilesUserDir(dir))
	}
	dirs = append(dirs, trapProfilesStockDir(pluginconfig.CollectorsStockDir()))

	return multipath.New(dirs...)
}

// testDirsOverride is set by tests to redirect profile loading to temp dirs.
var testDirsOverride []string

func trapProfilesUserDir(collectorsDir string) string {
	return filepath.Join(collectorsDir, "snmp.trap-profiles")
}

func trapProfilesStockDir(collectorsDir string) string {
	return filepath.Join(collectorsDir, "snmp.trap-profiles", "default")
}

// trapProfilesDirFromThisFile resolves the stock profile directory relative to
// this source file, used in test mode.
func trapProfilesDirFromThisFile() string {
	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		return ""
	}
	base := filepath.Dir(thisFile)
	stockDir := filepath.Join(base, "..", "..", "config", "go.d", "snmp.trap-profiles", "default")
	if isDir(stockDir) {
		abs, _ := filepath.Abs(stockDir)
		return abs
	}
	return ""
}

func isDir(path string) bool {
	fi, err := os.Stat(path)
	if err != nil {
		return false
	}
	return fi.Mode().IsDir()
}

// loadProfileCache loads all profile YAMLs into a ProfileIndex.
func loadProfileCache() (*ProfileIndex, error) {
	profileDirs := getProfileDirs()
	seen := make(map[string]bool)

	var allTraps []*TrapDef

	for _, dir := range profileDirs {
		files, err := loadProfilesFromDir(dir, profileDirs)
		if err != nil {
			if pluginconfig.IsStock(dir) || !errors.Is(err, os.ErrNotExist) {
				return nil, fmt.Errorf("failed to load trap profiles from '%s': %w", dir, err)
			}
			continue
		}

		for _, file := range files {
			if seen[file.name] {
				continue
			}
			seen[file.name] = true
			allTraps = append(allTraps, file.traps...)
		}
	}

	if len(allTraps) == 0 {
		return nil, fmt.Errorf("no trap profiles found in %v", profileDirs)
	}

	index := &ProfileIndex{
		trapsByOID: make(map[string]*TrapDef, len(allTraps)),
	}
	namesByTrapName := make(map[string]*TrapDef, len(allTraps))

	for _, td := range allTraps {
		if existing, ok := index.trapsByOID[td.OID]; ok {
			return nil, fmt.Errorf("%s: duplicate trap OID %s (already defined in %s)", td.sourceFile, td.OID, existing.sourceFile)
		}
		if existing, ok := namesByTrapName[td.Name]; ok {
			return nil, fmt.Errorf("%s: duplicate trap name %s (already defined in %s)", td.sourceFile, td.Name, existing.sourceFile)
		}
		index.trapsByOID[td.OID] = td
		namesByTrapName[td.Name] = td
	}

	return index, nil
}

type loadedProfileFile struct {
	name  string
	traps []*TrapDef
}

// loadProfilesFromDir walks a directory and loads all .yaml/yml profile files.
func loadProfilesFromDir(dirpath string, extendsPaths multipath.MultiPath) ([]loadedProfileFile, error) {
	var files []loadedProfileFile
	if dirpath == "" {
		return nil, os.ErrNotExist
	}

	if err := filepath.WalkDir(dirpath, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		if !strings.HasSuffix(d.Name(), ".yaml") && !strings.HasSuffix(d.Name(), ".yml") {
			return nil
		}
		if strings.HasPrefix(d.Name(), "_") {
			return nil
		}

		fileTraps, _, ferr := loadProfile(path, extendsPaths, nil)
		if ferr != nil {
			return fmt.Errorf("invalid profile '%s': %w", path, ferr)
		}

		files = append(files, loadedProfileFile{name: d.Name(), traps: fileTraps})
		return nil
	}); err != nil {
		return nil, err
	}

	return files, nil
}

// loadProfile loads a single profile YAML and returns its validated trap definitions.
// Returns traps, loaded file-level varbinds, and error.
func loadProfile(filename string, extendsPaths multipath.MultiPath, stack []string) ([]*TrapDef, map[string]VarbindDef, error) {
	content, err := os.ReadFile(filename)
	if err != nil {
		return nil, nil, err
	}

	var def ProfileDefinition
	if err := unmarshalProfileYAML(content, &def); err != nil {
		return nil, nil, err
	}

	// resolve extends chain
	var allTraps []*TrapDef
	fileVarbinds := cloneVarbindMap(def.Varbinds)
	currentVarbinds := make(map[string]bool, len(def.Varbinds))
	for name := range def.Varbinds {
		currentVarbinds[name] = true
	}

	currentTrapOIDs := make(map[string]bool, len(def.Traps))
	for _, td := range def.Traps {
		if td.OID == "" {
			continue
		}
		if currentTrapOIDs[td.OID] {
			return nil, nil, fmt.Errorf("%s: duplicate trap OID %s in profile", filename, td.OID)
		}
		currentTrapOIDs[td.OID] = true
	}
	baseByOID := make(map[string]*TrapDef)
	allTrapPos := make(map[string]int)

	if len(def.Extends) > 0 {
		for _, extName := range def.Extends {
			if err := validateExtendsName(extName); err != nil {
				return nil, nil, fmt.Errorf("%s: invalid extends entry %q: %w", filename, extName, err)
			}
			if stacksContains(stack, extName) {
				return nil, nil, fmt.Errorf("%s: circular extends detected: %q", filename, extName)
			}
			extPath, err := extendsPaths.Find(extName)
			if err != nil {
				return nil, nil, fmt.Errorf("%s: cannot find extension %q: %v", filename, extName, err)
			}
			baseTraps, baseVarbinds, err := loadProfile(extPath, extendsPaths, append(stack, extName))
			if err != nil {
				return nil, nil, err
			}
			for _, bt := range baseTraps {
				baseByOID[bt.OID] = bt
				if currentTrapOIDs[bt.OID] {
					continue
				}
				if pos, ok := allTrapPos[bt.OID]; ok {
					allTraps[pos] = bt
				} else {
					allTrapPos[bt.OID] = len(allTraps)
					allTraps = append(allTraps, bt)
				}
			}
			mergeBaseVarbinds(fileVarbinds, baseVarbinds, currentVarbinds)
		}
	}

	absFile, _ := filepath.Abs(filename)
	if err := validateFileVarbinds(fileVarbinds, absFile); err != nil {
		return nil, nil, err
	}

	for i := range def.Traps {
		td := &def.Traps[i]
		td.sourceFile = absFile
		if base := baseByOID[td.OID]; base != nil {
			mergeTrapDef(td, base)
		}

		if err := validateTrapDef(td, fileVarbinds); err != nil {
			return nil, nil, err
		}

		td.sharedVarbinds = buildSharedVarbinds(td, fileVarbinds)
	}

	for i := range def.Traps {
		allTraps = append(allTraps, &def.Traps[i])
	}

	return allTraps, fileVarbinds, nil
}

func validateExtendsName(name string) error {
	if name == "" {
		return fmt.Errorf("empty filename")
	}
	if filepath.Base(name) != name || strings.ContainsRune(name, '\\') {
		return fmt.Errorf("must be a profile filename, not a path")
	}
	if name == "." || name == ".." {
		return fmt.Errorf("must be a profile filename, not a path")
	}
	if !strings.HasSuffix(name, ".yaml") && !strings.HasSuffix(name, ".yml") {
		return fmt.Errorf("must end with .yaml or .yml")
	}
	return nil
}

func mergeTrapDef(target *TrapDef, base *TrapDef) {
	if target.Name == "" {
		target.Name = base.Name
	}
	if target.Category == "" {
		target.Category = base.Category
	}
	if target.Severity == "" {
		target.Severity = base.Severity
	}
	if target.Description == "" {
		target.Description = base.Description
	}
	if target.Status == "" {
		target.Status = base.Status
	}
	if target.VarbindRefs == nil {
		target.VarbindRefs = cloneAnySlice(base.VarbindRefs)
	}
	if target.Labels == nil {
		target.Labels = cloneStringMap(base.Labels)
	} else {
		target.Labels = mergeStringMaps(base.Labels, target.Labels)
	}
	if target.DedupKeyVarbinds == nil {
		target.DedupKeyVarbinds = append([]string(nil), base.DedupKeyVarbinds...)
	}
}

func mergeBaseVarbinds(target, base map[string]VarbindDef, protected map[string]bool) {
	for name, vb := range base {
		if !protected[name] {
			target[name] = vb
		}
	}
}

func cloneVarbindMap(src map[string]VarbindDef) map[string]VarbindDef {
	dst := make(map[string]VarbindDef, len(src))
	for k, v := range src {
		dst[k] = v
	}
	return dst
}

func cloneAnySlice(src []any) []any {
	if src == nil {
		return nil
	}
	return append([]any(nil), src...)
}

func cloneStringMap(src map[string]string) map[string]string {
	if src == nil {
		return nil
	}
	dst := make(map[string]string, len(src))
	for k, v := range src {
		dst[k] = v
	}
	return dst
}

func mergeStringMaps(base, override map[string]string) map[string]string {
	dst := cloneStringMap(base)
	for k, v := range override {
		dst[k] = v
	}
	return dst
}

func stacksContains(stack []string, name string) bool {
	for _, s := range stack {
		if s == name {
			return true
		}
	}
	return false
}

func unmarshalProfileYAML(content []byte, def *ProfileDefinition) (err error) {
	defer func() {
		if v := recover(); v != nil {
			err = fmt.Errorf("panic parsing profile YAML: %v", v)
		}
	}()
	return yaml.Unmarshal(content, def)
}
