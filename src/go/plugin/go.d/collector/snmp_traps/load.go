// SPDX-License-Identifier: GPL-3.0-or-later

package snmp_traps

import (
	"compress/gzip"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"maps"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"sync"

	"gopkg.in/yaml.v2"

	"github.com/klauspost/compress/zstd"
	"github.com/netdata/netdata/go/plugins/pkg/executable"
	"github.com/netdata/netdata/go/plugins/pkg/multipath"
	"github.com/netdata/netdata/go/plugins/pkg/pluginconfig"
)

type profileLoadPaths struct {
	eagerDirs []string
	stockDir  string
	all       multipath.MultiPath
}

var maxProfileFileBytes int64 = 128 * 1024 * 1024

// getProfileDirs returns the multipath of profile directories.
// Order: user dirs first, then stock dir last.
func getProfileDirs() multipath.MultiPath {
	return getProfileLoadPaths().all
}

func getProfileLoadPaths() profileLoadPaths {
	if testDirsOverride != nil {
		return profileLoadPaths{eagerDirs: append([]string(nil), testDirsOverride...), all: multipath.New(testDirsOverride...)}
	}

	if executable.Name == "test" {
		stockDir := trapProfilesDirFromThisFile()
		return profileLoadPaths{stockDir: stockDir, all: multipath.New(stockDir)}
	}

	if dir := filepath.Join(executable.Directory, "../config/go.d/snmp.trap-profiles/default"); isDir(dir) {
		return profileLoadPaths{stockDir: dir, all: multipath.New(dir)}
	}

	var dirs []string
	for _, dir := range pluginconfig.CollectorsUserDirs() {
		dirs = append(dirs, trapProfilesUserDir(dir))
	}
	stockDir := trapProfilesStockDir(pluginconfig.CollectorsStockDir())
	allDirs := append(append([]string(nil), dirs...), stockDir)

	return profileLoadPaths{eagerDirs: dirs, stockDir: stockDir, all: multipath.New(allDirs...)}
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

// loadProfileCache loads user profiles and builds a lazy route table for stock profiles.
func loadProfileCache() (*ProfileIndex, error) {
	paths := getProfileLoadPaths()
	index, seen, err := loadUserProfiles(paths)
	if err != nil {
		return nil, err
	}

	if paths.stockDir != "" {
		store, err := buildStockProfileStore(paths.stockDir, paths.all, seen, index)
		if err != nil {
			if pluginconfig.IsStock(paths.stockDir) || !errors.Is(err, os.ErrNotExist) {
				return nil, fmt.Errorf("failed to load trap profile index from '%s': %w", paths.stockDir, err)
			}
		} else {
			index.stock = store
		}
	}

	if len(index.trapsByOID) == 0 && (index.stock == nil || index.stock.empty()) {
		return nil, fmt.Errorf("no trap profiles found in %v", paths.all)
	}

	return index, nil
}

// loadUserProfileCache reloads only user profiles and carries over the existing
// stock store. This is used for live reloads so Netdata upgrades do not live-load
// changed stock profile metadata without the matching process/job restart.
func loadUserProfileCache(current *ProfileIndex) (*ProfileIndex, error) {
	if current == nil || current.stock == nil {
		return loadProfileCache()
	}

	paths := getProfileLoadPaths()
	index, seen, err := loadUserProfiles(paths)
	if err != nil {
		return nil, err
	}

	index.stock = current.stock.cloneFiltered(seen)
	if err := copyLoadedStockTraps(index, current, seen); err != nil {
		return nil, err
	}

	if len(index.trapsByOID) == 0 && (index.stock == nil || index.stock.empty()) {
		return nil, fmt.Errorf("no trap profiles found in %v", paths.all)
	}

	return index, nil
}

func loadUserProfiles(paths profileLoadPaths) (*ProfileIndex, map[string]bool, error) {
	seen := make(map[string]bool)

	index := &ProfileIndex{
		trapsByOID:      make(map[string]*TrapDef),
		namesByTrapName: make(map[string]*TrapDef),
	}

	for _, dir := range paths.eagerDirs {
		files, err := loadProfilesFromDir(dir, paths.all)
		if err != nil {
			if !errors.Is(err, os.ErrNotExist) {
				return nil, nil, fmt.Errorf("failed to load trap profiles from '%s': %w", dir, err)
			}
			continue
		}

		for _, file := range files {
			if seen[file.name] {
				continue
			}
			seen[file.name] = true
			if err := index.addTraps(file.traps); err != nil {
				return nil, nil, err
			}
		}
	}

	return index, seen, nil
}

func (idx *ProfileIndex) addTraps(traps []*TrapDef) error {
	idx.mu.Lock()
	defer idx.mu.Unlock()
	return idx.addTrapsLocked(traps)
}

func (idx *ProfileIndex) addTrapsLocked(traps []*TrapDef) error {
	if idx.trapsByOID == nil {
		idx.trapsByOID = make(map[string]*TrapDef, len(traps))
	}
	if idx.namesByTrapName == nil {
		idx.namesByTrapName = make(map[string]*TrapDef, len(traps))
	}
	for _, td := range traps {
		if existing, ok := idx.trapsByOID[td.OID]; ok {
			return fmt.Errorf("%s: duplicate trap OID %s (already defined in %s)", td.sourceFile, td.OID, existing.sourceFile)
		}
		if existing, ok := idx.namesByTrapName[td.Name]; ok {
			return fmt.Errorf("%s: duplicate trap name %s (already defined in %s)", td.sourceFile, td.Name, existing.sourceFile)
		}
		idx.trapsByOID[td.OID] = td
		idx.namesByTrapName[td.Name] = td
	}
	return nil
}

type loadedProfileFile struct {
	name  string
	traps []*TrapDef
}

// loadProfilesFromDir walks a directory and loads all supported profile files.
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
		if !isProfileFileName(d.Name()) {
			return nil
		}
		if strings.HasPrefix(d.Name(), "_") {
			return nil
		}

		fileTraps, _, ferr := loadProfile(path, extendsPaths, nil)
		if ferr != nil {
			return fmt.Errorf("invalid profile '%s': %w", path, ferr)
		}

		files = append(files, loadedProfileFile{name: profileLogicalName(d.Name()), traps: fileTraps})
		return nil
	}); err != nil {
		return nil, err
	}

	return files, nil
}

// loadProfile loads a single profile YAML and returns its validated trap definitions.
// Returns traps, loaded file-level varbinds, and error.
func loadProfile(filename string, extendsPaths multipath.MultiPath, stack []string) ([]*TrapDef, map[string]VarbindDef, error) {
	content, err := readProfileFile(filename)
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
			extPath, err := findProfileExtends(extendsPaths, extName)
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

func readProfileFile(filename string) ([]byte, error) {
	return readMaybeCompressedFile(filename)
}

func readMaybeCompressedFile(filename string) ([]byte, error) {
	file, err := os.Open(filename)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	var r io.Reader = file
	var gz *gzip.Reader
	var zr *zstd.Decoder
	switch {
	case strings.HasSuffix(filename, ".zst"):
		zr, err = zstd.NewReader(file)
		if err != nil {
			return nil, err
		}
		defer zr.Close()
		r = zr
	case strings.HasSuffix(filename, ".gz"):
		gz, err = gzip.NewReader(file)
		if err != nil {
			return nil, err
		}
		defer gz.Close()
		r = gz
	}

	lr := io.LimitReader(r, maxProfileFileBytes+1)
	data, err := io.ReadAll(lr)
	if err != nil {
		return nil, err
	}
	if int64(len(data)) > maxProfileFileBytes {
		return nil, fmt.Errorf("profile file %q exceeds maximum decompressed size %d bytes", filename, maxProfileFileBytes)
	}
	return data, nil
}

func findProfileExtends(paths multipath.MultiPath, name string) (string, error) {
	var lastErr error
	for _, candidate := range profilePathCandidates(name) {
		path, err := paths.Find(candidate)
		if err == nil {
			return path, nil
		}
		lastErr = err
	}
	return "", lastErr
}

func isProfileFileName(name string) bool {
	return strings.HasSuffix(name, ".yaml") ||
		strings.HasSuffix(name, ".yml") ||
		strings.HasSuffix(name, ".yaml.zst") ||
		strings.HasSuffix(name, ".yml.zst") ||
		strings.HasSuffix(name, ".yaml.gz") ||
		strings.HasSuffix(name, ".yml.gz")
}

func profileLogicalName(name string) string {
	name = strings.TrimSuffix(name, ".zst")
	return strings.TrimSuffix(name, ".gz")
}

func profilePathCandidates(name string) []string {
	if strings.HasSuffix(name, ".zst") || strings.HasSuffix(name, ".gz") {
		return []string{name}
	}
	return []string{name, name + ".zst", name + ".gz"}
}

type stockProfileStore struct {
	dir              string
	extendsPaths     multipath.MultiPath
	files            map[string]string
	exactRoutes      map[string]string
	enterpriseRoutes map[string]string
	loaded           map[string]bool
	mu               sync.Mutex
}

func (s *stockProfileStore) cloneFiltered(replaced map[string]bool) *stockProfileStore {
	if s == nil {
		return nil
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	clone := &stockProfileStore{
		dir:              s.dir,
		extendsPaths:     s.extendsPaths,
		files:            make(map[string]string, len(s.files)),
		exactRoutes:      make(map[string]string, len(s.exactRoutes)),
		enterpriseRoutes: make(map[string]string, len(s.enterpriseRoutes)),
		loaded:           make(map[string]bool, len(s.loaded)),
	}
	for name, path := range s.files {
		if replaced[name] {
			continue
		}
		clone.files[name] = path
	}
	for oid, name := range s.exactRoutes {
		if replaced[name] {
			continue
		}
		clone.exactRoutes[oid] = name
	}
	for prefix, name := range s.enterpriseRoutes {
		if replaced[name] {
			continue
		}
		clone.enterpriseRoutes[prefix] = name
	}
	for name, loaded := range s.loaded {
		if replaced[name] {
			continue
		}
		clone.loaded[name] = loaded
	}

	return clone
}

func (s *stockProfileStore) loadedProfilePaths(replaced map[string]bool) map[string]string {
	if s == nil {
		return nil
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	paths := make(map[string]string)
	for name, loaded := range s.loaded {
		if !loaded || replaced[name] {
			continue
		}
		if path := s.files[name]; path != "" {
			paths[filepath.Clean(path)] = name
		}
	}
	return paths
}

func copyLoadedStockTraps(dst, current *ProfileIndex, replaced map[string]bool) error {
	if dst == nil || current == nil || current.stock == nil {
		return nil
	}

	loadedPaths := current.stock.loadedProfilePaths(replaced)
	if len(loadedPaths) == 0 {
		return nil
	}

	var traps []*TrapDef
	current.mu.RLock()
	for _, td := range current.trapsByOID {
		if td == nil {
			continue
		}
		if _, ok := loadedPaths[filepath.Clean(td.sourceFile)]; ok {
			traps = append(traps, td)
		}
	}
	current.mu.RUnlock()

	return dst.addTraps(traps)
}

func (s *stockProfileStore) empty() bool {
	return s == nil || (len(s.exactRoutes) == 0 && len(s.enterpriseRoutes) == 0)
}

func buildStockProfileStore(dir string, extendsPaths multipath.MultiPath, replaced map[string]bool, idx *ProfileIndex) (*stockProfileStore, error) {
	if store, ok, err := buildStockProfileStoreFromCatalogue(dir, extendsPaths, replaced, idx); ok || err != nil {
		return store, err
	}
	return buildStockProfileStoreFromProfiles(dir, extendsPaths, replaced, idx)
}

func buildStockProfileStoreFromProfiles(dir string, extendsPaths multipath.MultiPath, replaced map[string]bool, idx *ProfileIndex) (*stockProfileStore, error) {
	files, err := profileFilesInDir(dir)
	if err != nil {
		return nil, err
	}

	store := &stockProfileStore{
		dir:              dir,
		extendsPaths:     extendsPaths,
		files:            make(map[string]string),
		exactRoutes:      make(map[string]string),
		enterpriseRoutes: make(map[string]string),
		loaded:           make(map[string]bool),
	}

	stockOIDs := make(map[string]string)
	stockOIDFiles := make(map[string]string)
	stockNames := make(map[string]string)
	prefixFiles := make(map[string]map[string]bool)

	for _, path := range files {
		name := profileLogicalName(filepath.Base(path))
		if replaced[name] {
			continue
		}
		traps, _, err := loadProfile(path, extendsPaths, nil)
		if err != nil {
			return nil, fmt.Errorf("invalid profile '%s': %w", path, err)
		}
		if len(traps) == 0 {
			continue
		}
		store.files[name] = path

		for _, td := range traps {
			if existing := idx.lookupLoaded(td.OID); existing != nil {
				return nil, fmt.Errorf("%s: duplicate trap OID %s (already defined in %s)", td.sourceFile, td.OID, existing.sourceFile)
			}
			if existing := idx.loadedTrapNameSource(td.Name); existing != "" {
				return nil, fmt.Errorf("%s: duplicate trap name %s (already defined in %s)", td.sourceFile, td.Name, existing)
			}
			if existing := stockOIDs[td.OID]; existing != "" {
				return nil, fmt.Errorf("%s: duplicate trap OID %s (already defined in %s)", td.sourceFile, td.OID, existing)
			}
			if existing := stockNames[td.Name]; existing != "" {
				return nil, fmt.Errorf("%s: duplicate trap name %s (already defined in %s)", td.sourceFile, td.Name, existing)
			}
			stockOIDs[td.OID] = td.sourceFile
			stockOIDFiles[td.OID] = name
			stockNames[td.Name] = td.sourceFile

			if prefix, ok := enterpriseTrapPrefix(td.OID); ok {
				if prefixFiles[prefix] == nil {
					prefixFiles[prefix] = make(map[string]bool)
				}
				prefixFiles[prefix][name] = true
				continue
			}
			store.exactRoutes[td.OID] = name
		}
	}

	for oid := range stockOIDs {
		prefix, ok := enterpriseTrapPrefix(oid)
		if !ok {
			continue
		}
		filesForPrefix := prefixFiles[prefix]
		if len(filesForPrefix) == 1 {
			for file := range filesForPrefix {
				store.enterpriseRoutes[prefix] = file
			}
			continue
		}
		store.exactRoutes[oid] = stockOIDFiles[oid]
	}

	return store, nil
}

type stockProfileCatalogue map[string]stockProfileCatalogueEntry

type stockProfileCatalogueEntry struct {
	File     string   `json:"file"`
	TrapOIDs []string `json:"trap_oids"`
}

func buildStockProfileStoreFromCatalogue(
	dir string,
	extendsPaths multipath.MultiPath,
	replaced map[string]bool,
	idx *ProfileIndex,
) (*stockProfileStore, bool, error) {
	catalogue, err := loadStockProfileCatalogue(dir)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, false, nil
		}
		return nil, true, err
	}

	store := &stockProfileStore{
		dir:              dir,
		extendsPaths:     extendsPaths,
		files:            make(map[string]string),
		exactRoutes:      make(map[string]string),
		enterpriseRoutes: make(map[string]string),
		loaded:           make(map[string]bool),
	}

	stockOIDs := make(map[string]string)
	stockOIDFiles := make(map[string]string)
	prefixFiles := make(map[string]map[string]bool)

	for _, vendor := range sortedCatalogueVendors(catalogue) {
		entry := catalogue[vendor]
		if entry.File == "" {
			return nil, true, fmt.Errorf("stock trap profile catalogue entry %q has empty file", vendor)
		}
		if len(entry.TrapOIDs) == 0 {
			return nil, false, nil
		}

		path, name, err := catalogueProfilePath(dir, entry.File)
		if err != nil {
			return nil, true, fmt.Errorf("stock trap profile catalogue entry %q references file %q: %w", vendor, entry.File, err)
		}
		if replaced[name] {
			continue
		}
		store.files[name] = path

		for _, oid := range entry.TrapOIDs {
			source := path
			if existing := idx.lookupLoaded(oid); existing != nil {
				return nil, true, fmt.Errorf("%s: duplicate trap OID %s (already defined in %s)", source, oid, existing.sourceFile)
			}
			if existing := stockOIDs[oid]; existing != "" {
				return nil, true, fmt.Errorf("%s: duplicate trap OID %s (already defined in %s)", source, oid, existing)
			}
			stockOIDs[oid] = source
			stockOIDFiles[oid] = name

			if prefix, ok := enterpriseTrapPrefix(oid); ok {
				if prefixFiles[prefix] == nil {
					prefixFiles[prefix] = make(map[string]bool)
				}
				prefixFiles[prefix][name] = true
				continue
			}
			store.exactRoutes[oid] = name
		}
	}

	for oid := range stockOIDs {
		prefix, ok := enterpriseTrapPrefix(oid)
		if !ok {
			continue
		}
		filesForPrefix := prefixFiles[prefix]
		if len(filesForPrefix) == 1 {
			for file := range filesForPrefix {
				store.enterpriseRoutes[prefix] = file
			}
			continue
		}
		store.exactRoutes[oid] = stockOIDFiles[oid]
	}

	return store, true, nil
}

func loadStockProfileCatalogue(stockDir string) (stockProfileCatalogue, error) {
	if stockDir == "" {
		return nil, os.ErrNotExist
	}
	data, err := readStockProfileCatalogueFile(filepath.Dir(stockDir))
	if err != nil {
		return nil, err
	}
	var catalogue stockProfileCatalogue
	if err := json.Unmarshal(data, &catalogue); err != nil {
		return nil, fmt.Errorf("invalid stock trap profile catalogue: %w", err)
	}
	if len(catalogue) == 0 {
		return nil, fmt.Errorf("stock trap profile catalogue is empty")
	}
	return catalogue, nil
}

func readStockProfileCatalogueFile(dir string) ([]byte, error) {
	var lastErr error
	for _, name := range []string{"catalogue.json", "catalogue.json.zst", "catalogue.json.gz"} {
		data, err := readMaybeCompressedFile(filepath.Join(dir, name))
		if err == nil {
			return data, nil
		}
		if !errors.Is(err, os.ErrNotExist) {
			return nil, err
		}
		lastErr = err
	}
	if lastErr != nil {
		return nil, lastErr
	}
	return nil, os.ErrNotExist
}

func sortedCatalogueVendors(catalogue stockProfileCatalogue) []string {
	vendors := make([]string, 0, len(catalogue))
	for vendor := range catalogue {
		vendors = append(vendors, vendor)
	}
	slices.Sort(vendors)
	return vendors
}

func catalogueProfilePath(dir, name string) (string, string, error) {
	if filepath.Base(name) != name || strings.ContainsRune(name, '\\') {
		return "", "", fmt.Errorf("must be a profile filename, not a path")
	}
	if !profilePathIsSupported(name) {
		return "", "", fmt.Errorf("unsupported profile filename")
	}

	for _, candidate := range profilePathCandidates(name) {
		if !isProfileFileName(candidate) {
			continue
		}
		path := filepath.Join(dir, candidate)
		fi, err := os.Stat(path)
		if err != nil {
			if errors.Is(err, os.ErrNotExist) {
				continue
			}
			return "", "", err
		}
		if fi.IsDir() {
			return "", "", fmt.Errorf("%s is a directory", path)
		}
		return path, profileLogicalName(candidate), nil
	}
	return "", "", os.ErrNotExist
}

func profilePathIsSupported(name string) bool {
	for _, candidate := range profilePathCandidates(name) {
		if isProfileFileName(candidate) {
			return true
		}
	}
	return false
}

func (idx *ProfileIndex) loadedTrapNameSource(name string) string {
	idx.mu.RLock()
	defer idx.mu.RUnlock()
	if idx.namesByTrapName == nil {
		return ""
	}
	if td := idx.namesByTrapName[name]; td != nil {
		return td.sourceFile
	}
	return ""
}

func profileFilesInDir(dirpath string) ([]string, error) {
	if dirpath == "" {
		return nil, os.ErrNotExist
	}
	var files []string
	err := filepath.WalkDir(dirpath, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() || !isProfileFileName(d.Name()) || strings.HasPrefix(d.Name(), "_") {
			return nil
		}
		files = append(files, path)
		return nil
	})
	if err != nil {
		return nil, err
	}
	slices.Sort(files)
	return files, nil
}

func enterpriseTrapPrefix(oid string) (string, bool) {
	const prefix = "1.3.6.1.4.1."
	if !strings.HasPrefix(oid, prefix) {
		return "", false
	}
	rest := strings.TrimPrefix(oid, prefix)
	nextDot := strings.IndexByte(rest, '.')
	if nextDot <= 0 {
		return "", false
	}
	pen := rest[:nextDot]
	for i := 0; i < len(pen); i++ {
		if pen[i] < '0' || pen[i] > '9' {
			return "", false
		}
	}
	return prefix + pen, true
}

func (idx *ProfileIndex) loadStockForOID(oid string) error {
	if idx == nil || idx.stock == nil {
		return nil
	}
	return idx.stock.loadForOID(idx, oid)
}

func (s *stockProfileStore) loadForOID(idx *ProfileIndex, oid string) error {
	name := s.route(oid)
	if name == "" {
		return nil
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	if s.loaded[name] {
		return nil
	}
	path := s.files[name]
	if path == "" {
		return fmt.Errorf("stock trap profile %q not found in %s", name, s.dir)
	}
	traps, _, err := loadProfile(path, s.extendsPaths, nil)
	if err != nil {
		return fmt.Errorf("failed to lazy-load stock trap profile %q: %w", path, err)
	}
	if err := idx.addTraps(traps); err != nil {
		return err
	}
	s.loaded[name] = true
	return nil
}

func (s *stockProfileStore) route(oid string) string {
	for _, candidate := range []string{oid, alternateTrapOID(oid)} {
		if file := s.exactRoutes[candidate]; file != "" {
			return file
		}
		if prefix, ok := enterpriseTrapPrefix(candidate); ok {
			if file := s.enterpriseRoutes[prefix]; file != "" {
				return file
			}
		}
	}
	return ""
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
	maps.Copy(dst, src)
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
	maps.Copy(dst, src)
	return dst
}

func mergeStringMaps(base, override map[string]string) map[string]string {
	dst := cloneStringMap(base)
	maps.Copy(dst, override)
	return dst
}

func stacksContains(stack []string, name string) bool {
	return slices.Contains(stack, name)
}

func unmarshalProfileYAML(content []byte, def *ProfileDefinition) (err error) {
	defer func() {
		if v := recover(); v != nil {
			err = fmt.Errorf("panic parsing profile YAML: %v", v)
		}
	}()
	return yaml.Unmarshal(content, def)
}
