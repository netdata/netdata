// SPDX-License-Identifier: GPL-3.0-or-later

package composition

import (
	"context"
	"errors"
	"fmt"
	"maps"
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

func (rds runDiscoveryServices) valid() bool {
	return rds.Providers != nil && rds.Providers.Len() > 0
}

func (rg *runGeneration) startDiscovery(ctx context.Context) error {
	if rg == nil || ctx == nil || !rg.discovery.valid() {
		return errors.New("jobmgr composition: invalid discovery start")
	}
	pipeline, err := agentdiscovery.NewPipelineGeneration(
		agentdiscovery.PipelineConfig{
			BuildContext: rg.discovery.BuildContext,
			Providers:    rg.discovery.Providers,
		},
	)
	if err != nil {
		return err
	}
	decisions, err := jobmgrdiscovery.NewDecisionIndex(
		jobmgrdiscovery.DecisionConfig{
			Generation: rg.run.Generation(),
			RunJob:     rg.discovery.RunJob,
			AutoEnable: rg.discovery.AutoEnable,
			Commands:   rg.kernel,
			Plan: func(
				change jobmgrdiscovery.DiscoveredChange,
			) (jobmgr.WorkPlan, error) {
				return rg.dyncfg.PlanDiscovered(
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
		rg.discovery.Providers.Names(),
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
			rg.tasks,
			scope.Successor,
			permit,
			func(cause error) {
				if cause == nil {
					return
				}
				_ = rg.run.Dirty(cause)
				rg.kernel.Stop()
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
	return rg.kernel.SubmitPreparedAndWait(
		ctx,
		jobmgr.Request{
			UID: fmt.Sprintf(
				"jobmgr-discovery-%d",
				rg.run.Generation(),
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

func (pd *preparedDiscovery) Identity() lifecycle.ResourceIdentity {
	if pd == nil {
		return lifecycle.ResourceIdentity{}
	}
	return pd.identity
}

func (pd *preparedDiscovery) AcceptStart(
	ctx context.Context,
	expected uint64,
) (lifecycle.ReadyResource, error) {
	if pd == nil || ctx == nil || expected != pd.identity.Generation {
		return nil, errors.New("jobmgr composition: invalid discovery acceptance")
	}
	resource := &readyDiscovery{
		pipeline: pd.pipeline, decisions: pd.decisions,
		tasks: pd.tasks, identity: pd.identity,
		permit: pd.permit, fail: pd.fail,
		providerNames:    pd.pipeline.ProviderNames(),
		disabledNames:    pd.pipeline.DisabledProviderNames(),
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

func (pd *preparedDiscovery) Dispose(context.Context) error {
	if pd == nil {
		return nil
	}
	return pd.permit.AbortUnused()
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

func (rd *readyDiscovery) Identity() lifecycle.ResourceIdentity {
	if rd == nil {
		return lifecycle.ResourceIdentity{}
	}
	return rd.identity
}

func (rd *readyDiscovery) start(ctx context.Context) error {
	rd.mu.Lock()
	defer rd.mu.Unlock()
	if rd.started {
		return errors.New("jobmgr composition: discovery already started")
	}
	if err := rd.permit.ActivateExternal(lifecycle.LongLivedEProvider); err != nil {
		return err
	}
	rd.externalActivated = true
	for _, name := range rd.disabledNames {
		if err := rd.permit.ReleaseUnusedInherited(
			lifecycle.InheritedPipelineProvider,
			name,
		); err != nil {
			return err
		}
	}
	supervisorReady := make(chan struct{})
	supervisorRef, err := rd.tasks.StartInheritedWithPermit(
		context.WithoutCancel(ctx),
		rd.identity,
		lifecycle.InheritedPipelineSupervisor,
		rd.permit,
		func(runCtx context.Context) error {
			close(supervisorReady)
			if !waitDiscoveryPublication(
				runCtx,
				rd.published,
			) {
				return nil
			}
			return runDiscoverySupervisor(
				runCtx,
				rd.pipeline,
				rd.decisions,
				rd.fail,
			)
		},
	)
	if err != nil {
		return err
	}
	rd.supervisorRef = supervisorRef
	<-supervisorReady
	for _, name := range rd.providerNames {
		providerReady := make(chan struct{})
		ref, err := rd.tasks.StartInheritedWithPermitKey(
			context.WithoutCancel(ctx),
			rd.identity,
			lifecycle.InheritedPipelineProvider,
			name,
			rd.permit,
			func(runCtx context.Context) error {
				close(providerReady)
				if !waitDiscoveryPublication(
					runCtx,
					rd.published,
				) {
					return nil
				}
				return runDiscoveryProvider(
					runCtx,
					rd.pipeline,
					name,
					rd.fail,
				)
			},
		)
		if err != nil {
			return err
		}
		rd.providerRefs[name] = ref
		<-providerReady
	}
	rd.started = true
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

func (rd *readyDiscovery) abortStart() error {
	if rd == nil {
		return errors.New("jobmgr composition: invalid discovery start abort")
	}
	rd.mu.Lock()
	supervisorRef := rd.supervisorRef
	providerNames := append([]string(nil), rd.providerNames...)
	providerRefs := make(
		map[string]lifecycle.InheritedTaskRef,
		len(rd.providerRefs),
	)
	maps.Copy(providerRefs, rd.providerRefs)
	externalActivated := rd.externalActivated
	rd.mu.Unlock()

	// Publication is still closed, so every started child is a framework gate
	// waiter and must terminate when its inherited context is canceled.
	var cleanupErr error
	if supervisorRef.Generation != 0 {
		cleanupErr = errors.Join(
			cleanupErr,
			rd.tasks.CancelInherited(
				supervisorRef,
				rd.identity,
			),
		)
	}
	for _, name := range providerNames {
		if ref := providerRefs[name]; ref.Generation != 0 {
			cleanupErr = errors.Join(
				cleanupErr,
				rd.tasks.CancelInherited(
					ref,
					rd.identity,
				),
			)
		}
	}
	for _, name := range providerNames {
		ref := providerRefs[name]
		if ref.Generation == 0 {
			continue
		}
		joined, err := rd.tasks.JoinInherited(
			context.Background(),
			ref,
			rd.identity,
		)
		if !joined {
			cleanupErr = errors.Join(cleanupErr, err)
			continue
		}
		cleanupErr = errors.Join(
			cleanupErr,
			rd.tasks.ReleaseInherited(
				ref,
				rd.identity,
			),
		)
	}
	if supervisorRef.Generation != 0 {
		joined, err := rd.tasks.JoinInherited(
			context.Background(),
			supervisorRef,
			rd.identity,
		)
		if !joined {
			cleanupErr = errors.Join(cleanupErr, err)
		} else {
			cleanupErr = errors.Join(
				cleanupErr,
				rd.tasks.ReleaseInherited(
					supervisorRef,
					rd.identity,
				),
			)
		}
	}
	if externalActivated {
		cleanupErr = errors.Join(
			cleanupErr,
			rd.permit.ReleaseExternal(
				lifecycle.LongLivedEProvider,
			),
		)
	}
	cleanupErr = errors.Join(
		cleanupErr,
		rd.permit.AbortUnused(),
	)
	return cleanupErr
}

func (rd *readyDiscovery) Publish() error {
	if rd == nil {
		return errors.New("jobmgr composition: nil discovery publication")
	}
	rd.mu.Lock()
	defer rd.mu.Unlock()
	if !rd.started || rd.isPublished ||
		rd.stopped || rd.finalized {
		return errors.New(
			"jobmgr composition: invalid discovery publication",
		)
	}
	rd.isPublished = true
	close(rd.published)
	return nil
}

func (rd *readyDiscovery) AbortReady(ctx context.Context) error {
	return errors.Join(rd.Stop(ctx), rd.Finalize())
}

func (rd *readyDiscovery) Stop(ctx context.Context) error {
	if rd == nil || ctx == nil {
		return errors.New("jobmgr composition: invalid discovery stop")
	}
	rd.mu.Lock()
	if !rd.started {
		rd.mu.Unlock()
		return errors.New("jobmgr composition: discovery was not started")
	}
	if rd.stopped {
		rd.mu.Unlock()
		return nil
	}
	supervisorRef := rd.supervisorRef
	providerNames := append([]string(nil), rd.providerNames...)
	providerRefs := make(
		map[string]lifecycle.InheritedTaskRef,
		len(rd.providerRefs),
	)
	maps.Copy(providerRefs, rd.providerRefs)
	rd.mu.Unlock()

	cancelErr := cancelDiscoveryTasks(
		rd.tasks.CancelInherited,
		rd.identity,
		supervisorRef,
		providerNames,
		providerRefs,
	)
	var joinErr error
	for _, name := range providerNames {
		ref := providerRefs[name]
		joined, err := rd.tasks.JoinInherited(
			ctx,
			ref,
			rd.identity,
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
	supervisorJoined, supervisorErr := rd.tasks.JoinInherited(
		ctx,
		supervisorRef,
		rd.identity,
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
	rd.mu.Lock()
	rd.stopped = true
	rd.mu.Unlock()
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

func (rd *readyDiscovery) Finalize() error {
	if rd == nil {
		return errors.New("jobmgr composition: nil discovery finalization")
	}
	rd.mu.Lock()
	if !rd.stopped {
		rd.mu.Unlock()
		return errors.New("jobmgr composition: discovery finalized before stop")
	}
	if rd.finalized {
		rd.mu.Unlock()
		return nil
	}
	for _, name := range rd.providerNames {
		if rd.providerReleased[name] {
			continue
		}
		if err := rd.tasks.ReleaseInherited(
			rd.providerRefs[name],
			rd.identity,
		); err != nil {
			rd.mu.Unlock()
			return err
		}
		rd.providerReleased[name] = true
	}
	if !rd.supervisorReleased {
		if err := rd.tasks.ReleaseInherited(
			rd.supervisorRef,
			rd.identity,
		); err != nil {
			rd.mu.Unlock()
			return err
		}
		rd.supervisorReleased = true
	}
	if !rd.externalReleased {
		if err := rd.permit.ReleaseExternal(
			lifecycle.LongLivedEProvider,
		); err != nil {
			rd.mu.Unlock()
			return err
		}
		rd.externalReleased = true
	}
	if !rd.bytesReleased {
		if err := rd.permit.ReleaseBytes(); err != nil {
			rd.mu.Unlock()
			return err
		}
		rd.bytesReleased = true
	}
	if !rd.permitReturned {
		if err := rd.permit.Return(); err != nil {
			rd.mu.Unlock()
			return err
		}
		rd.permitReturned = true
	}
	rd.finalized = true
	rd.mu.Unlock()
	return nil
}
