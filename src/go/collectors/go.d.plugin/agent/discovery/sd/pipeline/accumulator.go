// SPDX-License-Identifier: GPL-3.0-or-later

package pipeline

import (
	"context"
	"sync"
	"time"

	"github.com/netdata/go.d.plugin/agent/discovery/sd/model"
	"github.com/netdata/go.d.plugin/logger"
)

func newAccumulator() *accumulator {
	return &accumulator{
		send:      make(chan struct{}, 1),
		sendEvery: time.Second * 3,
		mux:       &sync.Mutex{},
		tggs:      make(map[string]model.TargetGroup),
	}
}

type (
	accumulator struct {
		*logger.Logger
		discoverers []model.Discoverer
		send        chan struct{}
		sendEvery   time.Duration
		mux         *sync.Mutex
		tggs        map[string]model.TargetGroup
	}
)

func (a *accumulator) run(ctx context.Context, in chan []model.TargetGroup) {
	updates := make(chan []model.TargetGroup)

	var wg sync.WaitGroup
	for _, d := range a.discoverers {
		wg.Add(1)
		d := d
		go func() { defer wg.Done(); a.runDiscoverer(ctx, d, updates) }()
	}

	done := make(chan struct{})
	go func() { defer close(done); wg.Wait() }()

	tk := time.NewTicker(a.sendEvery)
	defer tk.Stop()

	for {
		select {
		case <-ctx.Done():
			select {
			case <-done:
				a.Info("all discoverers exited")
			case <-time.After(time.Second * 5):
				a.Warning("not all discoverers exited")
			}
			return
		case <-done:
			a.Info("all discoverers exited")
			a.trySend(in)
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

func (a *accumulator) runDiscoverer(ctx context.Context, d model.Discoverer, updates chan []model.TargetGroup) {
	done := make(chan struct{})
	go func() { defer close(done); d.Discover(ctx, updates) }()

	for {
		select {
		case <-ctx.Done():
			select {
			case <-done:
			case <-time.After(time.Second * 5):
			}
			return
		case <-done:
			return
		case tggs := <-updates:
			a.mux.Lock()
			a.groupsUpdate(tggs)
			a.mux.Unlock()
			a.triggerSend()
		}
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
