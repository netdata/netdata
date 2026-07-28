// SPDX-License-Identifier: GPL-3.0-or-later

package discovery

import (
	"context"
	"errors"
	"fmt"
	"maps"
	"slices"
	"sync"

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
	Diagnostics jobmgr.DiagnosticObserver // operational rejection observer
}

// DecisionIndex owns one run generation's desired and acknowledged discovery
// selections.
type DecisionIndex struct {
	generation  uint64                    // run generation
	runJob      map[string]struct{}       // allow-list filter set (empty = allow all)
	autoEnable  bool                      // publish discovered jobs as Running vs Accepted
	plan        PlanDiscovered            // builds a WorkPlan for a discovered change
	commands    PreparedCommandPort       // prepared-command port for submitting decisions
	diagnostics jobmgr.DiagnosticObserver // operational rejection observer

	sources      map[string]map[uint64]confgroup.Config               // per-source config sets (authoritative full set per source)
	candidates   map[string]map[decisionCandidateKey]confgroup.Config // per-job candidate configs by key
	acknowledged map[string]confgroup.Config                          // last acknowledged config per job full name
	pending      map[string]struct{}                                  // rejected selections/removals to retry on a later batch
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
		pending:      make(map[string]struct{}),
	}, nil
}

func (di *DecisionIndex) Apply(ctx context.Context, batch []*confgroup.Group) error {
	if di == nil || ctx == nil || batch == nil {
		return errors.New("jobmgr discovery: invalid decision batch")
	}
	affected := make(map[string]struct{}, len(di.pending))
	maps.Copy(affected, di.pending)
	for _, group := range batch {
		changed, err := di.applyGroup(group)
		if err != nil {
			return err
		}
		maps.Copy(affected, changed)
	}
	names := slices.Sorted(maps.Keys(affected))
	reconciliations := make([]decisionReconciliation, len(names))
	for i, name := range names {
		reconciliations[i] = di.prepareReconciliation(name)
	}
	var wg sync.WaitGroup
	for i := range reconciliations {
		reconciliation := &reconciliations[i]
		if reconciliation.err != nil || !reconciliation.submit {
			continue
		}
		wg.Go(func() {
			reconciliation.err = di.commands.SubmitPreparedAndWait(
				ctx,
				reconciliation.request,
				reconciliation.plan,
			)
		})
	}
	wg.Wait()

	var resultErr error
	for _, reconciliation := range reconciliations {
		if reconciliation.err == nil {
			delete(di.pending, reconciliation.fullName)
			if reconciliation.submit {
				di.acknowledge(reconciliation)
			}
			continue
		}
		if ctx.Err() != nil {
			if resultErr == nil {
				resultErr = errors.Join(ctx.Err(), reconciliation.err)
			}
			continue
		}
		if jobmgr.IsProposalRejection(reconciliation.err) {
			di.quarantine(reconciliation.fullName, reconciliation.err)
			continue
		}
		if resultErr == nil {
			resultErr = reconciliation.err
		}
	}
	return resultErr
}

func (di *DecisionIndex) quarantine(name string, err error) {
	di.pending[name] = struct{}{}
	di.observeRejection(name, err)
}

func (di *DecisionIndex) observeRejection(name string, err error) {
	jobmgr.ObserveDiagnostic(di.diagnostics, jobmgr.DiagnosticEvent{
		Level:      jobmgr.DiagnosticWarning,
		Name:       "discovered collector proposal quarantined",
		Resource:   name,
		State:      "quarantined",
		Generation: di.generation,
		Err:        err,
	})
}

func (di *DecisionIndex) applyGroup(
	group *confgroup.Group,
) (map[string]struct{}, error) {
	if di == nil || group == nil || group.Source == "" {
		return nil, errors.New("jobmgr discovery: invalid group")
	}
	affected := make(map[string]struct{})
	next := make(map[uint64]confgroup.Config, len(group.Configs))
	rejected := make(map[string]error)
	validNames := make(map[string]struct{})
	for _, config := range group.Configs {
		if config == nil || !di.allowed(config) {
			continue
		}
		cloned, err := config.Clone()
		if err != nil {
			fullName := config.FullName()
			if fullName != "" {
				affected[fullName] = struct{}{}
				rejected[fullName] = jobmgr.RejectProposal(fmt.Errorf(
					"jobmgr discovery: clone config %q: %w",
					fullName,
					err,
				))
			} else {
				jobmgr.ObserveDiagnostic(di.diagnostics, jobmgr.DiagnosticEvent{
					Level:      jobmgr.DiagnosticWarning,
					Name:       "discovered collector proposal quarantined",
					Resource:   group.Source,
					State:      "invalid identity",
					Generation: di.generation,
					Err:        jobmgr.RejectProposal(fmt.Errorf("jobmgr discovery: clone config: %w", err)),
				})
			}
			continue
		}
		hash := cloned.Hash()
		next[hash] = cloned
		fullName := cloned.FullName()
		affected[fullName] = struct{}{}
		validNames[fullName] = struct{}{}
	}
	for fullName, err := range rejected {
		di.observeRejection(fullName, err)
		if _, valid := validNames[fullName]; valid {
			continue
		}
		for hash, previous := range di.sources[group.Source] {
			if previous.FullName() == fullName {
				next[hash] = previous
			}
		}
	}

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
	for hash, cloned := range next {
		fullName := cloned.FullName()
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
	return affected, nil
}

func (di *DecisionIndex) allowed(config confgroup.Config) bool {
	if len(di.runJob) == 0 {
		return true
	}
	_, ok := di.runJob[config.Name()]
	return ok
}

type decisionReconciliation struct {
	fullName string
	next     confgroup.Config
	hasNext  bool
	submit   bool
	request  jobmgr.Request
	plan     jobmgr.WorkPlan
	err      error
}

func (di *DecisionIndex) prepareReconciliation(fullName string) decisionReconciliation {
	reconciliation := decisionReconciliation{fullName: fullName}
	current, hasCurrent := di.acknowledged[fullName]
	next, hasNext := di.selectConfig(fullName, current, hasCurrent)
	if !hasCurrent && !hasNext {
		return reconciliation
	}
	if hasCurrent && hasNext && current.UID() == next.UID() {
		return reconciliation
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
	plan, err := di.plan(change)
	if err != nil {
		reconciliation.err = err
		return reconciliation
	}
	if di.revision == ^uint64(0) {
		reconciliation.err = errors.New("jobmgr discovery: decision revision wrapped")
		return reconciliation
	}
	revision := di.revision + 1
	di.revision = revision
	reconciliation.next = next
	reconciliation.hasNext = hasNext
	reconciliation.submit = true
	reconciliation.request = jobmgr.Request{
		UID:     fmt.Sprintf("jobmgr-discovery-%d-%d", di.generation, revision),
		LaneKey: fullName,
		Source:  lifecycle.SourceJobManager,
		Route:   "internal/discovery/reconcile",
	}
	reconciliation.plan = plan
	return reconciliation
}

func (di *DecisionIndex) acknowledge(reconciliation decisionReconciliation) {
	if reconciliation.hasNext {
		di.acknowledged[reconciliation.fullName] = reconciliation.next
	} else {
		delete(di.acknowledged, reconciliation.fullName)
	}
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
