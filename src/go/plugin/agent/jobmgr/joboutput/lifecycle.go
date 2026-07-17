// SPDX-License-Identifier: GPL-3.0-or-later

package joboutput

import (
	"context"
	"errors"
	"fmt"
	"sync"

	"github.com/netdata/netdata/go/plugins/plugin/agent/jobmgr"
	"github.com/netdata/netdata/go/plugins/plugin/agent/jobmgr/lifecycle"
	"github.com/netdata/netdata/go/plugins/plugin/framework/jobruntime"
)

const (
	jobInstallRoute = "job/install"
	jobStopRoute    = "job/stop"
)

type JobFactory func(context.Context, string, uint64) (ConstructedJob, error)

func NewOpaqueV1Job(cleanup func(context.Context)) (ConstructedJob, error) {
	if cleanup == nil {
		return ConstructedJob{}, errors.New("job output: nil opaque V1 Cleanup")
	}
	return ConstructedJob{
		Variant:         JobVariantV1,
		Runtime:         jobruntime.NewV1Runtime(nil),
		SuppressCleanup: true,
		CollectorCleanup: func(ctx context.Context) error {
			cleanup(ctx)
			return nil
		},
	}, nil
}

// LifecycleSubsystem prepares job install/stop plans while CommandKernel owns
// their lane and current-resource authority.
type LifecycleSubsystem struct {
	retainedBytes int64
	factory       JobFactory
}

func NewLifecycleSubsystem(
	retainedBytes int64,
	factory JobFactory,
) (*LifecycleSubsystem, error) {
	if retainedBytes <= 0 || factory == nil {
		return nil, errors.New("job output: invalid lifecycle subsystem")
	}
	if _, err := lifecycle.NewJobLongLivedPlan(retainedBytes); err != nil {
		return nil, err
	}
	return &LifecycleSubsystem{
		retainedBytes: retainedBytes,
		factory:       factory,
	}, nil
}

func (subsystem *LifecycleSubsystem) Plan(request jobmgr.Request) (jobmgr.WorkPlan, error) {
	if subsystem == nil {
		return jobmgr.WorkPlan{}, errors.New("job output: nil lifecycle subsystem")
	}
	switch request.Route {
	case jobInstallRoute:
		return subsystem.planInstall(request)
	case jobStopRoute:
		return subsystem.planStop(request)
	default:
		return jobmgr.WorkPlan{}, errors.New("job output: unsupported lifecycle route")
	}
}

func (subsystem *LifecycleSubsystem) planInstall(request jobmgr.Request) (jobmgr.WorkPlan, error) {
	if len(request.Args) != 0 || request.LaneKey == "" {
		return jobmgr.WorkPlan{}, errors.New("job output: invalid install request")
	}
	permit, err := lifecycle.NewJobLongLivedPlan(subsystem.retainedBytes)
	if err != nil {
		return jobmgr.WorkPlan{}, err
	}
	return jobmgr.WorkPlan{
		Claims:     []string{"job:" + request.LaneKey},
		NoResponse: true,
		Resource: &jobmgr.ResourcePlan{
			Action: jobmgr.ResourceInstall,
			ID:     request.LaneKey,
			Permit: permit,
			Prepare: func(
				ctx context.Context,
				generation uint64,
				carrier lifecycle.LongLivedPermit,
			) (lifecycle.PreparedResource, error) {
				return PrepareJob(
					ctx,
					request.LaneKey,
					generation,
					carrier,
					func(ctx context.Context) (ConstructedJob, error) {
						return subsystem.factory(ctx, request.LaneKey, generation)
					},
				)
			},
		},
	}, nil
}

func (subsystem *LifecycleSubsystem) planStop(request jobmgr.Request) (jobmgr.WorkPlan, error) {
	if len(request.Args) != 0 || request.LaneKey == "" {
		return jobmgr.WorkPlan{}, errors.New("job output: invalid stop request")
	}
	return jobmgr.WorkPlan{
		Claims:     []string{"job:" + request.LaneKey},
		NoResponse: true,
		Resource: &jobmgr.ResourcePlan{
			Action: jobmgr.ResourceStop,
			ID:     request.LaneKey,
		},
	}, nil
}

// Manager is the command facade; it owns no job generation state.
type Manager struct {
	mu sync.Mutex

	kernel  CommandPort
	nextUID uint64
}

func NewManager(kernel CommandPort) (*Manager, error) {
	if kernel == nil {
		return nil, errors.New("job output: nil manager kernel")
	}
	return &Manager{kernel: kernel}, nil
}

func (manager *Manager) Start(ctx context.Context, key string) error {
	return manager.submit(ctx, key, jobInstallRoute)
}

func (manager *Manager) Restart(ctx context.Context, key string) error {
	if err := manager.submit(ctx, key, jobStopRoute); err != nil {
		return err
	}
	return manager.submit(ctx, key, jobInstallRoute)
}

func (manager *Manager) Stop(ctx context.Context, key string) error {
	return manager.submit(ctx, key, jobStopRoute)
}

func (manager *Manager) submit(ctx context.Context, key, route string) error {
	if manager == nil || manager.kernel == nil || key == "" || ctx == nil {
		return errors.New("job output: invalid manager request")
	}
	manager.mu.Lock()
	manager.nextUID++
	uid := fmt.Sprintf("job:%d", manager.nextUID)
	manager.mu.Unlock()
	deadline, _ := ctx.Deadline()
	return manager.kernel.Submit(ctx, jobmgr.Request{
		UID: uid, LaneKey: key, Source: lifecycle.SourceJobManager,
		Route: route, Deadline: deadline,
	})
}
