// SPDX-License-Identifier: GPL-3.0-or-later

package k8s_state

import (
	"context"

	"k8s.io/client-go/tools/cache"
	"k8s.io/client-go/util/workqueue"

	"github.com/netdata/netdata/go/plugins/logger"
)

func newReplicasetDiscoverer(si cache.SharedInformer, l *logger.Logger) *rsDiscoverer {
	if si == nil {
		panic("nil replicaset shared informer")
	}

	queue := workqueue.NewTypedWithConfig(workqueue.TypedQueueConfig[any]{Name: "replicaset"})

	_, _ = si.AddEventHandler(cache.ResourceEventHandlerFuncs{
		AddFunc:    func(obj any) { enqueue(queue, obj) },
		UpdateFunc: func(_, obj any) { enqueue(queue, obj) },
		DeleteFunc: func(obj any) { enqueue(queue, obj) },
	})

	return &rsDiscoverer{
		Logger:   l,
		informer: si,
		queue:    queue,
		readyCh:  make(chan struct{}),
		stopCh:   make(chan struct{}),
	}
}

type rsResource struct {
	src string
	val any
}

func (r rsResource) source() string         { return r.src }
func (r rsResource) kind() kubeResourceKind { return kubeResourceReplicaset }
func (r rsResource) value() any             { return r.val }

type rsDiscoverer struct {
	*logger.Logger
	informer cache.SharedInformer
	queue    *workqueue.Typed[any]
	readyCh  chan struct{}
	stopCh   chan struct{}
}

func (d *rsDiscoverer) run(ctx context.Context, in chan<- resource) {
	d.Info("replicaset_discoverer is started")
	defer func() { close(d.stopCh); d.Info("replicaset_discoverer is stopped") }()

	defer d.queue.ShutDown()

	go d.informer.Run(ctx.Done())

	if !cache.WaitForCacheSync(ctx.Done(), d.informer.HasSynced) {
		return
	}

	go d.runDiscover(ctx, in)

	close(d.readyCh)

	<-ctx.Done()
}

func (d *rsDiscoverer) ready() bool   { return isChanClosed(d.readyCh) }
func (d *rsDiscoverer) stopped() bool { return isChanClosed(d.stopCh) }

func (d *rsDiscoverer) runDiscover(ctx context.Context, in chan<- resource) {
	for {
		item, shutdown := d.queue.Get()
		if shutdown {
			return
		}

		func() {
			defer d.queue.Done(item)

			key := item.(string)
			ns, name, err := cache.SplitMetaNamespaceKey(key)
			if err != nil {
				return
			}

			item, exists, err := d.informer.GetStore().GetByKey(key)
			if err != nil {
				return
			}

			r := &rsResource{src: rsSource(ns, name)}
			if exists {
				r.val = item
			}
			send(ctx, in, r)
		}()
	}
}

func rsSource(namespace, name string) string {
	return "k8s/rs/" + namespace + "/" + name
}
