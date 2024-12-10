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

func New() *Collector {
	return &Collector{
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

type Collector struct {
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

func (c *Collector) Configuration() any {
	return c.Config
}

func (c *Collector) Init(context.Context) error {
	client, err := c.initClient()
	if err != nil {
		return fmt.Errorf("init k8s client: %v", err)
	}
	c.client = client

	c.ctx, c.ctxCancel = context.WithCancel(context.Background())

	c.discoverer = c.initDiscoverer(c.client)

	return nil
}

func (c *Collector) Check(context.Context) error {
	if c.client == nil || c.discoverer == nil {
		return errors.New("not initialized")
	}

	ver, err := c.client.Discovery().ServerVersion()
	if err != nil {
		return fmt.Errorf("failed to connect to K8s API server: %v", err)
	}

	c.Infof("successfully connected to the Kubernetes API server '%s'", ver)

	return nil
}

func (c *Collector) Charts() *module.Charts {
	return c.charts
}

func (c *Collector) Collect(context.Context) map[string]int64 {
	ms, err := c.collect()
	if err != nil {
		c.Error(err)
	}

	if len(ms) == 0 {
		return nil
	}
	return ms
}

func (c *Collector) Cleanup(context.Context) {
	if c.ctxCancel == nil {
		return
	}

	c.ctxCancel()

	done := make(chan struct{})
	go func() { defer close(done); c.wg.Wait() }()

	t := time.NewTimer(time.Second * 5)
	defer t.Stop()

	select {
	case <-done:
		return
	case <-t.C:
		return
	}
}
