// SPDX-License-Identifier: GPL-3.0-or-later

package k8ssd

import (
	"context"
	"fmt"
	"net"
	"strconv"
	"strings"

	"github.com/netdata/netdata/go/plugins/logger"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/agent/discovery/sd/model"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/client-go/tools/cache"
	"k8s.io/client-go/util/workqueue"
)

type serviceTargetGroup struct {
	targets []model.Target
	source  string
}

func (s serviceTargetGroup) Provider() string        { return "sd:k8s:service" }
func (s serviceTargetGroup) Source() string          { return s.source }
func (s serviceTargetGroup) Targets() []model.Target { return s.targets }

type ServiceTarget struct {
	model.Base `hash:"ignore"`

	hash uint64
	tuid string

	Address      string
	Namespace    string
	Name         string
	Annotations  map[string]any
	Labels       map[string]any
	Port         string
	PortName     string
	PortProtocol string
	ClusterIP    string
	ExternalName string
	Type         string
}

func (s ServiceTarget) Hash() uint64 { return s.hash }
func (s ServiceTarget) TUID() string { return s.tuid }

type serviceDiscoverer struct {
	*logger.Logger
	model.Base

	informer cache.SharedInformer
	queue    *workqueue.Typed[any]
}

func newServiceDiscoverer(inf cache.SharedInformer) *serviceDiscoverer {
	if inf == nil {
		panic("nil service informer")
	}

	queue := workqueue.NewTypedWithConfig(workqueue.TypedQueueConfig[any]{Name: "service"})

	_, _ = inf.AddEventHandler(cache.ResourceEventHandlerFuncs{
		AddFunc:    func(obj any) { enqueue(queue, obj) },
		UpdateFunc: func(_, obj any) { enqueue(queue, obj) },
		DeleteFunc: func(obj any) { enqueue(queue, obj) },
	})

	return &serviceDiscoverer{
		Logger:   log,
		informer: inf,
		queue:    queue,
	}
}

func (s *serviceDiscoverer) String() string {
	return "k8s service"
}

func (s *serviceDiscoverer) Discover(ctx context.Context, ch chan<- []model.TargetGroup) {
	s.Info("instance is started")
	defer s.Info("instance is stopped")
	defer s.queue.ShutDown()

	go s.informer.Run(ctx.Done())

	if !cache.WaitForCacheSync(ctx.Done(), s.informer.HasSynced) {
		s.Error("failed to sync caches")
		return
	}

	go s.run(ctx, ch)

	<-ctx.Done()
}

func (s *serviceDiscoverer) run(ctx context.Context, in chan<- []model.TargetGroup) {
	for {
		item, shutdown := s.queue.Get()
		if shutdown {
			return
		}

		s.handleQueueItem(ctx, in, item)
	}
}

func (s *serviceDiscoverer) handleQueueItem(ctx context.Context, in chan<- []model.TargetGroup, item any) {
	defer s.queue.Done(item)

	key := item.(string)
	namespace, name, err := cache.SplitMetaNamespaceKey(key)
	if err != nil {
		return
	}

	obj, exists, err := s.informer.GetStore().GetByKey(key)
	if err != nil {
		return
	}

	if !exists {
		tgg := &serviceTargetGroup{source: serviceSourceFromNsName(namespace, name)}
		send(ctx, in, tgg)
		return
	}

	svc, err := toService(obj)
	if err != nil {
		return
	}

	tgg := s.buildTargetGroup(svc)

	for _, tgt := range tgg.Targets() {
		tgt.Tags().Merge(s.Tags())
	}

	send(ctx, in, tgg)
}

func (s *serviceDiscoverer) buildTargetGroup(svc *corev1.Service) model.TargetGroup {
	// TODO: headless service?
	if svc.Spec.ClusterIP == "" || len(svc.Spec.Ports) == 0 {
		return &serviceTargetGroup{
			source: serviceSource(svc),
		}
	}
	return &serviceTargetGroup{
		source:  serviceSource(svc),
		targets: s.buildTargets(svc),
	}
}

func (s *serviceDiscoverer) buildTargets(svc *corev1.Service) (targets []model.Target) {
	for _, port := range svc.Spec.Ports {
		portNum := strconv.FormatInt(int64(port.Port), 10)
		tgt := &ServiceTarget{
			tuid:         serviceTUID(svc, port),
			Address:      net.JoinHostPort(svc.Name+"."+svc.Namespace+".svc", portNum),
			Namespace:    svc.Namespace,
			Name:         svc.Name,
			Annotations:  mapAny(svc.Annotations),
			Labels:       mapAny(svc.Labels),
			Port:         portNum,
			PortName:     port.Name,
			PortProtocol: string(port.Protocol),
			ClusterIP:    svc.Spec.ClusterIP,
			ExternalName: svc.Spec.ExternalName,
			Type:         string(svc.Spec.Type),
		}
		hash, err := calcHash(tgt)
		if err != nil {
			continue
		}
		tgt.hash = hash

		targets = append(targets, tgt)
	}

	return targets
}

func serviceTUID(svc *corev1.Service, port corev1.ServicePort) string {
	return fmt.Sprintf("%s_%s_%s_%s",
		svc.Namespace,
		svc.Name,
		strings.ToLower(string(port.Protocol)),
		strconv.FormatInt(int64(port.Port), 10),
	)
}

func serviceSourceFromNsName(namespace, name string) string {
	return fmt.Sprintf("discoverer=k8s,kind=service,namespace=%s,service_name=%s", namespace, name)
}

func serviceSource(svc *corev1.Service) string {
	return serviceSourceFromNsName(svc.Namespace, svc.Name)
}

func toService(obj any) (*corev1.Service, error) {
	svc, ok := obj.(*corev1.Service)
	if !ok {
		return nil, fmt.Errorf("received unexpected object type: %T", obj)
	}
	return svc, nil
}
