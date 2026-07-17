// SPDX-License-Identifier: GPL-3.0-or-later

package composition

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"sort"
	"sync"

	agentdiscovery "github.com/netdata/netdata/go/plugins/plugin/agent/discovery"
	"github.com/netdata/netdata/go/plugins/plugin/agent/jobmgr"
	"github.com/netdata/netdata/go/plugins/plugin/agent/jobmgr/joboutput"
	"github.com/netdata/netdata/go/plugins/plugin/agent/jobmgr/lifecycle"
	"github.com/netdata/netdata/go/plugins/plugin/framework/confgroup"
	"github.com/netdata/netdata/go/plugins/plugin/framework/dyncfg"
)

const (
	discoveryResourceID    = "__jobmgr_discovery__"
	discoveryRetainedBytes = int64(64 * 1024)
)

type runDiscoveryServices struct {
	BuildContext agentdiscovery.BuildContext
	Providers    *agentdiscovery.ProviderCatalog
	RunJob       []string
	AutoEnable   bool
}

func (services runDiscoveryServices) valid() bool {
	return services.Providers != nil && services.Providers.Len() > 0
}

func (generation *runGeneration) startDiscovery(ctx context.Context) error {
	if generation == nil || ctx == nil || !generation.discovery.valid() {
		return errors.New("jobmgr composition: invalid discovery start")
	}
	manager, err := agentdiscovery.NewManager(agentdiscovery.Config{
		Registry:     generation.discovery.BuildContext.Registry,
		BuildContext: generation.discovery.BuildContext,
		Providers:    generation.discovery.Providers.Factories(),
	})
	if err != nil {
		return err
	}
	permitPlan, err := lifecycle.NewPipelineLongLivedPlan(discoveryRetainedBytes)
	if err != nil {
		return err
	}
	result, err := lifecycle.NewSealedResult(204, "application/json", nil)
	if err != nil {
		return err
	}
	plan := jobmgr.WorkPlan{
		NoResponse: true,
		Transaction: &jobmgr.ResourceTransactionPlan{
			ID:                discoveryResourceID,
			AllocateSuccessor: true,
			Permit:            permitPlan,
			Prepare: func(
				context.Context,
				lifecycle.ReadyResource,
				lifecycle.ResourceTransactionScope,
				lifecycle.LongLivedPermit,
			) (lifecycle.PreparedResourceTransaction, error) {
				return nil, errors.New("jobmgr composition: discovery preparation was not rebound")
			},
		},
	}
	plan.Transaction.Prepare = func(
		_ context.Context,
		current lifecycle.ReadyResource,
		scope lifecycle.ResourceTransactionScope,
		permit lifecycle.LongLivedPermit,
	) (lifecycle.PreparedResourceTransaction, error) {
		if current != nil ||
			scope.Current.Valid() ||
			!scope.Successor.Valid() ||
			scope.Successor.ID != discoveryResourceID {
			return nil, errors.New("jobmgr composition: discovery resource already exists")
		}
		prepared, err := newPreparedDiscovery(
			manager,
			generation.discovery,
			generation.dyncfg,
			generation.kernel,
			generation.tasks,
			scope.Successor,
			permit,
			func(cause error) {
				if cause == nil {
					return
				}
				_ = generation.run.Dirty(cause)
				generation.kernel.Stop()
			},
		)
		if err != nil {
			return nil, err
		}
		return joboutput.PrepareResourceTransaction(
			joboutput.ResourceTransactionSpec{
				Scope:       scope,
				Disposition: lifecycle.ResourceTransactionInstalled,
				Successor:   prepared,
				Result:      result,
				Cleanup:     func() error { return nil },
			},
		)
	}
	return generation.kernel.SubmitPreparedAndWait(
		ctx,
		jobmgr.Request{
			UID: fmt.Sprintf(
				"jobmgr-discovery-%d",
				generation.run.Generation(),
			),
			LaneKey: discoveryResourceID,
			Source:  lifecycle.SourceJobManager,
			Route:   "internal/discovery/start",
		},
		plan,
	)
}

type preparedDiscovery struct {
	manager    *agentdiscovery.Manager
	services   runDiscoveryServices
	controller *joboutput.DynCfgJobController
	commands   jobmgr.PreparedCommandPort
	tasks      *lifecycle.TaskSupervisor
	identity   lifecycle.ResourceIdentity
	permit     lifecycle.LongLivedPermit
	fail       func(error)
}

func newPreparedDiscovery(
	manager *agentdiscovery.Manager,
	services runDiscoveryServices,
	controller *joboutput.DynCfgJobController,
	commands jobmgr.PreparedCommandPort,
	tasks *lifecycle.TaskSupervisor,
	identity lifecycle.ResourceIdentity,
	permit lifecycle.LongLivedPermit,
	fail func(error),
) (*preparedDiscovery, error) {
	if manager == nil || !services.valid() || controller == nil ||
		commands == nil || tasks == nil || !identity.Valid() ||
		!permit.Valid() || permit.Owner() != identity ||
		permit.Class() != lifecycle.LongLivedPipeline || fail == nil {
		return nil, errors.New("jobmgr composition: invalid prepared discovery")
	}
	return &preparedDiscovery{
		manager: manager, services: services, controller: controller,
		commands: commands, tasks: tasks, identity: identity, permit: permit,
		fail: fail,
	}, nil
}

func (prepared *preparedDiscovery) Identity() lifecycle.ResourceIdentity {
	if prepared == nil {
		return lifecycle.ResourceIdentity{}
	}
	return prepared.identity
}

func (prepared *preparedDiscovery) AcceptStart(
	ctx context.Context,
	expected uint64,
) (lifecycle.ReadyResource, error) {
	if prepared == nil || ctx == nil || expected != prepared.identity.Generation {
		return nil, errors.New("jobmgr composition: invalid discovery acceptance")
	}
	resource := &readyDiscovery{
		manager: prepared.manager, services: prepared.services,
		controller: prepared.controller, commands: prepared.commands,
		tasks: prepared.tasks, identity: prepared.identity, permit: prepared.permit,
		groups: make(chan []*confgroup.Group, 1),
		fail:   prepared.fail,
	}
	if err := resource.start(ctx); err != nil {
		return resource, err
	}
	return resource, nil
}

func (prepared *preparedDiscovery) Dispose(context.Context) error {
	if prepared == nil {
		return nil
	}
	return prepared.permit.AbortUnused()
}

type readyDiscovery struct {
	mu sync.Mutex

	manager    *agentdiscovery.Manager
	services   runDiscoveryServices
	controller *joboutput.DynCfgJobController
	commands   jobmgr.PreparedCommandPort
	tasks      *lifecycle.TaskSupervisor
	identity   lifecycle.ResourceIdentity
	permit     lifecycle.LongLivedPermit
	groups     chan []*confgroup.Group
	fail       func(error)
	managerRef lifecycle.InheritedTaskRef
	reconRef   lifecycle.InheritedTaskRef
	started    bool
	stopped    bool
	finalized  bool

	managerReleased  bool
	reconReleased    bool
	externalReleased bool
	bytesReleased    bool
	permitReturned   bool
}

func (resource *readyDiscovery) Identity() lifecycle.ResourceIdentity {
	if resource == nil {
		return lifecycle.ResourceIdentity{}
	}
	return resource.identity
}

func (resource *readyDiscovery) start(ctx context.Context) error {
	resource.mu.Lock()
	defer resource.mu.Unlock()
	if resource.started {
		return errors.New("jobmgr composition: discovery already started")
	}
	if err := resource.permit.ActivateExternal(lifecycle.LongLivedEProvider); err != nil {
		return err
	}
	managerRef, err := resource.tasks.StartInheritedWithPermit(
		context.WithoutCancel(ctx),
		resource.identity,
		lifecycle.InheritedPipelineProvider,
		resource.permit,
		func(runCtx context.Context) error {
			resource.manager.Run(runCtx, resource.groups)
			close(resource.groups)
			if runCtx.Err() == nil {
				err := errors.New(
					"jobmgr composition: discovery manager exited unexpectedly",
				)
				resource.fail(err)
				return err
			}
			return nil
		},
	)
	if err != nil {
		return errors.Join(
			err,
			resource.permit.ReleaseExternal(lifecycle.LongLivedEProvider),
			resource.permit.AbortUnused(),
		)
	}
	resource.managerRef = managerRef
	reconciler := newDiscoveryReconciler(
		resource.identity.Generation,
		resource.services,
		resource.controller,
		resource.commands,
	)
	reconRef, err := resource.tasks.StartInheritedWithPermit(
		context.WithoutCancel(ctx),
		resource.identity,
		lifecycle.InheritedPipelineSupervisor,
		resource.permit,
		func(runCtx context.Context) error {
			return reconciler.run(runCtx, resource.groups)
		},
	)
	if err != nil {
		_ = resource.tasks.CancelInherited(managerRef, resource.identity)
		joined, joinErr := resource.tasks.JoinInherited(
			context.Background(),
			managerRef,
			resource.identity,
		)
		if joined {
			joinErr = errors.Join(
				joinErr,
				resource.tasks.ReleaseInherited(managerRef, resource.identity),
			)
		}
		return errors.Join(
			err,
			joinErr,
			resource.permit.ReleaseExternal(lifecycle.LongLivedEProvider),
			resource.permit.AbortUnused(),
		)
	}
	resource.reconRef = reconRef
	resource.started = true
	return nil
}

func (resource *readyDiscovery) Publish() error {
	if resource == nil {
		return errors.New("jobmgr composition: nil discovery publication")
	}
	return nil
}

func (resource *readyDiscovery) AbortReady(ctx context.Context) error {
	return errors.Join(resource.Stop(ctx), resource.Finalize())
}

func (resource *readyDiscovery) Stop(ctx context.Context) error {
	if resource == nil || ctx == nil {
		return errors.New("jobmgr composition: invalid discovery stop")
	}
	resource.mu.Lock()
	if !resource.started {
		resource.mu.Unlock()
		return errors.New("jobmgr composition: discovery was not started")
	}
	if resource.stopped {
		resource.mu.Unlock()
		return nil
	}
	managerRef, reconRef := resource.managerRef, resource.reconRef
	resource.mu.Unlock()

	cancelErr := errors.Join(
		resource.tasks.CancelInherited(managerRef, resource.identity),
		resource.tasks.CancelInherited(reconRef, resource.identity),
	)
	managerJoined, managerErr := resource.tasks.JoinInherited(
		ctx,
		managerRef,
		resource.identity,
	)
	reconJoined, reconErr := resource.tasks.JoinInherited(
		ctx,
		reconRef,
		resource.identity,
	)
	if !managerJoined {
		managerErr = errors.Join(
			managerErr,
			errors.New("jobmgr composition: discovery manager did not join"),
		)
	}
	if !reconJoined {
		reconErr = errors.Join(
			reconErr,
			errors.New("jobmgr composition: discovery reconciler did not join"),
		)
	}
	if err := errors.Join(cancelErr, managerErr, reconErr); err != nil {
		return err
	}
	resource.mu.Lock()
	resource.stopped = true
	resource.mu.Unlock()
	return nil
}

func (resource *readyDiscovery) Finalize() error {
	if resource == nil {
		return errors.New("jobmgr composition: nil discovery finalization")
	}
	resource.mu.Lock()
	if !resource.stopped {
		resource.mu.Unlock()
		return errors.New("jobmgr composition: discovery finalized before stop")
	}
	if resource.finalized {
		resource.mu.Unlock()
		return nil
	}
	managerRef, reconRef := resource.managerRef, resource.reconRef
	if !resource.managerReleased {
		if err := resource.tasks.ReleaseInherited(
			managerRef,
			resource.identity,
		); err != nil {
			resource.mu.Unlock()
			return err
		}
		resource.managerReleased = true
	}
	if !resource.reconReleased {
		if err := resource.tasks.ReleaseInherited(
			reconRef,
			resource.identity,
		); err != nil {
			resource.mu.Unlock()
			return err
		}
		resource.reconReleased = true
	}
	if !resource.externalReleased {
		if err := resource.permit.ReleaseExternal(
			lifecycle.LongLivedEProvider,
		); err != nil {
			resource.mu.Unlock()
			return err
		}
		resource.externalReleased = true
	}
	if !resource.bytesReleased {
		if err := resource.permit.ReleaseBytes(); err != nil {
			resource.mu.Unlock()
			return err
		}
		resource.bytesReleased = true
	}
	if !resource.permitReturned {
		if err := resource.permit.Return(); err != nil {
			resource.mu.Unlock()
			return err
		}
		resource.permitReturned = true
	}
	resource.finalized = true
	resource.mu.Unlock()
	return nil
}

type discoveryReconciler struct {
	generation uint64
	services   runDiscoveryServices
	controller *joboutput.DynCfgJobController
	commands   jobmgr.PreparedCommandPort
	sources    map[string]map[uint64]confgroup.Config
	selected   map[string]confgroup.Config
	nextUID    uint64
}

func newDiscoveryReconciler(
	generation uint64,
	services runDiscoveryServices,
	controller *joboutput.DynCfgJobController,
	commands jobmgr.PreparedCommandPort,
) *discoveryReconciler {
	return &discoveryReconciler{
		generation: generation, services: services,
		controller: controller, commands: commands,
		sources:  make(map[string]map[uint64]confgroup.Config),
		selected: make(map[string]confgroup.Config),
	}
}

func (reconciler *discoveryReconciler) run(
	ctx context.Context,
	groups <-chan []*confgroup.Group,
) error {
	for {
		select {
		case <-ctx.Done():
			return nil
		case batch, ok := <-groups:
			if !ok {
				if ctx.Err() != nil {
					return nil
				}
				return errors.New("jobmgr composition: discovery manager exited")
			}
			for _, group := range batch {
				if err := reconciler.applyGroup(ctx, group); err != nil {
					return err
				}
			}
		}
	}
}

func (reconciler *discoveryReconciler) applyGroup(
	ctx context.Context,
	group *confgroup.Group,
) error {
	if group == nil || group.Source == "" {
		return errors.New("jobmgr composition: invalid discovery group")
	}
	affected := make(map[string]struct{})
	for _, config := range reconciler.sources[group.Source] {
		affected[config.FullName()] = struct{}{}
	}
	next := make(map[uint64]confgroup.Config, len(group.Configs))
	for _, config := range group.Configs {
		if config == nil || !reconciler.allowed(config) {
			continue
		}
		cloned, err := config.Clone()
		if err != nil {
			return err
		}
		next[cloned.Hash()] = cloned
		affected[cloned.FullName()] = struct{}{}
	}
	if len(next) == 0 {
		delete(reconciler.sources, group.Source)
	} else {
		reconciler.sources[group.Source] = next
	}
	names := make([]string, 0, len(affected))
	for name := range affected {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		if err := reconciler.reconcile(ctx, name); err != nil {
			return err
		}
	}
	return nil
}

func (reconciler *discoveryReconciler) allowed(config confgroup.Config) bool {
	if len(reconciler.services.RunJob) == 0 {
		return true
	}
	return slices.Contains(reconciler.services.RunJob, config.Name())
}

func (reconciler *discoveryReconciler) reconcile(
	ctx context.Context,
	fullName string,
) error {
	current, hasCurrent := reconciler.selected[fullName]
	next, hasNext := reconciler.selectConfig(fullName, current, hasCurrent)
	if hasCurrent && hasNext && current.UID() == next.UID() {
		return nil
	}
	var change joboutput.DiscoveredJobChange
	if hasNext {
		change.Config = next
		change.Status = dyncfg.StatusAccepted
		if reconciler.services.AutoEnable {
			change.Status = dyncfg.StatusRunning
		}
	} else {
		change.Config = current
		change.Remove = true
	}
	plan, err := reconciler.controller.PlanDiscovered(change)
	if err != nil {
		return err
	}
	reconciler.nextUID++
	if reconciler.nextUID == 0 {
		return errors.New("jobmgr composition: discovery command UID wrapped")
	}
	if err := reconciler.commands.SubmitPreparedAndWait(
		ctx,
		jobmgr.Request{
			UID: fmt.Sprintf(
				"jobmgr-discovery-%d-%d",
				reconciler.generation,
				reconciler.nextUID,
			),
			LaneKey: fullName,
			Source:  lifecycle.SourceJobManager,
			Route:   "internal/discovery/reconcile",
		},
		plan,
	); err != nil {
		return err
	}
	if hasNext {
		reconciler.selected[fullName] = next
	} else {
		delete(reconciler.selected, fullName)
	}
	return nil
}

func (reconciler *discoveryReconciler) selectConfig(
	fullName string,
	current confgroup.Config,
	hasCurrent bool,
) (confgroup.Config, bool) {
	var candidates []confgroup.Config
	for _, configs := range reconciler.sources {
		for _, config := range configs {
			if config.FullName() == fullName {
				candidates = append(candidates, config)
			}
		}
	}
	if len(candidates) == 0 {
		return nil, false
	}
	sort.Slice(candidates, func(i, j int) bool {
		left, right := candidates[i], candidates[j]
		if left.SourceTypePriority() != right.SourceTypePriority() {
			return left.SourceTypePriority() > right.SourceTypePriority()
		}
		return left.UID() < right.UID()
	})
	priority := candidates[0].SourceTypePriority()
	if hasCurrent && current.SourceTypePriority() == priority {
		for _, candidate := range candidates {
			if candidate.UID() == current.UID() {
				return candidate, true
			}
		}
	}
	return candidates[0], true
}
