// SPDX-License-Identifier: GPL-3.0-or-later

package k8s_state

import (
	"context"
	_ "embed"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/agent/module"

	"k8s.io/client-go/kubernetes"
)

//go:embed "config_schema.json"
var configSchema string

func init() {
	module.Register("k8s_state", module.Creator{
		JobConfigSchema: configSchema,
		Defaults: module.Defaults{
			Disabled: true,
		},
		Create: func() module.Module { return New() },
		Config: func() any { return &Config{} },
	})
}

func New() *KubeState {
	return &KubeState{
		initDelay:     time.Second * 3,
		newKubeClient: newKubeClient,
		charts:        baseCharts.Copy(),
		once:          &sync.Once{},
		wg:            &sync.WaitGroup{},
		state:         newKubeState(),
	}
}

type Config struct {
	UpdateEvery int `yaml:"update_every,omitempty" json:"update_every"`
}

type (
	KubeState struct {
		module.Base
		Config `yaml:",inline" json:""`

		charts *module.Charts

		client        kubernetes.Interface
		newKubeClient func() (kubernetes.Interface, error)

		startTime       time.Time
		initDelay       time.Duration
		once            *sync.Once
		wg              *sync.WaitGroup
		discoverer      discoverer
		ctx             context.Context
		ctxCancel       context.CancelFunc
		kubeClusterID   string
		kubeClusterName string

		state *kubeState
	}
	discoverer interface {
		run(ctx context.Context, in chan<- resource)
		ready() bool
		stopped() bool
	}
)

func (ks *KubeState) Configuration() any {
	return ks.Config
}

func (ks *KubeState) Init() error {
	client, err := ks.initClient()
	if err != nil {
		ks.Errorf("client initialization: %v", err)
		return err
	}
	ks.client = client

	ks.ctx, ks.ctxCancel = context.WithCancel(context.Background())

	ks.discoverer = ks.initDiscoverer(ks.client)

	return nil
}

func (ks *KubeState) Check() error {
	if ks.client == nil || ks.discoverer == nil {
		ks.Error("not initialized job")
		return errors.New("not initialized")
	}

	ver, err := ks.client.Discovery().ServerVersion()
	if err != nil {
		err := fmt.Errorf("failed to connect to K8s API server: %v", err)
		ks.Error(err)
		return err
	}

	ks.Infof("successfully connected to the Kubernetes API server '%s'", ver)

	return nil
}

func (ks *KubeState) Charts() *module.Charts {
	return ks.charts
}

func (ks *KubeState) Collect() map[string]int64 {
	ms, err := ks.collect()
	if err != nil {
		ks.Error(err)
	}

	if len(ms) == 0 {
		return nil
	}
	return ms
}

func (ks *KubeState) Cleanup() {
	if ks.ctxCancel == nil {
		return
	}
	ks.ctxCancel()

	c := make(chan struct{})
	go func() { defer close(c); ks.wg.Wait() }()

	t := time.NewTimer(time.Second * 5)
	defer t.Stop()

	select {
	case <-c:
		return
	case <-t.C:
		return
	}
}
