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

	sources             map[string]map[uint64]confgroup.Config               // per-source config sets (authoritative full set per source)
	candidates          map[string]map[decisionCandidateKey]confgroup.Config // per-job candidate configs by key
	acknowledged        map[string]decisionSelection                         // last acknowledged config and authoritative source slot
	candidateRejections map[string]map[decisionCandidateRevision]struct{}    // exact rejected candidate revisions
	pendingRemovals     map[string]struct{}                                  // rejected removals to retry on a later batch
	revision            uint64                                               // monotonic decision revision
}

type decisionCandidateKey struct {
	source string
	hash   uint64
}

type decisionCandidateRevision struct {
	key decisionCandidateKey
	uid string
}

type decisionSelection struct {
	confgroup.Config
	key decisionCandidateKey
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
		generation:          config.Generation,
		runJob:              runJob,
		autoEnable:          config.AutoEnable,
		plan:                config.Plan,
		commands:            config.Commands,
		diagnostics:         config.Diagnostics,
		sources:             make(map[string]map[uint64]confgroup.Config),
		candidates:          make(map[string]map[decisionCandidateKey]confgroup.Config),
		acknowledged:        make(map[string]decisionSelection),
		candidateRejections: make(map[string]map[decisionCandidateRevision]struct{}),
		pendingRemovals:     make(map[string]struct{}),
	}, nil
}

func (di *DecisionIndex) Apply(ctx context.Context, batch []*confgroup.Group) error {
	if di == nil || ctx == nil || batch == nil {
		return errors.New("jobmgr discovery: invalid decision batch")
	}
	affected := make(map[string]struct{}, len(di.pendingRemovals))
	maps.Copy(affected, di.pendingRemovals)
	for _, group := range batch {
		changed, err := di.applyGroup(group)
		if err != nil {
			return err
		}
		maps.Copy(affected, changed)
	}
	di.pruneCandidateRejections(affected)

	var resultErr error
	for len(affected) > 0 {
		retry, err := di.reconcileRound(ctx, slices.Sorted(maps.Keys(affected)))
		if resultErr == nil {
			resultErr = err
		}
		affected = retry
	}
	return resultErr
}

func (di *DecisionIndex) reconcileRound(
	ctx context.Context,
	names []string,
) (map[string]struct{}, error) {
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

	retry := make(map[string]struct{})
	var resultErr error
	for _, reconciliation := range reconciliations {
		if reconciliation.err == nil {
			delete(di.pendingRemovals, reconciliation.fullName)
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
			if reconciliation.hasNext {
				di.rejectCandidate(reconciliation, reconciliation.err)
				retry[reconciliation.fullName] = struct{}{}
			} else {
				di.deferRemoval(reconciliation.fullName, reconciliation.err)
			}
			continue
		}
		if resultErr == nil {
			resultErr = reconciliation.err
		}
	}
	return retry, resultErr
}

func (di *DecisionIndex) rejectCandidate(reconciliation decisionReconciliation, err error) {
	rejected := di.candidateRejections[reconciliation.fullName]
	if rejected == nil {
		rejected = make(map[decisionCandidateRevision]struct{})
		di.candidateRejections[reconciliation.fullName] = rejected
	}
	rejected[candidateRevision(reconciliation.candidateKey, reconciliation.next)] = struct{}{}
	di.observeRejection(reconciliation.fullName, err)
}

func (di *DecisionIndex) deferRemoval(name string, err error) {
	di.pendingRemovals[name] = struct{}{}
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

func (di *DecisionIndex) applyGroup(group *confgroup.Group) (map[string]struct{}, error) {
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

func (di *DecisionIndex) pruneCandidateRejections(affected map[string]struct{}) {
	for fullName := range affected {
		rejected := di.candidateRejections[fullName]
		candidates := di.candidates[fullName]
		for revision := range rejected {
			candidate, ok := candidates[revision.key]
			if !ok || candidateRevision(revision.key, candidate) != revision {
				delete(rejected, revision)
			}
		}
		if len(rejected) == 0 {
			delete(di.candidateRejections, fullName)
		}
	}
}

func (di *DecisionIndex) allowed(config confgroup.Config) bool {
	if len(di.runJob) == 0 {
		return true
	}
	_, ok := di.runJob[config.Name()]
	return ok
}

type decisionReconciliation struct {
	fullName     string
	next         confgroup.Config
	candidateKey decisionCandidateKey
	hasNext      bool
	submit       bool
	request      jobmgr.Request
	plan         jobmgr.WorkPlan
	err          error
}

func (di *DecisionIndex) prepareReconciliation(fullName string) decisionReconciliation {
	reconciliation := decisionReconciliation{fullName: fullName}
	current, hasCurrent := di.acknowledged[fullName]
	next, candidateKey, hasNext := di.selectConfig(fullName, current.Config, hasCurrent)
	// A rejected replacement never changed graph ownership. Keep that incumbent
	// against same/lower sources until its authoritative source is removed.
	if hasCurrent &&
		di.incumbentSourceBlocked(fullName, current.key.source) &&
		(!hasNext || next.SourceTypePriority() <= current.Config.SourceTypePriority()) {
		return reconciliation
	}
	if !hasCurrent && !hasNext {
		return reconciliation
	}
	if hasCurrent && hasNext && current.Config.UID() == next.UID() {
		return reconciliation
	}
	reconciliation.next = next
	reconciliation.candidateKey = candidateKey
	reconciliation.hasNext = hasNext
	var change DiscoveredChange
	if hasNext {
		change.Config = next
		change.Status = dyncfg.StatusAccepted
		if di.autoEnable {
			change.Status = dyncfg.StatusRunning
		}
	} else {
		change.Config = current.Config
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
		di.acknowledged[reconciliation.fullName] = decisionSelection{
			Config: reconciliation.next,
			key:    reconciliation.candidateKey,
		}
	} else {
		delete(di.acknowledged, reconciliation.fullName)
	}
}

func (di *DecisionIndex) incumbentSourceBlocked(fullName string, source string) bool {
	found := false
	for key, candidate := range di.candidates[fullName] {
		if key.source != source {
			continue
		}
		found = true
		if !di.candidateRejected(fullName, key, candidate) {
			return false
		}
	}
	return found
}

func (di *DecisionIndex) selectConfig(
	fullName string,
	current confgroup.Config,
	hasCurrent bool,
) (confgroup.Config, decisionCandidateKey, bool) {
	candidates := di.candidates[fullName]
	if len(candidates) == 0 {
		return nil, decisionCandidateKey{}, false
	}
	var selected confgroup.Config
	var selectedKey decisionCandidateKey
	maxPriority := -1
	for key, candidate := range candidates {
		if di.candidateRejected(fullName, key, candidate) {
			continue
		}
		priority := candidate.SourceTypePriority()
		if priority > maxPriority || priority == maxPriority && (selected == nil || candidate.UID() < selected.UID()) {
			selected = candidate
			selectedKey = key
			maxPriority = priority
		}
	}
	if selected == nil {
		return nil, decisionCandidateKey{}, false
	}
	if hasCurrent && current.SourceTypePriority() == maxPriority {
		for key, candidate := range candidates {
			if candidate.UID() == current.UID() && !di.candidateRejected(fullName, key, candidate) {
				return candidate, key, true
			}
		}
	}
	return selected, selectedKey, true
}

func (di *DecisionIndex) candidateRejected(
	fullName string,
	key decisionCandidateKey,
	candidate confgroup.Config,
) bool {
	_, ok := di.candidateRejections[fullName][candidateRevision(key, candidate)]
	return ok
}

func candidateRevision(key decisionCandidateKey, candidate confgroup.Config) decisionCandidateRevision {
	return decisionCandidateRevision{
		key: key,
		uid: candidate.UID(),
	}
}
