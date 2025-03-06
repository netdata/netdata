// SPDX-License-Identifier: GPL-3.0-or-later

package k8ssd

import (
	"fmt"
	"os"
	"testing"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/agent/discovery/sd/model"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/k8sclient"

	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/kubernetes/fake"
)

var discoveryTags, _ = model.ParseTags("k8s")

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
		"pod role config": {
			wantErr: false,
			cfg:     Config{Role: string(rolePod), Tags: "k8s"},
		},
		"service role config": {
			wantErr: false,
			cfg:     Config{Role: string(roleService), Tags: "k8s"},
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

	tests := map[string]struct {
		createSim func() discoverySim
	}{
		"multiple namespaces pod td": {
			createSim: func() discoverySim {
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
		},
		"multiple namespaces ClusterIP service td": {
			createSim: func() discoverySim {
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
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			sim := test.createSim()
			sim.run(t)
		})
	}
}

func prepareDiscoverer(role role, namespaces []string, objects ...runtime.Object) (*KubeDiscoverer, kubernetes.Interface) {
	client := fake.NewClientset(objects...)
	tags, _ := model.ParseTags("k8s")
	disc := &KubeDiscoverer{
		cfgSource:   "test=test",
		tags:        tags,
		role:        role,
		namespaces:  namespaces,
		client:      client,
		discoverers: nil,
		started:     make(chan struct{}),
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
