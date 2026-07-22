// SPDX-License-Identifier: GPL-3.0-or-later

package discovery

import (
	"context"
	"errors"
	"fmt"
	"maps"
	"slices"

	"github.com/netdata/netdata/go/plugins/plugin/agent/jobmgr"
	"github.com/netdata/netdata/go/plugins/plugin/agent/jobmgr/lifecycle"
	"github.com/netdata/netdata/go/plugins/plugin/framework/confgroup"
	"github.com/netdata/netdata/go/plugins/plugin/framework/dyncfg"
)

type DecisionConfig struct {
	Generation  uint64                    // run generation
	RunJob      []string                  // allow-list filter of job names (empty = allow all)
	AutoEnable  bool                      // publish discovered jobs as Running vs Accepted
	Plan        PlanDiscovered            // builds a WorkPlan for a discovered change
	Commands    PreparedCommandPort       // prepared-command port for submitting decisions
	Diagnostics jobmgr.DiagnosticObserver // optional transition trace sink
}

// DecisionIndex owns one run generation's desired and acknowledged discovery
// selections.
type DecisionIndex struct {
	generation  uint64                    // run generation
	runJob      map[string]struct{}       // allow-list filter set (empty = allow all)
	autoEnable  bool                      // publish discovered jobs as Running vs Accepted
	plan        PlanDiscovered            // builds a WorkPlan for a discovered change
	commands    PreparedCommandPort       // prepared-command port for submitting decisions
	diagnostics jobmgr.DiagnosticObserver // optional transition trace sink

	sources      map[string]map[uint64]confgroup.Config               // per-source config sets (authoritative full set per source)
	candidates   map[string]map[decisionCandidateKey]confgroup.Config // per-job candidate configs by key
	acknowledged map[string]confgroup.Config                          // last acknowledged config per job full name
	revision     uint64                                               // monotonic decision revision
}

type decisionCandidateKey struct {
	source string
	hash   uint64
}

func NewDecisionIndex(config DecisionConfig) (*DecisionIndex, error) {
	if config.Generation == 0 || config.Plan == nil || config.Commands == nil {
		return nil, errors.New("jobmgr discovery: invalid decision index")
	}
	runJob := make(map[string]struct{}, len(config.RunJob))
	for _, name := range config.RunJob {
		if name == "" {
			return nil, errors.New("jobmgr discovery: invalid run-job name")
		}
		runJob[name] = struct{}{}
	}
	return &DecisionIndex{
		generation:   config.Generation,
		runJob:       runJob,
		autoEnable:   config.AutoEnable,
		plan:         config.Plan,
		commands:     config.Commands,
		diagnostics:  config.Diagnostics,
		sources:      make(map[string]map[uint64]confgroup.Config),
		candidates:   make(map[string]map[decisionCandidateKey]confgroup.Config),
		acknowledged: make(map[string]confgroup.Config),
	}, nil
}

func (di *DecisionIndex) Apply(ctx context.Context, batch []*confgroup.Group) error {
	if di == nil || ctx == nil || batch == nil {
		return errors.New("jobmgr discovery: invalid decision batch")
	}
	di.trace("discovery decision batch received", "", "", len(batch), nil)
	for _, group := range batch {
		if err := di.applyGroup(ctx, group); err != nil {
			return err
		}
	}
	return nil
}

func (di *DecisionIndex) applyGroup(ctx context.Context, group *confgroup.Group) error {
	if di == nil || ctx == nil || group == nil || group.Source == "" {
		return errors.New("jobmgr discovery: invalid group")
	}
	di.trace("discovery source snapshot received", group.Source, "", len(group.Configs), nil)
	affected := make(map[string]struct{})
	for hash, config := range di.sources[group.Source] {
		fullName := config.FullName()
		affected[fullName] = struct{}{}
		candidates := di.candidates[fullName]
		delete(candidates, decisionCandidateKey{
			source: group.Source,
			hash:   hash,
		})
		if len(candidates) == 0 {
			delete(di.candidates, fullName)
		}
	}

	next := make(map[uint64]confgroup.Config, len(group.Configs))
	for _, config := range group.Configs {
		if config == nil || !di.allowed(config) {
			continue
		}
		cloned, err := config.Clone()
		if err != nil {
			return err
		}
		hash := cloned.Hash()
		next[hash] = cloned
		fullName := cloned.FullName()
		affected[fullName] = struct{}{}
		candidates := di.candidates[fullName]
		if candidates == nil {
			candidates = make(map[decisionCandidateKey]confgroup.Config)
			di.candidates[fullName] = candidates
		}
		candidates[decisionCandidateKey{
			source: group.Source,
			hash:   hash,
		}] = cloned
	}
	if len(next) == 0 {
		delete(di.sources, group.Source)
	} else {
		di.sources[group.Source] = next
	}

	names := slices.Sorted(maps.Keys(affected))
	for _, name := range names {
		if err := di.reconcile(ctx, name); err != nil {
			return err
		}
	}
	return nil
}

func (di *DecisionIndex) allowed(config confgroup.Config) bool {
	if len(di.runJob) == 0 {
		return true
	}
	_, ok := di.runJob[config.Name()]
	return ok
}

func (di *DecisionIndex) reconcile(ctx context.Context, fullName string) error {
	current, hasCurrent := di.acknowledged[fullName]
	next, hasNext := di.selectConfig(fullName, current, hasCurrent)
	if hasCurrent && hasNext && current.UID() == next.UID() {
		di.trace("discovery selection unchanged", "", fullName, 0, nil)
		return nil
	}
	var change DiscoveredChange
	if hasNext {
		change.Config = next
		change.Status = dyncfg.StatusAccepted
		if di.autoEnable {
			change.Status = dyncfg.StatusRunning
		}
	} else {
		change.Config = current
		change.Remove = true
	}
	state := "selected"
	if change.Remove {
		state = "removed"
	}
	di.trace("discovery reconciliation planned", state, fullName, 0, nil)
	plan, err := di.plan(change)
	if err != nil {
		di.trace("discovery reconciliation planning failed", state, fullName, 0, err)
		return err
	}
	if di.revision == ^uint64(0) {
		return errors.New("jobmgr discovery: decision revision wrapped")
	}
	revision := di.revision + 1
	if err := di.commands.SubmitPreparedAndWait(
		ctx,
		jobmgr.Request{
			UID:     fmt.Sprintf("jobmgr-discovery-%d-%d", di.generation, revision),
			LaneKey: fullName,
			Source:  lifecycle.SourceJobManager,
			Route:   "internal/discovery/reconcile",
		},
		plan,
	); err != nil {
		di.trace("discovery reconciliation failed", state, fullName, 0, err)
		return err
	}
	di.revision = revision
	if hasNext {
		di.acknowledged[fullName] = next
	} else {
		delete(di.acknowledged, fullName)
	}
	di.trace("discovery reconciliation completed", state, fullName, 0, nil)
	return nil
}

func (di *DecisionIndex) trace(name string, state string, resource string, count int, err error) {
	if di == nil {
		return
	}
	jobmgr.TraceDiagnostic(di.diagnostics, jobmgr.DiagnosticEvent{
		Name:       name,
		Resource:   resource,
		Phase:      state,
		Generation: di.generation,
		Count:      count,
		Err:        err,
	})
}

func (di *DecisionIndex) selectConfig(
	fullName string,
	current confgroup.Config,
	hasCurrent bool,
) (confgroup.Config, bool) {
	candidates := di.candidates[fullName]
	if len(candidates) == 0 {
		return nil, false
	}
	var selected confgroup.Config
	maxPriority := -1
	for _, candidate := range candidates {
		priority := candidate.SourceTypePriority()
		if priority > maxPriority || priority == maxPriority && (selected == nil || candidate.UID() < selected.UID()) {
			selected = candidate
			maxPriority = priority
		}
	}
	if hasCurrent && current.SourceTypePriority() == maxPriority {
		for _, candidate := range candidates {
			if candidate.UID() == current.UID() {
				return candidate, true
			}
		}
	}
	return selected, true
}
