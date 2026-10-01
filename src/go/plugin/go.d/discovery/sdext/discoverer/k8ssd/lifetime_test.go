// SPDX-License-Identifier: GPL-3.0-or-later

package k8ssd

import (
	"context"
	"testing"
	"testing/synctest"
	"time"

	"github.com/netdata/netdata/go/plugins/plugin/agent/discovery/sd/model"
	"k8s.io/client-go/tools/cache"
)

type lifetimeDiscoverer func(context.Context, chan<- []model.TargetGroup)

func (f lifetimeDiscoverer) Discover(ctx context.Context, out chan<- []model.TargetGroup) {
	f(ctx, out)
}

func TestKubeDiscovererJoinsCancelledChild(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		release, exited := make(chan struct{}), make(chan struct{})
		d := &KubeDiscoverer{
			Logger:  log,
			started: make(chan struct{}),
			discoverers: []model.Discoverer{
				lifetimeDiscoverer(func(ctx context.Context, out chan<- []model.TargetGroup) {
					defer close(exited)
					<-ctx.Done()
					<-release
					out <- []model.TargetGroup{&serviceTargetGroup{
						source: "final",
					}}
				}),
			},
		}
		done := make(chan struct{})
		go func() { defer close(done); d.Discover(ctx, make(chan []model.TargetGroup)) }()
		synctest.Wait()
		cancel()
		time.Sleep(6 * time.Second)
		select {
		case <-done:
			t.Error("Discover returned with a live child")
		default:
		}
		close(release)
		synctest.Wait()
		select {
		case <-exited:
		default:
			t.Error("child blocked on final send")
		}
		select {
		case <-done:
		default:
			t.Error("Discover did not finish")
		}
	})
}

type lifetimeInformer struct {
	cache.SharedInformer
	synced  bool
	release chan struct{}
	exited  chan struct{}
	store   cache.Store
}

func (i *lifetimeInformer) Run(stop <-chan struct{}) { defer close(i.exited); <-stop; <-i.release }
func (i *lifetimeInformer) HasSynced() bool          { return i.synced }
func (i *lifetimeInformer) GetStore() cache.Store    { return i.store }

type lifetimeStore struct {
	cache.Store
	entered chan struct{}
	release chan struct{}
	exited  chan struct{}
}

func (s *lifetimeStore) GetByKey(string) (any, bool, error) {
	close(s.entered)
	<-s.release
	close(s.exited)
	return nil, false, nil
}

func TestRoleDiscovererJoinsInformersAndWorker(t *testing.T) {
	for _, role := range []string{"pod", "service"} {
		for _, synced := range []bool{false, true} {
			name := role + "/cache-sync-failure"
			if synced {
				name = role + "/worker"
			}
			t.Run(name, func(t *testing.T) {
				synctest.Test(t, func(t *testing.T) {
					ctx, cancel := context.WithCancel(context.Background())
					release := make(chan struct{})
					store := &lifetimeStore{
						entered: make(chan struct{}),
						release: make(chan struct{}),
						exited:  make(chan struct{}),
					}
					newInformer := func() *lifetimeInformer {
						return &lifetimeInformer{
							SharedInformer: cache.NewSharedInformer(nil, nil, 0),
							synced:         synced,
							release:        release,
							exited:         make(chan struct{}),
							store:          store,
						}
					}
					inf := newInformer()
					informers := []*lifetimeInformer{inf}
					var d model.Discoverer
					if role == "pod" {
						cmap, secret := newInformer(), newInformer()
						informers = append(informers, cmap, secret)
						p := newPodDiscoverer(inf, cmap, secret)
						p.queue.Add("default/test")
						d = p
					} else {
						s := newServiceDiscoverer(inf)
						s.queue.Add("default/test")
						d = s
					}
					done := make(chan struct{})
					go func() { defer close(done); d.Discover(ctx, make(chan []model.TargetGroup)) }()
					synctest.Wait()
					if synced {
						select {
						case <-store.entered:
						default:
							t.Fatal("worker never started")
						}
					}
					cancel()
					synctest.Wait()
					select {
					case <-done:
						t.Error("Discover returned before informer exit")
					default:
					}
					close(release)
					synctest.Wait()
					if synced {
						select {
						case <-done:
							t.Error("Discover returned before worker exit")
						default:
						}
						close(store.release)
						synctest.Wait()
						select {
						case <-store.exited:
						default:
							t.Error("worker did not exit")
						}
					}
					for _, i := range informers {
						select {
						case <-i.exited:
						default:
							t.Error("informer did not exit")
						}
					}
					select {
					case <-done:
					default:
						t.Error("Discover did not finish")
					}
				})
			})
		}
	}
}
