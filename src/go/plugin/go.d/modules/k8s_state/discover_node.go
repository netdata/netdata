// SPDX-License-Identifier: GPL-3.0-or-later

package k8s_state

import (
	"context"

	"github.com/netdata/netdata/go/plugins/logger"

	"k8s.io/client-go/tools/cache"
	"k8s.io/client-go/util/workqueue"
)

func newNodeDiscoverer(si cache.SharedInformer, l *logger.Logger) *nodeDiscoverer {
	if si == nil {
		panic("nil node shared informer")
	}

	queue := workqueue.NewTypedWithConfig(workqueue.TypedQueueConfig[any]{Name: "node"})

	_, _ = si.AddEventHandler(cache.ResourceEventHandlerFuncs{
		AddFunc:    func(obj any) { enqueue(queue, obj) },
		UpdateFunc: func(_, obj any) { enqueue(queue, obj) },
		DeleteFunc: func(obj any) { enqueue(queue, obj) },
	})

	return &nodeDiscoverer{
		Logger:   l,
		informer: si,
		queue:    queue,
		readyCh:  make(chan struct{}),
		stopCh:   make(chan struct{}),
	}
}

type nodeResource struct {
	src string
	val any
}

func (r nodeResource) source() string         { return r.src }
func (r nodeResource) kind() kubeResourceKind { return kubeResourceNode }
func (r nodeResource) value() any             { return r.val }

type nodeDiscoverer struct {
	*logger.Logger
	informer cache.SharedInformer
	queue    *workqueue.Typed[any]
	readyCh  chan struct{}
	stopCh   chan struct{}
}

func (d *nodeDiscoverer) run(ctx context.Context, in chan<- resource) {
	d.Info("node_discoverer is started")
	defer func() { close(d.stopCh); d.Info("node_discoverer is stopped") }()

	defer d.queue.ShutDown()

	go d.informer.Run(ctx.Done())

	if !cache.WaitForCacheSync(ctx.Done(), d.informer.HasSynced) {
		return
	}

	go d.runDiscover(ctx, in)
	close(d.readyCh)

	<-ctx.Done()
}

func (d *nodeDiscoverer) ready() bool   { return isChanClosed(d.readyCh) }
func (d *nodeDiscoverer) stopped() bool { return isChanClosed(d.stopCh) }

func (d *nodeDiscoverer) runDiscover(ctx context.Context, in chan<- resource) {
	for {
		item, shutdown := d.queue.Get()
		if shutdown {
			return
		}

		func() {
			defer d.queue.Done(item)

			key := item.(string)
			_, name, err := cache.SplitMetaNamespaceKey(key)
			if err != nil {
				return
			}

			item, exists, err := d.informer.GetStore().GetByKey(key)
			if err != nil {
				return
			}

			r := &nodeResource{src: nodeSource(name)}
			if exists {
				r.val = item
			}
			send(ctx, in, r)
		}()
	}
}

func nodeSource(name string) string {
	return "k8s/node/" + name
}
