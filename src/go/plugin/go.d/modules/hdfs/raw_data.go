package hdfs

import (
	"encoding/json"
	"strings"
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
