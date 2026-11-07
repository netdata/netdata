// SPDX-License-Identifier: GPL-3.0-or-later

package composition

import (
	"context"
	"fmt"

	"github.com/netdata/netdata/go/plugins/plugin/agent/jobmgr"
)

func (rg *runGeneration) startOwnerServices(ctx context.Context) error {
	if len(rg.ownedServices) == 0 {
		return nil
	}
	serviceCtx, cancel := context.WithCancel(ctx)
	registry, err := rg.dyncfg.BindJobOwners(serviceCtx, rg.kernel)
	if err != nil {
		cancel()
		return err
	}
	rg.mu.Lock()
	defer rg.mu.Unlock()
	rg.ownerServicesCancel = cancel
	for _, service := range rg.ownedServices {
		if service == nil {
			continue
		}
		done := make(chan struct{})
		rg.ownerServicesDone = append(rg.ownerServicesDone, done)
		go func() {
			defer close(done)
			defer func() {
				if recovered := recover(); recovered != nil {
					failure := fmt.Errorf("jobmgr composition: owned-job service panicked: %v", recovered)
					jobmgr.ObserveDiagnostic(rg.diagnostics, jobmgr.DiagnosticEvent{
						Level: jobmgr.DiagnosticError,
						Name:  "owned-job service panicked",
						Err:   failure,
					})
					rg.run.Dirty(failure)
					rg.kernel.NotifyControlReady()
				}
			}()
			service.Run(serviceCtx, registry)
		}()
	}
	return nil
}

func (rg *runGeneration) stopOwnerServices() {
	rg.mu.Lock()
	cancel := rg.ownerServicesCancel
	rg.mu.Unlock()
	if cancel != nil {
		cancel()
	}
}

func (rg *runGeneration) waitOwnerServices(ctx context.Context) error {
	rg.mu.Lock()
	done := append([]<-chan struct{}(nil), rg.ownerServicesDone...)
	rg.mu.Unlock()
	for _, completion := range done {
		select {
		case <-completion:
		case <-ctx.Done():
			return fmt.Errorf("jobmgr composition: owned-job service shutdown: %w", ctx.Err())
		}
	}
	return nil
}
