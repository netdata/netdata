// SPDX-License-Identifier: GPL-3.0-or-later

package joboutput

import (
	"crypto/sha256"

	"github.com/netdata/netdata/go/plugins/plugin/agent/jobmgr/lifecycle"
	"github.com/netdata/netdata/go/plugins/plugin/framework/collectorapi"
	"github.com/netdata/netdata/go/plugins/plugin/framework/confgroup"
	"github.com/netdata/netdata/go/plugins/plugin/framework/dyncfg"
	"gopkg.in/yaml.v2"
)

func jobConfigIdentity(config confgroup.Config) collectorapi.JobConfigIdentity {
	if config == nil {
		return collectorapi.JobConfigIdentity{}
	}
	return sha256.Sum256([]byte(config.UID()))
}

func captureJobConfigLifecycle(constructed *ConstructedJob) {
	if constructed == nil || constructed.jobConfigSnapshot != nil ||
		constructed.jobConfigLifecycle == nil || !constructed.jobConfigIdentity.Valid() ||
		constructed.candidateJob == nil {
		return
	}
	var snapshot collectorapi.JobConfigLifecycleSnapshot
	if !callJobConfigLifecycle(func() {
		snapshot = constructed.jobConfigLifecycle.Capture(
			constructed.jobConfigIdentity,
			constructed.candidateJob,
		)
	}) || nilInterfaceValue(snapshot) {
		return
	}
	identity, ok := jobConfigLifecycleSnapshotIdentity(snapshot)
	if !ok || identity != constructed.jobConfigIdentity {
		return
	}
	constructed.jobConfigSnapshot = snapshot
}

func callJobConfigLifecycle(call func()) (completed bool) {
	if call == nil {
		return false
	}
	defer func() {
		if recover() != nil {
			completed = false
		}
	}()
	call()
	return true
}

func jobConfigLifecycleSnapshotIdentity(
	snapshot collectorapi.JobConfigLifecycleSnapshot,
) (identity collectorapi.JobConfigIdentity, ok bool) {
	if nilInterfaceValue(snapshot) {
		return collectorapi.JobConfigIdentity{}, false
	}
	ok = callJobConfigLifecycle(func() { identity = snapshot.Identity() })
	return identity, ok && identity.Valid()
}

type preparedJobConfigLifecycle struct {
	identity collectorapi.JobConfigIdentity
	snapshot collectorapi.JobConfigLifecycleSnapshot
	runtime  collectorapi.RuntimeJob
}

func preparedJobConfigLifecycleState(successor lifecycle.PreparedResource) preparedJobConfigLifecycle {
	if successor == nil {
		return preparedJobConfigLifecycle{}
	}
	provider, ok := successor.(interface {
		jobConfigLifecycleState() preparedJobConfigLifecycle
	})
	if !ok {
		return preparedJobConfigLifecycle{}
	}
	state := provider.jobConfigLifecycleState()
	if !state.identity.Valid() || state.runtime == nil {
		return preparedJobConfigLifecycle{}
	}
	if nilInterfaceValue(state.snapshot) {
		state.snapshot = nil
		return state
	}
	snapshotIdentity, ok := jobConfigLifecycleSnapshotIdentity(state.snapshot)
	if !ok || snapshotIdentity != state.identity {
		state.snapshot = nil
	}
	return state
}

type jobConfigLifecycleGraphState struct {
	identity collectorapi.JobConfigIdentity
	hook     collectorapi.JobConfigLifecycle
	config   confgroup.Config
	valid    bool
}

func (dcjc *DynCfgJobController) prepareJobConfigLifecycleReconcile(
	id string,
	postimage *dyncfg.GraphConfig,
	prepared preparedJobConfigLifecycle,
) func() {
	if dcjc == nil || dcjc.graph == nil || id == "" {
		return nil
	}
	previous := dcjc.currentJobConfigLifecycleGraphState(id)
	next := dcjc.postimageJobConfigLifecycleGraphState(postimage)
	snapshot := prepared.snapshot
	if snapshotIdentity, ok := jobConfigLifecycleSnapshotIdentity(snapshot); !ok ||
		!next.valid || snapshotIdentity != next.identity {
		snapshot = nil
	}
	if next.valid && snapshot == nil {
		callJobConfigLifecycle(func() {
			snapshot = next.hook.Project(next.identity, map[string]any(next.config))
		})
		if snapshotIdentity, ok := jobConfigLifecycleSnapshotIdentity(snapshot); !ok ||
			snapshotIdentity != next.identity {
			snapshot = nil
		}
	}
	if snapshotIdentity, ok := jobConfigLifecycleSnapshotIdentity(snapshot); ok &&
		next.valid && snapshotIdentity == next.identity {
		return func() {
			var runtime collectorapi.RuntimeJob
			if prepared.identity == next.identity && prepared.runtime != nil {
				runtime = prepared.runtime
			}
			callJobConfigLifecycle(func() { next.hook.Reconcile(previous.identity, snapshot, runtime) })
		}
	}
	if previous.valid && (!next.valid || next.identity != previous.identity) && previous.hook != nil {
		return func() {
			callJobConfigLifecycle(func() { previous.hook.Remove(previous.identity) })
		}
	}
	return nil
}

func (dcjc *DynCfgJobController) currentJobConfigLifecycleGraphState(id string) jobConfigLifecycleGraphState {
	record, ok := dcjc.graph.Lookup(id)
	if !ok {
		return jobConfigLifecycleGraphState{}
	}
	hook := dcjc.jobConfigLifecycleHook(record.Module, record.Status)
	if hook == nil {
		return jobConfigLifecycleGraphState{}
	}
	config, err := graphRecordConfig(record)
	if err != nil {
		return jobConfigLifecycleGraphState{}
	}
	return dcjc.jobConfigLifecycleGraphState(record.Module, record.Name, config, hook)
}

func (dcjc *DynCfgJobController) postimageJobConfigLifecycleGraphState(
	postimage *dyncfg.GraphConfig,
) jobConfigLifecycleGraphState {
	if postimage == nil {
		return jobConfigLifecycleGraphState{}
	}
	hook := dcjc.jobConfigLifecycleHook(postimage.Module, postimage.Status)
	if hook == nil {
		return jobConfigLifecycleGraphState{}
	}
	var config confgroup.Config
	if err := yaml.Unmarshal(postimage.Payload, &config); err != nil || config == nil ||
		config.Module() != postimage.Module || config.Name() != postimage.Name {
		return jobConfigLifecycleGraphState{}
	}
	return dcjc.jobConfigLifecycleGraphState(postimage.Module, postimage.Name, config, hook)
}

func (dcjc *DynCfgJobController) jobConfigLifecycleGraphState(
	module string,
	name string,
	config confgroup.Config,
	hook collectorapi.JobConfigLifecycle,
) jobConfigLifecycleGraphState {
	if config == nil || config.Module() != module || config.Name() != name {
		return jobConfigLifecycleGraphState{}
	}
	identity := jobConfigIdentity(config)
	return jobConfigLifecycleGraphState{
		identity: identity,
		hook:     hook,
		config:   config,
		valid:    identity.Valid(),
	}
}

func (dcjc *DynCfgJobController) jobConfigLifecycleHook(
	module string,
	status string,
) collectorapi.JobConfigLifecycle {
	if status != dyncfg.StatusRunning.String() && status != dyncfg.StatusFailed.String() {
		return nil
	}
	creator, ok := dcjc.modules.Lookup(module)
	if !ok || nilInterfaceValue(creator.JobConfigLifecycle) {
		return nil
	}
	return creator.JobConfigLifecycle
}
