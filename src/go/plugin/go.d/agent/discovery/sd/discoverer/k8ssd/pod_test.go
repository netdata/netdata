// SPDX-License-Identifier: GPL-3.0-or-later

package k8ssd

import (
	"context"
	"net"
	"strconv"
	"testing"
	"time"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/agent/discovery/sd/model"

	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/cache"
)

func TestPodTargetGroup_Provider(t *testing.T) {
	var p podTargetGroup
	assert.NotEmpty(t, p.Provider())
}

func TestPodTargetGroup_Source(t *testing.T) {
	tests := map[string]struct {
		createSim   func() discoverySim
		wantSources []string
	}{
		"pods with multiple ports": {
			createSim: func() discoverySim {
				httpd, nginx := newHTTPDPod(), newNGINXPod()
				disc, _ := prepareAllNsPodDiscoverer(httpd, nginx)

				return discoverySim{
					td: disc,
					wantTargetGroups: []model.TargetGroup{
						preparePodTargetGroup(httpd),
						preparePodTargetGroup(nginx),
					},
				}
			},
			wantSources: []string{
				"discoverer=k8s,kind=pod,namespace=default,pod_name=httpd-dd95c4d68-5bkwl,test=test",
				"discoverer=k8s,kind=pod,namespace=default,pod_name=nginx-7cfd77469b-q6kxj,test=test",
			},
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			sim := test.createSim()

			var sources []string
			for _, tgg := range sim.run(t) {
				sources = append(sources, tgg.Source())
			}

			assert.Equal(t, test.wantSources, sources)
		})
	}
}

func TestPodTargetGroup_Targets(t *testing.T) {
	tests := map[string]struct {
		createSim   func() discoverySim
		wantTargets int
	}{
		"pods with multiple ports": {
			createSim: func() discoverySim {
				httpd, nginx := newHTTPDPod(), newNGINXPod()
				discovery, _ := prepareAllNsPodDiscoverer(httpd, nginx)

				return discoverySim{
					td: discovery,
					wantTargetGroups: []model.TargetGroup{
						preparePodTargetGroup(httpd),
						preparePodTargetGroup(nginx),
					},
				}
			},
			wantTargets: 4,
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			sim := test.createSim()

			var targets int
			for _, tgg := range sim.run(t) {
				targets += len(tgg.Targets())
			}

			assert.Equal(t, test.wantTargets, targets)
		})
	}
}

func TestPodTarget_Hash(t *testing.T) {
	tests := map[string]struct {
		createSim  func() discoverySim
		wantHashes []uint64
	}{
		"pods with multiple ports": {
			createSim: func() discoverySim {
				httpd, nginx := newHTTPDPod(), newNGINXPod()
				discovery, _ := prepareAllNsPodDiscoverer(httpd, nginx)

				return discoverySim{
					td: discovery,
					wantTargetGroups: []model.TargetGroup{
						preparePodTargetGroup(httpd),
						preparePodTargetGroup(nginx),
					},
				}
			},
			wantHashes: []uint64{
				12448367577781070857,
				14383147416909666398,
				15844848658936368667,
				10371342506191352910,
			},
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			sim := test.createSim()

			var hashes []uint64
			for _, tgg := range sim.run(t) {
				for _, tg := range tgg.Targets() {
					hashes = append(hashes, tg.Hash())
				}
			}

			assert.Equal(t, test.wantHashes, hashes)
		})
	}
}

func TestPodTarget_TUID(t *testing.T) {
	tests := map[string]struct {
		createSim func() discoverySim
		wantTUID  []string
	}{
		"pods with multiple ports": {
			createSim: func() discoverySim {
				httpd, nginx := newHTTPDPod(), newNGINXPod()
				discovery, _ := prepareAllNsPodDiscoverer(httpd, nginx)

				return discoverySim{
					td: discovery,
					wantTargetGroups: []model.TargetGroup{
						preparePodTargetGroup(httpd),
						preparePodTargetGroup(nginx),
					},
				}
			},
			wantTUID: []string{
				"default_httpd-dd95c4d68-5bkwl_httpd_tcp_80",
				"default_httpd-dd95c4d68-5bkwl_httpd_tcp_443",
				"default_nginx-7cfd77469b-q6kxj_nginx_tcp_80",
				"default_nginx-7cfd77469b-q6kxj_nginx_tcp_443",
			},
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			sim := test.createSim()

			var tuid []string
			for _, tgg := range sim.run(t) {
				for _, tg := range tgg.Targets() {
					tuid = append(tuid, tg.TUID())
				}
			}

			assert.Equal(t, test.wantTUID, tuid)
		})
	}
}

func TestNewPodDiscoverer(t *testing.T) {
	tests := map[string]struct {
		podInf    cache.SharedInformer
		cmapInf   cache.SharedInformer
		secretInf cache.SharedInformer
		wantPanic bool
	}{
		"valid informers": {
			wantPanic: false,
			podInf:    cache.NewSharedInformer(nil, &corev1.Pod{}, resyncPeriod),
			cmapInf:   cache.NewSharedInformer(nil, &corev1.ConfigMap{}, resyncPeriod),
			secretInf: cache.NewSharedInformer(nil, &corev1.Secret{}, resyncPeriod),
		},
		"nil informers": {
			wantPanic: true,
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			f := func() { newPodDiscoverer(test.podInf, test.cmapInf, test.secretInf) }

			if test.wantPanic {
				assert.Panics(t, f)
			} else {
				assert.NotPanics(t, f)
			}
		})
	}
}

func TestPodDiscoverer_String(t *testing.T) {
	var p podDiscoverer
	assert.NotEmpty(t, p.String())
}

func TestPodDiscoverer_Discover(t *testing.T) {
	tests := map[string]struct {
		createSim func() discoverySim
	}{
		"ADD: pods exist before run": {
			createSim: func() discoverySim {
				httpd, nginx := newHTTPDPod(), newNGINXPod()
				td, _ := prepareAllNsPodDiscoverer(httpd, nginx)

				return discoverySim{
					td: td,
					wantTargetGroups: []model.TargetGroup{
						preparePodTargetGroup(httpd),
						preparePodTargetGroup(nginx),
					},
				}
			},
		},
		"ADD: pods exist before run and add after sync": {
			createSim: func() discoverySim {
				httpd, nginx := newHTTPDPod(), newNGINXPod()
				disc, client := prepareAllNsPodDiscoverer(httpd)
				podClient := client.CoreV1().Pods("default")

				return discoverySim{
					td: disc,
					runAfterSync: func(ctx context.Context) {
						_, _ = podClient.Create(ctx, nginx, metav1.CreateOptions{})
					},
					wantTargetGroups: []model.TargetGroup{
						preparePodTargetGroup(httpd),
						preparePodTargetGroup(nginx),
					},
				}
			},
		},
		"DELETE: remove pods after sync": {
			createSim: func() discoverySim {
				httpd, nginx := newHTTPDPod(), newNGINXPod()
				disc, client := prepareAllNsPodDiscoverer(httpd, nginx)
				podClient := client.CoreV1().Pods("default")

				return discoverySim{
					td: disc,
					runAfterSync: func(ctx context.Context) {
						time.Sleep(time.Millisecond * 50)
						_ = podClient.Delete(ctx, httpd.Name, metav1.DeleteOptions{})
						_ = podClient.Delete(ctx, nginx.Name, metav1.DeleteOptions{})
					},
					wantTargetGroups: []model.TargetGroup{
						preparePodTargetGroup(httpd),
						preparePodTargetGroup(nginx),
						prepareEmptyPodTargetGroup(httpd),
						prepareEmptyPodTargetGroup(nginx),
					},
				}
			},
		},
		"DELETE,ADD: remove and add pods after sync": {
			createSim: func() discoverySim {
				httpd, nginx := newHTTPDPod(), newNGINXPod()
				disc, client := prepareAllNsPodDiscoverer(httpd)
				podClient := client.CoreV1().Pods("default")

				return discoverySim{
					td: disc,
					runAfterSync: func(ctx context.Context) {
						time.Sleep(time.Millisecond * 50)
						_ = podClient.Delete(ctx, httpd.Name, metav1.DeleteOptions{})
						_, _ = podClient.Create(ctx, nginx, metav1.CreateOptions{})
					},
					wantTargetGroups: []model.TargetGroup{
						preparePodTargetGroup(httpd),
						prepareEmptyPodTargetGroup(httpd),
						preparePodTargetGroup(nginx),
					},
				}
			},
		},
		"ADD: pods with empty PodIP": {
			createSim: func() discoverySim {
				httpd, nginx := newHTTPDPod(), newNGINXPod()
				httpd.Status.PodIP = ""
				nginx.Status.PodIP = ""
				disc, _ := prepareAllNsPodDiscoverer(httpd, nginx)

				return discoverySim{
					td: disc,
					wantTargetGroups: []model.TargetGroup{
						prepareEmptyPodTargetGroup(httpd),
						prepareEmptyPodTargetGroup(nginx),
					},
				}
			},
		},
		"UPDATE: set pods PodIP after sync": {
			createSim: func() discoverySim {
				httpd, nginx := newHTTPDPod(), newNGINXPod()
				httpd.Status.PodIP = ""
				nginx.Status.PodIP = ""
				disc, client := prepareAllNsPodDiscoverer(httpd, nginx)
				podClient := client.CoreV1().Pods("default")

				return discoverySim{
					td: disc,
					runAfterSync: func(ctx context.Context) {
						time.Sleep(time.Millisecond * 50)
						_, _ = podClient.Update(ctx, newHTTPDPod(), metav1.UpdateOptions{})
						_, _ = podClient.Update(ctx, newNGINXPod(), metav1.UpdateOptions{})
					},
					wantTargetGroups: []model.TargetGroup{
						prepareEmptyPodTargetGroup(httpd),
						prepareEmptyPodTargetGroup(nginx),
						preparePodTargetGroup(newHTTPDPod()),
						preparePodTargetGroup(newNGINXPod()),
					},
				}
			},
		},
		"ADD: pods without containers": {
			createSim: func() discoverySim {
				httpd, nginx := newHTTPDPod(), newNGINXPod()
				httpd.Spec.Containers = httpd.Spec.Containers[:0]
				nginx.Spec.Containers = httpd.Spec.Containers[:0]
				disc, _ := prepareAllNsPodDiscoverer(httpd, nginx)

				return discoverySim{
					td: disc,
					wantTargetGroups: []model.TargetGroup{
						prepareEmptyPodTargetGroup(httpd),
						prepareEmptyPodTargetGroup(nginx),
					},
				}
			},
		},
		"Env: from value": {
			createSim: func() discoverySim {
				httpd := newHTTPDPod()
				mangle := func(c *corev1.Container) {
					c.Env = []corev1.EnvVar{
						{Name: "key1", Value: "value1"},
					}
				}
				mangleContainers(httpd.Spec.Containers, mangle)
				data := map[string]string{"key1": "value1"}

				disc, _ := prepareAllNsPodDiscoverer(httpd)

				return discoverySim{
					td: disc,
					wantTargetGroups: []model.TargetGroup{
						preparePodTargetGroupWithEnv(httpd, data),
					},
				}
			},
		},
		"Env: from Secret": {
			createSim: func() discoverySim {
				httpd := newHTTPDPod()
				mangle := func(c *corev1.Container) {
					c.Env = []corev1.EnvVar{
						{
							Name: "key1",
							ValueFrom: &corev1.EnvVarSource{SecretKeyRef: &corev1.SecretKeySelector{
								LocalObjectReference: corev1.LocalObjectReference{Name: "my-secret"},
								Key:                  "key1",
							}},
						},
					}
				}
				mangleContainers(httpd.Spec.Containers, mangle)
				data := map[string]string{"key1": "value1"}
				secret := prepareSecret("my-secret", data)

				disc, _ := prepareAllNsPodDiscoverer(httpd, secret)

				return discoverySim{
					td: disc,
					wantTargetGroups: []model.TargetGroup{
						preparePodTargetGroupWithEnv(httpd, data),
					},
				}
			},
		},
		"Env: from ConfigMap": {
			createSim: func() discoverySim {
				httpd := newHTTPDPod()
				mangle := func(c *corev1.Container) {
					c.Env = []corev1.EnvVar{
						{
							Name: "key1",
							ValueFrom: &corev1.EnvVarSource{ConfigMapKeyRef: &corev1.ConfigMapKeySelector{
								LocalObjectReference: corev1.LocalObjectReference{Name: "my-cmap"},
								Key:                  "key1",
							}},
						},
					}
				}
				mangleContainers(httpd.Spec.Containers, mangle)
				data := map[string]string{"key1": "value1"}
				cmap := prepareConfigMap("my-cmap", data)

				disc, _ := prepareAllNsPodDiscoverer(httpd, cmap)

				return discoverySim{
					td: disc,
					wantTargetGroups: []model.TargetGroup{
						preparePodTargetGroupWithEnv(httpd, data),
					},
				}
			},
		},
		"EnvFrom: from ConfigMap": {
			createSim: func() discoverySim {
				httpd := newHTTPDPod()
				mangle := func(c *corev1.Container) {
					c.EnvFrom = []corev1.EnvFromSource{
						{
							ConfigMapRef: &corev1.ConfigMapEnvSource{
								LocalObjectReference: corev1.LocalObjectReference{Name: "my-cmap"}},
						},
					}
				}
				mangleContainers(httpd.Spec.Containers, mangle)
				data := map[string]string{"key1": "value1", "key2": "value2"}
				cmap := prepareConfigMap("my-cmap", data)

				disc, _ := prepareAllNsPodDiscoverer(httpd, cmap)

				return discoverySim{
					td: disc,
					wantTargetGroups: []model.TargetGroup{
						preparePodTargetGroupWithEnv(httpd, data),
					},
				}
			},
		},
		"EnvFrom: from Secret": {
			createSim: func() discoverySim {
				httpd := newHTTPDPod()
				mangle := func(c *corev1.Container) {
					c.EnvFrom = []corev1.EnvFromSource{
						{
							SecretRef: &corev1.SecretEnvSource{
								LocalObjectReference: corev1.LocalObjectReference{Name: "my-secret"}},
						},
					}
				}
				mangleContainers(httpd.Spec.Containers, mangle)
				data := map[string]string{"key1": "value1", "key2": "value2"}
				secret := prepareSecret("my-secret", data)

				disc, _ := prepareAllNsPodDiscoverer(httpd, secret)

				return discoverySim{
					td: disc,
					wantTargetGroups: []model.TargetGroup{
						preparePodTargetGroupWithEnv(httpd, data),
					},
				}
			},
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			sim := test.createSim()
			sim.run(t)
		})
	}
}

func prepareAllNsPodDiscoverer(objects ...runtime.Object) (*KubeDiscoverer, kubernetes.Interface) {
	return prepareDiscoverer(rolePod, []string{corev1.NamespaceAll}, objects...)
}

func preparePodDiscoverer(namespaces []string, objects ...runtime.Object) (*KubeDiscoverer, kubernetes.Interface) {
	return prepareDiscoverer(rolePod, namespaces, objects...)
}

func mangleContainers(containers []corev1.Container, mange func(container *corev1.Container)) {
	for i := range containers {
		mange(&containers[i])
	}
}

var controllerTrue = true

func newHTTPDPod() *corev1.Pod {
	return &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:        "httpd-dd95c4d68-5bkwl",
			Namespace:   "default",
			UID:         "1cebb6eb-0c1e-495b-8131-8fa3e6668dc8",
			Annotations: map[string]string{"phase": "prod"},
			Labels:      map[string]string{"app": "httpd", "tier": "frontend"},
			OwnerReferences: []metav1.OwnerReference{
				{Name: "netdata-test", Kind: "DaemonSet", Controller: &controllerTrue},
			},
		},
		Spec: corev1.PodSpec{
			NodeName: "m01",
			Containers: []corev1.Container{
				{
					Name:  "httpd",
					Image: "httpd",
					Ports: []corev1.ContainerPort{
						{Name: "http", Protocol: corev1.ProtocolTCP, ContainerPort: 80},
						{Name: "https", Protocol: corev1.ProtocolTCP, ContainerPort: 443},
					},
				},
			},
		},
		Status: corev1.PodStatus{
			PodIP: "172.17.0.1",
		},
	}
}

func newNGINXPod() *corev1.Pod {
	return &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:        "nginx-7cfd77469b-q6kxj",
			Namespace:   "default",
			UID:         "09e883f2-d740-4c5f-970d-02cf02876522",
			Annotations: map[string]string{"phase": "prod"},
			Labels:      map[string]string{"app": "nginx", "tier": "frontend"},
			OwnerReferences: []metav1.OwnerReference{
				{Name: "netdata-test", Kind: "DaemonSet", Controller: &controllerTrue},
			},
		},
		Spec: corev1.PodSpec{
			NodeName: "m01",
			Containers: []corev1.Container{
				{
					Name:  "nginx",
					Image: "nginx",
					Ports: []corev1.ContainerPort{
						{Name: "http", Protocol: corev1.ProtocolTCP, ContainerPort: 80},
						{Name: "https", Protocol: corev1.ProtocolTCP, ContainerPort: 443},
					},
				},
			},
		},
		Status: corev1.PodStatus{
			PodIP: "172.17.0.2",
		},
	}
}

func prepareConfigMap(name string, data map[string]string) *corev1.ConfigMap {
	return &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: "default",
			UID:       types.UID("a03b8dc6-dc40-46dc-b571-5030e69d8167" + name),
		},
		Data: data,
	}
}

func prepareSecret(name string, data map[string]string) *corev1.Secret {
	secretData := make(map[string][]byte, len(data))
	for k, v := range data {
		secretData[k] = []byte(v)
	}
	return &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: "default",
			UID:       types.UID("a03b8dc6-dc40-46dc-b571-5030e69d8161" + name),
		},
		Data: secretData,
	}
}

func prepareEmptyPodTargetGroup(pod *corev1.Pod) *podTargetGroup {
	tgg := &podTargetGroup{source: podSource(pod)}
	tgg.source += ",test=test"
	return tgg
}

func preparePodTargetGroup(pod *corev1.Pod) *podTargetGroup {
	tgg := prepareEmptyPodTargetGroup(pod)

	for _, container := range pod.Spec.Containers {
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
				ControllerName: "netdata-test",
				ControllerKind: "DaemonSet",
				ContName:       container.Name,
				Image:          container.Image,
				Env:            nil,
				Port:           portNum,
				PortName:       port.Name,
				PortProtocol:   string(port.Protocol),
			}
			tgt.hash = mustCalcHash(tgt)
			tgt.Tags().Merge(discoveryTags)

			tgg.targets = append(tgg.targets, tgt)
		}
	}

	return tgg
}

func preparePodTargetGroupWithEnv(pod *corev1.Pod, env map[string]string) *podTargetGroup {
	tgg := preparePodTargetGroup(pod)

	for _, tgt := range tgg.Targets() {
		tgt.(*PodTarget).Env = mapAny(env)
		tgt.(*PodTarget).hash = mustCalcHash(tgt)
	}

	return tgg
}
