// SPDX-License-Identifier: GPL-3.0-or-later

package jobmgr

import (
	"fmt"
	"regexp"
	"sort"
	"strings"
	"sync"

	"github.com/netdata/netdata/go/plugins/plugin/agent/secrets/secretstore"
	"github.com/netdata/netdata/go/plugins/plugin/framework/confgroup"
	"github.com/netdata/netdata/go/plugins/plugin/framework/dyncfg"
)

var (
	reSecretRef      = regexp.MustCompile(`\$\{([^}]+)\}`)
	reUpperShorthand = regexp.MustCompile(`^[A-Z_][A-Z0-9_]*$`)
)

type secretStoreDeps struct {
	mu sync.RWMutex

	jobs map[string]*secretStoreJobState

	// storeKey -> internal job key -> ref
	exposed map[string]map[string]secretstore.JobRef
	running map[string]map[string]secretstore.JobRef
}

type secretStoreJobState struct {
	display string
	stores  map[string]struct{}
	running bool
}

func newSecretStoreDeps() *secretStoreDeps {
	return &secretStoreDeps{
		jobs:    make(map[string]*secretStoreJobState),
		exposed: make(map[string]map[string]secretstore.JobRef),
		running: make(map[string]map[string]secretstore.JobRef),
	}
}

func (d *secretStoreDeps) SetActiveJobStores(internalKey, display string, storeKeys []string) {
	internalKey = strings.TrimSpace(internalKey)
	if internalKey == "" {
		return
	}
	if display == "" {
		display = internalKey
	}

	normalized := normalizeStoreKeys(storeKeys)

	d.mu.Lock()
	defer d.mu.Unlock()

	state, ok := d.jobs[internalKey]
	if !ok {
		state = &secretStoreJobState{}
		d.jobs[internalKey] = state
	}

	for storeKey := range state.stores {
		d.removeRefLocked(d.exposed, storeKey, internalKey)
		if state.running {
			d.removeRefLocked(d.running, storeKey, internalKey)
		}
	}

	state.display = display
	state.stores = make(map[string]struct{}, len(normalized))
	for _, storeKey := range normalized {
		state.stores[storeKey] = struct{}{}
		ref := secretstore.JobRef{ID: internalKey, Display: display}
		d.addRefLocked(d.exposed, storeKey, ref)
		if state.running {
			d.addRefLocked(d.running, storeKey, ref)
		}
	}
}

func (d *secretStoreDeps) RemoveActiveJob(internalKey string) {
	internalKey = strings.TrimSpace(internalKey)
	if internalKey == "" {
		return
	}

	d.mu.Lock()
	defer d.mu.Unlock()

	state, ok := d.jobs[internalKey]
	if !ok {
		return
	}

	for storeKey := range state.stores {
		d.removeRefLocked(d.exposed, storeKey, internalKey)
		d.removeRefLocked(d.running, storeKey, internalKey)
	}
	delete(d.jobs, internalKey)
}

func (d *secretStoreDeps) setRunning(internalKey string, running bool) {
	internalKey = strings.TrimSpace(internalKey)
	if internalKey == "" {
		return
	}

	d.mu.Lock()
	defer d.mu.Unlock()

	state, ok := d.jobs[internalKey]
	if !ok {
		if !running {
			return
		}
		state = &secretStoreJobState{display: internalKey, stores: map[string]struct{}{}}
		d.jobs[internalKey] = state
	}

	if state.running == running {
		return
	}

	state.running = running
	for storeKey := range state.stores {
		ref := secretstore.JobRef{ID: internalKey, Display: state.display}
		if running {
			d.addRefLocked(d.running, storeKey, ref)
		} else {
			d.removeRefLocked(d.running, storeKey, internalKey)
		}
	}
}

func (d *secretStoreDeps) Impacted(storeKey string) (exposed []secretstore.JobRef, running []secretstore.JobRef) {
	storeKey = strings.TrimSpace(storeKey)
	if storeKey == "" {
		return nil, nil
	}

	d.mu.RLock()
	defer d.mu.RUnlock()

	exposed = collectSortedRefs(d.exposed[storeKey])
	running = collectSortedRefs(d.running[storeKey])
	return exposed, running
}

func (d *secretStoreDeps) addRefLocked(index map[string]map[string]secretstore.JobRef, storeKey string, ref secretstore.JobRef) {
	jobs, ok := index[storeKey]
	if !ok {
		jobs = make(map[string]secretstore.JobRef)
		index[storeKey] = jobs
	}
	jobs[ref.ID] = ref
}

func (d *secretStoreDeps) removeRefLocked(index map[string]map[string]secretstore.JobRef, storeKey, internalKey string) {
	jobs, ok := index[storeKey]
	if !ok {
		return
	}
	delete(jobs, internalKey)
	if len(jobs) == 0 {
		delete(index, storeKey)
	}
}

func collectSortedRefs(m map[string]secretstore.JobRef) []secretstore.JobRef {
	if len(m) == 0 {
		return nil
	}
	refs := make([]secretstore.JobRef, 0, len(m))
	for _, ref := range m {
		refs = append(refs, ref)
	}
	sort.Slice(refs, func(i, j int) bool {
		if refs[i].ID == refs[j].ID {
			return refs[i].Display < refs[j].Display
		}
		return refs[i].ID < refs[j].ID
	})
	return refs
}

func normalizeStoreKeys(storeKeys []string) []string {
	if len(storeKeys) == 0 {
		return nil
	}
	seen := make(map[string]struct{}, len(storeKeys))
	for _, key := range storeKeys {
		kind, name, err := secretstore.ParseStoreKey(key)
		if err != nil {
			continue
		}
		seen[secretstore.StoreKey(kind, name)] = struct{}{}
	}
	if len(seen) == 0 {
		return nil
	}
	out := make([]string, 0, len(seen))
	for key := range seen {
		out = append(out, key)
	}
	sort.Strings(out)
	return out
}

func extractSecretStoreKeys(cfg confgroup.Config) []string {
	seen := make(map[string]struct{})
	extractSecretStoreKeysFromValue(cfg, seen)
	if len(seen) == 0 {
		return nil
	}
	keys := make([]string, 0, len(seen))
	for key := range seen {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

func extractSecretStoreKeysFromValue(v any, seen map[string]struct{}) {
	switch value := v.(type) {
	case string:
		extractSecretStoreKeysFromString(value, seen)
	case confgroup.Config:
		for k, entry := range value {
			if isSecretStoreInternalKey(k) {
				continue
			}
			extractSecretStoreKeysFromValue(entry, seen)
		}
	case map[string]any:
		for k, entry := range value {
			if isSecretStoreInternalKey(k) {
				continue
			}
			extractSecretStoreKeysFromValue(entry, seen)
		}
	case map[any]any:
		for rawKey, entry := range value {
			if key, ok := rawKey.(string); ok && isSecretStoreInternalKey(key) {
				continue
			}
			extractSecretStoreKeysFromValue(entry, seen)
		}
	case []any:
		for _, entry := range value {
			extractSecretStoreKeysFromValue(entry, seen)
		}
	}
}

func extractSecretStoreKeysFromString(value string, seen map[string]struct{}) {
	if !strings.Contains(value, "${") {
		return
	}
	matches := reSecretRef.FindAllStringSubmatch(value, -1)
	for _, match := range matches {
		if len(match) < 2 {
			continue
		}
		inner := match[1]
		scheme, rest, hasScheme := strings.Cut(inner, ":")
		if !hasScheme {
			if reUpperShorthand.MatchString(inner) {
				continue
			}
			continue
		}
		if scheme != "store" {
			continue
		}
		kindPart, tail, ok := strings.Cut(rest, ":")
		if !ok {
			continue
		}
		namePart, _, ok := strings.Cut(tail, ":")
		if !ok {
			continue
		}
		kind := secretstore.StoreKind(strings.TrimSpace(kindPart))
		name := strings.TrimSpace(namePart)
		if !kind.IsValid() {
			continue
		}
		if err := dyncfg.JobNameRuleAllowDots(name); err != nil {
			continue
		}
		seen[secretstore.StoreKey(kind, name)] = struct{}{}
	}
}

func isSecretStoreInternalKey(k string) bool {
	return strings.HasPrefix(k, "__") && strings.HasSuffix(k, "__")
}

func secretStoreDisplay(cfg confgroup.Config) string {
	module := strings.TrimSpace(cfg.Module())
	job := strings.TrimSpace(cfg.Name())
	switch {
	case module != "" && job != "":
		return fmt.Sprintf("%s:%s", module, job)
	case cfg.FullName() != "":
		return cfg.FullName()
	default:
		return job
	}
}

func (m *Manager) syncSecretStoreDepsForConfig(cfg confgroup.Config) {
	if m.secretStoreDeps == nil {
		return
	}
	m.secretStoreDeps.SetActiveJobStores(cfg.FullName(), secretStoreDisplay(cfg), extractSecretStoreKeys(cfg))
}

func (m *Manager) syncSecretStoreDepsByFunction(fn dyncfg.Function) {
	if m.secretStoreDeps == nil || m.collectorCallbacks == nil {
		return
	}
	key, _, ok := m.collectorCallbacks.ExtractKey(fn)
	if !ok {
		return
	}
	entry, ok := m.collectorExposed.LookupByKey(key)
	if !ok {
		m.secretStoreDeps.RemoveActiveJob(key)
		return
	}
	m.syncSecretStoreDepsForConfig(entry.Cfg)
}
