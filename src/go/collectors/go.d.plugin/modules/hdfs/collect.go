// SPDX-License-Identifier: GPL-3.0-or-later

package hdfs

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/netdata/go.d.plugin/pkg/stm"
)

type (
	rawData map[string]json.RawMessage
	rawJMX  struct {
		Beans []rawData
	}
)

func (r rawJMX) isEmpty() bool {
	return len(r.Beans) == 0
}

func (r rawJMX) find(f func(rawData) bool) rawData {
	for _, v := range r.Beans {
		if f(v) {
			return v
		}
	}
	return nil
}

func (r rawJMX) findJvm() rawData {
	f := func(data rawData) bool { return string(data["modelerType"]) == "\"JvmMetrics\"" }
	return r.find(f)
}

func (r rawJMX) findRPCActivity() rawData {
	f := func(data rawData) bool { return strings.HasPrefix(string(data["modelerType"]), "\"RpcActivityForPort") }
	return r.find(f)
}

func (r rawJMX) findFSNameSystem() rawData {
	f := func(data rawData) bool { return string(data["modelerType"]) == "\"FSNamesystem\"" }
	return r.find(f)
}

func (r rawJMX) findFSDatasetState() rawData {
	f := func(data rawData) bool { return string(data["modelerType"]) == "\"FSDatasetState\"" }
	return r.find(f)
}

func (r rawJMX) findDataNodeActivity() rawData {
	f := func(data rawData) bool { return strings.HasPrefix(string(data["modelerType"]), "\"DataNodeActivity") }
	return r.find(f)
}

func (h *HDFS) collect() (map[string]int64, error) {
	var raw rawJMX
	err := h.client.doOKWithDecodeJSON(&raw)
	if err != nil {
		return nil, err
	}

	if raw.isEmpty() {
		return nil, errors.New("empty response")
	}

	mx := h.collectRawJMX(raw)

	return stm.ToMap(mx), nil
}

func (h HDFS) collectRawJMX(raw rawJMX) *metrics {
	var mx metrics
	switch h.nodeType {
	default:
		panic(fmt.Sprintf("unsupported node type : '%s'", h.nodeType))
	case nameNodeType:
		h.collectNameNode(&mx, raw)
	case dataNodeType:
		h.collectDataNode(&mx, raw)
	}
	return &mx
}

func (h HDFS) collectNameNode(mx *metrics, raw rawJMX) {
	err := h.collectJVM(mx, raw)
	if err != nil {
		h.Debugf("error on collecting jvm : %v", err)
	}

	err = h.collectRPCActivity(mx, raw)
	if err != nil {
		h.Debugf("error on collecting rpc activity : %v", err)
	}

	err = h.collectFSNameSystem(mx, raw)
	if err != nil {
		h.Debugf("error on collecting fs name system : %v", err)
	}
}

func (h HDFS) collectDataNode(mx *metrics, raw rawJMX) {
	err := h.collectJVM(mx, raw)
	if err != nil {
		h.Debugf("error on collecting jvm : %v", err)
	}

	err = h.collectRPCActivity(mx, raw)
	if err != nil {
		h.Debugf("error on collecting rpc activity : %v", err)
	}

	err = h.collectFSDatasetState(mx, raw)
	if err != nil {
		h.Debugf("error on collecting fs dataset state : %v", err)
	}

	err = h.collectDataNodeActivity(mx, raw)
	if err != nil {
		h.Debugf("error on collecting datanode activity state : %v", err)
	}
}

func (h HDFS) collectJVM(mx *metrics, raw rawJMX) error {
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

func (h HDFS) collectRPCActivity(mx *metrics, raw rawJMX) error {
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

func (h HDFS) collectFSNameSystem(mx *metrics, raw rawJMX) error {
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

func (h HDFS) collectFSDatasetState(mx *metrics, raw rawJMX) error {
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

func (h HDFS) collectDataNodeActivity(mx *metrics, raw rawJMX) error {
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

func writeJSONTo(dst interface{}, src interface{}) error {
	b, err := json.Marshal(src)
	if err != nil {
		return err
	}
	return json.Unmarshal(b, dst)
}
