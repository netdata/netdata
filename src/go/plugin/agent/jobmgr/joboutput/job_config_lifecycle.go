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

func preparedJobConfigLifecycleSnapshot(
	successor lifecycle.PreparedResource,
) collectorapi.JobConfigLifecycleSnapshot {
	if successor == nil {
		return nil
	}
	provider, ok := successor.(interface {
		jobConfigLifecycleSnapshot() collectorapi.JobConfigLifecycleSnapshot
	})
	if !ok {
		return nil
	}
	snapshot := provider.jobConfigLifecycleSnapshot()
	if nilInterfaceValue(snapshot) {
		return nil
	}
	return snapshot
}

type jobConfigLifecycleGraphState struct {
	identity collectorapi.JobConfigIdentity
	hook     collectorapi.JobConfigLifecycle
	valid    bool
}

func (dcjc *DynCfgJobController) prepareJobConfigLifecycleCommit(
	id string,
	postimage *dyncfg.GraphConfig,
	snapshot collectorapi.JobConfigLifecycleSnapshot,
) func() {
	if dcjc == nil || dcjc.graph == nil || id == "" {
		return nil
	}
	previous := dcjc.currentJobConfigLifecycleGraphState(id)
	next := dcjc.postimageJobConfigLifecycleGraphState(postimage)
	if snapshotIdentity, ok := jobConfigLifecycleSnapshotIdentity(snapshot); ok &&
		next.valid && snapshotIdentity == next.identity {
		return func() {
			callJobConfigLifecycle(func() { snapshot.Commit(previous.identity) })
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
	config, err := graphRecordConfig(record)
	if err != nil {
		return jobConfigLifecycleGraphState{}
	}
	return dcjc.jobConfigLifecycleGraphState(record.Module, record.Name, record.Status, config)
}

func (dcjc *DynCfgJobController) postimageJobConfigLifecycleGraphState(
	postimage *dyncfg.GraphConfig,
) jobConfigLifecycleGraphState {
	if postimage == nil {
		return jobConfigLifecycleGraphState{}
	}
	var config confgroup.Config
	if err := yaml.Unmarshal(postimage.Payload, &config); err != nil || config == nil ||
		config.Module() != postimage.Module || config.Name() != postimage.Name {
		return jobConfigLifecycleGraphState{}
	}
	return dcjc.jobConfigLifecycleGraphState(postimage.Module, postimage.Name, postimage.Status, config)
}

func (dcjc *DynCfgJobController) jobConfigLifecycleGraphState(
	module string,
	name string,
	status string,
	config confgroup.Config,
) jobConfigLifecycleGraphState {
	if config == nil || config.Module() != module || config.Name() != name {
		return jobConfigLifecycleGraphState{}
	}
	if status != dyncfg.StatusRunning.String() && status != dyncfg.StatusFailed.String() {
		return jobConfigLifecycleGraphState{}
	}
	creator, ok := dcjc.modules.Lookup(module)
	if !ok || creator.JobConfigLifecycle == nil {
		return jobConfigLifecycleGraphState{}
	}
	identity := jobConfigIdentity(config)
	return jobConfigLifecycleGraphState{
		identity: identity,
		hook:     creator.JobConfigLifecycle,
		valid:    identity.Valid(),
	}
}
