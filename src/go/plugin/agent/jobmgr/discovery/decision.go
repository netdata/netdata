// SPDX-License-Identifier: GPL-3.0-or-later

package discovery

import (
	"context"
	"errors"
	"fmt"
	"sort"

	"github.com/netdata/netdata/go/plugins/plugin/agent/jobmgr"
	"github.com/netdata/netdata/go/plugins/plugin/agent/jobmgr/lifecycle"
	"github.com/netdata/netdata/go/plugins/plugin/framework/confgroup"
	"github.com/netdata/netdata/go/plugins/plugin/framework/dyncfg"
)

type DecisionConfig struct {
	Generation uint64
	RunJob     []string
	AutoEnable bool
	Plan       PlanDiscovered
	Commands   PreparedCommandPort
}

// DecisionIndex owns one run generation's desired and acknowledged discovery
// selections.
type DecisionIndex struct {
	generation uint64
	runJob     map[string]struct{}
	autoEnable bool
	plan       PlanDiscovered
	commands   PreparedCommandPort

	sources      map[string]map[uint64]confgroup.Config
	candidates   map[string]map[decisionCandidateKey]confgroup.Config
	acknowledged map[string]acknowledgedConfig
	revision     uint64
}

type decisionCandidateKey struct {
	source string
	hash   uint64
}

type acknowledgedConfig struct {
	config   confgroup.Config
	revision uint64
}

func NewDecisionIndex(config DecisionConfig) (*DecisionIndex, error) {
	if config.Generation == 0 ||
		config.Plan == nil ||
		config.Commands == nil {
		return nil, errors.New(
			"jobmgr discovery: invalid decision index",
		)
	}
	runJob := make(map[string]struct{}, len(config.RunJob))
	for _, name := range config.RunJob {
		if name == "" {
			return nil, errors.New(
				"jobmgr discovery: invalid run-job name",
			)
		}
		runJob[name] = struct{}{}
	}
	return &DecisionIndex{
		generation: config.Generation,
		runJob:     runJob,
		autoEnable: config.AutoEnable,
		plan:       config.Plan,
		commands:   config.Commands,
		sources: make(
			map[string]map[uint64]confgroup.Config,
		),
		candidates: make(
			map[string]map[decisionCandidateKey]confgroup.Config,
		),
		acknowledged: make(
			map[string]acknowledgedConfig,
		),
	}, nil
}

func (index *DecisionIndex) Apply(
	ctx context.Context,
	batch []*confgroup.Group,
) error {
	if index == nil || ctx == nil || batch == nil {
		return errors.New("jobmgr discovery: invalid decision batch")
	}
	for _, group := range batch {
		if err := index.applyGroup(ctx, group); err != nil {
			return err
		}
	}
	return nil
}

func (index *DecisionIndex) applyGroup(
	ctx context.Context,
	group *confgroup.Group,
) error {
	if index == nil || ctx == nil ||
		group == nil || group.Source == "" {
		return errors.New("jobmgr discovery: invalid group")
	}
	affected := make(map[string]struct{})
	for hash, config := range index.sources[group.Source] {
		fullName := config.FullName()
		affected[fullName] = struct{}{}
		candidates := index.candidates[fullName]
		delete(candidates, decisionCandidateKey{
			source: group.Source,
			hash:   hash,
		})
		if len(candidates) == 0 {
			delete(index.candidates, fullName)
		}
	}

	next := make(map[uint64]confgroup.Config, len(group.Configs))
	for _, config := range group.Configs {
		if config == nil || !index.allowed(config) {
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
		candidates := index.candidates[fullName]
		if candidates == nil {
			candidates = make(
				map[decisionCandidateKey]confgroup.Config,
			)
			index.candidates[fullName] = candidates
		}
		candidates[decisionCandidateKey{
			source: group.Source,
			hash:   hash,
		}] = cloned
	}
	if len(next) == 0 {
		delete(index.sources, group.Source)
	} else {
		index.sources[group.Source] = next
	}

	names := make([]string, 0, len(affected))
	for name := range affected {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		if err := index.reconcile(ctx, name); err != nil {
			return err
		}
	}
	return nil
}

func (index *DecisionIndex) allowed(config confgroup.Config) bool {
	if len(index.runJob) == 0 {
		return true
	}
	_, ok := index.runJob[config.Name()]
	return ok
}

func (index *DecisionIndex) reconcile(
	ctx context.Context,
	fullName string,
) error {
	current, hasCurrent := index.acknowledged[fullName]
	next, hasNext := index.selectConfig(
		fullName,
		current.config,
		hasCurrent,
	)
	if hasCurrent && hasNext &&
		current.config.UID() == next.UID() {
		return nil
	}
	var change DiscoveredChange
	if hasNext {
		change.Config = next
		change.Status = dyncfg.StatusAccepted
		if index.autoEnable {
			change.Status = dyncfg.StatusRunning
		}
	} else {
		change.Config = current.config
		change.Remove = true
	}
	plan, err := index.plan(change)
	if err != nil {
		return err
	}
	if index.revision == ^uint64(0) {
		return errors.New(
			"jobmgr discovery: decision revision wrapped",
		)
	}
	revision := index.revision + 1
	if err := index.commands.SubmitPreparedAndWait(
		ctx,
		jobmgr.Request{
			UID: fmt.Sprintf(
				"jobmgr-discovery-%d-%d",
				index.generation,
				revision,
			),
			LaneKey: fullName,
			Source:  lifecycle.SourceJobManager,
			Route:   "internal/discovery/reconcile",
		},
		plan,
	); err != nil {
		return err
	}
	index.revision = revision
	if hasNext {
		index.acknowledged[fullName] = acknowledgedConfig{
			config:   next,
			revision: revision,
		}
	} else {
		delete(index.acknowledged, fullName)
	}
	return nil
}

func (index *DecisionIndex) selectConfig(
	fullName string,
	current confgroup.Config,
	hasCurrent bool,
) (confgroup.Config, bool) {
	candidates := index.candidates[fullName]
	if len(candidates) == 0 {
		return nil, false
	}
	var selected confgroup.Config
	maxPriority := -1
	for _, candidate := range candidates {
		priority := candidate.SourceTypePriority()
		if priority > maxPriority ||
			priority == maxPriority &&
				(selected == nil || candidate.UID() < selected.UID()) {
			selected = candidate
			maxPriority = priority
		}
	}
	if hasCurrent &&
		current.SourceTypePriority() == maxPriority {
		for _, candidate := range candidates {
			if candidate.UID() == current.UID() {
				return candidate, true
			}
		}
	}
	return selected, true
}

type DecisionCensus struct {
	Sources      int
	Candidates   int
	Acknowledged int
	Revision     uint64
}

func (index *DecisionIndex) Census() DecisionCensus {
	if index == nil {
		return DecisionCensus{}
	}
	census := DecisionCensus{
		Sources:      len(index.sources),
		Acknowledged: len(index.acknowledged),
		Revision:     index.revision,
	}
	for _, candidates := range index.candidates {
		census.Candidates += len(candidates)
	}
	return census
}
