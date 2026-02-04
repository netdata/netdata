// SPDX-License-Identifier: GPL-3.0-or-later

package netflow

import (
	"context"
	_ "embed"
	"errors"
	"fmt"
	"net"
	"sync"
	"time"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/agent/module"
)

//go:embed "config_schema.json"
var configSchema string

const (
	defaultUpdateEvery    = 10
	defaultAddress        = ":2055"
	defaultMaxPacketSize  = 65535
	defaultReceiveBuffer  = 4 * 1024 * 1024
	defaultMaxBuckets     = 60
	defaultMaxKeys        = 10000
	defaultSamplingRate   = 1
)

func init() {
	module.Register("netflow", module.Creator{
		JobConfigSchema: configSchema,
		Defaults: module.Defaults{
			UpdateEvery: defaultUpdateEvery,
		},
		Create:        func() module.Module { return New() },
		Config:        func() any { return &Config{} },
		Methods:       netflowMethods,
		MethodHandler: netflowFunctionHandler,
	})
}

func New() *Collector {
	c := &Collector{
		Config: Config{
			Address: defaultAddress,
			Protocols: ProtocolsConfig{
				NetFlowV5: true,
				NetFlowV9: true,
				IPFIX:     true,
			},
			Aggregation: AggregationConfig{
				MaxPacketSize: defaultMaxPacketSize,
				ReceiveBuffer: defaultReceiveBuffer,
				MaxBuckets:    defaultMaxBuckets,
				MaxKeys:       defaultMaxKeys,
			},
			Sampling: SamplingConfig{
				DefaultRate: defaultSamplingRate,
			},
		},
		charts: &module.Charts{},
	}

	c.funcRouter = newFuncRouter(c)

	return c
}

type Collector struct {
	module.Base
	Config `yaml:",inline" json:",inline"`

	charts *module.Charts

	conn   *net.UDPConn
	ctx    context.Context
	cancel context.CancelFunc
	wg     sync.WaitGroup

	decoder    *flowDecoder
	aggregator *flowAggregator

	funcRouter *funcRouter
}

func (c *Collector) Configuration() any {
	return c.Config
}

func (c *Collector) Init(context.Context) error {
	if err := c.validateConfig(); err != nil {
		return fmt.Errorf("config validation failed: %v", err)
	}

	bucketSeconds := c.Aggregation.BucketSeconds
	if bucketSeconds <= 0 {
		bucketSeconds = c.UpdateEvery
	}

	c.aggregator = newFlowAggregator(flowAggregatorConfig{
		bucketDuration: time.Duration(bucketSeconds) * time.Second,
		maxBuckets:     c.Aggregation.MaxBuckets,
		maxKeys:        c.Aggregation.MaxKeys,
		defaultRate:    c.Sampling.DefaultRate,
		exporters:      c.Exporters,
	})

	c.decoder = newFlowDecoder(flowDecoderConfig{
		enableV5:  c.Protocols.NetFlowV5,
		enableV9:  c.Protocols.NetFlowV9,
		enableIPFIX: c.Protocols.IPFIX,
	})

	c.ctx, c.cancel = context.WithCancel(context.Background())

	if err := c.startListener(); err != nil {
		return err
	}

	if err := c.addCharts(); err != nil {
		return err
	}

	return nil
}

func (c *Collector) Check(context.Context) error {
	if c.conn == nil {
		return errors.New("netflow listener not initialized")
	}
	return nil
}

func (c *Collector) Charts() *module.Charts {
	return c.charts
}

func (c *Collector) Collect(context.Context) map[string]int64 {
	mx := c.collect()
	if len(mx) == 0 {
		return nil
	}
	return mx
}

func (c *Collector) Cleanup(context.Context) {
	if c.cancel != nil {
		c.cancel()
	}
	if c.conn != nil {
		_ = c.conn.Close()
	}
	c.wg.Wait()
	if c.funcRouter != nil {
		c.funcRouter.Cleanup(context.Background())
	}
}
