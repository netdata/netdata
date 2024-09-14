// SPDX-License-Identifier: GPL-3.0-or-later

package kubernetes

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

type podTargetGroup struct {
	targets []model.Target
	source  string
}

func (p podTargetGroup) Provider() string        { return "sd:k8s:pod" }
func (p podTargetGroup) Source() string          { return p.source }
func (p podTargetGroup) Targets() []model.Target { return p.targets }

type PodTarget struct {
	model.Base `hash:"ignore"`

	hash uint64
	tuid string

	Address        string
	Namespace      string
	Name           string
	Annotations    map[string]any
	Labels         map[string]any
	NodeName       string
	PodIP          string
	ControllerName string
	ControllerKind string
	ContName       string
	Image          string
	Env            map[string]any
	Port           string
	PortName       string
	PortProtocol   string
}

func (p PodTarget) Hash() uint64 { return p.hash }
func (p PodTarget) TUID() string { return p.tuid }

func newPodDiscoverer(pod, cmap, secret cache.SharedInformer) *podDiscoverer {

	if pod == nil || cmap == nil || secret == nil {
		panic("nil pod or cmap or secret informer")
	}

	queue := workqueue.NewTypedWithConfig(workqueue.TypedQueueConfig[any]{Name: "pod"})

	_, _ = pod.AddEventHandler(cache.ResourceEventHandlerFuncs{
		AddFunc:    func(obj any) { enqueue(queue, obj) },
		UpdateFunc: func(_, obj any) { enqueue(queue, obj) },
		DeleteFunc: func(obj any) { enqueue(queue, obj) },
	})

	return &podDiscoverer{
		Logger:         log,
		podInformer:    pod,
		cmapInformer:   cmap,
		secretInformer: secret,
		queue:          queue,
	}
}

type podDiscoverer struct {
	*logger.Logger
	model.Base

	podInformer    cache.SharedInformer
	cmapInformer   cache.SharedInformer
	secretInformer cache.SharedInformer
	queue          *workqueue.Typed[any]
}

func (p *podDiscoverer) String() string {
	return "sd:k8s:pod"
}

func (p *podDiscoverer) Discover(ctx context.Context, in chan<- []model.TargetGroup) {
	p.Info("instance is started")
	defer p.Info("instance is stopped")
	defer p.queue.ShutDown()

	go p.podInformer.Run(ctx.Done())
	go p.cmapInformer.Run(ctx.Done())
	go p.secretInformer.Run(ctx.Done())

	if !cache.WaitForCacheSync(ctx.Done(),
		p.podInformer.HasSynced, p.cmapInformer.HasSynced, p.secretInformer.HasSynced) {
		p.Error("failed to sync caches")
		return
	}

	go p.run(ctx, in)

	<-ctx.Done()
}

func (p *podDiscoverer) run(ctx context.Context, in chan<- []model.TargetGroup) {
	for {
		item, shutdown := p.queue.Get()
		if shutdown {
			return
		}
		p.handleQueueItem(ctx, in, item)
	}
}

func (p *podDiscoverer) handleQueueItem(ctx context.Context, in chan<- []model.TargetGroup, item any) {
	defer p.queue.Done(item)

	key := item.(string)
	namespace, name, err := cache.SplitMetaNamespaceKey(key)
	if err != nil {
		return
	}

	obj, ok, err := p.podInformer.GetStore().GetByKey(key)
	if err != nil {
		return
	}

	if !ok {
		tgg := &podTargetGroup{source: podSourceFromNsName(namespace, name)}
		send(ctx, in, tgg)
		return
	}

	pod, err := toPod(obj)
	if err != nil {
		return
	}

	tgg := p.buildTargetGroup(pod)

	for _, tgt := range tgg.Targets() {
		tgt.Tags().Merge(p.Tags())
	}

	send(ctx, in, tgg)

}

func (p *podDiscoverer) buildTargetGroup(pod *corev1.Pod) model.TargetGroup {
	if pod.Status.PodIP == "" || len(pod.Spec.Containers) == 0 {
		return &podTargetGroup{
			source: podSource(pod),
		}
	}
	return &podTargetGroup{
		source:  podSource(pod),
		targets: p.buildTargets(pod),
	}
}

func (p *podDiscoverer) buildTargets(pod *corev1.Pod) (targets []model.Target) {
	var name, kind string
	for _, ref := range pod.OwnerReferences {
		if ref.Controller != nil && *ref.Controller {
			name = ref.Name
			kind = ref.Kind
			break
		}
	}

	for _, container := range pod.Spec.Containers {
		env := p.collectEnv(pod.Namespace, container)

		if len(container.Ports) == 0 {
			tgt := &PodTarget{
				tuid:           podTUID(pod, container),
				Address:        pod.Status.PodIP,
				Namespace:      pod.Namespace,
				Name:           pod.Name,
				Annotations:    mapAny(pod.Annotations),
				Labels:         mapAny(pod.Labels),
				NodeName:       pod.Spec.NodeName,
				PodIP:          pod.Status.PodIP,
				ControllerName: name,
				ControllerKind: kind,
				ContName:       container.Name,
				Image:          container.Image,
				Env:            mapAny(env),
			}
			hash, err := calcHash(tgt)
			if err != nil {
				continue
			}
			tgt.hash = hash

			targets = append(targets, tgt)
		} else {
			for _, port := range container.Ports {
				portNum := strconv.FormatUint(uint64(port.ContainerPort), 10)
				tgt := &PodTarget{
					tuid:           podTUIDWithPort(pod, container, port),
					Address:        net.JoinHostPort(pod.Status.PodIP, portNum),
					Namespace:      pod.Namespace,
					Name:           pod.Name,
					Annotations:    mapAny(pod.Annotations),
					Labels:         mapAny(pod.Labels),
					NodeName:       pod.Spec.NodeName,
					PodIP:          pod.Status.PodIP,
					ControllerName: name,
					ControllerKind: kind,
					ContName:       container.Name,
					Image:          container.Image,
					Env:            mapAny(env),
					Port:           portNum,
					PortName:       port.Name,
					PortProtocol:   string(port.Protocol),
				}
				hash, err := calcHash(tgt)
				if err != nil {
					continue
				}
				tgt.hash = hash

				targets = append(targets, tgt)
			}
		}
	}

	return targets
}

func (p *podDiscoverer) collectEnv(ns string, container corev1.Container) map[string]string {
	vars := make(map[string]string)

	// When a key exists in multiple sources,
	// the value associated with the last source will take precedence.
	// Values defined by an Env with a duplicate key will take precedence.
	//
	// Order (https://github.com/kubernetes/kubectl/blob/master/pkg/describe/describe.go)
	// - envFrom: configMapRef, secretRef
	// - env: value || valueFrom: fieldRef, resourceFieldRef, secretRef, configMap

	for _, src := range container.EnvFrom {
		switch {
		case src.ConfigMapRef != nil:
			p.envFromConfigMap(vars, ns, src)
		case src.SecretRef != nil:
			p.envFromSecret(vars, ns, src)
		}
	}

	for _, env := range container.Env {
		if env.Name == "" || isVar(env.Name) {
			continue
		}
		switch {
		case env.Value != "":
			vars[env.Name] = env.Value
		case env.ValueFrom != nil && env.ValueFrom.SecretKeyRef != nil:
			p.valueFromSecret(vars, ns, env)
		case env.ValueFrom != nil && env.ValueFrom.ConfigMapKeyRef != nil:
			p.valueFromConfigMap(vars, ns, env)
		}
	}

	if len(vars) == 0 {
		return nil
	}
	return vars
}

func (p *podDiscoverer) valueFromConfigMap(vars map[string]string, ns string, env corev1.EnvVar) {
	if env.ValueFrom.ConfigMapKeyRef.Name == "" || env.ValueFrom.ConfigMapKeyRef.Key == "" {
		return
	}

	sr := env.ValueFrom.ConfigMapKeyRef
	key := ns + "/" + sr.Name

	item, exist, err := p.cmapInformer.GetStore().GetByKey(key)
	if err != nil || !exist {
		return
	}

	cmap, err := toConfigMap(item)
	if err != nil {
		return
	}

	if v, ok := cmap.Data[sr.Key]; ok {
		vars[env.Name] = v
	}
}

func (p *podDiscoverer) valueFromSecret(vars map[string]string, ns string, env corev1.EnvVar) {
	if env.ValueFrom.SecretKeyRef.Name == "" || env.ValueFrom.SecretKeyRef.Key == "" {
		return
	}

	secretKey := env.ValueFrom.SecretKeyRef
	key := ns + "/" + secretKey.Name

	item, exist, err := p.secretInformer.GetStore().GetByKey(key)
	if err != nil || !exist {
		return
	}

	secret, err := toSecret(item)
	if err != nil {
		return
	}

	if v, ok := secret.Data[secretKey.Key]; ok {
		vars[env.Name] = string(v)
	}
}

func (p *podDiscoverer) envFromConfigMap(vars map[string]string, ns string, src corev1.EnvFromSource) {
	if src.ConfigMapRef.Name == "" {
		return
	}

	key := ns + "/" + src.ConfigMapRef.Name
	item, exist, err := p.cmapInformer.GetStore().GetByKey(key)
	if err != nil || !exist {
		return
	}

	cmap, err := toConfigMap(item)
	if err != nil {
		return
	}

	for k, v := range cmap.Data {
		vars[src.Prefix+k] = v
	}
}

func (p *podDiscoverer) envFromSecret(vars map[string]string, ns string, src corev1.EnvFromSource) {
	if src.SecretRef.Name == "" {
		return
	}

	key := ns + "/" + src.SecretRef.Name
	item, exist, err := p.secretInformer.GetStore().GetByKey(key)
	if err != nil || !exist {
		return
	}

	secret, err := toSecret(item)
	if err != nil {
		return
	}

	for k, v := range secret.Data {
		vars[src.Prefix+k] = string(v)
	}
}

func podTUID(pod *corev1.Pod, container corev1.Container) string {
	return fmt.Sprintf("%s_%s_%s",
		pod.Namespace,
		pod.Name,
		container.Name,
	)
}

func podTUIDWithPort(pod *corev1.Pod, container corev1.Container, port corev1.ContainerPort) string {
	return fmt.Sprintf("%s_%s_%s_%s_%s",
		pod.Namespace,
		pod.Name,
		container.Name,
		strings.ToLower(string(port.Protocol)),
		strconv.FormatUint(uint64(port.ContainerPort), 10),
	)
}

func podSourceFromNsName(namespace, name string) string {
	return fmt.Sprintf("discoverer=k8s,kind=pod,namespace=%s,pod_name=%s", namespace, name)
}

func podSource(pod *corev1.Pod) string {
	return podSourceFromNsName(pod.Namespace, pod.Name)
}

func toPod(obj any) (*corev1.Pod, error) {
	pod, ok := obj.(*corev1.Pod)
	if !ok {
		return nil, fmt.Errorf("received unexpected object type: %T", obj)
	}
	return pod, nil
}

func toConfigMap(obj any) (*corev1.ConfigMap, error) {
	cmap, ok := obj.(*corev1.ConfigMap)
	if !ok {
		return nil, fmt.Errorf("received unexpected object type: %T", obj)
	}
	return cmap, nil
}

func toSecret(obj any) (*corev1.Secret, error) {
	secret, ok := obj.(*corev1.Secret)
	if !ok {
		return nil, fmt.Errorf("received unexpected object type: %T", obj)
	}
	return secret, nil
}

func isVar(name string) bool {
	// Variable references $(VAR_NAME) are expanded using the previous defined
	// environment variables in the container and any service environment
	// variables.
	return strings.IndexByte(name, '$') != -1
}

func mapAny(src map[string]string) map[string]any {
	if src == nil {
		return nil
	}
	m := make(map[string]any, len(src))
	for k, v := range src {
		m[k] = v
	}
	return m
}
