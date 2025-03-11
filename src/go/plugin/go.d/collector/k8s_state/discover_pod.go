// SPDX-License-Identifier: GPL-3.0-or-later

package k8s_state

import (
	"context"
	"strings"

	corev1 "k8s.io/api/core/v1"

	"github.com/netdata/netdata/go/plugins/logger"

	"k8s.io/client-go/tools/cache"
	"k8s.io/client-go/util/workqueue"
)

func newPodDiscoverer(si cache.SharedInformer, l *logger.Logger) *podDiscoverer {
	if si == nil {
		panic("nil pod shared informer")
	}

	queue := workqueue.NewTypedWithConfig(workqueue.TypedQueueConfig[string]{Name: "pod"})

	_, _ = si.AddEventHandler(cache.ResourceEventHandlerFuncs{
		AddFunc:    func(obj any) { enqueue(queue, obj) },
		UpdateFunc: func(_, obj any) { enqueue(queue, obj) },
		DeleteFunc: func(obj any) { enqueue(queue, obj) },
	})

	return &podDiscoverer{
		Logger:   l,
		informer: si,
		queue:    queue,
		readyCh:  make(chan struct{}),
		stopCh:   make(chan struct{}),
	}
}

type podResource struct {
	src string
	val any
}

func (r podResource) source() string         { return r.src }
func (r podResource) kind() kubeResourceKind { return kubeResourcePod }
func (r podResource) value() any             { return r.val }

type podDiscoverer struct {
	*logger.Logger
	informer cache.SharedInformer
	queue    *workqueue.Typed[string]
	readyCh  chan struct{}
	stopCh   chan struct{}
}

func (d *podDiscoverer) run(ctx context.Context, in chan<- resource) {
	d.Info("pod_discoverer is started")
	defer func() { close(d.stopCh); d.Info("pod_discoverer is stopped") }()

	defer d.queue.ShutDown()

	go d.informer.Run(ctx.Done())

	if !cache.WaitForCacheSync(ctx.Done(), d.informer.HasSynced) {
		return
	}

	go d.runDiscover(ctx, in)
	close(d.readyCh)

	<-ctx.Done()
}

func (d *podDiscoverer) ready() bool   { return isChanClosed(d.readyCh) }
func (d *podDiscoverer) stopped() bool { return isChanClosed(d.stopCh) }

func (d *podDiscoverer) runDiscover(ctx context.Context, in chan<- resource) {
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

			r := &podResource{src: podSource(ns, name)}
			if exists {
				r.val = item
			}
			if pod, err := toPod(r); err == nil && hasIgnoreAnnotation(pod) {
				return
			}
			send(ctx, in, r)
		}()
	}
}

func podSource(namespace, name string) string {
	return "k8s/pod/" + namespace + "/" + name
}

func hasIgnoreAnnotation(pod *corev1.Pod) bool {
	if pod == nil {
		return false
	}
	v := pod.Annotations["netdata.cloud/ignore"]
	return strings.EqualFold(v, "true") || strings.EqualFold(v, "yes")
}
