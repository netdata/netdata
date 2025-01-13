// SPDX-License-Identifier: GPL-3.0-or-later

package kubernetes

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
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/cache"
)

func TestServiceTargetGroup_Provider(t *testing.T) {
	var s serviceTargetGroup
	assert.NotEmpty(t, s.Provider())
}

func TestServiceTargetGroup_Source(t *testing.T) {
	tests := map[string]struct {
		createSim   func() discoverySim
		wantSources []string
	}{
		"ClusterIP svc with multiple ports": {
			createSim: func() discoverySim {
				httpd, nginx := newHTTPDClusterIPService(), newNGINXClusterIPService()
				disc, _ := prepareAllNsSvcDiscoverer(httpd, nginx)

				return discoverySim{
					td: disc,
					wantTargetGroups: []model.TargetGroup{
						prepareSvcTargetGroup(httpd),
						prepareSvcTargetGroup(nginx),
					},
				}
			},
			wantSources: []string{
				"discoverer=k8s,kind=service,namespace=default,service_name=httpd-cluster-ip-service",
				"discoverer=k8s,kind=service,namespace=default,service_name=nginx-cluster-ip-service",
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

func TestServiceTargetGroup_Targets(t *testing.T) {
	tests := map[string]struct {
		createSim   func() discoverySim
		wantTargets int
	}{
		"ClusterIP svc with multiple ports": {
			createSim: func() discoverySim {
				httpd, nginx := newHTTPDClusterIPService(), newNGINXClusterIPService()
				disc, _ := prepareAllNsSvcDiscoverer(httpd, nginx)

				return discoverySim{
					td: disc,
					wantTargetGroups: []model.TargetGroup{
						prepareSvcTargetGroup(httpd),
						prepareSvcTargetGroup(nginx),
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

func TestServiceTarget_Hash(t *testing.T) {
	tests := map[string]struct {
		createSim  func() discoverySim
		wantHashes []uint64
	}{
		"ClusterIP svc with multiple ports": {
			createSim: func() discoverySim {
				httpd, nginx := newHTTPDClusterIPService(), newNGINXClusterIPService()
				disc, _ := prepareAllNsSvcDiscoverer(httpd, nginx)

				return discoverySim{
					td: disc,
					wantTargetGroups: []model.TargetGroup{
						prepareSvcTargetGroup(httpd),
						prepareSvcTargetGroup(nginx),
					},
				}
			},
			wantHashes: []uint64{
				7927590792385601508,
				11876820484195315354,
				12042303898632345558,
				12464836168762314838,
			},
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			sim := test.createSim()

			var hashes []uint64
			for _, tgg := range sim.run(t) {
				for _, tgt := range tgg.Targets() {
					hashes = append(hashes, tgt.Hash())
				}
			}

			assert.Equal(t, test.wantHashes, hashes)
		})
	}
}

func TestServiceTarget_TUID(t *testing.T) {
	tests := map[string]struct {
		createSim func() discoverySim
		wantTUID  []string
	}{
		"ClusterIP svc with multiple ports": {
			createSim: func() discoverySim {
				httpd, nginx := newHTTPDClusterIPService(), newNGINXClusterIPService()
				disc, _ := prepareAllNsSvcDiscoverer(httpd, nginx)

				return discoverySim{
					td: disc,
					wantTargetGroups: []model.TargetGroup{
						prepareSvcTargetGroup(httpd),
						prepareSvcTargetGroup(nginx),
					},
				}
			},
			wantTUID: []string{
				"default_httpd-cluster-ip-service_tcp_80",
				"default_httpd-cluster-ip-service_tcp_443",
				"default_nginx-cluster-ip-service_tcp_80",
				"default_nginx-cluster-ip-service_tcp_443",
			},
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			sim := test.createSim()

			var tuid []string
			for _, tgg := range sim.run(t) {
				for _, tgt := range tgg.Targets() {
					tuid = append(tuid, tgt.TUID())
				}
			}

			assert.Equal(t, test.wantTUID, tuid)
		})
	}
}

func TestNewServiceDiscoverer(t *testing.T) {
	tests := map[string]struct {
		informer  cache.SharedInformer
		wantPanic bool
	}{
		"valid informer": {
			wantPanic: false,
			informer:  cache.NewSharedInformer(nil, &corev1.Service{}, resyncPeriod),
		},
		"nil informer": {
			wantPanic: true,
			informer:  nil,
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			f := func() { newServiceDiscoverer(test.informer) }

			if test.wantPanic {
				assert.Panics(t, f)
			} else {
				assert.NotPanics(t, f)
			}
		})
	}
}

func TestServiceDiscoverer_String(t *testing.T) {
	var s serviceDiscoverer
	assert.NotEmpty(t, s.String())
}

func TestServiceDiscoverer_Discover(t *testing.T) {
	tests := map[string]struct {
		createSim func() discoverySim
	}{
		"ADD: ClusterIP svc exist before run": {
			createSim: func() discoverySim {
				httpd, nginx := newHTTPDClusterIPService(), newNGINXClusterIPService()
				disc, _ := prepareAllNsSvcDiscoverer(httpd, nginx)

				return discoverySim{
					td: disc,
					wantTargetGroups: []model.TargetGroup{
						prepareSvcTargetGroup(httpd),
						prepareSvcTargetGroup(nginx),
					},
				}
			},
		},
		"ADD: ClusterIP svc exist before run and add after sync": {
			createSim: func() discoverySim {
				httpd, nginx := newHTTPDClusterIPService(), newNGINXClusterIPService()
				disc, client := prepareAllNsSvcDiscoverer(httpd)
				svcClient := client.CoreV1().Services("default")

				return discoverySim{
					td: disc,
					runAfterSync: func(ctx context.Context) {
						_, _ = svcClient.Create(ctx, nginx, metav1.CreateOptions{})
					},
					wantTargetGroups: []model.TargetGroup{
						prepareSvcTargetGroup(httpd),
						prepareSvcTargetGroup(nginx),
					},
				}
			},
		},
		"DELETE: ClusterIP svc remove after sync": {
			createSim: func() discoverySim {
				httpd, nginx := newHTTPDClusterIPService(), newNGINXClusterIPService()
				disc, client := prepareAllNsSvcDiscoverer(httpd, nginx)
				svcClient := client.CoreV1().Services("default")

				return discoverySim{
					td: disc,
					runAfterSync: func(ctx context.Context) {
						time.Sleep(time.Millisecond * 50)
						_ = svcClient.Delete(ctx, httpd.Name, metav1.DeleteOptions{})
						_ = svcClient.Delete(ctx, nginx.Name, metav1.DeleteOptions{})
					},
					wantTargetGroups: []model.TargetGroup{
						prepareSvcTargetGroup(httpd),
						prepareSvcTargetGroup(nginx),
						prepareEmptySvcTargetGroup(httpd),
						prepareEmptySvcTargetGroup(nginx),
					},
				}
			},
		},
		"ADD,DELETE: ClusterIP svc remove and add after sync": {
			createSim: func() discoverySim {
				httpd, nginx := newHTTPDClusterIPService(), newNGINXClusterIPService()
				disc, client := prepareAllNsSvcDiscoverer(httpd)
				svcClient := client.CoreV1().Services("default")

				return discoverySim{
					td: disc,
					runAfterSync: func(ctx context.Context) {
						time.Sleep(time.Millisecond * 50)
						_ = svcClient.Delete(ctx, httpd.Name, metav1.DeleteOptions{})
						_, _ = svcClient.Create(ctx, nginx, metav1.CreateOptions{})
					},
					wantTargetGroups: []model.TargetGroup{
						prepareSvcTargetGroup(httpd),
						prepareEmptySvcTargetGroup(httpd),
						prepareSvcTargetGroup(nginx),
					},
				}
			},
		},
		"ADD: Headless svc exist before run": {
			createSim: func() discoverySim {
				httpd, nginx := newHTTPDHeadlessService(), newNGINXHeadlessService()
				disc, _ := prepareAllNsSvcDiscoverer(httpd, nginx)

				return discoverySim{
					td: disc,
					wantTargetGroups: []model.TargetGroup{
						prepareEmptySvcTargetGroup(httpd),
						prepareEmptySvcTargetGroup(nginx),
					},
				}
			},
		},
		"UPDATE: Headless => ClusterIP svc after sync": {
			createSim: func() discoverySim {
				httpd, nginx := newHTTPDHeadlessService(), newNGINXHeadlessService()
				httpdUpd, nginxUpd := *httpd, *nginx
				httpdUpd.Spec.ClusterIP = "10.100.0.1"
				nginxUpd.Spec.ClusterIP = "10.100.0.2"
				disc, client := prepareAllNsSvcDiscoverer(httpd, nginx)
				svcClient := client.CoreV1().Services("default")

				return discoverySim{
					td: disc,
					runAfterSync: func(ctx context.Context) {
						time.Sleep(time.Millisecond * 50)
						_, _ = svcClient.Update(ctx, &httpdUpd, metav1.UpdateOptions{})
						_, _ = svcClient.Update(ctx, &nginxUpd, metav1.UpdateOptions{})
					},
					wantTargetGroups: []model.TargetGroup{
						prepareEmptySvcTargetGroup(httpd),
						prepareEmptySvcTargetGroup(nginx),
						prepareSvcTargetGroup(&httpdUpd),
						prepareSvcTargetGroup(&nginxUpd),
					},
				}
			},
		},
		"ADD: ClusterIP svc with zero exposed ports": {
			createSim: func() discoverySim {
				httpd, nginx := newHTTPDClusterIPService(), newNGINXClusterIPService()
				httpd.Spec.Ports = httpd.Spec.Ports[:0]
				nginx.Spec.Ports = httpd.Spec.Ports[:0]
				disc, _ := prepareAllNsSvcDiscoverer(httpd, nginx)

				return discoverySim{
					td: disc,
					wantTargetGroups: []model.TargetGroup{
						prepareEmptySvcTargetGroup(httpd),
						prepareEmptySvcTargetGroup(nginx),
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

func prepareAllNsSvcDiscoverer(objects ...runtime.Object) (*KubeDiscoverer, kubernetes.Interface) {
	return prepareDiscoverer(roleService, []string{corev1.NamespaceAll}, objects...)
}

func prepareSvcDiscoverer(namespaces []string, objects ...runtime.Object) (*KubeDiscoverer, kubernetes.Interface) {
	return prepareDiscoverer(roleService, namespaces, objects...)
}

func newHTTPDClusterIPService() *corev1.Service {
	return &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:        "httpd-cluster-ip-service",
			Namespace:   "default",
			Annotations: map[string]string{"phase": "prod"},
			Labels:      map[string]string{"app": "httpd", "tier": "frontend"},
		},
		Spec: corev1.ServiceSpec{
			Ports: []corev1.ServicePort{
				{Name: "http", Protocol: corev1.ProtocolTCP, Port: 80},
				{Name: "https", Protocol: corev1.ProtocolTCP, Port: 443},
			},
			Type:      corev1.ServiceTypeClusterIP,
			ClusterIP: "10.100.0.1",
			Selector:  map[string]string{"app": "httpd", "tier": "frontend"},
		},
	}
}

func newNGINXClusterIPService() *corev1.Service {
	return &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:        "nginx-cluster-ip-service",
			Namespace:   "default",
			Annotations: map[string]string{"phase": "prod"},
			Labels:      map[string]string{"app": "nginx", "tier": "frontend"},
		},
		Spec: corev1.ServiceSpec{
			Ports: []corev1.ServicePort{
				{Name: "http", Protocol: corev1.ProtocolTCP, Port: 80},
				{Name: "https", Protocol: corev1.ProtocolTCP, Port: 443},
			},
			Type:      corev1.ServiceTypeClusterIP,
			ClusterIP: "10.100.0.2",
			Selector:  map[string]string{"app": "nginx", "tier": "frontend"},
		},
	}
}

func newHTTPDHeadlessService() *corev1.Service {
	svc := newHTTPDClusterIPService()
	svc.Name = "httpd-headless-service"
	svc.Spec.ClusterIP = ""
	return svc
}

func newNGINXHeadlessService() *corev1.Service {
	svc := newNGINXClusterIPService()
	svc.Name = "nginx-headless-service"
	svc.Spec.ClusterIP = ""
	return svc
}

func prepareEmptySvcTargetGroup(svc *corev1.Service) *serviceTargetGroup {
	return &serviceTargetGroup{source: serviceSource(svc)}
}

func prepareSvcTargetGroup(svc *corev1.Service) *serviceTargetGroup {
	tgg := prepareEmptySvcTargetGroup(svc)

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
		tgt.hash = mustCalcHash(tgt)
		tgt.Tags().Merge(discoveryTags)
		tgg.targets = append(tgg.targets, tgt)
	}

	return tgg
}
