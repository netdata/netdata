// SPDX-License-Identifier: GPL-3.0-or-later

package kubernetes

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/netdata/go.d.plugin/agent/discovery/sd/model"
	"github.com/netdata/go.d.plugin/logger"
	"github.com/netdata/go.d.plugin/pkg/k8sclient"

	"github.com/ilyam8/hashstructure"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/watch"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/cache"
	"k8s.io/client-go/util/workqueue"
)

const (
	envNodeName = "MY_NODE_NAME"
)

var log = logger.New().With(
	slog.String("component", "discovery sd k8s"),
)

func NewKubeDiscoverer(cfg Config) (*KubeDiscoverer, error) {
	if err := validateConfig(cfg); err != nil {
		return nil, fmt.Errorf("config validation: %v", err)
	}

	client, err := k8sclient.New("Netdata/service-td")
	if err != nil {
		return nil, fmt.Errorf("create clientset: %v", err)
	}

	ns := cfg.Namespaces
	if len(ns) == 0 {
		ns = []string{corev1.NamespaceAll}
	}

	d := &KubeDiscoverer{
		Logger:      log,
		namespaces:  ns,
		podConf:     cfg.Pod,
		svcConf:     cfg.Service,
		client:      client,
		discoverers: make([]model.Discoverer, 0, len(ns)),
		started:     make(chan struct{}),
	}

	return d, nil
}

type KubeDiscoverer struct {
	*logger.Logger

	podConf *PodConfig
	svcConf *ServiceConfig

	namespaces  []string
	client      kubernetes.Interface
	discoverers []model.Discoverer
	started     chan struct{}
}

func (d *KubeDiscoverer) String() string {
	return "k8s td manager"
}

const resyncPeriod = 10 * time.Minute

func (d *KubeDiscoverer) Discover(ctx context.Context, in chan<- []model.TargetGroup) {
	d.Info("instance is started")
	defer d.Info("instance is stopped")

	for _, namespace := range d.namespaces {
		if err := d.setupPodDiscoverer(ctx, d.podConf, namespace); err != nil {
			d.Errorf("create pod discoverer: %v", err)
			return
		}
		if err := d.setupServiceDiscoverer(ctx, d.svcConf, namespace); err != nil {
			d.Errorf("create service discoverer: %v", err)
			return
		}
	}

	if len(d.discoverers) == 0 {
		d.Warning("no discoverers registered")
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

func (d *KubeDiscoverer) setupPodDiscoverer(ctx context.Context, conf *PodConfig, namespace string) error {
	if conf == nil {
		return nil
	}

	if conf.LocalMode {
		name := os.Getenv(envNodeName)
		if name == "" {
			return fmt.Errorf("local_mode is enabled, but env '%s' not set", envNodeName)
		}
		conf.Selector.Field = joinSelectors(conf.Selector.Field, "spec.nodeName="+name)
	}

	tags, err := model.ParseTags(conf.Tags)
	if err != nil {
		return fmt.Errorf("parse tags: %v", err)
	}

	pod := d.client.CoreV1().Pods(namespace)
	podLW := &cache.ListWatch{
		ListFunc: func(options metav1.ListOptions) (runtime.Object, error) {
			options.FieldSelector = conf.Selector.Field
			options.LabelSelector = conf.Selector.Label
			return pod.List(ctx, options)
		},
		WatchFunc: func(options metav1.ListOptions) (watch.Interface, error) {
			options.FieldSelector = conf.Selector.Field
			options.LabelSelector = conf.Selector.Label
			return pod.Watch(ctx, options)
		},
	}

	cmap := d.client.CoreV1().ConfigMaps(namespace)
	cmapLW := &cache.ListWatch{
		ListFunc: func(options metav1.ListOptions) (runtime.Object, error) {
			return cmap.List(ctx, options)
		},
		WatchFunc: func(options metav1.ListOptions) (watch.Interface, error) {
			return cmap.Watch(ctx, options)
		},
	}

	secret := d.client.CoreV1().Secrets(namespace)
	secretLW := &cache.ListWatch{
		ListFunc: func(options metav1.ListOptions) (runtime.Object, error) {
			return secret.List(ctx, options)
		},
		WatchFunc: func(options metav1.ListOptions) (watch.Interface, error) {
			return secret.Watch(ctx, options)
		},
	}

	td := newPodDiscoverer(
		cache.NewSharedInformer(podLW, &corev1.Pod{}, resyncPeriod),
		cache.NewSharedInformer(cmapLW, &corev1.ConfigMap{}, resyncPeriod),
		cache.NewSharedInformer(secretLW, &corev1.Secret{}, resyncPeriod),
	)
	td.Tags().Merge(tags)

	d.discoverers = append(d.discoverers, td)

	return nil
}

func (d *KubeDiscoverer) setupServiceDiscoverer(ctx context.Context, conf *ServiceConfig, namespace string) error {
	if conf == nil {
		return nil
	}

	tags, err := model.ParseTags(conf.Tags)
	if err != nil {
		return fmt.Errorf("parse tags: %v", err)
	}

	svc := d.client.CoreV1().Services(namespace)

	svcLW := &cache.ListWatch{
		ListFunc: func(options metav1.ListOptions) (runtime.Object, error) {
			options.FieldSelector = conf.Selector.Field
			options.LabelSelector = conf.Selector.Label
			return svc.List(ctx, options)
		},
		WatchFunc: func(options metav1.ListOptions) (watch.Interface, error) {
			options.FieldSelector = conf.Selector.Field
			options.LabelSelector = conf.Selector.Label
			return svc.Watch(ctx, options)
		},
	}

	inf := cache.NewSharedInformer(svcLW, &corev1.Service{}, resyncPeriod)

	td := newServiceDiscoverer(inf)
	td.Tags().Merge(tags)

	d.discoverers = append(d.discoverers, td)

	return nil
}

func enqueue(queue *workqueue.Type, obj any) {
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
