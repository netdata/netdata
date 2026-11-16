// SPDX-License-Identifier: GPL-3.0-or-later

package discovery

import (
	"context"
	"errors"
	"fmt"
	"maps"
	"reflect"
	"slices"
	"strings"
	"sync"

	"github.com/netdata/netdata/go/plugins/plugin/agent/jobmgr"
	"github.com/netdata/netdata/go/plugins/plugin/agent/jobmgr/lifecycle"
	"github.com/netdata/netdata/go/plugins/plugin/framework/confgroup"
)

type InventoryConfig struct {
	Generation  uint64
	RunJob      []string
	AutoEnable  bool
	Plan        PlanInventory
	Commands    PreparedCommandPort
	Diagnostics jobmgr.DiagnosticObserver
}

// InventoryIndex owns source observations and their ordered delivery for one run.
// Selection, activation and candidate failures belong to the job controller.
// Apply is called serially by the discovery pipeline.
type InventoryIndex struct {
	generation  uint64
	runJob      map[string]struct{}
	autoEnable  bool
	plan        PlanInventory
	commands    PreparedCommandPort
	diagnostics jobmgr.DiagnosticObserver

	sources          map[string]map[uint64]SourceCandidate
	sourceNames      map[string]map[string]uint64
	presence         map[string]map[string]uint64
	candidates       map[string]map[inventoryCandidateKey]SourceCandidate
	versions         map[string]uint64
	pending          map[string][]SourceInventoryBatch
	incarnation      uint64
	deliverySequence uint64
}

type inventoryCandidateKey struct {
	source string
	hash   uint64
}

func NewInventoryIndex(config InventoryConfig) (*InventoryIndex, error) {
	if config.Generation == 0 || config.Plan == nil || config.Commands == nil {
		return nil, errors.New("jobmgr discovery: invalid inventory index")
	}
	runJob := make(map[string]struct{}, len(config.RunJob))
	for _, name := range config.RunJob {
		if name == "" {
			return nil, errors.New("jobmgr discovery: invalid run-job name")
		}
		runJob[name] = struct{}{}
	}
	return &InventoryIndex{
		generation:  config.Generation,
		runJob:      runJob,
		autoEnable:  config.AutoEnable,
		plan:        config.Plan,
		commands:    config.Commands,
		diagnostics: config.Diagnostics,
		sources:     make(map[string]map[uint64]SourceCandidate),
		sourceNames: make(map[string]map[string]uint64),
		presence:    make(map[string]map[string]uint64),
		candidates:  make(map[string]map[inventoryCandidateKey]SourceCandidate),
		versions:    make(map[string]uint64),
		pending:     make(map[string][]SourceInventoryBatch),
	}, nil
}

func (ii *InventoryIndex) Apply(ctx context.Context, batch []*confgroup.Group) error {
	if ii == nil || ctx == nil || batch == nil {
		return errors.New("jobmgr discovery: invalid inventory batch")
	}
	for _, group := range batch {
		if group == nil || group.Source == "" {
			return errors.New("jobmgr discovery: invalid group")
		}
	}
	observations := make(map[string][]SourceInventory)
	for _, group := range batch {
		if err := ii.applyGroup(group, observations); err != nil {
			return err
		}
	}
	for name, inventory := range observations {
		ii.pending[name] = append(ii.pending[name], SourceInventoryBatch{
			FullName:     name,
			Observations: inventory,
		})
	}
	return ii.deliver(ctx)
}

func (ii *InventoryIndex) applyGroup(group *confgroup.Group, observations map[string][]SourceInventory) error {
	previous := ii.sources[group.Source]
	next := make(map[uint64]SourceCandidate, len(group.Configs))
	rejected := make(map[string]error)
	validNames := make(map[string]struct{})
	observedNames := make(map[string]struct{})
	for _, config := range group.Configs {
		if config == nil || !ii.allowed(config) {
			continue
		}
		observedNames[config.FullName()] = struct{}{}
		cloned, err := config.Clone()
		if err != nil {
			name := config.FullName()
			cloneErr := fmt.Errorf("jobmgr discovery: clone config %q: %w", name, err)
			if name != "" {
				rejected[name] = cloneErr
			} else {
				ii.observe(group.Source, "invalid identity", cloneErr)
			}
			continue
		}
		hash := cloned.Hash()
		candidate := previous[hash]
		if !reflect.DeepEqual(candidate.Config, cloned) {
			if ii.incarnation == ^uint64(0) {
				return errors.New("jobmgr discovery: candidate incarnation wrapped")
			}
			ii.incarnation++
			candidate = SourceCandidate{
				Config:      cloned,
				Source:      group.Source,
				Incarnation: ii.incarnation,
			}
		}
		next[hash] = candidate
		validNames[cloned.FullName()] = struct{}{}
	}
	for name, err := range rejected {
		ii.observe(name, "invalid source config", err)
		if _, valid := validNames[name]; valid {
			continue
		}
		// A malformed replacement is not an authoritative source removal.
		for hash, candidate := range previous {
			if candidate.Config.FullName() == name {
				next[hash] = candidate
			}
		}
	}

	affected := make(map[string]struct{})
	previousNames := ii.sourceNames[group.Source]
	nextNames := make(map[string]uint64, len(observedNames))
	for name := range observedNames {
		lifetime := previousNames[name]
		if lifetime == 0 {
			if ii.incarnation == ^uint64(0) {
				return errors.New("jobmgr discovery: source incarnation wrapped")
			}
			ii.incarnation++
			lifetime = ii.incarnation
			affected[name] = struct{}{}
			if ii.presence[name] == nil {
				ii.presence[name] = make(map[string]uint64)
			}
			ii.presence[name][group.Source] = lifetime
		}
		nextNames[name] = lifetime
	}
	for name := range previousNames {
		if _, exists := nextNames[name]; !exists {
			affected[name] = struct{}{}
			delete(ii.presence[name], group.Source)
			if len(ii.presence[name]) == 0 {
				delete(ii.presence, name)
			}
		}
	}
	if len(nextNames) == 0 {
		delete(ii.sourceNames, group.Source)
	} else {
		ii.sourceNames[group.Source] = nextNames
	}
	for hash, candidate := range previous {
		if replacement, ok := next[hash]; ok && replacement.Incarnation == candidate.Incarnation {
			continue
		}
		name := candidate.Config.FullName()
		affected[name] = struct{}{}
		delete(ii.candidates[name], inventoryCandidateKey{
			source: group.Source,
			hash:   hash,
		})
		if len(ii.candidates[name]) == 0 {
			delete(ii.candidates, name)
		}
	}
	for hash, candidate := range next {
		if old, ok := previous[hash]; ok && old.Incarnation == candidate.Incarnation {
			continue
		}
		name := candidate.Config.FullName()
		affected[name] = struct{}{}
		if ii.candidates[name] == nil {
			ii.candidates[name] = make(map[inventoryCandidateKey]SourceCandidate)
		}
		ii.candidates[name][inventoryCandidateKey{
			source: group.Source,
			hash:   hash,
		}] = candidate
	}
	if len(next) == 0 {
		delete(ii.sources, group.Source)
	} else {
		ii.sources[group.Source] = next
	}
	for _, name := range slices.Sorted(maps.Keys(affected)) {
		if ii.versions[name] == ^uint64(0) {
			return errors.New("jobmgr discovery: inventory version wrapped")
		}
		ii.versions[name]++
		inventory := SourceInventory{
			FullName:   name,
			Version:    ii.versions[name],
			AutoEnable: ii.autoEnable,
			Candidates: slices.Collect(maps.Values(ii.candidates[name])),
			Sources:    maps.Clone(ii.presence[name]),
		}
		// This is a stable wire order, not runtime source arbitration.
		slices.SortFunc(inventory.Candidates, func(a, b SourceCandidate) int {
			if a.Source != b.Source {
				return strings.Compare(a.Source, b.Source)
			}
			return strings.Compare(a.Config.UID(), b.Config.UID())
		})
		observations[name] = append(observations[name], inventory)
	}
	return nil
}

func (ii *InventoryIndex) allowed(config confgroup.Config) bool {
	if len(ii.runJob) == 0 {
		return true
	}
	_, ok := ii.runJob[config.Name()]
	return ok
}

type inventoryDelivery struct {
	request jobmgr.Request
	plan    jobmgr.WorkPlan
	err     error
}

func (ii *InventoryIndex) deliver(ctx context.Context) error {
	names := slices.Sorted(maps.Keys(ii.pending))
	deliveries := make([][]inventoryDelivery, len(names))
	for i, name := range names {
		for _, inventory := range ii.pending[name] {
			// Each planner invocation receives its own payload. Failed delivery
			// must replay every original immutable observation and version.
			payload, err := cloneInventoryBatch(inventory)
			var plan jobmgr.WorkPlan
			if err == nil {
				plan, err = ii.plan(payload)
			}

			var request jobmgr.Request
			if err == nil {
				if ii.deliverySequence == ^uint64(0) {
					err = errors.New("jobmgr discovery: delivery sequence wrapped")
				} else {
					ii.deliverySequence++
					// Transport identity is independent of untrusted config metadata.
					request = jobmgr.Request{
						UID:     fmt.Sprintf("jobmgr-discovery-%d-%d", ii.generation, ii.deliverySequence),
						LaneKey: name,
						Source:  lifecycle.SourceJobManager,
						Route:   "internal/discovery/inventory",
					}
				}
			}
			deliveries[i] = append(deliveries[i], inventoryDelivery{
				request: request,
				plan:    plan,
				err:     err,
			})
			if err != nil {
				break
			}
		}
	}
	completed := make([]int, len(names))
	failures := make([]error, len(names))
	var wg sync.WaitGroup
	for i := range names {
		wg.Go(func() {
			for _, delivery := range deliveries[i] {
				err := delivery.err
				if err == nil {
					err = ii.commands.SubmitPreparedAndWait(ctx, delivery.request, delivery.plan)
				}
				if err != nil {
					failures[i] = err
					return
				}
				completed[i]++
			}
		})
	}
	wg.Wait()
	var resultErr error
	for i, name := range names {
		remaining := ii.pending[name][completed[i]:]
		if len(remaining) == 0 {
			delete(ii.pending, name)
		} else {
			ii.pending[name] = slices.Clone(remaining)
		}
		if err := failures[i]; err != nil {
			ii.observe(name, "inventory delivery failed", err)
			// Retirement cuts attempts before cancelling discovery. Keep the
			// observation pending while the pipeline drains during shutdown.
			if jobmgr.ContainsOnlyErrorLeaves(err, jobmgr.ErrProcessAttemptRetired, jobmgr.ErrProcessAttemptStopped) {
				continue
			}
			resultErr = errors.Join(resultErr, err)
		}
	}
	return resultErr
}

func cloneInventoryBatch(batch SourceInventoryBatch) (SourceInventoryBatch, error) {
	batch.Observations = slices.Clone(batch.Observations)
	for i := range batch.Observations {
		cloned, err := cloneInventory(batch.Observations[i])
		if err != nil {
			return SourceInventoryBatch{}, err
		}
		batch.Observations[i] = cloned
	}
	return batch, nil
}

func cloneInventory(inventory SourceInventory) (SourceInventory, error) {
	inventory.Sources = maps.Clone(inventory.Sources)
	inventory.Candidates = slices.Clone(inventory.Candidates)
	for i := range inventory.Candidates {
		cloned, err := inventory.Candidates[i].Config.Clone()
		if err != nil {
			return SourceInventory{}, err
		}
		inventory.Candidates[i].Config = cloned
	}
	return inventory, nil
}

func (ii *InventoryIndex) observe(name, state string, err error) {
	jobmgr.ObserveDiagnostic(ii.diagnostics, jobmgr.DiagnosticEvent{
		Level:      jobmgr.DiagnosticWarning,
		Name:       "discovered collector inventory",
		Resource:   name,
		State:      state,
		Generation: ii.generation,
		Err:        err,
	})
}
