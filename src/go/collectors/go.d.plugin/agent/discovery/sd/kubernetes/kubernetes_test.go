// SPDX-License-Identifier: GPL-3.0-or-later

package kubernetes

import (
	"fmt"
	"os"
	"testing"

	"github.com/netdata/go.d.plugin/agent/discovery/sd/model"
	"github.com/netdata/go.d.plugin/pkg/k8sclient"

	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/kubernetes/fake"
)

var discoveryTags model.Tags = map[string]struct{}{"k8s": {}}

func TestMain(m *testing.M) {
	_ = os.Setenv(envNodeName, "m01")
	_ = os.Setenv(k8sclient.EnvFakeClient, "true")
	code := m.Run()
	_ = os.Unsetenv(envNodeName)
	_ = os.Unsetenv(k8sclient.EnvFakeClient)
	os.Exit(code)
}

func TestNewKubeDiscoverer(t *testing.T) {
	tests := map[string]struct {
		cfg     Config
		wantErr bool
	}{
		"pod and service config": {
			wantErr: false,
			cfg:     Config{Pod: &PodConfig{}, Service: &ServiceConfig{}},
		},
		"pod config": {
			wantErr: false,
			cfg:     Config{Pod: &PodConfig{}},
		},
		"service config": {
			wantErr: false,
			cfg:     Config{Service: &ServiceConfig{}},
		},
		"empty config": {
			wantErr: true,
			cfg:     Config{},
		},
	}
	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			disc, err := NewKubeDiscoverer(test.cfg)

			if test.wantErr {
				assert.Error(t, err)
				assert.Nil(t, disc)
			} else {
				assert.NoError(t, err)
				assert.NotNil(t, disc)
			}
		})
	}
}

func TestKubeDiscoverer_Discover(t *testing.T) {
	const prod = "prod"
	const dev = "dev"
	prodNamespace := newNamespace(prod)
	devNamespace := newNamespace(dev)

	tests := map[string]func() discoverySim{
		"multiple namespaces pod td": func() discoverySim {
			httpdProd, nginxProd := newHTTPDPod(), newNGINXPod()
			httpdProd.Namespace = prod
			nginxProd.Namespace = prod

			httpdDev, nginxDev := newHTTPDPod(), newNGINXPod()
			httpdDev.Namespace = dev
			nginxDev.Namespace = dev

			disc, _ := preparePodDiscoverer(
				[]string{prod, dev},
				prodNamespace, devNamespace, httpdProd, nginxProd, httpdDev, nginxDev)

			return discoverySim{
				td:               disc,
				sortBeforeVerify: true,
				wantTargetGroups: []model.TargetGroup{
					preparePodTargetGroup(httpdDev),
					preparePodTargetGroup(nginxDev),
					preparePodTargetGroup(httpdProd),
					preparePodTargetGroup(nginxProd),
				},
			}
		},
		"multiple namespaces ClusterIP service td": func() discoverySim {
			httpdProd, nginxProd := newHTTPDClusterIPService(), newNGINXClusterIPService()
			httpdProd.Namespace = prod
			nginxProd.Namespace = prod

			httpdDev, nginxDev := newHTTPDClusterIPService(), newNGINXClusterIPService()
			httpdDev.Namespace = dev
			nginxDev.Namespace = dev

			disc, _ := prepareSvcDiscoverer(
				[]string{prod, dev},
				prodNamespace, devNamespace, httpdProd, nginxProd, httpdDev, nginxDev)

			return discoverySim{
				td:               disc,
				sortBeforeVerify: true,
				wantTargetGroups: []model.TargetGroup{
					prepareSvcTargetGroup(httpdDev),
					prepareSvcTargetGroup(nginxDev),
					prepareSvcTargetGroup(httpdProd),
					prepareSvcTargetGroup(nginxProd),
				},
			}
		},
	}

	for name, createSim := range tests {
		t.Run(name, func(t *testing.T) {
			sim := createSim()
			sim.run(t)
		})
	}
}

func prepareDiscoverer(role string, namespaces []string, objects ...runtime.Object) (*KubeDiscoverer, kubernetes.Interface) {
	client := fake.NewSimpleClientset(objects...)
	disc := &KubeDiscoverer{
		namespaces:  namespaces,
		client:      client,
		discoverers: nil,
		started:     make(chan struct{}),
	}
	switch role {
	case "pod":
		disc.podConf = &PodConfig{Tags: "k8s"}
	case "svc":
		disc.svcConf = &ServiceConfig{Tags: "k8s"}
	}
	return disc, client
}

func newNamespace(name string) *corev1.Namespace {
	return &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: name}}
}

func mustCalcHash(obj any) uint64 {
	hash, err := calcHash(obj)
	if err != nil {
		panic(fmt.Sprintf("hash calculation: %v", err))
	}
	return hash
}
