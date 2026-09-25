// SPDX-License-Identifier: GPL-3.0-or-later

package pipeline

import (
	"context"
	"sync"
	"time"

	"github.com/netdata/netdata/go/plugins/logger"
	"github.com/netdata/netdata/go/plugins/plugin/agent/discovery/sd/model"
)

func newAccumulator() *accumulator {
	return &accumulator{
		send:      make(chan struct{}, 1),
		sendEvery: time.Second * 2,
		mux:       &sync.Mutex{},
		tggs:      make(map[string]model.TargetGroup),
	}
}

type accumulator struct {
	*logger.Logger
	discoverers []model.Discoverer
	send        chan struct{}
	sendEvery   time.Duration
	mux         *sync.Mutex
	tggs        map[string]model.TargetGroup
}

func (a *accumulator) run(ctx context.Context, in chan []model.TargetGroup) {
	var wg sync.WaitGroup
	for _, d := range a.discoverers {
		wg.Add(1)
		d := d
		go func() { defer wg.Done(); a.runDiscoverer(ctx, d) }()
	}

	done := make(chan struct{})
	go func() { defer close(done); wg.Wait() }()

	tk := time.NewTicker(a.sendEvery)
	defer tk.Stop()

	for {
		select {
		case <-ctx.Done():
			<-done
			a.Info("all discoverers exited")
			a.finalSend(ctx, in)
			return
		case <-done:
			if !isDone(ctx) {
				a.Info("all discoverers exited before ctx done")
			} else {
				a.Info("all discoverers exited")
			}
			a.finalSend(ctx, in)
			return
		case <-tk.C:
			select {
			case <-a.send:
				a.trySend(in)
			default:
			}
		}
	}
}

func (a *accumulator) runDiscoverer(ctx context.Context, d model.Discoverer) {
	updates := make(chan []model.TargetGroup)
	go func() { defer close(updates); d.Discover(ctx, updates) }()

	// Keep receiving through cancellation: a child may finish with a final send.
	// Closing updates proves Discover has returned, including for finite discovery.
	for tggs := range updates {
		a.mux.Lock()
		a.groupsUpdate(tggs)
		a.mux.Unlock()
		a.triggerSend()
	}
	if !isDone(ctx) {
		a.Infof("discoverer '%v' exited before ctx done", d)
	}
}

func (a *accumulator) trySend(in chan<- []model.TargetGroup) {
	a.mux.Lock()
	defer a.mux.Unlock()

	select {
	case in <- a.groupsList():
		a.groupsReset()
	default:
		a.triggerSend()
	}
}

func (a *accumulator) finalSend(ctx context.Context, in chan<- []model.TargetGroup) {
	a.mux.Lock()
	tggs := a.groupsList()
	if len(tggs) == 0 {
		a.mux.Unlock()
		return
	}
	a.mux.Unlock()

	select {
	case in <- tggs:
		a.mux.Lock()
		a.groupsReset()
		a.mux.Unlock()
		return
	default:
	}

	select {
	case <-ctx.Done():
	case in <- tggs:
		a.mux.Lock()
		a.groupsReset()
		a.mux.Unlock()
	}
}

func (a *accumulator) triggerSend() {
	select {
	case a.send <- struct{}{}:
	default:
	}
}

func (a *accumulator) groupsUpdate(tggs []model.TargetGroup) {
	for _, tgg := range tggs {
		a.tggs[tgg.Source()] = tgg
	}
}

func (a *accumulator) groupsReset() {
	for key := range a.tggs {
		delete(a.tggs, key)
	}
}

func (a *accumulator) groupsList() []model.TargetGroup {
	tggs := make([]model.TargetGroup, 0, len(a.tggs))
	for _, tgg := range a.tggs {
		if tgg != nil {
			tggs = append(tggs, tgg)
		}
	}
	return tggs
}

func isDone(ctx context.Context) bool {
	select {
	case <-ctx.Done():
		return true
	default:
		return false
	}
}
