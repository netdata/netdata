// SPDX-License-Identifier: GPL-3.0-or-later

package k8s_state

import (
	"context"
	"strings"

	authorizationv1 "k8s.io/api/authorization/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/tools/cache"
	"k8s.io/client-go/util/workqueue"

	"github.com/netdata/netdata/go/plugins/logger"
)

func newPVCDiscoverer(si cache.SharedInformer, l *logger.Logger) *pvcDiscoverer {
	if si == nil {
		panic("nil pvc shared informer")
	}

	queue := workqueue.NewTypedWithConfig(workqueue.TypedQueueConfig[string]{Name: "pvc"})

	_, _ = si.AddEventHandler(cache.ResourceEventHandlerFuncs{
		AddFunc:    func(obj any) { enqueue(queue, obj) },
		UpdateFunc: func(_, obj any) { enqueue(queue, obj) },
		DeleteFunc: func(obj any) { enqueue(queue, obj) },
	})

	return &pvcDiscoverer{
		Logger:   l,
		informer: si,
		queue:    queue,
		readyCh:  make(chan struct{}),
		stopCh:   make(chan struct{}),
	}
}

type pvcResource struct {
	src string
	val any
}

func (r pvcResource) source() string         { return r.src }
func (r pvcResource) kind() kubeResourceKind { return kubeResourcePVC }
func (r pvcResource) value() any             { return r.val }

type pvcDiscoverer struct {
	*logger.Logger
	informer cache.SharedInformer
	queue    *workqueue.Typed[string]
	readyCh  chan struct{}
	stopCh   chan struct{}
}

func (d *pvcDiscoverer) run(ctx context.Context, in chan<- resource) {
	d.Info("pvc_discoverer is started")
	defer func() { close(d.stopCh); d.Info("pvc_discoverer is stopped") }()

	defer d.queue.ShutDown()

	go d.informer.Run(ctx.Done())

	if !cache.WaitForCacheSync(ctx.Done(), d.informer.HasSynced) {
		return
	}

	go d.runDiscover(ctx, in)

	close(d.readyCh)

	<-ctx.Done()
}

func (d *pvcDiscoverer) ready() bool   { return isChanClosed(d.readyCh) }
func (d *pvcDiscoverer) stopped() bool { return isChanClosed(d.stopCh) }

func (d *pvcDiscoverer) runDiscover(ctx context.Context, in chan<- resource) {
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

			r := &pvcResource{src: pvcSource(ns, name)}
			if exists {
				r.val = item
			}
			send(ctx, in, r)
		}()
	}
}

func pvcSource(namespace, name string) string {
	return "k8s/pvc/" + namespace + "/" + name
}

// canListWatchPVCs reports whether the service account may list and watch PersistentVolumeClaims in all namespaces.
// The informer of a forbidden resource never syncs, which would keep the discovery (and so every other kind) not
// ready, so the PVC discoverer is added only when the access is granted.
func (d *kubeDiscovery) canListWatchPVCs(ctx context.Context) bool {
	var denied []string

	for _, verb := range []string{"list", "watch"} {
		review := &authorizationv1.SelfSubjectAccessReview{
			Spec: authorizationv1.SelfSubjectAccessReviewSpec{
				// The empty group is the core API group, the empty namespace means all namespaces.
				ResourceAttributes: &authorizationv1.ResourceAttributes{
					Verb:     verb,
					Resource: "persistentvolumeclaims",
				},
			},
		}

		resp, err := d.client.AuthorizationV1().SelfSubjectAccessReviews().Create(ctx, review, metav1.CreateOptions{})
		if err != nil {
			d.Warningf("PersistentVolumeClaim metrics disabled: cannot check if the service account can %s "+
				"persistentvolumeclaims: %v", verb, err)
			return false
		}
		if !resp.Status.Allowed {
			denied = append(denied, verb)
		}
	}

	if len(denied) > 0 {
		d.Warningf("PersistentVolumeClaim metrics disabled: the service account cannot %s persistentvolumeclaims "+
			"(grant them in the ClusterRole)", strings.Join(denied, "/"))
		return false
	}

	return true
}
