// SPDX-License-Identifier: GPL-3.0-or-later

package hdfs

import (
	_ "embed"
	"errors"
	"strings"
	"time"

	"github.com/netdata/go.d.plugin/pkg/web"

	"github.com/netdata/go.d.plugin/agent/module"
)

//go:embed "config_schema.json"
var configSchema string

func init() {
	module.Register("hdfs", module.Creator{
		JobConfigSchema: configSchema,
		Create:          func() module.Module { return New() },
	})
}

// New creates HDFS with default values.
func New() *HDFS {
	config := Config{
		HTTP: web.HTTP{
			Request: web.Request{
				URL: "http://127.0.0.1:50070/jmx",
			},
			Client: web.Client{
				Timeout: web.Duration{Duration: time.Second}},
		},
	}

	return &HDFS{
		Config: config,
	}
}

type nodeType string

const (
	dataNodeType nodeType = "DataNode"
	nameNodeType nodeType = "NameNode"
)

// Config is the HDFS module configuration.
type Config struct {
	web.HTTP `yaml:",inline"`
}

// HDFS HDFS module.
type HDFS struct {
	module.Base
	Config `yaml:",inline"`

	nodeType
	client *client
}

// Cleanup makes cleanup.
func (HDFS) Cleanup() {}

func (h HDFS) createClient() (*client, error) {
	httpClient, err := web.NewHTTPClient(h.Client)
	if err != nil {
		return nil, err
	}

	return newClient(httpClient, h.Request), nil
}

func (h HDFS) determineNodeType() (nodeType, error) {
	var raw rawJMX
	err := h.client.doOKWithDecodeJSON(&raw)
	if err != nil {
		return "", err
	}

	if raw.isEmpty() {
		return "", errors.New("empty response")
	}

	jvm := raw.findJvm()
	if jvm == nil {
		return "", errors.New("couldn't find jvm in response")
	}

	v, ok := jvm["tag.ProcessName"]
	if !ok {
		return "", errors.New("couldn't find process name in JvmMetrics")
	}

	t := nodeType(strings.Trim(string(v), "\""))
	if t == nameNodeType || t == dataNodeType {
		return t, nil
	}
	return "", errors.New("unknown node type")
}

// Init makes initialization.
func (h *HDFS) Init() bool {
	cl, err := h.createClient()
	if err != nil {
		h.Errorf("error on creating client : %v", err)
		return false
	}
	h.client = cl

	return true
}

// Check makes check.
func (h *HDFS) Check() bool {
	t, err := h.determineNodeType()
	if err != nil {
		h.Errorf("error on node type determination : %v", err)
		return false
	}
	h.nodeType = t

	return len(h.Collect()) > 0
}

// Charts returns Charts.
func (h HDFS) Charts() *Charts {
	switch h.nodeType {
	default:
		return nil
	case nameNodeType:
		return nameNodeCharts()
	case dataNodeType:
		return dataNodeCharts()
	}
}

// Collect collects metrics.
func (h *HDFS) Collect() map[string]int64 {
	mx, err := h.collect()

	if err != nil {
		h.Error(err)
	}

	if len(mx) == 0 {
		return nil
	}

	return mx
}
