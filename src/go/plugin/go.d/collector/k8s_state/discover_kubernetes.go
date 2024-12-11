// SPDX-License-Identifier: GPL-3.0-or-later

package k8s_state

import (
	"context"
	"os"
	"sync"
	"time"

	"github.com/netdata/netdata/go/plugins/logger"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/watch"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/cache"
	"k8s.io/client-go/util/workqueue"
)

type discoverer interface {
	run(ctx context.Context, in chan<- resource)
	ready() bool
	stopped() bool
}

func newKubeDiscovery(client kubernetes.Interface, l *logger.Logger) *kubeDiscovery {
	return &kubeDiscovery{
		client:  client,
		Logger:  l,
		readyCh: make(chan struct{}),
		stopCh:  make(chan struct{}),
	}
}

type kubeDiscovery struct {
	*logger.Logger
	client      kubernetes.Interface
	discoverers []discoverer
	readyCh     chan struct{}
	stopCh      chan struct{}
}

func (d *kubeDiscovery) run(ctx context.Context, in chan<- resource) {
	d.Info("kube_discoverer is started")
	defer func() { close(d.stopCh); d.Info("kube_discoverer is stopped") }()

	d.discoverers = d.setupDiscoverers(ctx)

	var wg sync.WaitGroup
	updates := make(chan resource)

	for _, dd := range d.discoverers {
		wg.Add(1)
		go func(dd discoverer) { defer wg.Done(); dd.run(ctx, updates) }(dd)
	}

	wg.Add(1)
	go func() { defer wg.Done(); d.runDiscover(ctx, updates, in) }()

	close(d.readyCh)
	wg.Wait()
	<-ctx.Done()
}

func (d *kubeDiscovery) ready() bool {
	if !isChanClosed(d.readyCh) {
		return false
	}
	for _, dd := range d.discoverers {
		if !dd.ready() {
			return false
		}
	}
	return true
}

func (d *kubeDiscovery) stopped() bool {
	if !isChanClosed(d.stopCh) {
		return false
	}
	for _, dd := range d.discoverers {
		if !dd.stopped() {
			return false
		}
	}
	return true
}

func (d *kubeDiscovery) runDiscover(ctx context.Context, updates chan resource, in chan<- resource) {
	for {
		select {
		case <-ctx.Done():
			return
		case r := <-updates:
			select {
			case <-ctx.Done():
				return
			case in <- r:
			}
		}
	}
}

const resyncPeriod = 10 * time.Minute

var (
	myNodeName = os.Getenv("MY_NODE_NAME")
)

func (d *kubeDiscovery) setupDiscoverers(ctx context.Context) []discoverer {
	node := d.client.CoreV1().Nodes()
	nodeWatcher := &cache.ListWatch{
		ListFunc:  func(options metav1.ListOptions) (runtime.Object, error) { return node.List(ctx, options) },
		WatchFunc: func(options metav1.ListOptions) (watch.Interface, error) { return node.Watch(ctx, options) },
	}

	pod := d.client.CoreV1().Pods(corev1.NamespaceAll)
	podWatcher := &cache.ListWatch{
		ListFunc: func(options metav1.ListOptions) (runtime.Object, error) {
			if myNodeName != "" {
				options.FieldSelector = "spec.nodeName=" + myNodeName
			}
			return pod.List(ctx, options)
		},
		WatchFunc: func(options metav1.ListOptions) (watch.Interface, error) {
			if myNodeName != "" {
				options.FieldSelector = "spec.nodeName=" + myNodeName
			}
			return pod.Watch(ctx, options)
		},
	}

	return []discoverer{
		newNodeDiscoverer(cache.NewSharedInformer(nodeWatcher, &corev1.Node{}, resyncPeriod), d.Logger),
		newPodDiscoverer(cache.NewSharedInformer(podWatcher, &corev1.Pod{}, resyncPeriod), d.Logger),
	}
}

func enqueue(queue *workqueue.Typed[any], obj any) {
	key, err := cache.DeletionHandlingMetaNamespaceKeyFunc(obj)
	if err != nil {
		return
	}
	queue.Add(key)
}

func send(ctx context.Context, in chan<- resource, r resource) {
	if r == nil {
		return
	}
	select {
	case <-ctx.Done():
	case in <- r:
	}
}

func isChanClosed(ch chan struct{}) bool {
	select {
	case <-ch:
		return true
	default:
		return false
	}
}
