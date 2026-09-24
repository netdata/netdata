// SPDX-License-Identifier: GPL-3.0-or-later

package sd

import (
	"context"
	"errors"
	"sync"

	"github.com/netdata/netdata/go/plugins/plugin/framework/dyncfg"
)

type sdActorCommand struct {
	ctx      context.Context
	prepared dyncfg.PreparedCommand
	result   chan sdActorResult
}
type sdActorResult struct {
	applied dyncfg.AppliedCommand
	err     error
}
type sdPreparedCommand struct {
	sd       *ServiceDiscovery
	prepared dyncfg.PreparedCommand
	mu       sync.Mutex
	consumed bool
}

func (d *ServiceDiscovery) prepareDyncfgCommand(fn dyncfg.Function) (dyncfg.PreparedCommand, error) {
	prepared, err := d.handler.Prepare(fn)
	if err != nil {
		return nil, err
	}
	return &sdPreparedCommand{
		sd:       d,
		prepared: prepared,
	}, nil
}
func (p *sdPreparedCommand) take() error {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.consumed {
		return errors.New("service discovery: prepared command consumed")
	}
	p.consumed = true
	return nil
}
func (p *sdPreparedCommand) Dispose(ctx context.Context) error {
	if err := p.take(); err != nil {
		return err
	}
	return p.prepared.Dispose(ctx)
}
func (p *sdPreparedCommand) Apply(ctx context.Context) (dyncfg.AppliedCommand, error) {
	if err := p.take(); err != nil {
		return dyncfg.AppliedCommand{}, err
	}
	if ctx == nil {
		_ = p.prepared.Dispose(context.Background())
		return dyncfg.AppliedCommand{}, errors.New("service discovery: nil apply context")
	}
	request := sdActorCommand{
		ctx:      ctx,
		prepared: p.prepared,
		result:   make(chan sdActorResult, 1),
	}
	select {
	case <-ctx.Done():
		_ = p.prepared.Dispose(context.Background())
		return dyncfg.AppliedCommand{}, context.Cause(ctx)
	case <-p.sd.ctx.Done():
		_ = p.prepared.Dispose(context.Background())
		return dyncfg.AppliedCommand{}, context.Cause(p.sd.ctx)
	case p.sd.actorCommands <- request:
	}
	// The actor owns the adoption decision after handoff. Caller cancellation must
	// not race an accepted result into an ordinary failure reply.
	result := <-request.result
	return result.applied, result.err
}
func (d *ServiceDiscovery) applyActorCommand(ctx context.Context, request sdActorCommand) bool {
	applied, err := request.prepared.Apply(request.ctx)
	if err != nil {
		request.result <- sdActorResult{
			err: err,
		}
		return true
	}
	ack := make(chan struct{})
	var once sync.Once
	activate := applied.Published
	applied.Published = func() { once.Do(func() { close(ack) }) }
	request.result <- sdActorResult{
		applied: applied,
	}
	// Commands, runtime events, and discovered output share this publication cut.
	// On write failure the bridge ends the SD lifetime without releasing activation.
	select {
	case <-ctx.Done():
		d.mgr.StopAll()
		return false
	case <-ack:
		if ctx.Err() != nil {
			d.mgr.StopAll()
			return false
		}
		if activate != nil {
			activate()
		}
		return true
	}
}
