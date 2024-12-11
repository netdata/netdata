// SPDX-License-Identifier: GPL-3.0-or-later

package hdfs

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/stm"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/web"
)

const (
	dataNodeType = "DataNode"
	nameNodeType = "NameNode"
)

func (c *Collector) collect() (map[string]int64, error) {
	req, err := web.NewHTTPRequest(c.RequestConfig)
	if err != nil {
		return nil, fmt.Errorf("failed to create HTTP request: %v", err)
	}

	var raw rawJMX
	if err := web.DoHTTP(c.httpClient).RequestJSON(req, &raw); err != nil {
		return nil, err
	}

	if raw.isEmpty() {
		return nil, errors.New("empty response")
	}

	mx := c.collectRawJMX(raw)

	return stm.ToMap(mx), nil
}

func (c *Collector) determineNodeType() (string, error) {
	req, err := web.NewHTTPRequest(c.RequestConfig)
	if err != nil {
		return "", fmt.Errorf("failed to create HTTP request: %v", err)
	}

	var raw rawJMX
	if err := web.DoHTTP(c.httpClient).RequestJSON(req, &raw); err != nil {
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

	t := strings.Trim(string(v), "\"")
	if t == nameNodeType || t == dataNodeType {
		return t, nil
	}
	return "", errors.New("unknown node type")
}

func (c *Collector) collectRawJMX(raw rawJMX) *metrics {
	var mx metrics
	switch c.nodeType {
	default:
		panic(fmt.Sprintf("unsupported node type : '%s'", c.nodeType))
	case nameNodeType:
		c.collectNameNode(&mx, raw)
	case dataNodeType:
		c.collectDataNode(&mx, raw)
	}
	return &mx
}

func (c *Collector) collectNameNode(mx *metrics, raw rawJMX) {
	if err := c.collectJVM(mx, raw); err != nil {
		c.Debugf("error on collecting jvm : %v", err)
	}

	if err := c.collectRPCActivity(mx, raw); err != nil {
		c.Debugf("error on collecting rpc activity : %v", err)
	}

	if err := c.collectFSNameSystem(mx, raw); err != nil {
		c.Debugf("error on collecting fs name system : %v", err)
	}
}

func (c *Collector) collectDataNode(mx *metrics, raw rawJMX) {
	if err := c.collectJVM(mx, raw); err != nil {
		c.Debugf("error on collecting jvm : %v", err)
	}

	if err := c.collectRPCActivity(mx, raw); err != nil {
		c.Debugf("error on collecting rpc activity : %v", err)
	}

	if err := c.collectFSDatasetState(mx, raw); err != nil {
		c.Debugf("error on collecting fs dataset state : %v", err)
	}

	if err := c.collectDataNodeActivity(mx, raw); err != nil {
		c.Debugf("error on collecting datanode activity state : %v", err)
	}
}

func (c *Collector) collectJVM(mx *metrics, raw rawJMX) error {
	v := raw.findJvm()
	if v == nil {
		return nil
	}

	var jvm jvmMetrics
	err := writeJSONTo(&jvm, v)
	if err != nil {
		return err
	}

	mx.Jvm = &jvm
	return nil
}

func (c *Collector) collectRPCActivity(mx *metrics, raw rawJMX) error {
	v := raw.findRPCActivity()
	if v == nil {
		return nil
	}

	var rpc rpcActivityMetrics
	err := writeJSONTo(&rpc, v)
	if err != nil {
		return err
	}

	mx.Rpc = &rpc
	return nil
}

func (c *Collector) collectFSNameSystem(mx *metrics, raw rawJMX) error {
	v := raw.findFSNameSystem()
	if v == nil {
		return nil
	}

	var fs fsNameSystemMetrics
	err := writeJSONTo(&fs, v)
	if err != nil {
		return err
	}

	fs.CapacityUsed = fs.CapacityDfsUsed + fs.CapacityUsedNonDFS

	mx.FSNameSystem = &fs
	return nil
}

func (c *Collector) collectFSDatasetState(mx *metrics, raw rawJMX) error {
	v := raw.findFSDatasetState()
	if v == nil {
		return nil
	}

	var fs fsDatasetStateMetrics
	err := writeJSONTo(&fs, v)
	if err != nil {
		return err
	}

	fs.CapacityUsed = fs.Capacity - fs.Remaining
	fs.CapacityUsedNonDFS = fs.CapacityUsed - fs.DfsUsed

	mx.FSDatasetState = &fs
	return nil
}

func (c *Collector) collectDataNodeActivity(mx *metrics, raw rawJMX) error {
	v := raw.findDataNodeActivity()
	if v == nil {
		return nil
	}

	var dna dataNodeActivityMetrics
	err := writeJSONTo(&dna, v)
	if err != nil {
		return err
	}

	mx.DataNodeActivity = &dna
	return nil
}

func writeJSONTo(dst, src any) error {
	b, err := json.Marshal(src)
	if err != nil {
		return err
	}
	return json.Unmarshal(b, dst)
}
