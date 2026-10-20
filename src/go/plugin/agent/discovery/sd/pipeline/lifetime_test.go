// SPDX-License-Identifier: GPL-3.0-or-later

package pipeline

import (
	"context"
	"testing"
	"testing/synctest"
	"time"

	"github.com/netdata/netdata/go/plugins/logger"
	"github.com/netdata/netdata/go/plugins/plugin/agent/discovery/sd/model"
	"github.com/netdata/netdata/go/plugins/plugin/framework/confgroup"
)

func TestPipelineRunJoinsCancelledDiscoverer(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		release := make(chan struct{})
		exited := make(chan struct{})
		disc := accumulatorDiscoverer(func(ctx context.Context, ch chan<- []model.TargetGroup) {
			defer close(exited)
			<-ctx.Done()
			<-release
			// Shutdown must keep draining a child that finishes with a last send.
			ch <- []model.TargetGroup{newMockTargetGroup("final")}
		})
		p := &Pipeline{
			Logger:      logger.New(),
			accum:       newAccumulator(),
			discoverers: []model.Discoverer{disc},
		}
		p.accum.Logger = p.Logger
		done := make(chan struct{})
		go func() { defer close(done); p.Run(ctx, make(chan []*confgroup.Group)) }()
		synctest.Wait()
		cancel()
		time.Sleep(11 * time.Second)
		select {
		case <-done:
			t.Error("pipeline returned while its discoverer was still alive")
		default:
		}
		close(release)
		synctest.Wait()
		select {
		case <-exited:
		default:
			t.Error("discoverer blocked on its final send")
		}
		select {
		case <-done:
		default:
			t.Error("pipeline failed to join discoverer")
		}
	})
}

func TestPipelinePanicCancelsAndJoinsDiscoverer(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		canceled, release, exited := make(chan struct{}), make(chan struct{}), make(chan struct{})
		discoverer := accumulatorDiscoverer(func(ctx context.Context, out chan<- []model.TargetGroup) {
			defer close(exited)
			out <- []model.TargetGroup{newMockTargetGroup("source", "target")}
			<-ctx.Done()
			close(canceled)
			<-release
			out <- []model.TargetGroup{newMockTargetGroup("final")}
		})
		p, err := New(Config{
			Name: "panic",
			Discoverer: DiscovererPayload{
				Kind:   "test",
				Config: []byte(`{}`),
			},
			Services: []ServiceRuleConfig{{ID: "test", Match: "true", ConfigTemplate: "- null"}},
		}, func(DiscovererPayload, string) ([]model.Discoverer, error) {
			return []model.Discoverer{discoverer}, nil
		})
		if err != nil {
			t.Fatal(err)
		}
		recovered := make(chan any, 1)
		go func() {
			defer func() { recovered <- recover() }()
			p.Run(ctx, make(chan []*confgroup.Group))
		}()
		synctest.Wait()
		time.Sleep(3 * time.Second)
		synctest.Wait()
		select {
		case <-canceled:
		default:
			t.Error("panic did not cancel the discoverer")
		}
		var panicValue any
		select {
		case panicValue = <-recovered:
			t.Error("panic escaped while the discoverer was still alive")
		default:
		}
		// Also release the original failure path so a failing test leaves no child behind.
		cancel()
		close(release)
		synctest.Wait()
		if panicValue == nil {
			select {
			case panicValue = <-recovered:
			default:
				t.Error("pipeline did not finish after child release")
			}
		}
		if panicValue == nil {
			t.Error("fixture did not trigger the rendered-template panic")
		}
		select {
		case <-exited:
		default:
			t.Error("pipeline did not join the discoverer")
		}
	})
}
