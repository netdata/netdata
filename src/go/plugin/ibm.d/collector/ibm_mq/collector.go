//go:build ignore
// +build ignore

package ibm_mq

// #cgo CFLAGS: -I/opt/mqm/inc
// #cgo LDFLAGS: -L/opt/mqm/lib64 -lmqm
import "C"

import (
	"context"
	_ "embed"
	"sync"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/agent/module"
)

//go:embed "config_schema.json"
var configSchema string

const (
	// Set to the minimum required version of the monitored application.
	// If the version is lower than this, the collector will not run.
	minVersion = "9.1"
	// Set to the name of the collector.
	collectorName = "ibm_mq"
)

// Config is the configuration for the collector.
type Config struct {
	Channel       string `yaml:"channel"`
	QueueManager  string `yaml:"queue_manager"`
	Host          string `yaml:"host"`
	Port          int    `yaml:"port"`
	User          string `yaml:"user"`
	Password      string `yaml:"password"`
	module.Module `yaml:",inline"`
}

// Collector is the collector type.
type Collector struct {
	conf Config
	module.Base
	sync.RWMutex
}

// New creates a new collector.
func New() *Collector {
	return &Collector{}
}

// Init is called once when the collector is created.
func (c *Collector) Init(ctx context.Context) error {
	return nil
}

// Check is called once when the collector is created.
func (c *Collector) Check(ctx context.Context) (bool, error) {
	return true, nil
}

// Collect is called to collect metrics.
func (c *Collector) Collect(ctx context.Context) map[string]int64 {
	mx := make(map[string]int64)

	qMgr, err := c.connect()
	if err != nil {
		c.Error(err)
		return nil
	}
	defer qMgr.Disc()

	if err := c.collectQueueMetrics(qMgr, mx); err != nil {
		c.Error(err)
	}

	if err := c.collectChannelMetrics(qMgr, mx); err != nil {
		c.Error(err)
	}

	if err := c.collectQueueManagerMetrics(qMgr, mx); err != nil {
		c.Error(err)
	}

	return mx
}

func init() {
	module.Register(collectorName, module.Creator{
		JobConfigSchema: configSchema,
		Create: func() module.Module {
			return New()
		},
		Config: func() any { return &Config{} },
	})
}
