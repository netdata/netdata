// SPDX-License-Identifier: GPL-3.0-or-later

package composition

import (
	"context"
	"errors"
	"fmt"
	"sync"

	agentdiscovery "github.com/netdata/netdata/go/plugins/plugin/agent/discovery"
	"github.com/netdata/netdata/go/plugins/plugin/agent/jobmgr"
	jobmgrdiscovery "github.com/netdata/netdata/go/plugins/plugin/agent/jobmgr/discovery"
	"github.com/netdata/netdata/go/plugins/plugin/agent/jobmgr/joboutput"
	"github.com/netdata/netdata/go/plugins/plugin/agent/jobmgr/lifecycle"
)

const discoveryResourceID = "__jobmgr_discovery__"

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
	pipeline, err := agentdiscovery.NewPipelineGeneration(
		agentdiscovery.PipelineConfig{
			BuildContext: generation.discovery.BuildContext,
			Providers:    generation.discovery.Providers,
		},
	)
	if err != nil {
		return err
	}
	decisions, err := jobmgrdiscovery.NewDecisionIndex(
		jobmgrdiscovery.DecisionConfig{
			Generation: generation.run.Generation(),
			RunJob:     generation.discovery.RunJob,
			AutoEnable: generation.discovery.AutoEnable,
			Commands:   generation.kernel,
			Plan: func(
				change jobmgrdiscovery.DiscoveredChange,
			) (jobmgr.WorkPlan, error) {
				return generation.dyncfg.PlanDiscovered(
					joboutput.DiscoveredJobChange{
						Config: change.Config,
						Status: change.Status,
						Remove: change.Remove,
					},
				)
			},
		},
	)
	if err != nil {
		return err
	}
	permitPlan, err := lifecycle.NewPipelineLongLivedPlan(
		generation.discovery.Providers.Names(),
	)
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
			pipeline,
			decisions,
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
	pipeline  *agentdiscovery.PipelineGeneration
	decisions *jobmgrdiscovery.DecisionIndex
	tasks     *lifecycle.TaskSupervisor
	identity  lifecycle.ResourceIdentity
	permit    lifecycle.LongLivedPermit
	fail      func(error)
}

func newPreparedDiscovery(
	pipeline *agentdiscovery.PipelineGeneration,
	decisions *jobmgrdiscovery.DecisionIndex,
	tasks *lifecycle.TaskSupervisor,
	identity lifecycle.ResourceIdentity,
	permit lifecycle.LongLivedPermit,
	fail func(error),
) (*preparedDiscovery, error) {
	if pipeline == nil || decisions == nil ||
		len(pipeline.ProviderNames()) == 0 ||
		tasks == nil || !identity.Valid() ||
		!permit.Valid() || permit.Owner() != identity ||
		permit.Class() != lifecycle.LongLivedPipeline || fail == nil {
		return nil, errors.New("jobmgr composition: invalid prepared discovery")
	}
	return &preparedDiscovery{
		pipeline: pipeline, decisions: decisions, tasks: tasks,
		identity: identity, permit: permit, fail: fail,
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
		pipeline: prepared.pipeline, decisions: prepared.decisions,
		tasks: prepared.tasks, identity: prepared.identity,
		permit: prepared.permit, fail: prepared.fail,
		providerNames:    prepared.pipeline.ProviderNames(),
		disabledNames:    prepared.pipeline.DisabledProviderNames(),
		providerRefs:     make(map[string]lifecycle.InheritedTaskRef),
		providerReleased: make(map[string]bool),
		published:        make(chan struct{}),
	}
	if err := resource.start(ctx); err != nil {
		cleanupErr := resource.abortStart()
		if cleanupErr == nil {
			return nil, err
		}
		return resource, errors.Join(err, cleanupErr)
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

	pipeline      *agentdiscovery.PipelineGeneration
	decisions     *jobmgrdiscovery.DecisionIndex
	tasks         *lifecycle.TaskSupervisor
	identity      lifecycle.ResourceIdentity
	permit        lifecycle.LongLivedPermit
	fail          func(error)
	providerNames []string
	disabledNames []string
	published     chan struct{}
	supervisorRef lifecycle.InheritedTaskRef
	providerRefs  map[string]lifecycle.InheritedTaskRef
	started       bool
	isPublished   bool
	stopped       bool
	finalized     bool

	providerReleased   map[string]bool
	supervisorReleased bool
	externalActivated  bool
	externalReleased   bool
	bytesReleased      bool
	permitReturned     bool
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
	resource.externalActivated = true
	for _, name := range resource.disabledNames {
		if err := resource.permit.ReleaseUnusedInherited(
			lifecycle.InheritedPipelineProvider,
			name,
		); err != nil {
			return err
		}
	}
	supervisorReady := make(chan struct{})
	supervisorRef, err := resource.tasks.StartInheritedWithPermit(
		context.WithoutCancel(ctx),
		resource.identity,
		lifecycle.InheritedPipelineSupervisor,
		resource.permit,
		func(runCtx context.Context) error {
			close(supervisorReady)
			if !waitDiscoveryPublication(
				runCtx,
				resource.published,
			) {
				return nil
			}
			return runDiscoverySupervisor(
				runCtx,
				resource.pipeline,
				resource.decisions,
				resource.fail,
			)
		},
	)
	if err != nil {
		return err
	}
	resource.supervisorRef = supervisorRef
	<-supervisorReady
	for _, name := range resource.providerNames {
		name := name
		providerReady := make(chan struct{})
		ref, err := resource.tasks.StartInheritedWithPermitKey(
			context.WithoutCancel(ctx),
			resource.identity,
			lifecycle.InheritedPipelineProvider,
			name,
			resource.permit,
			func(runCtx context.Context) error {
				close(providerReady)
				if !waitDiscoveryPublication(
					runCtx,
					resource.published,
				) {
					return nil
				}
				return runDiscoveryProvider(
					runCtx,
					resource.pipeline,
					name,
					resource.fail,
				)
			},
		)
		if err != nil {
			return err
		}
		resource.providerRefs[name] = ref
		<-providerReady
	}
	resource.started = true
	return nil
}

func waitDiscoveryPublication(
	ctx context.Context,
	published <-chan struct{},
) bool {
	select {
	case <-ctx.Done():
		return false
	case <-published:
		return true
	}
}

func runDiscoverySupervisor(
	ctx context.Context,
	pipeline *agentdiscovery.PipelineGeneration,
	decisions *jobmgrdiscovery.DecisionIndex,
	fail func(error),
) (err error) {
	defer func() {
		if recovered := recover(); recovered != nil {
			err = fmt.Errorf(
				"%w in discovery supervisor: %v",
				lifecycle.ErrTaskPanic,
				recovered,
			)
		}
		if err != nil && ctx.Err() == nil {
			fail(err)
		}
	}()
	err = pipeline.Run(ctx, decisions.Apply)
	if err == nil && ctx.Err() == nil {
		err = errors.New(
			"jobmgr composition: discovery supervisor exited unexpectedly",
		)
	}
	return err
}

func runDiscoveryProvider(
	ctx context.Context,
	pipeline *agentdiscovery.PipelineGeneration,
	name string,
	fail func(error),
) (err error) {
	defer func() {
		if recovered := recover(); recovered != nil {
			err = fmt.Errorf(
				"%w in discovery provider %q: %v",
				lifecycle.ErrTaskPanic,
				name,
				recovered,
			)
		}
		if err != nil && ctx.Err() == nil {
			fail(err)
		}
	}()
	return pipeline.RunProvider(ctx, name)
}

func (resource *readyDiscovery) abortStart() error {
	if resource == nil {
		return errors.New("jobmgr composition: invalid discovery start abort")
	}
	resource.mu.Lock()
	supervisorRef := resource.supervisorRef
	providerNames := append([]string(nil), resource.providerNames...)
	providerRefs := make(
		map[string]lifecycle.InheritedTaskRef,
		len(resource.providerRefs),
	)
	for name, ref := range resource.providerRefs {
		providerRefs[name] = ref
	}
	externalActivated := resource.externalActivated
	resource.mu.Unlock()

	// Publication is still closed, so every started child is a framework gate
	// waiter and must terminate when its inherited context is canceled.
	var cleanupErr error
	if supervisorRef.Generation != 0 {
		cleanupErr = errors.Join(
			cleanupErr,
			resource.tasks.CancelInherited(
				supervisorRef,
				resource.identity,
			),
		)
	}
	for _, name := range providerNames {
		if ref := providerRefs[name]; ref.Generation != 0 {
			cleanupErr = errors.Join(
				cleanupErr,
				resource.tasks.CancelInherited(
					ref,
					resource.identity,
				),
			)
		}
	}
	for _, name := range providerNames {
		ref := providerRefs[name]
		if ref.Generation == 0 {
			continue
		}
		joined, err := resource.tasks.JoinInherited(
			context.Background(),
			ref,
			resource.identity,
		)
		if !joined {
			cleanupErr = errors.Join(cleanupErr, err)
			continue
		}
		cleanupErr = errors.Join(
			cleanupErr,
			resource.tasks.ReleaseInherited(
				ref,
				resource.identity,
			),
		)
	}
	if supervisorRef.Generation != 0 {
		joined, err := resource.tasks.JoinInherited(
			context.Background(),
			supervisorRef,
			resource.identity,
		)
		if !joined {
			cleanupErr = errors.Join(cleanupErr, err)
		} else {
			cleanupErr = errors.Join(
				cleanupErr,
				resource.tasks.ReleaseInherited(
					supervisorRef,
					resource.identity,
				),
			)
		}
	}
	if externalActivated {
		cleanupErr = errors.Join(
			cleanupErr,
			resource.permit.ReleaseExternal(
				lifecycle.LongLivedEProvider,
			),
		)
	}
	cleanupErr = errors.Join(
		cleanupErr,
		resource.permit.AbortUnused(),
	)
	return cleanupErr
}

func (resource *readyDiscovery) Publish() error {
	if resource == nil {
		return errors.New("jobmgr composition: nil discovery publication")
	}
	resource.mu.Lock()
	defer resource.mu.Unlock()
	if !resource.started || resource.isPublished ||
		resource.stopped || resource.finalized {
		return errors.New(
			"jobmgr composition: invalid discovery publication",
		)
	}
	resource.isPublished = true
	close(resource.published)
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
	supervisorRef := resource.supervisorRef
	providerNames := append([]string(nil), resource.providerNames...)
	providerRefs := make(
		map[string]lifecycle.InheritedTaskRef,
		len(resource.providerRefs),
	)
	for name, ref := range resource.providerRefs {
		providerRefs[name] = ref
	}
	resource.mu.Unlock()

	cancelErr := cancelDiscoveryTasks(
		resource.tasks.CancelInherited,
		resource.identity,
		supervisorRef,
		providerNames,
		providerRefs,
	)
	var joinErr error
	for _, name := range providerNames {
		ref := providerRefs[name]
		joined, err := resource.tasks.JoinInherited(
			ctx,
			ref,
			resource.identity,
		)
		if !joined {
			err = errors.Join(
				err,
				fmt.Errorf(
					"jobmgr composition: discovery provider %q did not join",
					name,
				),
			)
		}
		joinErr = errors.Join(joinErr, err)
	}
	supervisorJoined, supervisorErr := resource.tasks.JoinInherited(
		ctx,
		supervisorRef,
		resource.identity,
	)
	if !supervisorJoined {
		supervisorErr = errors.Join(
			supervisorErr,
			errors.New(
				"jobmgr composition: discovery supervisor did not join",
			),
		)
	}
	if err := errors.Join(cancelErr, joinErr, supervisorErr); err != nil {
		return err
	}
	resource.mu.Lock()
	resource.stopped = true
	resource.mu.Unlock()
	return nil
}

func cancelDiscoveryTasks(
	cancel func(
		lifecycle.InheritedTaskRef,
		lifecycle.ResourceIdentity,
	) error,
	identity lifecycle.ResourceIdentity,
	supervisorRef lifecycle.InheritedTaskRef,
	providerNames []string,
	providerRefs map[string]lifecycle.InheritedTaskRef,
) error {
	if cancel == nil {
		return errors.New("jobmgr composition: nil discovery cancellation")
	}
	err := cancel(supervisorRef, identity)
	for _, name := range providerNames {
		err = errors.Join(
			err,
			cancel(providerRefs[name], identity),
		)
	}
	return err
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
	for _, name := range resource.providerNames {
		if resource.providerReleased[name] {
			continue
		}
		if err := resource.tasks.ReleaseInherited(
			resource.providerRefs[name],
			resource.identity,
		); err != nil {
			resource.mu.Unlock()
			return err
		}
		resource.providerReleased[name] = true
	}
	if !resource.supervisorReleased {
		if err := resource.tasks.ReleaseInherited(
			resource.supervisorRef,
			resource.identity,
		); err != nil {
			resource.mu.Unlock()
			return err
		}
		resource.supervisorReleased = true
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
