// SPDX-License-Identifier: GPL-3.0-or-later

package k8ssd

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/netdata/netdata/go/plugins/logger"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/agent/discovery/sd/model"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/k8sclient"

	"github.com/gohugoio/hashstructure"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/watch"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/cache"
	"k8s.io/client-go/util/workqueue"
)

type role string

const (
	rolePod     role = "pod"
	roleService role = "service"
)

const (
	envNodeName = "MY_NODE_NAME"
)

var log = logger.New().With(
	slog.String("component", "service discovery"),
	slog.String("discoverer", "kubernetes"),
)

func NewKubeDiscoverer(cfg Config) (*KubeDiscoverer, error) {
	if err := validateConfig(cfg); err != nil {
		return nil, fmt.Errorf("config validation: %v", err)
	}

	tags, err := model.ParseTags(cfg.Tags)
	if err != nil {
		return nil, fmt.Errorf("parse tags: %v", err)
	}

	client, err := k8sclient.New("Netdata/service-td")
	if err != nil {
		return nil, fmt.Errorf("create clientset: %v", err)
	}

	ns := cfg.Namespaces
	if len(ns) == 0 {
		ns = []string{corev1.NamespaceAll}
	}

	selectorField := cfg.Selector.Field
	if role(cfg.Role) == rolePod && cfg.Pod.LocalMode {
		name := os.Getenv(envNodeName)
		if name == "" {
			return nil, fmt.Errorf("local_mode is enabled, but env '%s' not set", envNodeName)
		}
		selectorField = joinSelectors(selectorField, "spec.nodeName="+name)
	}

	d := &KubeDiscoverer{
		Logger:        log,
		client:        client,
		tags:          tags,
		role:          role(cfg.Role),
		namespaces:    ns,
		selectorLabel: cfg.Selector.Label,
		selectorField: selectorField,
		discoverers:   make([]model.Discoverer, 0, len(ns)),
		started:       make(chan struct{}),
	}

	return d, nil
}

type KubeDiscoverer struct {
	*logger.Logger

	client kubernetes.Interface

	tags          model.Tags
	role          role
	namespaces    []string
	selectorLabel string
	selectorField string
	discoverers   []model.Discoverer
	started       chan struct{}
}

func (d *KubeDiscoverer) String() string {
	return "sd:k8s"
}

const resyncPeriod = 10 * time.Minute

func (d *KubeDiscoverer) Discover(ctx context.Context, in chan<- []model.TargetGroup) {
	d.Info("instance is started")
	defer d.Info("instance is stopped")

	for _, namespace := range d.namespaces {
		var dd model.Discoverer
		switch d.role {
		case rolePod:
			dd = d.setupPodDiscoverer(ctx, namespace)
		case roleService:
			dd = d.setupServiceDiscoverer(ctx, namespace)
		default:
			d.Errorf("unknown role: '%s'", d.role)
			continue
		}
		d.discoverers = append(d.discoverers, dd)
	}

	if len(d.discoverers) == 0 {
		d.Error("no discoverers registered")
		return
	}

	d.Infof("registered: %v", d.discoverers)

	var wg sync.WaitGroup
	updates := make(chan []model.TargetGroup)

	for _, disc := range d.discoverers {
		wg.Add(1)
		go func(disc model.Discoverer) { defer wg.Done(); disc.Discover(ctx, updates) }(disc)
	}

	done := make(chan struct{})
	go func() { defer close(done); wg.Wait() }()

	close(d.started)

	for {
		select {
		case <-ctx.Done():
			select {
			case <-done:
				d.Info("all discoverers exited")
			case <-time.After(time.Second * 5):
				d.Warning("not all discoverers exited")
			}
			return
		case <-done:
			d.Info("all discoverers exited")
			return
		case tggs := <-updates:
			select {
			case <-ctx.Done():
			case in <- tggs:
			}
		}
	}
}

func (d *KubeDiscoverer) setupPodDiscoverer(ctx context.Context, ns string) *podDiscoverer {
	pod := d.client.CoreV1().Pods(ns)
	podLW := &cache.ListWatch{
		ListFunc: func(opts metav1.ListOptions) (runtime.Object, error) {
			opts.FieldSelector = d.selectorField
			opts.LabelSelector = d.selectorLabel
			return pod.List(ctx, opts)
		},
		WatchFunc: func(opts metav1.ListOptions) (watch.Interface, error) {
			opts.FieldSelector = d.selectorField
			opts.LabelSelector = d.selectorLabel
			return pod.Watch(ctx, opts)
		},
	}

	cmap := d.client.CoreV1().ConfigMaps(ns)
	cmapLW := &cache.ListWatch{
		ListFunc: func(opts metav1.ListOptions) (runtime.Object, error) {
			return cmap.List(ctx, opts)
		},
		WatchFunc: func(opts metav1.ListOptions) (watch.Interface, error) {
			return cmap.Watch(ctx, opts)
		},
	}

	secret := d.client.CoreV1().Secrets(ns)
	secretLW := &cache.ListWatch{
		ListFunc: func(opts metav1.ListOptions) (runtime.Object, error) {
			return secret.List(ctx, opts)
		},
		WatchFunc: func(opts metav1.ListOptions) (watch.Interface, error) {
			return secret.Watch(ctx, opts)
		},
	}

	td := newPodDiscoverer(
		cache.NewSharedInformer(podLW, &corev1.Pod{}, resyncPeriod),
		cache.NewSharedInformer(cmapLW, &corev1.ConfigMap{}, resyncPeriod),
		cache.NewSharedInformer(secretLW, &corev1.Secret{}, resyncPeriod),
	)
	td.Tags().Merge(d.tags)

	return td
}

func (d *KubeDiscoverer) setupServiceDiscoverer(ctx context.Context, namespace string) *serviceDiscoverer {
	svc := d.client.CoreV1().Services(namespace)

	svcLW := &cache.ListWatch{
		ListFunc: func(opts metav1.ListOptions) (runtime.Object, error) {
			opts.FieldSelector = d.selectorField
			opts.LabelSelector = d.selectorLabel
			return svc.List(ctx, opts)
		},
		WatchFunc: func(opts metav1.ListOptions) (watch.Interface, error) {
			opts.FieldSelector = d.selectorField
			opts.LabelSelector = d.selectorLabel
			return svc.Watch(ctx, opts)
		},
	}

	inf := cache.NewSharedInformer(svcLW, &corev1.Service{}, resyncPeriod)

	td := newServiceDiscoverer(inf)
	td.Tags().Merge(d.tags)

	return td
}

func enqueue(queue *workqueue.Typed[any], obj any) {
	key, err := cache.DeletionHandlingMetaNamespaceKeyFunc(obj)
	if err != nil {
		return
	}
	queue.Add(key)
}

func send(ctx context.Context, in chan<- []model.TargetGroup, tgg model.TargetGroup) {
	if tgg == nil {
		return
	}
	select {
	case <-ctx.Done():
	case in <- []model.TargetGroup{tgg}:
	}
}

func calcHash(obj any) (uint64, error) {
	return hashstructure.Hash(obj, nil)
}

func joinSelectors(srs ...string) string {
	var i int
	for _, v := range srs {
		if v != "" {
			srs[i] = v
			i++
		}
	}
	return strings.Join(srs[:i], ",")
}
