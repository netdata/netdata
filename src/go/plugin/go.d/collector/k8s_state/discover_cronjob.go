// SPDX-License-Identifier: GPL-3.0-or-later

package k8s_state

import (
	"context"

	"k8s.io/client-go/tools/cache"
	"k8s.io/client-go/util/workqueue"

	"github.com/netdata/netdata/go/plugins/logger"
)

func newCronJobDiscoverer(si cache.SharedInformer, l *logger.Logger) *cronJobDiscoverer {
	if si == nil {
		panic("nil cronjob shared informer")
	}

	queue := workqueue.NewTypedWithConfig(workqueue.TypedQueueConfig[string]{Name: "cronjob"})

	_, _ = si.AddEventHandler(cache.ResourceEventHandlerFuncs{
		AddFunc:    func(obj any) { enqueue(queue, obj) },
		UpdateFunc: func(_, obj any) { enqueue(queue, obj) },
		DeleteFunc: func(obj any) { enqueue(queue, obj) },
	})

	return &cronJobDiscoverer{
		Logger:   l,
		informer: si,
		queue:    queue,
		readyCh:  make(chan struct{}),
		stopCh:   make(chan struct{}),
	}
}

type cronjobResource struct {
	src string
	val any
}

func (r cronjobResource) source() string         { return r.src }
func (r cronjobResource) kind() kubeResourceKind { return kubeResourceCronJob }
func (r cronjobResource) value() any             { return r.val }

type cronJobDiscoverer struct {
	*logger.Logger
	informer cache.SharedInformer
	queue    *workqueue.Typed[string]
	readyCh  chan struct{}
	stopCh   chan struct{}
}

func (d *cronJobDiscoverer) run(ctx context.Context, in chan<- resource) {
	d.Info("cronjob_discoverer is started")
	defer func() { close(d.stopCh); d.Info("cronjob_discoverer is stopped") }()

	defer d.queue.ShutDown()

	go d.informer.Run(ctx.Done())

	if !cache.WaitForCacheSync(ctx.Done(), d.informer.HasSynced) {
		return
	}

	go d.runDiscover(ctx, in)

	close(d.readyCh)

	<-ctx.Done()
}

func (d *cronJobDiscoverer) runDiscover(ctx context.Context, in chan<- resource) {
	for {
		key, shutdown := d.queue.Get()
		if shutdown {
			return
		}

		func() {
			defer d.queue.Done(key)

			ns, name, err := cache.SplitMetaNamespaceKey(key)
			if err != nil {
				return
			}

			item, exists, err := d.informer.GetStore().GetByKey(key)
			if err != nil {
				return
			}

			r := &cronjobResource{src: cronjobSource(ns, name)}
			if exists {
				r.val = item
			}
			send(ctx, in, r)
		}()
	}
}

func cronjobSource(namespace, name string) string {
	return "k8s/cj/" + namespace + "/" + name
}

func (d *cronJobDiscoverer) ready() bool   { return isChanClosed(d.readyCh) }
func (d *cronJobDiscoverer) stopped() bool { return isChanClosed(d.stopCh) }
